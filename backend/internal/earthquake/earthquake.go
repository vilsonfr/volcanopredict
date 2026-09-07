// Package earthquake is the sole place in the codebase allowed to write
// temporal SQL predicates over the `earthquakes` table (design.md D10,
// mirroring D7 for observations). Every other package that needs "what did
// we know about this earthquake at time T" calls in here instead of
// building its own WHERE clause — that is what keeps the data-leakage
// guarantee auditable at a single point.
//
// It deliberately duplicates the shape of internal/observation rather than
// generalizing it: the natural keys differ, and hiding that difference
// behind a parameter would make the one thing a reader most needs to see
// the one thing hardest to see.
package earthquake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vilsonfr/volcanopredict/backend/internal/dataquality"
)

// Querier is the subset of pgxpool.Pool (or pgx.Tx) this package needs,
// so callers can pass either a pool or a transaction. It is deliberately
// the same narrow pair internal/observation asks for: every write here
// goes through a RETURNING query, so Exec is not needed and not offered.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Provenance selects which mix of real/synthetic data a query returns.
// As in internal/observation, there is no zero value meaning "both":
// callers must choose explicitly.
type Provenance int

const (
	// ProvenanceUnspecified is the zero value and is rejected by List.
	ProvenanceUnspecified Provenance = iota
	ProvenanceRealOnly
	ProvenanceSyntheticOnly
	ProvenanceAll
)

var (
	// ErrFutureAsOf is returned when a caller asks for an as-of instant
	// later than now. Asking about the future is the caller's mistake, not
	// something to quietly answer with the present.
	ErrFutureAsOf = errors.New("earthquake: as-of instant is in the future")
	// ErrProvenanceUnspecified is returned when List is called without an
	// explicit Provenance filter.
	ErrProvenanceUnspecified = errors.New("earthquake: provenance filter must be specified explicitly")
	// ErrInvalidCoordinate is returned for a latitude or longitude outside
	// the only range those numbers can occupy.
	ErrInvalidCoordinate = errors.New("earthquake: coordinate out of range")
)

// Earthquake is one bitemporal version of one seismic event.
type Earthquake struct {
	ID         int64
	ExternalID string
	SourceID   int64
	OccurredAt time.Time
	IngestedAt time.Time

	Latitude  float64
	Longitude float64
	DepthKm   *float64

	Magnitude     *float64
	MagnitudeType string
	SourceStatus  string

	RMS            *float64
	AzimuthalGap   *float64
	StationCount   *int
	MinDistanceDeg *float64

	IsSynthetic     bool
	QualityState    dataquality.State
	QualityReason   string
	ParserVersion   string
	SourceVersion   string
	SourceUpdatedAt *time.Time
	Raw             json.RawMessage

	// DistanceKm is filled only by proximity queries.
	DistanceKm *float64
}

// New is the shape accepted for a write.
//
// IngestedAt is deliberately absent: the database assigns it (DEFAULT plus
// the BEFORE INSERT trigger from migration 009), and this package never
// sends that column. There is nothing here for a caller to smuggle in.
type New struct {
	ExternalID string
	SourceID   int64
	OccurredAt time.Time

	Latitude  float64
	Longitude float64
	DepthKm   *float64

	Magnitude     *float64
	MagnitudeType string
	SourceStatus  string

	RMS            *float64
	AzimuthalGap   *float64
	StationCount   *int
	MinDistanceDeg *float64

	IsSynthetic     bool
	Quality         dataquality.Verdict
	ParserVersion   string
	SourceVersion   string
	SourceUpdatedAt *time.Time
	Raw             json.RawMessage
}

// Query describes a temporal read. AsOf nil means current knowledge.
type Query struct {
	SourceID     *int64
	ExternalID   *string
	OccurredFrom *time.Time
	OccurredTo   *time.Time
	AsOf         *time.Time
	Provenance   Provenance

	MinMagnitude *float64

	// QualityStates, when non-empty, restricts the result to those states.
	QualityStates []dataquality.State

	// Near, when set with RadiusKm, restricts to events within the radius
	// and fills DistanceKm on every result.
	NearLatitude  *float64
	NearLongitude *float64
	RadiusKm      *float64

	// AfterID resumes a keyset scan: only rows ordered after this id are
	// returned. For distance-ordered queries AfterDistanceKm must be set
	// too, because the keyset must cover exactly the tuple the ORDER BY
	// uses — a cursor on id alone against an order on (distance, id) both
	// repeats and skips rows.
	AfterID         int64
	AfterDistanceKm *float64

	Limit int
}

const selectColumns = `
	id, external_id, source_id, occurred_at, ingested_at,
	ST_Y(location::geometry) AS latitude, ST_X(location::geometry) AS longitude,
	depth_km, magnitude, coalesce(magnitude_type, ''), coalesce(source_status, ''),
	rms, azimuthal_gap, station_count, min_distance_deg,
	is_synthetic, quality_state, coalesce(quality_reason, ''),
	parser_version, coalesce(source_version, ''), source_updated_at, raw
`

