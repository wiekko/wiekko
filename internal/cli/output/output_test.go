// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseFormat(t *testing.T) {
	for in, want := range map[string]Format{"": Table, "table": Table, "MD": Markdown, "markdown": Markdown, "json": JSON} {
		got, err := ParseFormat(in)
		if err != nil || got != want {
			t.Errorf("ParseFormat(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseFormat("yaml"); err == nil {
		t.Error("ParseFormat(yaml): want error")
	}
}

func TestRecord(t *testing.T) {
	fields := []Field{{"User", "ada"}, {"Product", ""}}
	data := map[string]string{"user": "ada"}

	var b bytes.Buffer
	if err := Record(&b, Table, "ignored", fields, data); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "User:     ada\nProduct:  -\n" {
		t.Errorf("table = %q", got)
	}

	b.Reset()
	if err := Record(&b, Markdown, "Identity", fields, data); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "## Identity\n\n- **User:** ada\n- **Product:** -\n" {
		t.Errorf("markdown = %q", got)
	}

	b.Reset()
	if err := Record(&b, JSON, "", fields, data); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(b.String()); got != "{\n  \"user\": \"ada\"\n}" {
		t.Errorf("json = %q", got)
	}
}
