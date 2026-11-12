package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/access"
	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func newAuthorizer(roles *testutil.MockProductRoleStore) *Authorizer {
	return &Authorizer{productRoles: roles}
}

func TestRequireRole(t *testing.T) {
	t.Run("admin bypasses check", func(t *testing.T) {
		authz := newAuthorizer(nil) // store not called for admins
		user := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })

		role, err := authz.RequireRole(context.Background(), user, "product-001", model.RoleOwner)
		if err != nil {
			t.Fatalf("RequireRole: %v", err)
		}
		if role != model.RoleOwner {
			t.Fatalf("role = %q, want %q", role, model.RoleOwner)
		}
	})

	t.Run("user with sufficient role", func(t *testing.T) {
		authz := newAuthorizer(&testutil.MockProductRoleStore{
			GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
				return testutil.MakeProductRole(func(pr *model.ProductRole) { pr.Role = model.RoleDeveloper }), nil
			},
		})
		user := testutil.MakeUser()

		role, err := authz.RequireRole(context.Background(), user, "product-001", model.RoleViewer)
		if err != nil {
			t.Fatalf("RequireRole: %v", err)
		}
		if role != model.RoleDeveloper {
			t.Fatalf("role = %q, want %q", role, model.RoleDeveloper)
		}
	})

	t.Run("user with exact role", func(t *testing.T) {
		authz := newAuthorizer(&testutil.MockProductRoleStore{
			GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
				return testutil.MakeProductRole(func(pr *model.ProductRole) { pr.Role = model.RoleTester }), nil
			},
		})
		user := testutil.MakeUser()

		role, err := authz.RequireRole(context.Background(), user, "product-001", model.RoleTester)
		if err != nil {
			t.Fatalf("RequireRole: %v", err)
		}
		if role != model.RoleTester {
			t.Fatalf("role = %q, want %q", role, model.RoleTester)
		}
	})

	t.Run("user with insufficient role", func(t *testing.T) {
		authz := newAuthorizer(&testutil.MockProductRoleStore{
			GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
				return testutil.MakeProductRole(func(pr *model.ProductRole) { pr.Role = model.RoleViewer }), nil
			},
		})
		user := testutil.MakeUser()

		_, err := authz.RequireRole(context.Background(), user, "product-001", model.RoleOwner)
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("user with no role", func(t *testing.T) {
		authz := newAuthorizer(&testutil.MockProductRoleStore{
			GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
				return nil, errors.New("not found")
			},
		})
		user := testutil.MakeUser()

		_, err := authz.RequireRole(context.Background(), user, "product-001", model.RoleViewer)
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestRequireRoleTokenRestriction(t *testing.T) {
	owner := &testutil.MockProductRoleStore{
		GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
			return testutil.MakeProductRole(func(pr *model.ProductRole) { pr.Role = model.RoleOwner }), nil
		},
	}
	tokenCtx := func(r *access.Restriction) context.Context {
		return access.WithRestriction(context.Background(), r)
	}

	t.Run("product outside the token's set", func(t *testing.T) {
		ctx := tokenCtx(&access.Restriction{ProductsRestricted: true, Products: []string{"other"}})
		_, err := newAuthorizer(owner).RequireRole(ctx, testutil.MakeUser(), "product-001", model.RoleViewer)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("product outside the token's set, admin", func(t *testing.T) {
		ctx := tokenCtx(&access.Restriction{ProductsRestricted: true})
		admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
		_, err := newAuthorizer(nil).RequireRole(ctx, admin, "product-001", model.RoleViewer)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("role cap lowers the effective role", func(t *testing.T) {
		ctx := tokenCtx(&access.Restriction{RoleCap: model.RoleTester})
		role, err := newAuthorizer(owner).RequireRole(ctx, testutil.MakeUser(), "product-001", model.RoleViewer)
		if err != nil || role != model.RoleTester {
			t.Fatalf("role = %q, %v; want tester", role, err)
		}
		_, err = newAuthorizer(owner).RequireRole(ctx, testutil.MakeUser(), "product-001", model.RoleDeveloper)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("capped role passed a developer check: %v", err)
		}
	})

	t.Run("role cap applies to admins", func(t *testing.T) {
		ctx := tokenCtx(&access.Restriction{RoleCap: model.RoleViewer})
		admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
		role, err := newAuthorizer(nil).RequireRole(ctx, admin, "product-001", model.RoleViewer)
		if err != nil || role != model.RoleViewer {
			t.Fatalf("role = %q, %v; want viewer", role, err)
		}
	})

	t.Run("allowed product keeps the role", func(t *testing.T) {
		ctx := tokenCtx(&access.Restriction{ProductsRestricted: true, Products: []string{"product-001"}})
		role, err := newAuthorizer(owner).RequireRole(ctx, testutil.MakeUser(), "product-001", model.RoleOwner)
		if err != nil || role != model.RoleOwner {
			t.Fatalf("role = %q, %v; want owner", role, err)
		}
	})
}

func TestRestrictProductRoles(t *testing.T) {
	list := []ProductWithRole{
		{Product: &model.Product{ID: "a"}, Role: model.RoleOwner},
		{Product: &model.Product{ID: "b"}, Role: "admin"},
		{Product: &model.Product{ID: "c"}, Role: model.RoleViewer},
	}
	if got := restrictProductRoles(context.Background(), list); len(got) != 3 {
		t.Fatalf("sessions must see everything, got %d", len(got))
	}
	ctx := access.WithRestriction(context.Background(), &access.Restriction{
		RoleCap: model.RoleTester, ProductsRestricted: true, Products: []string{"b", "c"}})
	got := restrictProductRoles(ctx, list)
	if len(got) != 2 || got[0].Product.ID != "b" || got[0].Role != model.RoleTester || got[1].Role != model.RoleViewer {
		t.Fatalf("got %+v", got)
	}
}
