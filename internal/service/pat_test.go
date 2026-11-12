package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nexmap.net/internal/access"
	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func TestPATCreate(t *testing.T) {
	member := &testutil.MockProductRoleStore{
		GetByUserAndProductFunc: func(_ context.Context, _, productID string) (*model.ProductRole, error) {
			if productID == "mine" {
				return testutil.MakeProductRole(), nil
			}
			return nil, errors.New("not found")
		},
	}
	newSvc := func(stored **model.PAT, maxDays string) *PATService {
		return &PATService{
			pats: &testutil.MockPATStore{
				CreateFunc: func(_ context.Context, p *model.PAT) error { *stored = p; return nil },
			},
			productRoles: member,
			settings: &testutil.MockSettingStore{
				GetFunc: func(_ context.Context, _ string) (string, error) { return maxDays, nil },
			},
		}
	}
	user := testutil.MakeUser(func(u *model.User) { u.ID = "u1" })
	future := func(d time.Duration) string { return time.Now().UTC().Add(d).Format(time.RFC3339) }

	t.Run("defaults to full access", func(t *testing.T) {
		var stored *model.PAT
		result, err := newSvc(&stored, "0").Create(context.Background(), user, CreatePATInput{Name: "my-token"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if !strings.HasPrefix(result.RawToken, "nxm_") {
			t.Fatalf("RawToken = %q, want nxm_ prefix", result.RawToken)
		}
		if stored == nil || stored.TokenHash == "" || stored.UserID != "u1" {
			t.Fatalf("stored = %+v", stored)
		}
		if stored.Scopes != access.FullAccess().String() || stored.RoleCap != "" || stored.ProductsRestricted {
			t.Fatalf("restriction = %q %q %v", stored.Scopes, stored.RoleCap, stored.ProductsRestricted)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		var stored *model.PAT
		_, err := newSvc(&stored, "0").Create(context.Background(), user, CreatePATInput{})
		if err == nil || err.Error() != "token name is required" {
			t.Fatalf("err = %v, want 'token name is required'", err)
		}
	})

	t.Run("restriction is stored", func(t *testing.T) {
		var stored *model.PAT
		exp := future(48 * time.Hour)
		_, err := newSvc(&stored, "0").Create(context.Background(), user, CreatePATInput{
			Name: "ci", ExpiresAt: exp, Grants: access.Grants{access.Tests: access.Write},
			RoleCap: model.RoleTester, ProductsRestricted: true, ProductIDs: []string{"mine", "mine"},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if stored.ExpiresAt != exp || stored.Scopes != "tests:write" || stored.RoleCap != model.RoleTester ||
			!stored.ProductsRestricted || len(stored.ProductIDs) != 1 {
			t.Fatalf("stored = %+v", stored)
		}
	})

	cases := []struct {
		name      string
		user      *model.User
		ctx       context.Context
		in        CreatePATInput
		maxDays   string
		forbidden bool
	}{
		{name: "admin scope for a non-admin", in: CreatePATInput{Grants: access.Grants{access.Admin: access.Read}}, forbidden: true},
		{name: "product the user is not a member of", in: CreatePATInput{ProductsRestricted: true, ProductIDs: []string{"other"}}, forbidden: true},
		{name: "restricted to no products", in: CreatePATInput{ProductsRestricted: true}},
		{name: "products without restriction", in: CreatePATInput{ProductIDs: []string{"mine"}}},
		{name: "unknown role cap", in: CreatePATInput{RoleCap: "god"}},
		{name: "expiry in the past", in: CreatePATInput{ExpiresAt: future(-time.Hour)}},
		{name: "expiry not RFC 3339", in: CreatePATInput{ExpiresAt: "2030-01-01"}},
		{name: "no expiry with a maximum lifetime", maxDays: "30"},
		{name: "expiry beyond the maximum lifetime", maxDays: "30", in: CreatePATInput{ExpiresAt: future(31 * 24 * time.Hour)}},
		{
			name: "token creating a broader token",
			ctx: access.WithRestriction(context.Background(),
				&access.Restriction{Grants: access.Grants{access.Account: access.Write, access.Tests: access.Write}}),
			in:        CreatePATInput{Grants: access.Grants{access.Defects: access.Read}},
			forbidden: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stored *model.PAT
			ctx, u, maxDays := tc.ctx, tc.user, tc.maxDays
			if ctx == nil {
				ctx = context.Background()
			}
			if u == nil {
				u = user
			}
			if maxDays == "" {
				maxDays = "0"
			}
			tc.in.Name = "tok"
			_, err := newSvc(&stored, maxDays).Create(ctx, u, tc.in)
			if err == nil {
				t.Fatal("expected an error")
			}
			if errors.Is(err, ErrForbidden) != tc.forbidden {
				t.Fatalf("err = %v, forbidden = %v", err, tc.forbidden)
			}
			if stored != nil {
				t.Fatal("token stored despite the error")
			}
		})
	}

	t.Run("within the maximum lifetime", func(t *testing.T) {
		var stored *model.PAT
		_, err := newSvc(&stored, "30").Create(context.Background(), user,
			CreatePATInput{Name: "tok", ExpiresAt: future(29 * 24 * time.Hour)})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
	})

	t.Run("admin may grant admin and any product", func(t *testing.T) {
		var stored *model.PAT
		admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
		_, err := newSvc(&stored, "0").Create(context.Background(), admin, CreatePATInput{Name: "tok",
			Grants: access.Grants{access.Admin: access.Write}, ProductsRestricted: true, ProductIDs: []string{"other"}})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
	})

	t.Run("token creating a narrower token", func(t *testing.T) {
		var stored *model.PAT
		ctx := access.WithRestriction(context.Background(),
			&access.Restriction{Grants: access.Grants{access.Account: access.Write, access.Tests: access.Write}})
		_, err := newSvc(&stored, "0").Create(ctx, user,
			CreatePATInput{Name: "tok", Grants: access.Grants{access.Tests: access.Read}})
		if err != nil {
			t.Fatalf("Create: %v", err)
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
