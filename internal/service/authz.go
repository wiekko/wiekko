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

// ErrTwoFactorRequired is returned when a product requires two-factor
// authentication of its members and the user has not set it up (SC-107). It
// wraps ErrForbidden.
var ErrTwoFactorRequired = fmt.Errorf("%w: this product requires two-factor authentication; set it up on your profile to access it", ErrForbidden)

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = fmt.Errorf("not found")

// Authorizer checks product-level role-based access.
type Authorizer struct {
	productRoles store.ProductRoleStore
	// products is consulted for the product's 2FA requirement; nil skips
	// that check (unit tests that do not care about it).
	products store.ProductStore
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

	if err := a.checkTwoFactor(ctx, user, productID); err != nil {
		return "", err
	}

	return role, nil
}

// checkTwoFactor keeps session users without 2FA out of products that require
// it (SC-107), administrators included. Token-authenticated requests are
// exempt: a token is a separate credential, as with the instance-level rules.
func (a *Authorizer) checkTwoFactor(ctx context.Context, user *model.User, productID string) error {
	if a.products == nil || user.TOTPEnabled || access.ViaToken(ctx) {
		return nil
	}
	p, err := a.products.GetByID(ctx, productID)
	if err != nil {
		// Fail closed: without the product there is nothing to grant anyway.
		return fmt.Errorf("check product 2FA requirement: %w", err)
	}
	if p.TwoFactorRequired {
		return ErrTwoFactorRequired
	}
	return nil
}

// roleRank returns a numeric rank for role comparison (see model.RoleRank).
// Higher rank means more permissions.
func roleRank(role string) int {
	return model.RoleRank(role)
}
