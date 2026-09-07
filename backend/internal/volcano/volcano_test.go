package volcano_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
	"github.com/vilsonfr/volcanopredict/backend/internal/volcano"
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
		 VALUES ($1, 'test', true, 'CC-BY-4.0', 'Test Source') RETURNING id`, name).Scan(&id)
	if err != nil {
		t.Fatalf("failed to insert test data source: %v", err)
	}
	return id
}

func TestUpsert_RejectsMissingAttribution(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)

	_, _, err := volcano.Upsert(context.Background(), pool, volcano.Record{
		Name: "No Source Volcano", Latitude: 1, Longitude: 1,
	})
	if err == nil {
		t.Fatal("expected Upsert to reject a record without source_id/source_ref")
	}
}

func TestUpsert_RejectsInvalidCoordinate(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)
	srcID := mustSource(t, pool, "src-invalid-coord")

	_, _, err := volcano.Upsert(context.Background(), pool, volcano.Record{
		SourceID: srcID, SourceRef: "1", Name: "Bad Coord", Latitude: 200, Longitude: 1,
	})
	if err == nil {
		t.Fatal("expected Upsert to reject an out-of-range latitude")
	}
}

func TestUpsert_InsertThenIdempotentThenUpdate(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)
	srcID := mustSource(t, pool, "src-lifecycle")

	rec := volcano.Record{SourceID: srcID, SourceRef: "100", Name: "Mount Fixture", Country: "Testlandia", Latitude: 1, Longitude: 2}

	first, outcome, err := volcano.Upsert(context.Background(), pool, rec)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if outcome != volcano.OutcomeInserted {
		t.Fatalf("expected OutcomeInserted, got %v", outcome)
	}

	again, outcome, err := volcano.Upsert(context.Background(), pool, rec)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if outcome != volcano.OutcomeUnchanged {
		t.Fatalf("expected OutcomeUnchanged on identical reimport, got %v", outcome)
	}
	if again.ID != first.ID {
		t.Fatalf("expected stable id across reimport, got %d then %d", first.ID, again.ID)
	}

	rec.Country = "Neverland" // attribute changed at the source
	updated, outcome, err := volcano.Upsert(context.Background(), pool, rec)
	if err != nil {
		t.Fatalf("third upsert: %v", err)
	}
	if outcome != volcano.OutcomeUpdated {
		t.Fatalf("expected OutcomeUpdated after a changed attribute, got %v", outcome)
	}
	if updated.ID != first.ID {
		t.Fatalf("expected stable id after update, got %d then %d", first.ID, updated.ID)
	}
	if updated.Country != "Neverland" {
		t.Fatalf("expected updated country to persist, got %q", updated.Country)
	}
}

func TestMarkAbsent_ThenReappearClearsFlag(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)
	srcID := mustSource(t, pool, "src-absence")
	ctx := context.Background()

	rec := volcano.Record{SourceID: srcID, SourceRef: "200", Name: "Vanishing Peak", Latitude: 1, Longitude: 1}
	created, _, err := volcano.Upsert(ctx, pool, rec)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	marked, err := volcano.MarkAbsent(ctx, pool, srcID, []string{})
	if err != nil {
		t.Fatalf("mark absent: %v", err)
	}
	if marked != 1 {
		t.Fatalf("expected 1 volcano marked absent, got %d", marked)
	}

	v, err := volcano.GetBySourceRef(ctx, pool, srcID, "200")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if v.AbsentFromSourceAt == nil {
		t.Fatal("expected absent_from_source_at to be set")
	}
	if v.ID != created.ID {
		t.Fatalf("expected same id preserved while marked absent, got %d then %d", created.ID, v.ID)
	}

	// Volcano reappears in a later run: Upsert must clear the flag.
	reappeared, outcome, err := volcano.Upsert(ctx, pool, rec)
	if err != nil {
		t.Fatalf("reappear upsert: %v", err)
	}
	if outcome != volcano.OutcomeUpdated {
		t.Fatalf("expected OutcomeUpdated when clearing absence, got %v", outcome)
	}
	if reappeared.ID != created.ID {
		t.Fatalf("expected same id after reappearing, got %d then %d", created.ID, reappeared.ID)
	}

	v, err = volcano.GetBySourceRef(ctx, pool, srcID, "200")
	if err != nil {
		t.Fatalf("get after reappear: %v", err)
	}
	if v.AbsentFromSourceAt != nil {
		t.Fatal("expected absent_from_source_at to be cleared after reappearing")
	}
}
