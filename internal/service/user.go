// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"wiekko.net/internal/access"
	"wiekko.net/internal/auth"
	"wiekko.net/internal/model"
	"wiekko.net/internal/store"
)

// UserService handles user account operations.
type UserService struct {
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx           store.Transactor
	users        store.UserStore
	sessions     store.SessionStore
	settings     store.SettingStore
	invitations  store.InvitationStore
	products     store.ProductStore
	productRoles store.ProductRoleStore
	provider     auth.Provider

	// regLimiter bounds account creation per client address; nil disables it.
	regLimiter *attemptLimiter
}

// Errors returned when a new account would clash with an existing one. Their
// messages are shown to the person registering.
var (
	ErrUsernameTaken = errors.New("this username is already taken")
	ErrEmailTaken    = errors.New("an account with this email address already exists")
)

// AccountInputError is a problem with what someone entered to create an
// account: a missing field, a password the policy rejects. Its message is
// meant for that person.
type AccountInputError struct{ Err error }

func (e *AccountInputError) Error() string { return e.Err.Error() }
func (e *AccountInputError) Unwrap() error { return e.Err }

func inputError(err error) error { return &AccountInputError{Err: err} }

// Errors returned for invitation tokens that cannot be used. Their messages
// are shown to the person holding the link.
var (
	ErrInvitationInvalid  = errors.New("invalid invitation token")
	ErrInvitationAccepted = errors.New("invitation already accepted")
	ErrInvitationExpired  = errors.New("invitation expired")
)

// InvitationDetails describes a usable invitation to the person holding its link.
type InvitationDetails struct {
	Email       string
	ProductName string // empty for an invitation to the installation only
	Role        string // empty for an invitation to the installation only
}

