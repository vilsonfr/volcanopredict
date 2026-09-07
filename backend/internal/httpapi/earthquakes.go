package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/vilsonfr/volcanopredict/backend/internal/dataquality"
	"github.com/vilsonfr/volcanopredict/backend/internal/earthquake"
	"github.com/vilsonfr/volcanopredict/backend/internal/ingestion"
	"github.com/vilsonfr/volcanopredict/backend/internal/source"
	"github.com/vilsonfr/volcanopredict/backend/internal/usgs"
)

// EarthquakeDTO is the wire shape of one seismic event.
type EarthquakeDTO struct {
	ID int64 `json:"id"`
	// ExternalID is the source's own identifier, never rewritten, so any row
	// can be taken back to the origin.
	ExternalID string    `json:"external_id"`
	SourceID   int64     `json:"source_id"`
	OccurredAt time.Time `json:"occurred_at"`
	// IngestedAt is when this system learned of the event. Both instants are
	// exposed because the difference between them is the whole temporal model.
	IngestedAt time.Time `json:"ingested_at"`

	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	DepthKm   *float64 `json:"depth_km,omitempty"`

	Magnitude     *float64 `json:"magnitude,omitempty"`
	MagnitudeType string   `json:"magnitude_type,omitempty"`
	// SourceStatus is 'automatic' or 'reviewed' at the source. An automatic
	// solution has not been looked at by a person, and a consumer weighing the
	// two equally would be treating a preliminary estimate as a final one.
	SourceStatus string `json:"source_status,omitempty"`

	RMS            *float64 `json:"rms,omitempty"`
	AzimuthalGap   *float64 `json:"azimuthal_gap,omitempty"`
	StationCount   *int     `json:"station_count,omitempty"`
	MinDistanceDeg *float64 `json:"min_distance_deg,omitempty"`

	// IsSynthetic is never omitted, even when false: provenance must not have
	// to be inferred from a missing field.
	IsSynthetic bool `json:"is_synthetic"`
	// QualityState is likewise never omitted. A consumer that cannot see it
	// would read a suspect record as a good one.
	QualityState  string `json:"quality_state"`
	QualityReason string `json:"quality_reason,omitempty"`

	ParserVersion   string     `json:"parser_version"`
	SourceVersion   string     `json:"source_version,omitempty"`
	SourceUpdatedAt *time.Time `json:"source_updated_at,omitempty"`

	// DistanceKm is present only for proximity queries.
	DistanceKm *float64 `json:"distance_km,omitempty"`

	// Raw is the payload exactly as the source sent it, included only when
	// the caller asks with include_raw=true. It is large, and most callers do
	// not want it on every item — but reconciling a value with its origin has
	// to be possible without leaving the API.
	Raw json.RawMessage `json:"raw,omitempty"`
}

func earthquakeDTO(e earthquake.Earthquake, includeRaw bool) EarthquakeDTO {
	dto := EarthquakeDTO{
		ID:              e.ID,
		ExternalID:      e.ExternalID,
		SourceID:        e.SourceID,
		OccurredAt:      e.OccurredAt,
		IngestedAt:      e.IngestedAt,
		Latitude:        e.Latitude,
		Longitude:       e.Longitude,
		DepthKm:         e.DepthKm,
		Magnitude:       e.Magnitude,
		MagnitudeType:   e.MagnitudeType,
		SourceStatus:    e.SourceStatus,
		RMS:             e.RMS,
		AzimuthalGap:    e.AzimuthalGap,
		StationCount:    e.StationCount,
		MinDistanceDeg:  e.MinDistanceDeg,
		IsSynthetic:     e.IsSynthetic,
		QualityState:    e.QualityState.String(),
		QualityReason:   e.QualityReason,
		ParserVersion:   e.ParserVersion,
		SourceVersion:   e.SourceVersion,
		SourceUpdatedAt: e.SourceUpdatedAt,
		DistanceKm:      e.DistanceKm,
	}
	if includeRaw {
		dto.Raw = e.Raw
	}
	return dto
}

