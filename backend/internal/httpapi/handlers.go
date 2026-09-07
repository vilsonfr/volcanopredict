package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/vilsonfr/volcanopredict/backend/internal/observation"
	"github.com/vilsonfr/volcanopredict/backend/internal/source"
	"github.com/vilsonfr/volcanopredict/backend/internal/volcano"
)

// Server holds the dependencies the handlers need.
type Server struct {
	DB volcano.Querier
	// SchemaVersion is the migration version the binary expects. /health
	// refuses to report healthy against a database that is behind it.
	SchemaVersion int64
}

// VolcanoDTO is the wire shape of a catalog entry.
type VolcanoDTO struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Country    string   `json:"country,omitempty"`
	Latitude   float64  `json:"latitude"`
	Longitude  float64  `json:"longitude"`
	ElevationM *float64 `json:"elevation_m,omitempty"`
	Status     string   `json:"status,omitempty"`
	// SourceRef is the identifier the originating source uses, so a caller can
	// trace any row back to its origin.
	SourceRef string `json:"source_ref"`
	// AbsentFromSource marks a volcano the source stopped listing. It is kept
	// in the catalog forever and never deleted.
	AbsentFromSource bool `json:"absent_from_source,omitempty"`
	// DistanceM is present only for proximity queries.
	DistanceM *float64 `json:"distance_m,omitempty"`
}

// ObservationDTO is the wire shape of an observation.
type ObservationDTO struct {
	ID         int64     `json:"id"`
	VolcanoID  int64     `json:"volcano_id"`
	Kind       string    `json:"kind"`
	ObservedAt time.Time `json:"observed_at"`
	// IngestedAt is when the system learned of the fact, as opposed to when it
	// happened. Both are exposed because the difference is the whole point of
	// the temporal model.
	IngestedAt time.Time `json:"ingested_at"`
	Value      any       `json:"value"`
	// IsSynthetic is never omitted, even when false: a reader must never have
	// to infer provenance from a missing field.
	IsSynthetic bool  `json:"is_synthetic"`
	SourceID    int64 `json:"source_id"`
}

