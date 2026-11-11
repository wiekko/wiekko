package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

const invitationTTL = 7 * 24 * time.Hour // 7 days

// InvitationService handles invitation operations.
type InvitationService struct {
	invitations store.InvitationStore
	authz       *Authorizer
}

// CreateInvitationResult contains the raw token (shown once) and the invitation record.
type CreateInvitationResult struct {
	RawToken   string
	Invitation *model.Invitation
}

// Create generates a new invitation. An invitation to a product carries the
// role the new user gets there and requires the owner role in that product
// (FR-305); an invitation without a product requires an administrator.
func (s *InvitationService) Create(ctx context.Context, inviter *model.User, email, productID, role string) (*CreateInvitationResult, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, errors.New("email is required")
	}
	if productID == "" {
		if role != "" {
			return nil, errors.New("a role can only be given with a product")
		}
		if !inviter.IsAdmin {
			return nil, fmt.Errorf("%w: only administrators can invite users without a product", ErrForbidden)
		}
	} else {
		if roleRank(role) < 0 {
			return nil, fmt.Errorf("invalid role %q", role)
		}
		if _, err := s.authz.RequireRole(ctx, inviter, productID, model.RoleOwner); err != nil {
			return nil, err
		}
	}

	rawToken, err := generateToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	now := time.Now().UTC()
	inv := &model.Invitation{
		ID:        uuid.New().String(),
		Email:     email,
		TokenHash: hashPATToken(rawToken),
		InvitedBy: inviter.ID,
		ProductID: productID,
		Role:      role,
		ExpiresAt: now.Add(invitationTTL).Format(time.RFC3339),
		CreatedAt: now.Format(time.RFC3339),
	}

	if err := s.invitations.Create(ctx, inv); err != nil {
		return nil, fmt.Errorf("store invitation: %w", err)
	}
	return &CreateInvitationResult{RawToken: rawToken, Invitation: inv}, nil
}

// List returns all invitations.
func (s *InvitationService) List(ctx context.Context) ([]*model.Invitation, error) {
	return s.invitations.List(ctx)
}

// Delete removes an invitation.
func (s *InvitationService) Delete(ctx context.Context, id string) error {
	return s.invitations.Delete(ctx, id)
}
