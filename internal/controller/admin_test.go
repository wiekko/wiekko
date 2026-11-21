// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"nexmap.net/internal/controller"
	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
	"nexmap.net/internal/testutil"
)

func TestAdminListUsers(t *testing.T) {
	env := newTestEnv(t)
	users := []*model.User{testutil.MakeUser()}
	env.Users.ListFunc = func(_ context.Context, _ store.UserFilter) ([]*model.User, error) {
		return users, nil
	}

	ctrl := controller.NewAdmin(env.Services.User, env.Services.Setting)
	w, r := authedRequest(http.MethodGet, "/admin/users", "", testutil.MakeUser(func(u *model.User) { u.IsAdmin = true }))
	ctrl.ListUsers(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAdminGetUser(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		env := newTestEnv(t)
		user := testutil.MakeUser()
		env.Users.GetByIDFunc = func(_ context.Context, _ string) (*model.User, error) { return user, nil }

		ctrl := controller.NewAdmin(env.Services.User, env.Services.Setting)
		w, r := authedRequest(http.MethodGet, "/admin/users/u1", "", testutil.MakeUser(func(u *model.User) { u.IsAdmin = true }))
		r.SetPathValue("id", "u1")
		ctrl.GetUser(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
	})

	t.Run("not found", func(t *testing.T) {
		env := newTestEnv(t)
		env.Users.GetByIDFunc = func(_ context.Context, _ string) (*model.User, error) {
			return nil, errors.New("not found")
		}

		ctrl := controller.NewAdmin(env.Services.User, env.Services.Setting)
		w, r := authedRequest(http.MethodGet, "/admin/users/bad", "", testutil.MakeUser(func(u *model.User) { u.IsAdmin = true }))
		r.SetPathValue("id", "bad")
		ctrl.GetUser(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})
}

func TestAdminLockUser(t *testing.T) {
	env := newTestEnv(t)
	env.Users.UpdateStatusFunc = func(_ context.Context, _, _ string) error { return nil }

	ctrl := controller.NewAdmin(env.Services.User, env.Services.Setting)
	w, r := authedRequest(http.MethodPost, "/admin/users/u1/lock", "", testutil.MakeUser(func(u *model.User) { u.IsAdmin = true }))
	r.SetPathValue("id", "u1")
	ctrl.LockUser(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAdminUnlockUser(t *testing.T) {
	env := newTestEnv(t)
	env.Users.UpdateStatusFunc = func(_ context.Context, _, _ string) error { return nil }

	ctrl := controller.NewAdmin(env.Services.User, env.Services.Setting)
	w, r := authedRequest(http.MethodPost, "/admin/users/u1/unlock", "", testutil.MakeUser(func(u *model.User) { u.IsAdmin = true }))
	r.SetPathValue("id", "u1")
	ctrl.UnlockUser(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAdminSetPassword(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		env := newTestEnv(t)
		env.Users.UpdatePasswordFunc = func(_ context.Context, _, _ string) error { return nil }
		env.Sessions.DeleteByUserIDFunc = func(_ context.Context, _ string) error { return nil }

		ctrl := controller.NewAdmin(env.Services.User, env.Services.Setting)
		admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
		w, r := authedRequest(http.MethodPost, "/admin/users/u1/password", `{"password":"NewP@ss1!"}`, admin)
		r.SetPathValue("id", "u1")
		ctrl.SetPassword(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
		}
	})

	t.Run("missing password", func(t *testing.T) {
		env := newTestEnv(t)
		ctrl := controller.NewAdmin(env.Services.User, env.Services.Setting)
		admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
		w, r := authedRequest(http.MethodPost, "/admin/users/u1/password", `{}`, admin)
		r.SetPathValue("id", "u1")
		ctrl.SetPassword(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}

func TestAdminApproveUser(t *testing.T) {
	env := newTestEnv(t)
	env.Users.GetByIDFunc = func(_ context.Context, _ string) (*model.User, error) {
		return testutil.MakeUser(func(u *model.User) { u.Status = model.UserStatusPendingApproval }), nil
	}
	env.Users.UpdateStatusFunc = func(_ context.Context, _, _ string) error { return nil }

	ctrl := controller.NewAdmin(env.Services.User, env.Services.Setting)
	admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
	w, r := authedRequest(http.MethodPost, "/admin/users/u1/approve", "", admin)
	r.SetPathValue("id", "u1")
	ctrl.ApproveUser(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAdminDeleteUser(t *testing.T) {
	env := newTestEnv(t)
	env.Users.DeleteFunc = func(_ context.Context, _ string) error { return nil }

	ctrl := controller.NewAdmin(env.Services.User, env.Services.Setting)
	admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
	w, r := authedRequest(http.MethodDelete, "/admin/users/u1", "", admin)
	r.SetPathValue("id", "u1")
	ctrl.DeleteUser(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAdminGetSettings(t *testing.T) {
	env := newTestEnv(t)
	env.Settings.GetAllFunc = func(_ context.Context) (map[string]string, error) {
		return map[string]string{"moderation_enabled": "true"}, nil
	}

	ctrl := controller.NewAdmin(env.Services.User, env.Services.Setting)
	admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
	w, r := authedRequest(http.MethodGet, "/admin/settings", "", admin)
	ctrl.GetSettings(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Data map[string]string `json:"data"`
	}
	err := json.NewDecoder(w.Body).Decode(&body)
	if err != nil {
		t.Fatalf("error decoding JSON body: %v", err)
	}
	if body.Data["moderation_enabled"] != "true" {
		t.Fatalf("moderation_enabled = %q, want %q", body.Data["moderation_enabled"], "true")
	}
}

func TestAdminUpdateSettings(t *testing.T) {
	env := newTestEnv(t)
	env.Settings.SetFunc = func(_ context.Context, _, _ string) error { return nil }
	env.Settings.GetAllFunc = func(_ context.Context) (map[string]string, error) {
		return map[string]string{"moderation_enabled": "false"}, nil
	}

	ctrl := controller.NewAdmin(env.Services.User, env.Services.Setting)
	admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
	w, r := authedRequest(http.MethodPatch, "/admin/settings", `{"moderation_enabled":"false"}`, admin)
	ctrl.UpdateSettings(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