// RegisterInput contains the fields required to register a new user.
type RegisterInput struct {
	Username    string
	Email       string
	DisplayName string
	Password    string
	// ClientIP is the address the request came from, for the limit on
	// account creation per address. Empty: not limited (internal callers).
	ClientIP string
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
		return nil, inputError(errors.New("self-registration is disabled"))
	}
	if err := s.countRegistration(input.ClientIP); err != nil {
		return nil, err
	}

	input, err = normalizeRegisterInput(input)
	if err != nil {
		return nil, err
	}
	if err := s.ensureAvailable(ctx, input.Username, input.Email); err != nil {
		return nil, err
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

// LookupInvitation returns what a usable invitation offers, for the page its
// link opens. Accepted, expired and unknown tokens are reported as errors.
func (s *UserService) LookupInvitation(ctx context.Context, rawToken string) (*InvitationDetails, error) {
	inv, err := s.usableInvitation(ctx, rawToken)
	if err != nil {
		return nil, err
	}
	details := &InvitationDetails{Email: inv.Email, Role: inv.Role}
	if inv.ProductID != "" && s.products != nil {
		if p, err := s.products.GetByID(ctx, inv.ProductID); err == nil {
			details.ProductName = p.Name
		}
	}
	return details, nil
}

// AcceptInvitation registers a user from an invitation token. A product
// invitation also makes the new user a member of that product with the
// invited role (FR-77).
func (s *UserService) AcceptInvitation(ctx context.Context, rawToken string, input RegisterInput) (*model.User, error) {
	if err := s.countRegistration(input.ClientIP); err != nil {
		return nil, err
	}
	inv, err := s.usableInvitation(ctx, rawToken)
	if err != nil {
		return nil, err
	}

	// The invitation was sent to one address; the account gets that address.
	if inv.Email != "" {
		if strings.TrimSpace(input.Email) == "" {
			input.Email = inv.Email
		} else if !strings.EqualFold(strings.TrimSpace(input.Email), inv.Email) {
			return nil, inputError(fmt.Errorf("this invitation is for %s; register with that email address", inv.Email))
		}
	}
	input, err = normalizeRegisterInput(input)
	if err != nil {
		return nil, err
	}
	if err := s.ensureAvailable(ctx, input.Username, input.Email); err != nil {
		return nil, err
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
		Status:       model.UserStatusActive, // invitation accounts skip moderation (FR-69)
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	// The account, its membership and the used-up invitation stand or fall
	// together. MarkAccepted only succeeds for an invitation nobody accepted
	// meanwhile, so of two concurrent accepts the second rolls back.
	err = inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.users.Create(ctx, user); err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		if inv.ProductID != "" {
			if err := s.productRoles.Create(ctx, &model.ProductRole{
				ID:        uuid.New().String(),
				UserID:    user.ID,
				ProductID: inv.ProductID,
				Role:      inv.Role,
				CreatedAt: now,
			}); err != nil {
				return fmt.Errorf("add product membership: %w", err)
			}
		}
		if err := s.invitations.MarkAccepted(ctx, inv.ID, now); err != nil {
			if errors.Is(err, store.ErrNotChanged) {
				return ErrInvitationAccepted
			}
			return fmt.Errorf("mark invitation accepted: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}

// countRegistration counts one attempt to create an account from clientIP and
// returns an *AttemptsBlockedError once the address has used up its attempts
// (registrationMaxPerAddress within RegistrationWindow). Every attempt counts,
// whether it succeeds or not: each may cost a password hash.
func (s *UserService) countRegistration(clientIP string) error {
	if clientIP == "" {
		return nil
	}
	key := limiterIP(clientIP)
	if err := s.regLimiter.check(key); err != nil {
		return err
	}
	s.regLimiter.add(key)
	return nil
}

// usableInvitation resolves a raw invitation token to an invitation that has
// neither been accepted nor expired.
func (s *UserService) usableInvitation(ctx context.Context, rawToken string) (*model.Invitation, error) {
	if rawToken == "" {
		return nil, ErrInvitationInvalid
	}
	inv, err := s.invitations.GetByTokenHash(ctx, hashPATToken(rawToken))
	if err != nil {
		return nil, ErrInvitationInvalid
	}
	if inv.AcceptedAt != "" {
		return nil, ErrInvitationAccepted
	}
	expiresAt, err := time.Parse(time.RFC3339, inv.ExpiresAt)
	if err != nil || time.Now().UTC().After(expiresAt) {
		return nil, ErrInvitationExpired
	}
	return inv, nil
}

// normalizeRegisterInput trims the identifying fields, checks the required
// ones and falls back to the username as display name.
func normalizeRegisterInput(input RegisterInput) (RegisterInput, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.Username == "" || input.Email == "" || input.Password == "" {
		return input, inputError(errors.New("username, email and password are required"))
	}
	if input.DisplayName == "" {
		input.DisplayName = input.Username
	}
	if err := validateEmail(input.Email); err != nil {
		return input, err
	}
	if err := validateDisplayName(input.DisplayName); err != nil {
		return input, err
	}
	return input, nil
}

const (
	maxEmailLength       = 254 // RFC 5321
	maxDisplayNameLength = 100
)

// validateEmail checks that email is a plain address ("ada@example.com"),
// already trimmed. The address is not verified â€” wiekko sends no email â€” but
// it is shown to administrators and owners and must not be free text.
func validateEmail(email string) error {
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > maxEmailLength {
		return inputError(errors.New("enter a valid email address, like name@example.com"))
	}
	return nil
}

func validateDisplayName(name string) error {
	if name == "" {
		return inputError(errors.New("the display name must not be empty"))
	}
	if utf8.RuneCountInString(name) > maxDisplayNameLength {
		return inputError(fmt.Errorf("the display name must not be longer than %d characters", maxDisplayNameLength))
	}
	return nil
}

// ensureAvailable reports a clash with an existing account before the insert,
// so the person registering gets a clear message instead of a database error.
func (s *UserService) ensureAvailable(ctx context.Context, username, email string) error {
	if _, err := s.users.GetByUsername(ctx, username); err == nil {
		return ErrUsernameTaken
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check username: %w", err)
	}
	if _, err := s.users.GetByEmail(ctx, email); err == nil {
		return ErrEmailTaken
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check email: %w", err)
	}
	return nil
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
		email := strings.TrimSpace(*input.Email)
		if err := validateEmail(email); err != nil {
			return nil, err
		}
		if !strings.EqualFold(email, user.Email) {
			// Checked here so the person gets a clear message, not the
			// database's complaint about its unique index.
			if other, err := s.users.GetByEmail(ctx, email); err == nil && other.ID != user.ID {
				return nil, ErrEmailTaken
			} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("check email: %w", err)
			}
		}
		user.Email = email
	}
	if input.DisplayName != nil {
		name := strings.TrimSpace(*input.DisplayName)
		if err := validateDisplayName(name); err != nil {
			return nil, err
		}
		user.DisplayName = name
	}
	user.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.users.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// ChangePassword changes a user's own password and signs the user out
// everywhere else: whoever may have known the old password could also hold a
// session. keepSessionID is the session the change was made from, which stays
// valid; pass "" when there is none (a request with an access token).
func (s *UserService) ChangePassword(ctx context.Context, id, oldPassword, newPassword, keepSessionID string) error {
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
	if err := s.users.UpdatePassword(ctx, id, hash); err != nil {
		return err
	}
	return s.endSessions(ctx, id, keepSessionID)
}

// endSessions deletes the user's web sessions except keepSessionID (none if
// empty), after a password change. Access tokens are not affected.
func (s *UserService) endSessions(ctx context.Context, userID, keepSessionID string) error {
	var err error
	if keepSessionID == "" {
		err = s.sessions.DeleteByUserID(ctx, userID)
	} else {
		err = s.sessions.DeleteByUserIDExcept(ctx, userID, keepSessionID)
	}
	if err != nil {
		return fmt.Errorf("the password was changed, but signing out other sessions failed: %w", err)
	}
	return nil
}

// ErrLastAdmin is returned when an action would leave the instance without an
// active administrator. Nobody could then manage users or settings, and there
// is no way back in short of editing the database.
var ErrLastAdmin = errors.New("this is the only active administrator and cannot be locked or deleted")

// ensureNotLastAdmin refuses to take the last active administrator out of
// service (by locking or deleting the account).
func (s *UserService) ensureNotLastAdmin(ctx context.Context, targetID string) error {
	// Inside the caller's transaction, the lock keeps two administrators from
	// removing each other at the same time and leaving none.
	if err := lock(ctx, s.tx, lockAdmins); err != nil {
		return err
	}
	target, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		return err
	}
	if !target.IsAdmin || target.Status != model.UserStatusActive {
		return nil
	}
	active, err := s.users.List(ctx, store.UserFilter{Status: model.UserStatusActive})
	if err != nil {
		return fmt.Errorf("list administrators: %w", err)
	}
	for _, u := range active {
		if u.IsAdmin && u.ID != targetID {
			return nil
		}
	}
	return ErrLastAdmin
}

// RequestDeletion marks a user's account for deletion. From a web session it
// needs the user's current password as confirmation: deleting an account
// cannot be undone, and a session alone (an unlocked laptop) must not be
// enough. A request made with a token passes password as "".
func (s *UserService) RequestDeletion(ctx context.Context, id, password string) error {
	if access.FromContext(ctx) == nil {
		user, err := s.users.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if err := confirmPassword(s.provider, user, password); err != nil {
			return err
		}
	}
	moderation, _ := s.settings.Get(ctx, "moderation_enabled")
	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.ensureNotLastAdmin(ctx, id); err != nil {
			return err
		}
		if moderation == "true" {
			return s.users.UpdateStatus(ctx, id, model.UserStatusDeletionRequested)
		}
		// No moderation: delete immediately (FR-72).
		return s.users.Delete(ctx, id)
	})
}

// AdminSetPassword sets a new password for any user (admin only) and ends all
// of that user's sessions. keepSessionID is the administrator's own session:
// it only matches when administrators reset their own password, which then
// does not sign them out.
func (s *UserService) AdminSetPassword(ctx context.Context, targetID, newPassword, keepSessionID string) error {
	hash, err := s.validateAndHashPassword(ctx, newPassword)
	if err != nil {
		return err
	}
	// The new password and the end of the old sessions go together.
	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.users.UpdatePassword(ctx, targetID, hash); err != nil {
			return err
		}
		return s.endSessions(ctx, targetID, keepSessionID)
	})
}

// AdminLockUser locks a user account.
func (s *UserService) AdminLockUser(ctx context.Context, targetID string) error {
	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.ensureNotLastAdmin(ctx, targetID); err != nil {
			return err
		}
		return s.users.UpdateStatus(ctx, targetID, model.UserStatusLocked)
	})
}

// AdminUnlockUser unlocks a user account.
func (s *UserService) AdminUnlockUser(ctx context.Context, targetID string) error {
	return s.users.UpdateStatus(ctx, targetID, model.UserStatusActive)
}

// AdminDeleteUser deletes a user account and all associated data.
func (s *UserService) AdminDeleteUser(ctx context.Context, targetID string) error {
	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.ensureNotLastAdmin(ctx, targetID); err != nil {
			return err
		}
		return s.users.Delete(ctx, targetID)
	})
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
		return "", inputError(err)
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
