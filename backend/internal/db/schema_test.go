package db_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
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

// --- 3.2: observations bitemporal shape --------------------------------

func TestObservations_MissingIsSyntheticRejected(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "obs-missing-synthetic")
	volID := mustVolcano(t, pool, "Obs Missing Synthetic")

	// Deliberately omit is_synthetic: this must fail at the database, not
	// just in application code (design.md D6).
	_, err := pool.Exec(ctx, `
		INSERT INTO observations (volcano_id, kind, observed_at, value, source_id)
		VALUES ($1, 'seismic_count', now(), '{}'::jsonb, $2)
	`, volID, srcID)
	if err == nil {
		t.Fatal("expected INSERT omitting is_synthetic to fail")
	}
	if !strings.Contains(err.Error(), "is_synthetic") {
		t.Errorf("expected error to reference is_synthetic, got: %v", err)
	}
}

func TestObservations_MissingObservedAtRejected(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "obs-missing-observed-at")
	volID := mustVolcano(t, pool, "Obs Missing ObservedAt")

	_, err := pool.Exec(ctx, `
		INSERT INTO observations (volcano_id, kind, value, is_synthetic, source_id)
		VALUES ($1, 'seismic_count', '{}'::jsonb, false, $2)
	`, volID, srcID)
	if err == nil {
		t.Fatal("expected INSERT omitting observed_at to fail")
	}
}

func TestObservations_UnknownSourceRejected(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	volID := mustVolcano(t, pool, "Obs Unknown Source")

	_, err := pool.Exec(ctx, `
		INSERT INTO observations (volcano_id, kind, observed_at, value, is_synthetic, source_id)
		VALUES ($1, 'seismic_count', now(), '{}'::jsonb, false, 999999)
	`, volID)
	if err == nil {
		t.Fatal("expected INSERT with unknown source_id to fail")
	}
}

func TestObservations_IngestedAtAssignedByDatabase(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "obs-ingested-at")
	volID := mustVolcano(t, pool, "Obs IngestedAt")

	past := time.Now().Add(-72 * time.Hour)
	before := time.Now()
	var gotIngestedAt time.Time
	err := pool.QueryRow(ctx, `
		INSERT INTO observations (volcano_id, kind, observed_at, ingested_at, value, is_synthetic, source_id)
		VALUES ($1, 'seismic_count', now(), $2, '{}'::jsonb, false, $3)
		RETURNING ingested_at
	`, volID, past, srcID).Scan(&gotIngestedAt)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	after := time.Now()

	if gotIngestedAt.Before(before.Add(-time.Second)) || gotIngestedAt.After(after.Add(time.Second)) {
		t.Errorf("expected ingested_at to be assigned at persistence time, got %s (window %s..%s)", gotIngestedAt, before, after)
	}
	if gotIngestedAt.Equal(past) {
		t.Error("caller-supplied ingested_at must be ignored")
	}
}

// --- 3.3: earthquakes bitemporal revisions ------------------------------

func TestEarthquakes_TwoRevisionsCoexist(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "eq-revisions")

	_, err := pool.Exec(ctx, `
		INSERT INTO earthquakes (external_id, source_id, occurred_at, location, magnitude, is_synthetic)
		VALUES ('us1000abcd', $1, now() - interval '1 day', ST_GeogFromText('POINT(105.4 -6.1)'), 5.1, false)
	`, srcID)
	if err != nil {
		t.Fatalf("first insert failed: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO earthquakes (external_id, source_id, occurred_at, location, magnitude, is_synthetic)
		VALUES ('us1000abcd', $1, now() - interval '1 day', ST_GeogFromText('POINT(105.4 -6.1)'), 5.4, false)
	`, srcID)
	if err != nil {
		t.Fatalf("revised insert failed (revisions must coexist as new rows): %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM earthquakes WHERE external_id = 'us1000abcd' AND source_id = $1`, srcID).Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 coexisting revisions, got %d", count)
	}
}

// --- 3.4: as-of query uses an index, no sequential scan -----------------

