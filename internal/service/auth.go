// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"nexmap.net/internal/access"
	"nexmap.net/internal/auth"
	"nexmap.net/internal/config"
	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

const (
	sessionTokenBytes = 32
	csrfTokenBytes    = 32
	patTokenBytes     = 40
	patPrefix         = "nxm_"
	patPrefixLen      = 8 // "nxm_" + 4 hex chars
)

// AuthService handles authentication, session management, and PAT validation.
type AuthService struct {
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx       store.Transactor
	users    store.UserStore
	sessions store.SessionStore
	pats     store.PATStore
	provider auth.Provider
	cfg      *config.Config

	twoFactor      *TwoFactorService
	twoFactorStore store.TwoFactorStore

	// limiter blocks clients after repeated wrong passwords; nil disables it.
	limiter *loginLimiter
}

// LoginResult contains the data returned after a login attempt. When the user
// has two-factor authentication enabled, the password step yields no session:
// TOTPRequired is set and ChallengeToken must be presented to CompleteTOTPLogin
// together with a TOTP or recovery code.
type LoginResult struct {
	SessionID      string
	CSRFToken      string
	User           *model.User
	TOTPRequired   bool
	ChallengeToken string
}

const (
	// LoginChallengeTTL bounds how long the second login step may take.
	LoginChallengeTTL = 5 * time.Minute
	// LoginChallengeMaxAttempts caps code guesses per password login.
	LoginChallengeMaxAttempts = 5
)

// ErrLoginChallengeInvalid is returned when the second-step challenge is
// unknown, expired, or exhausted; the user has to start over with the password.
var ErrLoginChallengeInvalid = errors.New("login expired; please sign in again")

// Login authenticates a user and creates a session, or, for users with
// two-factor authentication, a pending login challenge.
func (s *AuthService) Login(ctx context.Context, username, password, ip, userAgent string) (*LoginResult, error) {
	// Checked before the password, so a blocked client costs no hashing.
	if err := s.limiter.check(ip, username); err != nil {
		return nil, err
	}
	identity, err := s.provider.Authenticate(ctx, username, password)
	if err != nil {
		s.limiter.fail(ip, username)
		return nil, err
	}
	s.limiter.succeed(ip, username)

	user, err := s.users.GetByID(ctx, identity.UserID)
	if err != nil {
		return nil, fmt.Errorf("fetch user: %w", err)
	}

	if user.TOTPEnabled {
		return s.createChallenge(ctx, user, ip, userAgent)
	}
	return s.createSession(ctx, user, ip, userAgent)
}

// CompleteTOTPLogin finishes a two-factor login: it checks the code against
// the user behind challengeToken and, on success, creates the session.
// Wrong codes count against the challenge; after LoginChallengeMaxAttempts
// the challenge is discarded and ErrLoginChallengeInvalid returned.
func (s *AuthService) CompleteTOTPLogin(ctx context.Context, challengeToken, code, ip, userAgent string) (*LoginResult, error) {
	if challengeToken == "" || s.twoFactorStore == nil {
		return nil, ErrLoginChallengeInvalid
	}
	id := hashPATToken(challengeToken)
	challenge, err := s.twoFactorStore.GetChallenge(ctx, id)
	if err != nil {
		return nil, ErrLoginChallengeInvalid
	}
	expiresAt, err := time.Parse(time.RFC3339, challenge.ExpiresAt)
	if err != nil || time.Now().UTC().After(expiresAt) {
		_ = s.twoFactorStore.DeleteChallenge(ctx, id)
		return nil, ErrLoginChallengeInvalid
	}

	// Count the attempt before checking the code, so concurrent guesses
	// cannot slip past the limit.
	attempts, err := s.twoFactorStore.IncrementChallengeAttempts(ctx, id)
	if err != nil {
		return nil, ErrLoginChallengeInvalid
	}
	if attempts > LoginChallengeMaxAttempts {
		_ = s.twoFactorStore.DeleteChallenge(ctx, id)
		return nil, ErrLoginChallengeInvalid
	}

	user, err := s.users.GetByID(ctx, challenge.UserID)
	if err != nil {
		return nil, ErrLoginChallengeInvalid
	}
	if user.Status != model.UserStatusActive {
		_ = s.twoFactorStore.DeleteChallenge(ctx, id)
		return nil, errors.New("account is not active")
	}
	if !user.TOTPEnabled {
		// 2FA was reset by an administrator meanwhile; the password step
		// already succeeded, so the login can proceed.
		_ = s.twoFactorStore.DeleteChallenge(ctx, id)
		return s.createSession(ctx, user, ip, userAgent)
	}

	if err := s.twoFactor.Verify(ctx, user, code); err != nil {
		if attempts >= LoginChallengeMaxAttempts {
			_ = s.twoFactorStore.DeleteChallenge(ctx, id)
			return nil, ErrLoginChallengeInvalid
		}
		return nil, err
	}

	_ = s.twoFactorStore.DeleteChallenge(ctx, id)
	return s.createSession(ctx, user, ip, userAgent)
}

