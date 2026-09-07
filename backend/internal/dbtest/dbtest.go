// Package dbtest provides an ephemeral PostGIS instance for integration
// tests (design.md D10, tasks.md 7.1). It supports two backends, chosen by
// environment:
//
//   - TEST_DATABASE_URL set: connects to that already-running database
//     instead of starting a container. Intended for environments where
//     Docker-in-Docker is unavailable or too fragile (this repo's dev
//     container runs `go test` inside a container itself, so reaching the
//     host Docker daemon requires mounting the socket in).
//   - TEST_DATABASE_URL unset: starts a postgis/postgis:16-3.4 container
//     via testcontainers-go, requiring a reachable Docker daemon (mount
//     /var/run/docker.sock into whatever runs `go test`).
//
// Either way, callers get back a fresh, migrated database per test and
// tests using this package MUST be skipped under `go test -short`
// (call Skip(t) first) so the everyday local loop never needs Docker.
package dbtest

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/vilsonfr/volcanopredict/backend/internal/db"
)

// Skip marks t as skipped when running under `go test -short`, per the
// convention that integration tests never run in the fast local loop.
func Skip(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
}

// DB is a live, migrated database ready for a single test to use.
type DB struct {
	ConnString string
}

// Start provisions a database for the duration of the test, applies all
// embedded migrations, and registers cleanup. It calls Skip internally so
// every caller automatically respects -short.
func Start(t *testing.T) *DB {
	t.Helper()
	Skip(t)

	connString := os.Getenv("TEST_DATABASE_URL")
	if connString == "" {
		connString = startContainer(t)
	}

	if err := db.Migrate(connString); err != nil {
		t.Fatalf("dbtest: failed to migrate test database: %v", err)
	}

	return &DB{ConnString: connString}
}

// NewBareContainer starts a fresh postgis/postgis container (or connects
// to TEST_DATABASE_URL) WITHOUT applying any migration, for tests that
// need to control schema setup themselves — e.g. simulating a legacy
// volume by applying pre-goose SQL files directly before handing the
// database to db.Migrate. Skips under -short like Start does.
//
// When TEST_DATABASE_URL is set, callers are responsible for ensuring the
// target database starts empty; this function does not drop anything.
func NewBareContainer(t *testing.T) *DB {
	t.Helper()
	Skip(t)

	connString := os.Getenv("TEST_DATABASE_URL")
	if connString == "" {
		connString = startContainer(t)
	}
	return &DB{ConnString: connString}
}

func startContainer(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "postgis/postgis:16-3.4",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "volcanopredict_test",
			"POSTGRES_USER":     "volcano",
			"POSTGRES_PASSWORD": "volcano",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("dbtest: failed to start postgis container: %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("dbtest: failed to resolve container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("dbtest: failed to resolve mapped port: %v", err)
	}

	return fmt.Sprintf("postgres://volcano:volcano@%s:%s/volcanopredict_test?sslmode=disable", host, port.Port())
}
