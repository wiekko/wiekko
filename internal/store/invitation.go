package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"nexmap.net/internal/crypto"
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
	db   *DB
	keys *crypto.KeyRing
}

const invSelectColumns = `i.id, i.token_hash, i.invited_by, i.product_id, i.role, i.accepted_at, i.expires_at, i.created_at,
	pd_email.ciphertext, pd_email.key_id`

const invJoins = `FROM invitations i
	LEFT JOIN personal_data pd_email ON pd_email.owner_table = 'invitations' AND pd_email.owner_id = i.id AND pd_email.field_name = 'email'`

func (s *invitationStore) Create(ctx context.Context, inv *model.Invitation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("invitation create: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO invitations (id, token_hash, invited_by, product_id, role, accepted_at, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.ID, inv.TokenHash, inv.InvitedBy,
		nullableText(inv.ProductID), nullableText(inv.Role), nullableText(inv.AcceptedAt),
		inv.ExpiresAt, inv.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("invitation create: insert invitation: %w", err)
	}

	emailCT, emailKeyID, err := s.keys.EncryptFor(inv.Email, personalDataContext("invitations", inv.ID, "email"))
	if err != nil {
		return fmt.Errorf("invitation create: encrypt email: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO personal_data (id, owner_table, owner_id, field_name, ciphertext, key_id, blind_index)
		 VALUES (?, 'invitations', ?, 'email', ?, ?, NULL)`,
		uuid.New().String(), inv.ID, emailCT, emailKeyID,
	)
	if err != nil {
		return fmt.Errorf("invitation create: insert email pd: %w", err)
	}

	return tx.Commit()
}

func (s *invitationStore) GetByTokenHash(ctx context.Context, hash string) (*model.Invitation, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+invSelectColumns+` `+invJoins+` WHERE i.token_hash = ?`, hash)
	inv, err := s.scanInvitation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("invitation not found: %w", err)
	}
	return inv, err
}

func (s *invitationStore) List(ctx context.Context) ([]*model.Invitation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+invSelectColumns+` `+invJoins+` ORDER BY i.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var invs []*model.Invitation
	for rows.Next() {
		inv, err := s.scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		invs = append(invs, inv)
	}
	return invs, rows.Err()
}

func (s *invitationStore) Delete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("invitation delete: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `DELETE FROM personal_data WHERE owner_table = 'invitations' AND owner_id = ?`, id)
	if err != nil {
		return fmt.Errorf("invitation delete: delete personal data: %w", err)
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM invitations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("invitation delete: delete invitation: %w", err)
	}

	return tx.Commit()
}

func (s *invitationStore) MarkAccepted(ctx context.Context, id, acceptedAt string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE invitations SET accepted_at = ? WHERE id = ?`, acceptedAt, id)
	return err
}

func (s *invitationStore) scanInvitation(sc scanner) (*model.Invitation, error) {
	var inv model.Invitation
	var productID, role, acceptedAt sql.NullString
	var emailCT, emailKeyID sql.NullString

	err := sc.Scan(&inv.ID, &inv.TokenHash, &inv.InvitedBy,
		&productID, &role, &acceptedAt, &inv.ExpiresAt, &inv.CreatedAt,
		&emailCT, &emailKeyID)
	if err != nil {
		return nil, err
	}

	inv.ProductID = productID.String
	inv.Role = role.String
	inv.AcceptedAt = acceptedAt.String

	if emailCT.Valid && emailKeyID.Valid {
		inv.Email, err = s.keys.DecryptFor(emailCT.String, emailKeyID.String, personalDataContext("invitations", inv.ID, "email"))
		if err != nil {
			return nil, fmt.Errorf("decrypt invitation email: %w", err)
		}
	}

	return &inv, nil
}
