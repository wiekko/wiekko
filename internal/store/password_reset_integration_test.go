// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"wiekko.net/internal/crypto"
	"wiekko.net/internal/model"
)

func TestPasswordResetStore(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		users := &userStore{db: db, keys: crypto.TestKeyRing()}
		s := &passwordResetStore{db: db}
		alice := makeUser(t, users, "alice", false)

		tok := &model.PasswordResetToken{
			ID: uuid.NewString(), UserID: alice.ID, TokenHash: "hash-1",
			CreatedAt: ts(0), ExpiresAt: ts(time.Hour),
		}
		if err := s.Create(ctx, tok); err != nil {
			t.Fatalf("create: %v", err)
		}
		got, err := s.GetByTokenHash(ctx, "hash-1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.ID != tok.ID || got.UserID != alice.ID || got.ExpiresAt != tok.ExpiresAt || got.UsedAt != "" {
			t.Errorf("got %+v, want %+v", got, tok)
		}
		if _, err := s.GetByTokenHash(ctx, "nope"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("unknown hash: err = %v", err)
		}

		// Single use: the first consume wins, the second finds nothing.
		if ok, err := s.Consume(ctx, tok.ID, Now()); err != nil || !ok {
			t.Fatalf("first consume = %v, %v", ok, err)
		}
		if ok, err := s.Consume(ctx, tok.ID, Now()); err != nil || ok {
			t.Errorf("second consume = %v, %v; want false", ok, err)
		}
		if got, _ := s.GetByTokenHash(ctx, "hash-1"); got.UsedAt == "" {
			t.Error("used_at not set")
		}

		// An expired token cannot be consumed and is cleaned up.
		old := &model.PasswordResetToken{
			ID: uuid.NewString(), UserID: alice.ID, TokenHash: "hash-old",
			CreatedAt: ts(-2 * time.Hour), ExpiresAt: ts(-time.Hour),
		}
		if err := s.Create(ctx, old); err != nil {
			t.Fatal(err)
		}
		if ok, _ := s.Consume(ctx, old.ID, Now()); ok {
			t.Error("expired token was consumed")
		}
		if err := s.DeleteExpired(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetByTokenHash(ctx, "hash-old"); err == nil {
			t.Error("expired token survived DeleteExpired")
		}
		if _, err := s.GetByTokenHash(ctx, "hash-1"); err != nil {
			t.Errorf("unexpired token removed: %v", err)
		}

		// Deleting the user's tokens, and the user, removes them.
		if err := s.DeleteByUserID(ctx, alice.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetByTokenHash(ctx, "hash-1"); err == nil {
			t.Error("token survived DeleteByUserID")
		}
		again := &model.PasswordResetToken{ID: uuid.NewString(), UserID: alice.ID, TokenHash: "hash-2", CreatedAt: ts(0), ExpiresAt: ts(time.Hour)}
		if err := s.Create(ctx, again); err != nil {
			t.Fatal(err)
		}
		if err := users.Delete(ctx, alice.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetByTokenHash(ctx, "hash-2"); err == nil {
			t.Error("token survived deleting its user")
		}
	})
}
