package ingestion

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// CoverageKind says how much of a requested window was actually collected.
type CoverageKind string

const (
	// CoverageComplete: every instant of the window falls inside a
	// successful run's window.
	CoverageComplete CoverageKind = "complete"
	// CoveragePartial: some of it was collected, some was not.
	CoveragePartial CoverageKind = "partial"
	// CoverageNone: none of it was collected. A query over this window
	// returning nothing says nothing about whether anything happened.
	CoverageNone CoverageKind = "none"
)

// Gap is a stretch of a window that was never successfully collected.
type Gap struct {
	Start time.Time
	End   time.Time
}

// Coverage answers "was this window looked at?" for one source.
type Coverage struct {
	Kind        CoverageKind
	WindowStart time.Time
	WindowEnd   time.Time
	// Gaps are the uncovered stretches, in order. Empty when Kind is
	// CoverageComplete.
	Gaps []Gap
}

// Complete reports whether the whole window was collected.
func (c Coverage) Complete() bool { return c.Kind == CoverageComplete }

// CoverageFor computes how much of [start, end] was covered by successful
// runs of a source.
//
// This is the function that keeps §74 honest. A caller reporting a count
// over a window MUST consult it, because "zero earthquakes" and "we never
// looked" are the same number of rows.
func CoverageFor(ctx context.Context, q Querier, sourceID int64, start, end time.Time) (Coverage, error) {
	if end.Before(start) {
		return Coverage{}, fmt.Errorf("ingestion: coverage window ends before it starts (%s..%s)", start, end)
	}
	start, end = start.UTC(), end.UTC()

	rows, err := q.Query(ctx, `
		SELECT window_start, window_end
		FROM ingestion_runs
		WHERE source_id = $1 AND result = 'success'
		  AND window_start <= $3 AND window_end >= $2
		ORDER BY window_start
	`, sourceID, start, end)
	if err != nil {
		return Coverage{}, fmt.Errorf("ingestion: coverage lookup failed: %w", err)
	}
	defer rows.Close()

	var covered []Gap
	for rows.Next() {
		var g Gap
		if err := rows.Scan(&g.Start, &g.End); err != nil {
			return Coverage{}, fmt.Errorf("ingestion: coverage scan failed: %w", err)
		}
		covered = append(covered, Gap{Start: g.Start.UTC(), End: g.End.UTC()})
	}
	if err := rows.Err(); err != nil {
		return Coverage{}, fmt.Errorf("ingestion: coverage iteration failed: %w", err)
	}

	cov := Coverage{WindowStart: start, WindowEnd: end}
	cov.Gaps = subtract(start, end, covered)

	switch {
	case len(cov.Gaps) == 0:
		cov.Kind = CoverageComplete
	case len(cov.Gaps) == 1 && cov.Gaps[0].Start.Equal(start) && cov.Gaps[0].End.Equal(end):
		cov.Kind = CoverageNone
	default:
		cov.Kind = CoveragePartial
	}
	return cov, nil
}

// subtract returns the parts of [start, end] not covered by any interval
// in covered.
func subtract(start, end time.Time, covered []Gap) []Gap {
	if len(covered) == 0 {
		return []Gap{{Start: start, End: end}}
	}

	merged := mergeIntervals(covered)

	var gaps []Gap
	cursor := start
	for _, iv := range merged {
		if iv.End.Before(cursor) || iv.End.Equal(cursor) {
			continue
		}
		if iv.Start.After(cursor) {
			gapEnd := iv.Start
			if gapEnd.After(end) {
				gapEnd = end
			}
			if gapEnd.After(cursor) {
				gaps = append(gaps, Gap{Start: cursor, End: gapEnd})
			}
		}
		if iv.End.After(cursor) {
			cursor = iv.End
		}
		if !cursor.Before(end) {
			return gaps
		}
	}
	if cursor.Before(end) {
		gaps = append(gaps, Gap{Start: cursor, End: end})
	}
	return gaps
}

func mergeIntervals(in []Gap) []Gap {
	sorted := append([]Gap(nil), in...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start.Before(sorted[j].Start) })

	out := []Gap{sorted[0]}
	for _, iv := range sorted[1:] {
		last := &out[len(out)-1]
		// Touching intervals count as contiguous: two runs that meet
		// exactly leave no instant uncollected between them.
		if !iv.Start.After(last.End) {
			if iv.End.After(last.End) {
				last.End = iv.End
			}
			continue
		}
		out = append(out, iv)
	}
	return out
}

// GapsSince finds stretches where a source went longer than its expected
// collection cadence without a successful run, from since until now.
//
// This is the §75 "data gaps" question, and it is deliberately answered
// from the runs rather than from the data: a quiet source and a broken
// source produce identical silence in the earthquakes table.
func GapsSince(ctx context.Context, q Querier, sourceID int64, since time.Time, cadence time.Duration) ([]Gap, error) {
	if cadence <= 0 {
		return nil, fmt.Errorf("ingestion: a gap is only meaningful relative to an expected cadence, got %s", cadence)
	}
	cov, err := CoverageFor(ctx, q, sourceID, since, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	var out []Gap
	for _, g := range cov.Gaps {
		if g.End.Sub(g.Start) > cadence {
			out = append(out, g)
		}
	}
	return out, nil
}

// Status is the operational state of one source's ingestion, as the
// registro-de-fontes spec requires the catalog to expose it.
type Status struct {
	SourceID int64
	// EverRun distinguishes "enabled and never collected" from "last
	// collection failed" — two states that must not look alike.
	EverRun          bool
	LastRun          *Run
	LastSuccess      *Run
	StaleFor         time.Duration
	HasEverSucceeded bool
}

// StatusFor reports the operational state of a source's ingestion.
func StatusFor(ctx context.Context, q Querier, sourceID int64) (Status, error) {
	st := Status{SourceID: sourceID}

	last, err := LastRun(ctx, q, sourceID)
	switch {
	case err == nil:
		st.EverRun = true
		st.LastRun = &last
	case isNoRuns(err):
		return st, nil
	default:
		return Status{}, err
	}

	success, err := LastSuccess(ctx, q, sourceID)
	switch {
	case err == nil:
		st.HasEverSucceeded = true
		st.LastSuccess = &success
		if success.FinishedAt != nil {
			st.StaleFor = time.Since(*success.FinishedAt)
		}
	case isNoRuns(err):
		// Ran, never succeeded. Staleness is measured from the first
		// attempt, because the source has never delivered anything.
		st.StaleFor = time.Since(last.StartedAt)
	default:
		return Status{}, err
	}

	return st, nil
}

func isNoRuns(err error) bool {
	return errors.Is(err, ErrNoRuns)
}
