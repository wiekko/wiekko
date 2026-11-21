// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"wiekko.net/internal/model"
	"wiekko.net/internal/testutil"
)

func TestInvitationCreate(t *testing.T) {
	admin := testutil.MakeUser(func(u *model.User) { u.ID = "admin-1"; u.IsAdmin = true })

	t.Run("valid", func(t *testing.T) {
		var stored *model.Invitation
		svc := &InvitationService{
			invitations: &testutil.MockInvitationStore{
				CreateFunc: func(_ context.Context, inv *model.Invitation) error { stored = inv; return nil },
			},
			authz: &Authorizer{productRoles: &testutil.MockProductRoleStore{}},
		}
		result, err := svc.Create(context.Background(), admin, "new@example.com", "prod-1", "developer")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if result.RawToken == "" {
			t.Fatal("RawToken is empty")
		}
		if stored == nil {
			t.Fatal("invitation was not stored")
		}
		if stored.Email != "new@example.com" {
			t.Fatalf("Email = %q, want %q", stored.Email, "new@example.com")
		}
		if stored.InvitedBy != "admin-1" {
			t.Fatalf("InvitedBy = %q, want %q", stored.InvitedBy, "admin-1")
		}
		if stored.ProductID != "prod-1" {
			t.Fatalf("ProductID = %q, want %q", stored.ProductID, "prod-1")
		}
		if stored.Role != "developer" {
			t.Fatalf("Role = %q, want %q", stored.Role, "developer")
		}
		if stored.TokenHash == "" {
			t.Fatal("TokenHash is empty")
		}

		// Verify TTL is approximately 7 days.
		expiresAt, err := time.Parse(time.RFC3339, stored.ExpiresAt)
		if err != nil {
			t.Fatalf("parse ExpiresAt: %v", err)
		}
		expected := time.Now().UTC().Add(7 * 24 * time.Hour)
		diff := expiresAt.Sub(expected)
		if diff < -time.Minute || diff > time.Minute {
			t.Fatalf("ExpiresAt is off by %v from expected 7-day TTL", diff)
		}
	})

	t.Run("store error", func(t *testing.T) {
		svc := &InvitationService{
			invitations: &testutil.MockInvitationStore{
				CreateFunc: func(_ context.Context, _ *model.Invitation) error { return errors.New("db error") },
			},
		}
		_, err := svc.Create(context.Background(), admin, "a@b.com", "", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// A product invitation hands out a role, so only that product's owners (and
// administrators) may create one; anyone else could invite themselves in.
func TestInvitationCreateAuthorization(t *testing.T) {
	roles := map[string]string{"owner-1": model.RoleOwner, "dev-1": model.RoleDeveloper}
	newSvc := func() *InvitationService {
		return &InvitationService{
			invitations: &testutil.MockInvitationStore{
				CreateFunc: func(_ context.Context, _ *model.Invitation) error { return nil },
			},
			authz: &Authorizer{productRoles: &testutil.MockProductRoleStore{
				GetByUserAndProductFunc: func(_ context.Context, userID, productID string) (*model.ProductRole, error) {
					if role, ok := roles[userID]; ok && productID == "prod-1" {
						return &model.ProductRole{UserID: userID, ProductID: productID, Role: role}, nil
					}
					return nil, errors.New("not found")
				},
			}},
		}
	}
	user := func(id string, admin bool) *model.User {
		return testutil.MakeUser(func(u *model.User) { u.ID = id; u.IsAdmin = admin })
	}

	tests := []struct {
		name      string
		inviter   *model.User
		email     string
		productID string
		role      string
		wantErr   bool
		forbidden bool
	}{
		{"owner invites to own product", user("owner-1", false), "a@b.com", "prod-1", model.RoleOwner, false, false},
		{"developer may not invite", user("dev-1", false), "a@b.com", "prod-1", model.RoleViewer, true, true},
		{"non-member may not invite", user("stranger", false), "a@b.com", "prod-1", model.RoleViewer, true, true},
		{"owner of another product may not invite", user("owner-1", false), "a@b.com", "prod-2", model.RoleViewer, true, true},
		{"admin invites to any product", user("root", true), "a@b.com", "prod-2", model.RoleTester, false, false},
		{"unknown role", user("owner-1", false), "a@b.com", "prod-1", "superuser", true, false},
		{"product without role", user("owner-1", false), "a@b.com", "prod-1", "", true, false},
		{"admin invites without product", user("root", true), "a@b.com", "", "", false, false},
		{"non-admin may not invite without product", user("owner-1", false), "a@b.com", "", "", true, true},
		{"role without product", user("root", true), "a@b.com", "", model.RoleViewer, true, false},
		{"empty email", user("root", true), "  ", "", "", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newSvc().Create(context.Background(), tt.inviter, tt.email, tt.productID, tt.role)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.forbidden && !errors.Is(err, ErrForbidden) {
				t.Fatalf("err = %v, want ErrForbidden", err)
			}
		})
	}
}

func TestInvitationList(t *testing.T) {
	invs := []*model.Invitation{testutil.MakeInvitation()}
	svc := &InvitationService{
		invitations: &testutil.MockInvitationStore{
			ListFunc: func(_ context.Context) ([]*model.Invitation, error) { return invs, nil },
		},
	}
	result, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
}

func TestInvitationDelete(t *testing.T) {
	var deletedID string
	svc := &InvitationService{
		invitations: &testutil.MockInvitationStore{
			DeleteFunc: func(_ context.Context, id string) error { deletedID = id; return nil },
		},
	}
	if err := svc.Delete(context.Background(), "inv-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if deletedID != "inv-1" {
		t.Fatalf("deleted = %q, want %q", deletedID, "inv-1")
	}
}
