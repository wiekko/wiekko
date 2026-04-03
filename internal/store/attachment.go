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
	ListByOwner(ctx context.Context, ownerType, ownerID string) ([]*model.Attachment, error)
}

type attachmentStore struct {
	db *sql.DB
}

func (s *attachmentStore) Create(ctx context.Context, a *model.Attachment) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO attachments (id, owner_type, owner_id, user_id, filename, content_type, size, storage_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.OwnerType, a.OwnerID, a.UserID,
		a.Filename, a.ContentType, a.Size, a.StoragePath, a.CreatedAt,
	)
	return err
}

func (s *attachmentStore) GetByID(ctx context.Context, id string) (*model.Attachment, error) {
	var a model.Attachment
	err := s.db.QueryRowContext(ctx,
		`SELECT id, owner_type, owner_id, user_id, filename, content_type, size, storage_path, created_at
		 FROM attachments WHERE id = ?`, id,
	).Scan(&a.ID, &a.OwnerType, &a.OwnerID, &a.UserID,
		&a.Filename, &a.ContentType, &a.Size, &a.StoragePath, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("attachment not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *attachmentStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM attachments WHERE id = ?`, id)
	return err
}

func (s *attachmentStore) ListByOwner(ctx context.Context, ownerType, ownerID string) ([]*model.Attachment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, owner_type, owner_id, user_id, filename, content_type, size, storage_path, created_at
		 FROM attachments WHERE owner_type = ? AND owner_id = ? ORDER BY created_at`,
		ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var attachments []*model.Attachment
	for rows.Next() {
		var a model.Attachment
		if err := rows.Scan(&a.ID, &a.OwnerType, &a.OwnerID, &a.UserID,
			&a.Filename, &a.ContentType, &a.Size, &a.StoragePath, &a.CreatedAt); err != nil {
			return nil, err
		}
		attachments = append(attachments, &a)
	}
	return attachments, rows.Err()
}
