package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vilsonfr/volcanopredict/backend/internal/dataquality"
	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
	"github.com/vilsonfr/volcanopredict/backend/internal/earthquake"
	"github.com/vilsonfr/volcanopredict/backend/internal/httpapi"
	"github.com/vilsonfr/volcanopredict/backend/internal/ingestion"
	"github.com/vilsonfr/volcanopredict/backend/internal/usgs"
)

func eqServer(t *testing.T) (*pgxpool.Pool, http.Handler, int64) {
	t.Helper()
	tdb := dbtest.Start(t)
	pool, err := pgxpool.New(context.Background(), tdb.ConnString)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	var srcID int64
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM data_sources WHERE name = $1`, usgs.SourceName).Scan(&srcID); err != nil {
		t.Fatalf("resolving the USGS source: %v", err)
	}

	mux := http.NewServeMux()
	(&httpapi.Server{DB: pool, SchemaVersion: 0}).RegisterRoutes(mux)
	return pool, httpapi.WithObservability(mux), srcID
}

func getJSON(t *testing.T, h http.Handler, url string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %s: %v (body: %s)", url, err, rec.Body.String())
	}
	return rec.Code, body
}

func seedEq(t *testing.T, pool *pgxpool.Pool, srcID int64, id string, occurred time.Time, lat, lon float64, mag *float64, q dataquality.Verdict) {
	t.Helper()
	n := earthquake.ForTest(id, srcID, occurred, lat, lon)
	n.Magnitude = mag
	n.Quality = q
	if _, _, err := earthquake.Save(context.Background(), pool, n); err != nil {
		t.Fatalf("seeding %s: %v", id, err)
	}
}

func mag(v float64) *float64 { return &v }

func valid() dataquality.Verdict {
	return dataquality.Verdict{State: dataquality.StateValid}
}

func suspect() dataquality.Verdict {
	return dataquality.Verdict{State: dataquality.StateSuspect, Rules: []string{dataquality.RuleImpossibleValue}}
}

// --- 9.1: the collection under the existing contract --------------------

func TestEarthquakes_PagingWalksTheWholeCollectionExactlyOnce(t *testing.T) {
	pool, h, srcID := eqServer(t)
	base := time.Now().UTC().Add(-48 * time.Hour)

	const total = 25
	for i := 0; i < total; i++ {
		seedEq(t, pool, srcID, fmt.Sprintf("ev%03d", i), base.Add(time.Duration(i)*time.Minute),
			float64(i%80)-40, float64(i%170)-85, mag(float64(i)/10), valid())
	}

	seen := map[string]int{}
	url := "/api/v1/earthquakes?limit=7"
	for page := 0; page < 20; page++ {
		code, body := getJSON(t, h, url)
		if code != http.StatusOK {
			t.Fatalf("page %d: HTTP %d: %v", page, code, body)
		}
		for _, item := range body["data"].([]any) {
			seen[item.(map[string]any)["external_id"].(string)]++
		}
		meta := body["meta"].(map[string]any)
		next, _ := meta["next_cursor"].(string)
		if next == "" {
			break
		}
		url = "/api/v1/earthquakes?limit=7&cursor=" + next
	}

	if len(seen) != total {
		t.Fatalf("walking the collection must visit all %d events, saw %d", total, len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("event %s came back %d times: paging must not repeat", id, n)
		}
	}
}

func TestEarthquakes_InvertedWindowIsRejectedNamingTheParameter(t *testing.T) {
	_, h, _ := eqServer(t)

	code, body := getJSON(t, h,
		"/api/v1/earthquakes?from=2026-09-03T00:00:00Z&to=2026-09-01T00:00:00Z")
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %v", code, body)
	}
	errObj := body["error"].(map[string]any)
	if errObj["param"] != "from" {
		t.Fatalf("the error must name the offending parameter, got %v", errObj)
	}
}

func TestEarthquakes_HalfSpecifiedProximityIsRejected(t *testing.T) {
	_, h, _ := eqServer(t)

	code, body := getJSON(t, h, "/api/v1/earthquakes?lat=-6.1&radius_km=100")
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %v", code, body)
	}
	if body["error"].(map[string]any)["param"] != "lon" {
		t.Fatalf("the error must name the missing parameter, got %v", body["error"])
	}
}

func TestEarthquakes_FutureAsOfIsRejected(t *testing.T) {
	_, h, _ := eqServer(t)

	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	code, _ := getJSON(t, h, "/api/v1/earthquakes?as_of="+future)
	if code != http.StatusBadRequest {
		t.Fatalf("an as-of in the future must be rejected, got %d", code)
	}
}

func TestEarthquakes_ProximityReturnsDistanceAndOrdersByIt(t *testing.T) {
	pool, h, srcID := eqServer(t)
	occurred := time.Now().UTC().Add(-time.Hour)

	seedEq(t, pool, srcID, "near", occurred, -6.15, 105.45, mag(4.0), valid())
	seedEq(t, pool, srcID, "mid", occurred, -6.60, 105.80, mag(4.0), valid())
	seedEq(t, pool, srcID, "far", occurred, 35.0, -117.9, mag(4.0), valid())

	code, body := getJSON(t, h, "/api/v1/earthquakes?lat=-6.102&lon=105.423&radius_km=200")
	if code != http.StatusOK {
		t.Fatalf("HTTP %d: %v", code, body)
	}
	data := body["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("expected the 2 events within 200 km, got %d", len(data))
	}
	first := data[0].(map[string]any)
	if first["external_id"] != "near" {
		t.Fatalf("results must be ordered by increasing distance, got %v first", first["external_id"])
	}
	if _, ok := first["distance_km"]; !ok {
		t.Fatal("a proximity result must carry its distance")
	}
}

// --- 9.2: quality travels with the data ---------------------------------

func TestEarthquakes_QualityStateIsAlwaysPresentAndFilterable(t *testing.T) {
	pool, h, srcID := eqServer(t)
	occurred := time.Now().UTC().Add(-time.Hour)

	seedEq(t, pool, srcID, "good", occurred, 1, 1, mag(5.0), valid())
	seedEq(t, pool, srcID, "bad", occurred, 2, 2, mag(42.0), suspect())

	code, body := getJSON(t, h, "/api/v1/earthquakes")
	if code != http.StatusOK {
		t.Fatalf("HTTP %d: %v", code, body)
	}
	for _, item := range body["data"].([]any) {
		m := item.(map[string]any)
		if _, ok := m["quality_state"]; !ok {
			t.Fatalf("every item must carry its quality state, got %v", m)
		}
		if m["external_id"] == "bad" {
			if m["quality_state"] == "valid" {
				t.Fatal("a suspect record must not be served as valid")
			}
			if m["quality_reason"] == nil || m["quality_reason"] == "" {
				t.Fatal("a non-valid record must carry the reason")
			}
		}
	}

	// The composition of a mixed set must be observable without inspecting
	// every item.
	quality := body["meta"].(map[string]any)["quality"].(map[string]any)
	if quality["valid"] != float64(1) || quality["suspect"] != float64(1) {
		t.Fatalf("meta.quality must report the composition of the page, got %v", quality)
	}

	// And filtering must actually exclude.
	code, body = getJSON(t, h, "/api/v1/earthquakes?quality=valid")
	if code != http.StatusOK {
		t.Fatalf("HTTP %d: %v", code, body)
	}
	for _, item := range body["data"].([]any) {
		if item.(map[string]any)["quality_state"] != "valid" {
			t.Fatalf("a valid-only query returned %v", item)
		}
	}
	if len(body["data"].([]any)) != 1 {
		t.Fatalf("expected exactly the valid record, got %d", len(body["data"].([]any)))
	}
}

func TestEarthquakes_InventedQualityValueIsRejected(t *testing.T) {
	_, h, _ := eqServer(t)

	code, body := getJSON(t, h, "/api/v1/earthquakes?quality=probably-fine")
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invented quality state, got %d", code)
	}
	if body["error"].(map[string]any)["param"] != "quality" {
		t.Fatalf("the error must name the parameter, got %v", body["error"])
	}
}

// --- 9.3: an empty page must say WHY it is empty -------------------------

// The §74 case, at the API boundary: two empty collections that mean
// opposite things must not look alike.
func TestEarthquakes_EmptyCollectionDistinguishesNoEventsFromNoCollection(t *testing.T) {
	pool, h, srcID := eqServer(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// A window that WAS collected and genuinely had no events.
	collectedFrom := now.Add(-6 * time.Hour)
	collectedTo := now.Add(-4 * time.Hour)
	run, err := ingestion.Begin(ctx, pool, srcID, ingestion.ModeBackfill, collectedFrom, collectedTo)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := ingestion.Finish(ctx, pool, run.ID, ingestion.Counts{}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	code, body := getJSON(t, h, fmt.Sprintf("/api/v1/earthquakes?from=%s&to=%s",
		collectedFrom.Format(time.RFC3339), collectedTo.Format(time.RFC3339)))
	if code != http.StatusOK {
		t.Fatalf("HTTP %d: %v", code, body)
	}
	if len(body["data"].([]any)) != 0 {
		t.Fatal("expected an empty collection")
	}
	coverage := body["meta"].(map[string]any)["coverage"].(map[string]any)
	if coverage["kind"] != "complete" {
		t.Fatalf("a collected window must report complete coverage, got %v", coverage)
	}

	// A window that was NEVER collected. Same empty data, opposite meaning.
	uncollectedFrom := now.Add(-30 * time.Hour)
	uncollectedTo := now.Add(-28 * time.Hour)
	code, body = getJSON(t, h, fmt.Sprintf("/api/v1/earthquakes?from=%s&to=%s",
		uncollectedFrom.Format(time.RFC3339), uncollectedTo.Format(time.RFC3339)))
	if code != http.StatusOK {
		t.Fatalf("HTTP %d: %v", code, body)
	}
	if len(body["data"].([]any)) != 0 {
		t.Fatal("expected an empty collection")
	}
	coverage = body["meta"].(map[string]any)["coverage"].(map[string]any)
	if coverage["kind"] == "complete" {
		t.Fatal("a window that was never collected must NOT report complete coverage: that is how an outage becomes a fake quiet period")
	}
	if coverage["note"] == nil || coverage["note"] == "" {
		t.Fatal("the response must spell out what the caller may not conclude")
	}
	if len(coverage["gaps"].([]any)) == 0 {
		t.Fatal("the uncollected stretch must be identified")
	}
}

// --- 9.4: ingestion status, sanitized ------------------------------------

func TestIngestionStatus_ReportsFailureWithoutLeakingInternals(t *testing.T) {
	pool, h, srcID := eqServer(t)
	ctx := context.Background()
	now := time.Now().UTC()

	run, err := ingestion.Begin(ctx, pool, srcID, ingestion.ModeIncremental, now.Add(-time.Hour), now)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// A cause carrying exactly the kind of internals that must not escape.
	leaky := fmt.Errorf("usgs: source unavailable after 3 attempts: Get \"https://user:hunter2@earthquake.usgs.gov/fdsnws/event/1/query\": dial tcp: /home/app/secret.pem")
	if _, err := ingestion.Fail(ctx, pool, run.ID, leaky, ingestion.Counts{}); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	code, body := getJSON(t, h, "/api/v1/ingestion")
	if code != http.StatusOK {
		t.Fatalf("HTTP %d: %v", code, body)
	}
	data := body["data"].([]any)
	var found bool
	for _, item := range data {
		m := item.(map[string]any)
		if m["source"] != usgs.SourceName {
			continue
		}
		found = true
		if m["ever_collected"] != true {
			t.Fatal("a source whose collection failed HAS run")
		}
		if m["healthy"] != false {
			t.Fatal("a source whose last run failed is not healthy")
		}
		reason, _ := m["failure_reason"].(string)
		if reason == "" {
			t.Fatal("the status must describe the failure")
		}
		for _, leak := range []string{"hunter2", "secret.pem", "dial tcp", "https://"} {
			if strings.Contains(reason, leak) {
				t.Fatalf("the sanitized reason leaked %q: %s", leak, reason)
			}
		}
	}
	if !found {
		t.Fatal("the enabled source must appear in the ingestion status")
	}
}

func TestIngestionStatus_NeverCollectedIsDistinctFromFailed(t *testing.T) {
	_, h, _ := eqServer(t)

	code, body := getJSON(t, h, "/api/v1/ingestion")
	if code != http.StatusOK {
		t.Fatalf("HTTP %d: %v", code, body)
	}
	for _, item := range body["data"].([]any) {
		m := item.(map[string]any)
		if m["source"] != usgs.SourceName {
			continue
		}
		if m["ever_collected"] != false {
			t.Fatal("a source with no runs must report that it was never collected")
		}
		if _, has := m["failure_reason"]; has {
			t.Fatal("a source that never ran has no failure to report")
		}
	}
}

// --- 9.5: raw payload reachable ------------------------------------------

func TestEarthquakes_RawPayloadIsReachableOnDemand(t *testing.T) {
	pool, h, srcID := eqServer(t)
	occurred := time.Now().UTC().Add(-time.Hour)

	n := earthquake.ForTest("us-raw", srcID, occurred, -6.1, 105.4)
	n.Magnitude = mag(5.1)
	n.ParserVersion = usgs.ParserVersion
	n.Raw = json.RawMessage(`{"id":"us-raw","properties":{"mag":5.1}}`)
	if _, _, err := earthquake.Save(context.Background(), pool, n); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Not included by default: it is large and most callers do not want it.
	code, body := getJSON(t, h, "/api/v1/earthquakes")
	if code != http.StatusOK {
		t.Fatalf("HTTP %d: %v", code, body)
	}
	if _, has := body["data"].([]any)[0].(map[string]any)["raw"]; has {
		t.Fatal("raw must not be included unless asked for")
	}

	code, body = getJSON(t, h, "/api/v1/earthquakes?include_raw=true")
	if code != http.StatusOK {
		t.Fatalf("HTTP %d: %v", code, body)
	}
	item := body["data"].([]any)[0].(map[string]any)
	raw, has := item["raw"].(map[string]any)
	if !has {
		t.Fatalf("include_raw must return the original payload, got %v", item)
	}
	// The normalized magnitude must be rederivable from what came back.
	props := raw["properties"].(map[string]any)
	if props["mag"] != item["magnitude"] {
		t.Fatalf("the raw payload must reconcile with the normalized value: raw %v vs normalized %v", props["mag"], item["magnitude"])
	}
	if item["parser_version"] != usgs.ParserVersion {
		t.Fatalf("the parser version must accompany the record, got %v", item["parser_version"])
	}
}

// The GVP catalog is enabled but arrives by swapping a versioned snapshot,
// not on a collection cadence. Listing it here as "never collected" would
// report a broken pipeline where there is no pipeline.
func TestIngestionStatus_OnlyListsSourcesThatAreActuallyCollected(t *testing.T) {
	_, h, _ := eqServer(t)

	code, body := getJSON(t, h, "/api/v1/ingestion")
	if code != http.StatusOK {
		t.Fatalf("HTTP %d: %v", code, body)
	}
	for _, item := range body["data"].([]any) {
		if item.(map[string]any)["source"] == "Smithsonian Global Volcanism Program" {
			t.Fatal("the GVP catalog has no collection cadence and must not appear as an uncollected ingestion source")
		}
	}
	// And the source that IS collected on a cadence must be there.
	var found bool
	for _, item := range body["data"].([]any) {
		if item.(map[string]any)["source"] == usgs.SourceName {
			found = true
		}
	}
	if !found {
		t.Fatal("the USGS source is collected on a cadence and must appear")
	}
}
