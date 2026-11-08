package controller_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nexmap.net/internal/buildinfo"
	"nexmap.net/internal/controller"
)

func TestHealth(t *testing.T) {
	w := httptest.NewRecorder()
	controller.Health(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q, want %q", body.Status, "ok")
	}
}

func TestVersion(t *testing.T) {
	w := httptest.NewRecorder()
	controller.Version(w, httptest.NewRequest(http.MethodGet, "/version", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data struct {
			Version  string `json:"version"`
			APILevel int    `json:"api_level"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Version == "" {
		t.Error("version is empty")
	}
	if body.Data.APILevel != buildinfo.APILevel {
		t.Errorf("api_level = %d, want %d", body.Data.APILevel, buildinfo.APILevel)
	}
}
