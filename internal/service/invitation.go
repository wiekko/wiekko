package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

const invitationTTL = 7 * 24 * time.Hour // 7 days

// InvitationService handles invitation operations.
type InvitationService struct {
	invitations store.InvitationStore
}

// CreateInvitationResult contains the raw token (shown once) and the invitation record.
type CreateInvitationResult struct {
	RawToken   string
	Invitation *model.Invitation
}

// Create generates a new invitation.
func (s *InvitationService) Create(ctx context.Context, invitedBy, email, productID, role string) (*CreateInvitationResult, error) {
	rawToken, err := generateToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	now := time.Now().UTC()
	inv := &model.Invitation{
		ID:        uuid.New().String(),
		Email:     email,
		TokenHash: hashPATToken(rawToken),
		InvitedBy: invitedBy,
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
