// Package ingestion records what was collected and when, and answers the
// question that the data table alone cannot: was this window actually
// looked at?
//
// This is not telemetry. The §74 rule — never interpret absence of data as
// absence of activity — is unenforceable without it: a window with no
// earthquakes and a window with no collection both look like zero rows in
// `earthquakes`. Coverage is derived from the runs recorded here, never
// inferred from the data (design.md D6).
package ingestion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Querier is the subset of pgxpool.Pool (or pgx.Tx) this package needs.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Result is how a run ended.
type Result string

const (
	ResultRunning Result = "running"
	ResultSuccess Result = "success"
	ResultFailure Result = "failure"
	// ResultSkipped is a cycle that deliberately did not run — most often
	// because the previous cycle for the same source was still going.
	ResultSkipped Result = "skipped"
)

// Mode distinguishes how the window was chosen, because it changes how the
// next run's anchor is computed.
type Mode string

const (
	// ModeIncremental: window anchored on the last successful run, asking
	// the source what changed since then.
	ModeIncremental Mode = "incremental"
	// ModeBackfill: an explicit historical window.
	ModeBackfill Mode = "backfill"
	// ModeManual: an explicit window asked for by an operator.
	ModeManual Mode = "manual"
)

// Run is one recorded ingestion attempt.
type Run struct {
	ID          int64
	SourceID    int64
	StartedAt   time.Time
	FinishedAt  *time.Time
	WindowStart time.Time
	WindowEnd   time.Time
	Mode        Mode
	Result      Result
	Inserted    int
	Updated     int
	Unchanged   int
	Rejected    int
	ErrorMsg    string
}

// Counts is the tally a run reports.
type Counts struct {
	Inserted  int
	Updated   int
	Unchanged int
	Rejected  int
}

// ErrNoRuns is returned when a source has never been collected.
var ErrNoRuns = errors.New("ingestion: source has no recorded runs")

const runColumns = `
	id, source_id, started_at, finished_at, window_start, window_end,
	mode, result, inserted, updated, unchanged, rejected, coalesce(error_message, '')
`

// Begin records the start of a run and returns it. The run is left in
// ResultRunning until Finish or Fail closes it, so a process killed
// mid-ingestion leaves a run that is visibly unfinished rather than
// silently absent.
func Begin(ctx context.Context, q Querier, sourceID int64, mode Mode, windowStart, windowEnd time.Time) (Run, error) {
	if windowEnd.Before(windowStart) {
		return Run{}, fmt.Errorf("ingestion: window ends before it starts (%s..%s)", windowStart, windowEnd)
	}
	rows, err := q.Query(ctx, fmt.Sprintf(`
		INSERT INTO ingestion_runs (source_id, window_start, window_end, mode, result)
		VALUES ($1, $2, $3, $4, 'running')
		RETURNING %s
	`, runColumns), sourceID, windowStart.UTC(), windowEnd.UTC(), string(mode))
	if err != nil {
		return Run{}, fmt.Errorf("ingestion: recording run start failed: %w", err)
	}
	defer rows.Close()
	return scanOne(rows)
}

// Finish closes a run as successful with its counts.
func Finish(ctx context.Context, q Querier, runID int64, c Counts) (Run, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		UPDATE ingestion_runs
		SET finished_at = now(), result = 'success',
		    inserted = $2, updated = $3, unchanged = $4, rejected = $5
		WHERE id = $1
		RETURNING %s
	`, runColumns), runID, c.Inserted, c.Updated, c.Unchanged, c.Rejected)
	if err != nil {
		return Run{}, fmt.Errorf("ingestion: closing run %d failed: %w", runID, err)
	}
	defer rows.Close()
	return scanOne(rows)
}

// Fail closes a run as failed, with the cause.
//
// It writes a NEW state onto this run only; the previous run's record is
// untouched. A failure must never erase the evidence that an earlier
// collection succeeded.
func Fail(ctx context.Context, q Querier, runID int64, cause error, c Counts) (Run, error) {
	msg := "unknown error"
	if cause != nil {
		msg = cause.Error()
	}
	rows, err := q.Query(ctx, fmt.Sprintf(`
		UPDATE ingestion_runs
		SET finished_at = now(), result = 'failure', error_message = $2,
		    inserted = $3, updated = $4, unchanged = $5, rejected = $6
		WHERE id = $1
		RETURNING %s
	`, runColumns), runID, msg, c.Inserted, c.Updated, c.Unchanged, c.Rejected)
	if err != nil {
		return Run{}, fmt.Errorf("ingestion: recording run %d failure: %w", runID, err)
	}
	defer rows.Close()
	return scanOne(rows)
}

// Skip records a cycle that deliberately did not run.
func Skip(ctx context.Context, q Querier, sourceID int64, mode Mode, windowStart, windowEnd time.Time, reason string) (Run, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		INSERT INTO ingestion_runs (source_id, finished_at, window_start, window_end, mode, result, error_message)
		VALUES ($1, now(), $2, $3, $4, 'skipped', $5)
		RETURNING %s
	`, runColumns), sourceID, windowStart.UTC(), windowEnd.UTC(), string(mode), reason)
	if err != nil {
		return Run{}, fmt.Errorf("ingestion: recording skipped cycle failed: %w", err)
	}
	defer rows.Close()
	return scanOne(rows)
}

// LastRun returns the most recent run for a source, whatever its result.
func LastRun(ctx context.Context, q Querier, sourceID int64) (Run, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM ingestion_runs WHERE source_id = $1
		ORDER BY started_at DESC, id DESC LIMIT 1
	`, runColumns), sourceID)
	if err != nil {
		return Run{}, fmt.Errorf("ingestion: last run lookup failed: %w", err)
	}
	defer rows.Close()
	run, err := scanOne(rows)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, fmt.Errorf("%w (source %d)", ErrNoRuns, sourceID)
	}
	return run, err
}

// LastSuccess returns the most recent successful run for a source. It is
// what the incremental anchor is computed from.
func LastSuccess(ctx context.Context, q Querier, sourceID int64) (Run, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM ingestion_runs WHERE source_id = $1 AND result = 'success'
		ORDER BY window_end DESC, id DESC LIMIT 1
	`, runColumns), sourceID)
	if err != nil {
		return Run{}, fmt.Errorf("ingestion: last successful run lookup failed: %w", err)
	}
	defer rows.Close()
	run, err := scanOne(rows)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, fmt.Errorf("%w with result success (source %d)", ErrNoRuns, sourceID)
	}
	return run, err
}

func scanOne(rows pgx.Rows) (Run, error) {
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Run{}, fmt.Errorf("ingestion: query failed: %w", err)
		}
		return Run{}, pgx.ErrNoRows
	}
	var r Run
	var mode, result string
	if err := rows.Scan(
		&r.ID, &r.SourceID, &r.StartedAt, &r.FinishedAt, &r.WindowStart, &r.WindowEnd,
		&mode, &result, &r.Inserted, &r.Updated, &r.Unchanged, &r.Rejected, &r.ErrorMsg,
	); err != nil {
		return Run{}, fmt.Errorf("ingestion: scan failed: %w", err)
	}
	r.Mode = Mode(mode)
	r.Result = Result(result)
	r.StartedAt = r.StartedAt.UTC()
	r.WindowStart = r.WindowStart.UTC()
	r.WindowEnd = r.WindowEnd.UTC()
	if r.FinishedAt != nil {
		f := r.FinishedAt.UTC()
		r.FinishedAt = &f
	}
	return r, nil
}
