package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexmap.net/internal/auth"
	"nexmap.net/internal/config"
	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func newAuthService(users *testutil.MockUserStore, sessions *testutil.MockSessionStore, pats *testutil.MockPATStore, provider *testutil.MockAuthProvider) *AuthService {
	return &AuthService{
		users:    users,
		sessions: sessions,
		pats:     pats,
		provider: provider,
		cfg:      &config.Config{SessionTTL: time.Hour},
	}
}

func TestLogin(t *testing.T) {
	user := testutil.MakeUser()

	t.Run("success", func(t *testing.T) {
		var createdSession *model.Session
		svc := newAuthService(
			&testutil.MockUserStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.User, error) { return user, nil },
			},
			&testutil.MockSessionStore{
				CreateFunc: func(_ context.Context, s *model.Session) error {
					createdSession = s
					return nil
				},
			},
			nil,
			&testutil.MockAuthProvider{
				AuthenticateFunc: func(_ context.Context, _, _ string) (*auth.Identity, error) {
					return &auth.Identity{UserID: user.ID}, nil
				},
			},
		)

		result, err := svc.Login(context.Background(), "testuser", "pass", "1.2.3.4", "Agent")
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if result.SessionID == "" {
			t.Fatal("SessionID is empty")
		}
		if result.CSRFToken == "" {
			t.Fatal("CSRFToken is empty")
		}
		if result.User.ID != user.ID {
			t.Fatalf("User.ID = %q, want %q", result.User.ID, user.ID)
		}
		if createdSession == nil {
			t.Fatal("session was not created")
		}
		if createdSession.IPAddress != "1.2.3.4" {
			t.Fatalf("IPAddress = %q, want %q", createdSession.IPAddress, "1.2.3.4")
		}
	})

	t.Run("provider rejects credentials", func(t *testing.T) {
		svc := newAuthService(nil, nil, nil,
			&testutil.MockAuthProvider{
				AuthenticateFunc: func(_ context.Context, _, _ string) (*auth.Identity, error) {
					return nil, errors.New("invalid credentials")
				},
			},
		)
		_, err := svc.Login(context.Background(), "user", "wrong", "", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("user not found after auth", func(t *testing.T) {
		svc := newAuthService(
			&testutil.MockUserStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.User, error) {
					return nil, errors.New("not found")
				},
			},
			nil, nil,
			&testutil.MockAuthProvider{
				AuthenticateFunc: func(_ context.Context, _, _ string) (*auth.Identity, error) {
					return &auth.Identity{UserID: "u1"}, nil
				},
			},
		)
		_, err := svc.Login(context.Background(), "user", "pass", "", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestLogout(t *testing.T) {
	var deletedID string
	svc := newAuthService(nil,
		&testutil.MockSessionStore{
			DeleteFunc: func(_ context.Context, id string) error {
				deletedID = id
				return nil
			},
		}, nil, nil,
	)

	if err := svc.Logout(context.Background(), "sess-1"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if deletedID != "sess-1" {
		t.Fatalf("deleted session = %q, want %q", deletedID, "sess-1")
	}
}

func TestValidateSession(t *testing.T) {
	activeUser := testutil.MakeUser()

	t.Run("valid session", func(t *testing.T) {
		session := testutil.MakeSession()
		svc := newAuthService(
			&testutil.MockUserStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.User, error) { return activeUser, nil },
			},
			&testutil.MockSessionStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Session, error) { return session, nil },
			}, nil, nil,
		)

		user, csrf, err := svc.ValidateSession(context.Background(), "sess-1")
		if err != nil {
			t.Fatalf("ValidateSession: %v", err)
		}
		if user.ID != activeUser.ID {
			t.Fatalf("user.ID = %q, want %q", user.ID, activeUser.ID)
		}
		if csrf != session.CSRFToken {
			t.Fatalf("csrf = %q, want %q", csrf, session.CSRFToken)
		}
	})

	t.Run("session not found", func(t *testing.T) {
		svc := newAuthService(nil,
			&testutil.MockSessionStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Session, error) {
					return nil, errors.New("not found")
				},
			}, nil, nil,
		)
		_, _, err := svc.ValidateSession(context.Background(), "bad")
		if err == nil || err.Error() != "invalid session" {
			t.Fatalf("err = %v, want 'invalid session'", err)
		}
	})

	t.Run("expired session", func(t *testing.T) {
		session := testutil.MakeSession(func(s *model.Session) {
			s.ExpiresAt = testutil.PastRFC3339(time.Hour)
		})
		deleted := false
		svc := newAuthService(nil,
			&testutil.MockSessionStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Session, error) { return session, nil },
				DeleteFunc:  func(_ context.Context, _ string) error { deleted = true; return nil },
			}, nil, nil,
		)
		_, _, err := svc.ValidateSession(context.Background(), "sess-1")
		if err == nil || err.Error() != "session expired" {
			t.Fatalf("err = %v, want 'session expired'", err)
		}
		if !deleted {
			t.Fatal("expired session was not deleted")
		}
	})

	t.Run("user not found", func(t *testing.T) {
		session := testutil.MakeSession()
		svc := newAuthService(
			&testutil.MockUserStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.User, error) {
					return nil, errors.New("not found")
				},
			},
			&testutil.MockSessionStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Session, error) { return session, nil },
			}, nil, nil,
		)
		_, _, err := svc.ValidateSession(context.Background(), "sess-1")
		if err == nil || err.Error() != "user not found" {
			t.Fatalf("err = %v, want 'user not found'", err)
		}
	})

	t.Run("user locked", func(t *testing.T) {
		lockedUser := testutil.MakeUser(func(u *model.User) { u.Status = model.UserStatusLocked })
		session := testutil.MakeSession()
		svc := newAuthService(
			&testutil.MockUserStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.User, error) { return lockedUser, nil },
			},
			&testutil.MockSessionStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Session, error) { return session, nil },
			}, nil, nil,
		)
		_, _, err := svc.ValidateSession(context.Background(), "sess-1")
		if err == nil || err.Error() != "account is not active" {
			t.Fatalf("err = %v, want 'account is not active'", err)
		}
	})
}

