package usgs_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vilsonfr/volcanopredict/backend/internal/dataquality"
	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
	"github.com/vilsonfr/volcanopredict/backend/internal/earthquake"
	"github.com/vilsonfr/volcanopredict/backend/internal/ingestion"
	"github.com/vilsonfr/volcanopredict/backend/internal/usgs"
)

func mustPool(t *testing.T, connString string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func usgsSourceID(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM data_sources WHERE name = $1`, usgs.SourceName).Scan(&id); err != nil {
		t.Fatalf("resolving the USGS source: %v", err)
	}
	return id
}

// serveBodies stands in for the source, handing out one body per request.
// The last body is repeated for any further request, which is what an
// unchanging source looks like.
func serveBodies(t *testing.T, bodies ...string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1)) - 1
		if n >= len(bodies) {
			n = len(bodies) - 1
		}
		fmt.Fprint(w, bodies[n])
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func ingesterFor(t *testing.T, srv *httptest.Server, pool *pgxpool.Pool, srcID int64) *usgs.Ingester {
	t.Helper()
	c := usgs.NewClient()
	c.BaseURL = srv.URL
	c.RetryBackoff = time.Millisecond
	ing := usgs.NewIngester(c, pool, srcID)
	return ing
}

// collection is one FDSN-shaped document built around the given features.
func collection(features ...string) string {
	return fmt.Sprintf(
		`{"type":"FeatureCollection","metadata":{"api":"2.7.0","count":%d},"features":[%s]}`,
		len(features), strings.Join(features, ","))
}

func feature(id string, timeMs int64, mag string, magType, status string, depth float64) string {
	return fmt.Sprintf(
		`{"type":"Feature","id":%q,"properties":{"mag":%s,"magType":%q,"time":%d,"updated":%d,"status":%q},"geometry":{"type":"Point","coordinates":[105.4,-6.1,%v]}}`,
		id, mag, magType, timeMs, timeMs+1000, status, depth)
}

func recentMs(d time.Duration) int64 {
	return time.Now().UTC().Add(-d).UnixMilli()
}

// --- 8.1 / 6.2: a second identical run changes nothing -------------------

func TestIngest_SecondRunOverUnchangedSourceReportsNoChanges(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := usgsSourceID(t, pool)
	ctx := context.Background()

	body := collection(
		feature("us-a", recentMs(2*time.Hour), "5.1", "mb", "automatic", 35),
		feature("us-b", recentMs(3*time.Hour), "4.2", "ml", "reviewed", 12),
	)
	srv, _ := serveBodies(t, body)
	ing := ingesterFor(t, srv, pool, srcID)

	now := time.Now().UTC()
	first, err := ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-24*time.Hour), now)
	if err != nil {
		t.Fatalf("first ingestion: %v", err)
	}
	if first.Counts.Inserted != 2 {
		t.Fatalf("expected 2 events inserted, got %+v", first.Counts)
	}

	second, err := ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-24*time.Hour), now)
	if err != nil {
		t.Fatalf("second ingestion: %v", err)
	}
	if second.Counts.Inserted != 0 || second.Counts.Updated != 0 {
		t.Fatalf("a second run over an unchanged source must report zero inserted and zero updated, got %+v", second.Counts)
	}
	if second.Counts.Unchanged != 2 {
		t.Fatalf("expected both events reported unchanged, got %+v", second.Counts)
	}
}

// --- 6.3: revision creates a version; as-of still sees the old one ------

// This is the test the V0.1 could not write: it needs a source that
// revises. The magnitudes and the automatic→reviewed promotion below are
// the shape USGS actually publishes (see docs/DATA_SOURCES.md).
func TestIngest_RevisionCreatesAVersionAndAsOfStillSeesTheOldOne(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := usgsSourceID(t, pool)
	ctx := context.Background()

	occurred := recentMs(4 * time.Hour)
	before := collection(feature("us7000tdmm", occurred, "5.2", "mb", "automatic", 35))
	after := collection(feature("us7000tdmm", occurred, "5.0", "ml", "reviewed", 17.0))

	srv, _ := serveBodies(t, before, after)
	ing := ingesterFor(t, srv, pool, srcID)
	now := time.Now().UTC()

	if _, err := ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-24*time.Hour), now); err != nil {
		t.Fatalf("first ingestion: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	beforeRevision := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)

	rep, err := ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-24*time.Hour), now)
	if err != nil {
		t.Fatalf("second ingestion: %v", err)
	}
	if rep.Counts.Updated != 1 {
		t.Fatalf("a revised magnitude must be reported as an update, got %+v", rep.Counts)
	}

	// Both versions coexist.
	var versions int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM earthquakes WHERE external_id = 'us7000tdmm'`).Scan(&versions); err != nil {
		t.Fatalf("count: %v", err)
	}
	if versions != 2 {
		t.Fatalf("expected the pre- and post-revision versions to coexist, got %d rows", versions)
	}

	// The current read returns only the revised one.
	current, err := earthquake.List(ctx, pool, earthquake.Query{
		SourceID: &srcID, Provenance: earthquake.ProvenanceAll,
	})
	if err != nil {
		t.Fatalf("current List: %v", err)
	}
	if len(current) != 1 {
		t.Fatalf("the current read must collapse to one row, got %d", len(current))
	}
	if current[0].Magnitude == nil || *current[0].Magnitude != 5.0 {
		t.Fatalf("current magnitude: got %v, want 5.0", current[0].Magnitude)
	}
	if current[0].SourceStatus != "reviewed" {
		t.Fatalf("current status: got %q, want reviewed", current[0].SourceStatus)
	}

	// And an as-of query from before the revision still returns what was
	// known then — the whole point of the bitemporal store.
	past, err := earthquake.List(ctx, pool, earthquake.Query{
		SourceID: &srcID, AsOf: &beforeRevision, Provenance: earthquake.ProvenanceAll,
	})
	if err != nil {
		t.Fatalf("as-of List: %v", err)
	}
	if len(past) != 1 {
		t.Fatalf("expected one row as of before the revision, got %d", len(past))
	}
	if past[0].Magnitude == nil || *past[0].Magnitude != 5.2 {
		t.Fatalf("as-of magnitude: got %v, want the pre-revision 5.2", past[0].Magnitude)
	}
	if past[0].SourceStatus != "automatic" {
		t.Fatalf("as-of status: got %q, want the pre-revision automatic", past[0].SourceStatus)
	}
}

