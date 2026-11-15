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

// ComponentService handles component and capability CRUD.
type ComponentService struct {
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx           store.Transactor
	components   store.ComponentStore
	capabilities store.CapabilityStore
	comments     ownerPurger
	audit        store.AuditStore
	authz        *Authorizer
	owns         *ownership
}

// CreateComponentInput contains the fields required to create a component.
type CreateComponentInput struct {
	ProductID   string
	Name        string
	Description string
	MilestoneID string
}

// UpdateComponentInput contains the fields that can be updated on a component.
type UpdateComponentInput struct {
	// IfUpdatedAt is the updated_at of the version the client edited. When
	// set and the item has changed since, the update fails with ErrConflict.
	IfUpdatedAt string
	Name        *string
	Description *string
	MilestoneID *string
}

// CreateCapabilityInput contains the fields required to create a capability.
type CreateCapabilityInput struct {
	ProductID   string
	ComponentID string
	Name        string
	Description string
	MilestoneID string
}

// UpdateCapabilityInput contains the fields that can be updated on a capability.
type UpdateCapabilityInput struct {
	// IfUpdatedAt is the updated_at of the version the client edited. When
	// set and the item has changed since, the update fails with ErrConflict.
	IfUpdatedAt string
	Name        *string
	Description *string
	ComponentID *string
	MilestoneID *string
}

