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
	products      store.ProductStore
	productRoles  store.ProductRoleStore
	statuses      store.StatusStore
	categories    store.CategoryStore
	pyramidLevels store.PyramidLevelStore
	settings      store.SettingStore
	attachments   productPurger
	audit         store.AuditStore
	authz         *Authorizer
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
		StatusSync:     true,
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

	s.seedDefaults(ctx, product.ID, now)

	return product, nil
}

// seedDefaults creates default statuses and pyramid levels for a new product.
func (s *ProductService) seedDefaults(ctx context.Context, productID, now string) {
	// Default statuses (synced: same for both entity types).
	defaultStatuses := []struct {
		Name      string
		Position  int
		IsDefault bool
		IsClosed  bool
	}{
		{Name: "New", Position: 1, IsDefault: true},
		{Name: "In Progress", Position: 2},
		{Name: "In Review", Position: 3},
		{Name: "Done", Position: 4, IsClosed: true},
	}

	for _, et := range []string{"requirement", "defect"} {
		for _, ds := range defaultStatuses {
			_ = s.statuses.Create(ctx, &model.Status{
				ID:         uuid.New().String(),
				ProductID:  productID,
				EntityType: et,
				Name:       ds.Name,
				Position:   ds.Position,
				IsDefault:  ds.IsDefault,
				IsClosed:   ds.IsClosed,
				CreatedAt:  now,
			})
		}
	}

	// Default requirement categories.
	defaultReqCategories := []struct {
		Name     string
		Prefix   string
		Position int
	}{
		{Name: "Functionality", Prefix: "FR", Position: 1},
		{Name: "Architecture", Prefix: "AR", Position: 2},
		{Name: "Security", Prefix: "SR", Position: 3},
		{Name: "UX", Prefix: "UR", Position: 4},
	}

	for _, dc := range defaultReqCategories {
		_ = s.categories.Create(ctx, &model.Category{
			ID:         uuid.New().String(),
			ProductID:  productID,
			EntityType: "requirement",
			Name:       dc.Name,
			Prefix:     dc.Prefix,
			Position:   dc.Position,
			CreatedAt:  now,
		})
	}

	// Default task categories. Tasks carry no product statement, so they get
	// their own prefix axis rather than sharing the requirement one.
	defaultTaskCategories := []struct {
		Name     string
		Prefix   string
		Position int
	}{
		{Name: "Maintenance", Prefix: "MT", Position: 1},
		{Name: "Tooling", Prefix: "TL", Position: 2},
		{Name: "Spike", Prefix: "SP", Position: 3},
	}

	for _, dc := range defaultTaskCategories {
		_ = s.categories.Create(ctx, &model.Category{
			ID:         uuid.New().String(),
			ProductID:  productID,
			EntityType: "task",
			Name:       dc.Name,
			Prefix:     dc.Prefix,
			Position:   dc.Position,
			CreatedAt:  now,
		})
	}

	// Default constraint categories.
	defaultConCategories := []struct {
		Name     string
		Prefix   string
		Position int
	}{
		{Name: "Architecture", Prefix: "AC", Position: 1},
		{Name: "Security", Prefix: "SC", Position: 2},
		{Name: "UX", Prefix: "UC", Position: 3},
	}

	for _, dc := range defaultConCategories {
		_ = s.categories.Create(ctx, &model.Category{
			ID:         uuid.New().String(),
			ProductID:  productID,
			EntityType: "constraint",
			Name:       dc.Name,
			Prefix:     dc.Prefix,
			Position:   dc.Position,
			CreatedAt:  now,
		})
	}

	// Default pyramid levels.
	defaultLevels := []struct {
		Name     string
		Position int
	}{
		{Name: "Unit", Position: 1},
		{Name: "Integration", Position: 2},
		{Name: "End-to-End", Position: 3},
	}

	for _, dl := range defaultLevels {
		_ = s.pyramidLevels.Create(ctx, &model.PyramidLevel{
			ID:        uuid.New().String(),
			ProductID: productID,
			Name:      dl.Name,
			Position:  dl.Position,
			CreatedAt: now,
		})
	}
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

	// Comment and attachment rows cascade with the product, but the stored
	// attachment bytes are outside SQL's reach — unlink them while the rows
	// that name their paths still exist.
	if err := s.attachments.PurgeProduct(ctx, productID); err != nil {
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

// ProductWithRole pairs a product with the requesting user's role in it.
type ProductWithRole struct {
	Product *model.Product
	Role    string // "admin" when an admin has no explicit membership
}

// List returns all products the user has access to. Admins see all.
func (s *ProductService) List(ctx context.Context, user *model.User) ([]*model.Product, error) {
	withRoles, err := s.ListWithRoles(ctx, user)
	if err != nil {
		return nil, err
	}

	var products []*model.Product
	for _, pr := range withRoles {
		products = append(products, pr.Product)
	}
	return products, nil
}

// ListWithRoles returns the products the user can access along with their role
// in each. It resolves all roles from a single query rather than one lookup per
// product, which keeps it cheap enough for the product switcher.
func (s *ProductService) ListWithRoles(ctx context.Context, user *model.User) ([]ProductWithRole, error) {
	roles, err := s.productRoles.ListByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	roleByProduct := make(map[string]string, len(roles))
	for _, role := range roles {
		roleByProduct[role.ProductID] = role.Role
	}

	// Admins see every product; an explicit membership still wins over the bypass.
	if user.IsAdmin {
		products, err := s.products.List(ctx)
		if err != nil {
			return nil, err
		}
		result := make([]ProductWithRole, 0, len(products))
		for _, p := range products {
			role := roleByProduct[p.ID]
			if role == "" {
				role = "admin"
			}
			result = append(result, ProductWithRole{Product: p, Role: role})
		}
		return result, nil
	}

	var result []ProductWithRole
	for _, role := range roles {
		p, err := s.products.GetByID(ctx, role.ProductID)
		if err != nil {
			continue
		}
		result = append(result, ProductWithRole{Product: p, Role: role.Role})
	}
	return result, nil
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

// SplitStatuses permanently separates requirement and defect statuses for a product.
// This is a one-way operation and cannot be undone. Requires owner role.
func (s *ProductService) SplitStatuses(ctx context.Context, user *model.User, productID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	product, err := s.products.GetByID(ctx, productID)
	if err != nil {
		return err
	}

	if !product.StatusSync {
		return errors.New("statuses are already split")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	product.StatusSync = false
	product.UpdatedAt = now

	if err := s.products.Update(ctx, product); err != nil {
		return fmt.Errorf("split statuses: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "product", EntityID: product.ID,
		UserID: user.ID, Action: "split_statuses", CreatedAt: now,
	})

	return nil
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
