package usgs

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ParserVersion is stamped onto every record this package produces.
//
// It MUST be bumped whenever the extraction below changes what it puts in
// a field. Without it, a parse bug fixed later leaves no way to tell which
// stored rows were born wrong (§54, design.md D4). It is a fact about the
// data, not a package version.
const ParserVersion = "usgs-geojson/1"

// ErrMalformed reports that the response could not be interpreted as an
// FDSN GeoJSON document. It is the "parse step" failure of the
// ingestao-fontes spec, kept distinct from a single bad record inside an
// otherwise valid response.
var ErrMalformed = errors.New("usgs: parse: response is not a well-formed FDSN GeoJSON document")

// Event is one earthquake as the source described it, after extraction
// but before validation and quality evaluation.
//
// Pointer fields are the ones the source genuinely omits. They are
// pointers rather than zero values because absence and zero are different
// facts and must stay different: a real ci41531128 has magnitude exactly
// 0.0 while omitting nst, dmin and gap entirely.
type Event struct {
	ID   string
	Time time.Time
	// SourceUpdatedAt is the source's own "updated" instant. Recorded,
	// but never trusted as the deduplication key (design.md D3).
	SourceUpdatedAt time.Time

	Latitude  float64
	Longitude float64
	// DepthKm is in kilometres, which is the unit the source publishes on
	// the third coordinate. Named for the unit so no caller has to guess
	// (§21: tudo deverá possuir unidade explícita).
	DepthKm *float64

	Magnitude     *float64
	MagnitudeType string
	Status        string

	RMS            *float64
	AzimuthalGap   *float64
	StationCount   *int
	MinDistanceDeg *float64

	Place string

	// Raw is the event exactly as the source sent it, preserved so any
	// derived value can be reconstructed from its origin (§18).
	Raw json.RawMessage
}

// Response is a parsed FDSN GeoJSON document.
type Response struct {
	// SourceVersion is the service version the source declares for itself
	// (metadata.api), recorded per record so a shape change can be traced
	// to the service edition that produced it.
	SourceVersion string
	// Count is the source's own count, kept to cross-check against the
	// number of features actually delivered.
	Count  int
	Events []Event
}

// wire mirrors the FDSN GeoJSON document. Every numeric field the source
// may omit is a pointer here for the same reason it is one in Event: so
// "absent" survives decoding instead of becoming 0.
type wire struct {
	Type     string `json:"type"`
	Metadata struct {
		API   string `json:"api"`
		Count int    `json:"count"`
	} `json:"metadata"`
	Features []json.RawMessage `json:"features"`
}

type wireFeature struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Properties struct {
		Mag     *float64 `json:"mag"`
		MagType string   `json:"magType"`
		Time    *int64   `json:"time"`
		Updated *int64   `json:"updated"`
		Status  string   `json:"status"`
		Place   string   `json:"place"`
		NST     *int     `json:"nst"`
		DMin    *float64 `json:"dmin"`
		RMS     *float64 `json:"rms"`
		Gap     *float64 `json:"gap"`
	} `json:"properties"`
	Geometry struct {
		Type        string     `json:"type"`
		Coordinates []*float64 `json:"coordinates"`
	} `json:"geometry"`
}

// RecordError is a single event that could not be extracted from an
// otherwise valid response. It does not abort the response: the remaining
// events are still returned, and the caller counts these as rejected
// (ingestao-fontes spec, "Registro individual é inválido").
type RecordError struct {
	Index int
	ID    string
	Err   error
}

func (e RecordError) Error() string {
	return fmt.Sprintf("usgs: parse: feature %d (id %q): %v", e.Index, e.ID, e.Err)
}

func (e RecordError) Unwrap() error { return e.Err }

// Parse extracts events from an FDSN GeoJSON response body.
//
// A body that is not a FeatureCollection at all fails outright with
// ErrMalformed — the whole response is untrustworthy, and importing part
// of it would be importing a guess. A single feature that cannot be
// extracted is returned in the second result instead, leaving the rest
// usable.
func Parse(body []byte) (Response, []RecordError, error) {
	var w wire
	if err := json.Unmarshal(body, &w); err != nil {
		return Response{}, nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if w.Type != "FeatureCollection" {
		return Response{}, nil, fmt.Errorf("%w: expected a FeatureCollection, got type %q", ErrMalformed, w.Type)
	}
	if w.Features == nil {
		return Response{}, nil, fmt.Errorf(`%w: document has no "features" array`, ErrMalformed)
	}

	resp := Response{SourceVersion: w.Metadata.API, Count: w.Metadata.Count}
	var recErrs []RecordError

	for i, rawFeature := range w.Features {
		ev, err := parseFeature(rawFeature)
		if err != nil {
			var id string
			var probe struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(rawFeature, &probe)
			id = probe.ID
			recErrs = append(recErrs, RecordError{Index: i, ID: id, Err: err})
			continue
		}
		resp.Events = append(resp.Events, ev)
	}

	return resp, recErrs, nil
}

func parseFeature(raw json.RawMessage) (Event, error) {
	var f wireFeature
	if err := json.Unmarshal(raw, &f); err != nil {
		return Event{}, fmt.Errorf("feature is not decodable: %w", err)
	}
	if f.ID == "" {
		return Event{}, errors.New(`feature has no "id": the source's own identifier is the natural key and cannot be invented`)
	}
	if f.Properties.Time == nil {
		return Event{}, errors.New(`feature has no "time": an event without an occurrence instant cannot be placed on a timeline`)
	}
	if len(f.Geometry.Coordinates) < 2 || f.Geometry.Coordinates[0] == nil || f.Geometry.Coordinates[1] == nil {
		return Event{}, errors.New("feature has no usable [longitude, latitude] coordinates")
	}

	ev := Event{
		ID: f.ID,
		// The source publishes epoch milliseconds. UTC always (§21).
		Time:           time.UnixMilli(*f.Properties.Time).UTC(),
		Longitude:      *f.Geometry.Coordinates[0],
		Latitude:       *f.Geometry.Coordinates[1],
		Magnitude:      f.Properties.Mag,
		MagnitudeType:  f.Properties.MagType,
		Status:         f.Properties.Status,
		RMS:            f.Properties.RMS,
		AzimuthalGap:   f.Properties.Gap,
		StationCount:   f.Properties.NST,
		MinDistanceDeg: f.Properties.DMin,
		Place:          f.Properties.Place,
		Raw:            append(json.RawMessage(nil), raw...),
	}
	if len(f.Geometry.Coordinates) >= 3 {
		ev.DepthKm = f.Geometry.Coordinates[2]
	}
	if f.Properties.Updated != nil {
		ev.SourceUpdatedAt = time.UnixMilli(*f.Properties.Updated).UTC()
	}
	return ev, nil
}
