package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/vilsonfr/volcanopredict/backend/internal/config"
	"github.com/vilsonfr/volcanopredict/backend/internal/db"
	"github.com/vilsonfr/volcanopredict/backend/internal/ingestion"
	"github.com/vilsonfr/volcanopredict/backend/internal/source"
	"github.com/vilsonfr/volcanopredict/backend/internal/usgs"
)

// DefaultBackfillWindow is how much history `ingest -backfill` covers when
// no window is given. Short on purpose: enough to exercise the pipeline and
// give the quality engine something to chew on, without a first load that
// takes hours. Widen it with -since when you actually want depth.
const DefaultBackfillWindow = 90 * 24 * time.Hour

// runIngest is the manual entry point to ingestion. The scheduler calls the
// exact same code path (design.md D7), so anything verified here is what
// runs unattended.
func runIngest(args []string) int {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	backfill := fs.Bool("backfill", false, "ingest by occurrence window instead of by what the source changed")
	since := fs.Duration("since", DefaultBackfillWindow, "how far back the backfill window reaches")
	start := fs.String("start", "", "explicit window start (RFC3339); implies a manual window")
	end := fs.String("end", "", "explicit window end (RFC3339); defaults to now")
	pageSize := fs.Int("page-size", 0, "events per request (0 = the service ceiling of 20000)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		log.Printf("ingest: %v", err)
		return 1
	}

	ctx := context.Background()
	if err := db.WaitReady(ctx, cfg.DatabaseURL, cfg.DBConnectTimeout); err != nil {
		log.Printf("ingest: %v", err)
		return 1
	}
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Printf("ingest: %v", err)
		return 1
	}
	defer pool.Close()

	// No external data enters the system from an unregistered or disabled
	// source (registro-de-fontes spec). This is also what stops a blocked
	// source from ever being collected by accident.
	src, err := source.GetEnabledByName(ctx, pool, usgs.SourceName)
	if err != nil {
		log.Printf("ingest: %v", err)
		return 1
	}

	ing := usgs.NewIngester(usgs.NewClient(), pool, src.ID)
	ing.PageSize = *pageSize

	var report usgs.Report
	var ingestErr error

	switch {
	case *start != "":
		windowStart, err := time.Parse(time.RFC3339, *start)
		if err != nil {
			log.Printf("ingest: -start is not a valid RFC3339 instant: %v", err)
			return 2
		}
		windowEnd := time.Now().UTC()
		if *end != "" {
			windowEnd, err = time.Parse(time.RFC3339, *end)
			if err != nil {
				log.Printf("ingest: -end is not a valid RFC3339 instant: %v", err)
				return 2
			}
		}
		if windowEnd.Before(windowStart) {
			log.Printf("ingest: the window ends before it starts (%s..%s)", windowStart, windowEnd)
			return 2
		}
		report, ingestErr = ing.IngestWindow(ctx, ingestion.ModeManual, windowStart.UTC(), windowEnd.UTC())

	case *backfill:
		now := time.Now().UTC()
		report, ingestErr = ing.IngestWindow(ctx, ingestion.ModeBackfill, now.Add(-*since), now)

	default:
		report, ingestErr = ing.IngestIncremental(ctx, *since)
	}

	// The run is recorded either way; printing it after a failure is how an
	// operator sees how far it got before breaking.
	printIngestReport(report)
	if ingestErr != nil {
		log.Printf("ingest: %v", ingestErr)
		return 1
	}
	return 0
}

func printIngestReport(r usgs.Report) {
	fmt.Printf("ingestion run %d (%s)\n", r.Run.ID, r.Run.Result)
	fmt.Printf("  source version: %s\n", orDash(r.SourceVersion))
	fmt.Printf("  window:         %s .. %s\n",
		r.Run.WindowStart.Format(time.RFC3339), r.Run.WindowEnd.Format(time.RFC3339))
	fmt.Printf("  inserted:       %d\n", r.Counts.Inserted)
	fmt.Printf("  updated:        %d\n", r.Counts.Updated)
	fmt.Printf("  unchanged:      %d\n", r.Counts.Unchanged)
	fmt.Printf("  rejected:       %d\n", r.Counts.Rejected)
	if r.Run.ErrorMsg != "" {
		fmt.Printf("  error:          %s\n", r.Run.ErrorMsg)
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
