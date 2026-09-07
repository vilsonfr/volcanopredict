// Package volcano is the domain and repository for the volcano catalog
// (catalogo-vulcoes spec, design.md D7). It owns identity across
// reimports: a volcano's stable identifier is derived from
// (source_id, source_ref), and this package is the only place that
// upserts by that pair or marks a volcano absent from its source.
package volcano

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier is the subset of pgxpool.Pool (or pgx.Tx) this package needs.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Volcano is a single row of the catalog.
type Volcano struct {
	ID                 int64
	Name               string
	Country            string
	Latitude           float64
	Longitude          float64
	ElevationM         *float64
	Status             string
	SourceID           int64
	SourceRef          string
	AbsentFromSourceAt *time.Time
}

// Record is the shape accepted for an upsert coming from an external
// source. SourceID and SourceRef are mandatory: the catalogo-vulcoes spec
// requires that "registro de vulcão exige atribuição" — an insert without
// a source reference is rejected, not silently accepted with a NULL
// origin.
type Record struct {
	SourceID   int64
	SourceRef  string
	Name       string
	Country    string
	Latitude   float64
	Longitude  float64
	ElevationM *float64
	Status     string
}

// ErrMissingAttribution is returned by Upsert when SourceID or SourceRef
// is not provided.
var ErrMissingAttribution = errors.New("volcano: record must carry source_id and source_ref")

// ErrInvalidCoordinate is returned by Upsert when latitude/longitude are
// outside the valid WGS84 range. The database also enforces this via
// CHECK constraints (migration 008); this check exists so callers can
// reject and log at the application layer without relying on a failed
// statement's error text, per the catalogo-vulcoes spec: "esse registro é
// rejeitado, o erro é registrado em log identificando o vulcão".
var ErrInvalidCoordinate = errors.New("volcano: latitude/longitude out of valid range")

// UpsertOutcome reports what Upsert actually did, so importers can build
// an accurate report without re-deriving it from row counts themselves.
type UpsertOutcome int

const (
	OutcomeInserted UpsertOutcome = iota
	OutcomeUpdated
	OutcomeUnchanged
)

// Validate checks Record invariants that Upsert enforces regardless of
// what the database would also catch, so callers (e.g. an importer
// iterating many records) can validate before ever issuing a statement.
func (r Record) Validate() error {
	if r.SourceID == 0 || r.SourceRef == "" {
		return ErrMissingAttribution
	}
	if r.Latitude < -90 || r.Latitude > 90 || r.Longitude < -180 || r.Longitude > 180 {
		return fmt.Errorf("%w: lat=%v lon=%v", ErrInvalidCoordinate, r.Latitude, r.Longitude)
	}
	return nil
}

