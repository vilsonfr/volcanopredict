// Package gvp parses the Smithsonian Global Volcanism Program (GVP)
// Holocene Volcano List snapshot and imports it into the volcano catalog
// (design.md D4/D5, catalogo-vulcoes spec).
//
// Format note: the real snapshot (backend/data/gvp/GVP_Volcano_List_Holocene_*.xls,
// see backend/data/gvp/MANIFEST.md for provenance and checksum) is NOT a
// CSV, despite an earlier iteration of this package assuming one. It is
// SpreadsheetML (the Excel 2003 XML workbook format,
// urn:schemas-microsoft-com:office:spreadsheet), saved with a `.xls`
// extension. The file is also not well-formed XML: it contains raw `<`
// characters inside text content (e.g. "Rift zone / Oceanic crust (< 15
// km)"), which a strict XML parser rejects with "not well-formed (invalid
// token)". This package sanitizes those occurrences before decoding — see
// sanitizeRawLessThan.
package gvp

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// expectedHeader is the exact, ordered set of columns this parser accepts
// for the Holocene Volcano List export. The header is validated exactly
// (both names and order) — this parser does not tolerantly re-map columns
// by name if the source's layout changes; a changed header is a
// structural failure (task 5.4: "falhando explicitamente diante de
// formato inesperado", explicitly not a name-based tolerant mapping).
var expectedHeader = []string{
	"Volcano Number",
	"Volcano Name",
	"Country",
	"Volcanic Region Group",
	"Volcanic Region",
	"Volcano Landform",
	"Primary Volcano Type",
	"Activity Evidence",
	"Last Known Eruption",
	"Latitude",
	"Longitude",
	"Elevation (m)",
	"Tectonic Setting",
	"Dominant Rock Type",
}

// Column positions in expectedHeader. Fixed positions are safe here only
// because the header is validated exactly against expectedHeader before
// any data row is parsed.
const (
	colVolcanoNumber = iota
	colVolcanoName
	colCountry
	colVolcanicRegionGroup
	colVolcanicRegion
	colVolcanoLandform
	colPrimaryVolcanoType
	colActivityEvidence
	colLastKnownEruption
	colLatitude
	colLongitude
	colElevationM
	colTectonicSetting
	colDominantRockType
)

