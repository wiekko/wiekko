package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/crypto"
	"nexmap.net/internal/model"
)

// ts renders a fixed offset from now in the canonical stored format.
func ts(d time.Duration) string { return FormatTimestamp(time.Now().Add(d).Truncate(time.Second)) }

func makeUser(t *testing.T, s UserStore, username string, admin bool) *model.User {
	t.Helper()
	now := ts(0)
	u := &model.User{
		ID: uuid.NewString(), Username: username, Email: username + "@example.com",
		DisplayName: "User " + username, PasswordHash: "hash-" + username, IsAdmin: admin,
		Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Create(context.Background(), u); err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return u
}

func TestUserStoreRoundTrip(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		s := &userStore{db: db, keys: crypto.TestKeyRing()}

		alice := makeUser(t, s, "alice", true)
		makeUser(t, s, "bob", false)

		got, err := s.GetByID(ctx, alice.ID)
		if err != nil {
			t.Fatalf("get by id: %v", err)
		}
		if got.Username != "alice" || !got.IsAdmin || got.Email != alice.Email || got.DisplayName != alice.DisplayName {
			t.Errorf("got %+v, want alice as admin with personal data", got)
		}
		// Timestamps must come back exactly as written, on every engine.
		if got.CreatedAt != alice.CreatedAt || got.UpdatedAt != alice.UpdatedAt {
			t.Errorf("timestamps = %q/%q, want %q/%q", got.CreatedAt, got.UpdatedAt, alice.CreatedAt, alice.UpdatedAt)
		}

		if got, err := s.GetByUsername(ctx, "bob"); err != nil || got.IsAdmin {
			t.Errorf("get by username: %+v, %v", got, err)
		}
		if got, err := s.GetByEmail(ctx, "alice@example.com"); err != nil || got.ID != alice.ID {
			t.Errorf("get by email: %+v, %v", got, err)
		}
		if _, err := s.GetByID(ctx, uuid.NewString()); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("missing user: err = %v, want ErrNoRows", err)
		}

		alice.Username = "alice2"
		alice.IsAdmin = false
		alice.Email = "a2@example.com"
		alice.UpdatedAt = ts(time.Minute)
		if err := s.Update(ctx, alice); err != nil {
			t.Fatalf("update: %v", err)
		}
		got, err = s.GetByEmail(ctx, "a2@example.com")
		if err != nil {
			t.Fatalf("get by new email: %v", err)
		}
		if got.Username != "alice2" || got.IsAdmin || got.UpdatedAt != alice.UpdatedAt {
			t.Errorf("after update got %+v", got)
		}

		if err := s.UpdateStatus(ctx, alice.ID, model.UserStatusPendingApproval); err != nil {
			t.Fatalf("update status: %v", err)
		}
		if err := s.UpdatePassword(ctx, alice.ID, "new-hash"); err != nil {
			t.Fatalf("update password: %v", err)
		}
		got, _ = s.GetByID(ctx, alice.ID)
		if got.Status != model.UserStatusPendingApproval || got.PasswordHash != "new-hash" {
			t.Errorf("status/password not updated: %+v", got)
		}
		if _, err := time.Parse(time.RFC3339, got.UpdatedAt); err != nil {
			t.Errorf("updated_at %q is not RFC 3339: %v", got.UpdatedAt, err)
		}

		pending, err := s.List(ctx, UserFilter{Status: model.UserStatusPendingApproval})
		if err != nil || len(pending) != 1 || pending[0].ID != alice.ID {
			t.Errorf("list pending: %v, %v", pending, err)
		}
		if n, err := s.Count(ctx); err != nil || n != 2 {
			t.Errorf("count = %d, %v; want 2", n, err)
		}

		if err := s.Delete(ctx, alice.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if n, _ := s.Count(ctx); n != 1 {
			t.Errorf("count after delete = %d, want 1", n)
		}
	})
}