func (s *Server) handleListEarthquakes(w http.ResponseWriter, r *http.Request) {
	limit, err := PageSize(r)
	if err != nil {
		WriteParamError(w, r, err)
		return
	}
	cursor, err := CursorParam(r)
	if err != nil {
		WriteParamError(w, r, err)
		return
	}

	q := earthquake.Query{Limit: limit, AfterID: cursor.ID, AfterDistanceKm: cursor.DistanceM}

	if asOf, ok, err := timeParam(r, "as_of"); err != nil {
		WriteParamError(w, r, err)
		return
	} else if ok {
		if asOf.After(time.Now().UTC()) {
			WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter,
				"as_of não pode ser um instante futuro.", "as_of")
			return
		}
		q.AsOf = &asOf
	}

	from, hasFrom, err := timeParam(r, "from")
	if err != nil {
		WriteParamError(w, r, err)
		return
	}
	to, hasTo, err := timeParam(r, "to")
	if err != nil {
		WriteParamError(w, r, err)
		return
	}
	if hasFrom && hasTo && from.After(to) {
		WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter,
			"O início da janela é posterior ao fim.", "from")
		return
	}
	if hasFrom {
		q.OccurredFrom = &from
	}
	if hasTo {
		q.OccurredTo = &to
	}

	switch r.URL.Query().Get("provenance") {
	case "", "real":
		// Defaulting to real-only is deliberate, as elsewhere: an omitted
		// filter must never mix synthetic records into what a caller reads.
		q.Provenance = earthquake.ProvenanceRealOnly
	case "synthetic":
		q.Provenance = earthquake.ProvenanceSyntheticOnly
	case "all":
		q.Provenance = earthquake.ProvenanceAll
	default:
		WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter,
			`Valor inválido. Use "real", "synthetic" ou "all".`, "provenance")
		return
	}

	if states, err := qualityParam(r); err != nil {
		WriteParamError(w, r, err)
		return
	} else {
		q.QualityStates = states
	}

	if minMag, ok, err := floatParam(r, "min_magnitude"); err != nil {
		WriteParamError(w, r, err)
		return
	} else if ok {
		q.MinMagnitude = &minMag
	}

	lat, hasLat, err := floatParam(r, "lat")
	if err != nil {
		WriteParamError(w, r, err)
		return
	}
	lon, hasLon, err := floatParam(r, "lon")
	if err != nil {
		WriteParamError(w, r, err)
		return
	}
	radiusKm, hasRadius, err := floatParam(r, "radius_km")
	if err != nil {
		WriteParamError(w, r, err)
		return
	}
	if hasLat || hasLon || hasRadius {
		// All-or-nothing: answering a half-specified proximity query by
		// guessing the missing part returns the wrong set, silently.
		if !hasLat || !hasLon || !hasRadius {
			WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter,
				"Consulta por proximidade exige lat, lon e radius_km juntos.", missingProximityParam(hasLat, hasLon, hasRadius))
			return
		}
		if err := earthquake.ValidateCoordinate(lat, lon); err != nil {
			WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter,
				"Coordenada fora da faixa válida.", "lat")
			return
		}
		if radiusKm <= 0 {
			WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter,
				"radius_km deve ser positivo.", "radius_km")
			return
		}
		q.NearLatitude = &lat
		q.NearLongitude = &lon
		q.RadiusKm = &radiusKm
	}

	includeRaw := r.URL.Query().Get("include_raw") == "true"

	ctx, cancel := contextWithTimeout(r, 15*time.Second)
	defer cancel()

	events, err := earthquake.List(ctx, s.DB, q)
	if err != nil {
		if errors.Is(err, earthquake.ErrInvalidCoordinate) || errors.Is(err, earthquake.ErrFutureAsOf) {
			WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter, err.Error(), "")
			return
		}
		WriteInternal(w, r, err)
		return
	}

	data := make([]EarthquakeDTO, 0, len(events))
	for _, e := range events {
		data = append(data, earthquakeDTO(e, includeRaw))
	}

	meta := Meta{Count: len(data)}
	if len(events) == limit && limit > 0 {
		last := events[len(events)-1]
		next := Cursor{ID: last.ID}
		// A distance-ordered page resumes on (distance, id), matching the
		// order the rows came back in.
		if last.DistanceKm != nil {
			next.DistanceM = last.DistanceKm
		}
		meta.NextCursor = next.Encode()
	}
	meta.Quality = qualityComposition(events)

	// A window query must declare how much of that window was actually
	// collected. Without it, an empty page reads as "nothing happened" when
	// it may mean "we never looked" (§74).
	if hasFrom || hasTo {
		coverage, err := s.coverageFor(ctx, from, to, hasFrom, hasTo)
		if err != nil {
			WriteInternal(w, r, err)
			return
		}
		meta.Coverage = coverage
	}

	WriteData(w, r, data, meta, false, s.catalogAttribution(ctx)...)
}

