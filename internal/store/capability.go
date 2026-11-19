package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// CapabilityStore defines data access operations for capabilities.
type CapabilityStore interface {
	Create(ctx context.Context, c *model.Capability) error
	GetByID(ctx context.Context, id string) (*model.Capability, error)
	Update(ctx context.Context, c *model.Capability) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string) ([]*model.Capability, error)
	ListByComponentID(ctx context.Context, componentID string) ([]*model.Capability, error)
	ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Capability, error)
	LinkRequirement(ctx context.Context, link *model.RequirementCapability) error
	UnlinkRequirement(ctx context.Context, capabilityID, requirementID string) error
	ListRequirementsByCapability(ctx context.Context, capabilityID string) ([]*model.RequirementCapability, error)
	ListCapabilitiesByRequirement(ctx context.Context, requirementID string) ([]*model.RequirementCapability, error)
}

type capabilityStore struct {
	db *DB
}

func (s *capabilityStore) Create(ctx context.Context, c *model.Capability) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO capabilities (id, product_id, component_id, name, description, milestone_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ProductID, nullableText(c.ComponentID),
		c.Name, c.Description, nullableText(c.MilestoneID),
		c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (s *capabilityStore) GetByID(ctx context.Context, id string) (*model.Capability, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, component_id, name, description, milestone_id, created_at, updated_at
		 FROM capabilities WHERE id = ?`, id)
	c, err := scanCapability(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("capability not found: %w", err)
	}
	return c, err
}

func (s *capabilityStore) Update(ctx context.Context, c *model.Capability) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE capabilities SET component_id = ?, name = ?, description = ?, milestone_id = ?, updated_at = ?
		 WHERE id = ?`,
		nullableText(c.ComponentID), c.Name, c.Description,
		nullableText(c.MilestoneID), c.UpdatedAt, c.ID,
	)
	return err
}

func (s *capabilityStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM capabilities WHERE id = ?`, id)
	return err
}

func (s *capabilityStore) ListByProductID(ctx context.Context, productID string) ([]*model.Capability, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, component_id, name, description, milestone_id, created_at, updated_at
		 FROM capabilities WHERE product_id = ? ORDER BY name`, productID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var caps []*model.Capability
	for rows.Next() {
		c, err := scanCapability(rows)
		if err != nil {
			return nil, err
		}
		caps = append(caps, c)
	}
	return caps, rows.Err()
}

func (s *capabilityStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Capability, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, component_id, name, description, milestone_id, created_at, updated_at
		 FROM capabilities WHERE milestone_id = ? ORDER BY name`, milestoneID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var caps []*model.Capability
	for rows.Next() {
		c, err := scanCapability(rows)
		if err != nil {
			return nil, err
		}
		caps = append(caps, c)
	}
	return caps, rows.Err()
}

func (s *capabilityStore) ListByComponentID(ctx context.Context, componentID string) ([]*model.Capability, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, component_id, name, description, milestone_id, created_at, updated_at
		 FROM capabilities WHERE component_id = ? ORDER BY name`, componentID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var caps []*model.Capability
	for rows.Next() {
		c, err := scanCapability(rows)
		if err != nil {
			return nil, err
		}
		caps = append(caps, c)
	}
	return caps, rows.Err()
}

// LinkRequirement is idempotent: linking a pair that is already linked keeps
// the existing link and succeeds (DF-3).
func (s *capabilityStore) LinkRequirement(ctx context.Context, link *model.RequirementCapability) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO requirement_capabilities (id, requirement_id, capability_id, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (requirement_id, capability_id) DO NOTHING`,
		link.ID, link.RequirementID, link.CapabilityID, link.CreatedAt,
	)
	return err
}

func (s *capabilityStore) UnlinkRequirement(ctx context.Context, capabilityID, requirementID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM requirement_capabilities WHERE capability_id = ? AND requirement_id = ?`,
		capabilityID, requirementID)
	return err
}

func (s *capabilityStore) ListRequirementsByCapability(ctx context.Context, capabilityID string) ([]*model.RequirementCapability, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, requirement_id, capability_id, created_at
		 FROM requirement_capabilities WHERE capability_id = ? ORDER BY created_at, id`,
		capabilityID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var links []*model.RequirementCapability
	for rows.Next() {
		var l model.RequirementCapability
		if err := rows.Scan(&l.ID, &l.RequirementID, &l.CapabilityID, &l.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, &l)
	}
	return links, rows.Err()
}

func (s *capabilityStore) ListCapabilitiesByRequirement(ctx context.Context, requirementID string) ([]*model.RequirementCapability, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, requirement_id, capability_id, created_at
		 FROM requirement_capabilities WHERE requirement_id = ? ORDER BY created_at, id`,
		requirementID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var links []*model.RequirementCapability
	for rows.Next() {
		var l model.RequirementCapability
		if err := rows.Scan(&l.ID, &l.RequirementID, &l.CapabilityID, &l.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, &l)
	}
	return links, rows.Err()
}

func scanCapability(sc scanner) (*model.Capability, error) {
	var c model.Capability
	var componentID, milestoneID sql.NullString
	err := sc.Scan(&c.ID, &c.ProductID, &componentID, &c.Name, &c.Description,
		&milestoneID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.ComponentID = componentID.String
	c.MilestoneID = milestoneID.String
	return &c, nil
}
