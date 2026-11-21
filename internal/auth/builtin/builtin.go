// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package builtin

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"nexmap.net/internal/auth"
	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

const (
	defaultIterations = 600_000
	saltLength        = 16
	keyLength         = 32
	hashPrefix        = "$pbkdf2-sha256$"
)

// Provider implements auth.Provider using the local database.
type Provider struct {
	users      store.UserStore
	iterations int

	// dummyHash is a hash of a random password at the provider's cost,
	// computed on first use. See Authenticate.
	dummyOnce sync.Once
	dummyHash string
}

// New creates a built-in auth provider.
// If iterations is 0, the OWASP-recommended default (600,000) is used.
func New(users store.UserStore, iterations int) *Provider {
	if iterations <= 0 {
		iterations = defaultIterations
	}
	return &Provider{users: users, iterations: iterations}
}

// Authenticate verifies a username and password.
//
// Every attempt costs one password hash, whether or not the account exists
// and is active. Otherwise the answer for an unknown name would come back
// much faster than for a known one, and the response time alone would tell
// which usernames exist.
func (p *Provider) Authenticate(ctx context.Context, username, password string) (*auth.Identity, error) {
	user, err := p.users.GetByUsername(ctx, username)
	if err != nil {
		p.burnHash(password)
		return nil, errors.New("invalid credentials")
	}
	if user.Status != model.UserStatusActive {
		p.burnHash(password)
		return nil, errors.New("account is not active")
	}
	if err := p.VerifyPassword(password, user.PasswordHash); err != nil {
		return nil, errors.New("invalid credentials")
	}
	return &auth.Identity{
		UserID:      user.ID,
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
	}, nil
}

// burnHash spends the time a real password check would take.
func (p *Provider) burnHash(password string) {
	p.dummyOnce.Do(func() {
		random := make([]byte, 32)
		_, _ = rand.Read(random)
		p.dummyHash, _ = p.HashPassword(base64.RawStdEncoding.EncodeToString(random))
	})
	_ = p.VerifyPassword(password, p.dummyHash)
}

func (p *Provider) ValidatePassword(password string, policy auth.PasswordPolicy) error {
	if len(password) < policy.MinLength {
		return fmt.Errorf("password must be at least %d characters", policy.MinLength)
	}
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}
	if policy.RequireUppercase && !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}
	if policy.RequireLowercase && !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}
	if policy.RequireDigit && !hasDigit {
		return errors.New("password must contain at least one digit")
	}
	if policy.RequireSpecial && !hasSpecial {
		return errors.New("password must contain at least one special character")
	}
	return nil
}

func (p *Provider) HashPassword(password string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	dk, err := pbkdf2.Key(sha256.New, password, salt, p.iterations, keyLength)
	if err != nil {
		return "", fmt.Errorf("pbkdf2: %w", err)
	}

	// Format: $pbkdf2-sha256$<iterations>$<base64-salt>$<base64-hash>
	encoded := fmt.Sprintf("%s%d$%s$%s",
		hashPrefix,
		p.iterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(dk),
	)
	return encoded, nil
}

func (p *Provider) VerifyPassword(password, encoded string) error {
	if !strings.HasPrefix(encoded, hashPrefix) {
		return errors.New("unsupported hash format")
	}

	parts := strings.Split(strings.TrimPrefix(encoded, hashPrefix), "$")
	if len(parts) != 3 {
		return errors.New("malformed hash")
	}

	iterations, err := strconv.Atoi(parts[0])
	if err != nil {
		return errors.New("malformed hash: invalid iterations")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return errors.New("malformed hash: invalid salt")
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return errors.New("malformed hash: invalid hash")
	}

	dk, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(expected))
	if err != nil {
		return fmt.Errorf("pbkdf2: %w", err)
	}

	if subtle.ConstantTimeCompare(dk, expected) != 1 {
		return errors.New("password mismatch")
	}
	return nil
}

func (p *Provider) SupportsPasswordChange() bool {
	return true
}
