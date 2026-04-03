package service

import (
	"context"
	"errors"
	"testing"

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
