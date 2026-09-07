package main

import (
    "context"
    "encoding/json"
    "log"
    "net/http"
    "os"
    "time"
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
    mux := http.NewServeMux()

    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
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

    addr := os.Getenv("HTTP_ADDR")
    if addr == "" { addr = ":8080" }
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

var _ = context.Background()
