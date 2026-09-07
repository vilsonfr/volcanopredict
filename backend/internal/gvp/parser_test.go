package gvp_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilsonfr/volcanopredict/backend/internal/gvp"
)

const (
	fixturePath   = "../../testdata/gvp_holocene_fixture.xls"
	malformedPath = "../../testdata/gvp_holocene_malformed.xls"
	// realSnapshotDir holds the versioned GVP snapshot. Tests that read it
	// assert against the real export, not a fixture we authored.
	realSnapshotDir = "../../data/gvp"
)

type snapshot struct {
	Version string
	Rows    []gvp.Row
	RowErrs []gvp.RowError
}

func parseFile(t *testing.T, path string) (snapshot, error) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	v, rows, rowErrs, err := gvp.Parse(f)
	return snapshot{Version: v, Rows: rows, RowErrs: rowErrs}, err
}

func TestParse_Fixture(t *testing.T) {
	snap, err := parseFile(t, fixturePath)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if snap.Version != "9.9.9" {
		t.Errorf("Version = %q, want 9.9.9 (read from the sheet, not hardcoded)", snap.Version)
	}

	// FIXTURE-0004 has an empty Latitude and must be reported as a row error,
	// not silently dropped and not defaulted to zero.
	if len(snap.RowErrs) != 1 {
		t.Fatalf("RowErrs = %d, want 1 (the row with no latitude); errs=%v", len(snap.RowErrs), snap.RowErrs)
	}
	if !strings.Contains(snap.RowErrs[0].Err.Error(), "Latitude") {
		t.Errorf("row error should name the offending column, got %v", snap.RowErrs[0].Err)
	}

	if len(snap.Rows) != 3 {
		t.Fatalf("Rows = %d, want 3", len(snap.Rows))
	}

	first := snap.Rows[0]
	if first.SourceRef != "FIXTURE-0001" || first.Name != "Testonia Peak" {
		t.Errorf("unexpected first row: %+v", first)
	}
	if first.Latitude != -6.102 || first.Longitude != 105.423 {
		t.Errorf("coordinates = (%v, %v), want (-6.102, 105.423)", first.Latitude, first.Longitude)
	}
	if first.ElevationM == nil || *first.ElevationM != 741 {
		t.Errorf("ElevationM = %v, want 741", first.ElevationM)
	}
	if first.Status != "Eruption Dated" {
		t.Errorf("Status = %q, want %q (from Activity Evidence)", first.Status, "Eruption Dated")
	}
}

// The real GVP export contains raw '<' inside cell text, which makes the file
// invalid XML. If sanitising ever regresses, this row is where it shows up.
func TestParse_HandlesRawLessThanInCellText(t *testing.T) {
	snap, err := parseFile(t, fixturePath)
	if err != nil {
		t.Fatalf("Parse failed on a file containing a raw '<' in cell text: %v", err)
	}
	if len(snap.Rows) == 0 {
		t.Fatal("no rows parsed")
	}
	if snap.Rows[0].Name != "Testonia Peak" {
		t.Errorf("row following the raw '<' cell is misaligned: %+v", snap.Rows[0])
	}
}

// SpreadsheetML omits empty cells and marks the next one with ss:Index.
// Ignoring that attribute shifts every later column left, silently, with no
// parse error — so the misalignment would surface as wrong coordinates.
func TestParse_HonoursSparseCellIndex(t *testing.T) {
	snap, err := parseFile(t, fixturePath)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	var sparse *gvp.Row
	for i := range snap.Rows {
		if snap.Rows[i].SourceRef == "FIXTURE-0002" {
			sparse = &snap.Rows[i]
		}
	}
	if sparse == nil {
		t.Fatal("FIXTURE-0002 (the sparse row) was not parsed")
	}
	if sparse.Latitude != 35.36 || sparse.Longitude != 138.73 {
		t.Errorf("sparse row coordinates = (%v, %v), want (35.36, 138.73); ss:Index was likely ignored",
			sparse.Latitude, sparse.Longitude)
	}
	if sparse.PrimaryVolcanoType != "Shield" {
		t.Errorf("sparse row PrimaryVolcanoType = %q, want %q", sparse.PrimaryVolcanoType, "Shield")
	}
}

