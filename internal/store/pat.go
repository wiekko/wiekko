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
	db *DB
}

const patColumns = `id, user_id, name, token_hash, token_prefix, last_used_at, expires_at, created_at, revoked_at,
	scopes, role_cap, products_restricted`

// Create stores a token together with the products it is restricted to.
func (s *patStore) Create(ctx context.Context, pat *model.PAT) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("pat create: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO personal_access_tokens (`+patColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pat.ID, pat.UserID, pat.Name, pat.TokenHash, pat.TokenPrefix,
		nullableText(pat.LastUsedAt), nullableText(pat.ExpiresAt), pat.CreatedAt, nullableText(pat.RevokedAt),
		pat.Scopes, nullableText(pat.RoleCap), pat.ProductsRestricted,
	)
	if err != nil {
		return fmt.Errorf("pat create: insert token: %w", err)
	}
	for _, productID := range pat.ProductIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO personal_access_token_products (token_id, product_id) VALUES (?, ?)`,
			pat.ID, productID); err != nil {
			return fmt.Errorf("pat create: insert product: %w", err)
		}
	}
	return tx.Commit()
}

// loadProducts fills in the products of restricted tokens.
func (s *patStore) loadProducts(ctx context.Context, pats ...*model.PAT) error {
	for _, p := range pats {
		if !p.ProductsRestricted {
			continue
		}
		rows, err := s.db.QueryContext(ctx,
			`SELECT product_id FROM personal_access_token_products WHERE token_id = ? ORDER BY product_id`, p.ID)
		if err != nil {
			return err
		}
		p.ProductIDs = []string{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				closeRows(rows)
				return err
			}
			p.ProductIDs = append(p.ProductIDs, id)
		}
		err = rows.Err()
		closeRows(rows)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *patStore) GetByTokenHash(ctx context.Context, hash string) (*model.PAT, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+patColumns+` FROM personal_access_tokens WHERE token_hash = ?`, hash)
	pat, err := scanPAT(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("token not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	if err := s.loadProducts(ctx, pat); err != nil {
		return nil, err
	}
	return pat, nil
}

func (s *patStore) ListByUserID(ctx context.Context, userID string) ([]*model.PAT, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+patColumns+` FROM personal_access_tokens WHERE user_id = ? ORDER BY created_at DESC`, userID)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	closeRows(rows)
	if err := s.loadProducts(ctx, pats...); err != nil {
		return nil, err
	}
	return pats, nil
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
	var lastUsed, expiresAt, revokedAt, roleCap sql.NullString
	err := s.Scan(&pat.ID, &pat.UserID, &pat.Name, &pat.TokenHash, &pat.TokenPrefix,
		&lastUsed, &expiresAt, &pat.CreatedAt, &revokedAt,
		&pat.Scopes, &roleCap, &pat.ProductsRestricted)
	if err != nil {
		return nil, err
	}
	pat.LastUsedAt = lastUsed.String
	pat.ExpiresAt = expiresAt.String
	pat.RevokedAt = revokedAt.String
	pat.RoleCap = roleCap.String
	return &pat, nil
}
