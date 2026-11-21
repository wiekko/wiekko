// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package patterns

import (
	"context"
	"strings"
	"testing"
)

func renderToString(t *testing.T, render func(*strings.Builder) error) string {
	t.Helper()
	var sb strings.Builder
	if err := render(&sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

// The error region and the out-of-band swap must agree on the element ID, or
// the message silently goes nowhere.
func TestErrorRegionAndInlineErrorAgreeOnID(t *testing.T) {
	region := renderToString(t, func(sb *strings.Builder) error {
		return ErrorRegion(AttachmentsErrorRegion).Render(context.Background(), sb)
	})
	if !strings.Contains(region, `id="attachments-error"`) {
		t.Fatalf("region markup lacks the id: %s", region)
	}

	msg := renderToString(t, func(sb *strings.Builder) error {
		return InlineError(AttachmentsErrorRegion, "That file is too large.").Render(context.Background(), sb)
	})
	for _, want := range []string{`id="attachments-error"`, `hx-swap-oob="innerHTML"`, "That file is too large."} {
		if !strings.Contains(msg, want) {
			t.Fatalf("inline error missing %q: %s", want, msg)
		}
	}
}

func TestClearedErrorRegionEmptiesTheRegion(t *testing.T) {
	out := renderToString(t, func(sb *strings.Builder) error {
		return ClearedErrorRegion(CommentsErrorRegion).Render(context.Background(), sb)
	})
	if !strings.Contains(out, `id="comments-error"`) || !strings.Contains(out, `hx-swap-oob="innerHTML"`) {
		t.Fatalf("cleared region markup is wrong: %s", out)
	}
	// Nothing between the tags, so the region really empties.
	if !strings.Contains(out, `hx-swap-oob="innerHTML"></div>`) {
		t.Fatalf("cleared region is not empty: %s", out)
	}
}

// A rendered message must be escaped, never injected as markup.
func TestInlineErrorEscapesTheMessage(t *testing.T) {
	out := renderToString(t, func(sb *strings.Builder) error {
		return InlineError(AttachmentsErrorRegion, `<script>alert(1)</script>`).Render(context.Background(), sb)
	})
	if strings.Contains(out, "<script>") {
		t.Fatalf("message was not escaped: %s", out)
	}
}

func TestHumanizeBytes(t *testing.T) {
	for in, want := range map[int64]string{
		0: "0 B", 512: "512 B", 1023: "1023 B",
		1024: "1.0 KB", 1536: "1.5 KB", 10 << 20: "10.0 MB", 3 << 30: "3.0 GB",
	} {
		if got := HumanizeBytes(in); got != want {
			t.Fatalf("HumanizeBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