// Row is a single parsed data row, with fields still as strings/floats but
// not yet validated against catalog business rules (e.g. geographic
// range) — that validation is the importer's job (task 5.6), so it can be
// applied uniformly and reported per-record.
type Row struct {
	// Line is the 1-based row number within the SpreadsheetML <Table>
	// (row 1 is the GVP metadata banner, row 2 is the header, data starts
	// at row 3), for error messages and logs that must identify the
	// offending record.
	Line int

	SourceRef string // Volcano Number
	Name      string // Volcano Name
	Country   string

	// Status carries "Activity Evidence" (e.g. "Eruption Observed",
	// "Eruption Dated", "Evidence Uncertain"). The real GVP export has no
	// column named "Status" — this is the closest available analog to the
	// free-text status field the catalog schema (migrations/001_init.sql)
	// already has, and is used as such rather than adding a new column.
	Status string

	VolcanicRegionGroup string
	VolcanicRegion      string
	VolcanoLandform     string
	PrimaryVolcanoType  string
	LastKnownEruption   string
	TectonicSetting     string
	DominantRockType    string

	Latitude   float64
	Longitude  float64
	ElevationM *float64
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

// ErrMalformed wraps structural failures: missing/renamed/reordered
// headers, an empty file, or a file that is not valid SpreadsheetML at
// all. Structural failures always abort the whole parse — there's nothing
// safe to salvage from a file whose shape doesn't match what the importer
// expects.
type ErrMalformed struct {
	Reason string
}

func (e ErrMalformed) Error() string {
	return fmt.Sprintf("gvp: malformed snapshot: %s", e.Reason)
}

// rawLessThan matches a raw `<` that cannot possibly be starting a real
// XML tag — real tags open with a letter, `/`, `?`, or `!`. The known
// occurrences in the GVP export (see MANIFEST.md) are things like
// "(< 15 km)", i.e. `<` followed by a space or a digit.
var rawLessThan = regexp.MustCompile(`<([ 0-9])`)

// sanitizeRawLessThan escapes raw `<` characters that are not well-formed
// XML markup, without touching legitimate tags (which never start with a
// space or digit).
func sanitizeRawLessThan(data []byte) []byte {
	return rawLessThan.ReplaceAll(data, []byte("&lt;$1"))
}

// versionPattern extracts a dotted version number (e.g. "5.4.0") from the
// GVP metadata banner's free text, e.g.
// "Global Volcanism Program - Volcanoes of the World 5.4.0".
var versionPattern = regexp.MustCompile(`\d+(?:\.\d+)+`)

const ssNamespace = "urn:schemas-microsoft-com:office:spreadsheet"

// Parse reads a GVP Holocene Volcano List SpreadsheetML (.xls)
// snapshot. It returns the database version string extracted from the
// file's own metadata banner (never hardcoded), the successfully parsed
// rows, a list of per-row errors for rows that were skipped, and a non-nil
// error only for structural failures that make the entire file unusable.
func Parse(r io.Reader) (version string, rows []Row, rowErrs []RowError, err error) {
	raw, readErr := io.ReadAll(r)
	if readErr != nil {
		return "", nil, nil, ErrMalformed{Reason: fmt.Sprintf("failed to read input: %v", readErr)}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil, nil, ErrMalformed{Reason: "file is empty"}
	}

	sanitized := sanitizeRawLessThan(raw)

	tableRows, decodeErr := decodeSpreadsheetRows(sanitized)
	if decodeErr != nil {
		return "", nil, nil, ErrMalformed{Reason: fmt.Sprintf("failed to decode SpreadsheetML: %v", decodeErr)}
	}

	if len(tableRows) < 2 {
		return "", nil, nil, ErrMalformed{Reason: fmt.Sprintf("expected at least a metadata row and a header row, got %d row(s)", len(tableRows))}
	}

	version = extractVersion(tableRows[0])
	if version == "" {
		return "", nil, nil, ErrMalformed{Reason: fmt.Sprintf("could not extract a database version from the metadata row: %v", tableRows[0])}
	}

	header := tableRows[1]
	if err := validateHeader(header); err != nil {
		return "", nil, nil, err
	}

	for i := 2; i < len(tableRows); i++ {
		line := i + 1 // 1-based row number within the table
		record := padTo(tableRows[i], len(expectedHeader))
		row, err := parseRow(record, line)
		if err != nil {
			rowErrs = append(rowErrs, RowError{Line: line, Err: err})
			continue
		}
		rows = append(rows, row)
	}

	return version, rows, rowErrs, nil
}

func extractVersion(metadataRow []string) string {
	for _, cell := range metadataRow {
		if m := versionPattern.FindString(cell); m != "" {
			return m
		}
	}
	return ""
}

func validateHeader(header []string) error {
	header = padTo(header, len(expectedHeader))
	if len(header) != len(expectedHeader) {
		return ErrMalformed{Reason: fmt.Sprintf("expected %d columns, got %d: %v", len(expectedHeader), len(header), header)}
	}
	for i, want := range expectedHeader {
		got := strings.TrimSpace(header[i])
		if got != want {
			return ErrMalformed{Reason: fmt.Sprintf("unexpected header at column %d: expected %q, got %q (full header: %v)", i+1, want, got, header)}
		}
	}
	return nil
}

func padTo(record []string, n int) []string {
	if len(record) >= n {
		return record
	}
	out := make([]string, n)
	copy(out, record)
	return out
}

// decodeSpreadsheetRows walks the SpreadsheetML token stream and returns
// each <Row> as a slice of cell text, honoring ss:Index on <Cell> to
// reposition columns when the source skips empty cells (a valid
// SpreadsheetML optimization that would otherwise silently misalign
// columns).
func decodeSpreadsheetRows(sanitized []byte) ([][]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(sanitized))

	var rows [][]string
	var currentRow []string
	nextIndex := 0 // 0-based index the next <Cell> without ss:Index lands at

	inData := false
	var dataBuf strings.Builder

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space != ssNamespace {
				continue
			}
			switch t.Name.Local {
			case "Row":
				currentRow = nil
				nextIndex = 0
			case "Cell":
				colIndex := nextIndex
				if idxAttr := findAttr(t.Attr, "Index"); idxAttr != "" {
					if parsed, err := strconv.Atoi(idxAttr); err == nil {
						colIndex = parsed - 1 // ss:Index is 1-based
					}
				}
				if colIndex >= len(currentRow) {
					grown := make([]string, colIndex+1)
					copy(grown, currentRow)
					currentRow = grown
				}
				nextIndex = colIndex + 1
			case "Data":
				inData = true
				dataBuf.Reset()
			}
		case xml.CharData:
			if inData {
				dataBuf.Write(t)
			}
		case xml.EndElement:
			if t.Name.Space != ssNamespace {
				continue
			}
			switch t.Name.Local {
			case "Data":
				if inData && nextIndex > 0 {
					currentRow[nextIndex-1] = dataBuf.String()
				}
				inData = false
			case "Row":
				rows = append(rows, currentRow)
				currentRow = nil
			}
		}
	}

	return rows, nil
}

