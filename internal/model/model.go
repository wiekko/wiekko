package model

// User represents a registered user account.
type User struct {
	ID           string
	Username     string
	Email        string
	DisplayName  string
	PasswordHash string
	IsAdmin      bool
	Status       string // "pending_approval", "active", "locked", "deletion_requested"
	TOTPSecret   string // plaintext base32; encrypted at rest by the store
	TOTPEnabled  bool   // false while a secret is only pending confirmation
	CreatedAt    string // RFC 3339
	UpdatedAt    string // RFC 3339
}

// User status constants.
const (
	UserStatusPendingApproval   = "pending_approval"
	UserStatusActive            = "active"
	UserStatusLocked            = "locked"
	UserStatusDeletionRequested = "deletion_requested"
)

// Session represents a server-side session.
type Session struct {
	ID        string
	UserID    string
	CSRFToken string
	CreatedAt string
	ExpiresAt string
	IPAddress string
	UserAgent string
}

// LoginChallenge is a pending second login step: the password was accepted
// and the user still has to present a TOTP or recovery code.
type LoginChallenge struct {
	ID        string // SHA-256 of the raw challenge token
	UserID    string
	Attempts  int
	CreatedAt string
	ExpiresAt string
	IPAddress string
	UserAgent string
}

// PAT represents a personal access token.
type PAT struct {
	ID          string
	UserID      string
	Name        string
	TokenHash   string
	TokenPrefix string
	LastUsedAt  string // empty if never used
	ExpiresAt   string // empty if no expiry
	CreatedAt   string
	RevokedAt   string // empty if not revoked
}

// ProductRole represents a user's role within a product.
type ProductRole struct {
	ID        string
	UserID    string
	ProductID string
	Role      string // "owner", "developer", "tester", "viewer"
	CreatedAt string
}

// Product role constants.
const (
	RoleOwner     = "owner"
	RoleDeveloper = "developer"
	RoleTester    = "tester"
	RoleViewer    = "viewer"
)

// Invitation represents an invitation to register.
type Invitation struct {
	ID         string
	Email      string
	TokenHash  string
	InvitedBy  string
	ProductID  string // empty if not product-scoped
	Role       string // empty if not product-scoped
	AcceptedAt string // empty if not yet accepted
	ExpiresAt  string
	CreatedAt  string
}
