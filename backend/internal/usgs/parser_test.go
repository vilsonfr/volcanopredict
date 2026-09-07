package usgs_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vilsonfr/volcanopredict/backend/internal/usgs"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "usgs", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return b
}

// --- 4.1: parse over the real response ----------------------------------

func TestParse_RealResponse(t *testing.T) {
	resp, recErrs, err := usgs.Parse(fixture(t, "query_m5_2026-09-01_03.json"))
	if err != nil {
		t.Fatalf("the real captured response must parse: %v", err)
	}
	if len(recErrs) != 0 {
		t.Fatalf("no record in a real response should be rejected, got %v", recErrs)
	}
	if len(resp.Events) != 15 {
		t.Fatalf("expected the 15 events the source reported, got %d", len(resp.Events))
	}
	if resp.Count != 15 {
		t.Fatalf("expected the source's own count to be read, got %d", resp.Count)
	}
	if resp.SourceVersion != "2.7.0" {
		t.Fatalf("expected the source service version to be recorded, got %q", resp.SourceVersion)
	}

	// Spot-check the first event against the captured bytes.
	ev := resp.Events[0]
	if ev.ID != "us7000tdrv" {
		t.Fatalf("expected the source's own identifier, got %q", ev.ID)
	}
	if ev.Magnitude == nil || *ev.Magnitude != 6.2 {
		t.Fatalf("magnitude: got %v, want 6.2", ev.Magnitude)
	}
	if ev.MagnitudeType != "mww" {
		t.Fatalf("magnitude type must travel with the number, got %q", ev.MagnitudeType)
	}
	if ev.Status != "reviewed" {
		t.Fatalf("status: got %q, want reviewed", ev.Status)
	}
	if ev.DepthKm == nil || *ev.DepthKm != 125.905 {
		t.Fatalf("depth: got %v, want 125.905", ev.DepthKm)
	}
	if ev.Latitude != -56.2062 || ev.Longitude != -27.9085 {
		t.Fatalf("coordinates: got (%v, %v), want (-56.2062, -27.9085)", ev.Latitude, ev.Longitude)
	}
	if ev.StationCount == nil || *ev.StationCount != 64 {
		t.Fatalf("station count: got %v, want 64", ev.StationCount)
	}
	if ev.AzimuthalGap == nil || *ev.AzimuthalGap != 46 {
		t.Fatalf("azimuthal gap: got %v, want 46", ev.AzimuthalGap)
	}

	// time is epoch milliseconds at the source; it must land in UTC.
	wantTime := time.UnixMilli(1788392817617).UTC()
	if !ev.Time.Equal(wantTime) {
		t.Fatalf("occurrence: got %s, want %s", ev.Time, wantTime)
	}
	if ev.Time.Location() != time.UTC {
		t.Fatalf("timestamps must be normalized to UTC, got %s", ev.Time.Location())
	}
	if ev.SourceUpdatedAt.IsZero() {
		t.Fatal("the source's own updated instant must be recorded")
	}
}

// --- 4.1: a malformed document fails at the parse step ------------------

func TestParse_MalformedFailsNamingTheStep(t *testing.T) {
	for _, name := range []string{"malformed_not_a_collection.json", "malformed_truncated.json"} {
		t.Run(name, func(t *testing.T) {
			_, _, err := usgs.Parse(fixture(t, name))
			if err == nil {
				t.Fatal("expected a malformed document to fail outright, not to import partially")
			}
			if !errors.Is(err, usgs.ErrMalformed) {
				t.Fatalf("expected the failure to name the parse step (ErrMalformed), got %v", err)
			}
		})
	}
}

// A valid document whose individual records are broken must not take the
// good ones down with them.
func TestParse_BadRecordsRejectedIndividually(t *testing.T) {
	resp, recErrs, err := usgs.Parse(fixture(t, "query_with_bad_records.json"))
	if err != nil {
		t.Fatalf("a structurally valid document must not abort: %v", err)
	}
	if len(resp.Events) != 2 {
		t.Fatalf("expected the 2 good records to survive, got %d", len(resp.Events))
	}
	if len(recErrs) != 2 {
		t.Fatalf("expected the 2 broken records to be reported, got %d: %v", len(recErrs), recErrs)
	}
}

