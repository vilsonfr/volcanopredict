package db

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestWaitReady_Unreachable exercises the "banco inalcançável" scenario
// from the ambiente-local spec without needing Docker: it points at a
// host that refuses connections and expects a prompt, named failure
// instead of hanging until some external timeout.
func TestWaitReady_Unreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Port 1 is reserved and nothing should ever be listening there.
	connString := "postgres://user:pass@127.0.0.1:1/db?sslmode=disable"
	start := time.Now()
	err := WaitReady(ctx, connString, 1*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error when database is unreachable")
	}
	if !strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("error should name the target host, got: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("WaitReady should respect the configured timeout, took %s", elapsed)
	}
}

func TestRedactConnString(t *testing.T) {
	in := "postgres://volcano:secret@db:5432/volcanopredict?sslmode=disable"
	out := redactConnString(in)
	if strings.Contains(out, "secret") {
		t.Errorf("expected credentials to be redacted, got: %s", out)
	}
	if !strings.Contains(out, "db:5432") {
		t.Errorf("expected host to remain visible, got: %s", out)
	}
}
