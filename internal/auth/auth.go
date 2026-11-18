package auth

import "context"

// Identity represents an authenticated user regardless of provider.
type Identity struct {
	UserID      string
	Username    string
	Email       string
	DisplayName string
	IsAdmin     bool
}

// PasswordPolicy defines configurable password requirements.
type PasswordPolicy struct {
	MinLength        int
	RequireUppercase bool
	RequireLowercase bool
	RequireDigit     bool
	RequireSpecial   bool
}

// Provider is the pluggable authentication interface (SC-35).
// The built-in provider uses username/password stored in the database.
// Future providers (LDAP, OIDC) implement the same interface.
type Provider interface {
	// Authenticate verifies credentials and returns an identity.
	Authenticate(ctx context.Context, username, password string) (*Identity, error)
	// ValidatePassword checks whether a password meets the given policy.
	ValidatePassword(password string, policy PasswordPolicy) error
	// HashPassword returns a hashed password string.
	HashPassword(password string) (string, error)
	// VerifyPassword checks a password against a stored hash.
	VerifyPassword(password, hash string) error
	// SupportsPasswordChange indicates whether this provider manages passwords.
	SupportsPasswordChange() bool
}
