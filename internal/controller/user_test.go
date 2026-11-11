package controller_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"nexmap.net/internal/controller"
	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func TestGetMe(t *testing.T) {
	env := newTestEnv(t)
	ctrl := controller.NewUser(env.Services.User)

	user := testutil.MakeUser(func(u *model.User) { u.Username = "alice" })
	w, r := authedRequest(http.MethodGet, "/user/me", "", user)
	ctrl.GetMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Data struct {
			Username string `json:"username"`
		} `json:"data"`
	}
	err := json.NewDecoder(w.Body).Decode(&body)
	if err != nil {
		t.Fatalf("error decoding JSON body: %v", err)
	}
	if body.Data.Username != "alice" {
		t.Fatalf("username = %q, want %q", body.Data.Username, "alice")
	}
}

func TestUpdateMe(t *testing.T) {
	env := newTestEnv(t)
	user := testutil.MakeUser()
	env.Users.GetByIDFunc = func(_ context.Context, _ string) (*model.User, error) {
		u := *user
		return &u, nil
	}
	env.Users.UpdateFunc = func(_ context.Context, _ *model.User) error { return nil }

	ctrl := controller.NewUser(env.Services.User)
	w, r := authedRequest(http.MethodPatch, "/user/me", `{"email":"new@test.com"}`, user)
	ctrl.UpdateMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestChangePassword(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		env := newTestEnv(t)
		user := testutil.MakeUser()
		env.Users.GetByIDFunc = func(_ context.Context, _ string) (*model.User, error) { return user, nil }
		env.Users.UpdatePasswordFunc = func(_ context.Context, _, _ string) error { return nil }
		env.Sessions.DeleteByUserIDFunc = func(_ context.Context, _ string) error { return nil }

		ctrl := controller.NewUser(env.Services.User)
		w, r := authedRequest(http.MethodPost, "/user/me/password",
			`{"old_password":"old","new_password":"New1Pass!"}`, user)
		ctrl.ChangePassword(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		env := newTestEnv(t)
		ctrl := controller.NewUser(env.Services.User)
		user := testutil.MakeUser()
		w, r := authedRequest(http.MethodPost, "/user/me/password", `{"old_password":"old"}`, user)
		ctrl.ChangePassword(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}

func TestRequestDeletion(t *testing.T) {
	env := newTestEnv(t)
	user := testutil.MakeUser()
	env.Users.DeleteFunc = func(_ context.Context, _ string) error { return nil }
	// moderation disabled (default), so immediate delete.

	ctrl := controller.NewUser(env.Services.User)
	w, r := authedRequest(http.MethodPost, "/user/me/delete", "", user)
	ctrl.RequestDeletion(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequestDeletion_Error(t *testing.T) {
	env := newTestEnv(t)
	user := testutil.MakeUser()
	env.Users.DeleteFunc = func(_ context.Context, _ string) error { return errors.New("db error") }

	ctrl := controller.NewUser(env.Services.User)
	w, r := authedRequest(http.MethodPost, "/user/me/delete", "", user)
	ctrl.RequestDeletion(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