func missingProximityParam(hasLat, hasLon, hasRadius bool) string {
	switch {
	case !hasLat:
		return "lat"
	case !hasLon:
		return "lon"
	default:
		return "radius_km"
	}
}

// qualityParam reads the optional quality filter, rejecting an invented
// state rather than quietly returning everything.
func qualityParam(r *http.Request) ([]dataquality.State, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("quality"))
	if raw == "" {
		return nil, nil
	}
	var out []dataquality.State
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		st, err := dataquality.ParseState(part)
		if err != nil {
			return nil, badParam("quality",
				`Valor inválido. Use valid, suspect, duplicate, outlier, corrupted ou delayed.`)
		}
		out = append(out, st)
	}
	return out, nil
}

// qualityComposition counts the states present in a page, so a consumer can
// see that a set is of mixed quality instead of having to inspect every item.
func qualityComposition(events []earthquake.Earthquake) map[string]int {
	if len(events) == 0 {
		return nil
	}
	out := map[string]int{}
	for _, e := range events {
		out[e.QualityState.String()]++
	}
	return out
}

// coverageFor reports how much of the requested window was collected, for
// every enabled source that is actually ingested.
//
// An open-ended window is closed at now for the question to be answerable:
// "was the future collected?" has no meaning.
func (s *Server) coverageFor(ctx context.Context, from, to time.Time, hasFrom, hasTo bool) (*CoverageDTO, error) {
	now := time.Now().UTC()
	start, end := from.UTC(), to.UTC()
	if !hasFrom {
		// With no lower bound there is nothing to claim coverage over:
		// the collection could reach arbitrarily far back.
		return nil, nil
	}
	if !hasTo || end.After(now) {
		end = now
	}
	if end.Before(start) {
		return nil, nil
	}

	src, err := source.GetEnabledByName(ctx, s.DB, usgs.SourceName)
	switch {
	case err == nil:
	case errors.Is(err, source.ErrNotFound), errors.Is(err, source.ErrDisabled):
		// The source not being registered or enabled is a real answer: no
		// collection exists for this window.
		return &CoverageDTO{
			Kind: string(ingestion.CoverageNone),
			From: start,
			To:   end,
			Gaps: []GapDTO{{From: start, To: end}},
			Note: coverageNote(ingestion.CoverageNone),
		}, nil
	default:
		// Anything else — a database hiccup, a timeout — is NOT evidence
		// that the window went uncollected. Claiming "none" here would turn
		// an internal failure into an affirmative statement about the
		// world, which is the one thing this endpoint exists to avoid.
		return nil, err
	}

	cov, err := ingestion.CoverageFor(ctx, s.DB, src.ID, start, end)
	if err != nil {
		return nil, err
	}

	dto := &CoverageDTO{
		Kind: string(cov.Kind),
		From: cov.WindowStart,
		To:   cov.WindowEnd,
		Note: coverageNote(cov.Kind),
	}
	for _, g := range cov.Gaps {
		dto.Gaps = append(dto.Gaps, GapDTO{From: g.Start, To: g.End})
	}
	return dto, nil
}

