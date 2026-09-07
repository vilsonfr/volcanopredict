package usgs_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vilsonfr/volcanopredict/backend/internal/usgs"
)

// testClient points a real Client at a local server. Nothing here mocks
// the HTTP interface: the point is to exercise the actual client against
// real status codes, real timeouts and real truncated bodies. A mock
// would only test the mock.
func testClient(t *testing.T, srv *httptest.Server) *usgs.Client {
	t.Helper()
	c := usgs.NewClient()
	c.BaseURL = srv.URL
	c.HTTP = &http.Client{Timeout: 2 * time.Second}
	c.RetryBackoff = time.Millisecond
	return c
}

func TestFetch_Success(t *testing.T) {
	var got atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Add(1)
		if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "volcanopredict") {
			t.Errorf("the client must identify this project to the source, got User-Agent %q", ua)
		}
		fmt.Fprint(w, `{"type":"FeatureCollection","metadata":{"api":"2.7.0","count":0},"features":[]}`)
	}))
	defer srv.Close()

	body, err := testClient(t, srv).Fetch(context.Background(), usgs.Query{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("expected a body")
	}
	if n := got.Load(); n != 1 {
		t.Fatalf("expected exactly 1 request for a successful fetch, got %d", n)
	}
}

func TestFetch_RetriesServerErrorsThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `{"type":"FeatureCollection","metadata":{"api":"2.7.0","count":0},"features":[]}`)
	}))
	defer srv.Close()

	if _, err := testClient(t, srv).Fetch(context.Background(), usgs.Query{}); err != nil {
		t.Fatalf("expected the third attempt to succeed: %v", err)
	}
	if n := calls.Load(); n != 3 {
		t.Fatalf("expected 3 attempts, got %d", n)
	}
}

func TestFetch_GivesUpAfterMaxAttempts(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	c.MaxAttempts = 3
	_, err := c.Fetch(context.Background(), usgs.Query{})
	if err == nil {
		t.Fatal("expected the fetch to fail after exhausting attempts")
	}
	if !errors.Is(err, usgs.ErrSourceUnavailable) {
		t.Fatalf("expected ErrSourceUnavailable so the run records 'source down' and not 'source changed shape', got %v", err)
	}
	if n := calls.Load(); n != 3 {
		t.Fatalf("expected exactly MaxAttempts=3 requests, got %d", n)
	}
}

// A client that retries a 400 just adds load to the source: the query is
// wrong and asking again will not fix it.
func TestFetch_DoesNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := testClient(t, srv).Fetch(context.Background(), usgs.Query{})
	if err == nil {
		t.Fatal("expected a 400 to fail")
	}
	if !errors.Is(err, usgs.ErrTooManyEvents) {
		t.Fatalf("expected the service's 400 to be reported as a rejected query, got %v", err)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("a 400 must not be retried, got %d requests", n)
	}
}

func TestFetch_TimesOutAndRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	c.HTTP = &http.Client{Timeout: 50 * time.Millisecond}
	c.MaxAttempts = 2

	start := time.Now()
	_, err := c.Fetch(context.Background(), usgs.Query{})
	if err == nil {
		t.Fatal("expected the request to time out")
	}
	if !errors.Is(err, usgs.ErrSourceUnavailable) {
		t.Fatalf("a timeout is the source being unavailable, got %v", err)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("expected the timeout to be retried up to MaxAttempts=2, got %d requests", n)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("the per-request timeout must bound the wait, took %s", elapsed)
	}
}

