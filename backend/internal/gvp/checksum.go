package gvp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
)

// manifestSHA256Pattern extracts the SHA-256 documented in the snapshot's
// MANIFEST.md, e.g. the table row "| SHA-256 | `5f53afea...` |".
//
// The manifest is the human-readable record of provenance (version, DOI,
// download date, checksum). Reading the digest from it keeps one source of
// truth instead of duplicating the value in Go code, where the two could
// drift apart unnoticed.
var manifestSHA256Pattern = regexp.MustCompile(`(?i)SHA-256\s*\|\s*` + "`" + `([0-9a-f]{64})` + "`")

// ErrChecksumMismatch reports a snapshot whose bytes no longer match the
// digest its manifest records.
type ErrChecksumMismatch struct {
	FilePath string
	Want     string
	Got      string
}

func (e ErrChecksumMismatch) Error() string {
	return fmt.Sprintf(
		"gvp: snapshot checksum mismatch for %s\n  manifest: %s\n  actual:   %s\n"+
			"The file on disk is not the catalog the manifest describes. Either restore the "+
			"original snapshot, or — if you deliberately replaced it — update MANIFEST.md with "+
			"the new version, download date and checksum.",
		e.FilePath, e.Want, e.Got)
}

// VerifyChecksum checks that the snapshot file still matches the SHA-256
// recorded in its manifest.
//
// This guards reproducibility (design.md D4): a back-test only means something
// if the catalog it ran against is the catalog the manifest claims. A silent
// byte change — a well-meaning edit, a line-ending normalisation, a truncated
// download — would otherwise pass unnoticed and quietly invalidate every
// result derived from it.
func VerifyChecksum(snapshotPath, manifestPath string) error {
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("gvp: cannot read snapshot manifest %s: %w", manifestPath, err)
	}
	m := manifestSHA256Pattern.FindSubmatch(manifest)
	if m == nil {
		return fmt.Errorf("gvp: no SHA-256 recorded in %s", manifestPath)
	}
	want := string(m[1])

	f, err := os.Open(snapshotPath)
	if err != nil {
		return fmt.Errorf("gvp: failed to open snapshot %s: %w", snapshotPath, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("gvp: failed to hash snapshot %s: %w", snapshotPath, err)
	}
	got := hex.EncodeToString(h.Sum(nil))

	if got != want {
		return ErrChecksumMismatch{FilePath: snapshotPath, Want: want, Got: got}
	}
	return nil
}
