package gvp_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
	"github.com/vilsonfr/volcanopredict/backend/internal/gvp"
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
		 VALUES ($1, 'test', true, 'public-domain-us-govt-work-attribution-required', 'Test GVP') RETURNING id`, name).Scan(&id)
	if err != nil {
		t.Fatalf("failed to insert test data source: %v", err)
	}
	return id
}

// openFixture opens the hand-written synthetic fixture — never real GVP
// data. See backend/testdata/README.md.
func openFixture(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open("../../testdata/gvp_holocene_fixture.csv")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestImport_InitialRun(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)
	srcID := mustSource(t, pool, "src-import-initial")
	ctx := context.Background()

	report, err := gvp.Import(ctx, pool, srcID, openFixture(t))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	// The fixture has 4 rows; one (FIXTURE-0003) carries an out-of-range
	// latitude and must be rejected, not imported.
	if report.Inserted != 3 {
		t.Fatalf("expected 3 inserted, got %d (report=%+v)", report.Inserted, report)
	}
	if report.Updated != 0 || report.Unchanged != 0 {
		t.Fatalf("expected no updates/unchanged on first run, got %+v", report)
	}
	if len(report.Rejected) != 1 {
		t.Fatalf("expected exactly 1 rejected record, got %d: %+v", len(report.Rejected), report.Rejected)
	}
	if report.Rejected[0].SourceRef != "FIXTURE-0003" {
		t.Fatalf("expected the rejected record to be FIXTURE-0003, got %+v", report.Rejected[0])
	}

	if _, err := volcano.GetBySourceRef(ctx, pool, srcID, "FIXTURE-0003"); err == nil {
		t.Fatal("expected the invalid-coordinate record to never reach the catalog")
	}
}

func TestImport_SecondRunWithoutChangesIsIdempotent(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)
	srcID := mustSource(t, pool, "src-import-idempotent")
	ctx := context.Background()

	first, err := gvp.Import(ctx, pool, srcID, openFixture(t))
	if err != nil {
		t.Fatalf("first Import: %v", err)
	}
	if first.Inserted == 0 {
		t.Fatalf("expected the first import to insert something, got %+v", first)
	}

	first1, err := volcano.GetBySourceRef(ctx, pool, srcID, "FIXTURE-0001")
	if err != nil {
		t.Fatalf("get after first import: %v", err)
	}

	second, err := gvp.Import(ctx, pool, srcID, openFixture(t))
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if second.Inserted != 0 || second.Updated != 0 {
		t.Fatalf("expected zero inserts and zero updates on unchanged reimport, got %+v", second)
	}
	if second.Unchanged != 3 {
		t.Fatalf("expected 3 unchanged records on reimport, got %+v", second)
	}

	second1, err := volcano.GetBySourceRef(ctx, pool, srcID, "FIXTURE-0001")
	if err != nil {
		t.Fatalf("get after second import: %v", err)
	}
	if second1.ID != first1.ID {
		t.Fatalf("expected stable id across reimport, got %d then %d", first1.ID, second1.ID)
	}
}

func TestImport_AttributeChangeIsAnUpdate(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)
	srcID := mustSource(t, pool, "src-import-update")
	ctx := context.Background()

	if _, err := gvp.Import(ctx, pool, srcID, openFixture(t)); err != nil {
		t.Fatalf("first Import: %v", err)
	}

	changed := strings.Replace(
		mustReadFixture(t),
		"FIXTURE-0001,Testonia Peak,Testlandia,-6.1,105.4,741,Historical",
		"FIXTURE-0001,Testonia Peak,Testlandia,-6.1,105.4,900,Historical",
		1,
	)

	report, err := gvp.Import(ctx, pool, srcID, strings.NewReader(changed))
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if report.Updated != 1 {
		t.Fatalf("expected exactly 1 update for the changed elevation, got %+v", report)
	}

	v, err := volcano.GetBySourceRef(ctx, pool, srcID, "FIXTURE-0001")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if v.ElevationM == nil || *v.ElevationM != 900 {
		t.Fatalf("expected elevation to be updated to 900, got %+v", v.ElevationM)
	}
}

func TestImport_VolcanoMissingFromLaterRunIsMarkedAbsentNotDeleted(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)
	srcID := mustSource(t, pool, "src-import-absence")
	ctx := context.Background()

	if _, err := gvp.Import(ctx, pool, srcID, openFixture(t)); err != nil {
		t.Fatalf("first Import: %v", err)
	}

	shrunk := "Volcano Number,Volcano Name,Country,Latitude,Longitude,Elevation (m),Status\n" +
		"FIXTURE-0001,Testonia Peak,Testlandia,-6.1,105.4,741,Historical\n"

	report, err := gvp.Import(ctx, pool, srcID, strings.NewReader(shrunk))
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if report.MarkedAbsent != 2 {
		t.Fatalf("expected 2 volcanoes marked absent (FIXTURE-0002, FIXTURE-0004), got %+v", report)
	}

	v, err := volcano.GetBySourceRef(ctx, pool, srcID, "FIXTURE-0002")
	if err != nil {
		t.Fatalf("expected the missing volcano to still exist in the catalog, got error: %v", err)
	}
	if v.AbsentFromSourceAt == nil {
		t.Fatal("expected the missing volcano to be marked absent, not just left untouched")
	}
}

func mustReadFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../testdata/gvp_holocene_fixture.csv")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(data)
}
