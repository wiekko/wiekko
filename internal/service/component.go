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

// ComponentService handles component and functionality CRUD.
type ComponentService struct {
	components      store.ComponentStore
	functionalities store.FunctionalityStore
	audit           store.AuditStore
	authz           *Authorizer
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
	Name        *string
	Description *string
	MilestoneID *string
}

// CreateFunctionalityInput contains the fields required to create a functionality.
type CreateFunctionalityInput struct {
	ProductID   string
	ComponentID string
	Name        string
	Description string
	MilestoneID string
}

// UpdateFunctionalityInput contains the fields that can be updated on a functionality.
type UpdateFunctionalityInput struct {
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
	if err := s.components.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("create component: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "component", EntityID: c.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return c, nil
}

// GetComponent returns a component by ID. Requires viewer role.
func (s *ComponentService) GetComponent(ctx context.Context, user *model.User, productID, id string) (*model.Component, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.components.GetByID(ctx, id)
}

// UpdateComponent modifies a component. Requires owner role.
func (s *ComponentService) UpdateComponent(ctx context.Context, user *model.User, productID, id string, input UpdateComponentInput) (*model.Component, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}

	c, err := s.components.GetByID(ctx, id)
	if err != nil {
		return nil, err
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

	if err := s.components.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("update component: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "component", EntityID: c.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	return c, nil
}

// DeleteComponent removes a component. Requires owner role.
func (s *ComponentService) DeleteComponent(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.components.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete component: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "component", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListComponents returns all components for a product. Requires viewer role.
func (s *ComponentService) ListComponents(ctx context.Context, user *model.User, productID string) ([]*model.Component, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.components.ListByProductID(ctx, productID)
}

// CreateFunctionality creates a new functionality. Requires owner role.
func (s *ComponentService) CreateFunctionality(ctx context.Context, user *model.User, input CreateFunctionalityInput) (*model.Functionality, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleOwner); err != nil {
		return nil, err
	}

	if input.Name == "" {
		return nil, errors.New("functionality name is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	f := &model.Functionality{
		ID:          uuid.New().String(),
		ProductID:   input.ProductID,
		ComponentID: input.ComponentID,
		Name:        input.Name,
		Description: input.Description,
		MilestoneID: input.MilestoneID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.functionalities.Create(ctx, f); err != nil {
		return nil, fmt.Errorf("create functionality: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "functionality", EntityID: f.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return f, nil
}

// GetFunctionality returns a functionality by ID. Requires viewer role.
func (s *ComponentService) GetFunctionality(ctx context.Context, user *model.User, productID, id string) (*model.Functionality, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.functionalities.GetByID(ctx, id)
}

// UpdateFunctionality modifies a functionality. Requires owner role.
func (s *ComponentService) UpdateFunctionality(ctx context.Context, user *model.User, productID, id string, input UpdateFunctionalityInput) (*model.Functionality, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}

	f, err := s.functionalities.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if input.Name != nil {
		f.Name = *input.Name
	}
	if input.Description != nil {
		f.Description = *input.Description
	}
	if input.ComponentID != nil {
		f.ComponentID = *input.ComponentID
	}
	if input.MilestoneID != nil {
		f.MilestoneID = *input.MilestoneID
	}
	f.UpdatedAt = now

	if err := s.functionalities.Update(ctx, f); err != nil {
		return nil, fmt.Errorf("update functionality: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "functionality", EntityID: f.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	return f, nil
}

// DeleteFunctionality removes a functionality. Requires owner role.
func (s *ComponentService) DeleteFunctionality(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.functionalities.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete functionality: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "functionality", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListFunctionalities returns all functionalities for a product. Requires viewer role.
func (s *ComponentService) ListFunctionalities(ctx context.Context, user *model.User, productID string) ([]*model.Functionality, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.functionalities.ListByProductID(ctx, productID)
}

// ListFunctionalitiesByComponent returns functionalities for a component. Requires viewer role.
func (s *ComponentService) ListFunctionalitiesByComponent(ctx context.Context, user *model.User, productID, componentID string) ([]*model.Functionality, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.functionalities.ListByComponentID(ctx, componentID)
}
