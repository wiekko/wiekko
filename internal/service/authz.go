package service

import (
	"context"
	"fmt"

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
// Admins bypass the check (SR-202). Returns the user's effective role.
func (a *Authorizer) RequireRole(ctx context.Context, user *model.User, productID, minRole string) (string, error) {
	if user.IsAdmin {
		return model.RoleOwner, nil
	}

	role, err := a.productRoles.GetByUserAndProduct(ctx, user.ID, productID)
	if err != nil {
		return "", fmt.Errorf("%w: no access to product", ErrForbidden)
	}

	if roleRank(role.Role) < roleRank(minRole) {
		return "", fmt.Errorf("%w: requires %s role", ErrForbidden, minRole)
	}

	return role.Role, nil
}

// roleRank returns a numeric rank for role comparison.
// Higher rank means more permissions.
func roleRank(role string) int {
	switch role {
	case model.RoleViewer:
		return 0
	case model.RoleTester:
		return 1
	case model.RoleDeveloper:
		return 2
	case model.RoleOwner:
		return 3
	default:
		return -1
	}
}
