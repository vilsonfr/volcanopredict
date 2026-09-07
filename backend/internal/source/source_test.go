package source_test

import (
	"context"
	"errors"
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
