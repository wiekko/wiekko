// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"wiekko.net/internal/model"
)

// TwoFactorStore holds TOTP recovery codes and pending login challenges.
type TwoFactorStore interface {
	// ReplaceRecoveryCodes deletes the user's recovery codes and stores the given hashes.
	ReplaceRecoveryCodes(ctx context.Context, userID string, hashes []string) error
	// DeleteRecoveryCodes removes all of the user's recovery codes.
	DeleteRecoveryCodes(ctx context.Context, userID string) error
	// ConsumeRecoveryCode marks an unused code as used; false if none matched.
	ConsumeRecoveryCode(ctx context.Context, userID, hash string) (bool, error)
	// CountUnusedRecoveryCodes returns how many codes the user has left.
	CountUnusedRecoveryCodes(ctx context.Context, userID string) (int, error)

	CreateChallenge(ctx context.Context, c *model.LoginChallenge) error
	GetChallenge(ctx context.Context, id string) (*model.LoginChallenge, error)
	// IncrementChallengeAttempts bumps the attempt counter and returns the new value.
	IncrementChallengeAttempts(ctx context.Context, id string) (int, error)
	DeleteChallenge(ctx context.Context, id string) error
	DeleteExpiredChallenges(ctx context.Context, now string) error
}

type twoFactorStore struct {
	db *DB
}

func (s *twoFactorStore) ReplaceRecoveryCodes(ctx context.Context, userID string, hashes []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("recovery codes replace: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM totp_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("recovery codes replace: delete: %w", err)
	}
	now := Now()
	for _, h := range hashes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO totp_recovery_codes (id, user_id, code_hash, created_at) VALUES (?, ?, ?, ?)`,
			uuid.New().String(), userID, h, now,
		); err != nil {
			return fmt.Errorf("recovery codes replace: insert: %w", err)
		}
	}
	return tx.Commit()
}

func (s *twoFactorStore) DeleteRecoveryCodes(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM totp_recovery_codes WHERE user_id = ?`, userID)
	return err
}

func (s *twoFactorStore) ConsumeRecoveryCode(ctx context.Context, userID, hash string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE totp_recovery_codes SET used_at = ?
		 WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`,
		Now(), userID, hash,
	)
	if err != nil {
		return false, fmt.Errorf("recovery code consume: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("recovery code consume: %w", err)
	}
	return n > 0, nil
}

func (s *twoFactorStore) CountUnusedRecoveryCodes(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM totp_recovery_codes WHERE user_id = ? AND used_at IS NULL`, userID,
	).Scan(&n)
	return n, err
}

func (s *twoFactorStore) CreateChallenge(ctx context.Context, c *model.LoginChallenge) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO login_challenges (id, user_id, attempts, created_at, expires_at, ip_address, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.UserID, c.Attempts, c.CreatedAt, c.ExpiresAt, c.IPAddress, c.UserAgent,
	)
	return err
}

func (s *twoFactorStore) GetChallenge(ctx context.Context, id string) (*model.LoginChallenge, error) {
	var c model.LoginChallenge
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, attempts, created_at, expires_at, ip_address, user_agent
		 FROM login_challenges WHERE id = ?`, id,
	).Scan(&c.ID, &c.UserID, &c.Attempts, &c.CreatedAt, &c.ExpiresAt, &c.IPAddress, &c.UserAgent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("login challenge not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *twoFactorStore) IncrementChallengeAttempts(ctx context.Context, id string) (int, error) {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE login_challenges SET attempts = attempts + 1 WHERE id = ?`, id); err != nil {
		return 0, fmt.Errorf("login challenge attempts: %w", err)
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT attempts FROM login_challenges WHERE id = ?`, id).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("login challenge attempts: %w", err)
	}
	return n, nil
}

func (s *twoFactorStore) DeleteChallenge(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_challenges WHERE id = ?`, id)
	return err
}

func (s *twoFactorStore) DeleteExpiredChallenges(ctx context.Context, now string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_challenges WHERE expires_at < ?`, now)
	return err
}
