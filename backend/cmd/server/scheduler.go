package main

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vilsonfr/volcanopredict/backend/internal/config"
	"github.com/vilsonfr/volcanopredict/backend/internal/ingestion"
	"github.com/vilsonfr/volcanopredict/backend/internal/source"
	"github.com/vilsonfr/volcanopredict/backend/internal/usgs"
)

// startIngestionScheduler wires the periodic USGS cycle, if it is enabled.
//
// Everything that can go wrong here — the source not being registered, not
// being enabled, the network being down — is logged and dropped. None of it
// may stop the process: the HTTP service is expected to keep answering with
// whatever it already has (ingestao-fontes spec, "Fonte indisponível na
// subida").
func startIngestionScheduler(ctx context.Context, pool *pgxpool.Pool, cfg config.Config) {
	if cfg.IngestInterval <= 0 {
		log.Print("startup: automatic ingestion disabled (INGEST_INTERVAL is 0)")
		return
	}

	src, err := source.GetEnabledByName(ctx, pool, usgs.SourceName)
	if err != nil {
		log.Printf("startup: automatic ingestion not started: %v", err)
		return
	}

	ing := usgs.NewIngester(usgs.NewClient(), pool, src.ID)
	sched := &ingestion.Scheduler{
		Pool:     pool,
		Interval: cfg.IngestInterval,
		Name:     usgs.SourceName,
		Cycle: func(ctx context.Context) error {
			// The same call the subcommand makes: the scheduler adds a
			// timer and a lock, never behavior.
			_, err := ing.IngestIncremental(ctx, cfg.IngestBackfillOnFirstRun)
			return err
		},
		OnSkip: func(reason string) {
			now := time.Now().UTC()
			if _, err := ingestion.Skip(ctx, pool, src.ID, ingestion.ModeIncremental, now, now, reason); err != nil {
				log.Printf("ingestion: recording skipped cycle failed: %v", err)
			}
		},
	}

	go sched.Run(ctx)
}
