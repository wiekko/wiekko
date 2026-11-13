package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/testutil"
)

// TestEnforcedKeepsLastKnownValue checks that a failing read of the setting
// neither switches enforcement off nor on.
func TestEnforcedKeepsLastKnownValue(t *testing.T) {
	value, fail := "false", true
	svc := &TwoFactorService{settings: &testutil.MockSettingStore{
		GetFunc: func(context.Context, string) (string, error) {
			if fail {
				return "", errors.New("database is unavailable")
			}
			return value, nil
		},
	}}
	ctx := context.Background()

	if svc.Enforced(ctx) {
		t.Error("enforced before the setting was ever read")
	}

	fail, value = false, "true"
	if !svc.Enforced(ctx) {
		t.Fatal("not enforced although the setting says so")
	}
	fail = true
	if !svc.Enforced(ctx) {
		t.Error("a failed read switched enforcement off")
	}

	fail, value = false, "false"
	if svc.Enforced(ctx) {
		t.Fatal("enforced although the setting was turned off")
	}
	fail = true
	if svc.Enforced(ctx) {
		t.Error("a failed read switched enforcement on")
	}
}
