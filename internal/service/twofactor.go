// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wiekko.net/internal/auth"
	"wiekko.net/internal/auth/totp"
	"wiekko.net/internal/model"
	"wiekko.net/internal/store"
)

// Instance settings that make TOTP mandatory (SC-34). SettingTwoFactorEnforced
// covers everyone; the others cover administrators and users who hold a given
// role in at least one product (see SettingTwoFactorEnforcedRole).
const (
	SettingTwoFactorEnforced       = "2fa_enforced"
	SettingTwoFactorEnforcedAdmins = "2fa_enforced_admins"
)

// TwoFactorRoles lists the product roles that can be covered by an instance
// setting, in display order.
var TwoFactorRoles = []string{model.RoleOwner, model.RoleDeveloper, model.RoleTester, model.RoleViewer}

// SettingTwoFactorEnforcedRole is the instance setting that makes TOTP
// mandatory for users holding role in at least one product.
func SettingTwoFactorEnforcedRole(role string) string { return "2fa_enforced_" + role }

// twoFactorPolicy is the instance-level enforcement as configured.
type twoFactorPolicy struct {
	everyone bool
	admins   bool
	roles    map[string]bool
}

func (p *twoFactorPolicy) anyRole() bool {
	for _, on := range p.roles {
		if on {
			return true
		}
	}
	return false
}

// TwoFactorRoleGroup names the users a role setting covers, e.g. "product
// owners".
func TwoFactorRoleGroup(role string) string {
	switch role {
	case model.RoleOwner:
		return "product owners"
	case model.RoleDeveloper:
		return "developers"
	case model.RoleTester:
		return "testers"
	case model.RoleViewer:
		return "viewers"
	}
	return role + "s"
}

// TwoFactorRequirement says whether and why a user must use two-factor
// authentication.
type TwoFactorRequirement struct {
	// Instance lists the instance-level rules that cover the user, e.g.
	// "all users", "administrators", "product owners". While it is not empty
	// the user must set up 2FA before using Wiekko at all.
	Instance []string
	// Products are the products the user is a member of that require 2FA.
	// Without 2FA the user cannot reach them.
	Products []*model.Product
}

// Required reports whether any rule requires 2FA of the user.
func (r TwoFactorRequirement) Required() bool {
	return len(r.Instance) > 0 || len(r.Products) > 0
}

// String describes the requirement for messages, e.g. "required for
// administrators and by the product Alpha".
func (r TwoFactorRequirement) String() string {
	var parts []string
	if len(r.Instance) > 0 {
		parts = append(parts, "required for "+joinWords(r.Instance))
	}
	if len(r.Products) > 0 {
		names := make([]string, len(r.Products))
		for i, p := range r.Products {
			names[i] = p.Name
		}
		noun := "the product "
		if len(names) > 1 {
			noun = "the products "
		}
		prefix := "required by "
		if len(parts) > 0 {
			prefix = "by "
		}
		parts = append(parts, prefix+noun+joinWords(names))
	}
	return strings.Join(parts, " and ")
}