func coverageNote(kind ingestion.CoverageKind) string {
	switch kind {
	case ingestion.CoverageComplete:
		return "A janela inteira foi coletada. Uma coleção vazia aqui significa que nada ocorreu."
	case ingestion.CoveragePartial:
		return "Parte da janela não foi coletada. Ausência de eventos nos intervalos listados NÃO significa ausência de atividade."
	default:
		return "Esta janela não foi coletada. Ausência de eventos NÃO significa ausência de atividade."
	}
}

// --- ingestion status ---------------------------------------------------

// IngestionStatusDTO is the operational state of one source's ingestion.
type IngestionStatusDTO struct {
	Source string `json:"source"`
	// EverCollected distinguishes a source that was enabled and never
	// collected from one whose last collection failed. They must not look
	// alike.
	EverCollected bool `json:"ever_collected"`
	// Healthy is true only when the most recent run succeeded.
	Healthy         bool       `json:"healthy"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	LastRunResult   string     `json:"last_run_result,omitempty"`
	LastSuccessAt   *time.Time `json:"last_success_at,omitempty"`
	StaleForSeconds *int64     `json:"stale_for_seconds,omitempty"`
	// FailureReason describes what went wrong, already sanitized: it never
	// carries SQL, file paths, credentials or a stack trace.
	FailureReason string `json:"failure_reason,omitempty"`
}

func (s *Server) handleIngestionStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()

	srcs, err := source.List(ctx, s.DB)
	if err != nil {
		WriteInternal(w, r, err)
		return
	}

	data := make([]IngestionStatusDTO, 0)
	for _, src := range srcs {
		if !src.Enabled {
			continue
		}
		// Only sources that are actually collected on a cadence belong
		// here. The GVP catalog is enabled but arrives by swapping a
		// versioned snapshot and running import-catalog, so it has no
		// ingestion runs — listing it as "never collected" would read as a
		// broken pipeline when nothing is broken.
		if src.CollectionCadence == "" {
			continue
		}
		st, err := ingestion.StatusFor(ctx, s.DB, src.ID)
		if err != nil {
			WriteInternal(w, r, err)
			return
		}

		dto := IngestionStatusDTO{Source: src.Name, EverCollected: st.EverRun}
		if st.LastRun != nil {
			at := st.LastRun.StartedAt
			dto.LastRunAt = &at
			dto.LastRunResult = string(st.LastRun.Result)
			dto.Healthy = st.LastRun.Result == ingestion.ResultSuccess
			if st.LastRun.Result == ingestion.ResultFailure {
				dto.FailureReason = sanitizeFailure(st.LastRun.ErrorMsg)
			}
		}
		if st.LastSuccess != nil && st.LastSuccess.FinishedAt != nil {
			at := *st.LastSuccess.FinishedAt
			dto.LastSuccessAt = &at
		}
		if st.EverRun {
			secs := int64(st.StaleFor.Seconds())
			dto.StaleForSeconds = &secs
		}
		data = append(data, dto)
	}

	WriteData(w, r, data, Meta{Count: len(data)}, false)
}

// sanitizeFailure keeps the shape of the failure while refusing to leak
// internals to an unauthenticated caller. The full text stays in the log.
func sanitizeFailure(msg string) string {
	switch {
	case msg == "":
		return ""
	case strings.Contains(msg, "usgs: source unavailable"),
		strings.Contains(msg, "request to source failed"),
		strings.Contains(msg, "source returned HTTP"):
		return "A fonte externa não respondeu."
	case strings.Contains(msg, "usgs: parse:"):
		return "A resposta da fonte não tinha o formato esperado."
	case strings.Contains(msg, "usgs: query matches more events"):
		return "A consulta à fonte excedeu o limite de eventos por requisição."
	default:
		// Anything unrecognized is assumed to carry internals.
		return "Falha na ingestão. Consulte o log pelo identificador da execução."
	}
}
