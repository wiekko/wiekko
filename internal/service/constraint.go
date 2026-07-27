package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// ConstraintService handles constraint CRUD.
type ConstraintService struct {
	constraints store.ConstraintStore
	categories  store.CategoryStore
	comments    ownerPurger
	audit       store.AuditStore
	authz       *Authorizer
}

// CreateConstraintInput contains the fields required to create a constraint.
type CreateConstraintInput struct {
	ProductID       string
	CategoryID      string
	Title           string
	Description     string
	ComplianceState string
	MilestoneID     string
	Priority        int
}

// UpdateConstraintInput contains the fields that can be updated on a constraint.
type UpdateConstraintInput struct {
	CategoryID      *string
	Title           *string
	Description     *string
	ComplianceState *string
	MilestoneID     *string
	Priority        *int
}

// CreateConstraint creates a new constraint. Requires owner role.
func (s *ConstraintService) CreateConstraint(ctx context.Context, user *model.User, input CreateConstraintInput) (*model.Constraint, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleOwner); err != nil {
		return nil, err
	}

	if input.Title == "" {
		return nil, errors.New("constraint title is required")
	}
	if input.CategoryID == "" {
		return nil, errors.New("constraint category is required")
	}

	// Verify category exists and is a constraint category.
	cat, err := s.categories.GetByID(ctx, input.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("invalid category: %w", err)
	}
	if cat.EntityType != "constraint" {
		return nil, errors.New("category is not a constraint category")
	}

	if input.ComplianceState == "" {
		input.ComplianceState = model.ComplianceNotAssessed
	}

	seq, err := s.constraints.NextSequenceNumber(ctx, input.ProductID)
	if err != nil {
		return nil, fmt.Errorf("next sequence number: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.Constraint{
		ID:              uuid.New().String(),
		ProductID:       input.ProductID,
		CategoryID:      input.CategoryID,
		SequenceNumber:  seq,
		Title:           input.Title,
		Description:     input.Description,
		ComplianceState: input.ComplianceState,
		MilestoneID:     input.MilestoneID,
		Priority:        input.Priority,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.constraints.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("create constraint: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "constraint", EntityID: c.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return c, nil
}

// GetConstraint returns a constraint by ID. Requires viewer role.
func (s *ConstraintService) GetConstraint(ctx context.Context, user *model.User, productID, id string) (*model.Constraint, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.constraints.GetByID(ctx, id)
}

// UpdateConstraint modifies a constraint. Requires owner role.
func (s *ConstraintService) UpdateConstraint(ctx context.Context, user *model.User, productID, id string, input UpdateConstraintInput) (*model.Constraint, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}

	c, err := s.constraints.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.CategoryID != nil {
		c.CategoryID = *input.CategoryID
	}
	if input.Title != nil {
		c.Title = *input.Title
	}
	if input.Description != nil {
		c.Description = *input.Description
	}
	if input.ComplianceState != nil {
		c.ComplianceState = *input.ComplianceState
	}
	if input.MilestoneID != nil {
		c.MilestoneID = *input.MilestoneID
	}
	if input.Priority != nil {
		c.Priority = *input.Priority
	}

	now := time.Now().UTC().Format(time.RFC3339)
	c.UpdatedAt = now

	if err := s.constraints.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("update constraint: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "constraint", EntityID: c.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	return c, nil
}

// DeleteConstraint removes a constraint. Requires owner role.
func (s *ConstraintService) DeleteConstraint(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.constraints.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete constraint: %w", err)
	}

	// Comments and attachments hang off polymorphic owner columns, so no
	// foreign key cascades them away with the row above.
	if err := s.comments.PurgeOwner(ctx, productID, model.OwnerTypeConstraint, id); err != nil {
		return err
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "constraint", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListConstraints returns constraints for a product. Requires viewer role.
func (s *ConstraintService) ListConstraints(ctx context.Context, user *model.User, productID string, filter store.ConstraintFilter) ([]*model.Constraint, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.constraints.ListByProductID(ctx, productID, filter)
}

// ListConstraintsByMilestone returns the constraints assigned to a milestone.
// Rows belonging to another product are skipped. Requires viewer role.
func (s *ConstraintService) ListConstraintsByMilestone(ctx context.Context, user *model.User, productID, milestoneID string) ([]*model.Constraint, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	all, err := s.constraints.ListByMilestoneID(ctx, milestoneID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Constraint, 0, len(all))
	for _, c := range all {
		if c.ProductID != productID {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// LinkCapability links a capability to a constraint. Requires owner role.
func (s *ConstraintService) LinkCapability(ctx context.Context, user *model.User, productID, constraintID, capabilityID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	return s.constraints.LinkCapability(ctx, &model.ConstraintCapability{
		ID:           uuid.New().String(),
		ConstraintID: constraintID,
		CapabilityID: capabilityID,
		CreatedAt:    now,
	})
}

// UnlinkCapability removes a capability link from a constraint. Requires owner role.
func (s *ConstraintService) UnlinkCapability(ctx context.Context, user *model.User, productID, constraintID, capabilityID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	return s.constraints.UnlinkCapability(ctx, constraintID, capabilityID)
}

// ListConstraintCapabilities returns capability links for a constraint. Requires viewer role.
func (s *ConstraintService) ListConstraintCapabilities(ctx context.Context, user *model.User, productID, constraintID string) ([]*model.ConstraintCapability, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.constraints.ListCapabilitiesByConstraint(ctx, constraintID)
}
