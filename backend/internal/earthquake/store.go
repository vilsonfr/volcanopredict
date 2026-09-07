package earthquake

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/vilsonfr/volcanopredict/backend/internal/dataquality"
)

// Outcome reports what a Save did.
type Outcome int

const (
	// OutcomeInserted: no version of this event existed.
	OutcomeInserted Outcome = iota
	// OutcomeUpdated: a version existed and the source's content changed,
	// so a new version was appended.
	OutcomeUpdated
	// OutcomeUnchanged: a version existed with identical content, so
	// nothing was written.
	OutcomeUnchanged
)

func (o Outcome) String() string {
	switch o {
	case OutcomeInserted:
		return "inserted"
	case OutcomeUpdated:
		return "updated"
	default:
		return "unchanged"
	}
}

// Save appends a new version of an event, or does nothing when the source
// is telling us something we already know.
//
// Deduplication is by comparison of the normalized CONTENT, never by
// trusting the source's own `updated` field (design.md D3). A source that
// re-emits an identical payload with a fresh `updated` — which USGS does
// routinely — would otherwise create one version per ingestion cycle,
// inflating the history with revisions that revised nothing and making
// "second run reports zero changes" impossible to honor.
//
// ingested_at is never in the column list: the database owns it.
func Save(ctx context.Context, q Querier, n New) (Earthquake, Outcome, error) {
	if n.ExternalID == "" {
		return Earthquake{}, OutcomeUnchanged, fmt.Errorf("earthquake: external_id is required: the source's own identifier is the natural key")
	}
	if n.OccurredAt.IsZero() {
		return Earthquake{}, OutcomeUnchanged, fmt.Errorf("earthquake: occurred_at is required for %s", n.ExternalID)
	}
	if err := ValidateCoordinate(n.Latitude, n.Longitude); err != nil {
		return Earthquake{}, OutcomeUnchanged, fmt.Errorf("%w (event %s)", err, n.ExternalID)
	}
	// The quality engine is not optional. An unevaluated record must not
	// reach the table, where it would later read as fact.
	if err := n.Quality.Validate(); err != nil {
		return Earthquake{}, OutcomeUnchanged, fmt.Errorf("%w (event %s)", err, n.ExternalID)
	}
	if n.ParserVersion == "" {
		return Earthquake{}, OutcomeUnchanged, fmt.Errorf("earthquake: parser_version is required for %s: without it a parse bug is untraceable", n.ExternalID)
	}
	if len(n.Raw) == 0 {
		return Earthquake{}, OutcomeUnchanged, fmt.Errorf("earthquake: raw payload is required for %s: a derived value must stay reconstructable", n.ExternalID)
	}

	current, err := currentVersion(ctx, q, n.SourceID, n.ExternalID)
	if err != nil {
		return Earthquake{}, OutcomeUnchanged, err
	}
	if current != nil && sameContent(*current, n) {
		return *current, OutcomeUnchanged, nil
	}

	outcome := OutcomeInserted
	if current != nil {
		outcome = OutcomeUpdated
	}

	inserted, err := insert(ctx, q, n)
	if err != nil {
		return Earthquake{}, OutcomeUnchanged, err
	}
	return inserted, outcome, nil
}

func currentVersion(ctx context.Context, q Querier, sourceID int64, externalID string) (*Earthquake, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT %s, NULL::double precision AS distance_km
		FROM earthquakes
		WHERE source_id = $1 AND external_id = $2
		ORDER BY ingested_at DESC, id DESC
		LIMIT 1
	`, selectColumns), sourceID, externalID)
	if err != nil {
		return nil, fmt.Errorf("earthquake: looking up current version failed: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("earthquake: looking up current version failed: %w", err)
		}
		return nil, nil
	}
	e, err := scan(rows)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func insert(ctx context.Context, q Querier, n New) (Earthquake, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		INSERT INTO earthquakes (
			external_id, source_id, occurred_at, location,
			depth_km, magnitude, magnitude_type, source_status,
			rms, azimuthal_gap, station_count, min_distance_deg,
			is_synthetic, quality_state, quality_reason,
			parser_version, source_version, source_updated_at, raw
		) VALUES (
			$1, $2, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326)::geography,
			$6, $7, nullif($8, ''), nullif($9, ''),
			$10, $11, $12, $13,
			$14, $15, nullif($16, ''),
			$17, nullif($18, ''), $19, $20
		)
		RETURNING %s, NULL::double precision AS distance_km
	`, selectColumns),
		n.ExternalID, n.SourceID, n.OccurredAt.UTC(), n.Longitude, n.Latitude,
		n.DepthKm, n.Magnitude, n.MagnitudeType, n.SourceStatus,
		n.RMS, n.AzimuthalGap, n.StationCount, n.MinDistanceDeg,
		n.IsSynthetic, n.Quality.State.String(), n.Quality.Reason(),
		n.ParserVersion, n.SourceVersion, n.SourceUpdatedAt, []byte(n.Raw),
	)
	if err != nil {
		return Earthquake{}, fmt.Errorf("earthquake: insert failed for %s: %w", n.ExternalID, err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Earthquake{}, fmt.Errorf("earthquake: insert failed for %s: %w", n.ExternalID, err)
		}
		return Earthquake{}, fmt.Errorf("earthquake: insert returned no row for %s", n.ExternalID)
	}
	return scan(rows)
}

