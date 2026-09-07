package main

import "net/http"

// withCORS lets the browser frontend, served from a different origin during
// development, read the API. Only safe, read-only methods are allowed: the API
// is public and read-only in V0.1, so there is nothing to protect with
// credentials — and no credentials are accepted.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