func (s *AuthService) createChallenge(ctx context.Context, user *model.User, ip, userAgent string) (*LoginResult, error) {
	if s.twoFactorStore == nil {
		return nil, errors.New("two-factor authentication is not available")
	}
	token, err := generateToken(sessionTokenBytes)
	if err != nil {
		return nil, fmt.Errorf("generate challenge token: %w", err)
	}
	now := time.Now().UTC()
	// Opportunistic cleanup keeps the table from growing with abandoned logins.
	_ = s.twoFactorStore.DeleteExpiredChallenges(ctx, now.Format(time.RFC3339))
	challenge := &model.LoginChallenge{
		ID:        hashPATToken(token),
		UserID:    user.ID,
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(LoginChallengeTTL).Format(time.RFC3339),
		IPAddress: ip,
		UserAgent: userAgent,
	}
	if err := s.twoFactorStore.CreateChallenge(ctx, challenge); err != nil {
		return nil, fmt.Errorf("create login challenge: %w", err)
	}
	return &LoginResult{User: user, TOTPRequired: true, ChallengeToken: token}, nil
}

func (s *AuthService) createSession(ctx context.Context, user *model.User, ip, userAgent string) (*LoginResult, error) {
	sessionID, err := generateToken(sessionTokenBytes)
	if err != nil {
		return nil, fmt.Errorf("generate session token: %w", err)
	}
	csrfToken, err := generateToken(csrfTokenBytes)
	if err != nil {
		return nil, fmt.Errorf("generate CSRF token: %w", err)
	}

	now := time.Now().UTC()
	session := &model.Session{
		ID:        sessionID,
		UserID:    user.ID,
		CSRFToken: csrfToken,
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(s.cfg.SessionTTL).Format(time.RFC3339),
		IPAddress: ip,
		UserAgent: userAgent,
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &LoginResult{
		SessionID: sessionID,
		CSRFToken: csrfToken,
		User:      user,
	}, nil
}

// Logout destroys a session.
func (s *AuthService) Logout(ctx context.Context, sessionID string) error {
	return s.sessions.Delete(ctx, sessionID)
}

// ValidateSession checks a session ID and returns the associated user and CSRF token.
func (s *AuthService) ValidateSession(ctx context.Context, sessionID string) (*model.User, string, error) {
	session, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return nil, "", errors.New("invalid session")
	}

	expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt)
	if err != nil {
		return nil, "", errors.New("invalid session expiry")
	}
	if time.Now().UTC().After(expiresAt) {
		_ = s.sessions.Delete(ctx, sessionID)
		return nil, "", errors.New("session expired")
	}

	user, err := s.users.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, "", errors.New("user not found")
	}
	if user.Status != model.UserStatusActive {
		return nil, "", errors.New("account is not active")
	}

	return user, session.CSRFToken, nil
}

// ValidatePAT checks a raw personal access token and returns the associated
// user and the token's restriction (SC-51 – SC-56). Unless the token carries
// the admin scope, the user is returned as a copy without administrator
// rights, so no admin check anywhere can be reached through the token.
func (s *AuthService) ValidatePAT(ctx context.Context, rawToken string) (*model.User, *access.Restriction, error) {
	if !strings.HasPrefix(rawToken, patPrefix) {
		return nil, nil, errors.New("invalid token format")
	}

	hash := hashPATToken(rawToken)
	pat, err := s.pats.GetByTokenHash(ctx, hash)
	if err != nil {
		return nil, nil, errors.New("invalid token")
	}

	if pat.RevokedAt != "" {
		return nil, nil, errors.New("token revoked")
	}
	if pat.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, pat.ExpiresAt)
		if err != nil {
			return nil, nil, errors.New("invalid token expiry")
		}
		if time.Now().UTC().After(expiresAt) {
			return nil, nil, errors.New("token expired")
		}
	}
	restriction, err := access.FromPAT(pat)
	if err != nil {
		return nil, nil, errors.New("invalid token scopes")
	}

	user, err := s.users.GetByID(ctx, pat.UserID)
	if err != nil {
		return nil, nil, errors.New("user not found")
	}
	if user.Status != model.UserStatusActive {
		return nil, nil, errors.New("account is not active")
	}
	if user.IsAdmin && !restriction.Allows(access.Admin, access.Read) {
		u := *user
		u.IsAdmin = false
		user = &u
	}

	// Record the last use (best-effort; SC-59).
	now := time.Now().UTC().Format(time.RFC3339)
	_ = s.pats.UpdateLastUsed(ctx, pat.ID, now)

	return user, restriction, nil
}

func generateToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashPATToken(rawToken string) string {
	h := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(h[:])
}
