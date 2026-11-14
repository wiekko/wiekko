package service

import (
	"context"
	"errors"
	"slices"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

// policyService returns a TwoFactorService whose settings and product roles
// come from the given maps; *fail makes every read fail.
func policyService(settings map[string]string, roles map[string][]string, fail *bool) *TwoFactorService {
	return &TwoFactorService{
		settings: &testutil.MockSettingStore{
			GetAllFunc: func(context.Context) (map[string]string, error) {
				if *fail {
					return nil, errors.New("database is unavailable")
				}
				return settings, nil
			},
		},
		productRoles: &testutil.MockProductRoleStore{
			ListByUserIDFunc: func(_ context.Context, userID string) ([]*model.ProductRole, error) {
				if *fail {
					return nil, errors.New("database is unavailable")
				}
				var out []*model.ProductRole
				for _, r := range roles[userID] {
					out = append(out, &model.ProductRole{UserID: userID, ProductID: "p-" + r, Role: r})
				}
				return out, nil
			},
		},
	}
}

// TestRequiredForKeepsLastKnownValue checks that a failing read of the
// settings neither switches enforcement off nor on.
func TestRequiredForKeepsLastKnownValue(t *testing.T) {
	settings := map[string]string{SettingTwoFactorEnforced: "false"}
	fail := true
	svc := policyService(settings, nil, &fail)
	user := &model.User{ID: "u1"}
	ctx := context.Background()

	if svc.RequiredFor(ctx, user) {
		t.Error("enforced before the setting was ever read")
	}

	fail, settings[SettingTwoFactorEnforced] = false, "true"
	if !svc.RequiredFor(ctx, user) {
		t.Fatal("not enforced although the setting says so")
	}
	fail = true
	if !svc.RequiredFor(ctx, user) {
		t.Error("a failed read switched enforcement off")
	}

	fail, settings[SettingTwoFactorEnforced] = false, "false"
	if svc.RequiredFor(ctx, user) {
		t.Fatal("enforced although the setting was turned off")
	}
	fail = true
	if svc.RequiredFor(ctx, user) {
		t.Error("a failed read switched enforcement on")
	}
}

// TestRequiredForKeepsLastKnownRoles checks that a role-based rule keeps
// applying while the user's roles cannot be read.
func TestRequiredForKeepsLastKnownRoles(t *testing.T) {
	settings := map[string]string{SettingTwoFactorEnforcedRole(model.RoleOwner): "true"}
	roles := map[string][]string{"u1": {model.RoleOwner}}
	fail := false
	svc := policyService(settings, roles, &fail)
	user := &model.User{ID: "u1"}
	ctx := context.Background()

	if !svc.RequiredFor(ctx, user) {
		t.Fatal("owner rule does not apply to an owner")
	}
	fail = true
	if !svc.RequiredFor(ctx, user) {
		t.Error("a failed read switched the role rule off")
	}
}

func TestInstanceRules(t *testing.T) {
	admin := &model.User{ID: "admin", IsAdmin: true}
	owner := &model.User{ID: "owner"}
	dev := &model.User{ID: "dev"}
	nobody := &model.User{ID: "nobody"}
	roles := map[string][]string{
		"admin": {},
		"owner": {model.RoleOwner, model.RoleViewer},
		"dev":   {model.RoleDeveloper},
	}

	tests := []struct {
		name     string
		settings map[string]string
		user     *model.User
		want     []string
	}{
		{"nothing enforced", map[string]string{}, admin, nil},
		{"everyone", map[string]string{SettingTwoFactorEnforced: "true", SettingTwoFactorEnforcedAdmins: "true"}, nobody, []string{"all users"}},
		{"admins: admin", map[string]string{SettingTwoFactorEnforcedAdmins: "true"}, admin, []string{"administrators"}},
		{"admins: owner", map[string]string{SettingTwoFactorEnforcedAdmins: "true"}, owner, nil},
		{"owners: owner", map[string]string{"2fa_enforced_owner": "true"}, owner, []string{"product owners"}},
		{"owners: developer", map[string]string{"2fa_enforced_owner": "true"}, dev, nil},
		{"owners: admin without membership", map[string]string{"2fa_enforced_owner": "true"}, admin, nil},
		{"viewers: owner who also views", map[string]string{"2fa_enforced_viewer": "true"}, owner, []string{"viewers"}},
		{"admins and owners: owner", map[string]string{SettingTwoFactorEnforcedAdmins: "true", "2fa_enforced_owner": "true"}, owner, []string{"product owners"}},
		{"developers and testers: dev", map[string]string{"2fa_enforced_developer": "true", "2fa_enforced_tester": "true"}, dev, []string{"developers"}},
		{"roles: user without products", map[string]string{"2fa_enforced_viewer": "true", "2fa_enforced_owner": "true"}, nobody, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fail := false
			svc := policyService(tc.settings, roles, &fail)
			got := svc.instanceRules(context.Background(), tc.user)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("rules = %q, want %q", got, tc.want)
			}
			if svc.RequiredFor(context.Background(), tc.user) != (len(tc.want) > 0) {
				t.Fatal("RequiredFor disagrees with the rules")
			}
		})
	}
}

