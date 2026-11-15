package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/access"
	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// ProductService handles product CRUD and member management.
type ProductService struct {
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx            store.Transactor
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
	// IfUpdatedAt is the updated_at of the version the client edited. When
	// set and the item has changed since, the update fails with ErrConflict.
	IfUpdatedAt    string
	Name           *string
	Description    *string
	EstimationMode *string
	WorkflowMode   *string
	// DefectPrefix renames every defect identifier at once (FR-043); it must
	// not be a prefix of any of the product's categories.
	DefectPrefix *string
	// DocumentPrefix renames every document identifier at once (FR-050); it
	// must differ from the defect prefix and every category prefix.
	DocumentPrefix *string
	// TwoFactorRequired keeps members without 2FA out of the product
	// (SC-107). Only users with 2FA set up may turn it on.
	TwoFactorRequired *bool
}

// ErrOwnTwoFactorRequired is returned when a user without 2FA tries to make a
// product require it.
var ErrOwnTwoFactorRequired = errors.New("set up two-factor authentication on your own account before requiring it for the product")

// Create creates a new product and assigns the creator as owner (FR-303).
// Checks product_creation_enabled setting unless user is admin (FR-301, FR-302).
func (s *ProductService) Create(ctx context.Context, user *model.User, input CreateProductInput) (*model.Product, error) {
	// A product-restricted token could not reach the product it creates.
	if r := access.FromContext(ctx); r != nil && r.ProductsRestricted {
		return nil, fmt.Errorf("%w: this token is restricted to existing products", ErrForbidden)
	}
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
	// The product, its owner and its default workflow come into existence
	// together: a product without an owner is unreachable, and one without
	// default categories rejects new requirements.
	err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.products.Create(ctx, product); err != nil {
			return fmt.Errorf("create product: %w", err)
		}
		// Assign creator as owner (FR-303).
		if err := s.productRoles.Create(ctx, &model.ProductRole{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			ProductID: product.ID,
			Role:      model.RoleOwner,
			CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("assign owner role: %w", err)
		}
		if err := s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "product", EntityID: product.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		}); err != nil {
			return err
		}
		return s.seedDefaults(ctx, product.ID, now)
	})
	if err != nil {
		return nil, err
	}

	return product, nil
}

