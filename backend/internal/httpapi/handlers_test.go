package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vilsonfr/volcanopredict/backend/internal/db"
	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
	"github.com/vilsonfr/volcanopredict/backend/internal/httpapi"
	"github.com/vilsonfr/volcanopredict/backend/internal/volcano"
)

type envelope struct {
	Data []map[string]any `json:"data"`
	Meta struct {
		Count      int    `json:"count"`
		NextCursor string `json:"next_cursor"`
		Disclaimer string `json:"disclaimer"`
		RequestID  string `json:"request_id"`
	} `json:"meta"`
	Attribution []map[string]any `json:"attribution"`
}

type errBody struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		Param     string `json:"param"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

// newServer boots a real Postgres, migrates it, seeds a small catalog and
// returns an HTTP handler wired exactly as production wires it.
func newServer(t *testing.T) (http.Handler, *pgxpool.Pool) {
	t.Helper()
	inst := dbtest.Start(t)

	pool, err := pgxpool.New(context.Background(), inst.ConnString)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	ctx := context.Background()
	var sourceID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO data_sources(name, base_url, category, license, attribution, enabled)
		VALUES ('Test Source', 'https://example.test/', 'test', 'CC0', 'Test Attribution', true)
		RETURNING id`).Scan(&sourceID)
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}

	seed := []struct {
		ref, name, country, status string
		lat, lon                   float64
	}{
		{"1", "Krakatau", "Indonesia", "Eruption Observed", -6.102, 105.423},
		{"2", "Salak", "Indonesia", "Eruption Dated", -6.72, 106.73},
		{"3", "Etna", "Italy", "Eruption Observed", 37.748, 14.999},
		{"4", "Fuji", "Japan", "Eruption Dated", 35.361, 138.728},
		{"5", "Hekla", "Iceland", "Evidence Credible", 63.98, -19.7},
	}
	for _, s := range seed {
		_, _, err := volcano.Upsert(ctx, pool, volcano.Record{
			SourceID: sourceID, SourceRef: s.ref, Name: s.name,
			Country: s.country, Latitude: s.lat, Longitude: s.lon, Status: s.status,
		})
		if err != nil {
			t.Fatalf("seed volcano %s: %v", s.name, err)
		}
	}

	mux := http.NewServeMux()
	(&httpapi.Server{DB: pool, SchemaVersion: db.ExpectedSchemaVersion}).RegisterRoutes(mux)
	return httpapi.WithObservability(mux), pool
}

