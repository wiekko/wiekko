package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// PasswordResetStore defines data access operations for password reset
// tokens (SC-37). Callers pass and receive token hashes, never raw tokens.
type PasswordResetStore interface {
	Create(ctx context.Context, t *model.PasswordResetToken) error
	GetByTokenHash(ctx context.Context, hash string) (*model.PasswordResetToken, error)
	// Consume marks the token used if it is still unused and not expired at
	// now, and reports whether it did. Of two requests racing with the same
	// token, only one gets true.
	Consume(ctx context.Context, id, now string) (bool, error)
	DeleteByUserID(ctx context.Context, userID string) error
	DeleteExpired(ctx context.Context) error
}

type passwordResetStore struct {
	db *DB
}

func (s *passwordResetStore) Create(ctx context.Context, t *model.PasswordResetToken) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO password_reset_tokens (id, user_id, token_hash, created_at, expires_at, used_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.ID, t.UserID, t.TokenHash, t.CreatedAt, t.ExpiresAt, nullableText(t.UsedAt))
	if err != nil {
		return fmt.Errorf("password reset create: %w", err)
	}
	return nil
}

func (s *passwordResetStore) GetByTokenHash(ctx context.Context, hash string) (*model.PasswordResetToken, error) {
	var t model.PasswordResetToken
	var usedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, created_at, expires_at, used_at
		 FROM password_reset_tokens WHERE token_hash = ?`, hash).
		Scan(&t.ID, &t.UserID, &t.TokenHash, &t.CreatedAt, &t.ExpiresAt, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("password reset token not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	t.UsedAt = usedAt.String
	return &t, nil
}

func (s *passwordResetStore) Consume(ctx context.Context, id, now string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE password_reset_tokens SET used_at = ?
		 WHERE id = ? AND used_at IS NULL AND expires_at > ?`, now, id, now)
	if err != nil {
		return false, fmt.Errorf("password reset consume: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("password reset consume: %w", err)
	}
	return n == 1, nil
}

func (s *passwordResetStore) DeleteByUserID(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM password_reset_tokens WHERE user_id = ?`, userID)
	return err
}

// DeleteExpired removes tokens that can no longer be used: expired ones, and
// used ones once their expiry has passed too.
func (s *passwordResetStore) DeleteExpired(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM password_reset_tokens WHERE expires_at < ?`, Now())
	return err
}
