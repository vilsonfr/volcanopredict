package httpapi

import (
	"context"
	"net/http"
	"time"
)

// contextWithTimeout bounds a handler's database work. Without it a slow query
// holds a connection for as long as the client keeps the socket open.
func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}
