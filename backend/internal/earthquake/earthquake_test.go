package earthquake_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vilsonfr/volcanopredict/backend/internal/dataquality"
	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
	"github.com/vilsonfr/volcanopredict/backend/internal/earthquake"
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

func mustSource(t *testing.T, pool *pgxpool.Pool, name string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO data_sources (name, category, enabled, license, attribution)
		VALUES ($1, 'test', true, 'test-license', 'Test')
		RETURNING id
	`, name).Scan(&id)
	if err != nil {
		t.Fatalf("seeding source %s: %v", name, err)
	}
	return id
}

func f64(v float64) *float64 { return &v }

// tick advances the clock enough for two inserts to get distinct
// ingested_at values from the database.
func tick() { time.Sleep(5 * time.Millisecond) }

func mustSave(t *testing.T, pool *pgxpool.Pool, n earthquake.New) (earthquake.Earthquake, earthquake.Outcome) {
	t.Helper()
	e, outcome, err := earthquake.Save(context.Background(), pool, n)
	if err != nil {
		t.Fatalf("Save(%s): %v", n.ExternalID, err)
	}
	return e, outcome
}

// --- 6.1: the same temporal discipline as internal/observation ----------

func TestList_ProvenanceUnspecifiedRejected(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)

	_, err := earthquake.List(context.Background(), pool, earthquake.Query{})
	if !errors.Is(err, earthquake.ErrProvenanceUnspecified) {
		t.Fatalf("forgetting to choose a provenance must be an error, not an implicit 'both': %v", err)
	}
}

func TestList_FutureAsOfRejected(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)

	future := time.Now().UTC().Add(time.Hour)
	_, err := earthquake.List(context.Background(), pool, earthquake.Query{
		AsOf: &future, Provenance: earthquake.ProvenanceAll,
	})
	if !errors.Is(err, earthquake.ErrFutureAsOf) {
		t.Fatalf("an as-of in the future must be rejected, not answered with the present: %v", err)
	}
}

// The caller has no field to smuggle ingested_at through, and the database
// would overwrite it anyway. This pins both halves.
func TestSave_IngestedAtIsAssignedByTheDatabase(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-ingested-at")

	before := time.Now().UTC().Add(-time.Second)
	e, _ := mustSave(t, pool, earthquake.ForTest("us-eq-1", srcID, time.Now().UTC().Add(-time.Hour), 1, 2))
	after := time.Now().UTC().Add(time.Second)

	if e.IngestedAt.Before(before) || e.IngestedAt.After(after) {
		t.Fatalf("ingested_at must be the persistence instant, got %s (window %s..%s)", e.IngestedAt, before, after)
	}
}

func TestList_CurrentReturnsOneRowPerEvent(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-current")
	ctx := context.Background()
	occurred := time.Now().UTC().Add(-2 * time.Hour)

	n := earthquake.ForTest("us-dup", srcID, occurred, 10, 20)
	n.Magnitude = f64(5.0)
	mustSave(t, pool, n)
	tick()
	n.Magnitude = f64(5.3)
	mustSave(t, pool, n)
	tick()
	n.Magnitude = f64(5.5)
	mustSave(t, pool, n)

	got, err := earthquake.List(ctx, pool, earthquake.Query{SourceID: &srcID, Provenance: earthquake.ProvenanceAll})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("the current read must collapse versions to one row per event, got %d", len(got))
	}
	if got[0].Magnitude == nil || *got[0].Magnitude != 5.5 {
		t.Fatalf("the current read must return the latest version, got %v", got[0].Magnitude)
	}
}

func TestList_ProvenanceFilterExcludesSynthetic(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-provenance")
	ctx := context.Background()
	occurred := time.Now().UTC().Add(-time.Hour)

	real := earthquake.ForTest("us-real", srcID, occurred, 1, 1)
	mustSave(t, pool, real)
	synth := earthquake.ForTest("us-synth", srcID, occurred, 2, 2)
	synth.IsSynthetic = true
	mustSave(t, pool, synth)

	got, err := earthquake.List(ctx, pool, earthquake.Query{SourceID: &srcID, Provenance: earthquake.ProvenanceRealOnly})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, e := range got {
		if e.IsSynthetic {
			t.Fatalf("a real-only query returned synthetic event %s", e.ExternalID)
		}
	}
	if len(got) != 1 {
		t.Fatalf("expected the one real event, got %d", len(got))
	}
}

// --- 6.2: deduplication by content, not by the source's `updated` -------

func TestSave_ReingestingIdenticalContentChangesNothing(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-idempotent")
	ctx := context.Background()

	n := earthquake.ForTest("us-idem", srcID, time.Now().UTC().Add(-time.Hour), 5, 6)
	n.Magnitude = f64(4.2)
	n.MagnitudeType = "mb"
	n.SourceStatus = "automatic"

	if _, outcome := mustSave(t, pool, n); outcome != earthquake.OutcomeInserted {
		t.Fatalf("first save should insert, got %s", outcome)
	}
	tick()
	if _, outcome := mustSave(t, pool, n); outcome != earthquake.OutcomeUnchanged {
		t.Fatalf("re-ingesting identical content must report unchanged, got %s", outcome)
	}

	var versions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM earthquakes WHERE external_id = 'us-idem'`).Scan(&versions); err != nil {
		t.Fatalf("count: %v", err)
	}
	if versions != 1 {
		t.Fatalf("an unchanged re-ingestion must not append a version, got %d rows", versions)
	}
}

