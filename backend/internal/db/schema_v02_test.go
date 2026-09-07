package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
)

// --- 2.1/2.2: the ingestion columns on earthquakes ----------------------

// The point of quality_state having no DEFAULT: "nobody evaluated this"
// must never be readable as "this is fine".
func TestEarthquakes_InsertWithoutQualityStateFails(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()
	srcID := mustSource(t, pool, "eq-quality-required")

	_, err := pool.Exec(ctx, `
		INSERT INTO earthquakes (external_id, source_id, occurred_at, location, is_synthetic, parser_version, raw)
		VALUES ('nq1', $1, now(), ST_GeogFromText('POINT(0 0)'), false, 'test', '{}'::jsonb)
	`, srcID)
	if err == nil {
		t.Fatal("expected INSERT omitting quality_state to fail: the column must have no DEFAULT")
	}
	if !strings.Contains(err.Error(), "quality_state") {
		t.Fatalf("expected the error to name quality_state, got: %v", err)
	}
}

func TestEarthquakes_InsertWithoutParserVersionOrRawFails(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()
	srcID := mustSource(t, pool, "eq-provenance-required")

	for _, tc := range []struct {
		name string
		sql  string
	}{
		{"sem parser_version", `
			INSERT INTO earthquakes (external_id, source_id, occurred_at, location, is_synthetic, quality_state, raw)
			VALUES ('np1', $1, now(), ST_GeogFromText('POINT(0 0)'), false, 'valid', '{}'::jsonb)`},
		{"sem raw", `
			INSERT INTO earthquakes (external_id, source_id, occurred_at, location, is_synthetic, quality_state, parser_version)
			VALUES ('nr1', $1, now(), ST_GeogFromText('POINT(0 0)'), false, 'valid', 'test')`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, tc.sql, srcID); err == nil {
				t.Fatal("expected the insert to be rejected: reconstructing a value from its origin depends on these columns")
			}
		})
	}
}

func TestEarthquakes_UnknownQualityStateRejected(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()
	srcID := mustSource(t, pool, "eq-quality-enum")

	_, err := pool.Exec(ctx, `
		INSERT INTO earthquakes (external_id, source_id, occurred_at, location, is_synthetic, quality_state, quality_reason, parser_version, raw)
		VALUES ('bad1', $1, now(), ST_GeogFromText('POINT(0 0)'), false, 'probably-fine', 'alguma razao', 'test', '{}'::jsonb)
	`, srcID)
	if err == nil {
		t.Fatal("expected an invented quality state to be rejected by the database")
	}
	if !strings.Contains(err.Error(), "earthquakes_quality_state_known") {
		t.Fatalf("expected the quality-state constraint to reject this, got: %v", err)
	}
}

// A non-valid state without a reason is a mark nobody can act on.
func TestEarthquakes_NonValidStateRequiresReason(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()
	srcID := mustSource(t, pool, "eq-quality-reason")

	_, err := pool.Exec(ctx, `
		INSERT INTO earthquakes (external_id, source_id, occurred_at, location, is_synthetic, quality_state, parser_version, raw)
		VALUES ('sus1', $1, now(), ST_GeogFromText('POINT(0 0)'), false, 'suspect', 'test', '{}'::jsonb)
	`, srcID)
	if err == nil {
		t.Fatal("expected a suspect row with no reason to be rejected")
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO earthquakes (external_id, source_id, occurred_at, location, is_synthetic, quality_state, quality_reason, parser_version, raw)
		VALUES ('sus2', $1, now(), ST_GeogFromText('POINT(0 0)'), false, 'suspect', 'impossible_magnitude', 'test', '{}'::jsonb)
	`, srcID)
	if err != nil {
		t.Fatalf("a suspect row carrying its reason must be accepted: %v", err)
	}
}

// The 006 indexes must still serve the as-of read after 013/014 widened
// the table; a sequential scan here would be a silent regression.
func TestEarthquakes_AsOfStillUsesIndexAfterV02Columns(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()
	srcID := mustSource(t, pool, "eq-index-after-v02")

	for _, id := range []string{"a", "b", "c"} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO earthquakes (external_id, source_id, occurred_at, location, is_synthetic, quality_state, parser_version, raw)
			VALUES ($2, $1, now(), ST_GeogFromText('POINT(0 0)'), false, 'valid', 'test', '{}'::jsonb)
		`, srcID, id); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `ANALYZE earthquakes`); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	// EXPLAIN devolve uma linha por linha do plano: ler só a primeira
	// esconderia justamente onde o índice aparece.
	if _, err := pool.Exec(ctx, `SET enable_seqscan = off`); err != nil {
		t.Fatalf("disable seqscan: %v", err)
	}
	rows, err := pool.Query(ctx, `
		EXPLAIN
		SELECT DISTINCT ON (external_id, source_id) external_id, magnitude
		FROM earthquakes
		WHERE source_id = $1 AND ingested_at <= now()
		ORDER BY external_id, source_id, ingested_at DESC
	`, srcID)
	if err != nil {
		t.Fatalf("explain: %v", err)
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
	if err := rows.Err(); err != nil {
		t.Fatalf("plan rows: %v", err)
	}

	planText := plan.String()
	if strings.Contains(planText, "Seq Scan") {
		t.Fatalf("expected an index to still back the as-of read after 013/014, got a sequential scan:\n%s", planText)
	}
	if !strings.Contains(planText, "earthquakes_natural_key_ingested_idx") {
		t.Fatalf("expected the natural-key index from migration 006 to be usable, plan was:\n%s", planText)
	}
}

