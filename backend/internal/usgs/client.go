// Package usgs is the source adapter for the USGS Earthquake Hazards
// Program (ANSS ComCat), served by the FDSN Event Web Service.
//
// It is the first adapter that talks to the network at runtime, and its
// shape is meant to be the mold for the next one: fetch, parse, validate
// and normalize are separate steps, and a failure names the step it
// happened in (ingestao-fontes spec).
//
// Why FDSN and not the real-time GeoJSON feeds: the feeds have a fixed
// window and do not expose revision. An event corrected two weeks ago
// never reappears in all_day. FDSN accepts updatedafter, which answers
// "what changed since this instant" — the question an incremental
// ingestion into a bitemporal store has to ask (design.md D1).
package usgs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DefaultBaseURL is the FDSN Event Web Service query endpoint.
const DefaultBaseURL = "https://earthquake.usgs.gov/fdsnws/event/1/query"

// UserAgent identifies this project to the source. A scientific platform
// that hammers the source it depends on anonymously does not deserve the
// data.
const UserAgent = "volcanopredict/0.2 (+https://github.com/vilsonfr/volcanopredict)"

// MaxEventsPerRequest is the ceiling the FDSN service documents for a
// single query. A window expected to exceed it is walked in pages rather
// than silently truncated.
const MaxEventsPerRequest = 20000

// ErrTooManyEvents is returned by the service (HTTP 400) when a query
// would match more events than it will serve at once.
var ErrTooManyEvents = errors.New("usgs: query matches more events than the service will return at once")

// ErrSourceUnavailable is returned when the source could not be reached
// or kept failing across every allowed attempt. It is deliberately
// distinct from a parse failure: the ingestion run records which step
// broke, and "the source is down" is not "the source changed shape".
var ErrSourceUnavailable = errors.New("usgs: source unavailable")

// Client fetches from the FDSN Event Web Service.
//
// Zero value is not usable; construct with NewClient.
type Client struct {
	BaseURL string
	HTTP    *http.Client

	// MaxAttempts bounds how many times a single request is tried before
	// giving up, including the first.
	MaxAttempts int
	// RetryBackoff is the wait before the second attempt; it doubles for
	// each attempt after that.
	RetryBackoff time.Duration

	// sleep is time.Sleep in production, replaced in tests so retry
	// behavior can be asserted without spending the wall clock on it.
	sleep func(time.Duration)
}

// NewClient returns a Client with conservative defaults: a bounded
// per-request timeout, a small number of attempts, and growing waits
// between them.
func NewClient() *Client {
	return &Client{
		BaseURL:      DefaultBaseURL,
		HTTP:         &http.Client{Timeout: 60 * time.Second},
		MaxAttempts:  3,
		RetryBackoff: 2 * time.Second,
		sleep:        time.Sleep,
	}
}

// Query describes one request to the service.
//
// The two time filters answer different questions and are not
// interchangeable:
//
//   - Start/End filter on when the earthquake HAPPENED. That is what a
//     backfill wants.
//   - UpdatedAfter filters on when the SOURCE last changed the record,
//     including events that happened long before. That is what an
//     incremental run wants, and it is the only one that surfaces
//     revisions.
type Query struct {
	Start        time.Time
	End          time.Time
	UpdatedAfter time.Time

	// MinMagnitude is optional. Ingestion is global and unfiltered by
	// default (proposal.md): deciding today that small events do not
	// matter is a decision that cannot be undone later.
	MinMagnitude *float64

	// Limit and Offset walk a window that exceeds MaxEventsPerRequest.
	// Offset is 1-based, as the service defines it.
	Limit  int
	Offset int
}