// --- 5.3/5.4: quality marks and preserves; revision re-evaluates --------

func TestIngest_ImplausibleValueIsMarkedAndKept(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := usgsSourceID(t, pool)
	ctx := context.Background()

	body := collection(
		feature("us-good", recentMs(time.Hour), "5.1", "mb", "reviewed", 35),
		feature("us-impossible", recentMs(time.Hour), "42.0", "mb", "automatic", 35),
	)
	srv, _ := serveBodies(t, body)
	ing := ingesterFor(t, srv, pool, srcID)
	now := time.Now().UTC()

	rep, err := ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-24*time.Hour), now)
	if err != nil {
		t.Fatalf("ingestion: %v", err)
	}
	if rep.Counts.Inserted != 2 {
		t.Fatalf("the implausible record must be KEPT, marked — not dropped: %+v", rep.Counts)
	}

	var state, reason string
	if err := pool.QueryRow(ctx, `
		SELECT quality_state, coalesce(quality_reason, '') FROM earthquakes WHERE external_id = 'us-impossible'
	`).Scan(&state, &reason); err != nil {
		t.Fatalf("the implausible record must still be retrievable: %v", err)
	}
	if state == "valid" {
		t.Fatal("a magnitude of 42 must not be stored as valid")
	}
	if !strings.Contains(reason, dataquality.RuleImpossibleValue) {
		t.Fatalf("the stored reason must name the rule that fired, got %q", reason)
	}

	// And the good one is untouched.
	if err := pool.QueryRow(ctx, `
		SELECT quality_state FROM earthquakes WHERE external_id = 'us-good'
	`).Scan(&state); err != nil {
		t.Fatalf("good record: %v", err)
	}
	if state != "valid" {
		t.Fatalf("the plausible record must be valid, got %q", state)
	}
}

