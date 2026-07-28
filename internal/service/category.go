package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// CategoryService handles category CRUD.
type CategoryService struct {
	categories store.CategoryStore
	audit      store.AuditStore
	authz      *Authorizer
}

// CreateCategoryInput contains the fields required to create a category.
type CreateCategoryInput struct {
	ProductID  string
	EntityType string
	Name       string
	Prefix     string
	Position   int
}

// UpdateCategoryInput contains the fields that can be updated on a category.
type UpdateCategoryInput struct {
	Name     *string
	Prefix   *string
	Position *int
}

// CreateCategory creates a new category. Requires owner role.
func (s *CategoryService) CreateCategory(ctx context.Context, user *model.User, input CreateCategoryInput) (*model.Category, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleOwner); err != nil {
		return nil, err
	}

	if input.Name == "" {
		return nil, errors.New("category name is required")
	}
	if input.Prefix == "" {
		return nil, errors.New("category prefix is required")
	}
	if input.EntityType != "requirement" && input.EntityType != "task" && input.EntityType != "constraint" {
		return nil, errors.New("entity_type must be 'requirement', 'task' or 'constraint'")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	cat := &model.Category{
		ID:         uuid.New().String(),
		ProductID:  input.ProductID,
		EntityType: input.EntityType,
		Name:       input.Name,
		Prefix:     strings.ToUpper(input.Prefix),
		Position:   input.Position,
		CreatedAt:  now,
	}
	if err := s.categories.Create(ctx, cat); err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "category", EntityID: cat.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return cat, nil
}

// GetCategory returns a category by ID. Requires viewer role.
func (s *CategoryService) GetCategory(ctx context.Context, user *model.User, productID, id string) (*model.Category, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.categories.GetByID(ctx, id)
}

// UpdateCategory modifies a category. Requires owner role.
func (s *CategoryService) UpdateCategory(ctx context.Context, user *model.User, productID, id string, input UpdateCategoryInput) (*model.Category, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}

	cat, err := s.categories.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Name != nil {
		cat.Name = *input.Name
	}
	if input.Prefix != nil {
		cat.Prefix = strings.ToUpper(*input.Prefix)
	}
	if input.Position != nil {
		cat.Position = *input.Position
	}

	if err := s.categories.Update(ctx, cat); err != nil {
		return nil, fmt.Errorf("update category: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "category", EntityID: cat.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	return cat, nil
}

// DeleteCategory removes a category. Requires owner role.
// Returns an error if requirements or constraints still reference this category.
func (s *CategoryService) DeleteCategory(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.categories.Delete(ctx, id); err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY") || strings.Contains(err.Error(), "foreign key") ||
			strings.Contains(err.Error(), "RESTRICT") {
			return errors.New("cannot delete category: items are still assigned to it")
		}
		return fmt.Errorf("delete category: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "category", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListCategories returns categories for a product and entity type. Requires viewer role.
func (s *CategoryService) ListCategories(ctx context.Context, user *model.User, productID, entityType string) ([]*model.Category, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.categories.ListByProductID(ctx, productID, entityType)
}
