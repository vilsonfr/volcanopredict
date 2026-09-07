// Package gvp parses the Smithsonian Global Volcanism Program (GVP)
// Holocene Volcano List snapshot and imports it into the volcano catalog
// (design.md D4/D5, catalogo-vulcoes spec).
//
// IMPORTANT — format caveat: as of this writing the real snapshot could
// not be downloaded (volcano.si.edu returns HTTP 403 from a Cloudflare
// bot check for every attempt made, including with a browser User-Agent;
// see docs/DATA_SOURCES.md for the exact URLs and status codes tried).
// The column layout this parser expects (a CSV with the header names
// below) is therefore UNVERIFIED against the real export and may need
// adjustment once an actual snapshot is obtained. This package is tested
// only against a hand-written synthetic fixture
// (backend/testdata/gvp_holocene_fixture.csv), never against real GVP
// data — no real snapshot exists in this repository.
package gvp

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// requiredHeaders are the column names this parser expects, in any order.
// A file missing any of these fails structurally (task 5.4: "falhando
// explicitamente diante de formato inesperado") rather than importing a
// partial, silently-wrong shape.
var requiredHeaders = []string{
	"Volcano Number",
	"Volcano Name",
	"Country",
	"Latitude",
	"Longitude",
	"Elevation (m)",
	"Status",
}

// Row is a single parsed data row, with fields still as strings/floats
// but not yet validated against catalog business rules (e.g. geographic
// range) — that validation is the importer's job (task 5.6), so it can be
// applied uniformly and reported per-record.
type Row struct {
	// Line is the 1-based line number in the source file, for error
	// messages and logs that must identify the offending record.
	Line       int
	SourceRef  string
	Name       string
	Country    string
	Latitude   float64
	Longitude  float64
	ElevationM *float64
	Status     string
}

// RowError describes a single row that could not be parsed into a Row.
// Rows with a RowError are excluded from the returned []Row but do not
// abort parsing of the rest of the file.
type RowError struct {
	Line int
	Err  error
}

func (e RowError) Error() string {
	return fmt.Sprintf("gvp: row %d: %v", e.Line, e.Err)
}

// ErrMalformed wraps structural failures: missing/renamed headers, an
// empty file, or a file that is not valid CSV at all. Structural failures
// always abort the whole parse — there's nothing safe to salvage from a
// file whose shape doesn't match what the importer expects.
type ErrMalformed struct {
	Reason string
}

func (e ErrMalformed) Error() string {
	return fmt.Sprintf("gvp: malformed snapshot: %s", e.Reason)
}

// Parse reads a GVP Holocene Volcano List CSV snapshot. It returns the
// successfully parsed rows, a list of per-row errors for rows that were
// skipped, and a non-nil error only for structural failures that make the
// entire file unusable.
func Parse(r io.Reader) ([]Row, []RowError, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // validated explicitly below, with a clearer error

	header, err := cr.Read()
	if err == io.EOF {
		return nil, nil, ErrMalformed{Reason: "file is empty, no header row found"}
	}
	if err != nil {
		return nil, nil, ErrMalformed{Reason: fmt.Sprintf("failed to read header row: %v", err)}
	}

	index := make(map[string]int, len(header))
	for i, h := range header {
		index[strings.TrimSpace(h)] = i
	}
	var missing []string
	for _, want := range requiredHeaders {
		if _, ok := index[want]; !ok {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		return nil, nil, ErrMalformed{Reason: fmt.Sprintf("missing required column(s): %s", strings.Join(missing, ", "))}
	}

	var rows []Row
	var rowErrs []RowError
	line := 1 // header was line 1
	for {
		line++
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, ErrMalformed{Reason: fmt.Sprintf("failed to read line %d: %v", line, err)}
		}

		row, err := parseRow(record, index, line)
		if err != nil {
			rowErrs = append(rowErrs, RowError{Line: line, Err: err})
			continue
		}
		rows = append(rows, row)
	}

	return rows, rowErrs, nil
}

func field(record []string, index map[string]int, name string) (string, bool) {
	i, ok := index[name]
	if !ok || i >= len(record) {
		return "", false
	}
	return strings.TrimSpace(record[i]), true
}

func parseRow(record []string, index map[string]int, line int) (Row, error) {
	num, ok := field(record, index, "Volcano Number")
	if !ok || num == "" {
		return Row{}, fmt.Errorf("missing Volcano Number")
	}
	name, ok := field(record, index, "Volcano Name")
	if !ok || name == "" {
		return Row{}, fmt.Errorf("missing Volcano Name")
	}
	country, _ := field(record, index, "Country")

	latRaw, ok := field(record, index, "Latitude")
	if !ok || latRaw == "" {
		return Row{}, fmt.Errorf("missing Latitude")
	}
	lat, err := strconv.ParseFloat(latRaw, 64)
	if err != nil {
		return Row{}, fmt.Errorf("unparseable Latitude %q: %w", latRaw, err)
	}

	lonRaw, ok := field(record, index, "Longitude")
	if !ok || lonRaw == "" {
		return Row{}, fmt.Errorf("missing Longitude")
	}
	lon, err := strconv.ParseFloat(lonRaw, 64)
	if err != nil {
		return Row{}, fmt.Errorf("unparseable Longitude %q: %w", lonRaw, err)
	}

	var elevation *float64
	if elevRaw, ok := field(record, index, "Elevation (m)"); ok && elevRaw != "" {
		e, err := strconv.ParseFloat(elevRaw, 64)
		if err != nil {
			return Row{}, fmt.Errorf("unparseable Elevation (m) %q: %w", elevRaw, err)
		}
		elevation = &e
	}

	status, _ := field(record, index, "Status")

	return Row{
		Line:       line,
		SourceRef:  num,
		Name:       name,
		Country:    country,
		Latitude:   lat,
		Longitude:  lon,
		ElevationM: elevation,
		Status:     status,
	}, nil
}
