package ingestion

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Cycle is one unit of scheduled work: whatever an adapter does when the
// timer fires.
type Cycle func(ctx context.Context) error

// Scheduler runs a Cycle on an interval, in-process.
//
// It is deliberately a thin shell (design.md D7): it owns the timer and
// the lock, and nothing else. All ingestion logic lives in the adapter,
// which the subcommand calls directly — so the test suite exercises
// ingestion without ever waiting on a clock.
type Scheduler struct {
	Pool     *pgxpool.Pool
	Interval time.Duration
	// Name identifies this scheduler's lock. Two processes using the same
	// name contend for the same advisory lock.
	Name  string
	Cycle Cycle

	// OnSkip is called when a cycle is skipped because another holds the
	// lock. Optional.
	OnSkip func(reason string)
}

// Run drives the loop until ctx is cancelled.
//
// It never returns an error for a failed cycle: a source being down must
// not take the process with it (ingestao-fontes spec, design.md D8). The
// failure is the adapter's to record; the scheduler's job is to be there
// for the next tick.
func (s *Scheduler) Run(ctx context.Context) {
	if s.Interval <= 0 {
		log.Printf("ingestion: scheduler %q disabled (interval %s)", s.Name, s.Interval)
		return
	}

	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()

	log.Printf("ingestion: scheduler %q started, every %s", s.Name, s.Interval)
	for {
		select {
		case <-ctx.Done():
			log.Printf("ingestion: scheduler %q stopped", s.Name)
			return
		case <-ticker.C:
			s.RunOnce(ctx)
		}
	}
}

// RunOnce executes exactly one cycle, taking the lock first. It is
// exported so a test can drive the cycle without waiting for a tick.
func (s *Scheduler) RunOnce(ctx context.Context) {
	acquired, release, err := TryLock(ctx, s.Pool, s.Name)
	if err != nil {
		log.Printf("ingestion: scheduler %q could not check the lock: %v", s.Name, err)
		return
	}
	if !acquired {
		reason := "previous cycle still running"
		log.Printf("ingestion: scheduler %q skipped: %s", s.Name, reason)
		if s.OnSkip != nil {
			s.OnSkip(reason)
		}
		return
	}
	defer release()

	if err := s.Cycle(ctx); err != nil {
		// Logged, never propagated: the HTTP service keeps serving.
		log.Printf("ingestion: scheduler %q cycle failed: %v", s.Name, err)
	}
}

// TryLock takes a Postgres advisory lock for name without blocking.
//
// An in-memory mutex would be enough for one process and would fail
// silently the moment a second replica existed — each would think it held
// the lock. The advisory lock is correct in both cases and costs no new
// infrastructure; it is the same mechanism the migrations already use.
//
// The returned release function is safe to call once.
func TryLock(ctx context.Context, pool *pgxpool.Pool, name string) (bool, func(), error) {
	key := lockKey(name)

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("ingestion: acquiring a connection for the lock failed: %w", err)
	}

	var acquired bool
	// The lock is held by the SESSION, so it must be taken and released on
	// the same connection — hence acquiring one explicitly rather than
	// going through the pool for each statement.
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&acquired); err != nil {
		conn.Release()
		return false, nil, fmt.Errorf("ingestion: advisory lock attempt failed: %w", err)
	}
	if !acquired {
		conn.Release()
		return false, nil, nil
	}

	released := false
	return true, func() {
		if released {
			return
		}
		released = true
		// Best effort on a fresh context: the caller's may already be
		// cancelled, and failing to unlock would strand the lock until the
		// connection closed.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, key); err != nil {
			log.Printf("ingestion: releasing advisory lock %q failed: %v", name, err)
		}
		conn.Release()
	}, nil
}

// lockKey turns a name into the int64 advisory-lock key space.
func lockKey(name string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("volcanopredict/ingestion/" + name))
	return int64(h.Sum64())
}
