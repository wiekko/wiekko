package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// PATStore defines data access operations for personal access tokens.
type PATStore interface {
	Create(ctx context.Context, pat *model.PAT) error
	GetByTokenHash(ctx context.Context, hash string) (*model.PAT, error)
	ListByUserID(ctx context.Context, userID string) ([]*model.PAT, error)
	Revoke(ctx context.Context, id, revokedAt string) error
	UpdateLastUsed(ctx context.Context, id, at string) error
}

type patStore struct {
	db *sql.DB
}

func (s *patStore) Create(ctx context.Context, pat *model.PAT) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO personal_access_tokens (id, user_id, name, token_hash, token_prefix, last_used_at, expires_at, created_at, revoked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pat.ID, pat.UserID, pat.Name, pat.TokenHash, pat.TokenPrefix,
		nullableText(pat.LastUsedAt), nullableText(pat.ExpiresAt), pat.CreatedAt, nullableText(pat.RevokedAt),
	)
	return err
}

func (s *patStore) GetByTokenHash(ctx context.Context, hash string) (*model.PAT, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, last_used_at, expires_at, created_at, revoked_at
		 FROM personal_access_tokens WHERE token_hash = ?`, hash)
	pat, err := scanPAT(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("token not found: %w", err)
	}
	return pat, err
}

func (s *patStore) ListByUserID(ctx context.Context, userID string) ([]*model.PAT, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, last_used_at, expires_at, created_at, revoked_at
		 FROM personal_access_tokens WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var pats []*model.PAT
	for rows.Next() {
		p, err := scanPAT(rows)
		if err != nil {
			return nil, err
		}
		pats = append(pats, p)
	}
	return pats, rows.Err()
}

func (s *patStore) Revoke(ctx context.Context, id, revokedAt string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE personal_access_tokens SET revoked_at = ? WHERE id = ?`, revokedAt, id)
	return err
}

func (s *patStore) UpdateLastUsed(ctx context.Context, id, at string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE personal_access_tokens SET last_used_at = ? WHERE id = ?`, at, id)
	return err
}

func scanPAT(s scanner) (*model.PAT, error) {
	var pat model.PAT
	var lastUsed, expiresAt, revokedAt sql.NullString
	err := s.Scan(&pat.ID, &pat.UserID, &pat.Name, &pat.TokenHash, &pat.TokenPrefix,
		&lastUsed, &expiresAt, &pat.CreatedAt, &revokedAt)
	if err != nil {
		return nil, err
	}
	pat.LastUsedAt = lastUsed.String
	pat.ExpiresAt = expiresAt.String
	pat.RevokedAt = revokedAt.String
	return &pat, nil
}