// seedDefaults creates default statuses and pyramid levels for a new product.
func (s *ProductService) seedDefaults(ctx context.Context, productID, now string) error {
	// Default statuses (synced: same for both entity types).
	// New items land in "New", which is not active: an owner activates them
	// by moving them on (kanban, FR-318). "Done" is not active either, so
	// only owners reopen finished work.
	defaultStatuses := []struct {
		Name      string
		Position  int
		IsDefault bool
		IsClosed  bool
		IsActive  bool
	}{
		{Name: "New", Position: 1, IsDefault: true},
		{Name: "In Progress", Position: 2, IsActive: true},
		{Name: "In Review", Position: 3, IsActive: true},
		{Name: "Done", Position: 4, IsClosed: true},
	}

	for _, et := range []string{"requirement", "defect"} {
		for _, ds := range defaultStatuses {
			if err := s.statuses.Create(ctx, &model.Status{
				ID:         uuid.New().String(),
				ProductID:  productID,
				EntityType: et,
				Name:       ds.Name,
				Position:   ds.Position,
				IsDefault:  ds.IsDefault,
				IsClosed:   ds.IsClosed,
				IsActive:   ds.IsActive,
				CreatedAt:  now,
			}); err != nil {
				return fmt.Errorf("seed defaults: %w", err)
			}
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
		if err := s.categories.Create(ctx, &model.Category{
			ID:         uuid.New().String(),
			ProductID:  productID,
			EntityType: "requirement",
			Name:       dc.Name,
			Prefix:     dc.Prefix,
			Position:   dc.Position,
			CreatedAt:  now,
		}); err != nil {
			return fmt.Errorf("seed defaults: %w", err)
		}
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
		if err := s.categories.Create(ctx, &model.Category{
			ID:         uuid.New().String(),
			ProductID:  productID,
			EntityType: "task",
			Name:       dc.Name,
			Prefix:     dc.Prefix,
			Position:   dc.Position,
			CreatedAt:  now,
		}); err != nil {
			return fmt.Errorf("seed defaults: %w", err)
		}
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
		if err := s.categories.Create(ctx, &model.Category{
			ID:         uuid.New().String(),
			ProductID:  productID,
			EntityType: "constraint",
			Name:       dc.Name,
			Prefix:     dc.Prefix,
			Position:   dc.Position,
			CreatedAt:  now,
		}); err != nil {
			return fmt.Errorf("seed defaults: %w", err)
		}
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
		if err := s.pyramidLevels.Create(ctx, &model.PyramidLevel{
			ID:        uuid.New().String(),
			ProductID: productID,
			Name:      dl.Name,
			Position:  dl.Position,
			CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("seed defaults: %w", err)
		}
	}
	return nil
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

	var product *model.Product
	err := inTx(ctx, s.tx, func(ctx context.Context) error {
		// The entity's lock serializes concurrent edits: each one reads the
		// row as the previous one left it, so none overwrites another's change.
		// IfUpdatedAt carries the version the client edited, if it sent one.
		if err := lock(ctx, s.tx, lockEntity(productID)); err != nil {
			return err
		}
		var err error
		product, err = s.products.GetByID(ctx, productID)
		if err != nil {
			return err
		}
		if err := checkVersion(input.IfUpdatedAt, product.UpdatedAt); err != nil {
			return err
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
		if input.DefectPrefix != nil {
			prefix, err := normalizeDefectPrefix(*input.DefectPrefix)
			if err != nil {
				return err
			}
			if cat, err := s.categories.GetByProductAndPrefix(ctx, productID, prefix); err == nil && cat != nil {
				return fmt.Errorf("prefix %q is already used by the %s category %q", prefix, cat.EntityType, cat.Name)
			}
			product.DefectPrefix = prefix
		}
		if input.DocumentPrefix != nil {
			prefix, err := normalizeDocumentPrefix(*input.DocumentPrefix)
			if err != nil {
				return err
			}
			if cat, err := s.categories.GetByProductAndPrefix(ctx, productID, prefix); err == nil && cat != nil {
				return fmt.Errorf("prefix %q is already used by the %s category %q", prefix, cat.EntityType, cat.Name)
			}
			product.DocumentPrefix = prefix
		}
		if (input.DefectPrefix != nil || input.DocumentPrefix != nil) && product.DefectPrefix == product.DocumentPrefix {
			return fmt.Errorf("defects and documents cannot share the prefix %q", product.DefectPrefix)
		}
		if input.TwoFactorRequired != nil {
			if *input.TwoFactorRequired && !product.TwoFactorRequired && !user.TOTPEnabled {
				return ErrOwnTwoFactorRequired
			}
			product.TwoFactorRequired = *input.TwoFactorRequired
		}
		switchesToScrum := false
		if input.WorkflowMode != nil {
			switchesToScrum = *input.WorkflowMode == model.WorkflowModeScrum && product.WorkflowMode != model.WorkflowModeScrum
			product.WorkflowMode = *input.WorkflowMode
		}
		product.UpdatedAt = now

		err = inTx(ctx, s.tx, func(ctx context.Context) error {
			if err := s.products.Update(ctx, product); err != nil {
				return fmt.Errorf("update product: %w", err)
			}
			// In kanban mode an item's column decides its activation; in scrum
			// mode the stored flag does, so it has to start from the sprint plan.
			if switchesToScrum {
				if err := s.products.ResetActivationToSprints(ctx, product.ID); err != nil {
					return err
				}
			}
			return s.audit.Create(ctx, &model.AuditEntry{
				ID: uuid.New().String(), EntityType: "product", EntityID: product.ID,
				UserID: user.ID, Action: "update", CreatedAt: now,
			})
		})
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return product, nil
}

// Delete removes a product. Requires owner role.
func (s *ProductService) Delete(ctx context.Context, user *model.User, productID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.products.Delete(ctx, productID); err != nil {
			return fmt.Errorf("delete product: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "product", EntityID: productID,
			UserID: user.ID, Action: "delete", CreatedAt: now,
		})
	})
	if err != nil {
		return err
	}

	// Comment and attachment rows went with the product (foreign keys), but
	// the stored attachment bytes are outside SQL's reach. Unlink them only
	// now: had the delete failed, the product would still need them.
	return s.attachments.PurgeProduct(context.WithoutCancel(ctx), productID)
}

// ProductWithRole pairs a product with the requesting user's role in it.
type ProductWithRole struct {
	Product *model.Product
	Role    string // "admin" when an admin has no explicit membership
	// TwoFactorLocked is set when the product requires 2FA and the user has
	// not set it up: the product is listed, but cannot be opened (SC-107).
	TwoFactorLocked bool
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
		return markTwoFactorLocked(ctx, user, restrictProductRoles(ctx, result)), nil
	}

	var result []ProductWithRole
	for _, role := range roles {
		p, err := s.products.GetByID(ctx, role.ProductID)
		if err != nil {
			continue
		}
		result = append(result, ProductWithRole{Product: p, Role: role.Role})
	}
	return markTwoFactorLocked(ctx, user, restrictProductRoles(ctx, result)), nil
}

// markTwoFactorLocked flags the products the user cannot open for lack of
// 2FA, mirroring Authorizer.checkTwoFactor.
func markTwoFactorLocked(ctx context.Context, user *model.User, list []ProductWithRole) []ProductWithRole {
	if user.TOTPEnabled || access.ViaToken(ctx) {
		return list
	}
	for i := range list {
		list[i].TwoFactorLocked = list[i].Product.TwoFactorRequired
	}
	return list
}

// restrictProductRoles applies a token's restriction to a product listing:
// only the token's products are listed, with their roles capped (SC-402,
// SC-403). An administrator's "admin" pseudo-role counts as owner.
func restrictProductRoles(ctx context.Context, list []ProductWithRole) []ProductWithRole {
	r := access.FromContext(ctx)
	if r == nil {
		return list
	}
	out := make([]ProductWithRole, 0, len(list))
	for _, pr := range list {
		if !r.AllowsProduct(pr.Product.ID) {
			continue
		}
		if r.RoleCap != "" {
			role := pr.Role
			if role == "admin" {
				role = model.RoleOwner
			}
			pr.Role = r.CapRole(role)
		}
		out = append(out, pr)
	}
	return out
}

// AddMember adds a user to a product with the given role (FR-304).
func (s *ProductService) AddMember(ctx context.Context, user *model.User, productID, targetUserID, role string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if roleRank(role) < 0 {
		return fmt.Errorf("invalid role %q", role)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	pr := &model.ProductRole{
		ID:        uuid.New().String(),
		UserID:    targetUserID,
		ProductID: productID,
		Role:      role,
		CreatedAt: now,
	}
	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.productRoles.Create(ctx, pr); err != nil {
			return err
		}
		return s.auditMember(ctx, user, productID, targetUserID, "", role)
	})
}

// ErrLastOwner is returned when a change would leave a product without an
// owner: nobody but an administrator could then manage it or its team.
var ErrLastOwner = errors.New("this is the product's only owner; make another member an owner first")

// memberRole returns the target's current role in the product ("" if not a
// member) and whether another member is an owner.
func (s *ProductService) memberRole(ctx context.Context, productID, targetUserID string) (role string, otherOwner bool, err error) {
	members, err := s.productRoles.ListByProductID(ctx, productID)
	if err != nil {
		return "", false, fmt.Errorf("list members: %w", err)
	}
	for _, m := range members {
		switch {
		case m.UserID == targetUserID:
			role = m.Role
		case m.Role == model.RoleOwner:
			otherOwner = true
		}
	}
	return role, otherOwner, nil
}

// auditMember records a change to a product's team in the product's audit
// trail: who may do what in a product is as much part of its history as its
// content. An empty old or new role stands for "not a member".
func (s *ProductService) auditMember(ctx context.Context, actor *model.User, productID, targetUserID, oldRole, newRole string) error {
	if oldRole == "" {
		oldRole = "no access"
	}
	if newRole == "" {
		newRole = "no access"
	}
	return s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "product", EntityID: productID,
		UserID: actor.ID, Action: "update", FieldName: "role of user " + targetUserID,
		OldValue: oldRole, NewValue: newRole,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// UpdateMember changes a member's role. The last-owner check and the change
// run under the product's member lock, so two concurrent demotions cannot
// both see the other owner and leave none.
func (s *ProductService) UpdateMember(ctx context.Context, user *model.User, productID, targetUserID, role string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if roleRank(role) < 0 {
		return fmt.Errorf("invalid role %q", role)
	}
	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := lock(ctx, s.tx, lockMembers(productID)); err != nil {
			return err
		}
		oldRole, otherOwner, err := s.memberRole(ctx, productID, targetUserID)
		if err != nil {
			return err
		}
		if oldRole == model.RoleOwner && role != model.RoleOwner && !otherOwner {
			return ErrLastOwner
		}
		if err := s.productRoles.Update(ctx, &model.ProductRole{
			UserID: targetUserID, ProductID: productID, Role: role,
		}); err != nil {
			return err
		}
		return s.auditMember(ctx, user, productID, targetUserID, oldRole, role)
	})
}

