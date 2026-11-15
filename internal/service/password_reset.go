package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// Password reset (SC-110): a link sent to the address stored on the account,
// usable once, for passwordResetTTL.
const passwordResetTTL = time.Hour

// Limits for asking for a reset link. The form is public, so it is limited
// per client address; each account additionally gets only a few emails per
// window, whoever asks. Running into the account limit is not reported:
// the answer must not tell whether an account exists.
const (
	PasswordResetWindow       = time.Hour
	passwordResetsPerAddress  = 5
	passwordResetMailsPerUser = 3
)

// ErrPasswordResetInvalid is returned for a reset link that is unknown, used,
// expired or belongs to an account that cannot sign in.
var ErrPasswordResetInvalid = errors.New("this password reset link is not valid any more; ask for a new one")

// PasswordResetService lets users who forgot their password set a new one
// through a link sent by email. Without mail it is disabled.
type PasswordResetService struct {
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx       store.Transactor
	users    store.UserStore
	resets   store.PasswordResetStore
	accounts *UserService
	mail     *Mailer

	// ipLimiter bounds requests per client address, userLimiter emails per
	// account; nil disables them.
	ipLimiter   *attemptLimiter
	userLimiter *attemptLimiter
}

// Enabled reports whether password resets are offered: only with mail.
func (s *PasswordResetService) Enabled() bool { return s != nil && s.mail.Enabled() }

// Request asks for a reset link for the account with the given username or
// email address. The link goes to the address stored on the account, never to
// one typed into the form, and only for an active account.
//
// Request returns nil whether or not such an account exists, and sends the
// email in the background, so neither the answer nor its timing tells which
// accounts there are. It returns an error only when the feature is off, the
// input is empty, or clientIP has asked too often (an *AttemptsBlockedError,
// which depends on the asker alone).
func (s *PasswordResetService) Request(ctx context.Context, identifier, clientIP string) error {
	if !s.Enabled() {
		return ErrMailDisabled
	}
	if clientIP != "" {
		key := limiterIP(clientIP)
		if err := s.ipLimiter.check(key); err != nil {
			return err
		}
		s.ipLimiter.add(key)
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return inputError(errors.New("enter your username or email address"))
	}
	if len(identifier) > maxEmailLength {
		return nil
	}

	user, err := s.findUser(ctx, identifier)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.ErrorContext(ctx, "password reset: look up account", "err", err)
		}
		return nil
	}
	if user.Status != model.UserStatusActive || user.Email == "" {
		return nil
	}
	userKey := "user:" + user.ID
	if s.userLimiter.check(userKey) != nil {
		return nil
	}
	s.userLimiter.add(userKey)

	rawToken, err := s.issue(ctx, user.ID)
	if err != nil {
		slog.ErrorContext(ctx, "password reset: store token", "err", err)
		return nil
	}
	s.mail.enqueue(mailPurposePasswordReset, s.mail.passwordResetMessage(user.Email, rawToken, passwordResetTTL))
	slog.InfoContext(ctx, "password reset requested", "user_id", user.ID)
	return nil
}

// findUser looks identifier up as an email address if it has an "@", then as
// a username.
func (s *PasswordResetService) findUser(ctx context.Context, identifier string) (*model.User, error) {
	if strings.Contains(identifier, "@") {
		if u, err := s.users.GetByEmail(ctx, identifier); err == nil {
			return u, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return s.users.GetByUsername(ctx, identifier)
}

// issue replaces the user's outstanding reset links by a new one and returns
// its raw token. Only the newest link works.
func (s *PasswordResetService) issue(ctx context.Context, userID string) (string, error) {
	rawToken, err := generateToken(32)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC().Truncate(time.Second)
	// The new link replaces the old ones in one step.
	err = inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.resets.DeleteByUserID(ctx, userID); err != nil {
			return err
		}
		return s.resets.Create(ctx, &model.PasswordResetToken{
			ID:        uuid.NewString(),
			UserID:    userID,
			TokenHash: hashPATToken(rawToken),
			CreatedAt: store.FormatTimestamp(now),
			ExpiresAt: store.FormatTimestamp(now.Add(passwordResetTTL)),
		})
	})
	if err != nil {
		return "", err
	}
	return rawToken, nil
}

// Check reports whether rawToken is a usable reset link, for the page it
// opens.
func (s *PasswordResetService) Check(ctx context.Context, rawToken string) error {
	if !s.Enabled() {
		return ErrPasswordResetInvalid
	}
	_, _, err := s.usable(ctx, rawToken)
	return err
}

// Reset sets a new password through a reset link. The password policy is
// checked before the link is used up, so a rejected password leaves the link
// usable. Afterwards every session of the user ends; the user signs in anew
// (with the second factor, if set up).
func (s *PasswordResetService) Reset(ctx context.Context, rawToken, newPassword string) error {
	if !s.Enabled() {
		return ErrPasswordResetInvalid
	}
	tok, user, err := s.usable(ctx, rawToken)
	if err != nil {
		return err
	}
	hash, err := s.accounts.validateAndHashPassword(ctx, newPassword)
	if err != nil {
		return err
	}
	// Using up the link, setting the password and ending the old sessions
	// happen together: a failure must neither burn the link with the password
	// unchanged nor change the password and leave a thief's session alive.
	err = inTx(ctx, s.tx, func(ctx context.Context) error {
		ok, err := s.resets.Consume(ctx, tok.ID, store.FormatTimestamp(time.Now().UTC().Truncate(time.Second)))
		if err != nil {
			return fmt.Errorf("use reset link: %w", err)
		}
		if !ok {
			// Used or expired between the check above and now.
			return ErrPasswordResetInvalid
		}
		if err := s.users.UpdatePassword(ctx, user.ID, hash); err != nil {
			return fmt.Errorf("set password: %w", err)
		}
		if err := s.resets.DeleteByUserID(ctx, user.ID); err != nil {
			return fmt.Errorf("remove used reset links: %w", err)
		}
		return s.accounts.endSessions(ctx, user.ID, "")
	})
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "password reset", "user_id", user.ID)
	return nil
}

// usable resolves rawToken to an unused, unexpired reset token of an active
// user.
func (s *PasswordResetService) usable(ctx context.Context, rawToken string) (*model.PasswordResetToken, *model.User, error) {
	if rawToken == "" || len(rawToken) > 128 {
		return nil, nil, ErrPasswordResetInvalid
	}
	tok, err := s.resets.GetByTokenHash(ctx, hashPATToken(rawToken))
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, nil, fmt.Errorf("look up reset link: %w", err)
		}
		return nil, nil, ErrPasswordResetInvalid
	}
	if tok.UsedAt != "" {
		return nil, nil, ErrPasswordResetInvalid
	}
	expires, err := time.Parse(time.RFC3339Nano, tok.ExpiresAt)
	if err != nil || !time.Now().Before(expires) {
		return nil, nil, ErrPasswordResetInvalid
	}
	user, err := s.users.GetByID(ctx, tok.UserID)
	if err != nil || user.Status != model.UserStatusActive {
		return nil, nil, ErrPasswordResetInvalid
	}
	return tok, user, nil
}