// Re-evaluating must not rewrite history: the earlier version keeps the
// verdict it was given.
func TestIngest_ReEvaluationDoesNotAlterTheEarlierVersion(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := usgsSourceID(t, pool)
	ctx := context.Background()

	occurred := recentMs(2 * time.Hour)
	bad := collection(feature("us-fixed", occurred, "42.0", "mb", "automatic", 35))
	good := collection(feature("us-fixed", occurred, "5.2", "mb", "reviewed", 35))

	srv, _ := serveBodies(t, bad, good)
	ing := ingesterFor(t, srv, pool, srcID)
	now := time.Now().UTC()

	if _, err := ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-24*time.Hour), now); err != nil {
		t.Fatalf("first ingestion: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	beforeFix := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)

	if _, err := ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-24*time.Hour), now); err != nil {
		t.Fatalf("second ingestion: %v", err)
	}

	current, err := earthquake.List(ctx, pool, earthquake.Query{SourceID: &srcID, Provenance: earthquake.ProvenanceAll})
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if len(current) != 1 || current[0].QualityState != dataquality.StateValid {
		t.Fatalf("the corrected version must be valid, got %+v", current)
	}

	past, err := earthquake.List(ctx, pool, earthquake.Query{
		SourceID: &srcID, AsOf: &beforeFix, Provenance: earthquake.ProvenanceAll,
	})
	if err != nil {
		t.Fatalf("as-of: %v", err)
	}
	if len(past) != 1 {
		t.Fatalf("expected one row before the fix, got %d", len(past))
	}
	if past[0].QualityState == dataquality.StateValid {
		t.Fatal("the earlier version was never valid, and re-evaluating must not retroactively say it was")
	}
}

// --- 4.3: a broken record does not take the batch down ------------------

func TestIngest_BadRecordsRejectedWithoutAbortingTheBatch(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := usgsSourceID(t, pool)
	ctx := context.Background()

	body := collection(
		feature("us-ok-1", recentMs(time.Hour), "5.1", "mb", "reviewed", 35),
		// No time: unplaceable on a timeline.
		`{"type":"Feature","id":"us-notime","properties":{"mag":5.0,"status":"automatic"},"geometry":{"type":"Point","coordinates":[1,2,3]}}`,
		// Latitude 91 does not exist.
		`{"type":"Feature","id":"us-badlat","properties":{"mag":5.0,"time":1788000000000,"status":"automatic"},"geometry":{"type":"Point","coordinates":[10,91,5]}}`,
		feature("us-ok-2", recentMs(2*time.Hour), "4.4", "ml", "reviewed", 10),
	)
	srv, _ := serveBodies(t, body)
	ing := ingesterFor(t, srv, pool, srcID)
	now := time.Now().UTC()

	rep, err := ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-90*24*time.Hour), now)
	if err != nil {
		t.Fatalf("a batch with bad records must not fail as a whole: %v", err)
	}
	if rep.Counts.Inserted != 2 {
		t.Fatalf("expected the 2 good records to be stored, got %+v", rep.Counts)
	}
	if rep.Counts.Rejected != 2 {
		t.Fatalf("expected the 2 bad records to be counted as rejected, got %+v", rep.Counts)
	}
	if rep.Run.Result != ingestion.ResultSuccess {
		t.Fatalf("the run itself succeeded, got %s", rep.Run.Result)
	}
}

// --- 7.1: the run is recorded even when the source is down --------------

func TestIngest_SourceDownRecordsAFailedRun(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := usgsSourceID(t, pool)
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ing := ingesterFor(t, srv, pool, srcID)
	now := time.Now().UTC()

	_, err := ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-time.Hour), now)
	if err == nil {
		t.Fatal("expected the ingestion to fail while the source is down")
	}

	last, err := ingestion.LastRun(ctx, pool, srcID)
	if err != nil {
		t.Fatalf("a failed ingestion must still leave a run: %v", err)
	}
	if last.Result != ingestion.ResultFailure {
		t.Fatalf("expected a recorded failure, got %s", last.Result)
	}
	if last.ErrorMsg == "" {
		t.Fatal("the recorded failure must carry its cause")
	}

	// And the window must NOT count as collected.
	cov, err := ingestion.CoverageFor(ctx, pool, srcID, now.Add(-time.Hour), now)
	if err != nil {
		t.Fatalf("CoverageFor: %v", err)
	}
	if cov.Complete() {
		t.Fatal("a window whose collection failed must never report complete coverage: that is how an outage becomes a fake quiet period")
	}
}

// --- 8.4: the incremental anchor does not lose a revision ---------------

