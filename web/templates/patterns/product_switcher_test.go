// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package patterns

import (
	"testing"

	"wiekko.net/internal/model"
)

func TestSwitchTargetURL(t *testing.T) {
	tests := []struct {
		name         string
		section      string
		workflowMode string
		want         string
	}{
		// The 11 sections that map straight onto /products/{id}/<section>.
		{"requirements", "requirements", model.WorkflowModeKanban, "/products/p2/requirements"},
		{"defects", "defects", model.WorkflowModeKanban, "/products/p2/defects"},
		{"tasks", "tasks", model.WorkflowModeKanban, "/products/p2/tasks"},
		{"test cases", "test-cases", model.WorkflowModeKanban, "/products/p2/test-cases"},
		{"campaigns", "campaigns", model.WorkflowModeKanban, "/products/p2/campaigns"},
		{"components", "components", model.WorkflowModeKanban, "/products/p2/components"},
		{"capabilities", "capabilities", model.WorkflowModeKanban, "/products/p2/capabilities"},
		{"constraints", "constraints", model.WorkflowModeKanban, "/products/p2/constraints"},
		{"documents", "documents", model.WorkflowModeKanban, "/products/p2/documents"},
		{"milestones", "milestones", model.WorkflowModeKanban, "/products/p2/milestones"},
		{"team", "team", model.WorkflowModeKanban, "/products/p2/team"},
		{"settings", "settings", model.WorkflowModeKanban, "/products/p2/settings"},

		// Sections preserved across a switch regardless of the target workflow.
		{"requirements into scrum target", "requirements", model.WorkflowModeScrum, "/products/p2/requirements"},

		// Overview is the product root, not a segment.
		{"overview", "overview", model.WorkflowModeKanban, "/products/p2"},

		// Workflow-dependent: the workflow surface resolves against the
		// target's mode, so board and sprints map onto each other.
		{"board into kanban target", "board", model.WorkflowModeKanban, "/products/p2/board"},
		{"board into scrum target", "board", model.WorkflowModeScrum, "/products/p2/sprints"},
		{"sprints into kanban target", "sprints", model.WorkflowModeKanban, "/products/p2/board"},
		{"sprints into scrum target", "sprints", model.WorkflowModeScrum, "/products/p2/sprints"},

		// Not product-scoped.
		{"dashboard", "dashboard", model.WorkflowModeKanban, "/products/p2"},
		{"admin users", "admin-users", model.WorkflowModeKanban, "/products/p2"},
		{"admin settings", "admin-settings", model.WorkflowModeKanban, "/products/p2"},
		{"admin invitations", "admin-invitations", model.WorkflowModeKanban, "/products/p2"},
		{"empty", "", model.WorkflowModeKanban, "/products/p2"},

		// Unrecognised input must never reach the URL.
		{"unknown section", "no-such-section", model.WorkflowModeKanban, "/products/p2"},
		{"path traversal attempt", "../../admin/users", model.WorkflowModeKanban, "/products/p2"},
		{"section with query", "defects?x=1", model.WorkflowModeKanban, "/products/p2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SwitchTargetURL("p2", tt.workflowMode, tt.section)
			if got != tt.want {
				t.Errorf("SwitchTargetURL(p2, %q, %q) = %q, want %q",
					tt.workflowMode, tt.section, got, tt.want)
			}
		})
	}
}