// joinWords joins "a", "b", "c" as "a, b and c".
func joinWords(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

// totpIssuer is the issuer name authenticator apps display next to the account.
const totpIssuer = "Wiekko"

var (
	// ErrInvalidTOTPCode is returned when a TOTP or recovery code does not match
	// (or was already used).
	ErrInvalidTOTPCode = errors.New("invalid authentication code")
	// ErrTOTPCodeReused is returned for a correct TOTP code whose time step was
	// already used, e.g. the code just entered at login. It wraps
	// ErrInvalidTOTPCode, so callers that don't care about the reason need not
	// distinguish it.
	ErrTOTPCodeReused = fmt.Errorf("%w: code already used", ErrInvalidTOTPCode)
	// ErrTOTPAlreadyEnabled is returned when setting up TOTP twice.
	ErrTOTPAlreadyEnabled = errors.New("two-factor authentication is already enabled")
	// ErrTOTPNotEnabled is returned for operations that need TOTP to be active.
	ErrTOTPNotEnabled = errors.New("two-factor authentication is not enabled")
	// ErrTOTPNoPendingSetup is returned when confirming without starting setup first.
	ErrTOTPNoPendingSetup = errors.New("no two-factor setup in progress; start again")
	// ErrTOTPEnforced is returned when a user tries to disable TOTP while the
	// instance requires it.
	ErrTOTPEnforced = errors.New("two-factor authentication is required for your account and cannot be disabled")
	// ErrIncorrectPassword is returned when a re-entered password does not match.
	ErrIncorrectPassword = errors.New("current password is incorrect")
)

// TwoFactorService manages TOTP enrollment, verification and recovery codes.
type TwoFactorService struct {
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx           store.Transactor
	users        store.UserStore
	twoFactor    store.TwoFactorStore
	settings     store.SettingStore
	productRoles store.ProductRoleStore
	products     store.ProductStore
	provider     auth.Provider
	now          func() time.Time

	// limiter blocks a user's second factor after repeated wrong codes; nil
	// disables it.
	limiter *attemptLimiter

	// policyKnown is the enforcement policy as last read successfully;
	// policyStale is set while reading it fails. See policy.
	policyKnown atomic.Pointer[twoFactorPolicy]
	policyStale atomic.Bool
	// rolesKnown maps a user ID to the product roles last read for that user
	// ([]string); rolesStale is set while reading them fails.
	rolesKnown sync.Map
	rolesStale atomic.Bool
}

// TOTPSetup is what a user needs to add the account to an authenticator app.
type TOTPSetup struct {
	Secret          string // base32
	FormattedSecret string // grouped in blocks of four for manual entry
	URI             string // otpauth:// provisioning URI
	QRCodeSVG       string // the URI as an SVG QR code
}

func (s *TwoFactorService) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// policy returns the instance-level enforcement policy.
//
// The settings are read on every call, so a change takes effect at once. If
// they cannot be read (a database hiccup), the answer is the last one that
// could: an instance that enforces 2FA keeps doing so, rather than letting
// users who have not enrolled in with a password alone for as long as the
// fault lasts, and one that does not enforce it does not lock anyone out. Only
// before the first successful read is there nothing to go by, and nothing is
// enforced; wiekkod reads the settings once at startup (LoadPolicy), so that
// does not arise there.
func (s *TwoFactorService) policy(ctx context.Context) *twoFactorPolicy {
	all, err := s.settings.GetAll(ctx)
	if err != nil {
		known := s.policyKnown.Load()
		// Log once per fault, not once per request.
		if s.policyStale.CompareAndSwap(false, true) {
			slog.Warn("cannot read the 2FA enforcement settings; keeping the last known values",
				"known", known != nil, "err", err)
		}
		if known == nil {
			return &twoFactorPolicy{}
		}
		return known
	}
	p := &twoFactorPolicy{
		everyone: all[SettingTwoFactorEnforced] == "true",
		admins:   all[SettingTwoFactorEnforcedAdmins] == "true",
		roles:    make(map[string]bool, len(TwoFactorRoles)),
	}
	for _, r := range TwoFactorRoles {
		p.roles[r] = all[SettingTwoFactorEnforcedRole(r)] == "true"
	}
	s.policyKnown.Store(p)
	s.policyStale.Store(false)
	return p
}

// LoadPolicy reads the enforcement settings, so that they are known even if
// the database cannot be reached later (see policy).
func (s *TwoFactorService) LoadPolicy(ctx context.Context) {
	s.policy(ctx)
}

// userRoles returns the distinct product roles the user holds. Like policy, it
// falls back to the roles last read for the user when they cannot be read.
func (s *TwoFactorService) userRoles(ctx context.Context, userID string) []string {
	prs, err := s.productRoles.ListByUserID(ctx, userID)
	if err != nil {
		if s.rolesStale.CompareAndSwap(false, true) {
			slog.Warn("cannot read product roles for 2FA enforcement; keeping the last known values", "err", err)
		}
		if v, ok := s.rolesKnown.Load(userID); ok {
			return v.([]string)
		}
		return nil
	}
	roles := []string{}
	for _, pr := range prs {
		if !slices.Contains(roles, pr.Role) {
			roles = append(roles, pr.Role)
		}
	}
	s.rolesKnown.Store(userID, roles)
	s.rolesStale.Store(false)
	return roles
}

// instanceRules lists the instance-level rules that cover the user.
func (s *TwoFactorService) instanceRules(ctx context.Context, user *model.User) []string {
	p := s.policy(ctx)
	if p.everyone {
		return []string{"all users"}
	}
	var rules []string
	if p.admins && user.IsAdmin {
		rules = append(rules, "administrators")
	}
	if p.anyRole() {
		held := s.userRoles(ctx, user.ID)
		for _, r := range TwoFactorRoles {
			if p.roles[r] && slices.Contains(held, r) {
				rules = append(rules, TwoFactorRoleGroup(r))
			}
		}
	}
	return rules
}

// RequiredFor reports whether an instance-level rule requires the user to use
// two-factor authentication, i.e. whether they must set it up before using
// Wiekko at all. Products that require 2FA are not considered here; they only
// keep users without it out of themselves (see Authorizer.RequireRole).
func (s *TwoFactorService) RequiredFor(ctx context.Context, user *model.User) bool {
	return len(s.instanceRules(ctx, user)) > 0
}

// Requirement returns every rule, instance-level or product, that requires
// the user to use two-factor authentication.
func (s *TwoFactorService) Requirement(ctx context.Context, user *model.User) (TwoFactorRequirement, error) {
	req := TwoFactorRequirement{Instance: s.instanceRules(ctx, user)}
	products, err := s.products.ListTwoFactorRequiredForUser(ctx, user.ID)
	if err != nil {
		return req, err
	}
	req.Products = products
	return req, nil
}

// ProductLocked reports whether the product keeps the user out because it
// requires two-factor authentication and the user has not set it up. It is
// false for users who could not reach the product anyway, so that the answer
// tells nothing about products the user is not a member of.
func (s *TwoFactorService) ProductLocked(ctx context.Context, user *model.User, productID string) bool {
	return s.LockedProduct(ctx, user, productID) != nil
}

// LockedProduct returns the product if ProductLocked is true, else nil.
func (s *TwoFactorService) LockedProduct(ctx context.Context, user *model.User, productID string) *model.Product {
	if user.TOTPEnabled {
		return nil
	}
	p, err := s.products.GetByID(ctx, productID)
	if err != nil || !p.TwoFactorRequired {
		return nil
	}
	if !user.IsAdmin {
		if _, err := s.productRoles.GetByUserAndProduct(ctx, user.ID, productID); err != nil {
			return nil
		}
	}
	return p
}

// BeginSetup generates a new pending secret for the user. Calling it again
// before confirming replaces the pending secret.
func (s *TwoFactorService) BeginSetup(ctx context.Context, userID string) (*TOTPSetup, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.TOTPEnabled {
		return nil, ErrTOTPAlreadyEnabled
	}

	secret, err := totp.GenerateSecret()
	if err != nil {
		return nil, err
	}
	if err := s.users.SetTOTP(ctx, user.ID, secret, false); err != nil {
		return nil, err
	}
	return newTOTPSetup(user.Username, secret)
}

// PendingSetup returns the setup data for a secret that was generated but not
// yet confirmed, so a page re-render shows the same QR code. It returns nil
// when there is no pending setup.
func (s *TwoFactorService) PendingSetup(user *model.User) (*TOTPSetup, error) {
	if user.TOTPEnabled || user.TOTPSecret == "" {
		return nil, nil
	}
	return newTOTPSetup(user.Username, user.TOTPSecret)
}

func newTOTPSetup(account, secret string) (*TOTPSetup, error) {
	uri := totp.ProvisioningURI(totpIssuer, account, secret)
	svg, err := totp.QRCodeSVG(uri)
	if err != nil {
		return nil, err
	}
	return &TOTPSetup{
		Secret:          secret,
		FormattedSecret: totp.FormatSecret(secret),
		URI:             uri,
		QRCodeSVG:       svg,
	}, nil
}

// ConfirmSetup activates TOTP once the user proves the authenticator works.
// It returns a fresh set of recovery codes, which are shown exactly once.
func (s *TwoFactorService) ConfirmSetup(ctx context.Context, userID, code string) ([]string, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.TOTPEnabled {
		return nil, ErrTOTPAlreadyEnabled
	}
	if user.TOTPSecret == "" {
		return nil, ErrTOTPNoPendingSetup
	}

	step, ok := totp.Validate(user.TOTPSecret, code, s.clock())
	if !ok {
		return nil, ErrInvalidTOTPCode
	}
	// Enabling 2FA and handing out the recovery codes happen together:
	// 2FA switched on without codes the user ever saw is one lost phone away
	// from a locked-out account.
	var codes []string
	err = inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.users.SetTOTP(ctx, user.ID, user.TOTPSecret, true); err != nil {
			return err
		}
		// Burn the confirmation code so it cannot be reused for a login.
		if _, err := s.users.ConsumeTOTPStep(ctx, user.ID, step); err != nil {
			return err
		}
		codes, err = s.issueRecoveryCodes(ctx, user.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// Disable turns TOTP off after re-checking the password and a current code.
func (s *TwoFactorService) Disable(ctx context.Context, userID, password, code string) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	req, err := s.Requirement(ctx, user)
	if err != nil {
		return err
	}
	if req.Required() {
		return fmt.Errorf("%w (%s)", ErrTOTPEnforced, req)
	}
	if !user.TOTPEnabled {
		return ErrTOTPNotEnabled
	}
	if s.provider.SupportsPasswordChange() {
		if err := s.provider.VerifyPassword(password, user.PasswordHash); err != nil {
			return ErrIncorrectPassword
		}
	}
	if err := s.Verify(ctx, user, code); err != nil {
		return err
	}
	return s.clear(ctx, user.ID)
}

// RegenerateRecoveryCodes replaces all recovery codes after checking a current code.
func (s *TwoFactorService) RegenerateRecoveryCodes(ctx context.Context, userID, code string) ([]string, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.TOTPEnabled {
		return nil, ErrTOTPNotEnabled
	}
	if err := s.Verify(ctx, user, code); err != nil {
		return nil, err
	}
	return s.issueRecoveryCodes(ctx, user.ID)
}

// RecoveryCodesRemaining returns how many unused recovery codes the user has.
func (s *TwoFactorService) RecoveryCodesRemaining(ctx context.Context, userID string) (int, error) {
	return s.twoFactor.CountUnusedRecoveryCodes(ctx, userID)
}

// AdminReset removes a user's TOTP configuration, e.g. after a lost device.
// The user can log in with the password alone afterwards and, if the instance
// requires 2FA of them, is sent straight to setup.
func (s *TwoFactorService) AdminReset(ctx context.Context, targetID string) error {
	if _, err := s.users.GetByID(ctx, targetID); err != nil {
		return err
	}
	return s.clear(ctx, targetID)
}

// Verify checks a TOTP code (6 digits) or a recovery code for the user and
// consumes it, so neither can be used twice.
//
// Wrong codes are counted per user; after secondFactorMaxFailures within
// SecondFactorFailureWindow it returns an *AttemptsBlockedError without
// looking at the code, until the window ends. A correct code clears the count.
func (s *TwoFactorService) Verify(ctx context.Context, user *model.User, code string) error {
	if !user.TOTPEnabled || user.TOTPSecret == "" {
		return ErrTOTPNotEnabled
	}
	if err := s.limiter.check(user.ID); err != nil {
		return err
	}
	err := s.verifyCode(ctx, user, code)
	switch {
	case err == nil:
		s.limiter.reset(user.ID)
	case errors.Is(err, ErrInvalidTOTPCode) && !errors.Is(err, ErrTOTPCodeReused):
		// A reused code is a correct one typed twice, not a guess.
		if s.limiter.add(user.ID) {
			slog.Warn("second factor blocked after repeated wrong codes",
				"user_id", user.ID, "window", SecondFactorFailureWindow.String())
		}
	}
	return err
}

func (s *TwoFactorService) verifyCode(ctx context.Context, user *model.User, code string) error {
	if totp.IsNumericCode(code) {
		step, ok := totp.Validate(user.TOTPSecret, code, s.clock())
		if !ok {
			return ErrInvalidTOTPCode
		}
		fresh, err := s.users.ConsumeTOTPStep(ctx, user.ID, step)
		if err != nil {
			return err
		}
		if !fresh {
			return ErrTOTPCodeReused
		}
		return nil
	}
	if totp.NormalizeCode(code) == "" {
		return ErrInvalidTOTPCode
	}
	used, err := s.twoFactor.ConsumeRecoveryCode(ctx, user.ID, totp.HashRecoveryCode(code))
	if err != nil {
		return err
	}
	if !used {
		return ErrInvalidTOTPCode
	}
	return nil
}

func (s *TwoFactorService) issueRecoveryCodes(ctx context.Context, userID string) ([]string, error) {
	codes, err := totp.GenerateRecoveryCodes(totp.RecoveryCodeCount)
	if err != nil {
		return nil, err
	}
	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = totp.HashRecoveryCode(c)
	}
	if err := s.twoFactor.ReplaceRecoveryCodes(ctx, userID, hashes); err != nil {
		return nil, fmt.Errorf("store recovery codes: %w", err)
	}
	return codes, nil
}

func (s *TwoFactorService) clear(ctx context.Context, userID string) error {
	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.users.SetTOTP(ctx, userID, "", false); err != nil {
			return err
		}
		return s.twoFactor.DeleteRecoveryCodes(ctx, userID)
	})
}