// The event is revised in the gap between two cycles. With an exact anchor
// it would fall through; the safety overlap is what catches it.
func TestIngestIncremental_DoesNotLoseAnEventRevisedBetweenCycles(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := usgsSourceID(t, pool)
	ctx := context.Background()

	occurred := recentMs(6 * time.Hour)
	var revised atomic.Bool
	var lastUpdatedAfter atomic.Value
	lastUpdatedAfter.Store("")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastUpdatedAfter.Store(r.URL.Query().Get("updatedafter"))
		if revised.Load() {
			fmt.Fprint(w, collection(feature("us-gap", occurred, "5.9", "mww", "reviewed", 20)))
			return
		}
		fmt.Fprint(w, collection(feature("us-gap", occurred, "5.4", "mb", "automatic", 20)))
	}))
	defer srv.Close()

	ing := ingesterFor(t, srv, pool, srcID)

	if _, err := ing.IngestIncremental(ctx, time.Hour); err != nil {
		t.Fatalf("first cycle: %v", err)
	}
	// The source revises the event immediately after the first cycle
	// closed — the window an exact anchor would skip over.
	revised.Store(true)

	rep, err := ing.IngestIncremental(ctx, time.Hour)
	if err != nil {
		t.Fatalf("second cycle: %v", err)
	}
	if rep.Counts.Updated != 1 {
		t.Fatalf("the revision must be picked up by the next cycle, got %+v", rep.Counts)
	}

	// The second cycle must have asked with updatedafter, reaching back
	// before the end of the previous run.
	if got := lastUpdatedAfter.Load().(string); got == "" {
		t.Fatal("an incremental cycle must query by updatedafter: it is the only filter that surfaces revisions")
	}

	current, err := earthquake.List(ctx, pool, earthquake.Query{SourceID: &srcID, Provenance: earthquake.ProvenanceAll})
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if len(current) != 1 || current[0].Magnitude == nil || *current[0].Magnitude != 5.9 {
		t.Fatalf("expected the revised magnitude to be current, got %+v", current)
	}
}

func TestIngestIncremental_FirstCycleUsesTheFallbackWindow(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := usgsSourceID(t, pool)
	ctx := context.Background()

	srv, calls := serveBodies(t, collection())
	ing := ingesterFor(t, srv, pool, srcID)

	rep, err := ing.IngestIncremental(ctx, 2*time.Hour)
	if err != nil {
		t.Fatalf("first cycle on an empty database must work: %v", err)
	}
	if calls.Load() == 0 {
		t.Fatal("the first cycle must actually query the source")
	}
	span := rep.Run.WindowEnd.Sub(rep.Run.WindowStart)
	if span < 90*time.Minute || span > 3*time.Hour {
		t.Fatalf("the first cycle must cover roughly the fallback window, got %s", span)
	}
	if rep.Run.Result != ingestion.ResultSuccess {
		t.Fatalf("expected success, got %s", rep.Run.Result)
	}
}

// --- 8.5: interrupting and re-running leaves a consistent database ------

