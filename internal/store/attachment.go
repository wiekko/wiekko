package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// AttachmentStore defines data access operations for attachments.
type AttachmentStore interface {
	Create(ctx context.Context, a *model.Attachment) error
	GetByID(ctx context.Context, id string) (*model.Attachment, error)
	Delete(ctx context.Context, id string) error
	ListByOwner(ctx context.Context, productID, ownerType, ownerID string) ([]*model.Attachment, error)
}

type attachmentStore struct {
	db *sql.DB
}

const attachmentColumns = `id, product_id, owner_type, owner_id, user_id,
	filename, content_type, size, storage_path, created_at`

func scanAttachment(sc scanner) (*model.Attachment, error) {
	var a model.Attachment
	err := sc.Scan(&a.ID, &a.ProductID, &a.OwnerType, &a.OwnerID, &a.UserID,
		&a.Filename, &a.ContentType, &a.Size, &a.StoragePath, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *attachmentStore) Create(ctx context.Context, a *model.Attachment) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO attachments (`+attachmentColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.ProductID, a.OwnerType, a.OwnerID, a.UserID,
		a.Filename, a.ContentType, a.Size, a.StoragePath, a.CreatedAt,
	)
	return err
}

func (s *attachmentStore) GetByID(ctx context.Context, id string) (*model.Attachment, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+attachmentColumns+` FROM attachments WHERE id = ?`, id)
	a, err := scanAttachment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("attachment not found: %w", err)
	}
	return a, err
}

func (s *attachmentStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM attachments WHERE id = ?`, id)
	return err
}

func (s *attachmentStore) ListByOwner(ctx context.Context, productID, ownerType, ownerID string) ([]*model.Attachment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+attachmentColumns+`
		 FROM attachments WHERE product_id = ? AND owner_type = ? AND owner_id = ?
		 ORDER BY created_at, id`,
		productID, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var attachments []*model.Attachment
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, a)
	}
	return attachments, rows.Err()
}
