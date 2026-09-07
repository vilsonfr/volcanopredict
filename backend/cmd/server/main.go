package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/vilsonfr/volcanopredict/backend/internal/config"
	"github.com/vilsonfr/volcanopredict/backend/internal/db"
	"github.com/vilsonfr/volcanopredict/backend/internal/httpapi"
)

func main() {
	// Subcomandos precedem o servidor: importar o catalogo e um ato
	// deliberado, nao um passo de inicializacao (design.md D4).
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "import-catalog":
			os.Exit(runImportCatalog(os.Args[2:]))
		case "ingest":
			os.Exit(runIngest(os.Args[2:]))
		}
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("startup: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DBConnectTimeout)
	defer cancel()

	if err := db.WaitReady(ctx, cfg.DatabaseURL, cfg.DBConnectTimeout); err != nil {
		log.Fatalf("startup: %v", err)
	}

	// Migrations run to completion, inside their own transactions, before
	// the listener opens (design.md D3, ambiente-local spec: "migração
	// falha" scenario). A failure here must prevent the server from ever
	// serving traffic.
	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		log.Fatalf("startup: %v", err)
	}
	log.Print("startup: migrations applied")

	pool, err := db.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	defer pool.Close()

	mux := http.NewServeMux()

	// Todo dado servido vem da API versionada; nada e injetado em template
	// nem devolvido de memoria. O evento demo hardcoded que existia aqui foi
	// removido: ele saia como observacao real, sem marcacao de proveniencia,
	// contra a §4.1 da master spec.
	api := &httpapi.Server{DB: pool, SchemaVersion: db.ExpectedSchemaVersion}
	api.RegisterRoutes(mux)

	addr := cfg.HTTPAddr
	srv := &http.Server{Addr: addr, Handler: httpapi.WithObservability(withCORS(mux))}

	// Background ingestion starts only AFTER the listener is up, and its
	// failures never reach this goroutine (design.md D8). External data is
	// optional for answering a request; the database is not. A source being
	// down must not stop the service from serving what it already has.
	schedulerCtx, stopScheduler := context.WithCancel(context.Background())
	defer stopScheduler()
	startIngestionScheduler(schedulerCtx, pool, cfg)

	log.Printf("VolcanoPredict backend listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
