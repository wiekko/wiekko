package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// PATService handles personal access token operations.
type PATService struct {
	pats store.PATStore
}

// CreatePATResult contains the data returned after creating a PAT.
// The RawToken is only available at creation time.
type CreatePATResult struct {
	RawToken string
	PAT      *model.PAT
}

// Create generates a new personal access token.
// The raw token is returned exactly once; only its hash is stored.
func (s *PATService) Create(ctx context.Context, userID, name string, expiresAt *string) (*CreatePATResult, error) {
	if name == "" {
		return nil, errors.New("token name is required")
	}

	rawToken, err := generateToken(patTokenBytes)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	rawToken = patPrefix + rawToken

	now := time.Now().UTC().Format(time.RFC3339)
	pat := &model.PAT{
		ID:          uuid.New().String(),
		UserID:      userID,
		Name:        name,
		TokenHash:   hashPATToken(rawToken),
		TokenPrefix: rawToken[:patPrefixLen],
		CreatedAt:   now,
	}
	if expiresAt != nil {
		pat.ExpiresAt = *expiresAt
	}

	if err := s.pats.Create(ctx, pat); err != nil {
		return nil, fmt.Errorf("store token: %w", err)
	}
	return &CreatePATResult{RawToken: rawToken, PAT: pat}, nil
}

// List returns all PATs for a user.
func (s *PATService) List(ctx context.Context, userID string) ([]*model.PAT, error) {
	return s.pats.ListByUserID(ctx, userID)
}

// Revoke marks a PAT as revoked. Only the token's owner may revoke it.
func (s *PATService) Revoke(ctx context.Context, userID, patID string) error {
	pats, err := s.pats.ListByUserID(ctx, userID)
	if err != nil {
		return err
	}
	for _, p := range pats {
		if p.ID == patID {
			if p.RevokedAt != "" {
				return errors.New("token already revoked")
			}
			now := time.Now().UTC().Format(time.RFC3339)
			return s.pats.Revoke(ctx, patID, now)
		}
	}
	return errors.New("token not found")
}
