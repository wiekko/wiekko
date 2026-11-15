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
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx          store.Transactor
	constraints store.ConstraintStore
	categories  store.CategoryStore
	comments    ownerPurger
	audit       store.AuditStore
	authz       *Authorizer
	owns        *ownership
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
	// IfUpdatedAt is the updated_at of the version the client edited. When
	// set and the item has changed since, the update fails with ErrConflict.
	IfUpdatedAt     string
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
	if err := s.owns.require(ctx, input.ProductID, ref{kindCategory, input.CategoryID}, ref{kindMilestone, input.MilestoneID}); err != nil {
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

	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.Constraint{
		ID:              uuid.New().String(),
		ProductID:       input.ProductID,
		CategoryID:      input.CategoryID,
		Title:           input.Title,
		Description:     input.Description,
		ComplianceState: input.ComplianceState,
		MilestoneID:     input.MilestoneID,
		Priority:        input.Priority,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	// The sequence lock holds off concurrent creates between drawing the next
	// number and inserting it.
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := lock(ctx, s.tx, lockSequence("constraint", input.ProductID)); err != nil {
			return err
		}
		seq, err := s.constraints.NextSequenceNumber(ctx, input.ProductID)
		if err != nil {
			return fmt.Errorf("next sequence number: %w", err)
		}
		c.SequenceNumber = seq
		if err := s.constraints.Create(ctx, c); err != nil {
			return fmt.Errorf("create constraint: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), ProductID: c.ProductID, EntityType: "constraint", EntityID: c.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}

	return c, nil
}

// GetConstraint returns a constraint by ID. Requires viewer role.
func (s *ConstraintService) GetConstraint(ctx context.Context, user *model.User, productID, id string) (*model.Constraint, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindConstraint, id}); err != nil {
		return nil, err
	}
	return s.constraints.GetByID(ctx, id)
}

// UpdateConstraint modifies a constraint. Requires owner role.
func (s *ConstraintService) UpdateConstraint(ctx context.Context, user *model.User, productID, id string, input UpdateConstraintInput) (*model.Constraint, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindConstraint, id}); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindCategory, opt(input.CategoryID)}, ref{kindMilestone, opt(input.MilestoneID)}); err != nil {
		return nil, err
	}

	var c *model.Constraint
	err := inTx(ctx, s.tx, func(ctx context.Context) error {
		// The entity's lock serializes concurrent edits: each one reads the
		// row as the previous one left it, so none overwrites another's change.
		// IfUpdatedAt carries the version the client edited, if it sent one.
		if err := lock(ctx, s.tx, lockEntity(id)); err != nil {
			return err
		}
		var err error
		c, err = s.constraints.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if err := checkVersion(input.IfUpdatedAt, c.UpdatedAt); err != nil {
			return err
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

		if err := inTx(ctx, s.tx, func(ctx context.Context) error {
			if err := s.constraints.Update(ctx, c); err != nil {
				return fmt.Errorf("update constraint: %w", err)
			}
			return s.audit.Create(ctx, &model.AuditEntry{
				ID: uuid.New().String(), ProductID: c.ProductID, EntityType: "constraint", EntityID: c.ID,
				UserID: user.ID, Action: "update", CreatedAt: now,
			})
		}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// DeleteConstraint removes a constraint. Requires owner role.
func (s *ConstraintService) DeleteConstraint(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindConstraint, id}); err != nil {
		return err
	}

	return deleteOwned(ctx, s.tx, s.comments, s.audit, user, productID, model.OwnerTypeConstraint, id, s.constraints.Delete)
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
	if err := s.owns.require(ctx, productID, ref{kindConstraint, constraintID}, ref{kindCapability, capabilityID}); err != nil {
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
	if err := s.owns.require(ctx, productID, ref{kindConstraint, constraintID}, ref{kindCapability, capabilityID}); err != nil {
		return err
	}
	return s.constraints.UnlinkCapability(ctx, constraintID, capabilityID)
}

// ListConstraintCapabilities returns capability links for a constraint. Requires viewer role.
func (s *ConstraintService) ListConstraintCapabilities(ctx context.Context, user *model.User, productID, constraintID string) ([]*model.ConstraintCapability, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindConstraint, constraintID}); err != nil {
		return nil, err
	}
	return s.constraints.ListCapabilitiesByConstraint(ctx, constraintID)
}