// RegisterRoutes wires the versioned API onto mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/volcanoes", s.handleListVolcanoes)
	mux.HandleFunc("GET /api/v1/observations", s.handleListObservations)
	mux.HandleFunc("GET /api/v1/sources", s.handleListSources)
	mux.HandleFunc("GET /api/v1/earthquakes", s.handleListEarthquakes)
	mux.HandleFunc("GET /api/v1/ingestion", s.handleIngestionStatus)

	// Anything under /api that is not a known versioned route is a 404 in the
	// standard error shape, never a response from some implicit version.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, CodeNotFound,
			"Rota desconhecida. A API é versionada: use o prefixo /api/v1/.", "")
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 2*time.Second)
	defer cancel()

	var version int64
	err := s.DB.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version`).Scan(&version)
	if err != nil {
		WriteError(w, r, http.StatusServiceUnavailable, CodeUnavailable,
			"Banco de dados indisponível.", "")
		return
	}
	if version < s.SchemaVersion {
		// Serving against an older schema than the binary expects would mean
		// answering queries whose columns may not exist yet.
		WriteError(w, r, http.StatusServiceUnavailable, CodeUnavailable,
			"Schema do banco desatualizado para esta versão do servidor.", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"schema_version": version,
	})
}

func (s *Server) handleListVolcanoes(w http.ResponseWriter, r *http.Request) {
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

	lq := volcano.ListQuery{
		Limit:         limit,
		Country:       r.URL.Query().Get("country"),
		Search:        strings.TrimSpace(r.URL.Query().Get("q")),
		Status:        r.URL.Query().Get("status"),
		IncludeAbsent: r.URL.Query().Get("include_absent") == "true",
		AfterID:       cursor.ID,
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

	switch {
	case hasLat || hasLon || hasRadius:
		// A proximity query is all-or-nothing: answering a half-specified one
		// by guessing the missing part would silently return the wrong set.
		if !hasLat || !hasLon || !hasRadius {
			WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter,
				"Consulta por proximidade exige lat, lon e radius_km juntos.", "radius_km")
			return
		}
		if lat < -90 || lat > 90 {
			WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter,
				"Latitude fora da faixa válida (-90 a 90).", "lat")
			return
		}
		if lon < -180 || lon > 180 {
			WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter,
				"Longitude fora da faixa válida (-180 a 180).", "lon")
			return
		}
		if radiusKm <= 0 {
			WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter,
				"radius_km deve ser maior que zero.", "radius_km")
			return
		}
		lq.Near = &volcano.NearPoint{Lat: lat, Lon: lon, RadiusM: radiusKm * 1000}
		lq.AfterDistanceM = cursor.DistanceM
	}

	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()

	results, err := volcano.List(ctx, s.DB, lq)
	if err != nil {
		if errors.Is(err, volcano.ErrInvalidQuery) {
			WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter, err.Error(), "")
			return
		}
		WriteInternal(w, r, err)
		return
	}

	data := make([]VolcanoDTO, 0, len(results))
	for _, v := range results {
		data = append(data, VolcanoDTO{
			ID:               v.ID,
			Name:             v.Name,
			Country:          v.Country,
			Latitude:         v.Latitude,
			Longitude:        v.Longitude,
			ElevationM:       v.ElevationM,
			Status:           v.Status,
			SourceRef:        v.SourceRef,
			AbsentFromSource: v.AbsentFromSourceAt != nil,
			DistanceM:        v.DistanceM,
		})
	}

	meta := Meta{Count: len(data)}
	// A full page means there may be more; an empty or short page is the end.
	if len(results) == limit {
		last := results[len(results)-1]
		meta.NextCursor = Cursor{ID: last.ID, DistanceM: last.DistanceM}.Encode()
	}

	// The catalog is observed data, not derived, so no disclaimer applies —
	// but the source's attribution does.
	WriteData(w, r, data, meta, false, s.catalogAttribution(ctx)...)
}

func (s *Server) handleListObservations(w http.ResponseWriter, r *http.Request) {
	limit, err := PageSize(r)
	if err != nil {
		WriteParamError(w, r, err)
		return
	}

	q := observation.Query{Limit: limit}

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
		q.ObservedFrom = &from
	}
	if hasTo {
		q.ObservedTo = &to
	}

	switch r.URL.Query().Get("provenance") {
	case "", "real":
		// Defaulting to real-only is deliberate: an omitted filter must never
		// mix synthetic records into what a caller reads as observations.
		q.Provenance = observation.ProvenanceRealOnly
	case "synthetic":
		q.Provenance = observation.ProvenanceSyntheticOnly
	case "all":
		q.Provenance = observation.ProvenanceAll
	default:
		WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter,
			`Valor inválido. Use "real", "synthetic" ou "all".`, "provenance")
		return
	}

	if id, err := intParam(r, "volcano_id", 0); err != nil {
		WriteParamError(w, r, err)
		return
	} else if id > 0 {
		v := int64(id)
		q.VolcanoID = &v
	}

	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()

	obs, err := observation.List(ctx, s.DB, q)
	if err != nil {
		WriteInternal(w, r, err)
		return
	}

	data := make([]ObservationDTO, 0, len(obs))
	for _, o := range obs {
		data = append(data, ObservationDTO{
			ID:          o.ID,
			VolcanoID:   o.VolcanoID,
			Kind:        o.Kind,
			ObservedAt:  o.ObservedAt,
			IngestedAt:  o.IngestedAt,
			Value:       o.Value,
			IsSynthetic: o.IsSynthetic,
			SourceID:    o.SourceID,
		})
	}

	WriteData(w, r, data, Meta{Count: len(data)}, false)
}

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()

	srcs, err := source.List(ctx, s.DB)
	if err != nil {
		WriteInternal(w, r, err)
		return
	}
	WriteData(w, r, srcs, Meta{Count: len(srcs)}, false)
}

// catalogAttribution returns the credit required by the sources whose data the
// catalog serves. A failure here must not fail the request: missing attribution
// is logged, but returning no data at all would be worse.
func (s *Server) catalogAttribution(ctx context.Context) []Attribution {
	srcs, err := source.List(ctx, s.DB)
	if err != nil {
		return nil
	}
	var out []Attribution
	for _, src := range srcs {
		if !src.Enabled {
			continue
		}
		out = append(out, Attribution{
			Source:      src.Name,
			License:     src.License,
			Attribution: src.Attribution,
			URL:         src.BaseURL,
		})
	}
	return out
}
