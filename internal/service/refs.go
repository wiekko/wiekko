package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// refPattern matches a human-readable identifier such as "FR-12" or "mt-3":
// a category prefix (see normalizePrefix) and a sequence number.
var refPattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]{0,5})-([1-9][0-9]{0,9})$`)

// parseRef splits an identifier into its upper-cased prefix and sequence
// number. ok is false for anything else (in particular for UUIDs).
func parseRef(ref string) (prefix string, seq int, ok bool) {
	m := refPattern.FindStringSubmatch(strings.TrimSpace(ref))
	if m == nil {
		return "", 0, false
	}
	n, err := strconv.Atoi(m[2])
	if err != nil {
		return "", 0, false
	}
	return strings.ToUpper(m[1]), n, true
}

// categoryPrefixes maps category ID to prefix for the given entity types of a
// product.
func categoryPrefixes(ctx context.Context, categories store.CategoryStore, productID string, entityTypes ...string) (map[string]string, error) {
	m := make(map[string]string)
	for _, et := range entityTypes {
		cats, err := categories.ListByProductID(ctx, productID, et)
		if err != nil {
			return nil, fmt.Errorf("list %s categories: %w", et, err)
		}
		for _, c := range cats {
			m[c.ID] = c.Prefix
		}
	}
	return m, nil
}

// categoryForRef looks up the category a prefix belongs to and checks that it
// is one of the wanted entity types. Any mismatch is reported as not found.
func categoryForRef(ctx context.Context, categories store.CategoryStore, productID, prefix string, entityTypes ...string) (*model.Category, error) {
	cat, err := categories.GetByProductAndPrefix(ctx, productID, prefix)
	if err != nil {
		return nil, ErrNotFound
	}
	for _, et := range entityTypes {
		if cat.EntityType == et {
			return cat, nil
		}
	}
	return nil, ErrNotFound
}

// CategoryPrefixes maps the product's requirement and task category IDs to
// their prefixes, for rendering identifiers. Requires viewer role.
func (s *BacklogService) CategoryPrefixes(ctx context.Context, user *model.User, productID string) (map[string]string, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return categoryPrefixes(ctx, s.categories, productID, model.RequirementKindRequirement, model.RequirementKindTask)
}

// ResolveRequirementRef turns a requirement or task reference into its ID. A
// reference is either the ID itself, returned unchanged, or a human-readable
// identifier such as "FR-12". An identifier resolves only within the product
// and only when its prefix matches the item's category, so "MT-12" does not
// alias "FR-12". Requires viewer role.
func (s *BacklogService) ResolveRequirementRef(ctx context.Context, user *model.User, productID, ref string) (string, error) {
	prefix, seq, ok := parseRef(ref)
	if !ok {
		return ref, nil
	}
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return "", err
	}
	cat, err := categoryForRef(ctx, s.categories, productID, prefix, model.RequirementKindRequirement, model.RequirementKindTask)
	if err != nil {
		return "", fmt.Errorf("%w: no requirement or task %s", ErrNotFound, ref)
	}
	r, err := s.requirements.GetBySequence(ctx, productID, seq)
	if err != nil || r.CategoryID != cat.ID {
		return "", fmt.Errorf("%w: no requirement or task %s", ErrNotFound, ref)
	}
	return r.ID, nil
}

// CategoryPrefixes maps the product's constraint category IDs to their
// prefixes. Requires viewer role.
func (s *ConstraintService) CategoryPrefixes(ctx context.Context, user *model.User, productID string) (map[string]string, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return categoryPrefixes(ctx, s.categories, productID, "constraint")
}

// ResolveConstraintRef turns a constraint reference (ID or identifier such as
// "AC-42") into its ID, like ResolveRequirementRef. Requires viewer role.
func (s *ConstraintService) ResolveConstraintRef(ctx context.Context, user *model.User, productID, ref string) (string, error) {
	prefix, seq, ok := parseRef(ref)
	if !ok {
		return ref, nil
	}
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return "", err
	}
	cat, err := categoryForRef(ctx, s.categories, productID, prefix, "constraint")
	if err != nil {
		return "", fmt.Errorf("%w: no constraint %s", ErrNotFound, ref)
	}
	c, err := s.constraints.GetBySequence(ctx, productID, seq)
	if err != nil || c.CategoryID != cat.ID {
		return "", fmt.Errorf("%w: no constraint %s", ErrNotFound, ref)
	}
	return c.ID, nil
}