// --- 2.3: ingestion_runs ------------------------------------------------

func TestIngestionRuns_RunsCoexistAndLatestIsRecoverable(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()
	srcID := mustSource(t, pool, "runs-coexist")

	for _, r := range []struct {
		result string
		errMsg any
	}{{"success", nil}, {"failure", "source unreachable"}} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO ingestion_runs (source_id, finished_at, window_start, window_end, mode, result, error_message)
			VALUES ($1, now(), now() - interval '1 hour', now(), 'incremental', $2, $3)
		`, srcID, r.result, r.errMsg); err != nil {
			t.Fatalf("insert %s run: %v", r.result, err)
		}
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_runs WHERE source_id = $1`, srcID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected both runs to coexist (a failure must not overwrite the previous run), got %d", count)
	}

	var latest string
	if err := pool.QueryRow(ctx, `
		SELECT result FROM ingestion_runs WHERE source_id = $1 ORDER BY started_at DESC, id DESC LIMIT 1
	`, srcID).Scan(&latest); err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest != "failure" {
		t.Fatalf("expected the most recent run to be the failure, got %q", latest)
	}
}

func TestIngestionRuns_ConstraintsRejectIncoherentRows(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()
	srcID := mustSource(t, pool, "runs-constraints")

	for _, tc := range []struct {
		name string
		sql  string
	}{
		{"falha sem causa", `
			INSERT INTO ingestion_runs (source_id, finished_at, window_start, window_end, mode, result)
			VALUES ($1, now(), now() - interval '1 hour', now(), 'incremental', 'failure')`},
		{"janela invertida", `
			INSERT INTO ingestion_runs (source_id, finished_at, window_start, window_end, mode, result)
			VALUES ($1, now(), now(), now() - interval '1 hour', 'incremental', 'success')`},
		{"terminada sem fim", `
			INSERT INTO ingestion_runs (source_id, window_start, window_end, mode, result)
			VALUES ($1, now() - interval '1 hour', now(), 'incremental', 'success')`},
		{"em andamento com fim", `
			INSERT INTO ingestion_runs (source_id, finished_at, window_start, window_end, mode, result)
			VALUES ($1, now(), now() - interval '1 hour', now(), 'incremental', 'running')`},
		{"resultado inventado", `
			INSERT INTO ingestion_runs (source_id, finished_at, window_start, window_end, mode, result)
			VALUES ($1, now(), now() - interval '1 hour', now(), 'incremental', 'kinda-worked')`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, tc.sql, srcID); err == nil {
				t.Fatal("expected the database to reject this row")
			}
		})
	}

	// And the coherent shapes are accepted.
	if _, err := pool.Exec(ctx, `
		INSERT INTO ingestion_runs (source_id, window_start, window_end, mode, result)
		VALUES ($1, now() - interval '1 hour', now(), 'incremental', 'running')
	`, srcID); err != nil {
		t.Fatalf("an in-flight run must be representable: %v", err)
	}
}