// sameContent reports whether the incoming record says anything new.
//
// Fields deliberately NOT compared:
//
//   - source_updated_at: the source bumps it without changing anything.
//     Comparing it is precisely the mistake design.md D3 rules out.
//
//   - source_version and raw: the service version and the exact bytes can
//     change while every measured value stays identical. A reformatted
//     payload is not a revision.
//
//   - parser_version: a parser change that produces identical values
//     produced no new knowledge. A parser change that produces different
//     values already shows up in the fields below.
//
//   - quality_state and quality_reason: the verdict is DERIVED from the
//     content plus the circumstances of the collection, and the
//     circumstances are not news about the earthquake. Comparing them cost
//     130 fake revisions in the first real incremental cycle: events
//     collected by an explicit window came in `valid`, and the next
//     routine cycle re-read the same unchanged events and judged them
//     `delayed` — because they were now days old — appending a version
//     each. Nothing about the earthquakes had changed.
//
//     A genuine quality change never hides behind this: quality is a
//     function of the content, so an implausible magnitude becoming
//     plausible changes the magnitude too, which IS compared, and the new
//     version carries the fresh verdict.
func sameContent(cur Earthquake, n New) bool {
	// Truncated to microseconds because that is the resolution the column
	// stores: a Go instant carrying nanoseconds comes back rounded, and
	// comparing it raw would report a revision on every single re-ingestion
	// of an unchanged event.
	return sameInstant(cur.OccurredAt, n.OccurredAt) &&
		sameFloat(&cur.Latitude, &n.Latitude) &&
		sameFloat(&cur.Longitude, &n.Longitude) &&
		sameFloat(cur.DepthKm, n.DepthKm) &&
		sameFloat(cur.Magnitude, n.Magnitude) &&
		cur.MagnitudeType == n.MagnitudeType &&
		cur.SourceStatus == n.SourceStatus &&
		sameFloat(cur.RMS, n.RMS) &&
		sameFloat(cur.AzimuthalGap, n.AzimuthalGap) &&
		sameInt(cur.StationCount, n.StationCount) &&
		sameFloat(cur.MinDistanceDeg, n.MinDistanceDeg) &&
		cur.IsSynthetic == n.IsSynthetic
}

// sameInstant compares two instants at the resolution the database keeps.
func sameInstant(a, b time.Time) bool {
	return a.UTC().Truncate(time.Microsecond).Equal(b.UTC().Truncate(time.Microsecond))
}

// sameFloat treats absence as its own value: nil equals nil, and nil never
// equals a number — including 0. A real event with magnitude 0.0 and an
// event with no magnitude are different facts.
func sameFloat(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	// The values come from the same JSON both times, so they round-trip
	// exactly; the epsilon guards against float noise introduced by the
	// database round trip, not against genuine revision.
	return math.Abs(*a-*b) < 1e-9
}

func sameInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// ForTest builds a New with the required fields filled in, so tests that
// are about something else do not have to restate them.
func ForTest(externalID string, sourceID int64, occurredAt time.Time, lat, lon float64) New {
	return New{
		ExternalID:    externalID,
		SourceID:      sourceID,
		OccurredAt:    occurredAt,
		Latitude:      lat,
		Longitude:     lon,
		Quality:       dataquality.Verdict{State: dataquality.StateValid},
		ParserVersion: "test",
		Raw:           json.RawMessage(`{}`),
	}
}
