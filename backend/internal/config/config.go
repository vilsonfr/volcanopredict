// Package config reads and validates runtime configuration from environment
// variables. Per the ambiente-local spec, all runtime configuration SHALL
// come from environment variables, and a missing required variable SHALL
// cause the process to exit immediately with a message naming it — never a
// silent default.
package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds all runtime configuration read from the environment.
type Config struct {
	// DatabaseURL is the pgx-compatible connection string. Required.
	DatabaseURL string
	// DBConnectTimeout bounds how long the backend waits for the database
	// to accept connections on startup. Required, must parse as a Go
	// duration.
	DBConnectTimeout time.Duration
	// HTTPAddr is the listen address for the HTTP server. Optional;
	// defaults to ":8080" when empty, matching the documented behavior in
	// .env.example.
	HTTPAddr string
	// LogLevel is the minimum log level: debug, info, warn, error.
	// Optional; defaults to "info".
	LogLevel string
	// IngestInterval is how often the backend collects from enabled
	// sources. Optional; defaults to 15 minutes. Zero disables automatic
	// ingestion entirely, which is a supported configuration — a deployment
	// may prefer to drive it from outside.
	IngestInterval time.Duration
	// IngestBackfillOnFirstRun is how far back the very first incremental
	// cycle reaches when no successful run exists yet. Optional; defaults
	// to 24 hours, deliberately short: a large first load is a decision for
	// the `ingest -backfill` subcommand, not a side effect of booting.
	IngestBackfillOnFirstRun time.Duration
}

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// Load reads configuration from the process environment and validates it.
// It returns a descriptive error naming the offending variable instead of
// silently substituting a default for anything required.
func Load() (Config, error) {
	return load(os.LookupEnv)
}

// load is the testable core of Load, parameterized over the environment
// lookup function so unit tests do not need to mutate process-global state.
func load(lookup func(string) (string, bool)) (Config, error) {
	cfg := Config{}

	dbURL, ok := lookup("DATABASE_URL")
	if !ok || dbURL == "" {
		return Config{}, fmt.Errorf("config: required environment variable DATABASE_URL is not set")
	}
	cfg.DatabaseURL = dbURL

	timeoutRaw, ok := lookup("DB_CONNECT_TIMEOUT")
	if !ok || timeoutRaw == "" {
		return Config{}, fmt.Errorf("config: required environment variable DB_CONNECT_TIMEOUT is not set")
	}
	timeout, err := time.ParseDuration(timeoutRaw)
	if err != nil {
		return Config{}, fmt.Errorf("config: environment variable DB_CONNECT_TIMEOUT has invalid duration %q: %w", timeoutRaw, err)
	}
	if timeout <= 0 {
		return Config{}, fmt.Errorf("config: environment variable DB_CONNECT_TIMEOUT must be positive, got %q", timeoutRaw)
	}
	cfg.DBConnectTimeout = timeout

	addr, ok := lookup("HTTP_ADDR")
	if !ok || addr == "" {
		addr = ":8080"
	}
	cfg.HTTPAddr = addr

	level, ok := lookup("LOG_LEVEL")
	if !ok || level == "" {
		level = "info"
	}
	if !validLogLevels[level] {
		return Config{}, fmt.Errorf("config: environment variable LOG_LEVEL has invalid value %q, expected one of debug|info|warn|error", level)
	}
	cfg.LogLevel = level

	interval, err := optionalDuration(lookup, "INGEST_INTERVAL", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	if interval < 0 {
		return Config{}, fmt.Errorf("config: environment variable INGEST_INTERVAL must not be negative, got %s", interval)
	}
	cfg.IngestInterval = interval

	first, err := optionalDuration(lookup, "INGEST_FIRST_RUN_WINDOW", 24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	if first <= 0 {
		return Config{}, fmt.Errorf("config: environment variable INGEST_FIRST_RUN_WINDOW must be positive, got %s", first)
	}
	cfg.IngestBackfillOnFirstRun = first

	return cfg, nil
}

// optionalDuration reads a duration variable that has a default, still
// failing loudly on a value that is present but unparseable — a typo in a
// duration must not silently fall back to the default.
func optionalDuration(lookup func(string) (string, bool), name string, def time.Duration) (time.Duration, error) {
	raw, ok := lookup(name)
	if !ok || raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("config: environment variable %s has invalid duration %q: %w", name, raw, err)
	}
	return d, nil
}
