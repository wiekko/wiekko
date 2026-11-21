// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteServiceErrorMapsMissingRowTo404(t *testing.T) {
	w := httptest.NewRecorder()
	writeServiceError(w, fmt.Errorf("product not found: %w", sql.ErrNoRows))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"NOT_FOUND"`) {
		t.Errorf("body = %s, want code NOT_FOUND", body)
	}
	if strings.Contains(body, "sql:") {
		t.Errorf("body leaks the driver error: %s", body)
	}
}
