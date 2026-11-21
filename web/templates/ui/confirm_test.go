// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package ui

import (
	"context"
	"strings"
	"testing"
)

func TestConfirmAttributes(t *testing.T) {
	attrs := Confirm(ConfirmDanger, "Delete status", "Delete", "Delete status “Done”? This cannot be undone.")
	want := map[string]string{
		"hx-confirm":           "Delete status “Done”? This cannot be undone.",
		"data-confirm-variant": "danger",
		"data-confirm-title":   "Delete status",
		"data-confirm-label":   "Delete",
	}
	for k, v := range want {
		if attrs[k] != v {
			t.Errorf("%s = %v, want %q", k, attrs[k], v)
		}
	}
}

func TestItemName(t *testing.T) {
	cases := []struct{ id, title, want string }{
		{"FR-12", "Login", "FR-12 “Login”"},
		{"", "Login", "“Login”"},
		{"DF-3", "", "DF-3"},
		{"", " padded ", "“padded”"},
	}
	for _, c := range cases {
		if got := ItemName(c.id, c.title); got != c.want {
			t.Errorf("ItemName(%q, %q) = %q, want %q", c.id, c.title, got, c.want)
		}
	}
}

// TestConfirmTemplate checks the dialog modal.js fills in: it needs the
// message slot, a confirm button carrying both variants' classes, and a
// cancel button.
func TestConfirmTemplate(t *testing.T) {
	var sb strings.Builder
	if err := ConfirmTemplate().Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, s := range []string{
		`<template id="confirm-dialog-template">`,
		`id="confirm-dialog-title"`,
		`data-confirm-message`,
		`data-confirm-action`,
		`data-class-danger="` + ConfirmButtonClass(ConfirmDanger) + `"`,
		`data-class-neutral="` + ConfirmButtonClass(ConfirmNeutral) + `"`,
		`data-modal-close="confirm-dialog"`,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("confirm template lacks %s:\n%s", s, out)
		}
	}
	if strings.Contains(out, "<script") {
		t.Error("confirm template must not contain scripts: they would not run inside <template>")
	}
}

func TestConfirmButtonClass(t *testing.T) {
	if !strings.Contains(ConfirmButtonClass(ConfirmDanger), "bg-red-600") {
		t.Error("danger confirm button is not red")
	}
	if !strings.Contains(ConfirmButtonClass(ConfirmNeutral), "bg-indigo-600") {
		t.Error("neutral confirm button is not indigo")
	}
}
