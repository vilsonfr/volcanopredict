package gvp_test

import (
	"os"
	"strings"
	"testing"

	"github.com/vilsonfr/volcanopredict/backend/internal/gvp"
)

// Fixtures used here are synthetic test data written by hand — see
// backend/testdata/README.md. They are never a real GVP snapshot.

func TestParse_FixtureSample(t *testing.T) {
	f, err := os.Open("../../testdata/gvp_holocene_fixture.csv")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	rows, rowErrs, err := gvp.Parse(f)
	if err != nil {
		t.Fatalf("Parse returned structural error on a well-formed fixture: %v", err)
	}
	if len(rowErrs) != 0 {
		t.Fatalf("expected no row-level parse errors, got %v", rowErrs)
	}
	if len(rows) != 4 {
		t.Fatalf("expected 4 parsed rows, got %d", len(rows))
	}

	if rows[0].SourceRef != "FIXTURE-0001" || rows[0].Name != "Testonia Peak" {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[2].Latitude != 200.0 {
		t.Fatalf("expected row 3 to carry the out-of-range latitude unmodified for the importer to reject, got %v", rows[2].Latitude)
	}
	if rows[3].ElevationM != nil {
		t.Fatalf("expected row 4's empty elevation to parse as nil, got %v", *rows[3].ElevationM)
	}
}

func TestParse_MalformedMissingColumn(t *testing.T) {
	f, err := os.Open("../../testdata/gvp_holocene_malformed.csv")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	_, _, err = gvp.Parse(f)
	if err == nil {
		t.Fatal("expected Parse to fail explicitly on a file missing a required column")
	}
	var malformed gvp.ErrMalformed
	if !isErrMalformed(err, &malformed) {
		t.Fatalf("expected ErrMalformed, got %T: %v", err, err)
	}
}

func isErrMalformed(err error, target *gvp.ErrMalformed) bool {
	m, ok := err.(gvp.ErrMalformed)
	if ok {
		*target = m
	}
	return ok
}

func TestParse_EmptyFile(t *testing.T) {
	_, _, err := gvp.Parse(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected Parse to fail on an empty file")
	}
}
