package service

import (
	"context"

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
		if err := s.settings.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}
