package controller_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"nexmap.net/internal/auth"
	"nexmap.net/internal/config"
	"nexmap.net/internal/mail"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/store"
	"nexmap.net/internal/testutil"

	"net/http/httptest"
	"time"
)

// testEnv holds a fully wired service layer backed by mocks.
type testEnv struct {
	Users       *testutil.MockUserStore
	Sessions    *testutil.MockSessionStore
	PATs        *testutil.MockPATStore
	Settings    *testutil.MockSettingStore
	Invitations *testutil.MockInvitationStore
	Provider    *testutil.MockAuthProvider
	Services    *service.Services
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	return newTestEnvWithMail(t, nil)
}

// newTestEnvWithMail is newTestEnv sending email through sender (nil: none).
func newTestEnvWithMail(t *testing.T, sender mail.Sender) *testEnv {
	t.Helper()
	// New accounts are checked against existing ones; default to "none".
	notFound := func(context.Context, string) (*model.User, error) {
		return nil, fmt.Errorf("user not found: %w", sql.ErrNoRows)
	}
	users := &testutil.MockUserStore{GetByUsernameFunc: notFound, GetByEmailFunc: notFound}
	// Locking and deleting look the account up first; default to an ordinary user.
	users.GetByIDFunc = func(_ context.Context, id string) (*model.User, error) {
		return testutil.MakeUser(func(u *model.User) { u.ID = id }), nil
	}
	sessions := &testutil.MockSessionStore{}
	pats := &testutil.MockPATStore{}
	settings := &testutil.MockSettingStore{
		GetFunc: func(_ context.Context, _ string) (string, error) { return "", errors.New("not found") },
	}
	invitations := &testutil.MockInvitationStore{}
	provider := &testutil.MockAuthProvider{
		ValidatePasswordFunc:       func(_ string, _ auth.PasswordPolicy) error { return nil },
		HashPasswordFunc:           func(_ string) (string, error) { return "hashed", nil },
		VerifyPasswordFunc:         func(_, _ string) error { return nil },
		SupportsPasswordChangeFunc: func() bool { return true },
	}

	st := &store.Store{
		User:       users,
		Session:    sessions,
		PAT:        pats,
		Setting:    settings,
		Invitation: invitations,
	}

	cfg := &config.Config{SessionTTL: time.Hour, PublicURL: "https://nexmap.example.com"}
	blobs, err := storage.NewFS(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatalf("storage.NewFS: %v", err)
	}
	svc := service.NewWithMail(st, provider, cfg, blobs, sender)

	return &testEnv{
		Users:       users,
		Sessions:    sessions,
		PATs:        pats,
		Settings:    settings,
		Invitations: invitations,
		Provider:    provider,
		Services:    svc,
	}
}

// decodeRawToken decodes a JSON response body and returns the raw_token field.
func decodeRawToken(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Data struct {
			RawToken string `json:"raw_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("error decoding JSON body: %v", err)
	}
	if body.Data.RawToken == "" {
		t.Fatal("raw_token is empty")
	}
	return body.Data.RawToken
}

func authedRequest(method, target, body string, user *model.User) (*httptest.ResponseRecorder, *http.Request) {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	ctx := middleware.WithUser(r.Context(), user, "session", "csrf-tok")
	r = r.WithContext(ctx)
	return httptest.NewRecorder(), r
}