// List returns the current version of each earthquake matching q, as
// known at q.AsOf.
//
// The `ingested_at <= asOf` predicate below is the data-leakage guarantee.
// Removing or weakening it must turn TestDataLeakage_* red.
func List(ctx context.Context, q Querier, query Query) ([]Earthquake, error) {
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

	var (
		args  = []any{asOf}
		where []string
	)
	add := func(v any) int {
		args = append(args, v)
		return len(args)
	}

	if query.SourceID != nil {
		where = append(where, fmt.Sprintf("source_id = $%d", add(*query.SourceID)))
	}
	if query.ExternalID != nil {
		where = append(where, fmt.Sprintf("external_id = $%d", add(*query.ExternalID)))
	}
	if query.OccurredFrom != nil {
		where = append(where, fmt.Sprintf("occurred_at >= $%d", add(query.OccurredFrom.UTC())))
	}
	if query.OccurredTo != nil {
		where = append(where, fmt.Sprintf("occurred_at <= $%d", add(query.OccurredTo.UTC())))
	}
	if query.MinMagnitude != nil {
		// A null magnitude is not "below the floor" — it is unknown, and
		// silently dropping it would be answering a question nobody asked.
		where = append(where, fmt.Sprintf("magnitude IS NOT NULL AND magnitude >= $%d", add(*query.MinMagnitude)))
	}
	switch query.Provenance {
	case ProvenanceRealOnly:
		where = append(where, "is_synthetic = false")
	case ProvenanceSyntheticOnly:
		where = append(where, "is_synthetic = true")
	case ProvenanceAll:
	}
	if len(query.QualityStates) > 0 {
		states := make([]string, 0, len(query.QualityStates))
		for _, s := range query.QualityStates {
			states = append(states, s.String())
		}
		where = append(where, fmt.Sprintf("quality_state = ANY($%d)", add(states)))
	}

	distanceExpr := "NULL::double precision"
	if query.NearLatitude != nil || query.NearLongitude != nil || query.RadiusKm != nil {
		if query.NearLatitude == nil || query.NearLongitude == nil || query.RadiusKm == nil {
			return nil, fmt.Errorf("earthquake: a proximity query needs latitude, longitude and radius together")
		}
		if err := ValidateCoordinate(*query.NearLatitude, *query.NearLongitude); err != nil {
			return nil, err
		}
		pointIdx := add(fmt.Sprintf("SRID=4326;POINT(%v %v)", *query.NearLongitude, *query.NearLatitude))
		radiusIdx := add(*query.RadiusKm * 1000)
		where = append(where, fmt.Sprintf("ST_DWithin(location, $%d::geography, $%d)", pointIdx, radiusIdx))
		distanceExpr = fmt.Sprintf("ST_Distance(location, $%d::geography) / 1000.0", pointIdx)
	}

	whereSQL := "ingested_at <= $1"
	if len(where) > 0 {
		whereSQL += " AND " + strings.Join(where, " AND ")
	}

	sql := fmt.Sprintf(`
		SELECT * FROM (
			SELECT DISTINCT ON (external_id, source_id)
				%s, %s AS distance_km
			FROM earthquakes
			WHERE %s
			ORDER BY external_id, source_id, ingested_at DESC
		) current_versions
	`, selectColumns, distanceExpr, whereSQL)

	// Keyset pagination needs a total order, and the cursor must be over
	// exactly the tuple the ORDER BY uses; id breaks every tie.
	if query.NearLatitude != nil {
		if query.AfterDistanceKm != nil {
			sql += fmt.Sprintf(" WHERE (distance_km, id) > ($%d, $%d)",
				add(*query.AfterDistanceKm), add(query.AfterID))
		}
		sql += " ORDER BY distance_km ASC, id ASC"
	} else {
		if query.AfterID > 0 {
			sql += fmt.Sprintf(" WHERE id > $%d", add(query.AfterID))
		}
		sql += " ORDER BY id ASC"
	}
	if query.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT $%d", add(query.Limit))
	}

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("earthquake: list query failed: %w", err)
	}
	defer rows.Close()

	var out []Earthquake
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("earthquake: row iteration failed: %w", err)
	}
	return out, nil
}

func scan(rows pgx.Rows) (Earthquake, error) {
	var e Earthquake
	var qualityState string
	if err := rows.Scan(
		&e.ID, &e.ExternalID, &e.SourceID, &e.OccurredAt, &e.IngestedAt,
		&e.Latitude, &e.Longitude,
		&e.DepthKm, &e.Magnitude, &e.MagnitudeType, &e.SourceStatus,
		&e.RMS, &e.AzimuthalGap, &e.StationCount, &e.MinDistanceDeg,
		&e.IsSynthetic, &qualityState, &e.QualityReason,
		&e.ParserVersion, &e.SourceVersion, &e.SourceUpdatedAt, &e.Raw,
		&e.DistanceKm,
	); err != nil {
		return Earthquake{}, fmt.Errorf("earthquake: scan failed: %w", err)
	}
	state, err := dataquality.ParseState(qualityState)
	if err != nil {
		return Earthquake{}, fmt.Errorf("earthquake: stored quality state is not recognized: %w", err)
	}
	e.QualityState = state
	e.OccurredAt = e.OccurredAt.UTC()
	e.IngestedAt = e.IngestedAt.UTC()
	if e.SourceUpdatedAt != nil {
		u := e.SourceUpdatedAt.UTC()
		e.SourceUpdatedAt = &u
	}
	return e, nil
}

// ValidateCoordinate rejects a latitude or longitude that cannot exist.
func ValidateCoordinate(lat, lon float64) error {
	if lat < -90 || lat > 90 {
		return fmt.Errorf("%w: latitude %v is outside [-90, 90]", ErrInvalidCoordinate, lat)
	}
	if lon < -180 || lon > 180 {
		return fmt.Errorf("%w: longitude %v is outside [-180, 180]", ErrInvalidCoordinate, lon)
	}
	return nil
}
