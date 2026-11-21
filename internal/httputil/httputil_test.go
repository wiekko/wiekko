// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package httputil_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wiekko.net/internal/httputil"
)

func TestJSON(t *testing.T) {
	t.Run("wraps data in envelope", func(t *testing.T) {
		w := httptest.NewRecorder()
		httputil.JSON(w, http.StatusOK, map[string]string{"key": "val"})

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", ct)
		}

		var envelope struct {
			Data map[string]string `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if envelope.Data["key"] != "val" {
			t.Fatalf("data.key = %q, want %q", envelope.Data["key"], "val")
		}
	})

	t.Run("status 201", func(t *testing.T) {
		w := httptest.NewRecorder()
		httputil.JSON(w, http.StatusCreated, "created")
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
		}
	})

	t.Run("nil data", func(t *testing.T) {
		w := httptest.NewRecorder()
		httputil.JSON(w, http.StatusOK, nil)

		var envelope struct {
			Data any `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if envelope.Data != nil {
			t.Fatalf("data = %v, want nil", envelope.Data)
		}
	})
}

func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	httputil.Error(w, http.StatusBadRequest, "BAD_INPUT", "something went wrong")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if envelope.Error.Code != "BAD_INPUT" {
		t.Fatalf("error.code = %q, want %q", envelope.Error.Code, "BAD_INPUT")
	}
	if envelope.Error.Message != "something went wrong" {
		t.Fatalf("error.message = %q, want %q", envelope.Error.Message, "something went wrong")
	}
}

func TestDecodeJSON(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		body := strings.NewReader(`{"name":"test"}`)
		r := httptest.NewRequest(http.MethodPost, "/", body)
		w := httptest.NewRecorder()

		var dst struct {
			Name string `json:"name"`
		}
		if ok := httputil.DecodeJSON(w, r, &dst); !ok {
			t.Fatal("DecodeJSON returned false for valid JSON")
		}
		if dst.Name != "test" {
			t.Fatalf("name = %q, want %q", dst.Name, "test")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		body := strings.NewReader(`{invalid`)
		r := httptest.NewRequest(http.MethodPost, "/", body)
		w := httptest.NewRecorder()

		var dst struct{}
		if ok := httputil.DecodeJSON(w, r, &dst); ok {
			t.Fatal("DecodeJSON returned true for invalid JSON")
		}
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		body := strings.NewReader("")
		r := httptest.NewRequest(http.MethodPost, "/", body)
		w := httptest.NewRecorder()

		var dst struct{}
		if ok := httputil.DecodeJSON(w, r, &dst); ok {
			t.Fatal("DecodeJSON returned true for empty body")
		}
	})
}

func TestRequestBaseURL(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://wiekko.internal:8080/x", nil)
	if got := httputil.RequestBaseURL(r); got != "http://wiekko.internal:8080" {
		t.Errorf("plain: %q", got)
	}
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "wiekko.example.com, other")
	if got := httputil.RequestBaseURL(r); got != "https://wiekko.example.com" {
		t.Errorf("forwarded: %q", got)
	}
	r.Header.Set("X-Forwarded-Proto", "javascript")
	if got := httputil.RequestBaseURL(r); got != "http://wiekko.example.com" {
		t.Errorf("bad proto: %q", got)
	}
}