// RemoveMember removes a user from a product; see UpdateMember for the lock.
func (s *ProductService) RemoveMember(ctx context.Context, user *model.User, productID, targetUserID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := lock(ctx, s.tx, lockMembers(productID)); err != nil {
			return err
		}
		oldRole, otherOwner, err := s.memberRole(ctx, productID, targetUserID)
		if err != nil {
			return err
		}
		if oldRole == model.RoleOwner && !otherOwner {
			return ErrLastOwner
		}
		if err := s.productRoles.Delete(ctx, targetUserID, productID); err != nil {
			return err
		}
		return s.auditMember(ctx, user, productID, targetUserID, oldRole, "")
	})
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

	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.products.Update(ctx, product); err != nil {
			return fmt.Errorf("split statuses: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "product", EntityID: product.ID,
			UserID: user.ID, Action: "split_statuses", CreatedAt: now,
		})
	}); err != nil {
		return err
	}

	return nil
}

// TransferOwnership transfers product ownership to another user (FR-306):
// the new owner is promoted (or added) and the current owner becomes a
// developer, in one transaction, so a failure can never leave the product
// without an owner.
func (s *ProductService) TransferOwnership(ctx context.Context, user *model.User, productID, newOwnerID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if newOwnerID == "" || newOwnerID == user.ID {
		return errors.New("choose another user to transfer the ownership to")
	}

	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := lock(ctx, s.tx, lockMembers(productID)); err != nil {
			return err
		}
		// Promote first: should anything below fail, the transaction rolls
		// back, and even without one the product keeps an owner.
		existing, err := s.productRoles.GetByUserAndProduct(ctx, newOwnerID, productID)
		if err != nil {
			if err := s.productRoles.Create(ctx, &model.ProductRole{
				ID: uuid.New().String(), UserID: newOwnerID, ProductID: productID,
				Role: model.RoleOwner, CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}); err != nil {
				return fmt.Errorf("make new owner: %w", err)
			}
			if err := s.auditMember(ctx, user, productID, newOwnerID, "", model.RoleOwner); err != nil {
				return err
			}
		} else if existing.Role != model.RoleOwner {
			oldRole := existing.Role
			existing.Role = model.RoleOwner
			if err := s.productRoles.Update(ctx, existing); err != nil {
				return fmt.Errorf("make new owner: %w", err)
			}
			if err := s.auditMember(ctx, user, productID, newOwnerID, oldRole, model.RoleOwner); err != nil {
				return err
			}
		}

		if err := s.productRoles.Update(ctx, &model.ProductRole{
			UserID: user.ID, ProductID: productID, Role: model.RoleDeveloper,
		}); err != nil {
			return fmt.Errorf("demote current owner: %w", err)
		}
		return s.auditMember(ctx, user, productID, user.ID, model.RoleOwner, model.RoleDeveloper)
	})
}