// This is the failure mode design.md D3 exists to prevent: USGS bumps
// `updated` routinely without changing anything measured. Trusting it
// would append a version per ingestion cycle forever.
func TestSave_FreshUpdatedTimestampAloneDoesNotCreateAVersion(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-updated-bump")
	ctx := context.Background()

	n := earthquake.ForTest("us-bump", srcID, time.Now().UTC().Add(-time.Hour), 7, 8)
	n.Magnitude = f64(3.3)
	first := time.Now().UTC().Add(-30 * time.Minute)
	n.SourceUpdatedAt = &first
	mustSave(t, pool, n)
	tick()

	// Same content, brand-new `updated`, and a different raw payload —
	// exactly what a re-published but unrevised event looks like.
	later := time.Now().UTC()
	n.SourceUpdatedAt = &later
	n.Raw = json.RawMessage(`{"reformatted":true}`)
	_, outcome := mustSave(t, pool, n)
	if outcome != earthquake.OutcomeUnchanged {
		t.Fatalf("a fresh `updated` with identical content is not a revision, got %s", outcome)
	}

	var versions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM earthquakes WHERE external_id = 'us-bump'`).Scan(&versions); err != nil {
		t.Fatalf("count: %v", err)
	}
	if versions != 1 {
		t.Fatalf("expected no new version, got %d rows", versions)
	}
}

func TestSave_ChangedContentAppendsAVersion(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-revision")
	ctx := context.Background()

	n := earthquake.ForTest("us-rev", srcID, time.Now().UTC().Add(-time.Hour), 9, 10)
	n.Magnitude = f64(5.2)
	n.SourceStatus = "automatic"
	mustSave(t, pool, n)
	tick()

	n.Magnitude = f64(5.0)
	n.SourceStatus = "reviewed"
	_, outcome := mustSave(t, pool, n)
	if outcome != earthquake.OutcomeUpdated {
		t.Fatalf("a changed magnitude is a revision, got %s", outcome)
	}

	var versions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM earthquakes WHERE external_id = 'us-rev'`).Scan(&versions); err != nil {
		t.Fatalf("count: %v", err)
	}
	if versions != 2 {
		t.Fatalf("both versions must coexist, got %d rows", versions)
	}
}

// Absence and zero are different facts, and the deduplicator must not
// collapse them: an event losing its magnitude IS a revision.
func TestSave_AbsentIsNotZeroForDeduplication(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-absent-zero")

	n := earthquake.ForTest("us-absent", srcID, time.Now().UTC().Add(-time.Hour), 1, 1)
	n.Magnitude = f64(0)
	mustSave(t, pool, n)
	tick()

	n.Magnitude = nil
	_, outcome := mustSave(t, pool, n)
	if outcome != earthquake.OutcomeUpdated {
		t.Fatalf("magnitude 0 becoming absent is a real change, got %s", outcome)
	}
}

// --- 6.1 (cont.): the write path refuses incomplete records -------------

func TestSave_RefusesRecordWithoutQualityVerdict(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-no-quality")

	n := earthquake.ForTest("us-noq", srcID, time.Now().UTC().Add(-time.Hour), 1, 1)
	n.Quality = dataquality.Verdict{} // unevaluated
	_, _, err := earthquake.Save(context.Background(), pool, n)
	if !errors.Is(err, dataquality.ErrUnevaluated) {
		t.Fatalf("an unevaluated record must not reach the table, got %v", err)
	}
}