// Upsert inserts a new volcano or updates an existing one identified by
// (source_id, source_ref), per design.md D5's identity rule. It never
// deletes and never assigns a new ID to a volcano that already exists for
// that pair, so foreign keys from observations remain valid across
// reimports (spec: "referências permanecem válidas").
//
// If the row previously had absent_from_source_at set and now reappears
// in the source, that field is cleared as part of the same update.
func Upsert(ctx context.Context, q Querier, r Record) (Volcano, UpsertOutcome, error) {
	if err := r.Validate(); err != nil {
		return Volcano{}, 0, err
	}

	var existing Volcano
	var absentAt *time.Time
	err := q.QueryRow(ctx, `
		SELECT id, name, COALESCE(country, ''), latitude, longitude, elevation_m, COALESCE(status, ''), absent_from_source_at
		FROM volcanoes
		WHERE source_id = $1 AND source_ref = $2
	`, r.SourceID, r.SourceRef).Scan(
		&existing.ID, &existing.Name, &existing.Country, &existing.Latitude, &existing.Longitude,
		&existing.ElevationM, &existing.Status, &absentAt)

	if errors.Is(err, pgx.ErrNoRows) {
		var inserted Volcano
		insErr := q.QueryRow(ctx, `
			INSERT INTO volcanoes (name, country, latitude, longitude, elevation_m, status, source_id, source_ref)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, name, COALESCE(country, ''), latitude, longitude, elevation_m, COALESCE(status, ''), source_id, source_ref
		`, r.Name, nullableString(r.Country), r.Latitude, r.Longitude, r.ElevationM, nullableString(r.Status), r.SourceID, r.SourceRef).
			Scan(&inserted.ID, &inserted.Name, &inserted.Country, &inserted.Latitude, &inserted.Longitude,
				&inserted.ElevationM, &inserted.Status, &inserted.SourceID, &inserted.SourceRef)
		if insErr != nil {
			return Volcano{}, 0, fmt.Errorf("volcano: insert failed: %w", insErr)
		}
		return inserted, OutcomeInserted, nil
	}
	if err != nil {
		return Volcano{}, 0, fmt.Errorf("volcano: lookup failed: %w", err)
	}

	existing.SourceID = r.SourceID
	existing.SourceRef = r.SourceRef
	existing.AbsentFromSourceAt = absentAt

	unchanged := existing.Name == r.Name &&
		existing.Country == r.Country &&
		existing.Latitude == r.Latitude &&
		existing.Longitude == r.Longitude &&
		elevationEqual(existing.ElevationM, r.ElevationM) &&
		existing.Status == r.Status &&
		absentAt == nil

	if unchanged {
		return existing, OutcomeUnchanged, nil
	}

	var updated Volcano
	updErr := q.QueryRow(ctx, `
		UPDATE volcanoes
		SET name = $1, country = $2, latitude = $3, longitude = $4, elevation_m = $5, status = $6,
		    absent_from_source_at = NULL
		WHERE id = $7
		RETURNING id, name, COALESCE(country, ''), latitude, longitude, elevation_m, COALESCE(status, ''), source_id, source_ref
	`, r.Name, nullableString(r.Country), r.Latitude, r.Longitude, r.ElevationM, nullableString(r.Status), existing.ID).
		Scan(&updated.ID, &updated.Name, &updated.Country, &updated.Latitude, &updated.Longitude,
			&updated.ElevationM, &updated.Status, &updated.SourceID, &updated.SourceRef)
	if updErr != nil {
		return Volcano{}, 0, fmt.Errorf("volcano: update failed: %w", updErr)
	}
	return updated, OutcomeUpdated, nil
}

// MarkAbsent sets absent_from_source_at = now() for every volcano
// belonging to sourceID whose source_ref is not in seenRefs and that is
// not already marked absent. It never deletes rows (catalogo-vulcoes
// spec: "vulcões que desapareceram da fonte... marcados como ausentes na
// origem", project.md constraint #1 / master spec §89 item 12).
//
// It returns the number of rows newly marked.
func MarkAbsent(ctx context.Context, q Querier, sourceID int64, seenRefs []string) (int64, error) {
	tag, err := q.Exec(ctx, `
		UPDATE volcanoes
		SET absent_from_source_at = now()
		WHERE source_id = $1
		  AND absent_from_source_at IS NULL
		  AND NOT (source_ref = ANY($2))
	`, sourceID, seenRefs)
	if err != nil {
		return 0, fmt.Errorf("volcano: mark absent failed: %w", err)
	}
	return tag.RowsAffected(), nil
}

// GetBySourceRef fetches a single volcano by its natural key, mainly for
// tests that assert identity stability across reimports.
func GetBySourceRef(ctx context.Context, q Querier, sourceID int64, sourceRef string) (Volcano, error) {
	var v Volcano
	var absentAt *time.Time
	err := q.QueryRow(ctx, `
		SELECT id, name, COALESCE(country, ''), latitude, longitude, elevation_m, COALESCE(status, ''), source_id, source_ref, absent_from_source_at
		FROM volcanoes
		WHERE source_id = $1 AND source_ref = $2
	`, sourceID, sourceRef).Scan(&v.ID, &v.Name, &v.Country, &v.Latitude, &v.Longitude, &v.ElevationM, &v.Status, &v.SourceID, &v.SourceRef, &absentAt)
	if err != nil {
		return Volcano{}, fmt.Errorf("volcano: get failed: %w", err)
	}
	v.AbsentFromSourceAt = absentAt
	return v, nil
}

func elevationEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
