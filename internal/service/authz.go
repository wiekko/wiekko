package service

import (
	"context"
	"fmt"

	"nexmap.net/internal/access"
	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// ErrForbidden is returned when a user lacks sufficient permissions.
var ErrForbidden = fmt.Errorf("forbidden")

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = fmt.Errorf("not found")

// Authorizer checks product-level role-based access.
type Authorizer struct {
	productRoles store.ProductRoleStore
}

// RequireRole checks that the user has at least minRole on the product.
// Admins bypass the role lookup (SC-202). Returns the user's effective role.
//
// For token-authenticated requests the token's restriction applies on top
// (SC-402, SC-403): a product outside the token's set is not reachable, and
// the effective role is capped, for administrators as well.
func (a *Authorizer) RequireRole(ctx context.Context, user *model.User, productID, minRole string) (string, error) {
	restriction := access.FromContext(ctx)
	if !restriction.AllowsProduct(productID) {
		return "", fmt.Errorf("%w: no access to product", ErrForbidden)
	}

	role := model.RoleOwner
	if !user.IsAdmin {
		pr, err := a.productRoles.GetByUserAndProduct(ctx, user.ID, productID)
		if err != nil {
			return "", fmt.Errorf("%w: no access to product", ErrForbidden)
		}
		role = pr.Role
	}
	role = restriction.CapRole(role)

	if roleRank(role) < roleRank(minRole) {
		return "", fmt.Errorf("%w: requires %s role", ErrForbidden, minRole)
	}

	return role, nil
}

// roleRank returns a numeric rank for role comparison (see model.RoleRank).
// Higher rank means more permissions.
func roleRank(role string) int {
	return model.RoleRank(role)
}