func findAttr(attrs []xml.Attr, local string) string {
	for _, a := range attrs {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func parseRow(record []string, line int) (Row, error) {
	num := strings.TrimSpace(record[colVolcanoNumber])
	if num == "" {
		return Row{}, fmt.Errorf("missing Volcano Number")
	}
	name := strings.TrimSpace(record[colVolcanoName])
	if name == "" {
		return Row{}, fmt.Errorf("missing Volcano Name")
	}

	latRaw := strings.TrimSpace(record[colLatitude])
	if latRaw == "" {
		return Row{}, fmt.Errorf("missing Latitude")
	}
	lat, err := strconv.ParseFloat(latRaw, 64)
	if err != nil {
		return Row{}, fmt.Errorf("unparseable Latitude %q: %w", latRaw, err)
	}

	lonRaw := strings.TrimSpace(record[colLongitude])
	if lonRaw == "" {
		return Row{}, fmt.Errorf("missing Longitude")
	}
	lon, err := strconv.ParseFloat(lonRaw, 64)
	if err != nil {
		return Row{}, fmt.Errorf("unparseable Longitude %q: %w", lonRaw, err)
	}

	var elevation *float64
	if elevRaw := strings.TrimSpace(record[colElevationM]); elevRaw != "" {
		e, err := strconv.ParseFloat(elevRaw, 64)
		if err != nil {
			return Row{}, fmt.Errorf("unparseable Elevation (m) %q: %w", elevRaw, err)
		}
		elevation = &e
	}

	return Row{
		Line:                line,
		SourceRef:           num,
		Name:                name,
		Country:             strings.TrimSpace(record[colCountry]),
		VolcanicRegionGroup: strings.TrimSpace(record[colVolcanicRegionGroup]),
		VolcanicRegion:      strings.TrimSpace(record[colVolcanicRegion]),
		VolcanoLandform:     strings.TrimSpace(record[colVolcanoLandform]),
		PrimaryVolcanoType:  strings.TrimSpace(record[colPrimaryVolcanoType]),
		Status:              strings.TrimSpace(record[colActivityEvidence]),
		LastKnownEruption:   strings.TrimSpace(record[colLastKnownEruption]),
		Latitude:            lat,
		Longitude:           lon,
		ElevationM:          elevation,
		TectonicSetting:     strings.TrimSpace(record[colTectonicSetting]),
		DominantRockType:    strings.TrimSpace(record[colDominantRockType]),
	}, nil
}
