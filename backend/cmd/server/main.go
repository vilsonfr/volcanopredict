package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/vilsonfr/volcanopredict/backend/internal/config"
	"github.com/vilsonfr/volcanopredict/backend/internal/db"
)

type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Magnitude *float64  `json:"magnitude,omitempty"`
	Time      time.Time `json:"time"`
	Source    string    `json:"source"`
}

func main() {
	// Subcomandos precedem o servidor: importar o catalogo e um ato
	// deliberado, nao um passo de inicializacao (design.md D4).
	if len(os.Args) > 1 && os.Args[1] == "import-catalog" {
		os.Exit(runImportCatalog(os.Args[2:]))
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

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "error": "database unreachable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		mag := 4.8
		events := []Event{{
			ID: "demo-1", Type: "earthquake", Name: "Demo event",
			Latitude: -6.102, Longitude: 105.423, Magnitude: &mag,
			Time: time.Now().UTC(), Source: "demo",
		}}
		writeJSON(w, http.StatusOK, events)
	})

	addr := cfg.HTTPAddr
	srv := &http.Server{Addr: addr, Handler: logging(mux)}
	log.Printf("VolcanoPredict backend listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
