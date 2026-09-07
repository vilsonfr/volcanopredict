package config

import (
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}

func TestLoad_HappyPath(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"DATABASE_URL":       "postgres://u:p@localhost:5432/db?sslmode=disable",
		"DB_CONNECT_TIMEOUT": "30s",
		"HTTP_ADDR":          ":9090",
		"LOG_LEVEL":          "debug",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://u:p@localhost:5432/db?sslmode=disable" {
		t.Errorf("unexpected DatabaseURL: %s", cfg.DatabaseURL)
	}
	if cfg.DBConnectTimeout != 30*time.Second {
		t.Errorf("unexpected DBConnectTimeout: %s", cfg.DBConnectTimeout)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("unexpected HTTPAddr: %s", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("unexpected LogLevel: %s", cfg.LogLevel)
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"DATABASE_URL":       "postgres://u:p@localhost:5432/db",
		"DB_CONNECT_TIMEOUT": "5s",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("expected default HTTPAddr :8080, got %s", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default LogLevel info, got %s", cfg.LogLevel)
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	_, err := load(env(map[string]string{
		"DB_CONNECT_TIMEOUT": "5s",
	}))
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error should name the missing variable, got: %v", err)
	}
}

func TestLoad_MissingConnectTimeout(t *testing.T) {
	_, err := load(env(map[string]string{
		"DATABASE_URL": "postgres://u:p@localhost:5432/db",
	}))
	if err == nil {
		t.Fatal("expected error for missing DB_CONNECT_TIMEOUT")
	}
	if !strings.Contains(err.Error(), "DB_CONNECT_TIMEOUT") {
		t.Errorf("error should name the missing variable, got: %v", err)
	}
}

func TestLoad_InvalidConnectTimeout(t *testing.T) {
	_, err := load(env(map[string]string{
		"DATABASE_URL":       "postgres://u:p@localhost:5432/db",
		"DB_CONNECT_TIMEOUT": "not-a-duration",
	}))
	if err == nil {
		t.Fatal("expected error for invalid DB_CONNECT_TIMEOUT")
	}
	if !strings.Contains(err.Error(), "DB_CONNECT_TIMEOUT") {
		t.Errorf("error should name the offending variable, got: %v", err)
	}
}

func TestLoad_NonPositiveConnectTimeout(t *testing.T) {
	_, err := load(env(map[string]string{
		"DATABASE_URL":       "postgres://u:p@localhost:5432/db",
		"DB_CONNECT_TIMEOUT": "0s",
	}))
	if err == nil {
		t.Fatal("expected error for zero DB_CONNECT_TIMEOUT")
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	_, err := load(env(map[string]string{
		"DATABASE_URL":       "postgres://u:p@localhost:5432/db",
		"DB_CONNECT_TIMEOUT": "5s",
		"LOG_LEVEL":          "verbose",
	}))
	if err == nil {
		t.Fatal("expected error for invalid LOG_LEVEL")
	}
	if !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Errorf("error should name the offending variable, got: %v", err)
	}
}
