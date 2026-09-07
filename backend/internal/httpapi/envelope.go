// Package httpapi implements the versioned public HTTP contract described by
// the api-publica spec.
//
// Two rules are enforced here rather than in each handler, because "every
// handler must remember to do X" is a rule that eventually gets forgotten:
//
//   - every response carries source attribution when it serves external data;
//   - every response containing a derived value carries the scientific
//     disclaimer (§85 of the master spec), and no request parameter can
//     suppress it.
package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
)

// Disclaimer is attached to every response carrying a derived value. It is set
// by the serialisation layer, never by a handler, and there is deliberately no
// way for a request to turn it off.
const Disclaimer = "Resultado experimental de pesquisa. Não constitui alerta " +
	"oficial de emergência; consulte sempre as autoridades competentes."

// Meta carries response-level information that is not part of the payload.
type Meta struct {
	// Count is the number of items in this page.
	Count int `json:"count"`
	// NextCursor, when non-empty, fetches the next page. Empty means the
	// caller has reached the end of the collection.
	NextCursor string `json:"next_cursor,omitempty"`
	// Disclaimer is present whenever the payload contains a derived value.
	Disclaimer string `json:"disclaimer,omitempty"`
	// RequestID correlates this response with the server log.
	RequestID string `json:"request_id,omitempty"`
}

// Attribution names an external source whose data appears in the response,
// carrying the credit its terms require.
type Attribution struct {
	Source      string `json:"source"`
	License     string `json:"license,omitempty"`
	Attribution string `json:"attribution,omitempty"`
	URL         string `json:"url,omitempty"`
}

// Envelope is the single response shape for successful requests.
type Envelope struct {
	Data        any           `json:"data"`
	Meta        Meta          `json:"meta"`
	Attribution []Attribution `json:"attribution,omitempty"`
}

// ErrorBody is the single response shape for failures. It carries a stable,
// machine-readable code and a human-readable message, and deliberately never
// carries internal detail: SQL, file paths and stack traces go to the log,
// keyed by RequestID.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail is the error payload.
type ErrorDetail struct {
	// Code is stable across releases so clients can branch on it.
	Code string `json:"code"`
	// Message is safe to show a human and never leaks internals.
	Message string `json:"message"`
	// Param names the offending query parameter, when the failure is a
	// validation error.
	Param string `json:"param,omitempty"`
	// RequestID lets a caller quote the failure back to an operator, who can
	// find the technical detail in the log.
	RequestID string `json:"request_id,omitempty"`
}

// Error codes. These are part of the public contract.
const (
	CodeInvalidParameter = "invalid_parameter"
	CodeNotFound         = "not_found"
	CodeInternal         = "internal_error"
	CodeUnavailable      = "service_unavailable"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status line is already on the wire, so the response cannot be
		// rewritten; all that is left is to make the failure visible.
		log.Printf("httpapi: failed to encode response body: %v", err)
	}
}

// WriteError renders a failure in the standard shape.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message, param string) {
	writeJSON(w, status, ErrorBody{Error: ErrorDetail{
		Code:      code,
		Message:   message,
		Param:     param,
		RequestID: RequestIDFrom(r.Context()),
	}})
}

// WriteInternal logs the technical cause and returns a generic failure. The
// caller gets a request id and nothing else — leaking a SQL string or a file
// path to an unauthenticated client is both a security and a support problem.
func WriteInternal(w http.ResponseWriter, r *http.Request, cause error) {
	reqID := RequestIDFrom(r.Context())
	log.Printf("httpapi: request_id=%s internal error: %v", reqID, cause)
	WriteError(w, r, http.StatusInternalServerError, CodeInternal,
		"Erro interno ao processar a requisição.", "")
}

// WriteData renders a successful response.
//
// derived reports whether the payload contains any value produced by analysis
// rather than direct observation; when true the disclaimer is attached here,
// so that no handler can forget it and no request parameter can remove it.
func WriteData(w http.ResponseWriter, r *http.Request, data any, meta Meta, derived bool, attribution ...Attribution) {
	meta.RequestID = RequestIDFrom(r.Context())
	if derived {
		meta.Disclaimer = Disclaimer
	}
	writeJSON(w, http.StatusOK, Envelope{Data: data, Meta: meta, Attribution: attribution})
}
