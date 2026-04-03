package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/auth"
	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// UserService handles user account operations.
type UserService struct {
	users       store.UserStore
	sessions    store.SessionStore
	settings    store.SettingStore
	invitations store.InvitationStore
	provider    auth.Provider
}

// RegisterInput contains the fields required to register a new user.
type RegisterInput struct {
	Username    string
	Email       string
	DisplayName string
	Password    string
}

// UpdateProfileInput contains the fields that can be updated on a user profile.
type UpdateProfileInput struct {
	Email       *string
	DisplayName *string
}

// Register creates a new user account (self-registration).
func (s *UserService) Register(ctx context.Context, input RegisterInput) (*model.User, error) {
	enabled, err := s.settings.Get(ctx, "self_registration_enabled")
	if err != nil || enabled != "true" {
		return nil, errors.New("self-registration is disabled")
	}

	hash, err := s.validateAndHashPassword(ctx, input.Password)
	if err != nil {
		return nil, err
	}

	moderation, _ := s.settings.Get(ctx, "moderation_enabled")
	status := model.UserStatusActive
	if moderation == "true" {
		status = model.UserStatusPendingApproval
	}

	now := time.Now().UTC().Format(time.RFC3339)
	user := &model.User{
		ID:           uuid.New().String(),
		Username:     input.Username,
		Email:        input.Email,
		DisplayName:  input.DisplayName,
		PasswordHash: hash,
		IsAdmin:      false,
		Status:       status,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// AcceptInvitation registers a user from an invitation token.
func (s *UserService) AcceptInvitation(ctx context.Context, rawToken string, input RegisterInput) (*model.User, error) {
	tokenHash := hashPATToken(rawToken)
	inv, err := s.invitations.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, errors.New("invalid invitation token")
	}
	if inv.AcceptedAt != "" {
		return nil, errors.New("invitation already accepted")
	}
	expiresAt, err := time.Parse(time.RFC3339, inv.ExpiresAt)
	if err != nil || time.Now().UTC().After(expiresAt) {
		return nil, errors.New("invitation expired")
	}

	hash, err := s.validateAndHashPassword(ctx, input.Password)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	user := &model.User{
		ID:           uuid.New().String(),
		Username:     input.Username,
		Email:        input.Email,
		DisplayName:  input.DisplayName,
		PasswordHash: hash,
		IsAdmin:      false,
		Status:       model.UserStatusActive, // invitation accounts skip moderation (FR-204)
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	if err := s.invitations.MarkAccepted(ctx, inv.ID, now); err != nil {
		return nil, fmt.Errorf("mark invitation accepted: %w", err)
	}
	return user, nil
}

// GetByID returns a user by ID.
func (s *UserService) GetByID(ctx context.Context, id string) (*model.User, error) {
	return s.users.GetByID(ctx, id)
}

// List returns users matching the filter.
func (s *UserService) List(ctx context.Context, filter store.UserFilter) ([]*model.User, error) {
	return s.users.List(ctx, filter)
}

// UpdateProfile updates the user's profile fields.
func (s *UserService) UpdateProfile(ctx context.Context, id string, input UpdateProfileInput) (*model.User, error) {
	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if input.Email != nil {
		user.Email = *input.Email
	}
	if input.DisplayName != nil {
		user.DisplayName = *input.DisplayName
	}
	user.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.users.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// ChangePassword changes a user's own password.
func (s *UserService) ChangePassword(ctx context.Context, id, oldPassword, newPassword string) error {
	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.provider.VerifyPassword(oldPassword, user.PasswordHash); err != nil {
		return errors.New("current password is incorrect")
	}
	hash, err := s.validateAndHashPassword(ctx, newPassword)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, id, hash)
}

// RequestDeletion marks a user's account for deletion.
func (s *UserService) RequestDeletion(ctx context.Context, id string) error {
	moderation, _ := s.settings.Get(ctx, "moderation_enabled")
	if moderation == "true" {
		return s.users.UpdateStatus(ctx, id, model.UserStatusDeletionRequested)
	}
	// No moderation: delete immediately (FR-207).
	return s.users.Delete(ctx, id)
}

// AdminSetPassword sets a new password for any user (admin only).
func (s *UserService) AdminSetPassword(ctx context.Context, targetID, newPassword string) error {
	hash, err := s.validateAndHashPassword(ctx, newPassword)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, targetID, hash)
}

// AdminLockUser locks a user account.
func (s *UserService) AdminLockUser(ctx context.Context, targetID string) error {
	return s.users.UpdateStatus(ctx, targetID, model.UserStatusLocked)
}

// AdminUnlockUser unlocks a user account.
func (s *UserService) AdminUnlockUser(ctx context.Context, targetID string) error {
	return s.users.UpdateStatus(ctx, targetID, model.UserStatusActive)
}

// AdminDeleteUser deletes a user account and all associated data.
func (s *UserService) AdminDeleteUser(ctx context.Context, targetID string) error {
	return s.users.Delete(ctx, targetID)
}

// AdminApproveUser approves a pending user account.
func (s *UserService) AdminApproveUser(ctx context.Context, targetID string) error {
	user, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		return err
	}
	if user.Status != model.UserStatusPendingApproval {
		return errors.New("user is not pending approval")
	}
	return s.users.UpdateStatus(ctx, targetID, model.UserStatusActive)
}

// validateAndHashPassword loads the password policy, validates the password against it,
// and returns the hashed password.
func (s *UserService) validateAndHashPassword(ctx context.Context, password string) (string, error) {
	policy, err := s.loadPasswordPolicy(ctx)
	if err != nil {
		return "", fmt.Errorf("load password policy: %w", err)
	}
	if err := s.provider.ValidatePassword(password, policy); err != nil {
		return "", err
	}
	hash, err := s.provider.HashPassword(password)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return hash, nil
}

func (s *UserService) loadPasswordPolicy(ctx context.Context) (auth.PasswordPolicy, error) {
	policy := auth.PasswordPolicy{
		MinLength:        10,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireDigit:     true,
		RequireSpecial:   false,
	}

	if v, err := s.settings.Get(ctx, "password_min_length"); err == nil {
		if n, err := strconv.Atoi(v); err == nil {
			policy.MinLength = n
		}
	}
	if v, err := s.settings.Get(ctx, "password_require_uppercase"); err == nil {
		policy.RequireUppercase = v == "true"
	}
	if v, err := s.settings.Get(ctx, "password_require_lowercase"); err == nil {
		policy.RequireLowercase = v == "true"
	}
	if v, err := s.settings.Get(ctx, "password_require_digit"); err == nil {
		policy.RequireDigit = v == "true"
	}
	if v, err := s.settings.Get(ctx, "password_require_special"); err == nil {
		policy.RequireSpecial = v == "true"
	}

	return policy, nil
}
