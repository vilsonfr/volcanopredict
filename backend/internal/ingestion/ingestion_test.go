package ingestion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
	"github.com/vilsonfr/volcanopredict/backend/internal/ingestion"
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
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO data_sources (name, category, enabled, license, attribution)
		VALUES ($1, 'test', true, 'test-license', 'Test') RETURNING id
	`, name).Scan(&id); err != nil {
		t.Fatalf("seeding source: %v", err)
	}
	return id
}

// success records a completed successful run covering [start, end].
func success(t *testing.T, pool *pgxpool.Pool, srcID int64, start, end time.Time) ingestion.Run {
	t.Helper()
	ctx := context.Background()
	run, err := ingestion.Begin(ctx, pool, srcID, ingestion.ModeIncremental, start, end)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	done, err := ingestion.Finish(ctx, pool, run.ID, ingestion.Counts{Inserted: 1})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	return done
}

// --- 7.1: every run is recorded, failures included ----------------------

func TestRun_FailureIsRecordedAndDoesNotOverwriteThePrevious(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "runs-failure")
	ctx := context.Background()
	now := time.Now().UTC()

	good := success(t, pool, srcID, now.Add(-2*time.Hour), now.Add(-time.Hour))

	bad, err := ingestion.Begin(ctx, pool, srcID, ingestion.ModeIncremental, now.Add(-time.Hour), now)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	failed, err := ingestion.Fail(ctx, pool, bad.ID, errors.New("source unreachable"), ingestion.Counts{})
	if err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if failed.Result != ingestion.ResultFailure || failed.ErrorMsg == "" {
		t.Fatalf("a failed run must record the cause, got %+v", failed)
	}

	// The earlier success must still be there, untouched.
	stillGood, err := ingestion.LastSuccess(ctx, pool, srcID)
	if err != nil {
		t.Fatalf("LastSuccess: %v", err)
	}
	if stillGood.ID != good.ID {
		t.Fatalf("the failure overwrote the evidence of the earlier success: %d vs %d", stillGood.ID, good.ID)
	}

	last, err := ingestion.LastRun(ctx, pool, srcID)
	if err != nil {
		t.Fatalf("LastRun: %v", err)
	}
	if last.ID != failed.ID {
		t.Fatalf("the most recent run must be the failure, got %d", last.ID)
	}
}

// A process killed mid-ingestion must leave a visibly unfinished run, not
// nothing at all.
func TestRun_InterruptedRunStaysVisiblyUnfinished(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "runs-interrupted")
	ctx := context.Background()
	now := time.Now().UTC()

	run, err := ingestion.Begin(ctx, pool, srcID, ingestion.ModeBackfill, now.Add(-time.Hour), now)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if run.Result != ingestion.ResultRunning || run.FinishedAt != nil {
		t.Fatalf("a started run must be open, got %+v", run)
	}

	last, err := ingestion.LastRun(ctx, pool, srcID)
	if err != nil {
		t.Fatalf("LastRun: %v", err)
	}
	if last.Result != ingestion.ResultRunning {
		t.Fatalf("an interrupted run must remain discoverable as running, got %s", last.Result)
	}
	// And it must not count as coverage.
	cov, err := ingestion.CoverageFor(ctx, pool, srcID, now.Add(-time.Hour), now)
	if err != nil {
		t.Fatalf("CoverageFor: %v", err)
	}
	if cov.Kind != ingestion.CoverageNone {
		t.Fatalf("an unfinished run must not count as coverage, got %s", cov.Kind)
	}
}

// --- 7.2: coverage comes from the runs, not from the data ---------------

// The exact §74 case: the earthquakes table is empty either way, so the
// answer must come from somewhere else.
func TestCoverage_UncollectedWindowIsNotSilence(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "coverage-none")
	ctx := context.Background()
	now := time.Now().UTC()

	cov, err := ingestion.CoverageFor(ctx, pool, srcID, now.Add(-24*time.Hour), now)
	if err != nil {
		t.Fatalf("CoverageFor: %v", err)
	}
	if cov.Kind != ingestion.CoverageNone {
		t.Fatalf("a window never collected must report no coverage, got %s", cov.Kind)
	}
	if cov.Complete() {
		t.Fatal("a window never collected must not report complete coverage")
	}
	if len(cov.Gaps) != 1 {
		t.Fatalf("expected the whole window reported as one gap, got %v", cov.Gaps)
	}
}

func TestCoverage_FullyCollectedWindowIsComplete(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "coverage-complete")
	ctx := context.Background()
	now := time.Now().UTC()

	success(t, pool, srcID, now.Add(-10*time.Hour), now)

	cov, err := ingestion.CoverageFor(ctx, pool, srcID, now.Add(-5*time.Hour), now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("CoverageFor: %v", err)
	}
	if !cov.Complete() {
		t.Fatalf("a window inside a successful run must be complete, got %s with gaps %v", cov.Kind, cov.Gaps)
	}
	if len(cov.Gaps) != 0 {
		t.Fatalf("a complete window has no gaps, got %v", cov.Gaps)
	}
}

func TestCoverage_PartialWindowNamesTheUncoveredStretch(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "coverage-partial")
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// Collected 10h..6h ago and 3h..now. The 6h..3h stretch was never
	// collected — the ingestion was down.
	success(t, pool, srcID, now.Add(-10*time.Hour), now.Add(-6*time.Hour))
	success(t, pool, srcID, now.Add(-3*time.Hour), now)

	cov, err := ingestion.CoverageFor(ctx, pool, srcID, now.Add(-10*time.Hour), now)
	if err != nil {
		t.Fatalf("CoverageFor: %v", err)
	}
	if cov.Kind != ingestion.CoveragePartial {
		t.Fatalf("expected partial coverage, got %s", cov.Kind)
	}
	if len(cov.Gaps) != 1 {
		t.Fatalf("expected exactly one gap, got %v", cov.Gaps)
	}
	g := cov.Gaps[0]
	if !g.Start.Equal(now.Add(-6*time.Hour)) || !g.End.Equal(now.Add(-3*time.Hour)) {
		t.Fatalf("gap must be exactly the uncollected stretch 6h..3h ago, got %s..%s", g.Start, g.End)
	}
}

// Two runs that meet exactly leave nothing uncollected between them.
func TestCoverage_TouchingRunsLeaveNoGap(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "coverage-touching")
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	mid := now.Add(-5 * time.Hour)

	success(t, pool, srcID, now.Add(-10*time.Hour), mid)
	success(t, pool, srcID, mid, now)

	cov, err := ingestion.CoverageFor(ctx, pool, srcID, now.Add(-10*time.Hour), now)
	if err != nil {
		t.Fatalf("CoverageFor: %v", err)
	}
	if !cov.Complete() {
		t.Fatalf("contiguous runs must cover the whole window, got %s with gaps %v", cov.Kind, cov.Gaps)
	}
}

// --- 7.3: gaps relative to the expected cadence -------------------------

func TestGapsSince_FindsExactlyTheExpectedGap(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "gaps")
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	success(t, pool, srcID, now.Add(-12*time.Hour), now.Add(-8*time.Hour))
	// 8h..2h ago: nothing. The source was down for six hours.
	success(t, pool, srcID, now.Add(-2*time.Hour), now)

	gaps, err := ingestion.GapsSince(ctx, pool, srcID, now.Add(-12*time.Hour), time.Hour)
	if err != nil {
		t.Fatalf("GapsSince: %v", err)
	}
	if len(gaps) != 1 {
		t.Fatalf("expected one gap longer than the cadence, got %v", gaps)
	}
	if !gaps[0].Start.Equal(now.Add(-8*time.Hour)) || !gaps[0].End.Equal(now.Add(-2*time.Hour)) {
		t.Fatalf("gap boundaries wrong: got %s..%s", gaps[0].Start, gaps[0].End)
	}

	// With a cadence wider than the outage, the same stretch is not a gap.
	none, err := ingestion.GapsSince(ctx, pool, srcID, now.Add(-12*time.Hour), 24*time.Hour)
	if err != nil {
		t.Fatalf("GapsSince: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("a gap is only meaningful relative to the cadence, got %v", none)
	}
}

// --- 7.4: operational status --------------------------------------------

func TestStatus_NeverCollectedIsDistinctFromLastCollectionFailed(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()
	now := time.Now().UTC()

	neverID := mustSource(t, pool, "status-never")
	never, err := ingestion.StatusFor(ctx, pool, neverID)
	if err != nil {
		t.Fatalf("StatusFor: %v", err)
	}
	if never.EverRun {
		t.Fatal("a source that was never collected must report so")
	}
	if never.LastRun != nil || never.HasEverSucceeded {
		t.Fatalf("expected an empty status, got %+v", never)
	}

	failedID := mustSource(t, pool, "status-failed")
	run, err := ingestion.Begin(ctx, pool, failedID, ingestion.ModeIncremental, now.Add(-time.Hour), now)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := ingestion.Fail(ctx, pool, run.ID, errors.New("boom"), ingestion.Counts{}); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	failed, err := ingestion.StatusFor(ctx, pool, failedID)
	if err != nil {
		t.Fatalf("StatusFor: %v", err)
	}
	if !failed.EverRun {
		t.Fatal("a source whose collection failed HAS run: that is the whole distinction")
	}
	if failed.HasEverSucceeded {
		t.Fatal("this source never succeeded")
	}
	if failed.LastRun == nil || failed.LastRun.Result != ingestion.ResultFailure {
		t.Fatalf("expected the failure to be the last run, got %+v", failed.LastRun)
	}
	if failed.LastRun.ErrorMsg == "" {
		t.Fatal("the status must carry why the last collection failed")
	}
}

func TestStatus_ReportsLastSuccessAfterALaterFailure(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "status-mixed")
	ctx := context.Background()
	now := time.Now().UTC()

	good := success(t, pool, srcID, now.Add(-3*time.Hour), now.Add(-2*time.Hour))
	run, err := ingestion.Begin(ctx, pool, srcID, ingestion.ModeIncremental, now.Add(-time.Hour), now)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := ingestion.Fail(ctx, pool, run.ID, errors.New("source unreachable"), ingestion.Counts{}); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	st, err := ingestion.StatusFor(ctx, pool, srcID)
	if err != nil {
		t.Fatalf("StatusFor: %v", err)
	}
	if st.LastRun == nil || st.LastRun.Result != ingestion.ResultFailure {
		t.Fatalf("last run must be the failure, got %+v", st.LastRun)
	}
	if !st.HasEverSucceeded || st.LastSuccess == nil || st.LastSuccess.ID != good.ID {
		t.Fatalf("the status must still point at the last successful collection, got %+v", st.LastSuccess)
	}
	if st.StaleFor <= 0 {
		t.Fatalf("staleness must be measured from the last success, got %s", st.StaleFor)
	}
}

func TestSkip_RecordsThatACycleDeliberatelyDidNotRun(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "runs-skipped")
	ctx := context.Background()
	now := time.Now().UTC()

	sk, err := ingestion.Skip(ctx, pool, srcID, ingestion.ModeIncremental, now.Add(-time.Hour), now, "previous cycle still running")
	if err != nil {
		t.Fatalf("Skip: %v", err)
	}
	if sk.Result != ingestion.ResultSkipped {
		t.Fatalf("expected a skipped run, got %s", sk.Result)
	}
	// A skipped cycle collected nothing, so it must not claim coverage.
	cov, err := ingestion.CoverageFor(ctx, pool, srcID, now.Add(-time.Hour), now)
	if err != nil {
		t.Fatalf("CoverageFor: %v", err)
	}
	if cov.Kind != ingestion.CoverageNone {
		t.Fatalf("a skipped cycle must not count as coverage, got %s", cov.Kind)
	}
}