func TestAsOfQuery_UsesIndex(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "asof-index")
	volID := mustVolcano(t, pool, "AsOf Index")

	for i := 0; i < 5; i++ {
		_, err := pool.Exec(ctx, `
			INSERT INTO observations (volcano_id, kind, observed_at, value, is_synthetic, source_id)
			VALUES ($1, 'seismic_count', now() - ($2::int * interval '1 hour'), '{}'::jsonb, false, $3)
		`, volID, int32(i), srcID)
		if err != nil {
			t.Fatalf("seed insert failed: %v", err)
		}
	}

	rows, err := pool.Query(ctx, `
		EXPLAIN
		SELECT DISTINCT ON (volcano_id, kind, observed_at, source_id) *
		FROM observations
		WHERE observed_at BETWEEN now() - interval '10 hours' AND now()
		  AND ingested_at <= now()
		ORDER BY volcano_id, kind, observed_at, source_id, ingested_at DESC
	`)
	if err != nil {
		t.Fatalf("EXPLAIN query failed: %v", err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan plan line: %v", err)
		}
		plan.WriteString(line)
		plan.WriteString("\n")
	}

	planText := plan.String()
	if strings.Contains(planText, "Seq Scan") {
		t.Fatalf("expected as-of query to use an index, got a sequential scan:\n%s", planText)
	}
	if !strings.Contains(planText, "Index") {
		t.Fatalf("expected plan to mention index usage:\n%s", planText)
	}
}

// --- 3.5: data_sources license / attribution / audit --------------------

func TestDataSources_EnablingWithoutLicenseRejected(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO data_sources (name, category, enabled, license)
		VALUES ('No License Source', 'test', true, '')
	`)
	if err == nil {
		t.Fatal("expected enabling a source with empty license to be rejected")
	}
}

func TestDataSources_UpdatedAtChangesOnUpdate(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	var id int64
	var createdUpdatedAt time.Time
	err := pool.QueryRow(ctx, `
		INSERT INTO data_sources (name, category, enabled, license, attribution)
		VALUES ('Audit Source', 'test', false, 'CC-BY-4.0', 'Attribution')
		RETURNING id, updated_at
	`).Scan(&id, &createdUpdatedAt)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	var updatedAt time.Time
	err = pool.QueryRow(ctx, `
		UPDATE data_sources SET base_url = 'https://example.org' WHERE id = $1 RETURNING updated_at
	`, id).Scan(&updatedAt)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if !updatedAt.After(createdUpdatedAt) {
		t.Errorf("expected updated_at to advance on UPDATE: before=%s after=%s", createdUpdatedAt, updatedAt)
	}
}

// --- 3.6: data_sources cannot be deleted while referenced ---------------

func TestDataSources_DeleteReferencedSourceRejected(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "delete-protection")
	volID := mustVolcano(t, pool, "Delete Protection Volcano")
	_, err := pool.Exec(ctx, `
		INSERT INTO observations (volcano_id, kind, observed_at, value, is_synthetic, source_id)
		VALUES ($1, 'seismic_count', now(), '{}'::jsonb, false, $2)
	`, volID, srcID)
	if err != nil {
		t.Fatalf("seed observation failed: %v", err)
	}

	_, err = pool.Exec(ctx, `DELETE FROM data_sources WHERE id = $1`, srcID)
	if err == nil {
		t.Fatal("expected DELETE of a referenced source to fail")
	}

	// Disabling, in contrast, must work.
	_, err = pool.Exec(ctx, `UPDATE data_sources SET enabled = false WHERE id = $1`, srcID)
	if err != nil {
		t.Fatalf("expected disabling a referenced source to succeed: %v", err)
	}
}

// --- 3.7: volcanoes source identity + coordinate bounds ------------------

func TestVolcanoes_InvalidLatitudeRejected(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO volcanoes (name, latitude, longitude) VALUES ('Bad Latitude', 91, 105.4)
	`)
	if err == nil {
		t.Fatal("expected latitude 91 to be rejected")
	}
}

func TestVolcanoes_DuplicateSourceRefRejected(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	srcID := mustSource(t, pool, "volcano-source-ref")

	_, err := pool.Exec(ctx, `
		INSERT INTO volcanoes (name, source_id, source_ref) VALUES ('Volcano A', $1, '12345')
	`, srcID)
	if err != nil {
		t.Fatalf("first insert with source_ref should succeed: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO volcanoes (name, source_id, source_ref) VALUES ('Volcano B', $1, '12345')
	`, srcID)
	if err == nil {
		t.Fatal("expected duplicate (source_id, source_ref) to be rejected")
	}
}
