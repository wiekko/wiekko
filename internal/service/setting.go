package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"nexmap.net/internal/store"
)

// Known setting keys.
var knownSettings = map[string]bool{
	"self_registration_enabled":  true,
	"moderation_enabled":         true,
	"2fa_enforced":               true,
	"password_min_length":        true,
	"password_require_uppercase": true,
	"password_require_lowercase": true,
	"password_require_digit":     true,
	"password_require_special":   true,
	"product_creation_enabled":   true,
	SettingPATMaxLifetimeDays:    true,
}

// ErrInvalidSetting is returned for a setting value outside its range.
var ErrInvalidSetting = errors.New("invalid setting value")

// MaxPATLifetimeDays bounds the configurable maximum token lifetime.
const MaxPATLifetimeDays = 3650

// Bounds of the configurable minimum password length. The lower bound keeps
// an administrator (or a stolen administrator session) from switching the
// password policy off altogether.
const (
	MinPasswordMinLength = 8
	MaxPasswordMinLength = 128
)

// validateSetting checks a setting's value: numbers have a range, and every
// other known setting is a switch.
func validateSetting(key, value string) error {
	switch key {
	case SettingPATMaxLifetimeDays:
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > MaxPATLifetimeDays {
			return fmt.Errorf("%w: %s must be a number of days from 0 (no limit) to %d", ErrInvalidSetting, key, MaxPATLifetimeDays)
		}
	case "password_min_length":
		n, err := strconv.Atoi(value)
		if err != nil || n < MinPasswordMinLength || n > MaxPasswordMinLength {
			return fmt.Errorf("%w: %s must be a number from %d to %d", ErrInvalidSetting, key, MinPasswordMinLength, MaxPasswordMinLength)
		}
	default:
		if value != "true" && value != "false" {
			return fmt.Errorf("%w: %s must be true or false", ErrInvalidSetting, key)
		}
	}
	return nil
}

// SettingService handles instance settings.
type SettingService struct {
	settings store.SettingStore
}

// Get returns a single setting value.
func (s *SettingService) Get(ctx context.Context, key string) (string, error) {
	return s.settings.Get(ctx, key)
}

// GetAll returns all settings.
func (s *SettingService) GetAll(ctx context.Context) (map[string]string, error) {
	return s.settings.GetAll(ctx)
}

// Update applies a map of setting changes. Unknown keys are ignored.
func (s *SettingService) Update(ctx context.Context, changes map[string]string) error {
	for key, value := range changes {
		if !knownSettings[key] {
			continue
		}
		if err := validateSetting(key, value); err != nil {
			return err
		}
		if err := s.settings.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}
