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

// ProductService handles product CRUD and member management.
type ProductService struct {
	products     store.ProductStore
	productRoles store.ProductRoleStore
	settings     store.SettingStore
	audit        store.AuditStore
	authz        *Authorizer
}

// CreateProductInput contains the fields required to create a product.
type CreateProductInput struct {
	Name           string
	Description    string
	EstimationMode string
	WorkflowMode   string
}

// UpdateProductInput contains the fields that can be updated on a product.
type UpdateProductInput struct {
	Name           *string
	Description    *string
	EstimationMode *string
	WorkflowMode   *string
}

// Create creates a new product and assigns the creator as owner (FR-303).
// Checks product_creation_enabled setting unless user is admin (FR-301, FR-302).
func (s *ProductService) Create(ctx context.Context, user *model.User, input CreateProductInput) (*model.Product, error) {
	if !user.IsAdmin {
		enabled, err := s.settings.Get(ctx, "product_creation_enabled")
		if err != nil || enabled != "true" {
			return nil, fmt.Errorf("%w: product creation is disabled", ErrForbidden)
		}
	}

	if input.Name == "" {
		return nil, errors.New("product name is required")
	}
	if input.EstimationMode == "" {
		input.EstimationMode = model.EstimationModeDisabled
	}
	if input.WorkflowMode == "" {
		input.WorkflowMode = model.WorkflowModeKanban
	}

	now := time.Now().UTC().Format(time.RFC3339)
	product := &model.Product{
		ID:             uuid.New().String(),
		Name:           input.Name,
		Description:    input.Description,
		EstimationMode: input.EstimationMode,
		WorkflowMode:   input.WorkflowMode,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.products.Create(ctx, product); err != nil {
		return nil, fmt.Errorf("create product: %w", err)
	}

	// Assign creator as owner (FR-303).
	role := &model.ProductRole{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		ProductID: product.ID,
		Role:      model.RoleOwner,
		CreatedAt: now,
	}
	if err := s.productRoles.Create(ctx, role); err != nil {
		return nil, fmt.Errorf("assign owner role: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "product", EntityID: product.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return product, nil
}

// GetByID returns a product by ID. Requires viewer role.
func (s *ProductService) GetByID(ctx context.Context, user *model.User, productID string) (*model.Product, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.products.GetByID(ctx, productID)
}

// Update modifies a product. Requires owner role.
func (s *ProductService) Update(ctx context.Context, user *model.User, productID string, input UpdateProductInput) (*model.Product, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}

	product, err := s.products.GetByID(ctx, productID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if input.Name != nil {
		product.Name = *input.Name
	}
	if input.Description != nil {
		product.Description = *input.Description
	}
	if input.EstimationMode != nil {
		product.EstimationMode = *input.EstimationMode
	}
	if input.WorkflowMode != nil {
		product.WorkflowMode = *input.WorkflowMode
	}
	product.UpdatedAt = now

	if err := s.products.Update(ctx, product); err != nil {
		return nil, fmt.Errorf("update product: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "product", EntityID: product.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	return product, nil
}

// Delete removes a product. Requires owner role.
func (s *ProductService) Delete(ctx context.Context, user *model.User, productID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.products.Delete(ctx, productID); err != nil {
		return fmt.Errorf("delete product: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "product", EntityID: productID,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// List returns all products the user has access to. Admins see all.
func (s *ProductService) List(ctx context.Context, user *model.User) ([]*model.Product, error) {
	if user.IsAdmin {
		return s.products.List(ctx)
	}

	roles, err := s.productRoles.ListByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	var products []*model.Product
	for _, role := range roles {
		p, err := s.products.GetByID(ctx, role.ProductID)
		if err != nil {
			continue
		}
		products = append(products, p)
	}
	return products, nil
}

// AddMember adds a user to a product with the given role (FR-304).
func (s *ProductService) AddMember(ctx context.Context, user *model.User, productID, targetUserID, role string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	pr := &model.ProductRole{
		ID:        uuid.New().String(),
		UserID:    targetUserID,
		ProductID: productID,
		Role:      role,
		CreatedAt: now,
	}
	return s.productRoles.Create(ctx, pr)
}

// UpdateMember changes a member's role.
func (s *ProductService) UpdateMember(ctx context.Context, user *model.User, productID, targetUserID, role string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	pr := &model.ProductRole{
		UserID:    targetUserID,
		ProductID: productID,
		Role:      role,
	}
	return s.productRoles.Update(ctx, pr)
}

// RemoveMember removes a user from a product.
func (s *ProductService) RemoveMember(ctx context.Context, user *model.User, productID, targetUserID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	return s.productRoles.Delete(ctx, targetUserID, productID)
}

// ListMembers returns all members of a product.
func (s *ProductService) ListMembers(ctx context.Context, user *model.User, productID string) ([]*model.ProductRole, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}

	return s.productRoles.ListByProductID(ctx, productID)
}

// TransferOwnership transfers product ownership to another user (FR-306).
func (s *ProductService) TransferOwnership(ctx context.Context, user *model.User, productID, newOwnerID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	// Demote current owner to developer.
	if err := s.productRoles.Update(ctx, &model.ProductRole{
		UserID: user.ID, ProductID: productID, Role: model.RoleDeveloper,
	}); err != nil {
		return fmt.Errorf("demote current owner: %w", err)
	}

	// Check if new owner already has a role.
	existing, err := s.productRoles.GetByUserAndProduct(ctx, newOwnerID, productID)
	if err != nil {
		// No existing role — create one.
		now := time.Now().UTC().Format(time.RFC3339)
		return s.productRoles.Create(ctx, &model.ProductRole{
			ID: uuid.New().String(), UserID: newOwnerID, ProductID: productID,
			Role: model.RoleOwner, CreatedAt: now,
		})
	}

	// Promote to owner.
	existing.Role = model.RoleOwner
	return s.productRoles.Update(ctx, existing)
}
