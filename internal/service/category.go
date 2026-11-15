package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// prefixPattern constrains category prefixes to what reads well in an identifier
// such as "FR-12": one to six upper-case alphanumerics starting with a letter.
// Hyphens are excluded so the identifier stays unambiguous, and a leading digit
// is excluded so the prefix cannot be mistaken for the sequence number.
var prefixPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,5}$`)

// CategoryService handles category CRUD.
type CategoryService struct {
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx         store.Transactor
	categories store.CategoryStore
	// products holds the defect prefix, which category prefixes must not
	// take (FR-043). nil skips that check.
	products     store.ProductStore
	requirements store.RequirementStore
	constraints  store.ConstraintStore
	audit        store.AuditStore
	authz        *Authorizer
	owns         *ownership
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
	if input.EntityType != model.RequirementKindRequirement && input.EntityType != model.RequirementKindTask &&
		input.EntityType != "constraint" {
		return nil, errors.New("entity_type must be 'requirement', 'task' or 'constraint'")
	}

	prefix, err := normalizePrefix(input.Prefix)
	if err != nil {
		return nil, err
	}
	if err := s.checkNameFree(ctx, input.ProductID, input.EntityType, input.Name, ""); err != nil {
		return nil, err
	}
	if err := s.checkPrefixFree(ctx, input.ProductID, prefix, ""); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	cat := &model.Category{
		ID:         uuid.New().String(),
		ProductID:  input.ProductID,
		EntityType: input.EntityType,
		Name:       input.Name,
		Prefix:     prefix,
		Position:   input.Position,
		CreatedAt:  now,
	}
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.categories.Create(ctx, cat); err != nil {
			return fmt.Errorf("create category: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), ProductID: cat.ProductID, EntityType: "category", EntityID: cat.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}

	return cat, nil
}

// GetCategory returns a category by ID. Requires viewer role.
func (s *CategoryService) GetCategory(ctx context.Context, user *model.User, productID, id string) (*model.Category, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindCategory, id}); err != nil {
		return nil, err
	}
	return s.categories.GetByID(ctx, id)
}

// UpdateCategory modifies a category. Requires owner role.
//
// Changing the prefix is allowed even when items already reference the category.
// Identifiers are rendered from the current prefix (model.EntityIdentifier), so a
// rename relabels every existing item; nothing parses identifiers back, so this is
// a traceability decision for the owner rather than something to block here. The
// UI warns and asks for confirmation before sending the change.
func (s *CategoryService) UpdateCategory(ctx context.Context, user *model.User, productID, id string, input UpdateCategoryInput) (*model.Category, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindCategory, id}); err != nil {
		return nil, err
	}

	cat, err := s.categories.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Name != nil {
		if *input.Name == "" {
			return nil, errors.New("category name is required")
		}
		if err := s.checkNameFree(ctx, cat.ProductID, cat.EntityType, *input.Name, cat.ID); err != nil {
			return nil, err
		}
		cat.Name = *input.Name
	}
	if input.Prefix != nil {
		prefix, err := normalizePrefix(*input.Prefix)
		if err != nil {
			return nil, err
		}
		if err := s.checkPrefixFree(ctx, cat.ProductID, prefix, cat.ID); err != nil {
			return nil, err
		}
		cat.Prefix = prefix
	}
	if input.Position != nil {
		cat.Position = *input.Position
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.categories.Update(ctx, cat); err != nil {
			return fmt.Errorf("update category: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), ProductID: cat.ProductID, EntityType: "category", EntityID: cat.ID,
			UserID: user.ID, Action: "update", CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}

	return cat, nil
}

// DeleteCategory removes a category. Requires owner role.
//
// Refuses when items still reference the category, and when it is the last one of
// its entity type — BacklogService.resolveCategory needs at least one category per
// kind to file new items under.
func (s *CategoryService) DeleteCategory(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindCategory, id}); err != nil {
		return err
	}

	// The checks and the delete run under the product's category lock, so
	// two concurrent deletes cannot both see the other category and leave a
	// kind with none.
	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := lock(ctx, s.tx, lockCategories(productID)); err != nil {
			return err
		}
		cat, err := s.categories.GetByID(ctx, id)
		if err != nil {
			return err
		}

		count, err := s.usageCount(ctx, cat)
		if err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("cannot delete %q: %s still assigned to it", cat.Name, pluralItems(count))
		}

		siblings, err := s.categories.ListByProductID(ctx, cat.ProductID, cat.EntityType)
		if err != nil {
			return fmt.Errorf("list categories: %w", err)
		}
		if len(siblings) <= 1 {
			return fmt.Errorf("cannot delete the last %s category: at least one is needed to create new items", cat.EntityType)
		}

		if err := s.categories.Delete(ctx, id); err != nil {
			// Backstop for an item filed under the category meanwhile.
			if strings.Contains(err.Error(), "FOREIGN KEY") || strings.Contains(err.Error(), "foreign key") ||
				strings.Contains(err.Error(), "RESTRICT") {
				return errors.New("cannot delete category: items are still assigned to it")
			}
			return fmt.Errorf("delete category: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), ProductID: productID, EntityType: "category", EntityID: id,
			UserID: user.ID, Action: "delete", CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
	})
}

// ListCategories returns categories for a product and entity type. Requires viewer role.
func (s *CategoryService) ListCategories(ctx context.Context, user *model.User, productID, entityType string) ([]*model.Category, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.categories.ListByProductID(ctx, productID, entityType)
}

// UsageCount returns how many items reference the category. Requires viewer role.
func (s *CategoryService) UsageCount(ctx context.Context, user *model.User, productID, id string) (int, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return 0, err
	}
	if err := s.owns.require(ctx, productID, ref{kindCategory, id}); err != nil {
		return 0, err
	}
	cat, err := s.categories.GetByID(ctx, id)
	if err != nil {
		return 0, err
	}
	return s.usageCount(ctx, cat)
}

// UsageCounts returns item counts keyed by category ID for one entity type.
// Requires viewer role.
func (s *CategoryService) UsageCounts(ctx context.Context, user *model.User, productID, entityType string) (map[string]int, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}

	cats, err := s.categories.ListByProductID(ctx, productID, entityType)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}

	counts := make(map[string]int, len(cats))
	for _, cat := range cats {
		count, err := s.usageCount(ctx, cat)
		if err != nil {
			return nil, err
		}
		counts[cat.ID] = count
	}
	return counts, nil
}

// usageCount counts referencing items without an authorization check; callers
// have already established access.
func (s *CategoryService) usageCount(ctx context.Context, cat *model.Category) (int, error) {
	var (
		count int
		err   error
	)
	if cat.EntityType == "constraint" {
		count, err = s.constraints.CountByCategoryID(ctx, cat.ID)
	} else {
		count, err = s.requirements.CountByCategoryID(ctx, cat.ID)
	}
	if err != nil {
		return 0, fmt.Errorf("count items in category: %w", err)
	}
	return count, nil
}

// checkNameFree rejects a name already taken by another category of the same
// entity type. Names may repeat across types ("Architecture" is both a
// requirement and a constraint category by default).
func (s *CategoryService) checkNameFree(ctx context.Context, productID, entityType, name, selfID string) error {
	existing, err := s.categories.GetByProductAndName(ctx, productID, entityType, name)
	if err != nil || existing == nil || existing.ID == selfID {
		return nil
	}
	return fmt.Errorf("a %s category named %q already exists", entityType, name)
}

// checkPrefixFree rejects a prefix already taken anywhere in the product. The
// scope is product-wide rather than per entity type because constraints keep
// their own sequence counter, so a shared prefix would render two different items
// as the same identifier.
func (s *CategoryService) checkPrefixFree(ctx context.Context, productID, prefix, selfID string) error {
	if s.products != nil {
		if p, err := s.products.GetByID(ctx, productID); err == nil {
			if p.DefectPrefix == prefix {
				return fmt.Errorf("prefix %q is already used for defects", prefix)
			}
			if p.DocumentPrefix == prefix {
				return fmt.Errorf("prefix %q is already used for documents", prefix)
			}
		}
	}
	existing, err := s.categories.GetByProductAndPrefix(ctx, productID, prefix)
	if err != nil || existing == nil || existing.ID == selfID {
		return nil
	}
	return fmt.Errorf("prefix %q is already used by the %s category %q", prefix, existing.EntityType, existing.Name)
}

// normalizeDefectPrefix validates a defect identifier prefix by the same
// rules as category prefixes.
func normalizeDefectPrefix(prefix string) (string, error) {
	return normalizeEntityPrefix(prefix, "defect", model.DefaultDefectPrefix)
}

// normalizeDocumentPrefix does the same for document identifiers (FR-050).
func normalizeDocumentPrefix(prefix string) (string, error) {
	return normalizeEntityPrefix(prefix, "document", model.DefaultDocumentPrefix)
}

func normalizeEntityPrefix(prefix, what, example string) (string, error) {
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	if !prefixPattern.MatchString(prefix) {
		return "", fmt.Errorf("%s prefix must be 1-6 letters or digits, starting with a letter (e.g. %s)", what, example)
	}
	return prefix, nil
}

func normalizePrefix(prefix string) (string, error) {
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	if prefix == "" {
		return "", errors.New("category prefix is required")
	}
	if !prefixPattern.MatchString(prefix) {
		return "", errors.New("category prefix must be 1-6 letters or digits, starting with a letter (e.g. FR)")
	}
	return prefix, nil
}

func pluralItems(count int) string {
	if count == 1 {
		return "1 item is"
	}
	return fmt.Sprintf("%d items are", count)
}
