package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"nexmap.net/internal/model"
)

// InvitationStore defines data access operations for invitations.
type InvitationStore interface {
	Create(ctx context.Context, inv *model.Invitation) error
	GetByTokenHash(ctx context.Context, hash string) (*model.Invitation, error)
	List(ctx context.Context) ([]*model.Invitation, error)
	Delete(ctx context.Context, id string) error
	MarkAccepted(ctx context.Context, id, acceptedAt string) error
}

type invitationStore struct {
	db *sql.DB
}

func (s *invitationStore) Create(ctx context.Context, inv *model.Invitation) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO invitations (id, email, token_hash, invited_by, product_id, role, accepted_at, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.ID, inv.Email, inv.TokenHash, inv.InvitedBy,
		nullableText(inv.ProductID), nullableText(inv.Role), nullableText(inv.AcceptedAt),
		inv.ExpiresAt, inv.CreatedAt,
	)
	return err
}

func (s *invitationStore) GetByTokenHash(ctx context.Context, hash string) (*model.Invitation, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, token_hash, invited_by, product_id, role, accepted_at, expires_at, created_at
		 FROM invitations WHERE token_hash = ?`, hash)
	inv, err := scanInvitation(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invitation not found: %w", err)
	}
	return inv, err
}

func (s *invitationStore) List(ctx context.Context) ([]*model.Invitation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, token_hash, invited_by, product_id, role, accepted_at, expires_at, created_at
		 FROM invitations ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}(rows)

	var invs []*model.Invitation
	for rows.Next() {
		inv, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		invs = append(invs, inv)
	}
	return invs, rows.Err()
}

func (s *invitationStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM invitations WHERE id = ?`, id)
	return err
}

func (s *invitationStore) MarkAccepted(ctx context.Context, id, acceptedAt string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE invitations SET accepted_at = ? WHERE id = ?`, acceptedAt, id)
	return err
}

func scanInvitation(s scanner) (*model.Invitation, error) {
	var inv model.Invitation
	var productID, role, acceptedAt sql.NullString
	err := s.Scan(&inv.ID, &inv.Email, &inv.TokenHash, &inv.InvitedBy,
		&productID, &role, &acceptedAt, &inv.ExpiresAt, &inv.CreatedAt)
	if err != nil {
		return nil, err
	}
	inv.ProductID = productID.String
	inv.Role = role.String
	inv.AcceptedAt = acceptedAt.String
	return &inv, nil
}
