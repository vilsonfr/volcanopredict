package db_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/vilsonfr/volcanopredict/backend/internal/db"
	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
	"github.com/vilsonfr/volcanopredict/backend/migrations"
)

// Task 2.4. The V0.2 migrations are the first to run against a database
// that already holds real data, so the question that matters is not "do
// they apply?" but "do they apply *without destroying what is there?*".
//
// This walks the actual upgrade path: migrate to the V0.1 schema (version
// 10), put an earthquake in it, then run the migrations the way the
// backend does on boot, and check the row is still there.
func TestMigrate_OverAlreadyMigratedV01Database(t *testing.T) {
	tdb := dbtest.NewBareContainer(t)
	ctx := context.Background()

	sqlDB, err := sql.Open("pgx", tdb.ConnString)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	// Stop exactly where V0.1 stopped.
	if err := goose.UpTo(sqlDB, ".", 10); err != nil {
		t.Fatalf("migrating to the V0.1 schema failed: %v", err)
	}

	pool, err := pgxpool.New(ctx, tdb.ConnString)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	var srcID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM data_sources WHERE name = 'USGS Earthquake Hazards Program'
	`).Scan(&srcID); err != nil {
		t.Fatalf("resolve source: %v", err)
	}

	// A row written under the V0.1 shape: no quality_state, no raw — those
	// columns do not exist yet at version 10.
	if _, err := pool.Exec(ctx, `
		INSERT INTO earthquakes (external_id, source_id, occurred_at, location, magnitude, is_synthetic)
		VALUES ('legacy-eq-1', $1, now() - interval '2 days', ST_GeogFromText('POINT(105.4 -6.1)'), 5.1, false)
	`, srcID); err != nil {
		t.Fatalf("seeding a V0.1-shaped earthquake failed: %v", err)
	}

	// Now the upgrade, exactly as the backend performs it on boot.
	if err := db.Migrate(tdb.ConnString); err != nil {
		t.Fatalf("upgrading a populated V0.1 database failed: %v", err)
	}

	version, err := db.SchemaVersion(tdb.ConnString)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version < 15 {
		t.Fatalf("expected the V0.2 migrations to be applied, schema version is %d", version)
	}

	var magnitude float64
	var quality, reason string
	if err := pool.QueryRow(ctx, `
		SELECT magnitude, quality_state, coalesce(quality_reason, '')
		FROM earthquakes WHERE external_id = 'legacy-eq-1'
	`).Scan(&magnitude, &quality, &reason); err != nil {
		t.Fatalf("the pre-existing earthquake must survive the upgrade: %v", err)
	}
	if magnitude != 5.1 {
		t.Fatalf("pre-existing data was altered: magnitude %v", magnitude)
	}

	// A row written before the quality engine existed went through no
	// check at all. Backfilling it as 'valid' would assert it passed
	// checks that never ran, so the migration must mark it as
	// unevaluated instead.
	if quality == "valid" {
		t.Fatal("a row that predates the quality engine must not be backfilled as valid: nothing ever evaluated it")
	}
	if reason == "" {
		t.Fatal("the backfilled state must carry the reason it is not valid")
	}

	// Re-running must still be a no-op on a populated database.
	if err := db.Migrate(tdb.ConnString); err != nil {
		t.Fatalf("re-applying migrations on a populated database must be a no-op: %v", err)
	}
	after, err := db.SchemaVersion(tdb.ConnString)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if after != version {
		t.Fatalf("schema version moved on a no-op re-apply: %d -> %d", version, after)
	}
}
