// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"testing"

	"nexmap.net/internal/crypto"
)

// TestUserLookupIgnoresCase checks that usernames and email addresses are
// found whatever their case, so "Admin" cannot be registered next to "admin".
func TestUserLookupIgnoresCase(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		users := &userStore{db: db, keys: crypto.TestKeyRing()}
		u := makeUser(t, users, "Grace", false) // grace@example.com... as "Grace@example.com"

		for _, name := range []string{"Grace", "grace", "GRACE", " grace "} {
			got, err := users.GetByUsername(ctx, name)
			if err != nil || got.ID != u.ID {
				t.Errorf("GetByUsername(%q) = %v, %v", name, got, err)
			}
		}
		for _, email := range []string{"Grace@example.com", "grace@example.com", " GRACE@EXAMPLE.COM "} {
			got, err := users.GetByEmail(ctx, email)
			if err != nil || got.ID != u.ID {
				t.Errorf("GetByEmail(%q) = %v, %v", email, got, err)
			}
		}
		if _, err := users.GetByUsername(ctx, "grac"); err == nil {
			t.Error("a prefix of a username matched")
		}
	})
}

// TestPersonalDataIsBoundToItsRecord moves one user's encrypted values to
// another user's rows, as someone with write access to the database could, and
// checks that they are not accepted there.
func TestPersonalDataIsBoundToItsRecord(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		users := &userStore{db: db, keys: crypto.TestKeyRing()}
		attacker := makeUser(t, users, "mallory", false)
		victim := makeUser(t, users, "victor", false)
		if err := users.SetTOTP(ctx, attacker.ID, "JBSWY3DPEHPK3PXP", true); err != nil {
			t.Fatal(err)
		}

		var secret string
		if err := db.QueryRowContext(ctx, `SELECT totp_secret FROM users WHERE id = ?`, attacker.ID).Scan(&secret); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE users SET totp_secret = ?, totp_enabled = ? WHERE id = ?`, secret, true, victim.ID); err != nil {
			t.Fatal(err)
		}
		if got, err := users.GetByID(ctx, victim.ID); err == nil {
			t.Errorf("the attacker's TOTP secret was accepted for the victim: %q", got.TOTPSecret)
		}
		if got, err := users.GetByID(ctx, attacker.ID); err != nil || got.TOTPSecret != "JBSWY3DPEHPK3PXP" {
			t.Errorf("own secret unreadable: %v", err)
		}
	})
}
