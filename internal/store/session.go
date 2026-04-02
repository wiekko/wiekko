package store

import (
	"context"
	"database/sql"
	"fmt"

	"nexmap.net/internal/model"
)

// SessionStore defines data access operations for sessions.
type SessionStore interface {
	Create(ctx context.Context, session *model.Session) error
	GetByID(ctx context.Context, id string) (*model.Session, error)
	Delete(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID string) error
	DeleteExpired(ctx context.Context) error
}

type sessionStore struct {
	db *sql.DB
}

func (s *sessionStore) Create(ctx context.Context, session *model.Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, csrf_token, created_at, expires_at, ip_address, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.CSRFToken, session.CreatedAt, session.ExpiresAt,
		session.IPAddress, session.UserAgent,
	)
	return err
}

func (s *sessionStore) GetByID(ctx context.Context, id string) (*model.Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, csrf_token, created_at, expires_at, ip_address, user_agent FROM sessions WHERE id = ?`, id)

	var sess model.Session
	err := row.Scan(&sess.ID, &sess.UserID, &sess.CSRFToken, &sess.CreatedAt, &sess.ExpiresAt,
		&sess.IPAddress, &sess.UserAgent)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *sessionStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *sessionStore) DeleteByUserID(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (s *sessionStore) DeleteExpired(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < datetime('now')`)
	return err
}