// CreateComponent creates a new component. Requires owner role.
func (s *ComponentService) CreateComponent(ctx context.Context, user *model.User, input CreateComponentInput) (*model.Component, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, input.ProductID, ref{kindMilestone, input.MilestoneID}); err != nil {
		return nil, err
	}

	if input.Name == "" {
		return nil, errors.New("component name is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.Component{
		ID:          uuid.New().String(),
		ProductID:   input.ProductID,
		Name:        input.Name,
		Description: input.Description,
		MilestoneID: input.MilestoneID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.components.Create(ctx, c); err != nil {
			return fmt.Errorf("create component: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "component", EntityID: c.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}

	return c, nil
}

// GetComponent returns a component by ID. Requires viewer role.
func (s *ComponentService) GetComponent(ctx context.Context, user *model.User, productID, id string) (*model.Component, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindComponent, id}); err != nil {
		return nil, err
	}
	return s.components.GetByID(ctx, id)
}

// UpdateComponent modifies a component. Requires owner role.
func (s *ComponentService) UpdateComponent(ctx context.Context, user *model.User, productID, id string, input UpdateComponentInput) (*model.Component, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindComponent, id}); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindMilestone, opt(input.MilestoneID)}); err != nil {
		return nil, err
	}

	var c *model.Component
	err := inTx(ctx, s.tx, func(ctx context.Context) error {
		// The entity's lock serializes concurrent edits: each one reads the
		// row as the previous one left it, so none overwrites another's change.
		// IfUpdatedAt carries the version the client edited, if it sent one.
		if err := lock(ctx, s.tx, lockEntity(id)); err != nil {
			return err
		}
		var err error
		c, err = s.components.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if err := checkVersion(input.IfUpdatedAt, c.UpdatedAt); err != nil {
			return err
		}

		now := time.Now().UTC().Format(time.RFC3339)
		if input.Name != nil {
			c.Name = *input.Name
		}
		if input.Description != nil {
			c.Description = *input.Description
		}
		if input.MilestoneID != nil {
			c.MilestoneID = *input.MilestoneID
		}
		c.UpdatedAt = now

		if err := inTx(ctx, s.tx, func(ctx context.Context) error {
			if err := s.components.Update(ctx, c); err != nil {
				return fmt.Errorf("update component: %w", err)
			}
			return s.audit.Create(ctx, &model.AuditEntry{
				ID: uuid.New().String(), EntityType: "component", EntityID: c.ID,
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

// DeleteComponent removes a component. Requires owner role.
func (s *ComponentService) DeleteComponent(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindComponent, id}); err != nil {
		return err
	}

	return deleteOwned(ctx, s.tx, s.comments, s.audit, user, productID, model.OwnerTypeComponent, id, s.components.Delete)
}

// ListComponents returns all components for a product. Requires viewer role.
func (s *ComponentService) ListComponents(ctx context.Context, user *model.User, productID string) ([]*model.Component, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.components.ListByProductID(ctx, productID)
}

// CreateCapability creates a new capability. Requires owner role.
func (s *ComponentService) CreateCapability(ctx context.Context, user *model.User, input CreateCapabilityInput) (*model.Capability, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, input.ProductID, ref{kindComponent, input.ComponentID}, ref{kindMilestone, input.MilestoneID}); err != nil {
		return nil, err
	}

	if input.Name == "" {
		return nil, errors.New("capability name is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.Capability{
		ID:          uuid.New().String(),
		ProductID:   input.ProductID,
		ComponentID: input.ComponentID,
		Name:        input.Name,
		Description: input.Description,
		MilestoneID: input.MilestoneID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.capabilities.Create(ctx, c); err != nil {
			return fmt.Errorf("create capability: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "capability", EntityID: c.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}

	return c, nil
}

// GetCapability returns a capability by ID. Requires viewer role.
func (s *ComponentService) GetCapability(ctx context.Context, user *model.User, productID, id string) (*model.Capability, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindCapability, id}); err != nil {
		return nil, err
	}
	return s.capabilities.GetByID(ctx, id)
}

// UpdateCapability modifies a capability. Requires owner role.
func (s *ComponentService) UpdateCapability(ctx context.Context, user *model.User, productID, id string, input UpdateCapabilityInput) (*model.Capability, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindCapability, id}); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindComponent, opt(input.ComponentID)}, ref{kindMilestone, opt(input.MilestoneID)}); err != nil {
		return nil, err
	}

	var c *model.Capability
	err := inTx(ctx, s.tx, func(ctx context.Context) error {
		// The entity's lock serializes concurrent edits: each one reads the
		// row as the previous one left it, so none overwrites another's change.
		// IfUpdatedAt carries the version the client edited, if it sent one.
		if err := lock(ctx, s.tx, lockEntity(id)); err != nil {
			return err
		}
		var err error
		c, err = s.capabilities.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if err := checkVersion(input.IfUpdatedAt, c.UpdatedAt); err != nil {
			return err
		}

		now := time.Now().UTC().Format(time.RFC3339)
		if input.Name != nil {
			c.Name = *input.Name
		}
		if input.Description != nil {
			c.Description = *input.Description
		}
		if input.ComponentID != nil {
			c.ComponentID = *input.ComponentID
		}
		if input.MilestoneID != nil {
			c.MilestoneID = *input.MilestoneID
		}
		c.UpdatedAt = now

		if err := inTx(ctx, s.tx, func(ctx context.Context) error {
			if err := s.capabilities.Update(ctx, c); err != nil {
				return fmt.Errorf("update capability: %w", err)
			}
			return s.audit.Create(ctx, &model.AuditEntry{
				ID: uuid.New().String(), EntityType: "capability", EntityID: c.ID,
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

// DeleteCapability removes a capability. Requires owner role.
func (s *ComponentService) DeleteCapability(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindCapability, id}); err != nil {
		return err
	}

	return deleteOwned(ctx, s.tx, s.comments, s.audit, user, productID, model.OwnerTypeCapability, id, s.capabilities.Delete)
}

// ListCapabilities returns all capabilities for a product. Requires viewer role.
func (s *ComponentService) ListCapabilities(ctx context.Context, user *model.User, productID string) ([]*model.Capability, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.capabilities.ListByProductID(ctx, productID)
}

// ListCapabilitiesByComponent returns capabilities for a component. Requires viewer role.
func (s *ComponentService) ListCapabilitiesByComponent(ctx context.Context, user *model.User, productID, componentID string) ([]*model.Capability, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindComponent, componentID}); err != nil {
		return nil, err
	}
	return s.capabilities.ListByComponentID(ctx, componentID)
}

// ListComponentsByMilestone returns the components assigned to a milestone.
// Rows belonging to another product are skipped. Requires viewer role.
func (s *ComponentService) ListComponentsByMilestone(ctx context.Context, user *model.User, productID, milestoneID string) ([]*model.Component, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	all, err := s.components.ListByMilestoneID(ctx, milestoneID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Component, 0, len(all))
	for _, c := range all {
		if c.ProductID != productID {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// ListCapabilitiesByMilestone returns the capabilities assigned to a milestone.
// Rows belonging to another product are skipped. Requires viewer role.
func (s *ComponentService) ListCapabilitiesByMilestone(ctx context.Context, user *model.User, productID, milestoneID string) ([]*model.Capability, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	all, err := s.capabilities.ListByMilestoneID(ctx, milestoneID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Capability, 0, len(all))
	for _, c := range all {
		if c.ProductID != productID {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// LinkRequirementToCapability creates a link between a requirement and a capability. Requires owner role.
func (s *ComponentService) LinkRequirementToCapability(ctx context.Context, user *model.User, productID, capabilityID, requirementID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindCapability, capabilityID}, ref{kindRequirement, requirementID}); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	return s.capabilities.LinkRequirement(ctx, &model.RequirementCapability{
		ID:            uuid.New().String(),
		RequirementID: requirementID,
		CapabilityID:  capabilityID,
		CreatedAt:     now,
	})
}

// UnlinkRequirementFromCapability removes a link between a requirement and a capability. Requires owner role.
func (s *ComponentService) UnlinkRequirementFromCapability(ctx context.Context, user *model.User, productID, capabilityID, requirementID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindCapability, capabilityID}, ref{kindBacklogItem, requirementID}); err != nil {
		return err
	}
	return s.capabilities.UnlinkRequirement(ctx, capabilityID, requirementID)
}

// ListCapabilityRequirements returns requirement links for a capability. Requires viewer role.
func (s *ComponentService) ListCapabilityRequirements(ctx context.Context, user *model.User, productID, capabilityID string) ([]*model.RequirementCapability, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindCapability, capabilityID}); err != nil {
		return nil, err
	}
	return s.capabilities.ListRequirementsByCapability(ctx, capabilityID)
}

// ListRequirementCapabilities returns capability links for a requirement. Requires viewer role.
func (s *ComponentService) ListRequirementCapabilities(ctx context.Context, user *model.User, productID, requirementID string) ([]*model.RequirementCapability, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindBacklogItem, requirementID}); err != nil {
		return nil, err
	}
	return s.capabilities.ListCapabilitiesByRequirement(ctx, requirementID)
}

// ListLinkedCapabilities returns the full capability records linked to a
// requirement. Requires viewer role.
func (s *ComponentService) ListLinkedCapabilities(ctx context.Context, user *model.User, productID, requirementID string) ([]*model.Capability, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindBacklogItem, requirementID}); err != nil {
		return nil, err
	}
	links, err := s.capabilities.ListCapabilitiesByRequirement(ctx, requirementID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Capability, 0, len(links))
	for _, link := range links {
		cap, err := s.capabilities.GetByID(ctx, link.CapabilityID)
		if err != nil {
			continue
		}
		out = append(out, cap)
	}
	return out, nil
}
