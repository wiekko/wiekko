package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// CommentStore defines data access operations for comments.
type CommentStore interface {
	Create(ctx context.Context, c *model.Comment) error
	GetByID(ctx context.Context, id string) (*model.Comment, error)
	Update(ctx context.Context, c *model.Comment) error
	Delete(ctx context.Context, id string) error
	ListByOwner(ctx context.Context, ownerType, ownerID string) ([]*model.Comment, error)
}

type commentStore struct {
	db *sql.DB
}

func (s *commentStore) Create(ctx context.Context, c *model.Comment) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO comments (id, owner_type, owner_id, user_id, body, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.OwnerType, c.OwnerID, c.UserID, c.Body, c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (s *commentStore) GetByID(ctx context.Context, id string) (*model.Comment, error) {
	var c model.Comment
	err := s.db.QueryRowContext(ctx,
		`SELECT id, owner_type, owner_id, user_id, body, created_at, updated_at
		 FROM comments WHERE id = ?`, id,
	).Scan(&c.ID, &c.OwnerType, &c.OwnerID, &c.UserID, &c.Body, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("comment not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *commentStore) Update(ctx context.Context, c *model.Comment) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE comments SET body = ?, updated_at = ? WHERE id = ?`,
		c.Body, c.UpdatedAt, c.ID,
	)
	return err
}

func (s *commentStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM comments WHERE id = ?`, id)
	return err
}

func (s *commentStore) ListByOwner(ctx context.Context, ownerType, ownerID string) ([]*model.Comment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, owner_type, owner_id, user_id, body, created_at, updated_at
		 FROM comments WHERE owner_type = ? AND owner_id = ? ORDER BY created_at`,
		ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var comments []*model.Comment
	for rows.Next() {
		var c model.Comment
		if err := rows.Scan(&c.ID, &c.OwnerType, &c.OwnerID, &c.UserID, &c.Body, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, &c)
	}
	return comments, rows.Err()
}
