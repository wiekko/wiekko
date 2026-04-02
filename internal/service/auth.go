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
	users    store.UserStore
	sessions store.SessionStore
	pats     store.PATStore
	provider auth.Provider
	cfg      *config.Config
}

// LoginResult contains the data returned after a successful login.
type LoginResult struct {
	SessionID string
	CSRFToken string
	User      *model.User
}

// Login authenticates a user and creates a session.
func (s *AuthService) Login(ctx context.Context, username, password, ip, userAgent string) (*LoginResult, error) {
	identity, err := s.provider.Authenticate(ctx, username, password)
	if err != nil {
		return nil, err
	}

	user, err := s.users.GetByID(ctx, identity.UserID)
	if err != nil {
		return nil, fmt.Errorf("fetch user: %w", err)
	}

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

// ValidatePAT checks a raw personal access token and returns the associated user.
func (s *AuthService) ValidatePAT(ctx context.Context, rawToken string) (*model.User, error) {
	if !strings.HasPrefix(rawToken, patPrefix) {
		return nil, errors.New("invalid token format")
	}

	hash := hashPATToken(rawToken)
	pat, err := s.pats.GetByTokenHash(ctx, hash)
	if err != nil {
		return nil, errors.New("invalid token")
	}

	if pat.RevokedAt != "" {
		return nil, errors.New("token revoked")
	}
	if pat.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, pat.ExpiresAt)
		if err != nil {
			return nil, errors.New("invalid token expiry")
		}
		if time.Now().UTC().After(expiresAt) {
			return nil, errors.New("token expired")
		}
	}

	user, err := s.users.GetByID(ctx, pat.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}
	if user.Status != model.UserStatusActive {
		return nil, errors.New("account is not active")
	}

	// Update last-used timestamp asynchronously (best-effort).
	now := time.Now().UTC().Format(time.RFC3339)
	_ = s.pats.UpdateLastUsed(ctx, pat.ID, now)

	return user, nil
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