func TestTwoFactorRequirementString(t *testing.T) {
	alpha := &model.Product{Name: "Alpha"}
	beta := &model.Product{Name: "Beta"}
	tests := []struct {
		req  TwoFactorRequirement
		want string
	}{
		{TwoFactorRequirement{}, ""},
		{TwoFactorRequirement{Instance: []string{"all users"}}, "required for all users"},
		{TwoFactorRequirement{Instance: []string{"administrators", "product owners"}}, "required for administrators and product owners"},
		{TwoFactorRequirement{Products: []*model.Product{alpha}}, "required by the product Alpha"},
		{TwoFactorRequirement{Products: []*model.Product{alpha, beta}}, "required by the products Alpha and Beta"},
		{TwoFactorRequirement{Instance: []string{"developers"}, Products: []*model.Product{alpha}}, "required for developers and by the product Alpha"},
	}
	for _, tc := range tests {
		if got := tc.req.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
		if tc.req.Required() != (tc.want != "") {
			t.Errorf("Required() = %v for %q", tc.req.Required(), tc.want)
		}
	}
}

func TestLockedProduct(t *testing.T) {
	products := map[string]*model.Product{
		"strict": {ID: "strict", Name: "Strict", TwoFactorRequired: true},
		"open":   {ID: "open", Name: "Open"},
	}
	svc := &TwoFactorService{
		products: &testutil.MockProductStore{
			GetByIDFunc: func(_ context.Context, id string) (*model.Product, error) {
				if p, ok := products[id]; ok {
					return p, nil
				}
				return nil, errors.New("product not found")
			},
		},
		productRoles: &testutil.MockProductRoleStore{
			GetByUserAndProductFunc: func(_ context.Context, userID, productID string) (*model.ProductRole, error) {
				if userID == "member" {
					return &model.ProductRole{UserID: userID, ProductID: productID, Role: model.RoleViewer}, nil
				}
				return nil, errors.New("product role not found")
			},
		},
	}
	ctx := context.Background()
	member := &model.User{ID: "member"}
	enrolled := &model.User{ID: "member", TOTPEnabled: true}
	outsider := &model.User{ID: "outsider"}
	admin := &model.User{ID: "admin", IsAdmin: true}

	cases := []struct {
		name    string
		user    *model.User
		product string
		want    bool
	}{
		{"member without 2FA", member, "strict", true},
		{"member with 2FA", enrolled, "strict", false},
		{"product without requirement", member, "open", false},
		{"non-member learns nothing", outsider, "strict", false},
		{"admin without 2FA", admin, "strict", true},
		{"unknown product", member, "missing", false},
	}
	for _, tc := range cases {
		if got := svc.ProductLocked(ctx, tc.user, tc.product); got != tc.want {
			t.Errorf("%s: ProductLocked = %v, want %v", tc.name, got, tc.want)
		}
	}
}
