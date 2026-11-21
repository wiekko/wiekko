// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"io"
	"net/http"

	"nexmap.net/internal/buildinfo"
	"nexmap.net/internal/httputil"
)

// Health handles GET /health and returns a JSON status response.
func Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `{"status":"ok"}`)
}

// Version handles GET /version. It is public so a client can check
// compatibility before it has credentials (e.g. `nexmap version`, `nexmap login`).
func Version(w http.ResponseWriter, _ *http.Request) {
	httputil.JSON(w, http.StatusOK, map[string]any{
		"version":   buildinfo.String(),
		"api_level": buildinfo.APILevel,
	})
}
