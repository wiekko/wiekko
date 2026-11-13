package httputil

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

type dataEnvelope struct {
	Data any `json:"data"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// JSON writes a success response with the given status code and data payload.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(dataEnvelope{Data: data}); err != nil {
		slog.Error("failed to encode JSON response", "err", err)
	}
}

// Error writes an error response with the given status code, error code, and message.
func Error(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(errorEnvelope{Error: errorBody{Code: code, Message: message}}); err != nil {
		slog.Error("failed to encode JSON error response", "err", err)
	}
}

// DecodeJSON reads and decodes a JSON request body into dst.
// Returns false and writes an error response if decoding fails.
//
// The body is capped by the LimitBody middleware; one over the cap is answered
// with 413.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			Error(w, http.StatusRequestEntityTooLarge, "TOO_LARGE", "Request body is too large")
			return false
		}
		Error(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request body")
		return false
	}
	return true
}

// ErrorWithDetails writes an error response like Error, plus extra top-level
// fields a client needs to act on the error (e.g. a login challenge token).
func ErrorWithDetails(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	body := map[string]any{"error": errorBody{Code: code, Message: message}}
	for k, v := range details {
		if k != "error" {
			body[k] = v
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("failed to encode JSON error response", "err", err)
	}
}

// RequestBaseURL returns the scheme and host the client used to reach the
// server ("https://nexmap.example.com"). Behind a TLS-terminating proxy the
// scheme comes from X-Forwarded-Proto and the host from X-Forwarded-Host if
// set, else from the Host header the proxy passes on.
func RequestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := firstHeaderValue(r.Header.Get("X-Forwarded-Proto")); proto == "https" || proto == "http" {
		scheme = proto
	}
	host := r.Host
	if fwd := firstHeaderValue(r.Header.Get("X-Forwarded-Host")); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host
}

// firstHeaderValue returns the first entry of a comma-separated header value.
func firstHeaderValue(v string) string {
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	return strings.ToLower(strings.TrimSpace(v))
}
