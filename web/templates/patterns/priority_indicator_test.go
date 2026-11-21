// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package patterns

import (
	"context"
	"strings"
	"testing"

	"nexmap.net/web/templates"
)

// TestPriorityBadge checks how a priority is shown: a set priority as a
// coloured "P<n>" badge, and 0, the default meaning no priority (DF-2), as a
// muted dash in tables and detail pages and not at all on board cards.
func TestPriorityBadge(t *testing.T) {
	ctx := context.Background()
	badge := func(p int) string {
		return renderToString(t, func(sb *strings.Builder) error { return PriorityBadge(p).Render(ctx, sb) })
	}
	if got := badge(1); !strings.Contains(got, ">P1<") {
		t.Errorf("priority 1 = %q, want a P1 badge", got)
	}
	for _, p := range []int{0, -1} {
		got := badge(p)
		if strings.Contains(got, ">P") || !strings.Contains(got, "—") || !strings.Contains(got, `title="No priority"`) {
			t.Errorf("priority %d = %q, want a dash titled No priority", p, got)
		}
	}

	card := func(p int) string {
		c := templates.BoardCard{EntityType: "requirement", ID: "r1", Identifier: "FR-1", Title: "Record a sighting", Priority: p}
		return renderToString(t, func(sb *strings.Builder) error { return BoardCardView("p1", c, false).Render(ctx, sb) })
	}
	if got := card(2); !strings.Contains(got, ">P2<") {
		t.Error("board card with priority 2 shows no P2 badge")
	}
	if got := card(0); strings.Contains(got, ">P0<") || strings.Contains(got, "No priority") {
		t.Errorf("board card without priority shows a priority: %q", got)
	}
}
