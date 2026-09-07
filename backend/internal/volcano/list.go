package volcano

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidQuery reports a caller-supplied filter that cannot be honoured.
var ErrInvalidQuery = errors.New("volcano: invalid query")

// NearPoint asks for volcanoes within RadiusM metres of (Lat, Lon).
type NearPoint struct {
	Lat     float64
	Lon     float64
	RadiusM float64
}

// ListQuery describes a catalog listing.
//
// Pagination is keyset, not offset: offset degrades on large tables and, worse,
// can repeat or skip rows when writes land between pages, which would break the
// api-publica requirement that paging never repeats nor omits an item.
type ListQuery struct {
	// Near, when set, restricts results to a radius and orders them by
	// increasing distance from the point.
	Near *NearPoint
	// Country filters by exact country string when non-empty.
	Country string
	// Search matches volcanoes whose name or country contains the term,
	// case-insensitively.
	Search string
	// Status filters by the source's activity-evidence classification.
	Status string
	// IncludeAbsent controls whether volcanoes no longer present in their
	// source are returned. They are kept in the catalog forever, but a plain
	// listing should not silently mix them with current records.
	IncludeAbsent bool
	// Limit is the maximum number of rows to return. Callers must validate it
	// against their own maximum before calling.
	Limit int
	// AfterID resumes a keyset scan: only rows ordered after this id are
	// returned. For distance-ordered queries AfterDistanceM must be set too.
	AfterID int64
	// AfterDistanceM is the distance cursor component for Near queries.
	AfterDistanceM *float64
}

// Result is one catalog row, optionally carrying its distance from the query
// point.
type Result struct {
	Volcano
	// DistanceM is set only for Near queries.
	DistanceM *float64
}

const listColumns = `v.id, v.name, coalesce(v.country, ''), v.latitude, v.longitude,
	v.elevation_m, coalesce(v.status, ''), v.source_id, v.source_ref, v.absent_from_source_at`

// List returns catalog rows matching the query.
//
// Ordering is total in both modes — (distance, id) or (id) — so that a keyset
// cursor is unambiguous even when many volcanoes share a distance.
func List(ctx context.Context, q Querier, lq ListQuery) ([]Result, error) {
	if lq.Limit <= 0 {
		return nil, fmt.Errorf("%w: limit must be positive", ErrInvalidQuery)
	}

	var (
		sb    strings.Builder
		args  []any
		where []string
	)

	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if lq.Near != nil {
		n := *lq.Near
		if n.Lat < -90 || n.Lat > 90 {
			return nil, fmt.Errorf("%w: latitude %v is outside -90..90", ErrInvalidQuery, n.Lat)
		}
		if n.Lon < -180 || n.Lon > 180 {
			return nil, fmt.Errorf("%w: longitude %v is outside -180..180", ErrInvalidQuery, n.Lon)
		}
		if n.RadiusM <= 0 {
			return nil, fmt.Errorf("%w: radius must be positive", ErrInvalidQuery)
		}

		pt := fmt.Sprintf("ST_MakePoint(%s, %s)::geography", arg(n.Lon), arg(n.Lat))
		sb.WriteString("SELECT " + listColumns + ", ST_Distance(v.location, " + pt + ") AS distance_m\nFROM volcanoes v\n")
		where = append(where, fmt.Sprintf("ST_DWithin(v.location, %s, %s)", pt, arg(n.RadiusM)))

		if lq.AfterDistanceM != nil {
			// Keyset over the same tuple the ORDER BY uses.
			where = append(where, fmt.Sprintf(
				"(ST_Distance(v.location, %s), v.id) > (%s, %s)",
				pt, arg(*lq.AfterDistanceM), arg(lq.AfterID)))
		}
	} else {
		sb.WriteString("SELECT " + listColumns + ", NULL::double precision AS distance_m\nFROM volcanoes v\n")
		if lq.AfterID > 0 {
			where = append(where, fmt.Sprintf("v.id > %s", arg(lq.AfterID)))
		}
	}

	if lq.Country != "" {
		where = append(where, fmt.Sprintf("v.country = %s", arg(lq.Country)))
	}
	if lq.Status != "" {
		where = append(where, fmt.Sprintf("v.status = %s", arg(lq.Status)))
	}
	if lq.Search != "" {
		// ILIKE com curinga nas duas pontas: o termo pode aparecer em
		// qualquer posicao do nome ou do pais. Escapamos os curingas do
		// proprio termo para que "%" digitado pelo usuario seja tratado como
		// texto, e nao como "case tudo".
		term := "%" + escapeLike(lq.Search) + "%"
		where = append(where, fmt.Sprintf(
			"(v.name ILIKE %s ESCAPE '\\' OR v.country ILIKE %s ESCAPE '\\')",
			arg(term), arg(term)))
	}
	if !lq.IncludeAbsent {
		where = append(where, "v.absent_from_source_at IS NULL")
	}

	if len(where) > 0 {
		sb.WriteString("WHERE " + strings.Join(where, "\n  AND ") + "\n")
	}
	if lq.Near != nil {
		sb.WriteString("ORDER BY distance_m, v.id\n")
	} else {
		sb.WriteString("ORDER BY v.id\n")
	}
	sb.WriteString("LIMIT " + fmt.Sprintf("%d", lq.Limit))

	rows, err := q.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("volcano: list: %w", err)
	}
	defer rows.Close()

	var out []Result
	for rows.Next() {
		var r Result
		if err := rows.Scan(
			&r.ID, &r.Name, &r.Country, &r.Latitude, &r.Longitude,
			&r.ElevationM, &r.Status, &r.SourceID, &r.SourceRef,
			&r.AbsentFromSourceAt, &r.DistanceM,
		); err != nil {
			return nil, fmt.Errorf("volcano: list: scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("volcano: list: %w", err)
	}
	return out, nil
}

// escapeLike neutraliza os curingas do LIKE dentro de um termo de busca.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)
	return r.Replace(s)
}

// Count returns how many volcanoes are currently present in the catalog,
// excluding those marked absent from their source.
func Count(ctx context.Context, q Querier) (int64, error) {
	var n int64
	err := q.QueryRow(ctx,
		`SELECT count(*) FROM volcanoes WHERE absent_from_source_at IS NULL`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("volcano: count: %w", err)
	}
	return n, nil
}
