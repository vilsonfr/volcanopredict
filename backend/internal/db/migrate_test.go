package db_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vilsonfr/volcanopredict/backend/internal/db"
	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
)

// TestMigrate_IdempotentReapply covers task 2.3: applying migrations twice
// in a row must not reapply anything or fail (ambiente-local spec, "Banco
// já migrado").
func TestMigrate_IdempotentReapply(t *testing.T) {
	tdb := dbtest.Start(t)

	if err := db.Migrate(tdb.ConnString); err != nil {
		t.Fatalf("second Migrate call should be a no-op, got error: %v", err)
	}

	v1, err := db.SchemaVersion(tdb.ConnString)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if err := db.Migrate(tdb.ConnString); err != nil {
		t.Fatalf("third Migrate call should also be a no-op: %v", err)
	}
	v2, err := db.SchemaVersion(tdb.ConnString)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if v1 != v2 {
		t.Fatalf("schema version changed across idempotent re-applies: %d -> %d", v1, v2)
	}
}

// TestMigrate_FreshVolume covers the new-volume half of task 3.1: only
// PostGIS present, no tables at all, exactly what a volume looks like
// today now that docker-entrypoint-initdb.d is no longer mounted.
func TestMigrate_FreshVolume(t *testing.T) {
	tdb := dbtest.Start(t)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, tdb.ConnString)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM data_sources`).Scan(&count); err != nil {
		t.Fatalf("expected data_sources to exist and be queryable: %v", err)
	}
	if count == 0 {
		t.Fatal("expected seeded data_sources rows to exist")
	}
}

// TestMigrate_LegacyVolume covers the other half of task 3.1: a volume
// that already has the 001/002 schema applied directly (as
// docker-entrypoint-initdb.d used to do), with goose's version table
// still empty because goose never ran against it. Migration 003 must
// reconcile this into the same end state as a fresh volume, without
// duplicating the seeded data_sources rows.
func TestMigrate_LegacyVolume(t *testing.T) {
	dbtest.Skip(t)

	// Start a database with only PostGIS enabled (mirrors dbtest.Start's
	// container before any goose migration runs), then apply 001/002
	// exactly as the old init script did, bypassing goose entirely.
	container := dbtest.NewBareContainer(t)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, container.ConnString)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	for _, name := range []string{"001_init.sql", "002_sources.sql"} {
		sqlBytes, err := os.ReadFile(legacyMigrationPath(t, name))
		if err != nil {
			t.Fatalf("failed to read legacy migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("failed to apply legacy migration %s directly: %v", name, err)
		}
	}

	var preexistingSourceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM data_sources`).Scan(&preexistingSourceCount); err != nil {
		t.Fatalf("expected legacy data_sources to exist before migrating: %v", err)
	}
	if preexistingSourceCount == 0 {
		t.Fatal("legacy 002_sources.sql should have seeded rows")
	}

	// Now hand the same, already-legacy-populated database to goose.
	if err := db.Migrate(container.ConnString); err != nil {
		t.Fatalf("Migrate should reconcile a legacy volume, got error: %v", err)
	}

	var sourceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM data_sources`).Scan(&sourceCount); err != nil {
		t.Fatalf("data_sources query after migrate: %v", err)
	}
	if sourceCount != preexistingSourceCount {
		t.Fatalf("expected data_sources to converge without duplication: had %d, now %d", preexistingSourceCount, sourceCount)
	}

	// observations/earthquakes must now be the bitemporal shape, proving
	// the DROP/CREATE in 003+ actually ran on top of the legacy schema.
	var isSyntheticNullable string
	err = pool.QueryRow(ctx, `
		SELECT is_nullable FROM information_schema.columns
		WHERE table_name = 'observations' AND column_name = 'is_synthetic'
	`).Scan(&isSyntheticNullable)
	if err != nil {
		t.Fatalf("expected observations.is_synthetic to exist post-migration: %v", err)
	}
	if isSyntheticNullable != "NO" {
		t.Fatalf("expected observations.is_synthetic to be NOT NULL, got is_nullable=%s", isSyntheticNullable)
	}
}

func legacyMigrationPath(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations", name)
}

// TestMigrate_FailureBlocksServing covers the "migração falha" scenario:
// a bad connection string must produce an error, never a silent partial
// success.
func TestMigrate_FailureBlocksServing(t *testing.T) {
	dbtest.Skip(t)
	err := db.Migrate("postgres://nobody:nothing@127.0.0.1:1/nonexistent?sslmode=disable")
	if err == nil {
		t.Fatal("expected Migrate to fail against an unreachable database")
	}
}