func TestUserStoreTOTP(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		s := &userStore{db: db, keys: crypto.TestKeyRing()}
		u := makeUser(t, s, "carol", false)

		if err := s.SetTOTP(ctx, u.ID, "JBSWY3DPEHPK3PXP", true); err != nil {
			t.Fatalf("set totp: %v", err)
		}
		got, _ := s.GetByID(ctx, u.ID)
		if !got.TOTPEnabled || got.TOTPSecret != "JBSWY3DPEHPK3PXP" {
			t.Errorf("totp not stored: enabled=%v secret=%q", got.TOTPEnabled, got.TOTPSecret)
		}

		for _, tc := range []struct {
			step int64
			want bool
		}{{100, true}, {100, false}, {99, false}, {101, true}} {
			ok, err := s.ConsumeTOTPStep(ctx, u.ID, tc.step)
			if err != nil || ok != tc.want {
				t.Errorf("consume step %d = %v, %v; want %v", tc.step, ok, err, tc.want)
			}
		}

		if err := s.SetTOTP(ctx, u.ID, "", true); err != nil {
			t.Fatalf("clear totp: %v", err)
		}
		got, _ = s.GetByID(ctx, u.ID)
		if got.TOTPEnabled || got.TOTPSecret != "" {
			t.Errorf("totp not cleared: enabled=%v secret=%q", got.TOTPEnabled, got.TOTPSecret)
		}
		// Clearing resets the replay guard.
		if ok, _ := s.ConsumeTOTPStep(ctx, u.ID, 1); !ok {
			t.Error("replay guard not reset by SetTOTP")
		}
	})
}

func TestSessionStore(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		users := &userStore{db: db, keys: crypto.TestKeyRing()}
		s := &sessionStore{db: db}
		u := makeUser(t, users, "dave", false)

		live := &model.Session{ID: "3f9a0c1e5b7d42aa9c06e1d2b4f8a7c3e5d1f0b9a8c7d6e5f4a3b2c1d0e9f8a7", UserID: u.ID, CSRFToken: "csrf", CreatedAt: ts(0),
			ExpiresAt: ts(time.Hour), IPAddress: "127.0.0.1", UserAgent: "test"}
		expired := &model.Session{ID: "expired-session-token", UserID: u.ID, CSRFToken: "csrf2", CreatedAt: ts(-2 * time.Hour),
			ExpiresAt: ts(-time.Hour)}
		for _, sess := range []*model.Session{live, expired} {
			if err := s.Create(ctx, sess); err != nil {
				t.Fatalf("create session: %v", err)
			}
		}

		got, err := s.GetByID(ctx, live.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if *got != *live {
			t.Errorf("got %+v, want %+v", got, live)
		}

		if err := s.DeleteExpired(ctx); err != nil {
			t.Fatalf("delete expired: %v", err)
		}
		if _, err := s.GetByID(ctx, expired.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expired session survived: %v", err)
		}
		if _, err := s.GetByID(ctx, live.ID); err != nil {
			t.Errorf("live session deleted: %v", err)
		}

		// Deleting all but one session keeps that one and the sessions of
		// other users.
		other := &model.Session{ID: "other-session-token", UserID: u.ID, CSRFToken: "csrf3", CreatedAt: ts(0), ExpiresAt: ts(time.Hour)}
		stranger := makeUser(t, users, "erin", false)
		foreign := &model.Session{ID: "foreign-session-token", UserID: stranger.ID, CSRFToken: "csrf4", CreatedAt: ts(0), ExpiresAt: ts(time.Hour)}
		for _, sess := range []*model.Session{other, foreign} {
			if err := s.Create(ctx, sess); err != nil {
				t.Fatalf("create session: %v", err)
			}
		}
		if err := s.DeleteByUserIDExcept(ctx, u.ID, live.ID); err != nil {
			t.Fatalf("delete by user except: %v", err)
		}
		if _, err := s.GetByID(ctx, other.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("other session survived delete by user except: %v", err)
		}
		if _, err := s.GetByID(ctx, live.ID); err != nil {
			t.Errorf("kept session deleted: %v", err)
		}
		if _, err := s.GetByID(ctx, foreign.ID); err != nil {
			t.Errorf("another user's session deleted: %v", err)
		}

		if err := s.DeleteByUserID(ctx, u.ID); err != nil {
			t.Fatalf("delete by user: %v", err)
		}
		if _, err := s.GetByID(ctx, live.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("session survived delete by user: %v", err)
		}
	})
}

func TestPATStore(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		users := &userStore{db: db, keys: crypto.TestKeyRing()}
		s := &patStore{db: db}
		u := makeUser(t, users, "erin", false)

		older := &model.PAT{ID: uuid.NewString(), UserID: u.ID, Name: "ci", TokenHash: "h1", TokenPrefix: "nx_1",
			CreatedAt: ts(-time.Hour)}
		newer := &model.PAT{ID: uuid.NewString(), UserID: u.ID, Name: "cli", TokenHash: "h2", TokenPrefix: "nx_2",
			CreatedAt: ts(0), ExpiresAt: ts(24 * time.Hour)}
		for _, p := range []*model.PAT{older, newer} {
			if err := s.Create(ctx, p); err != nil {
				t.Fatalf("create pat: %v", err)
			}
		}

		got, err := s.GetByTokenHash(ctx, "h2")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if *got != *newer {
			t.Errorf("got %+v, want %+v", got, newer)
		}

		list, err := s.ListByUserID(ctx, u.ID)
		if err != nil || len(list) != 2 || list[0].ID != newer.ID {
			t.Fatalf("list = %v, %v; want newest first", list, err)
		}

		used, revoked := ts(time.Minute), ts(2*time.Minute)
		if err := s.UpdateLastUsed(ctx, older.ID, used); err != nil {
			t.Fatalf("update last used: %v", err)
		}
		if err := s.Revoke(ctx, older.ID, revoked); err != nil {
			t.Fatalf("revoke: %v", err)
		}
		got, _ = s.GetByTokenHash(ctx, "h1")
		if got.LastUsedAt != used || got.RevokedAt != revoked || got.ExpiresAt != "" {
			t.Errorf("after update got %+v", got)
		}
	})
}

