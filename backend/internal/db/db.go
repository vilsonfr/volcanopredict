// Package db owns the connection pool, startup readiness wait, and
// embedded schema migrations (design.md D3, D7). Nothing outside this
// package should construct its own pgx pool or invoke goose directly.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/vilsonfr/volcanopredict/backend/migrations"
)

// WaitReady blocks until the database at connString accepts connections,
// or until timeout elapses. It logs while waiting, per the ambiente-local
// spec's "banco ainda inicializando" scenario, and returns an error naming
// the failure cause when the timeout is exceeded.
func WaitReady(ctx context.Context, connString string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	logged := false
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		conn, err := pgxpool.New(attemptCtx, connString)
		if err == nil {
			pingErr := conn.Ping(attemptCtx)
			conn.Close()
			if pingErr == nil {
				cancel()
				return nil
			}
			lastErr = pingErr
		} else {
			lastErr = err
		}
		cancel()

		if time.Now().After(deadline) {
			return fmt.Errorf("db: database not reachable after %s (target=%q): %w", timeout, redactConnString(connString), lastErr)
		}
		if !logged {
			log.Printf("db: waiting for database to accept connections (target=%s)...", redactConnString(connString))
			logged = true
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("db: context cancelled while waiting for database (target=%q): %w", redactConnString(connString), ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// NewPool opens a pgx connection pool. Callers are expected to have called
// WaitReady first so this does not need its own retry loop.
func NewPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("db: failed to create connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: failed to ping database: %w", err)
	}
	return pool, nil
}

// Migrate applies all pending embedded migrations using goose as a library
// (design.md D3). It uses goose's own advisory lock, so concurrent starts
// of multiple replicas serialize safely instead of racing.
//
// connString must be a database/sql-compatible DSN (goose drives
// database/sql, not pgx directly).
func Migrate(connString string) error {
	sqlDB, err := sql.Open("pgx", connString)
	if err != nil {
		return fmt.Errorf("db: failed to open sql.DB for migrations: %w", err)
	}
	defer sqlDB.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("db: failed to set goose dialect: %w", err)
	}

	if err := goose.Up(sqlDB, "."); err != nil {
		return fmt.Errorf("db: migration failed: %w", err)
	}
	return nil
}

// SchemaVersion reports the current applied migration version, used by the
// health endpoint to confirm the running binary's expected schema is
// actually present.
func SchemaVersion(connString string) (int64, error) {
	sqlDB, err := sql.Open("pgx", connString)
	if err != nil {
		return 0, fmt.Errorf("db: failed to open sql.DB: %w", err)
	}
	defer sqlDB.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return 0, err
	}
	return goose.GetDBVersion(sqlDB)
}

// redactConnString strips credentials from a connection string before it
// reaches a log line.
func redactConnString(connString string) string {
	// Minimal redaction: connection strings here are either postgres://
	// URLs (userinfo between "://" and "@") or DSN key=value pairs. This
	// backend only ever logs the URL form, so handle that case and fall
	// back to returning the input unchanged if it doesn't match.
	const scheme = "://"
	i := indexOf(connString, scheme)
	if i < 0 {
		return connString
	}
	at := indexOf(connString[i+len(scheme):], "@")
	if at < 0 {
		return connString
	}
	return connString[:i+len(scheme)] + "***@" + connString[i+len(scheme)+at+1:]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
