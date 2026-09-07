// Package observation is the sole place in the codebase allowed to write
// temporal SQL predicates over observed_at/ingested_at/as-of (design.md
// D7). Every other package that needs "what did we know at time T" calls
// into this package instead of building its own WHERE clause — that is
// what keeps the data-leakage guarantee auditable at a single point.
package observation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Querier is the subset of pgxpool.Pool (or pgx.Tx) this package needs,
// so callers can pass either a pool or a transaction.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Provenance selects which mix of real/synthetic data a query returns.
// There is deliberately no zero-value that silently means "both": callers
// must say so explicitly (spec's "agregação não mistura proveniências
// silenciosamente").
type Provenance int

const (
	// ProvenanceUnspecified is the zero value and is rejected by List, so
	// a caller that forgets to choose gets an error instead of an
	// implicit "both" that could mix real and synthetic data silently.
	ProvenanceUnspecified Provenance = iota
	ProvenanceRealOnly
	ProvenanceSyntheticOnly
	ProvenanceAll
)

// ErrFutureAsOf is returned when a caller asks for an as-of instant that
// is later than the current instant (spec: "Consulta as-of no futuro é
// rejeitada").
var ErrFutureAsOf = errors.New("observation: as-of instant is in the future")

// ErrProvenanceUnspecified is returned when List is called without an
// explicit Provenance filter.
var ErrProvenanceUnspecified = errors.New("observation: provenance filter must be specified explicitly")

// Observation is a single bitemporal observation row.
type Observation struct {
	ID          int64
	VolcanoID   int64
	Kind        string
	ObservedAt  time.Time
	IngestedAt  time.Time
	Value       json.RawMessage
	IsSynthetic bool
	SourceID    int64
}

// Query describes a temporal read. AsOf, when nil, means "current
// knowledge" (spec: "ausência de instante retorna o estado atual").
type Query struct {
	VolcanoID  *int64
	Kind       *string
	ObservedFrom *time.Time
	ObservedTo   *time.Time
	AsOf       *time.Time
	Provenance Provenance
}

// NewObservation is the shape accepted for a write. IngestedAt is
// deliberately absent from this type: the database assigns it (schema
// DEFAULT plus a BEFORE INSERT trigger, migrations 004/009), and this
// package's Insert never sends a caller-supplied value for that column,
// so there is nothing here for a caller to smuggle in (spec: "tempo de
// conhecimento não é fornecido pelo cliente").
type NewObservation struct {
	VolcanoID   int64
	Kind        string
	ObservedAt  time.Time
	Value       json.RawMessage
	IsSynthetic bool
	SourceID    int64
}

// List returns observations matching q. With q.AsOf set, it returns
// exactly the current-version row per natural key
// (volcano_id, kind, observed_at, source_id) as known at that instant —
// i.e. it never returns a row whose ingested_at is later than AsOf, and
// never returns more than one row per natural key (spec: leitura corrente
// escolhe a versão de maior ingested_at sem duplicar linhas).
//
// AsOf is always interpreted in UTC. A nil AsOf means "now": the
// equivalent of an as-of query at the current instant.
func List(ctx context.Context, q Querier, query Query) ([]Observation, error) {
	if query.Provenance == ProvenanceUnspecified {
		return nil, ErrProvenanceUnspecified
	}

	now := time.Now().UTC()
	asOf := now
	if query.AsOf != nil {
		asOf = query.AsOf.UTC()
		if asOf.After(now) {
			return nil, fmt.Errorf("%w: requested %s, now is %s", ErrFutureAsOf, asOf, now)
		}
	}

	sql := `
		SELECT id, volcano_id, kind, observed_at, ingested_at, value, is_synthetic, source_id
		FROM (
			SELECT DISTINCT ON (volcano_id, kind, observed_at, source_id)
				id, volcano_id, kind, observed_at, ingested_at, value, is_synthetic, source_id
			FROM observations
			WHERE ingested_at <= $1
	`
	args := []any{asOf}

	if query.VolcanoID != nil {
		args = append(args, *query.VolcanoID)
		sql += fmt.Sprintf(" AND volcano_id = $%d", len(args))
	}
	if query.Kind != nil {
		args = append(args, *query.Kind)
		sql += fmt.Sprintf(" AND kind = $%d", len(args))
	}
	if query.ObservedFrom != nil {
		args = append(args, query.ObservedFrom.UTC())
		sql += fmt.Sprintf(" AND observed_at >= $%d", len(args))
	}
	if query.ObservedTo != nil {
		args = append(args, query.ObservedTo.UTC())
		sql += fmt.Sprintf(" AND observed_at <= $%d", len(args))
	}
	switch query.Provenance {
	case ProvenanceRealOnly:
		sql += " AND is_synthetic = false"
	case ProvenanceSyntheticOnly:
		sql += " AND is_synthetic = true"
	case ProvenanceAll:
		// no filter: both provenances included, explicitly requested.
	}

	sql += `
			ORDER BY volcano_id, kind, observed_at, source_id, ingested_at DESC
		) current_versions
		ORDER BY observed_at ASC, id ASC
	`

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("observation: list query failed: %w", err)
	}
	defer rows.Close()

	var out []Observation
	for rows.Next() {
		var o Observation
		if err := rows.Scan(&o.ID, &o.VolcanoID, &o.Kind, &o.ObservedAt, &o.IngestedAt, &o.Value, &o.IsSynthetic, &o.SourceID); err != nil {
			return nil, fmt.Errorf("observation: scan failed: %w", err)
		}
		o.ObservedAt = o.ObservedAt.UTC()
		o.IngestedAt = o.IngestedAt.UTC()
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("observation: row iteration failed: %w", err)
	}
	return out, nil
}

// Insert persists a new observation. ingested_at is never part of the
// column list here: the database's DEFAULT now() and its BEFORE INSERT
// trigger (migration 009) are what actually enforce that the caller
// cannot control this value, and by never accepting it in NewObservation
// or sending it in this statement, the Go layer cannot reintroduce that
// possibility even if a future refactor adds a field for it.
func Insert(ctx context.Context, q Querier, obs NewObservation) (Observation, error) {
	var out Observation
	err := q.QueryRow(ctx, `
		INSERT INTO observations (volcano_id, kind, observed_at, value, is_synthetic, source_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, volcano_id, kind, observed_at, ingested_at, value, is_synthetic, source_id
	`, obs.VolcanoID, obs.Kind, obs.ObservedAt.UTC(), obs.Value, obs.IsSynthetic, obs.SourceID).
		Scan(&out.ID, &out.VolcanoID, &out.Kind, &out.ObservedAt, &out.IngestedAt, &out.Value, &out.IsSynthetic, &out.SourceID)
	if err != nil {
		return Observation{}, fmt.Errorf("observation: insert failed: %w", err)
	}
	out.ObservedAt = out.ObservedAt.UTC()
	out.IngestedAt = out.IngestedAt.UTC()
	return out, nil
}