func TestIngest_IsResumableAfterInterruption(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := usgsSourceID(t, pool)
	ctx := context.Background()
	now := time.Now().UTC()

	full := collection(
		feature("us-r1", recentMs(time.Hour), "5.1", "mb", "reviewed", 35),
		feature("us-r2", recentMs(2*time.Hour), "4.8", "mb", "reviewed", 20),
		feature("us-r3", recentMs(3*time.Hour), "4.1", "ml", "reviewed", 10),
	)

	// First attempt dies partway: the source goes down on the request.
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	broken := ingesterFor(t, down, pool, srcID)
	if _, err := broken.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-24*time.Hour), now); err == nil {
		t.Fatal("expected the interrupted attempt to fail")
	}
	down.Close()

	// Re-run against a healthy source.
	srv, _ := serveBodies(t, full)
	ing := ingesterFor(t, srv, pool, srcID)
	rep, err := ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-24*time.Hour), now)
	if err != nil {
		t.Fatalf("re-running after an interruption must work: %v", err)
	}
	if rep.Counts.Inserted != 3 {
		t.Fatalf("the re-run must pick up everything, got %+v", rep.Counts)
	}

	// And a third run finds nothing new: no duplicate versions were left
	// behind by the interruption.
	again, err := ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-24*time.Hour), now)
	if err != nil {
		t.Fatalf("third run: %v", err)
	}
	if again.Counts.Inserted != 0 || again.Counts.Updated != 0 {
		t.Fatalf("the database must be consistent after the interruption, got %+v", again.Counts)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM earthquakes WHERE source_id = $1`, srcID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 3 {
		t.Fatalf("expected exactly 3 rows with no duplicates, got %d", rows)
	}
}

// --- raw payload survives the whole chain -------------------------------

func TestIngest_StoresRawPayloadAndParserVersion(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := usgsSourceID(t, pool)
	ctx := context.Background()

	srv, _ := serveBodies(t, collection(feature("us-raw", recentMs(time.Hour), "5.1", "mb", "reviewed", 35)))
	ing := ingesterFor(t, srv, pool, srcID)
	now := time.Now().UTC()

	if _, err := ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-24*time.Hour), now); err != nil {
		t.Fatalf("ingestion: %v", err)
	}

	var raw []byte
	var parserVersion, sourceVersion string
	if err := pool.QueryRow(ctx, `
		SELECT raw, parser_version, coalesce(source_version, '')
		FROM earthquakes WHERE external_id = 'us-raw'
	`).Scan(&raw, &parserVersion, &sourceVersion); err != nil {
		t.Fatalf("query: %v", err)
	}

	if parserVersion != usgs.ParserVersion {
		t.Fatalf("parser version: got %q, want %q", parserVersion, usgs.ParserVersion)
	}
	if sourceVersion != "2.7.0" {
		t.Fatalf("the source's own service version must be recorded, got %q", sourceVersion)
	}

	var decoded struct {
		ID         string `json:"id"`
		Properties struct {
			Mag float64 `json:"mag"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("stored raw payload is not decodable: %v", err)
	}
	if decoded.ID != "us-raw" || decoded.Properties.Mag != 5.1 {
		t.Fatalf("the stored raw payload must let the normalized fields be rederived, got %+v", decoded)
	}
}

// --- the defect the first real backfill exposed -------------------------

// Running the real thing marked all 642 events of a 2-day historical
// backfill as `delayed`, because "arrived more than a day after it
// happened" is trivially true for every historical record. The field
// stopped meaning anything and every record vanished from a valid-only
// query.
//
// Lateness describes the collection, not the event: it is only meaningful
// for an incremental cycle, where the pipeline was supposed to have seen
// the record already.
func TestIngest_BackfillDoesNotMarkHistoricalDataAsDelayed(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := usgsSourceID(t, pool)
	ctx := context.Background()

	// Five days old: far beyond any collection cadence.
	old := recentMs(5 * 24 * time.Hour)
	srv, _ := serveBodies(t, collection(feature("us-old", old, "5.1", "mb", "reviewed", 35)))
	ing := ingesterFor(t, srv, pool, srcID)
	now := time.Now().UTC()

	if _, err := ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-7*24*time.Hour), now); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var state, reason string
	if err := pool.QueryRow(ctx, `
		SELECT quality_state, coalesce(quality_reason, '') FROM earthquakes WHERE external_id = 'us-old'
	`).Scan(&state, &reason); err != nil {
		t.Fatalf("query: %v", err)
	}
	if state != "valid" {
		t.Fatalf("a backfilled historical event is not late — the operator asked for old data on purpose. Got %q (%s)", state, reason)
	}
}

// The other half: in an incremental cycle, an event that surfaces long
// after it happened IS a late arrival, and must still be marked.
func TestIngestIncremental_StillMarksGenuineLateArrivals(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := usgsSourceID(t, pool)
	ctx := context.Background()

	old := recentMs(5 * 24 * time.Hour)
	srv, _ := serveBodies(t, collection(feature("us-late", old, "5.1", "mb", "reviewed", 35)))
	ing := ingesterFor(t, srv, pool, srcID)

	if _, err := ing.IngestIncremental(ctx, time.Hour); err != nil {
		t.Fatalf("incremental: %v", err)
	}

	var state, reason string
	if err := pool.QueryRow(ctx, `
		SELECT quality_state, coalesce(quality_reason, '') FROM earthquakes WHERE external_id = 'us-late'
	`).Scan(&state, &reason); err != nil {
		t.Fatalf("query: %v", err)
	}
	if state != "delayed" {
		t.Fatalf("an event surfacing five days late in a routine cycle is a late arrival, got %q", state)
	}
	if !strings.Contains(reason, dataquality.RuleLateArrival) {
		t.Fatalf("the reason must name the late-arrival rule, got %q", reason)
	}
}
