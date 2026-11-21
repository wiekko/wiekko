// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package builtin_test

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/auth"
	"nexmap.net/internal/auth/builtin"
	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func newProvider(users *testutil.MockUserStore) *builtin.Provider {
	return builtin.New(users, 1) // 1 iteration for test speed
}

func TestValidatePassword(t *testing.T) {
	p := newProvider(&testutil.MockUserStore{})

	tests := []struct {
		name     string
		password string
		policy   auth.PasswordPolicy
		wantErr  bool
	}{
		{"too short", "ab", auth.PasswordPolicy{MinLength: 5}, true},
		{"meets min length", "abcde", auth.PasswordPolicy{MinLength: 5}, false},
		{"missing uppercase", "abcdef", auth.PasswordPolicy{MinLength: 1, RequireUppercase: true}, true},
		{"has uppercase", "Abcdef", auth.PasswordPolicy{MinLength: 1, RequireUppercase: true}, false},
		{"missing lowercase", "ABCDEF", auth.PasswordPolicy{MinLength: 1, RequireLowercase: true}, true},
		{"has lowercase", "ABCDEf", auth.PasswordPolicy{MinLength: 1, RequireLowercase: true}, false},
		{"missing digit", "abcdef", auth.PasswordPolicy{MinLength: 1, RequireDigit: true}, true},
		{"has digit", "abc1ef", auth.PasswordPolicy{MinLength: 1, RequireDigit: true}, false},
		{"missing special", "abcdef", auth.PasswordPolicy{MinLength: 1, RequireSpecial: true}, true},
		{"has special", "abc!ef", auth.PasswordPolicy{MinLength: 1, RequireSpecial: true}, false},
		{"all required valid", "Abc1!ef", auth.PasswordPolicy{
			MinLength: 5, RequireUppercase: true, RequireLowercase: true,
			RequireDigit: true, RequireSpecial: true,
		}, false},
		{"no requirements", "x", auth.PasswordPolicy{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := p.ValidatePassword(tc.password, tc.policy)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidatePassword(%q) err = %v, wantErr = %v", tc.password, err, tc.wantErr)
			}
		})
	}
}

func TestHashAndVerifyRoundTrip(t *testing.T) {
	p := newProvider(&testutil.MockUserStore{})

	t.Run("correct password verifies", func(t *testing.T) {
		hash, err := p.HashPassword("mysecret")
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}
		if err := p.VerifyPassword("mysecret", hash); err != nil {
			t.Fatalf("VerifyPassword: %v", err)
		}
	})

	t.Run("wrong password fails", func(t *testing.T) {
		hash, err := p.HashPassword("mysecret")
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}
		if err := p.VerifyPassword("wrong", hash); err == nil {
			t.Fatal("VerifyPassword should fail for wrong password")
		}
	})
}

func TestVerifyPasswordMalformed(t *testing.T) {
	p := newProvider(&testutil.MockUserStore{})

	tests := []struct {
		name    string
		encoded string
		wantMsg string
	}{
		{"empty", "", "unsupported hash format"},
		{"wrong prefix", "$argon2$foo$bar$baz", "unsupported hash format"},
		{"too few parts", "$pbkdf2-sha256$only", "malformed hash"},
		{"non-numeric iterations", "$pbkdf2-sha256$abc$c2FsdA$aGFzaA", "malformed hash: invalid iterations"},
		{"invalid salt base64", "$pbkdf2-sha256$1000$!!!$aGFzaA", "malformed hash: invalid salt"},
		{"invalid hash base64", "$pbkdf2-sha256$1000$c2FsdA$!!!", "malformed hash: invalid hash"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := p.VerifyPassword("password", tc.encoded)
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Error() != tc.wantMsg {
				t.Fatalf("error = %q, want %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

func TestAuthenticate(t *testing.T) {
	activeUser := testutil.MakeUser(func(u *model.User) {
		u.ID = "u1"
		u.Username = "alice"
	})

	t.Run("valid credentials", func(t *testing.T) {
		// Hash the password with low iterations for speed.
		p := builtin.New(nil, 1)
		hash, _ := p.HashPassword("secret")
		activeUser.PasswordHash = hash

		users := &testutil.MockUserStore{
			GetByUsernameFunc: func(_ context.Context, username string) (*model.User, error) {
				if username == "alice" {
					return activeUser, nil
				}
				return nil, errors.New("not found")
			},
		}
		provider := newProvider(users)

		identity, err := provider.Authenticate(context.Background(), "alice", "secret")
		if err != nil {
			t.Fatalf("Authenticate: %v", err)
		}
		if identity.UserID != "u1" {
			t.Fatalf("UserID = %q, want %q", identity.UserID, "u1")
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		p := builtin.New(nil, 1)
		hash, _ := p.HashPassword("secret")
		user := testutil.MakeUser(func(u *model.User) { u.PasswordHash = hash })

		users := &testutil.MockUserStore{
			GetByUsernameFunc: func(_ context.Context, _ string) (*model.User, error) {
				return user, nil
			},
		}
		provider := newProvider(users)

		_, err := provider.Authenticate(context.Background(), "alice", "wrongpass")
		if err == nil || err.Error() != "invalid credentials" {
			t.Fatalf("err = %v, want 'invalid credentials'", err)
		}
	})

	t.Run("locked user", func(t *testing.T) {
		user := testutil.MakeUser(func(u *model.User) { u.Status = model.UserStatusLocked })
		users := &testutil.MockUserStore{
			GetByUsernameFunc: func(_ context.Context, _ string) (*model.User, error) {
				return user, nil
			},
		}
		provider := newProvider(users)

		_, err := provider.Authenticate(context.Background(), "alice", "secret")
		if err == nil || err.Error() != "account is not active" {
			t.Fatalf("err = %v, want 'account is not active'", err)
		}
	})

	t.Run("nonexistent user", func(t *testing.T) {
		users := &testutil.MockUserStore{
			GetByUsernameFunc: func(_ context.Context, _ string) (*model.User, error) {
				return nil, errors.New("not found")
			},
		}
		provider := newProvider(users)

		_, err := provider.Authenticate(context.Background(), "nobody", "secret")
		if err == nil || err.Error() != "invalid credentials" {
			t.Fatalf("err = %v, want 'invalid credentials'", err)
		}
	})
}

func TestSupportsPasswordChange(t *testing.T) {
	p := newProvider(&testutil.MockUserStore{})
	if !p.SupportsPasswordChange() {
		t.Fatal("SupportsPasswordChange() = false, want true")
	}
}