// --- 4.2: absent is not zero, in both directions ------------------------

// ci41531128 is a real event with magnitude exactly 0.0 that omits nst,
// dmin and gap entirely. It is the case that catches a parser which
// decodes into plain floats: such a parser reads the real 0.0 magnitude
// and the three missing indicators identically.
func TestParse_AbsentIsNotZero(t *testing.T) {
	resp, recErrs, err := usgs.Parse(fixture(t, "query_absent_vs_zero.json"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(recErrs) != 0 || len(resp.Events) != 1 {
		t.Fatalf("expected exactly one clean event, got %d events and %v", len(resp.Events), recErrs)
	}
	ev := resp.Events[0]
	if ev.ID != "ci41531128" {
		t.Fatalf("wrong fixture event: %q", ev.ID)
	}

	// Present and zero.
	if ev.Magnitude == nil {
		t.Fatal("magnitude 0.0 is a measured value, not an absent one: it must not decode to absent")
	}
	if *ev.Magnitude != 0 {
		t.Fatalf("magnitude: got %v, want 0", *ev.Magnitude)
	}

	// Absent, and must not have become zero.
	if ev.StationCount != nil {
		t.Fatalf("nst is absent in the source; it must not become %v", *ev.StationCount)
	}
	if ev.MinDistanceDeg != nil {
		t.Fatalf("dmin is absent in the source; it must not become %v", *ev.MinDistanceDeg)
	}
	if ev.AzimuthalGap != nil {
		t.Fatalf("gap is absent in the source; it must not become %v", *ev.AzimuthalGap)
	}
}

// --- 4.4: the normalized record can be reconciled with the raw payload --

func TestParse_RawPayloadReconstructsTheNormalizedFields(t *testing.T) {
	resp, _, err := usgs.Parse(fixture(t, "query_m5_2026-09-01_03.json"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	for _, ev := range resp.Events {
		if len(ev.Raw) == 0 {
			t.Fatalf("event %s carries no raw payload: a derived value must be reconstructable from its origin", ev.ID)
		}

		var raw struct {
			ID         string `json:"id"`
			Properties struct {
				Mag     *float64 `json:"mag"`
				MagType string   `json:"magType"`
				Time    int64    `json:"time"`
				Status  string   `json:"status"`
			} `json:"properties"`
			Geometry struct {
				Coordinates []*float64 `json:"coordinates"`
			} `json:"geometry"`
		}
		if err := json.Unmarshal(ev.Raw, &raw); err != nil {
			t.Fatalf("event %s: raw payload is not decodable: %v", ev.ID, err)
		}

		if raw.ID != ev.ID {
			t.Fatalf("identifier drifted from the source: raw %q vs normalized %q", raw.ID, ev.ID)
		}
		if !time.UnixMilli(raw.Properties.Time).UTC().Equal(ev.Time) {
			t.Fatalf("event %s: occurrence cannot be rederived from raw", ev.ID)
		}
		if raw.Properties.MagType != ev.MagnitudeType || raw.Properties.Status != ev.Status {
			t.Fatalf("event %s: magnitude type or status drifted from raw", ev.ID)
		}
		if (raw.Properties.Mag == nil) != (ev.Magnitude == nil) {
			t.Fatalf("event %s: magnitude presence drifted from raw", ev.ID)
		}
		if raw.Properties.Mag != nil && *raw.Properties.Mag != *ev.Magnitude {
			t.Fatalf("event %s: magnitude drifted from raw", ev.ID)
		}
		if *raw.Geometry.Coordinates[1] != ev.Latitude || *raw.Geometry.Coordinates[0] != ev.Longitude {
			t.Fatalf("event %s: coordinates drifted from raw", ev.ID)
		}
	}
}

// --- 4.5: the parser version is a fact about the data -------------------

func TestParserVersion_IsStamped(t *testing.T) {
	if usgs.ParserVersion == "" {
		t.Fatal("the parser version must be non-empty: it is what tells which rows were born from which extraction")
	}
}
