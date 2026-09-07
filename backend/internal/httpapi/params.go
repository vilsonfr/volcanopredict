package httpapi

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// MaxPageSize caps how many items one page may carry. A request asking for
// more is rejected rather than silently reduced: silently returning fewer
// items than asked for makes a client's paging logic subtly wrong.
const MaxPageSize = 500

// DefaultPageSize is used when the caller does not specify one.
const DefaultPageSize = 100

// paramError is a validation failure that names the offending parameter.
type paramError struct {
	Param   string
	Message string
}

func (e paramError) Error() string { return e.Param + ": " + e.Message }

func badParam(name, msg string) error { return paramError{Param: name, Message: msg} }

// WriteParamError renders a validation failure, naming the parameter.
func WriteParamError(w http.ResponseWriter, r *http.Request, err error) bool {
	var pe paramError
	if !errors.As(err, &pe) {
		return false
	}
	WriteError(w, r, http.StatusBadRequest, CodeInvalidParameter, pe.Message, pe.Param)
	return true
}

// intParam reads an optional integer query parameter.
func intParam(r *http.Request, name string, def int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, badParam(name, fmt.Sprintf("valor inválido %q: esperado um número inteiro", raw))
	}
	return v, nil
}

// floatParam reads an optional float query parameter. ok reports presence.
func floatParam(r *http.Request, name string) (v float64, ok bool, err error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return 0, false, nil
	}
	v, parseErr := strconv.ParseFloat(raw, 64)
	if parseErr != nil {
		return 0, false, badParam(name, fmt.Sprintf("valor inválido %q: esperado um número", raw))
	}
	return v, true, nil
}

// timeParam reads an optional RFC 3339 timestamp query parameter.
func timeParam(r *http.Request, name string) (t time.Time, ok bool, err error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return time.Time{}, false, nil
	}
	parsed, parseErr := time.Parse(time.RFC3339, raw)
	if parseErr != nil {
		return time.Time{}, false, badParam(name,
			fmt.Sprintf("valor inválido %q: esperado um instante no formato RFC 3339, por exemplo 2026-09-07T14:00:00Z", raw))
	}
	return parsed.UTC(), true, nil
}

// PageSize reads and validates the requested page size.
func PageSize(r *http.Request) (int, error) {
	limit, err := intParam(r, "limit", DefaultPageSize)
	if err != nil {
		return 0, err
	}
	if limit <= 0 {
		return 0, badParam("limit", "deve ser maior que zero")
	}
	if limit > MaxPageSize {
		return 0, badParam("limit",
			fmt.Sprintf("máximo permitido é %d, recebido %d", MaxPageSize, limit))
	}
	return limit, nil
}

// Cursor is an opaque page position. It is encoded rather than exposed so that
// its internal shape can change without breaking clients that (inevitably)
// treat it as a string.
type Cursor struct {
	ID        int64
	DistanceM *float64
}

// Encode renders the cursor as an opaque token.
func (c Cursor) Encode() string {
	s := strconv.FormatInt(c.ID, 10)
	if c.DistanceM != nil {
		s += ":" + strconv.FormatFloat(*c.DistanceM, 'f', -1, 64)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

// DecodeCursor parses a cursor token produced by Encode.
func DecodeCursor(raw string) (Cursor, error) {
	if raw == "" {
		return Cursor{}, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return Cursor{}, badParam("cursor", "cursor inválido")
	}
	idPart, distPart, hasDist := strings.Cut(string(b), ":")
	id, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil {
		return Cursor{}, badParam("cursor", "cursor inválido")
	}
	c := Cursor{ID: id}
	if hasDist {
		d, err := strconv.ParseFloat(distPart, 64)
		if err != nil {
			return Cursor{}, badParam("cursor", "cursor inválido")
		}
		c.DistanceM = &d
	}
	return c, nil
}

// CursorParam reads the optional cursor query parameter.
func CursorParam(r *http.Request) (Cursor, error) {
	return DecodeCursor(strings.TrimSpace(r.URL.Query().Get("cursor")))
}