// URL renders the query as the request URL the service expects.
func (c *Client) URL(q Query) (string, error) {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("usgs: invalid base URL %q: %w", base, err)
	}

	v := url.Values{}
	v.Set("format", "geojson")
	// A total order is required for paging to be sound: without it the
	// service is free to return the same event on two pages, or none.
	v.Set("orderby", "time-asc")
	if !q.Start.IsZero() {
		v.Set("starttime", q.Start.UTC().Format(time.RFC3339))
	}
	if !q.End.IsZero() {
		v.Set("endtime", q.End.UTC().Format(time.RFC3339))
	}
	if !q.UpdatedAfter.IsZero() {
		v.Set("updatedafter", q.UpdatedAfter.UTC().Format(time.RFC3339))
	}
	if q.MinMagnitude != nil {
		v.Set("minmagnitude", strconv.FormatFloat(*q.MinMagnitude, 'f', -1, 64))
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.Offset > 0 {
		v.Set("offset", strconv.Itoa(q.Offset))
	}

	u.RawQuery = v.Encode()
	return u.String(), nil
}

// Fetch performs one request and returns the raw response body.
//
// It retries on transport errors and on 5xx, with a growing wait, up to
// MaxAttempts. It does NOT retry 4xx: a malformed query does not get
// better by being asked again, and retrying it just adds load to the
// source.
func (c *Client) Fetch(ctx context.Context, q Query) ([]byte, error) {
	target, err := c.URL(q)
	if err != nil {
		return nil, err
	}

	attempts := c.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	sleep := c.sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	wait := c.RetryBackoff
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("%w: %v (after %d attempts)", ErrSourceUnavailable, ctx.Err(), attempt-1)
			default:
			}
			log.Printf("usgs: retrying request (attempt %d/%d) after %s: %v", attempt, attempts, wait, lastErr)
			sleep(wait)
			wait *= 2
		}

		body, retryable, err := c.attempt(ctx, target)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w after %d attempts: %v", ErrSourceUnavailable, attempts, lastErr)
}

// attempt performs a single request. The bool reports whether the error
// is worth another try.
func (c *Client) attempt(ctx context.Context, target string) ([]byte, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, false, fmt.Errorf("usgs: building request failed: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("usgs: request to source failed: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusNoContent:
		// The service answers 204 for a window with no matching events.
		// That is a successful answer meaning "nothing here", not a
		// failure — and conflating the two is exactly how an outage
		// would get recorded as a quiet period.
		return []byte(`{"type":"FeatureCollection","features":[]}`), false, nil
	case resp.StatusCode == http.StatusBadRequest:
		return nil, false, fmt.Errorf("usgs: source rejected the query (HTTP 400): %w", ErrTooManyEvents)
	case resp.StatusCode >= 500:
		return nil, true, fmt.Errorf("usgs: source returned HTTP %d", resp.StatusCode)
	default:
		return nil, false, fmt.Errorf("usgs: source returned unexpected HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// A truncated body is a transport problem, not a shape problem:
		// worth another attempt.
		return nil, true, fmt.Errorf("usgs: reading response body failed: %w", err)
	}
	return body, false, nil
}

// FetchPages walks a window in pages and returns every page's raw body,
// in order.
//
// Paging exists because the service refuses a query matching more than
// MaxEventsPerRequest events outright (HTTP 400) rather than truncating
// it. Truncation would be worse — it would look like success — but it
// means a wide window has to be walked deliberately.
//
// pageSize caps each request; it is clamped to MaxEventsPerRequest. The
// walk stops on the first page that comes back short, which is how the
// service signals the end.
func (c *Client) FetchPages(ctx context.Context, q Query, pageSize int) ([][]byte, error) {
	if pageSize <= 0 || pageSize > MaxEventsPerRequest {
		pageSize = MaxEventsPerRequest
	}

	var pages [][]byte
	// The service's offset is 1-based.
	offset := 1
	for {
		page := q
		page.Limit = pageSize
		page.Offset = offset

		body, err := c.Fetch(ctx, page)
		if err != nil {
			return nil, err
		}
		pages = append(pages, body)

		resp, _, err := Parse(body)
		if err != nil {
			return nil, err
		}
		// A short page is the last page. Ordering is fixed to time-asc in
		// URL(), so paging cannot reshuffle events between requests and
		// return one twice or skip one.
		if len(resp.Events) < pageSize {
			return pages, nil
		}
		offset += pageSize
	}
}