// A body cut off mid-flight is a transport problem, not a shape problem,
// and is worth another attempt.
func TestFetch_TruncatedBodyIsRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set("Content-Length", "500")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"type":"FeatureCollection","features":[`)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			// Hijack and close so the client sees a short read.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("test server does not support hijacking")
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			conn.Close()
			return
		}
		fmt.Fprint(w, `{"type":"FeatureCollection","metadata":{"api":"2.7.0","count":0},"features":[]}`)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	if _, err := c.Fetch(context.Background(), usgs.Query{}); err != nil {
		t.Fatalf("expected the retry after a truncated body to succeed: %v", err)
	}
	if n := calls.Load(); n < 2 {
		t.Fatalf("expected the truncated response to be retried, got %d requests", n)
	}
}

// --- 3.3: query construction and paging ---------------------------------

func TestURL_WindowAndUpdatedAfterAreDistinct(t *testing.T) {
	c := usgs.NewClient()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)

	raw, err := c.URL(usgs.Query{Start: start, End: end})
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	if q.Get("starttime") != "2026-09-01T00:00:00Z" || q.Get("endtime") != "2026-09-03T00:00:00Z" {
		t.Fatalf("occurrence window not rendered: %s", u.RawQuery)
	}
	if q.Has("updatedafter") {
		t.Fatal("a window query must not also send updatedafter: they answer different questions")
	}
	if q.Get("format") != "geojson" {
		t.Fatalf("format: %q", q.Get("format"))
	}
	// Without a total order, paging could return the same event twice.
	if q.Get("orderby") == "" {
		t.Fatal("the query must impose a total order so paging is sound")
	}

	raw, err = c.URL(usgs.Query{UpdatedAfter: start})
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	u, _ = url.Parse(raw)
	if u.Query().Get("updatedafter") != "2026-09-01T00:00:00Z" {
		t.Fatalf("updatedafter not rendered: %s", u.RawQuery)
	}
}

func TestURL_OmitsMagnitudeFilterByDefault(t *testing.T) {
	raw, err := usgs.NewClient().URL(usgs.Query{})
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	u, _ := url.Parse(raw)
	if u.Query().Has("minmagnitude") {
		t.Fatal("ingestion is global by default: a magnitude floor must be opt-in, never implicit")
	}
}

// A window wider than one request must be walked without repeating or
// dropping an event.
func TestFetchPages_WalksAWindowWithoutRepeatingOrSkipping(t *testing.T) {
	const total = 25
	const pageSize = 10

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("limit") == "" || q.Get("offset") == "" {
			t.Errorf("expected the client to page with limit and offset, got %s", r.URL.RawQuery)
		}
		var limit, offset int
		fmt.Sscanf(q.Get("limit"), "%d", &limit)
		fmt.Sscanf(q.Get("offset"), "%d", &offset)

		var features []string
		for i := offset; i < offset+limit && i <= total; i++ {
			features = append(features, fmt.Sprintf(
				`{"type":"Feature","id":"ev%03d","properties":{"time":%d,"mag":1.0,"status":"automatic"},"geometry":{"type":"Point","coordinates":[0,0,10]}}`,
				i, 1788000000000+int64(i)*1000))
		}
		fmt.Fprintf(w, `{"type":"FeatureCollection","metadata":{"api":"2.7.0","count":%d},"features":[%s]}`,
			len(features), strings.Join(features, ","))
	}))
	defer srv.Close()

	pages, err := testClient(t, srv).FetchPages(context.Background(), usgs.Query{}, pageSize)
	if err != nil {
		t.Fatalf("FetchPages: %v", err)
	}

	seen := map[string]int{}
	var count int
	for _, page := range pages {
		resp, recErrs, err := usgs.Parse(page)
		if err != nil {
			t.Fatalf("parse page: %v", err)
		}
		if len(recErrs) != 0 {
			t.Fatalf("unexpected record errors: %v", recErrs)
		}
		for _, ev := range resp.Events {
			seen[ev.ID]++
			count++
		}
	}

	if count != total {
		t.Fatalf("expected every one of the %d events exactly once, got %d", total, count)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("event %s came back %d times: paging must not repeat", id, n)
		}
	}
	for i := 1; i <= total; i++ {
		if _, ok := seen[fmt.Sprintf("ev%03d", i)]; !ok {
			t.Fatalf("event ev%03d was skipped by paging", i)
		}
	}
}

func TestFetchPages_ClampsPageSizeToServiceCeiling(t *testing.T) {
	var sawLimit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawLimit = r.URL.Query().Get("limit")
		fmt.Fprint(w, `{"type":"FeatureCollection","metadata":{"api":"2.7.0","count":0},"features":[]}`)
	}))
	defer srv.Close()

	if _, err := testClient(t, srv).FetchPages(context.Background(), usgs.Query{}, 999999); err != nil {
		t.Fatalf("FetchPages: %v", err)
	}
	if sawLimit != "20000" {
		t.Fatalf("expected the page size to be clamped to the service ceiling of 20000, got %q", sawLimit)
	}
}
