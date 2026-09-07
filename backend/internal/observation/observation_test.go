package observation_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
	"github.com/vilsonfr/volcanopredict/backend/internal/observation"
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
	err := pool.QueryRow(context.Background(),
		`INSERT INTO data_sources (name, category, enabled, license, attribution)
		 VALUES ($1, 'test', false, 'CC-BY-4.0', 'Test Source') RETURNING id`, name).Scan(&id)
	if err != nil {
		t.Fatalf("failed to insert test data source: %v", err)
	}
	return id
}

func mustVolcano(t *testing.T, pool *pgxpool.Pool, name string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO volcanoes (name, latitude, longitude) VALUES ($1, -6.1, 105.4) RETURNING id`, name).Scan(&id)
	if err != nil {
		t.Fatalf("failed to insert test volcano: %v", err)
	}
	return id
}

func mustValue(v string) json.RawMessage { return json.RawMessage(v) }

// mustInsert persists an observation through the package under test and
// returns it, so every seeded row's ingested_at is a real value assigned
// by the database (schema DEFAULT + the migration 009 trigger), never a
// value the test fabricates. This matters: migration 009's trigger forces
// ingested_at = now() on every INSERT regardless of what is supplied, so
// tests must build their timelines from real wall-clock ordering between
// successive inserts rather than from planted timestamps.
func mustInsert(t *testing.T, pool *pgxpool.Pool, volID, srcID int64, kind string, observedAt time.Time, isSynthetic bool) observation.Observation {
	t.Helper()
	obs, err := observation.Insert(context.Background(), pool, observation.NewObservation{
		VolcanoID: volID, Kind: kind, ObservedAt: observedAt,
		Value: mustValue(`{}`), IsSynthetic: isSynthetic, SourceID: srcID,
	})
	if err != nil {
		t.Fatalf("mustInsert failed: %v", err)
	}
	return obs
}

// tick sleeps briefly so that a subsequent database now() reliably lands
// after any checkpoint captured just before this call. Postgres timestamps
// have microsecond resolution but wall-clock reads on the test side and
// round-trips add jitter, so a small real delay is used to keep the
// timeline in this test file unambiguous instead of depending on that
// resolution alone.
func tick() { time.Sleep(20 * time.Millisecond) }

// --- 4.1: current read and as-of read ------------------------------------

func TestList_AsOf_ExcludesLaterIngested(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "asof-excludes-later")
	volID := mustVolcano(t, pool, "AsOf Excludes Later")
	observedAt := time.Now().UTC().Add(-48 * time.Hour)

	mustInsert(t, pool, volID, srcID, "seismic_count_early", observedAt, false)
	tick()
	cutoff := time.Now().UTC()
	tick()
	// Ingested strictly after cutoff, even though the phenomenon is older
	// than the cutoff: must be invisible to an as-of query at cutoff. This
	// is the exact scenario the spec calls out ("registro ingerido depois
	// do instante consultado é invisível").
	mustInsert(t, pool, volID, srcID, "seismic_count_late", observedAt, false)

	kind := "seismic_count_late"
	results, err := observation.List(ctx, pool, observation.Query{
		VolcanoID:  &volID,
		Kind:       &kind,
		AsOf:       &cutoff,
		Provenance: observation.ProvenanceAll,
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected observation ingested after cutoff to be invisible, got %d results", len(results))
	}
}

func TestList_Current_NoDuplicatesPerNaturalKey(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "current-no-dup")
	volID := mustVolcano(t, pool, "Current No Dup")
	observedAt := time.Now().UTC().Add(-24 * time.Hour)

	// Three revisions of the same natural key (volcano_id, kind,
	// observed_at, source_id), each inserted with a real delay so each
	// gets a distinct, increasing ingested_at from the database.
	mustInsert(t, pool, volID, srcID, "magnitude", observedAt, false)
	tick()
	mustInsert(t, pool, volID, srcID, "magnitude", observedAt, false)
	tick()
	last := mustInsert(t, pool, volID, srcID, "magnitude", observedAt, false)

	kind := "magnitude"
	results, err := observation.List(ctx, pool, observation.Query{
		VolcanoID:  &volID,
		Kind:       &kind,
		Provenance: observation.ProvenanceAll,
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly one row for the natural key on a current read, got %d", len(results))
	}
	if !results[0].IngestedAt.Equal(last.IngestedAt) {
		t.Errorf("expected current read to pick the highest ingested_at revision (%s), got %s", last.IngestedAt, results[0].IngestedAt)
	}
}

func TestList_AsOf_Deterministic(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "asof-deterministic")
	volID := mustVolcano(t, pool, "AsOf Deterministic")
	observedAt := time.Now().UTC().Add(-10 * time.Hour)
	mustInsert(t, pool, volID, srcID, "deterministic", observedAt, false)

	tick()
	asOf := time.Now().UTC()
	kind := "deterministic"
	q := observation.Query{VolcanoID: &volID, Kind: &kind, AsOf: &asOf, Provenance: observation.ProvenanceAll}

	first, err := observation.List(ctx, pool, q)
	if err != nil {
		t.Fatalf("first List failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	second, err := observation.List(ctx, pool, q)
	if err != nil {
		t.Fatalf("second List failed: %v", err)
	}

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected both runs to return exactly one row, got %d and %d", len(first), len(second))
	}
	if first[0].ID != second[0].ID || !first[0].IngestedAt.Equal(second[0].IngestedAt) {
		t.Errorf("expected identical results across repeated as-of queries, got %+v and %+v", first[0], second[0])
	}
}

func TestList_NoAsOf_ReturnsCurrentState(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "no-asof-current")
	volID := mustVolcano(t, pool, "No AsOf Current")
	observedAt := time.Now().UTC().Add(-2 * time.Hour)
	mustInsert(t, pool, volID, srcID, "no-asof", observedAt, false)

	kind := "no-asof"
	results, err := observation.List(ctx, pool, observation.Query{VolcanoID: &volID, Kind: &kind, Provenance: observation.ProvenanceAll})
	if err != nil {
		t.Fatalf("List without AsOf failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected the observation to be visible when no AsOf is given, got %d", len(results))
	}
}

// --- 4.2: future as-of rejected -------------------------------------------

func TestList_FutureAsOfRejected(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	future := time.Now().UTC().Add(24 * time.Hour)
	_, err := observation.List(ctx, pool, observation.Query{AsOf: &future, Provenance: observation.ProvenanceAll})
	if err == nil {
		t.Fatal("expected a future as-of instant to be rejected")
	}
	if !errors.Is(err, observation.ErrFutureAsOf) {
		t.Errorf("expected ErrFutureAsOf, got: %v", err)
	}
}

// --- 4.3: provenance filter -------------------------------------------------

func TestList_ProvenanceFilter_RealOnlyExcludesSynthetic(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "provenance-filter")
	volID := mustVolcano(t, pool, "Provenance Filter")
	observedAt := time.Now().UTC().Add(-1 * time.Hour)

	mustInsert(t, pool, volID, srcID, "provenance-real", observedAt, false)
	mustInsert(t, pool, volID, srcID, "provenance-synthetic", observedAt, true)

	results, err := observation.List(ctx, pool, observation.Query{VolcanoID: &volID, Provenance: observation.ProvenanceRealOnly})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	for _, r := range results {
		if r.IsSynthetic {
			t.Fatalf("expected real-only filter to exclude synthetic observations, got %+v", r)
		}
	}

	syntheticResults, err := observation.List(ctx, pool, observation.Query{VolcanoID: &volID, Provenance: observation.ProvenanceSyntheticOnly})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	for _, r := range syntheticResults {
		if !r.IsSynthetic {
			t.Fatalf("expected synthetic-only filter to exclude real observations, got %+v", r)
		}
	}
}

func TestList_ProvenanceUnspecifiedRejected(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	_, err := observation.List(ctx, pool, observation.Query{})
	if !errors.Is(err, observation.ErrProvenanceUnspecified) {
		t.Errorf("expected ErrProvenanceUnspecified when Provenance is not set, got: %v", err)
	}
}

// --- 4.4: caller-supplied ingested_at is ignored ---------------------------

func TestInsert_IgnoresCallerSuppliedIngestedAt(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "insert-ignores-ingested-at")
	volID := mustVolcano(t, pool, "Insert Ignores IngestedAt")

	before := time.Now().UTC()
	obs, err := observation.Insert(ctx, pool, observation.NewObservation{
		VolcanoID: volID, Kind: "ignores-ingested-at", ObservedAt: time.Now().UTC().Add(-72 * time.Hour),
		Value: mustValue(`{}`), IsSynthetic: false, SourceID: srcID,
	})
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	after := time.Now().UTC()

	if obs.IngestedAt.Before(before.Add(-time.Second)) || obs.IngestedAt.After(after.Add(time.Second)) {
		t.Errorf("expected ingested_at to be assigned at persistence time, got %s (window %s..%s)", obs.IngestedAt, before, after)
	}

	// Also confirm this holds even when a caller writes ingested_at
	// directly at the SQL boundary this package owns (the case the spec's
	// "tempo de conhecimento não é fornecido pelo cliente" scenario
	// describes): the database trigger from migration 009 must win over an
	// explicit, deliberately backdated value in the raw INSERT statement.
	past := time.Now().UTC().Add(-72 * time.Hour)
	rawBefore := time.Now().UTC()
	_, err = pool.Exec(ctx, `
		INSERT INTO observations (volcano_id, kind, observed_at, ingested_at, value, is_synthetic, source_id)
		VALUES ($1, 'ignores-ingested-at-raw', $2, $3, '{}'::jsonb, false, $4)
	`, volID, time.Now().UTC(), past, srcID)
	if err != nil {
		t.Fatalf("raw insert failed: %v", err)
	}
	rawAfter := time.Now().UTC()

	kind := "ignores-ingested-at-raw"
	results, err := observation.List(ctx, pool, observation.Query{VolcanoID: &volID, Kind: &kind, Provenance: observation.ProvenanceAll})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly one result, got %d", len(results))
	}
	if results[0].IngestedAt.Equal(past) {
		t.Error("expected caller-supplied ingested_at to be overridden by the database")
	}
	if results[0].IngestedAt.Before(rawBefore.Add(-time.Second)) || results[0].IngestedAt.After(rawAfter.Add(time.Second)) {
		t.Errorf("expected the raw insert's ingested_at to be the persistence instant, got %s (window %s..%s)", results[0].IngestedAt, rawBefore, rawAfter)
	}
}

// --- 4.5: data leakage -------------------------------------------------------

// TestDataLeakage_AsOfNeverSeesFutureKnowledge is the executable form of the
// master spec's "sem informação futura" constraint. It inserts observations
// in an order that does NOT match their observed_at order — including a
// phenomenon from early in the timeline that only arrives (is ingested)
// much later, which is the exact shape that leaks future information if
// ingested_at is not enforced — then walks a sequence of as-of checkpoints
// captured from real wall-clock time between insertions and asserts that no
// returned row has ingested_at later than the checkpoint queried.
//
// If the as-of predicate in List were removed or weakened, this test must
// fail; see the sabotage verification recorded in the task report.
func TestDataLeakage_AsOfNeverSeesFutureKnowledge(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "data-leakage")
	volID := mustVolcano(t, pool, "Data Leakage Volcano")
	kind := "quake_count"
	base := time.Now().UTC().Add(-100 * time.Hour)

	mustInsert(t, pool, volID, srcID, kind, base, false) // phenomenon at base
	tick()
	checkpoint1 := time.Now().UTC()
	tick()

	mustInsert(t, pool, volID, srcID, kind, base.Add(10*time.Hour), false)
	tick()
	checkpoint2 := time.Now().UTC()
	tick()

	// Late-arriving observation: the phenomenon happened at base+2h (older
	// than the row inserted right above), but the system only learns of it
	// now, after checkpoint2.
	lateArriving := mustInsert(t, pool, volID, srcID, kind, base.Add(2*time.Hour), false)
	tick()
	checkpoint3 := time.Now().UTC()
	tick()

	mustInsert(t, pool, volID, srcID, kind, base.Add(20*time.Hour), false)
	tick()
	checkpoint4 := time.Now().UTC()
	tick()

	mustInsert(t, pool, volID, srcID, kind, base.Add(30*time.Hour), false)
	tick()
	checkpoint5 := time.Now().UTC()

	checkpoints := []time.Time{checkpoint1, checkpoint2, checkpoint3, checkpoint4, checkpoint5}
	for _, asOf := range checkpoints {
		asOf := asOf
		results, err := observation.List(ctx, pool, observation.Query{
			VolcanoID:  &volID,
			Kind:       &kind,
			AsOf:       &asOf,
			Provenance: observation.ProvenanceAll,
		})
		if err != nil {
			t.Fatalf("List at checkpoint %s failed: %v", asOf, err)
		}
		for _, r := range results {
			if r.IngestedAt.After(asOf) {
				t.Fatalf("DATA LEAKAGE: as-of query for %s returned observation id=%d with ingested_at=%s, which is knowledge from the future relative to the query instant", asOf, r.ID, r.IngestedAt)
			}
		}
	}

	// Sanity check that the late-arriving row is genuinely invisible until
	// its own ingested_at, so the assertion above isn't vacuously passing
	// because the row was never returned at all: as of checkpoint2 (before
	// it was ingested) it must be absent, and as of checkpoint3 (after) it
	// must be present.
	before, err := observation.List(ctx, pool, observation.Query{VolcanoID: &volID, Kind: &kind, AsOf: &checkpoint2, Provenance: observation.ProvenanceAll})
	if err != nil {
		t.Fatalf("List at checkpoint2 failed: %v", err)
	}
	for _, r := range before {
		if r.ID == lateArriving.ID {
			t.Fatal("expected the late-arriving observation to be invisible before its own ingested_at")
		}
	}

	after, err := observation.List(ctx, pool, observation.Query{VolcanoID: &volID, Kind: &kind, AsOf: &checkpoint3, Provenance: observation.ProvenanceAll})
	if err != nil {
		t.Fatalf("List at checkpoint3 failed: %v", err)
	}
	found := false
	for _, r := range after {
		if r.ID == lateArriving.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("expected the late-arriving observation to become visible once its ingested_at has passed")
	}
}
