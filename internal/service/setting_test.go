// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/testutil"
)

func TestSettingUpdate(t *testing.T) {
	t.Run("known keys written", func(t *testing.T) {
		written := map[string]string{}
		svc := &SettingService{
			settings: &testutil.MockSettingStore{
				SetFunc: func(_ context.Context, k, v string) error { written[k] = v; return nil },
			},
		}
		err := svc.Update(context.Background(), map[string]string{
			"moderation_enabled": "true",
			"2fa_enforced":       "false",
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if written["moderation_enabled"] != "true" {
			t.Fatalf("moderation_enabled = %q, want %q", written["moderation_enabled"], "true")
		}
		if written["2fa_enforced"] != "false" {
			t.Fatalf("2fa_enforced = %q, want %q", written["2fa_enforced"], "false")
		}
	})

	t.Run("unknown keys ignored", func(t *testing.T) {
		written := map[string]string{}
		svc := &SettingService{
			settings: &testutil.MockSettingStore{
				SetFunc: func(_ context.Context, k, v string) error { written[k] = v; return nil },
			},
		}
		err := svc.Update(context.Background(), map[string]string{
			"unknown_key":        "val",
			"moderation_enabled": "true",
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if _, ok := written["unknown_key"]; ok {
			t.Fatal("unknown key should not be written")
		}
		if _, ok := written["moderation_enabled"]; !ok {
			t.Fatal("known key should be written")
		}
	})

	t.Run("set error propagated", func(t *testing.T) {
		svc := &SettingService{
			settings: &testutil.MockSettingStore{
				SetFunc: func(_ context.Context, _, _ string) error { return errors.New("db error") },
			},
		}
		err := svc.Update(context.Background(), map[string]string{"moderation_enabled": "true"})
		if err == nil || err.Error() != "db error" {
			t.Fatalf("err = %v, want 'db error'", err)
		}
	})

	t.Run("empty map", func(t *testing.T) {
		svc := &SettingService{
			settings: &testutil.MockSettingStore{
				SetFunc: func(_ context.Context, _, _ string) error {
					t.Fatal("Set should not be called for empty map")
					return nil
				},
			},
		}
		if err := svc.Update(context.Background(), map[string]string{}); err != nil {
			t.Fatalf("Update: %v", err)
		}
	})
}

func TestSettingGet(t *testing.T) {
	svc := &SettingService{
		settings: &testutil.MockSettingStore{
			GetFunc: func(_ context.Context, key string) (string, error) {
				if key == "moderation_enabled" {
					return "true", nil
				}
				return "", errors.New("not found")
			},
		},
	}
	val, err := svc.Get(context.Background(), "moderation_enabled")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "true" {
		t.Fatalf("val = %q, want %q", val, "true")
	}
}

func TestSettingGetAll(t *testing.T) {
	expected := map[string]string{"a": "1", "b": "2"}
	svc := &SettingService{
		settings: &testutil.MockSettingStore{
			GetAllFunc: func(_ context.Context) (map[string]string, error) { return expected, nil },
		},
	}
	result, err := svc.GetAll(context.Background())
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
}
