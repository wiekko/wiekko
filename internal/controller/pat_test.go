package controller_test

import (
	"context"
	"net/http"
	"testing"

	"nexmap.net/internal/controller"
	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func TestPATList(t *testing.T) {
	env := newTestEnv(t)
	user := testutil.MakeUser()
	env.PATs.ListByUserIDFunc = func(_ context.Context, _ string) ([]*model.PAT, error) {
		return []*model.PAT{testutil.MakePAT()}, nil
	}

	ctrl := controller.NewPAT(env.Services.PAT)
	w, r := authedRequest(http.MethodGet, "/user/me/tokens", "", user)
	ctrl.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestPATCreate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		env := newTestEnv(t)
		user := testutil.MakeUser()
		env.PATs.CreateFunc = func(_ context.Context, _ *model.PAT) error { return nil }

		ctrl := controller.NewPAT(env.Services.PAT)
		w, r := authedRequest(http.MethodPost, "/user/me/tokens", `{"name":"my-token"}`, user)
		ctrl.Create(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
		}
		decodeRawToken(t, w)
	})

	t.Run("missing name", func(t *testing.T) {
		env := newTestEnv(t)
		user := testutil.MakeUser()
		ctrl := controller.NewPAT(env.Services.PAT)
		w, r := authedRequest(http.MethodPost, "/user/me/tokens", `{}`, user)
		ctrl.Create(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}

func TestPATRevoke(t *testing.T) {
	env := newTestEnv(t)
	user := testutil.MakeUser()
	pat := testutil.MakePAT(func(p *model.PAT) { p.UserID = user.ID })
	env.PATs.ListByUserIDFunc = func(_ context.Context, _ string) ([]*model.PAT, error) {
		return []*model.PAT{pat}, nil
	}
	env.PATs.RevokeFunc = func(_ context.Context, _, _ string) error { return nil }

	ctrl := controller.NewPAT(env.Services.PAT)
	w, r := authedRequest(http.MethodDelete, "/user/me/tokens/"+pat.ID, "", user)
	r.SetPathValue("id", pat.ID)
	ctrl.Revoke(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}
