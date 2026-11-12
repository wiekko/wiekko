package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/access"
	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// PATService handles personal access token operations.
type PATService struct {
	pats         store.PATStore
	productRoles store.ProductRoleStore
	settings     store.SettingStore
}

// SettingPATMaxLifetimeDays is the instance setting limiting how long a token
// may live, in days; "0" or empty means no limit (SC-409).
const SettingPATMaxLifetimeDays = "pat_max_lifetime_days"

// CreatePATInput describes a token to create. The zero restriction fields
// mean: full access to all product data, no role cap, all products.
type CreatePATInput struct {
	Name      string
	ExpiresAt string // RFC 3339; empty for no expiry

	// Grants are the scopes the token receives; nil means access.FullAccess.
	// The account and admin scopes are only granted when listed here.
	Grants             access.Grants
	RoleCap            string   // "" for no cap
	ProductsRestricted bool     // restrict the token to ProductIDs
	ProductIDs         []string // products the token reaches, if restricted
}

// CreatePATResult contains the data returned after creating a PAT.
// The RawToken is only available at creation time.
type CreatePATResult struct {
	RawToken string
	PAT      *model.PAT
}

// Create generates a new personal access token for user.
// The raw token is returned exactly once; only its hash is stored.
//
// The token never receives more than the user holds: products must be ones
// the user is a member of, and the admin scope needs an administrator. A
// request made with a token can only create tokens within that token's own
// restriction (SC-406).
func (s *PATService) Create(ctx context.Context, user *model.User, in CreatePATInput) (*CreatePATResult, error) {
	if in.Name == "" {
		return nil, errors.New("token name is required")
	}
	grants := in.Grants
	if grants == nil {
		grants = access.FullAccess()
	}
	if grants[access.Admin] > access.None && !user.IsAdmin {
		return nil, fmt.Errorf("%w: only administrators can grant the admin scope", ErrForbidden)
	}
	if in.RoleCap != "" && model.RoleRank(in.RoleCap) < 0 {
		return nil, fmt.Errorf("invalid role cap %q", in.RoleCap)
	}
	products, err := s.checkProducts(ctx, user, in)
	if err != nil {
		return nil, err
	}
	if err := s.checkExpiry(ctx, in.ExpiresAt); err != nil {
		return nil, err
	}

	restriction := &access.Restriction{Grants: grants, RoleCap: in.RoleCap,
		ProductsRestricted: in.ProductsRestricted, Products: products}
	if !restriction.SubsetOf(access.FromContext(ctx)) {
		return nil, fmt.Errorf("%w: a token can only create tokens with fewer rights than its own", ErrForbidden)
	}

	rawToken, err := generateToken(patTokenBytes)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	rawToken = patPrefix + rawToken

	now := time.Now().UTC().Format(time.RFC3339)
	pat := &model.PAT{
		ID:                 uuid.New().String(),
		UserID:             user.ID,
		Name:               in.Name,
		TokenHash:          hashPATToken(rawToken),
		TokenPrefix:        rawToken[:patPrefixLen],
		CreatedAt:          now,
		ExpiresAt:          in.ExpiresAt,
		Scopes:             grants.String(),
		RoleCap:            in.RoleCap,
		ProductsRestricted: in.ProductsRestricted,
		ProductIDs:         products,
	}

	if err := s.pats.Create(ctx, pat); err != nil {
		return nil, fmt.Errorf("store token: %w", err)
	}
	return &CreatePATResult{RawToken: rawToken, PAT: pat}, nil
}

// checkProducts validates the products a token is restricted to and returns
// them without duplicates.
func (s *PATService) checkProducts(ctx context.Context, user *model.User, in CreatePATInput) ([]string, error) {
	if !in.ProductsRestricted {
		if len(in.ProductIDs) > 0 {
			return nil, errors.New("products given for a token that is not restricted to products")
		}
		return nil, nil
	}
	if len(in.ProductIDs) == 0 {
		return nil, errors.New("a token restricted to products needs at least one product")
	}
	out := make([]string, 0, len(in.ProductIDs))
	for _, id := range in.ProductIDs {
		if slices.Contains(out, id) {
			continue
		}
		if !user.IsAdmin {
			if _, err := s.productRoles.GetByUserAndProduct(ctx, user.ID, id); err != nil {
				return nil, fmt.Errorf("%w: not a member of product %s", ErrForbidden, id)
			}
		}
		out = append(out, id)
	}
	return out, nil
}

// checkExpiry validates the expiry against the instance's maximum token
// lifetime (SC-409).
func (s *PATService) checkExpiry(ctx context.Context, expiresAt string) error {
	now := time.Now().UTC()
	var exp time.Time
	if expiresAt != "" {
		t, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return fmt.Errorf("invalid expiry %q: want RFC 3339", expiresAt)
		}
		if !t.After(now) {
			return errors.New("expiry must lie in the future")
		}
		exp = t
	}
	days := s.maxLifetimeDays(ctx)
	if days == 0 {
		return nil
	}
	if expiresAt == "" {
		return fmt.Errorf("tokens must expire within %d days on this instance", days)
	}
	if exp.After(now.AddDate(0, 0, days)) {
		return fmt.Errorf("tokens must expire within %d days on this instance", days)
	}
	return nil
}

// MaxLifetimeDays returns the instance's maximum token lifetime in days, or
// 0 for no limit.
func (s *PATService) MaxLifetimeDays(ctx context.Context) int { return s.maxLifetimeDays(ctx) }

func (s *PATService) maxLifetimeDays(ctx context.Context) int {
	if s.settings == nil {
		return 0
	}
	v, err := s.settings.Get(ctx, SettingPATMaxLifetimeDays)
	if err != nil || v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// List returns all PATs for a user.
func (s *PATService) List(ctx context.Context, userID string) ([]*model.PAT, error) {
	return s.pats.ListByUserID(ctx, userID)
}

// Revoke marks a PAT as revoked. Only the token's owner may revoke it.
func (s *PATService) Revoke(ctx context.Context, userID, patID string) error {
	pats, err := s.pats.ListByUserID(ctx, userID)
	if err != nil {
		return err
	}
	for _, p := range pats {
		if p.ID == patID {
			if p.RevokedAt != "" {
				return errors.New("token already revoked")
			}
			now := time.Now().UTC().Format(time.RFC3339)
			return s.pats.Revoke(ctx, patID, now)
		}
	}
	return errors.New("token not found")
}
