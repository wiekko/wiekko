package controller

import (
	"io"
	"net/http"
)

// Health handles GET /health and returns a JSON status response.
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `{"status":"ok"}`)
}