func TestSave_RefusesInvalidCoordinate(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-bad-coord")

	n := earthquake.ForTest("us-bad", srcID, time.Now().UTC().Add(-time.Hour), 91, 0)
	_, _, err := earthquake.Save(context.Background(), pool, n)
	if !errors.Is(err, earthquake.ErrInvalidCoordinate) {
		t.Fatalf("latitude 91 must be rejected, got %v", err)
	}
}

func TestSave_RefusesRecordWithoutRawOrParserVersion(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-no-provenance")
	ctx := context.Background()

	n := earthquake.ForTest("us-noraw", srcID, time.Now().UTC().Add(-time.Hour), 1, 1)
	n.Raw = nil
	if _, _, err := earthquake.Save(ctx, pool, n); err == nil {
		t.Fatal("a record with no raw payload must be refused")
	}

	n = earthquake.ForTest("us-nopv", srcID, time.Now().UTC().Add(-time.Hour), 1, 1)
	n.ParserVersion = ""
	if _, _, err := earthquake.Save(ctx, pool, n); err == nil {
		t.Fatal("a record with no parser version must be refused")
	}
}

// --- 6.5: proximity and magnitude ---------------------------------------

func TestList_ProximityReturnsDistanceOrderedAscending(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-proximity")
	ctx := context.Background()
	occurred := time.Now().UTC().Add(-time.Hour)

	// Krakatau is at roughly (-6.102, 105.423).
	mustSave(t, pool, earthquake.ForTest("near", srcID, occurred, -6.15, 105.45)) // ~6 km
	mustSave(t, pool, earthquake.ForTest("mid", srcID, occurred, -6.60, 105.80))  // ~70 km
	mustSave(t, pool, earthquake.ForTest("far", srcID, occurred, 35.00, -117.90)) // California

	lat, lon, radius := -6.102, 105.423, 200.0
	got, err := earthquake.List(ctx, pool, earthquake.Query{
		SourceID:      &srcID,
		Provenance:    earthquake.ProvenanceAll,
		NearLatitude:  &lat,
		NearLongitude: &lon,
		RadiusKm:      &radius,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected the 2 events within 200 km, got %d", len(got))
	}
	if got[0].ExternalID != "near" || got[1].ExternalID != "mid" {
		t.Fatalf("results must be ordered by increasing distance, got %s then %s", got[0].ExternalID, got[1].ExternalID)
	}
	for _, e := range got {
		if e.DistanceKm == nil {
			t.Fatalf("event %s came back without its distance", e.ExternalID)
		}
	}
	if *got[0].DistanceKm > *got[1].DistanceKm {
		t.Fatalf("distances out of order: %v then %v", *got[0].DistanceKm, *got[1].DistanceKm)
	}
}

func TestList_ProximityRejectsOutOfRangeLatitude(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)

	lat, lon, radius := 91.0, 0.0, 100.0
	_, err := earthquake.List(context.Background(), pool, earthquake.Query{
		Provenance: earthquake.ProvenanceAll, NearLatitude: &lat, NearLongitude: &lon, RadiusKm: &radius,
	})
	if !errors.Is(err, earthquake.ErrInvalidCoordinate) {
		t.Fatalf("latitude 91 must be rejected, got %v", err)
	}
}

func TestList_ProximityNeedsAllThreeParameters(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)

	lat := -6.1
	_, err := earthquake.List(context.Background(), pool, earthquake.Query{
		Provenance: earthquake.ProvenanceAll, NearLatitude: &lat,
	})
	if err == nil {
		t.Fatal("a half-specified proximity query must be rejected, not answered with a guess")
	}
}

