package source_test

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
	"github.com/vilsonfr/volcanopredict/backend/internal/source"
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

func TestGetByName_NotFound(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)

	_, err := source.GetByName(context.Background(), pool, "Does Not Exist")
	if !errors.Is(err, source.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetEnabledByName_RejectsDisabledSource(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO data_sources (name, category, enabled, license, attribution)
		VALUES ('Unlicensed Source', 'test', false, NULL, NULL)
	`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, err = source.GetEnabledByName(ctx, pool, "Unlicensed Source")
	if !errors.Is(err, source.ErrDisabled) {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}

func TestGetEnabledByName_ResolvesGVPAfterMigration010(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)

	s, err := source.GetEnabledByName(context.Background(), pool, "Smithsonian Global Volcanism Program")
	if err != nil {
		t.Fatalf("expected GVP to be enabled after migration 010's license seed, got: %v", err)
	}
	if s.License == "" || s.Attribution == "" {
		t.Fatalf("expected GVP to carry license and attribution, got %+v", s)
	}
}

// Migrations 011/012 add a second, independent reason a source cannot be
// used: the source itself forbids automated collection. These tests pin
// that a permissive license does NOT override that — the PVMBG case.

func TestEnabledSourceCannotBeBlocked(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)
	ctx := context.Background()

	// A license is present and non-empty, so the 007 constraint is
	// satisfied; only the 011 constraint can reject this.
	_, err := pool.Exec(ctx, `
		INSERT INTO data_sources (name, category, enabled, license, attribution, blocked_reason)
		VALUES ('Blocked But Licensed', 'test', true, 'some-permissive-license', 'Somebody', 'robots.txt forbids it')
	`)
	if err == nil {
		t.Fatal("expected a source with blocked_reason set to be rejected when enabled = true, but the insert succeeded")
	}
	if !strings.Contains(err.Error(), "data_sources_enabled_requires_not_blocked") {
		t.Fatalf("expected the blocked-source constraint to reject this, got: %v", err)
	}
}

func TestBlockedSourceMayExistWhileDisabled(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO data_sources (name, category, enabled, license, blocked_reason)
		VALUES ('Blocked And Disabled', 'test', false, 'some-permissive-license', 'robots.txt forbids it')
	`)
	if err != nil {
		t.Fatalf("a blocked source must still be registrable while disabled, for documentation: %v", err)
	}
}

func TestGetEnabledByName_ResolvesUSGSAfterMigration012(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)

	s, err := source.GetEnabledByName(context.Background(), pool, "USGS Earthquake Hazards Program")
	if err != nil {
		t.Fatalf("expected USGS to be enabled after migration 012's license seed, got: %v", err)
	}
	if s.License == "" || s.Attribution == "" {
		t.Fatalf("expected USGS to carry license and attribution, got %+v", s)
	}
}

func TestMigration012_PVMBGStaysBlockedAndDisabled(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)
	ctx := context.Background()

	var enabled bool
	var blocked, policy string
	err := pool.QueryRow(ctx, `
		SELECT enabled, coalesce(blocked_reason, ''), coalesce(collection_policy, '')
		FROM data_sources WHERE name = 'PVMBG'
	`).Scan(&enabled, &blocked, &policy)
	if err != nil {
		t.Fatalf("PVMBG must remain registered for documentation: %v", err)
	}
	if enabled {
		t.Fatal("PVMBG must not be enabled: the source forbids automated collection")
	}
	if blocked == "" {
		t.Fatal("PVMBG must carry the reason it is blocked, not just be silently disabled")
	}
	if !strings.Contains(policy, "Disallow") {
		t.Fatalf("expected the declared collection policy to record the robots.txt rule, got %q", policy)
	}
}

// The failure mode this guards against is a licensing migration enabling
// more rows than it meant to. After 012, exactly the two reviewed sources
// may be enabled.
func TestMigration012_EnablesOnlyTheReviewedSources(t *testing.T) {
	db := dbtest.Start(t)
	pool := mustPool(t, db.ConnString)

	srcs, err := source.List(context.Background(), pool)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var enabled []string
	for _, s := range srcs {
		if s.Enabled {
			enabled = append(enabled, s.Name)
		}
	}
	want := []string{"Smithsonian Global Volcanism Program", "USGS Earthquake Hazards Program"}
	sort.Strings(enabled)
	sort.Strings(want)
	if !slices.Equal(enabled, want) {
		t.Fatalf("enabled sources drifted.\n got: %v\nwant: %v", enabled, want)
	}
}