func TestParse_MalformedFailsLoudly(t *testing.T) {
	_, err := parseFile(t, malformedPath)
	if err == nil {
		t.Fatal("Parse accepted a snapshot missing required columns; it must fail structurally")
	}
	var malformed gvp.ErrMalformed
	if !errors.As(err, &malformed) {
		t.Fatalf("error = %T (%v), want gvp.ErrMalformed", err, err)
	}
	if !strings.Contains(err.Error(), "column") {
		t.Errorf("error should explain the column mismatch, got %v", err)
	}
}

func TestParse_EmptyInputFails(t *testing.T) {
	_, _, _, err := gvp.Parse(strings.NewReader(""))
	if err == nil {
		t.Fatal("Parse accepted an empty file")
	}
}

// TestParse_RealSnapshot asserts against the actual versioned GVP export.
// A fixture only proves the parser handles the format we imagined; this
// proves it handles the format the Smithsonian actually ships.
func TestParse_RealSnapshot(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join(realSnapshotDir, "*.xls"))
	if err != nil || len(matches) == 0 {
		t.Skipf("no real GVP snapshot in %s", realSnapshotDir)
	}

	snap, err := parseFile(t, matches[0])
	if err != nil {
		t.Fatalf("Parse failed on the real GVP snapshot: %v", err)
	}

	if snap.Version == "" {
		t.Error("no database version read from the real snapshot")
	}
	if len(snap.Rows) < 1000 {
		t.Errorf("parsed %d volcanoes from the real snapshot, expected over 1000", len(snap.Rows))
	}
	if len(snap.RowErrs) > 0 {
		t.Errorf("real snapshot produced %d row errors, expected none: %v", len(snap.RowErrs), snap.RowErrs)
	}

	// Every record must carry the fields the catalog depends on, and
	// coordinates must be inside the valid geographic range.
	for _, row := range snap.Rows {
		if row.SourceRef == "" || row.Name == "" {
			t.Fatalf("row %d has empty identity: %+v", row.Line, row)
		}
		if row.Latitude < -90 || row.Latitude > 90 {
			t.Errorf("row %d (%s) latitude out of range: %v", row.Line, row.Name, row.Latitude)
		}
		if row.Longitude < -180 || row.Longitude > 180 {
			t.Errorf("row %d (%s) longitude out of range: %v", row.Line, row.Name, row.Longitude)
		}
	}

	t.Logf("real snapshot: version %s, %d volcanoes", snap.Version, len(snap.Rows))
}

func TestVerifySnapshot_DetectsMismatch(t *testing.T) {
	dir := t.TempDir()
	snapshot := filepath.Join(dir, "snap.xls")
	manifest := filepath.Join(dir, "MANIFEST.md")

	if err := os.WriteFile(snapshot, []byte("original bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	// SHA-256 of "original bytes".
	const sum = "0d5f2b0d8a3f0b3e6e6e2e1a0d5c2e0a5b9e6c8a3f4d1e7b0c9a2f5d8e3b1c40"
	if err := os.WriteFile(manifest, []byte("| SHA-256 | `"+sum+"` |\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := gvp.VerifyChecksum(snapshot, manifest); err == nil {
		t.Fatal("VerifySnapshot accepted a file whose checksum does not match the manifest")
	}
}

// The shipped snapshot must always match its own manifest; if this fails, the
// file was modified (or line endings were normalised) and reproducibility is
// broken.
func TestVerifySnapshot_RealSnapshotMatchesManifest(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join(realSnapshotDir, "*.xls"))
	if err != nil || len(matches) == 0 {
		t.Skipf("no real GVP snapshot in %s", realSnapshotDir)
	}
	if err := gvp.VerifyChecksum(matches[0], filepath.Join(realSnapshotDir, "MANIFEST.md")); err != nil {
		t.Fatalf("shipped snapshot does not match its manifest: %v", err)
	}
}
