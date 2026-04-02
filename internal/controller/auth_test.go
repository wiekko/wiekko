package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nexmap.net/internal/auth"
	"nexmap.net/internal/controller"
	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func TestAuthLogin(t *testing.T) {
	t.Run("valid credentials", func(t *testing.T) {
		env := newTestEnv()
		user := testutil.MakeUser()
		env.Provider.AuthenticateFunc = func(_ context.Context, _, _ string) (*auth.Identity, error) {
			return &auth.Identity{UserID: user.ID}, nil
		}
		env.Users.GetByIDFunc = func(_ context.Context, _ string) (*model.User, error) {
			return user, nil
		}
		env.Sessions.CreateFunc = func(_ context.Context, _ *model.Session) error { return nil }

		ctrl := controller.NewAuth(env.Services.Auth, env.Services.User, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"test","password":"pass"}`))
		ctrl.Login(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}

		// Verify cookie is set.
		cookies := w.Result().Cookies()
		found := false
		for _, c := range cookies {
			if c.Name == "nexmap_session" && c.Value != "" {
				found = true
			}
		}
		if !found {
			t.Fatal("nexmap_session cookie not set")
		}

		// Verify response has csrf_token.
		var body struct {
			Data struct {
				CSRFToken string `json:"csrf_token"`
			} `json:"data"`
		}
		err := json.NewDecoder(w.Body).Decode(&body)
		if err != nil {
			t.Fatalf("error decoding JSON body: %v", err)
		}
		if body.Data.CSRFToken == "" {
			t.Fatal("csrf_token is empty in response")
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		env := newTestEnv()
		ctrl := controller.NewAuth(env.Services.Auth, env.Services.User, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"test"}`))
		ctrl.Login(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("wrong credentials", func(t *testing.T) {
		env := newTestEnv()
		env.Provider.AuthenticateFunc = func(_ context.Context, _, _ string) (*auth.Identity, error) {
			return nil, http.ErrAbortHandler
		}
		ctrl := controller.NewAuth(env.Services.Auth, env.Services.User, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"test","password":"wrong"}`))
		ctrl.Login(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

func TestAuthLogout(t *testing.T) {
	t.Run("with cookie", func(t *testing.T) {
		env := newTestEnv()
		env.Sessions.DeleteFunc = func(_ context.Context, _ string) error { return nil }
		ctrl := controller.NewAuth(env.Services.Auth, env.Services.User, false)

		r := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		r.AddCookie(&http.Cookie{Name: "nexmap_session", Value: "sess-1"})
		w := httptest.NewRecorder()
		ctrl.Logout(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}

		// Verify cookie is cleared.
		for _, c := range w.Result().Cookies() {
			if c.Name == "nexmap_session" && c.MaxAge != -1 {
				t.Fatal("cookie should have MaxAge=-1")
			}
		}
	})

	t.Run("no cookie", func(t *testing.T) {
		env := newTestEnv()
		ctrl := controller.NewAuth(env.Services.Auth, env.Services.User, false)
		w := httptest.NewRecorder()
		ctrl.Logout(w, httptest.NewRequest(http.MethodPost, "/auth/logout", nil))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}

func TestAuthRegister(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		env := newTestEnv()
		env.Settings.GetFunc = func(_ context.Context, key string) (string, error) {
			if key == "self_registration_enabled" {
				return "true", nil
			}
			return "", http.ErrAbortHandler
		}
		env.Users.CreateFunc = func(_ context.Context, _ *model.User) error { return nil }

		ctrl := controller.NewAuth(env.Services.Auth, env.Services.User, false)
		w := httptest.NewRecorder()
		body := `{"username":"alice","email":"a@b.com","password":"Str0ngP@ss!"}`
		r := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
		ctrl.Register(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		env := newTestEnv()
		ctrl := controller.NewAuth(env.Services.Auth, env.Services.User, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{"username":"alice"}`))
		ctrl.Register(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}

func TestAuthAcceptInvite(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		env := newTestEnv()
		inv := testutil.MakeInvitation()
		env.Invitations.GetByTokenHashFunc = func(_ context.Context, _ string) (*model.Invitation, error) {
			return inv, nil
		}
		env.Invitations.MarkAcceptedFunc = func(_ context.Context, _, _ string) error { return nil }
		env.Users.CreateFunc = func(_ context.Context, _ *model.User) error { return nil }

		ctrl := controller.NewAuth(env.Services.Auth, env.Services.User, false)
		w := httptest.NewRecorder()
		body := `{"token":"sometoken","username":"bob","email":"b@c.com","password":"Str0ngP@ss!"}`
		r := httptest.NewRequest(http.MethodPost, "/auth/accept-invite", strings.NewReader(body))
		ctrl.AcceptInvite(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		env := newTestEnv()
		ctrl := controller.NewAuth(env.Services.Auth, env.Services.User, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/auth/accept-invite", strings.NewReader(`{"token":"tok"}`))
		ctrl.AcceptInvite(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}
