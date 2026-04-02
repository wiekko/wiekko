package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func TestInvitationCreate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		var stored *model.Invitation
		svc := &InvitationService{
			invitations: &testutil.MockInvitationStore{
				CreateFunc: func(_ context.Context, inv *model.Invitation) error { stored = inv; return nil },
			},
		}
		result, err := svc.Create(context.Background(), "admin-1", "new@example.com", "prod-1", "developer")
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
		_, err := svc.Create(context.Background(), "admin", "a@b.com", "", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})
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
