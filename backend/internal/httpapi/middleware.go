package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"
)

type ctxKey int

const requestIDKey ctxKey = iota

// sensitiveParams are redacted from logs. A secret that reaches a query string
// has already leaked further than it should, but it must not also be written
// to disk in clear text.
var sensitiveParams = map[string]bool{
	"token":        true,
	"access_token": true,
	"api_key":      true,
	"apikey":       true,
	"key":          true,
	"password":     true,
	"secret":       true,
	"signature":    true,
}

const redacted = "[REDACTED]"

// RequestIDFrom returns the correlation id assigned to this request.
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// A weaker id is far better than failing the request: the id exists to
		// correlate logs, not to secure anything.
		return hex.EncodeToString([]byte(time.Now().UTC().Format("150405.000000")))
	}
	return hex.EncodeToString(b[:])
}

// statusRecorder captures the status code so the log line can report it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// redactQuery renders a query string with sensitive values masked.
func redactQuery(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "&")
	for i, p := range parts {
		name, _, found := strings.Cut(p, "=")
		if found && sensitiveParams[strings.ToLower(name)] {
			parts[i] = name + "=" + redacted
		}
	}
	return strings.Join(parts, "&")
}

// WithObservability assigns a correlation id and emits one structured log line
// per request, with sensitive query values redacted.
func WithObservability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := newRequestID()
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		r = r.WithContext(ctx)

		w.Header().Set("X-Request-Id", reqID)
		rec := &statusRecorder{ResponseWriter: w}

		start := time.Now()
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}

		log.Printf("request_id=%s method=%s path=%s query=%q status=%d duration=%s",
			reqID, r.Method, r.URL.Path, redactQuery(r.URL.RawQuery), rec.status, time.Since(start))
	})
}
