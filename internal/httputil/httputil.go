package httputil

import (
	"encoding/json"
	"log/slog"
	"net/http"
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
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
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