func TestValidatePAT(t *testing.T) {
	activeUser := testutil.MakeUser()
	rawToken := "nxm_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef12345678"
	tokenHash := hashPATToken(rawToken)

	makePATStore := func(pat *model.PAT) *testutil.MockPATStore {
		return &testutil.MockPATStore{
			GetByTokenHashFunc: func(_ context.Context, hash string) (*model.PAT, error) {
				if hash == tokenHash {
					return pat, nil
				}
				return nil, errors.New("not found")
			},
			UpdateLastUsedFunc: func(_ context.Context, _, _ string) error { return nil },
		}
	}

	assertValidPAT := func(t *testing.T, pat *model.PAT) {
		t.Helper()
		svc := newAuthService(
			&testutil.MockUserStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.User, error) { return activeUser, nil },
			}, nil, makePATStore(pat), nil,
		)
		user, err := svc.ValidatePAT(context.Background(), rawToken)
		if err != nil {
			t.Fatalf("ValidatePAT: %v", err)
		}
		if user.ID != activeUser.ID {
			t.Fatalf("user.ID = %q, want %q", user.ID, activeUser.ID)
		}
	}

	t.Run("valid token", func(t *testing.T) {
		pat := testutil.MakePAT(func(p *model.PAT) { p.TokenHash = tokenHash })
		assertValidPAT(t, pat)
	})

	t.Run("missing prefix", func(t *testing.T) {
		svc := newAuthService(nil, nil, nil, nil)
		_, err := svc.ValidatePAT(context.Background(), "badprefix_token")
		if err == nil || err.Error() != "invalid token format" {
			t.Fatalf("err = %v, want 'invalid token format'", err)
		}
	})

	t.Run("hash not found", func(t *testing.T) {
		svc := newAuthService(nil, nil,
			&testutil.MockPATStore{
				GetByTokenHashFunc: func(_ context.Context, _ string) (*model.PAT, error) {
					return nil, errors.New("not found")
				},
			}, nil,
		)
		_, err := svc.ValidatePAT(context.Background(), "nxm_unknown")
		if err == nil || err.Error() != "invalid token" {
			t.Fatalf("err = %v, want 'invalid token'", err)
		}
	})

	t.Run("revoked token", func(t *testing.T) {
		pat := testutil.MakePAT(func(p *model.PAT) {
			p.TokenHash = tokenHash
			p.RevokedAt = testutil.PastRFC3339(time.Hour)
		})
		svc := newAuthService(nil, nil, makePATStore(pat), nil)
		_, err := svc.ValidatePAT(context.Background(), rawToken)
		if err == nil || err.Error() != "token revoked" {
			t.Fatalf("err = %v, want 'token revoked'", err)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		pat := testutil.MakePAT(func(p *model.PAT) {
			p.TokenHash = tokenHash
			p.ExpiresAt = testutil.PastRFC3339(time.Hour)
		})
		svc := newAuthService(nil, nil, makePATStore(pat), nil)
		_, err := svc.ValidatePAT(context.Background(), rawToken)
		if err == nil || err.Error() != "token expired" {
			t.Fatalf("err = %v, want 'token expired'", err)
		}
	})

	t.Run("no expiry set", func(t *testing.T) {
		pat := testutil.MakePAT(func(p *model.PAT) {
			p.TokenHash = tokenHash
			p.ExpiresAt = "" // no expiry
		})
		assertValidPAT(t, pat)
	})

	t.Run("user not active", func(t *testing.T) {
		lockedUser := testutil.MakeUser(func(u *model.User) { u.Status = model.UserStatusLocked })
		pat := testutil.MakePAT(func(p *model.PAT) { p.TokenHash = tokenHash })
		svc := newAuthService(
			&testutil.MockUserStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.User, error) { return lockedUser, nil },
			}, nil, makePATStore(pat), nil,
		)
		_, err := svc.ValidatePAT(context.Background(), rawToken)
		if err == nil || err.Error() != "account is not active" {
			t.Fatalf("err = %v, want 'account is not active'", err)
		}
	})
}

// Ensure AuthService satisfies middleware interfaces.
var _ interface {
	ValidateSession(ctx context.Context, sessionID string) (*model.User, string, error)
} = (*AuthService)(nil)

var _ interface {
	ValidatePAT(ctx context.Context, rawToken string) (*model.User, error)
} = (*AuthService)(nil)
