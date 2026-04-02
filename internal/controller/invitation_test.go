package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"nexmap.net/internal/controller"
	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func TestInvitationCreate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		env := newTestEnv()
		env.Invitations.CreateFunc = func(_ context.Context, _ *model.Invitation) error { return nil }

		ctrl := controller.NewInvitation(env.Services.Invitation)
		admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
		w, r := authedRequest(http.MethodPost, "/admin/invitations", `{"email":"new@test.com"}`, admin)
		ctrl.Create(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
		}

		var body struct {
			Data struct {
				RawToken string `json:"raw_token"`
			} `json:"data"`
		}
		err := json.NewDecoder(w.Body).Decode(&body)
		if err != nil {
			t.Fatalf("error decoding JSON body: %v", err)
		}
		if body.Data.RawToken == "" {
			t.Fatal("raw_token is empty")
		}
	})

	t.Run("missing email", func(t *testing.T) {
		env := newTestEnv()
		ctrl := controller.NewInvitation(env.Services.Invitation)
		admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
		w, r := authedRequest(http.MethodPost, "/admin/invitations", `{}`, admin)
		ctrl.Create(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}

func TestInvitationList(t *testing.T) {
	env := newTestEnv()
	env.Invitations.ListFunc = func(_ context.Context) ([]*model.Invitation, error) {
		return []*model.Invitation{testutil.MakeInvitation()}, nil
	}

	ctrl := controller.NewInvitation(env.Services.Invitation)
	admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
	w, r := authedRequest(http.MethodGet, "/admin/invitations", "", admin)
	ctrl.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestInvitationDelete(t *testing.T) {
	env := newTestEnv()
	env.Invitations.DeleteFunc = func(_ context.Context, _ string) error { return nil }

	ctrl := controller.NewInvitation(env.Services.Invitation)
	admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
	w, r := authedRequest(http.MethodDelete, "/admin/invitations/inv-1", "", admin)
	r.SetPathValue("id", "inv-1")
	ctrl.Delete(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
