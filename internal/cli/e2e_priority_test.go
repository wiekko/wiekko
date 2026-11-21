// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli_test

// Priority of backlog items and defects from the CLI and MCP (FR-149): set on
// create, changed with req/task/defect update.

import (
	"testing"

	"wiekko.net/internal/cli"
	"wiekko.net/internal/cli/ops"
)

func TestE2EPriority(t *testing.T) {
	e := newAviary(t)
	req := func(args ...string) ops.RequirementDetail {
		t.Helper()
		return decode[ops.RequirementDetail](t, e.mustRun("", append(args, "-o", "json")...).stdout)
	}

	// Requirements and tasks: set on create, change, clear with 0.
	r := req("req", "create", "--kind", "requirement", "--title", "Prioritised", "--priority", "2")
	if r.Priority != 2 {
		t.Fatalf("created requirement priority = %d, want 2", r.Priority)
	}
	task := req("task", "create", "--title", "Prioritised task", "--parent", r.Identifier, "--priority", "5")
	if task.Priority != 5 {
		t.Fatalf("created task priority = %d, want 5", task.Priority)
	}
	if r = req("req", "update", r.Identifier, "--priority", "1"); r.Priority != 1 || r.Parent != nil {
		t.Fatalf("after update: priority %d, parent %+v", r.Priority, r.Parent)
	}
	if task = req("task", "update", task.Identifier, "--priority", "0"); task.Priority != 0 || task.Parent == nil {
		t.Fatalf("after clearing: priority %d, parent %+v", task.Priority, task.Parent)
	}
	// Parent and priority in one call.
	if task = req("task", "update", task.Identifier, "--parent", "", "--priority", "3"); task.Priority != 3 || task.Parent != nil {
		t.Fatalf("after moving: priority %d, parent %+v", task.Priority, task.Parent)
	}

	// Defects: a tester sets it on create, the owner changes it.
	d := decode[ops.DefectDetail](t, e.mustAs("bo", "", "defect", "create", "--title", "Urgent", "--priority", "4", "-o", "json").stdout)
	if d.Priority != 4 {
		t.Fatalf("created defect priority = %d, want 4", d.Priority)
	}
	d = decode[ops.DefectDetail](t, e.mustRun("", "defect", "update", d.Identifier, "--priority", "1", "-o", "json").stdout)
	if d.Priority != 1 {
		t.Fatalf("updated defect priority = %d, want 1", d.Priority)
	}

	// Refusals and errors.
	for _, args := range [][]string{
		{"defect", "update", d.Identifier},
		{"req", "update", r.Identifier},
		{"defect", "update", d.Identifier, "--priority", "-1"},
		{"req", "create", "--kind", "requirement", "--title", "Negative", "--priority", "-3"},
		{"defect", "create", "--title", "Negative", "--priority", "x"},
	} {
		if res := e.run("", "", args...); res.code != cli.ExitUsage {
			t.Errorf("%v: exit %d, want %d (%s)", args, res.code, cli.ExitUsage, res.stderr)
		}
	}
	wantForbidden(t, "developer changing a defect's priority", e.as("cy", "", "defect", "update", d.Identifier, "--priority", "2"))
	wantForbidden(t, "developer changing a requirement's priority", e.as("cy", "", "req", "update", r.Identifier, "--priority", "2"))
	if res := e.run("", "", "defect", "update", "DF-9999", "--priority", "1"); res.code != cli.ExitNotFound {
		t.Errorf("update of an unknown defect: exit %d, want %d", res.code, cli.ExitNotFound)
	}
	d = decode[ops.DefectDetail](t, e.mustRun("", "defect", "show", d.Identifier, "-o", "json").stdout)
	if d.Priority != 1 {
		t.Errorf("defect priority after refusals = %d, want 1", d.Priority)
	}
}

func TestE2EMCPPriority(t *testing.T) {
	e := newAviary(t)
	s := e.startMCP("mcp")

	_, got := mustTool(t, s, "create_backlog_item", map[string]any{"kind": "requirement", "title": "Via MCP", "priority": 2})
	item := got.(map[string]any)
	if item["priority"] != float64(2) {
		t.Fatalf("create_backlog_item priority = %v", item["priority"])
	}
	_, got = mustTool(t, s, "update_backlog_item", map[string]any{"ref": item["identifier"], "priority": 0})
	if got.(map[string]any)["priority"] != float64(0) {
		t.Fatalf("update_backlog_item priority = %v", got.(map[string]any)["priority"])
	}
	_, got = mustTool(t, s, "create_defect", map[string]any{"title": "Via MCP", "priority": 3})
	if got.(map[string]any)["priority"] != float64(3) {
		t.Fatalf("create_defect priority = %v", got.(map[string]any)["priority"])
	}
}
