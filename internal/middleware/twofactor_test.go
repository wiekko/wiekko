package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"nexmap.net/internal/model"
)

type fixedPolicy bool

func (p fixedPolicy) Enforced(context.Context) bool { return bool(p) }

func TestRequireTwoFactorEnrollment(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	plain := &model.User{ID: "u1"}
	enrolled := &model.User{ID: "u2", TOTPEnabled: true}

	tests := []struct {
		name     string
		enforced bool
		user     *model.User
		method   string // auth method
		path     string
		htmx     bool
		want     int
		location string
	}{
		{"not enforced", false, plain, "session", "/", false, http.StatusTeapot, ""},
		{"anonymous", true, nil, "", "/", false, http.StatusTeapot, ""},
		{"enrolled user", true, enrolled, "session", "/", false, http.StatusTeapot, ""},
		{"PAT request", true, plain, "pat", "/api/v1/products", false, http.StatusTeapot, ""},
		{"page redirected", true, plain, "session", "/products/x", false, http.StatusSeeOther, TwoFactorSetupPath},
		{"htmx redirected", true, plain, "session", "/products/x", true, http.StatusOK, ""},
		{"api blocked", true, plain, "session", "/api/v1/products", false, http.StatusForbidden, ""},
		{"setup page allowed", true, plain, "session", "/user/2fa/setup", false, http.StatusTeapot, ""},
		{"api setup allowed", true, plain, "session", "/api/v1/user/me/totp/confirm", false, http.StatusTeapot, ""},
		{"logout allowed", true, plain, "session", "/logout", false, http.StatusTeapot, ""},
		{"static allowed", true, plain, "session", "/static/css/styles.css", false, http.StatusTeapot, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.user != nil {
				r = r.WithContext(WithUser(r.Context(), tc.user, tc.method, "csrf"))
			}
			if tc.htmx {
				r.Header.Set("HX-Request", "true")
			}
			w := httptest.NewRecorder()
			RequireTwoFactorEnrollment(fixedPolicy(tc.enforced))(ok).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
			if tc.location != "" && w.Header().Get("Location") != tc.location {
				t.Fatalf("Location = %q, want %q", w.Header().Get("Location"), tc.location)
			}
			if tc.htmx && w.Header().Get("HX-Redirect") != TwoFactorSetupPath {
				t.Fatalf("HX-Redirect = %q", w.Header().Get("HX-Redirect"))
			}
		})
	}
}