func do(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func decodeOK(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var e envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	return e
}

func decodeErr(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) errBody {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	var e errBody
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	return e
}

func TestHealth_ReportsSchemaVersion(t *testing.T) {
	h, _ := newServer(t)
	rec := do(t, h, "/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if body["schema_version"] == nil {
		t.Error("health must report the schema version it is serving against")
	}
}

// A binary expecting a newer schema than the database has must not report
// healthy: it would answer queries against columns that may not exist.
func TestHealth_RefusesWhenSchemaIsBehind(t *testing.T) {
	inst := dbtest.Start(t)
	pool, err := pgxpool.New(context.Background(), inst.ConnString)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	mux := http.NewServeMux()
	(&httpapi.Server{DB: pool, SchemaVersion: db.ExpectedSchemaVersion + 100}).RegisterRoutes(mux)

	rec := do(t, httpapi.WithObservability(mux), "/health")
	if rec.Code == http.StatusOK {
		t.Fatal("health reported 200 against an outdated schema")
	}
}

func TestListVolcanoes_ReturnsCatalogWithAttribution(t *testing.T) {
	h, _ := newServer(t)
	e := decodeOK(t, do(t, h, "/api/v1/volcanoes"))

	if e.Meta.Count != 5 {
		t.Errorf("count = %d, want 5", e.Meta.Count)
	}
	if len(e.Attribution) == 0 {
		t.Error("catalog responses must carry the source attribution its terms require")
	}
	if e.Meta.RequestID == "" {
		t.Error("responses must carry a request id for log correlation")
	}
	// The catalog is observed data, not derived, so no disclaimer applies.
	if e.Meta.Disclaimer != "" {
		t.Errorf("unexpected disclaimer on observed data: %q", e.Meta.Disclaimer)
	}
	if e.Data[0]["source_ref"] == nil {
		t.Error("every row must be traceable to its source reference")
	}
}

func TestListVolcanoes_EmptyResultIs200NotFound(t *testing.T) {
	h, _ := newServer(t)
	e := decodeOK(t, do(t, h, "/api/v1/volcanoes?country=Atlantis"))
	if e.Meta.Count != 0 {
		t.Fatalf("count = %d, want 0", e.Meta.Count)
	}
	if e.Data == nil {
		t.Error("empty collection must serialise as [], not null")
	}
}

func TestListVolcanoes_SearchMatchesNameAndCountry(t *testing.T) {
	h, _ := newServer(t)

	byName := decodeOK(t, do(t, h, "/api/v1/volcanoes?q=krak"))
	if byName.Meta.Count != 1 || byName.Data[0]["name"] != "Krakatau" {
		t.Errorf("search by partial, lowercase name failed: %+v", byName.Data)
	}

	byCountry := decodeOK(t, do(t, h, "/api/v1/volcanoes?q=indonesia"))
	if byCountry.Meta.Count != 2 {
		t.Errorf("search by country = %d results, want 2", byCountry.Meta.Count)
	}
}

// A '%' typed by a user is a literal, not "match everything". Without escaping,
// searching for it would silently return the whole catalog.
func TestListVolcanoes_SearchTreatsWildcardAsLiteral(t *testing.T) {
	h, _ := newServer(t)
	e := decodeOK(t, do(t, h, "/api/v1/volcanoes?q=%25"))
	if e.Meta.Count != 0 {
		t.Errorf("searching for a literal '%%' returned %d rows; the LIKE wildcard leaked", e.Meta.Count)
	}
}

func TestListVolcanoes_FilterByStatus(t *testing.T) {
	h, _ := newServer(t)
	e := decodeOK(t, do(t, h, "/api/v1/volcanoes?status=Eruption+Observed"))
	if e.Meta.Count != 2 {
		t.Fatalf("count = %d, want 2", e.Meta.Count)
	}
	for _, v := range e.Data {
		if v["status"] != "Eruption Observed" {
			t.Errorf("status filter leaked %v", v["status"])
		}
	}
}

func TestListVolcanoes_NearOrdersByDistance(t *testing.T) {
	h, _ := newServer(t)
	// Jakarta: Krakatau and Salak are close, the rest are continents away.
	e := decodeOK(t, do(t, h, "/api/v1/volcanoes?lat=-6.21&lon=106.85&radius_km=500"))
	if e.Meta.Count != 2 {
		t.Fatalf("count = %d, want 2 (only the Indonesian pair); got %+v", e.Meta.Count, e.Data)
	}
	var prev float64
	for i, v := range e.Data {
		d, ok := v["distance_m"].(float64)
		if !ok {
			t.Fatalf("row %d has no distance_m: %+v", i, v)
		}
		if i > 0 && d < prev {
			t.Errorf("results are not ordered by increasing distance: %v after %v", d, prev)
		}
		prev = d
	}
}

func TestListVolcanoes_RejectsInvalidParams(t *testing.T) {
	h, _ := newServer(t)

	cases := []struct {
		name, target, wantParam string
	}{
		{"latitude out of range", "/api/v1/volcanoes?lat=91&lon=10&radius_km=50", "lat"},
		{"longitude out of range", "/api/v1/volcanoes?lat=10&lon=181&radius_km=50", "lon"},
		{"limit above maximum", "/api/v1/volcanoes?limit=99999", "limit"},
		{"limit not a number", "/api/v1/volcanoes?limit=abc", "limit"},
		{"partial proximity query", "/api/v1/volcanoes?lat=10", "radius_km"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := decodeErr(t, do(t, h, tc.target), http.StatusBadRequest)
			if e.Error.Code != httpapi.CodeInvalidParameter {
				t.Errorf("code = %q, want %q", e.Error.Code, httpapi.CodeInvalidParameter)
			}
			if e.Error.Param != tc.wantParam {
				t.Errorf("param = %q, want %q — the error must name what was rejected", e.Error.Param, tc.wantParam)
			}
		})
	}
}

