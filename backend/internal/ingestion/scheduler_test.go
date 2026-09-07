package ingestion_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
	"github.com/vilsonfr/volcanopredict/backend/internal/ingestion"
)

// --- 8.3: non-overlap is enforced by the database, not by memory --------

func TestTryLock_SecondHolderIsRefusedWhileTheFirstHoldsIt(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	got, release, err := ingestion.TryLock(ctx, pool, "lock-test")
	if err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	if !got {
		t.Fatal("the first caller must get the lock")
	}

	// A second attempt on a DIFFERENT pooled connection must be refused.
	// An in-memory mutex would let this through in a second process.
	second, release2, err := ingestion.TryLock(ctx, pool, "lock-test")
	if err != nil {
		t.Fatalf("second TryLock: %v", err)
	}
	if second {
		release2()
		release()
		t.Fatal("a second holder must be refused while the lock is held")
	}

	release()

	// Once released, it is available again.
	third, release3, err := ingestion.TryLock(ctx, pool, "lock-test")
	if err != nil {
		t.Fatalf("third TryLock: %v", err)
	}
	if !third {
		t.Fatal("the lock must be available again after release")
	}
	release3()
}

func TestTryLock_DifferentNamesDoNotContend(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	a, releaseA, err := ingestion.TryLock(ctx, pool, "source-a")
	if err != nil || !a {
		t.Fatalf("first lock: got=%v err=%v", a, err)
	}
	defer releaseA()

	b, releaseB, err := ingestion.TryLock(ctx, pool, "source-b")
	if err != nil {
		t.Fatalf("second lock: %v", err)
	}
	if !b {
		t.Fatal("two different sources must not block each other")
	}
	releaseB()
}

// Two cycles racing for the same source: one runs, the other records that
// it did not.
func TestScheduler_ConcurrentCyclesForOneSourceDoNotOverlap(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	var mu sync.Mutex
	var ran int
	var skipped int

	inCycle := make(chan struct{})
	holdCycle := make(chan struct{})

	sched := &ingestion.Scheduler{
		Pool:     pool,
		Interval: time.Hour, // never fires on its own during this test
		Name:     "overlap-test",
		Cycle: func(ctx context.Context) error {
			mu.Lock()
			ran++
			first := ran == 1
			mu.Unlock()
			if first {
				close(inCycle)
				<-holdCycle
			}
			return nil
		},
		OnSkip: func(string) {
			mu.Lock()
			skipped++
			mu.Unlock()
		},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sched.RunOnce(ctx)
	}()

	<-inCycle // the first cycle is inside, holding the lock

	// The second cycle must find the lock taken and skip.
	sched.RunOnce(ctx)

	close(holdCycle)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if ran != 1 {
		t.Fatalf("exactly one cycle may run at a time, %d ran", ran)
	}
	if skipped != 1 {
		t.Fatalf("the blocked cycle must record that it did not run, got %d skips", skipped)
	}
}

// A failing cycle is logged and dropped: the scheduler must survive it, or
// a source being down would take the process with it.
func TestScheduler_CycleFailureDoesNotStopTheScheduler(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	ctx := context.Background()

	var calls int
	sched := &ingestion.Scheduler{
		Pool:     pool,
		Interval: time.Hour,
		Name:     "failure-test",
		Cycle: func(ctx context.Context) error {
			calls++
			return errors.New("source unreachable")
		},
	}

	sched.RunOnce(ctx)
	sched.RunOnce(ctx)

	if calls != 2 {
		t.Fatalf("a failed cycle must not prevent the next one, got %d calls", calls)
	}
}

func TestScheduler_ZeroIntervalDisablesIt(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)

	var called bool
	sched := &ingestion.Scheduler{
		Pool:     pool,
		Interval: 0,
		Name:     "disabled",
		Cycle: func(ctx context.Context) error {
			called = true
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	sched.Run(ctx) // must return immediately, not block until timeout

	if called {
		t.Fatal("a disabled scheduler must not run a cycle")
	}
}
