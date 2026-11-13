package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"nexmap.net/internal/auth"
	"nexmap.net/internal/auth/totp"
	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// SettingTwoFactorEnforced is the instance setting that makes TOTP mandatory (SC-107).
const SettingTwoFactorEnforced = "2fa_enforced"

// totpIssuer is the issuer name authenticator apps display next to the account.
const totpIssuer = "Nexmap"

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
	ErrTOTPEnforced = errors.New("two-factor authentication is required on this instance and cannot be disabled")
	// ErrIncorrectPassword is returned when a re-entered password does not match.
	ErrIncorrectPassword = errors.New("current password is incorrect")
)

// TwoFactorService manages TOTP enrollment, verification and recovery codes.
type TwoFactorService struct {
	users     store.UserStore
	twoFactor store.TwoFactorStore
	settings  store.SettingStore
	provider  auth.Provider
	now       func() time.Time

	// limiter blocks a user's second factor after repeated wrong codes; nil
	// disables it.
	limiter *attemptLimiter
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

// Enforced reports whether the instance requires two-factor authentication.
// It fails open (false) if the setting cannot be read, so a database hiccup
// does not lock every user out of the web UI.
func (s *TwoFactorService) Enforced(ctx context.Context) bool {
	v, err := s.settings.Get(ctx, SettingTwoFactorEnforced)
	return err == nil && v == "true"
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
	if err := s.users.SetTOTP(ctx, user.ID, user.TOTPSecret, true); err != nil {
		return nil, err
	}
	// Burn the confirmation code so it cannot be reused for a login.
	if _, err := s.users.ConsumeTOTPStep(ctx, user.ID, step); err != nil {
		return nil, err
	}
	return s.issueRecoveryCodes(ctx, user.ID)
}

// Disable turns TOTP off after re-checking the password and a current code.
func (s *TwoFactorService) Disable(ctx context.Context, userID, password, code string) error {
	if s.Enforced(ctx) {
		return ErrTOTPEnforced
	}
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
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
// enforces 2FA, is sent straight to setup.
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
	if err := s.users.SetTOTP(ctx, userID, "", false); err != nil {
		return err
	}
	return s.twoFactor.DeleteRecoveryCodes(ctx, userID)
}