// Walking every page must visit each row exactly once. Off-by-one keyset
// cursors are easy to write and produce duplicates or gaps that only show up
// on large collections.
func TestListVolcanoes_PaginationVisitsEachRowOnce(t *testing.T) {
	h, _ := newServer(t)

	seen := map[string]int{}
	cursor := ""
	for page := 0; page < 20; page++ {
		target := "/api/v1/volcanoes?limit=2"
		if cursor != "" {
			target += "&cursor=" + cursor
		}
		e := decodeOK(t, do(t, h, target))
		for _, v := range e.Data {
			seen[v["source_ref"].(string)]++
		}
		if e.Meta.NextCursor == "" {
			break
		}
		cursor = e.Meta.NextCursor
	}

	if len(seen) != 5 {
		t.Errorf("paging saw %d distinct volcanoes, want 5 — rows were skipped", len(seen))
	}
	for ref, n := range seen {
		if n != 1 {
			t.Errorf("volcano %s appeared %d times across pages, want once", ref, n)
		}
	}
}

func TestObservations_RejectsInvertedWindowAndFutureAsOf(t *testing.T) {
	h, _ := newServer(t)

	inverted := decodeErr(t,
		do(t, h, "/api/v1/observations?from=2026-09-05T00:00:00Z&to=2026-09-01T00:00:00Z"),
		http.StatusBadRequest)
	if inverted.Error.Param != "from" {
		t.Errorf("param = %q, want from", inverted.Error.Param)
	}

	future := decodeErr(t,
		do(t, h, "/api/v1/observations?as_of=2999-01-01T00:00:00Z"),
		http.StatusBadRequest)
	if future.Error.Param != "as_of" {
		t.Errorf("param = %q, want as_of", future.Error.Param)
	}
}

// With no filter the API must not mix synthetic records into what a caller
// reads as observations.
func TestObservations_DefaultsToRealDataOnly(t *testing.T) {
	h, pool := newServer(t)

	ctx := context.Background()
	var volcanoID, sourceID int64
	if err := pool.QueryRow(ctx, `SELECT id, source_id FROM volcanoes LIMIT 1`).
		Scan(&volcanoID, &sourceID); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO observations(volcano_id, observed_at, kind, value, source_id, is_synthetic)
		VALUES ($1, now(), 'test', '{}'::jsonb, $2, true)`, volcanoID, sourceID)
	if err != nil {
		t.Fatal(err)
	}

	e := decodeOK(t, do(t, h, "/api/v1/observations"))
	if e.Meta.Count != 0 {
		t.Errorf("default listing returned %d rows; synthetic data leaked into an unfiltered read", e.Meta.Count)
	}

	all := decodeOK(t, do(t, h, "/api/v1/observations?provenance=synthetic"))
	if all.Meta.Count != 1 {
		t.Fatalf("explicit synthetic listing returned %d rows, want 1", all.Meta.Count)
	}
	if all.Data[0]["is_synthetic"] != true {
		t.Error("synthetic rows must be marked as such on the wire")
	}
}

func TestUnversionedAPIPathIs404(t *testing.T) {
	h, _ := newServer(t)
	e := decodeErr(t, do(t, h, "/api/volcanoes"), http.StatusNotFound)
	if e.Error.Code != httpapi.CodeNotFound {
		t.Errorf("code = %q, want %q", e.Error.Code, httpapi.CodeNotFound)
	}
}

// Error bodies must never leak SQL, file paths or stack traces to an
// unauthenticated caller.
func TestErrorsDoNotLeakInternals(t *testing.T) {
	h, _ := newServer(t)
	body := do(t, h, "/api/v1/volcanoes?limit=abc").Body.String()
	for _, needle := range []string{"SELECT", "pgx", ".go:", "/app/", "goroutine"} {
		if strings.Contains(body, needle) {
			t.Errorf("error body leaked internal detail %q: %s", needle, body)
		}
	}
}

func TestSources_ExposeLicenceAndAttribution(t *testing.T) {
	h, _ := newServer(t)
	e := decodeOK(t, do(t, h, "/api/v1/sources"))
	if e.Meta.Count == 0 {
		t.Fatal("source registry is empty")
	}
	var found bool
	for _, s := range e.Data {
		if s["Name"] == "Test Source" || s["name"] == "Test Source" {
			found = true
		}
	}
	if !found {
		t.Errorf("registered source missing from /api/v1/sources: %+v", e.Data)
	}
}
