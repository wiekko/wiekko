package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// ConstraintFilter controls which constraints are returned by ListByProductID.
type ConstraintFilter struct {
	CategoryID string // empty = all categories
}

// ConstraintStore defines data access operations for constraints.
type ConstraintStore interface {
	Create(ctx context.Context, c *model.Constraint) error
	GetByID(ctx context.Context, id string) (*model.Constraint, error)
	Update(ctx context.Context, c *model.Constraint) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string, filter ConstraintFilter) ([]*model.Constraint, error)
	ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Constraint, error)
	// GetBySequence returns the product's constraint with the given sequence number.
	GetBySequence(ctx context.Context, productID string, seq int) (*model.Constraint, error)
	NextSequenceNumber(ctx context.Context, productID string) (int, error)
	CountByCategoryID(ctx context.Context, categoryID string) (int, error)
	LinkCapability(ctx context.Context, link *model.ConstraintCapability) error
	UnlinkCapability(ctx context.Context, constraintID, capabilityID string) error
	ListCapabilitiesByConstraint(ctx context.Context, constraintID string) ([]*model.ConstraintCapability, error)
	ListConstraintsByCapability(ctx context.Context, capabilityID string) ([]*model.ConstraintCapability, error)
}

type constraintStore struct {
	db *DB
}

func (s *constraintStore) Create(ctx context.Context, c *model.Constraint) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO constraints (id, product_id, category_id, sequence_number, title, description,
		 compliance_state, milestone_id, priority, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ProductID, c.CategoryID, c.SequenceNumber,
		c.Title, c.Description, c.ComplianceState,
		nullableText(c.MilestoneID), c.Priority,
		c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (s *constraintStore) GetByID(ctx context.Context, id string) (*model.Constraint, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+constraintColumns+` FROM constraints WHERE id = ?`, id)
	c, err := scanConstraint(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("constraint not found: %w", err)
	}
	return c, err
}

func (s *constraintStore) GetBySequence(ctx context.Context, productID string, seq int) (*model.Constraint, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+constraintColumns+` FROM constraints WHERE product_id = ? AND sequence_number = ?`, productID, seq)
	c, err := scanConstraint(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("constraint not found: %w", err)
	}
	return c, err
}

func (s *constraintStore) Update(ctx context.Context, c *model.Constraint) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE constraints SET category_id = ?, title = ?, description = ?,
		 compliance_state = ?, milestone_id = ?, priority = ?, updated_at = ?
		 WHERE id = ?`,
		c.CategoryID, c.Title, c.Description,
		c.ComplianceState, nullableText(c.MilestoneID), c.Priority,
		c.UpdatedAt, c.ID,
	)
	return err
}

func (s *constraintStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM constraints WHERE id = ?`, id)
	return err
}

func (s *constraintStore) ListByProductID(ctx context.Context, productID string, filter ConstraintFilter) ([]*model.Constraint, error) {
	query := `SELECT ` + constraintColumns + ` FROM constraints WHERE product_id = ?`
	args := []any{productID}
	if filter.CategoryID != "" {
		query += ` AND category_id = ?`
		args = append(args, filter.CategoryID)
	}
	query += ` ORDER BY ` + itemOrder

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var constraints []*model.Constraint
	for rows.Next() {
		c, err := scanConstraint(rows)
		if err != nil {
			return nil, err
		}
		constraints = append(constraints, c)
	}
	return constraints, rows.Err()
}

func (s *constraintStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Constraint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+constraintColumns+` FROM constraints WHERE milestone_id = ? ORDER BY `+itemOrder,
		milestoneID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var constraints []*model.Constraint
	for rows.Next() {
		c, err := scanConstraint(rows)
		if err != nil {
			return nil, err
		}
		constraints = append(constraints, c)
	}
	return constraints, rows.Err()
}

func (s *constraintStore) NextSequenceNumber(ctx context.Context, productID string) (int, error) {
	var maxSeq sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(sequence_number) FROM constraints WHERE product_id = ?`,
		productID).Scan(&maxSeq)
	if err != nil {
		return 0, err
	}
	if maxSeq.Valid {
		return int(maxSeq.Int64) + 1, nil
	}
	return 1, nil
}

func (s *constraintStore) CountByCategoryID(ctx context.Context, categoryID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM constraints WHERE category_id = ?`,
		categoryID).Scan(&count)
	return count, err
}

func (s *constraintStore) LinkCapability(ctx context.Context, link *model.ConstraintCapability) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO constraint_capabilities (id, constraint_id, capability_id, created_at)
		 VALUES (?, ?, ?, ?)`,
		link.ID, link.ConstraintID, link.CapabilityID, link.CreatedAt,
	)
	return err
}

func (s *constraintStore) UnlinkCapability(ctx context.Context, constraintID, capabilityID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM constraint_capabilities WHERE constraint_id = ? AND capability_id = ?`,
		constraintID, capabilityID)
	return err
}

func (s *constraintStore) ListCapabilitiesByConstraint(ctx context.Context, constraintID string) ([]*model.ConstraintCapability, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, constraint_id, capability_id, created_at
		 FROM constraint_capabilities WHERE constraint_id = ? ORDER BY created_at, id`,
		constraintID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var links []*model.ConstraintCapability
	for rows.Next() {
		var l model.ConstraintCapability
		if err := rows.Scan(&l.ID, &l.ConstraintID, &l.CapabilityID, &l.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, &l)
	}
	return links, rows.Err()
}

func (s *constraintStore) ListConstraintsByCapability(ctx context.Context, capabilityID string) ([]*model.ConstraintCapability, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, constraint_id, capability_id, created_at
		 FROM constraint_capabilities WHERE capability_id = ? ORDER BY created_at, id`,
		capabilityID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var links []*model.ConstraintCapability
	for rows.Next() {
		var l model.ConstraintCapability
		if err := rows.Scan(&l.ID, &l.ConstraintID, &l.CapabilityID, &l.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, &l)
	}
	return links, rows.Err()
}

const constraintColumns = `id, product_id, category_id, sequence_number, title, description,
	compliance_state, milestone_id, priority, created_at, updated_at`

func scanConstraint(sc scanner) (*model.Constraint, error) {
	var c model.Constraint
	var milestoneID sql.NullString
	err := sc.Scan(&c.ID, &c.ProductID, &c.CategoryID, &c.SequenceNumber,
		&c.Title, &c.Description, &c.ComplianceState,
		&milestoneID, &c.Priority, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.MilestoneID = milestoneID.String
	return &c, nil
}
