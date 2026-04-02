package testutil

import (
	"time"

	"nexmap.net/internal/model"
)

// MakeUser returns an active user with sensible defaults. Overrides can customize fields.
func MakeUser(overrides ...func(*model.User)) *model.User {
	now := time.Now().UTC().Format(time.RFC3339)
	u := &model.User{
		ID:           "user-001",
		Username:     "testuser",
		Email:        "test@example.com",
		DisplayName:  "Test User",
		PasswordHash: "hashed",
		IsAdmin:      false,
		Status:       model.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	for _, fn := range overrides {
		fn(u)
	}
	return u
}

// MakeSession returns a valid session expiring 24h in the future.
func MakeSession(overrides ...func(*model.Session)) *model.Session {
	now := time.Now().UTC()
	s := &model.Session{
		ID:        "session-001",
		UserID:    "user-001",
		CSRFToken: "csrf-token-001",
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339),
		IPAddress: "127.0.0.1",
		UserAgent: "TestAgent/1.0",
	}
	for _, fn := range overrides {
		fn(s)
	}
	return s
}

// MakePAT returns a valid, non-revoked, non-expired PAT.
func MakePAT(overrides ...func(*model.PAT)) *model.PAT {
	now := time.Now().UTC().Format(time.RFC3339)
	p := &model.PAT{
		ID:          "pat-001",
		UserID:      "user-001",
		Name:        "test-token",
		TokenHash:   "somehash",
		TokenPrefix: "nxm_abcd",
		CreatedAt:   now,
	}
	for _, fn := range overrides {
		fn(p)
	}
	return p
}

// MakeInvitation returns a valid, non-accepted invitation.
func MakeInvitation(overrides ...func(*model.Invitation)) *model.Invitation {
	now := time.Now().UTC()
	inv := &model.Invitation{
		ID:        "inv-001",
		Email:     "invite@example.com",
		TokenHash: "invhash",
		InvitedBy: "user-001",
		ExpiresAt: now.Add(7 * 24 * time.Hour).Format(time.RFC3339),
		CreatedAt: now.Format(time.RFC3339),
	}
	for _, fn := range overrides {
		fn(inv)
	}
	return inv
}

// FutureRFC3339 returns a time d in the future formatted as RFC3339.
func FutureRFC3339(d time.Duration) string {
	return time.Now().UTC().Add(d).Format(time.RFC3339)
}

// PastRFC3339 returns a time d in the past formatted as RFC3339.
func PastRFC3339(d time.Duration) string {
	return time.Now().UTC().Add(-d).Format(time.RFC3339)
}
