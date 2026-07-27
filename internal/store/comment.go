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
	ListByOwner(ctx context.Context, productID, ownerType, ownerID string) ([]*model.Comment, error)
}

type commentStore struct {
	db *sql.DB
}

const commentColumns = `id, product_id, owner_type, owner_id, user_id, body, created_at, updated_at`

func scanComment(sc scanner) (*model.Comment, error) {
	var c model.Comment
	err := sc.Scan(&c.ID, &c.ProductID, &c.OwnerType, &c.OwnerID, &c.UserID,
		&c.Body, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *commentStore) Create(ctx context.Context, c *model.Comment) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO comments (`+commentColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ProductID, c.OwnerType, c.OwnerID, c.UserID, c.Body, c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (s *commentStore) GetByID(ctx context.Context, id string) (*model.Comment, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+commentColumns+` FROM comments WHERE id = ?`, id)
	c, err := scanComment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("comment not found: %w", err)
	}
	return c, err
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

// ListByOwner returns a thread oldest-first. created_at has second granularity,
// so id breaks ties to keep the order stable.
func (s *commentStore) ListByOwner(ctx context.Context, productID, ownerType, ownerID string) ([]*model.Comment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+commentColumns+`
		 FROM comments WHERE product_id = ? AND owner_type = ? AND owner_id = ?
		 ORDER BY created_at, id`,
		productID, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var comments []*model.Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}
