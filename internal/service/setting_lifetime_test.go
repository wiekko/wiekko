package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/testutil"
)

func TestSettingPATMaxLifetimeValidation(t *testing.T) {
	var stored string
	svc := &SettingService{settings: &testutil.MockSettingStore{
		SetFunc: func(_ context.Context, _, value string) error { stored = value; return nil },
	}}
	for _, ok := range []string{"0", "90", "3650"} {
		if err := svc.Update(context.Background(), map[string]string{SettingPATMaxLifetimeDays: ok}); err != nil || stored != ok {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"-1", "3651", "x", ""} {
		err := svc.Update(context.Background(), map[string]string{SettingPATMaxLifetimeDays: bad})
		if !errors.Is(err, ErrInvalidSetting) {
			t.Errorf("%q: err = %v, want ErrInvalidSetting", bad, err)
		}
	}
}