func TestSettingStore(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		s := &settingStore{db: db}

		// Seeded by the migrations.
		if v, err := s.Get(ctx, "password_min_length"); err != nil || v != "10" {
			t.Errorf("seeded setting = %q, %v", v, err)
		}
		if err := s.Set(ctx, "password_min_length", "12"); err != nil {
			t.Fatalf("set existing: %v", err)
		}
		if err := s.Set(ctx, "custom", "x"); err != nil {
			t.Fatalf("set new: %v", err)
		}
		all, err := s.GetAll(ctx)
		if err != nil {
			t.Fatalf("get all: %v", err)
		}
		if all["password_min_length"] != "12" || all["custom"] != "x" {
			t.Errorf("get all = %v", all)
		}
	})
}

func TestTwoFactorStore(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		users := &userStore{db: db, keys: crypto.TestKeyRing()}
		s := &twoFactorStore{db: db}
		u := makeUser(t, users, "frank", false)

		if err := s.ReplaceRecoveryCodes(ctx, u.ID, []string{"a", "b", "c"}); err != nil {
			t.Fatalf("replace codes: %v", err)
		}
		if ok, err := s.ConsumeRecoveryCode(ctx, u.ID, "b"); err != nil || !ok {
			t.Errorf("consume b = %v, %v", ok, err)
		}
		if ok, _ := s.ConsumeRecoveryCode(ctx, u.ID, "b"); ok {
			t.Error("code b consumed twice")
		}
		if n, err := s.CountUnusedRecoveryCodes(ctx, u.ID); err != nil || n != 2 {
			t.Errorf("unused = %d, %v; want 2", n, err)
		}
		if err := s.ReplaceRecoveryCodes(ctx, u.ID, []string{"d"}); err != nil {
			t.Fatalf("replace again: %v", err)
		}
		if n, _ := s.CountUnusedRecoveryCodes(ctx, u.ID); n != 1 {
			t.Errorf("unused after replace = %d, want 1", n)
		}
		if err := s.DeleteRecoveryCodes(ctx, u.ID); err != nil {
			t.Fatalf("delete codes: %v", err)
		}
		if n, _ := s.CountUnusedRecoveryCodes(ctx, u.ID); n != 0 {
			t.Errorf("unused after delete = %d, want 0", n)
		}

		live := &model.LoginChallenge{ID: "challenge-live", UserID: u.ID, CreatedAt: ts(0), ExpiresAt: ts(5 * time.Minute),
			IPAddress: "::1", UserAgent: "test"}
		stale := &model.LoginChallenge{ID: "challenge-stale", UserID: u.ID, CreatedAt: ts(-time.Hour), ExpiresAt: ts(-time.Minute)}
		for _, c := range []*model.LoginChallenge{live, stale} {
			if err := s.CreateChallenge(ctx, c); err != nil {
				t.Fatalf("create challenge: %v", err)
			}
		}
		got, err := s.GetChallenge(ctx, live.ID)
		if err != nil {
			t.Fatalf("get challenge: %v", err)
		}
		if *got != *live {
			t.Errorf("got %+v, want %+v", got, live)
		}
		for want := 1; want <= 2; want++ {
			if n, err := s.IncrementChallengeAttempts(ctx, live.ID); err != nil || n != want {
				t.Errorf("attempts = %d, %v; want %d", n, err, want)
			}
		}
		if err := s.DeleteExpiredChallenges(ctx, Now()); err != nil {
			t.Fatalf("delete expired: %v", err)
		}
		if _, err := s.GetChallenge(ctx, stale.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("stale challenge survived: %v", err)
		}
		if err := s.DeleteChallenge(ctx, live.ID); err != nil {
			t.Fatalf("delete challenge: %v", err)
		}
		if _, err := s.GetChallenge(ctx, live.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("live challenge survived delete: %v", err)
		}
	})
}
