// Package source is the registry of external data sources
// (registro-de-fontes spec). It is the single place that reads
// data_sources, so any package that ingests external data resolves its
// source through here instead of querying data_sources directly — that is
// what makes "toda fonte externa é registrada antes de ser usada"
// enforceable at one point instead of by convention.
package source

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Querier is the subset of pgxpool.Pool (or pgx.Tx) this package needs.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Source is a registered external data source row.
type Source struct {
	ID          int64
	Name        string
	BaseURL     string
	Category    string
	Enabled     bool
	License     string
	Attribution string
}

// ErrNotFound is returned when a source name has no matching row in
// data_sources (spec: "ingestão de fonte não registrada é rejeitada").
var ErrNotFound = errors.New("source: not registered")

// ErrDisabled is returned when a caller resolves a source that exists but
// is not enabled for ingestion (spec: "fonte registrada mas não
// habilitada... nenhuma ingestão a partir dela é permitida").
var ErrDisabled = errors.New("source: registered but not enabled for ingestion")

// GetByName resolves a source by its exact name in data_sources. Callers
// that are about to ingest data MUST call this (or GetEnabledByName) and
// fail the ingestion if it errors, rather than assuming a source_id.
func GetByName(ctx context.Context, q Querier, name string) (Source, error) {
	var s Source
	err := q.QueryRow(ctx, `
		SELECT id, name, COALESCE(base_url, ''), category, enabled,
		       COALESCE(license, ''), COALESCE(attribution, '')
		FROM data_sources
		WHERE name = $1
	`, name).Scan(&s.ID, &s.Name, &s.BaseURL, &s.Category, &s.Enabled, &s.License, &s.Attribution)
	if errors.Is(err, pgx.ErrNoRows) {
		return Source{}, fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	if err != nil {
		return Source{}, fmt.Errorf("source: lookup failed: %w", err)
	}
	return s, nil
}

// GetEnabledByName resolves a source by name and additionally rejects it
// when it is not enabled for ingestion, so an importer that calls this
// cannot proceed against a source that is registered for documentation
// purposes only.
func GetEnabledByName(ctx context.Context, q Querier, name string) (Source, error) {
	s, err := GetByName(ctx, q, name)
	if err != nil {
		return Source{}, err
	}
	if !s.Enabled {
		return Source{}, fmt.Errorf("%w: %q", ErrDisabled, name)
	}
	return s, nil
}

// List returns every registered source, ordered by name.
//
// The catalog is public on purpose: the api-publica spec requires that a
// consumer be able to reach the attribution a source demands, and the
// registro-de-fontes spec requires the catalog itself to be auditable —
// including sources that are registered but not yet enabled for ingestion.
func List(ctx context.Context, q Querier) ([]Source, error) {
	rows, err := q.Query(ctx, `
		SELECT id, name, coalesce(base_url, ''), category, enabled,
		       coalesce(license, ''), coalesce(attribution, '')
		FROM data_sources
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("source: list: %w", err)
	}
	defer rows.Close()

	var out []Source
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.ID, &s.Name, &s.BaseURL, &s.Category,
			&s.Enabled, &s.License, &s.Attribution); err != nil {
			return nil, fmt.Errorf("source: list: scan: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("source: list: %w", err)
	}
	return out, nil
}