func TestList_MinMagnitudeExcludesUnknownRatherThanAssumingZero(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-minmag")
	ctx := context.Background()
	occurred := time.Now().UTC().Add(-time.Hour)

	big := earthquake.ForTest("big", srcID, occurred, 1, 1)
	big.Magnitude = f64(6.0)
	mustSave(t, pool, big)

	small := earthquake.ForTest("small", srcID, occurred, 2, 2)
	small.Magnitude = f64(1.0)
	mustSave(t, pool, small)

	unknown := earthquake.ForTest("unknown", srcID, occurred, 3, 3)
	unknown.Magnitude = nil
	mustSave(t, pool, unknown)

	min := 5.0
	got, err := earthquake.List(ctx, pool, earthquake.Query{
		SourceID: &srcID, Provenance: earthquake.ProvenanceAll, MinMagnitude: &min,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ExternalID != "big" {
		t.Fatalf("expected only the M6 event, got %+v", got)
	}
}

// --- filters must run AFTER the current version is chosen ---------------

// The failure mode: with the filter inside the DISTINCT ON, the query picks
// the latest version *among the rows that match*, so a superseded version
// answers as if it were current knowledge. An M5.2 that the source later
// revised down to M4.1 would still come back for min_magnitude=5, labelled
// current.
func TestList_SupersededVersionDoesNotAnswerAFilterTheCurrentOneFails(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-filter-after-version")
	ctx := context.Background()
	occurred := time.Now().UTC().Add(-2 * time.Hour)

	n := earthquake.ForTest("us-downgraded", srcID, occurred, 1, 1)
	n.Magnitude = f64(5.2)
	mustSave(t, pool, n)
	tick()
	// The source revises it down: it is no longer an M5+ event.
	n.Magnitude = f64(4.1)
	mustSave(t, pool, n)

	min := 5.0
	got, err := earthquake.List(ctx, pool, earthquake.Query{
		SourceID: &srcID, Provenance: earthquake.ProvenanceAll, MinMagnitude: &min,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("the current version is M4.1 and must not match min_magnitude=5; got %d rows (magnitude %v)",
			len(got), got[0].Magnitude)
	}

	// And it does come back without the filter, as the revised value.
	all, err := earthquake.List(ctx, pool, earthquake.Query{
		SourceID: &srcID, Provenance: earthquake.ProvenanceAll,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 || all[0].Magnitude == nil || *all[0].Magnitude != 4.1 {
		t.Fatalf("expected the single revised version, got %+v", all)
	}
}

func TestList_SupersededQualityDoesNotAnswerAQualityFilter(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-quality-after-version")
	ctx := context.Background()
	occurred := time.Now().UTC().Add(-2 * time.Hour)

	n := earthquake.ForTest("us-fixed", srcID, occurred, 1, 1)
	n.Magnitude = f64(42)
	n.Quality = dataquality.Verdict{State: dataquality.StateOutlier, Rules: []string{dataquality.RuleImpossibleValue}}
	mustSave(t, pool, n)
	tick()
	n.Magnitude = f64(5.2)
	n.Quality = dataquality.Verdict{State: dataquality.StateValid}
	mustSave(t, pool, n)

	got, err := earthquake.List(ctx, pool, earthquake.Query{
		SourceID:      &srcID,
		Provenance:    earthquake.ProvenanceAll,
		QualityStates: []dataquality.State{dataquality.StateOutlier},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("the current version is valid; the superseded outlier must not answer an outlier filter, got %d", len(got))
	}
}

// The same trap on the geographic filter: a relocated event must be judged
// by where it is now, not by where a superseded version put it.
func TestList_SupersededLocationDoesNotAnswerAProximityQuery(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-geo-after-version")
	ctx := context.Background()
	occurred := time.Now().UTC().Add(-2 * time.Hour)

	// First located near Krakatau, then relocated far away.
	n := earthquake.ForTest("us-relocated", srcID, occurred, -6.15, 105.45)
	mustSave(t, pool, n)
	tick()
	n.Latitude, n.Longitude = 35.0, -117.9
	mustSave(t, pool, n)

	lat, lon, radius := -6.102, 105.423, 200.0
	got, err := earthquake.List(ctx, pool, earthquake.Query{
		SourceID:      &srcID,
		Provenance:    earthquake.ProvenanceAll,
		NearLatitude:  &lat,
		NearLongitude: &lon,
		RadiusKm:      &radius,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("the event was relocated out of the radius; the superseded location must not match, got %d", len(got))
	}
}

// A distance-ordered page resumed with an id-only cursor would silently
// restart the walk from the beginning.
func TestList_ProximityRejectsAnIdOnlyCursor(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)

	lat, lon, radius := -6.1, 105.4, 100.0
	_, err := earthquake.List(context.Background(), pool, earthquake.Query{
		Provenance:    earthquake.ProvenanceAll,
		NearLatitude:  &lat,
		NearLongitude: &lon,
		RadiusKm:      &radius,
		AfterID:       42,
	})
	if err == nil {
		t.Fatal("a distance-ordered page resumed on an id-only cursor must be an error, not a silent restart")
	}
}
