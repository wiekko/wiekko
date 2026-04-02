package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func TestPATCreate(t *testing.T) {
	t.Run("valid name", func(t *testing.T) {
		var stored *model.PAT
		svc := &PATService{
			pats: &testutil.MockPATStore{
				CreateFunc: func(_ context.Context, p *model.PAT) error { stored = p; return nil },
			},
		}
		result, err := svc.Create(context.Background(), "u1", "my-token", nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if !strings.HasPrefix(result.RawToken, "nxm_") {
			t.Fatalf("RawToken = %q, want nxm_ prefix", result.RawToken)
		}
		if stored == nil {
			t.Fatal("PAT was not stored")
		}
		if stored.TokenHash == "" {
			t.Fatal("TokenHash is empty")
		}
		if stored.UserID != "u1" {
			t.Fatalf("UserID = %q, want %q", stored.UserID, "u1")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		svc := &PATService{pats: &testutil.MockPATStore{}}
		_, err := svc.Create(context.Background(), "u1", "", nil)
		if err == nil || err.Error() != "token name is required" {
			t.Fatalf("err = %v, want 'token name is required'", err)
		}
	})

	t.Run("with expiresAt", func(t *testing.T) {
		var stored *model.PAT
		svc := &PATService{
			pats: &testutil.MockPATStore{
				CreateFunc: func(_ context.Context, p *model.PAT) error { stored = p; return nil },
			},
		}
		exp := "2030-01-01T00:00:00Z"
		_, err := svc.Create(context.Background(), "u1", "tok", &exp)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if stored.ExpiresAt != exp {
			t.Fatalf("ExpiresAt = %q, want %q", stored.ExpiresAt, exp)
		}
	})

	t.Run("nil expiresAt", func(t *testing.T) {
		var stored *model.PAT
		svc := &PATService{
			pats: &testutil.MockPATStore{
				CreateFunc: func(_ context.Context, p *model.PAT) error { stored = p; return nil },
			},
		}
		_, err := svc.Create(context.Background(), "u1", "tok", nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if stored.ExpiresAt != "" {
			t.Fatalf("ExpiresAt = %q, want empty", stored.ExpiresAt)
		}
	})
}

func TestPATList(t *testing.T) {
	pats := []*model.PAT{testutil.MakePAT()}
	svc := &PATService{
		pats: &testutil.MockPATStore{
			ListByUserIDFunc: func(_ context.Context, uid string) ([]*model.PAT, error) {
				if uid == "u1" {
					return pats, nil
				}
				return nil, nil
			},
		},
	}
	result, err := svc.List(context.Background(), "u1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
}

func TestPATRevoke(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		revoked := false
		pat := testutil.MakePAT(func(p *model.PAT) { p.UserID = "u1" })
		svc := &PATService{
			pats: &testutil.MockPATStore{
				ListByUserIDFunc: func(_ context.Context, _ string) ([]*model.PAT, error) {
					return []*model.PAT{pat}, nil
				},
				RevokeFunc: func(_ context.Context, _, _ string) error { revoked = true; return nil },
			},
		}
		if err := svc.Revoke(context.Background(), "u1", pat.ID); err != nil {
			t.Fatalf("Revoke: %v", err)
		}
		if !revoked {
			t.Fatal("token was not revoked")
		}
	})

	t.Run("already revoked", func(t *testing.T) {
		pat := testutil.MakePAT(func(p *model.PAT) {
			p.UserID = "u1"
			p.RevokedAt = "2025-01-01T00:00:00Z"
		})
		svc := &PATService{
			pats: &testutil.MockPATStore{
				ListByUserIDFunc: func(_ context.Context, _ string) ([]*model.PAT, error) {
					return []*model.PAT{pat}, nil
				},
			},
		}
		err := svc.Revoke(context.Background(), "u1", pat.ID)
		if err == nil || err.Error() != "token already revoked" {
			t.Fatalf("err = %v, want 'token already revoked'", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svc := &PATService{
			pats: &testutil.MockPATStore{
				ListByUserIDFunc: func(_ context.Context, _ string) ([]*model.PAT, error) {
					return nil, nil
				},
			},
		}
		err := svc.Revoke(context.Background(), "u1", "nonexistent")
		if err == nil || err.Error() != "token not found" {
			t.Fatalf("err = %v, want 'token not found'", err)
		}
	})

	t.Run("list error", func(t *testing.T) {
		svc := &PATService{
			pats: &testutil.MockPATStore{
				ListByUserIDFunc: func(_ context.Context, _ string) ([]*model.PAT, error) {
					return nil, errors.New("db error")
				},
			},
		}
		err := svc.Revoke(context.Background(), "u1", "p1")
		if err == nil || err.Error() != "db error" {
			t.Fatalf("err = %v, want 'db error'", err)
		}
	})
}
