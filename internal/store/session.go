// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// SessionStore defines data access operations for sessions.
//
// Callers always pass the session token as the browser holds it. The table
// only ever sees its SHA-256 (like access tokens, invitations and login
// challenges), so a copy of the database — a backup, a dump — does not contain
// anything that could be used as a session cookie.
type SessionStore interface {
	Create(ctx context.Context, session *model.Session) error
	GetByID(ctx context.Context, id string) (*model.Session, error)
	Delete(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID string) error
	// DeleteByUserIDExcept deletes the user's sessions except keepID.
	DeleteByUserIDExcept(ctx context.Context, userID, keepID string) error
	DeleteExpired(ctx context.Context) error
}

type sessionStore struct {
	db *DB
}

// sessionKey is what the sessions table stores in place of a session token.
// Tokens are 256 random bits, so a plain hash is enough: there is nothing to
// guess.
func sessionKey(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *sessionStore) Create(ctx context.Context, session *model.Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, csrf_token, created_at, expires_at, ip_address, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sessionKey(session.ID), session.UserID, session.CSRFToken, session.CreatedAt, session.ExpiresAt,
		session.IPAddress, session.UserAgent,
	)
	return err
}

func (s *sessionStore) GetByID(ctx context.Context, id string) (*model.Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT user_id, csrf_token, created_at, expires_at, ip_address, user_agent FROM sessions WHERE id = ?`,
		sessionKey(id))

	sess := model.Session{ID: id}
	err := row.Scan(&sess.UserID, &sess.CSRFToken, &sess.CreatedAt, &sess.ExpiresAt,
		&sess.IPAddress, &sess.UserAgent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *sessionStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, sessionKey(id))
	return err
}

func (s *sessionStore) DeleteByUserID(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (s *sessionStore) DeleteByUserIDExcept(ctx context.Context, userID, keepID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id <> ?`,
		userID, sessionKey(keepID))
	return err
}

func (s *sessionStore) DeleteExpired(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, Now())
	return err
}
