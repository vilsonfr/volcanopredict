package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/vilsonfr/volcanopredict/backend/internal/config"
	"github.com/vilsonfr/volcanopredict/backend/internal/db"
	"github.com/vilsonfr/volcanopredict/backend/internal/gvp"
	"github.com/vilsonfr/volcanopredict/backend/internal/source"
)

const (
	defaultSnapshotDir = "data/gvp"
	gvpSourceName      = "Smithsonian Global Volcanism Program"
)

// runImportCatalog imports the versioned GVP snapshot into the volcano
// catalog. It is a subcommand rather than a startup step on purpose: updating
// the catalog is a deliberate act tied to swapping the snapshot file, not
// something that should happen on every boot (design.md D4).
func runImportCatalog(args []string) int {
	fs := flag.NewFlagSet("import-catalog", flag.ContinueOnError)
	snapshot := fs.String("snapshot", "", "path to the GVP SpreadsheetML snapshot (default: the .xls in "+defaultSnapshotDir+")")
	manifest := fs.String("manifest", defaultSnapshotDir+"/MANIFEST.md", "path to the snapshot manifest holding the expected SHA-256")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	path := *snapshot
	if path == "" {
		found, err := findSnapshot(defaultSnapshotDir)
		if err != nil {
			log.Printf("import-catalog: %v", err)
			return 1
		}
		path = found
	}

	// Provenance before data: a snapshot whose bytes no longer match its
	// manifest is not the catalog the manifest describes, and importing it
	// would silently break reproducibility.
	if err := gvp.VerifyChecksum(path, *manifest); err != nil {
		log.Printf("import-catalog: %v", err)
		return 1
	}
	log.Printf("import-catalog: snapshot checksum verified: %s", path)

	cfg, err := config.Load()
	if err != nil {
		log.Printf("import-catalog: %v", err)
		return 1
	}

	ctx := context.Background()
	if err := db.WaitReady(ctx, cfg.DatabaseURL, cfg.DBConnectTimeout); err != nil {
		log.Printf("import-catalog: %v", err)
		return 1
	}
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Printf("import-catalog: %v", err)
		return 1
	}
	defer pool.Close()

	// The source must already be registered and enabled: no external data
	// enters the system from an unregistered source (registro-de-fontes spec).
	src, err := source.GetEnabledByName(ctx, pool, gvpSourceName)
	if err != nil {
		log.Printf("import-catalog: %v", err)
		return 1
	}

	f, err := os.Open(path)
	if err != nil {
		log.Printf("import-catalog: %v", err)
		return 1
	}
	defer f.Close()

	report, err := gvp.Import(ctx, pool, src.ID, f)
	if err != nil {
		log.Printf("import-catalog: %v", err)
		return 1
	}

	fmt.Printf("GVP catalog version %s\n", report.Version)
	fmt.Printf("  inserted:      %d\n", report.Inserted)
	fmt.Printf("  updated:       %d\n", report.Updated)
	fmt.Printf("  unchanged:     %d\n", report.Unchanged)
	fmt.Printf("  marked absent: %d\n", report.MarkedAbsent)
	fmt.Printf("  rejected:      %d\n", len(report.Rejected))
	for _, r := range report.Rejected {
		fmt.Printf("    line %d source_ref=%s name=%q: %v\n", r.Line, r.SourceRef, r.Name, r.Err)
	}
	return 0
}

func findSnapshot(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("cannot read snapshot directory %s: %w", dir, err)
	}
	var found []string
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 4 && e.Name()[len(e.Name())-4:] == ".xls" {
			found = append(found, dir+"/"+e.Name())
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no .xls snapshot found in %s", dir)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("found %d snapshots in %s; pass -snapshot to choose one", len(found), dir)
	}
}
