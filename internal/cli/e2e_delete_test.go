// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli_test

// Deleting backlog items and defects from the CLI (FR-153). Like every
// deletion the commands need --yes; the server decides who may delete.

import (
	"strings"
	"testing"

	"nexmap.net/internal/cli"
	"nexmap.net/internal/cli/ops"
)

func TestE2EDeleteBacklogItems(t *testing.T) {
	e := newAviary(t)
	create := func(args ...string) ops.RequirementDetail {
		t.Helper()
		return decode[ops.RequirementDetail](t, e.mustRun("", append(args, "-o", "json")...).stdout)
	}
	show := func(ref string) ops.RequirementDetail {
		t.Helper()
		return decode[ops.RequirementDetail](t, e.mustRun("", "req", "show", ref, "-o", "json").stdout)
	}
	wantGone := func(ref string) {
		t.Helper()
		if r := e.run("", "", "req", "show", ref); r.code != cli.ExitNotFound {
			t.Errorf("req show %s after delete: exit %d, want %d", ref, r.code, cli.ExitNotFound)
		}
	}

	// --children keep (the default): the grandchild moves up to the parent.
	top := create("req", "create", "--kind", "requirement", "--title", "Delete me later")
	mid := create("task", "create", "--title", "Middle", "--parent", top.Identifier)
	leaf := create("task", "create", "--title", "Leaf", "--parent", mid.Identifier)

	r := e.run("", "", "task", "delete", mid.Identifier)
	if r.code != cli.ExitUsage || !strings.Contains(r.stderr, "--yes") {
		t.Errorf("delete without --yes: exit %d, stderr %q", r.code, r.stderr)
	}
	show(mid.Identifier) // still there
	if r := e.run("", "", "task", "delete", mid.Identifier, "--yes", "--children", "orphan"); r.code != cli.ExitUsage {
		t.Errorf("bad --children: exit %d, want %d", r.code, cli.ExitUsage)
	}
	if r := e.run("", "", "task", "delete", top.Identifier, "--yes"); r.code != cli.ExitNotFound {
		t.Errorf("task delete on a requirement: exit %d, want %d", r.code, cli.ExitNotFound)
	}
	wantForbidden(t, "developer deleting a task", e.as("cy", "", "task", "delete", mid.Identifier, "--yes"))

	out := e.mustRun("", "task", "delete", mid.Identifier, "--yes").stdout
	if !strings.Contains(out, mid.Identifier) || !strings.Contains(out, "Middle") {
		t.Errorf("delete output = %q", out)
	}
	wantGone(mid.Identifier)
	if got := show(leaf.Identifier); got.Parent == nil || got.Parent.ID != top.ID {
		t.Errorf("kept child's parent = %+v, want %s", got.Parent, top.Identifier)
	}

	// --children delete: the whole subtree goes, reported as JSON.
	ref := decode[ops.Ref](t, e.mustRun("", "req", "delete", top.Identifier, "--yes", "--children", "delete", "-o", "json").stdout)
	if ref.ID != top.ID || ref.Identifier != top.Identifier {
		t.Errorf("deleted = %+v, want %s", ref, top.Identifier)
	}
	wantGone(top.Identifier)
	wantGone(leaf.Identifier)

	if r := e.run("", "", "req", "delete", "FR-9999", "--yes"); r.code != cli.ExitNotFound {
		t.Errorf("delete of an unknown item: exit %d, want %d", r.code, cli.ExitNotFound)
	}
}

func TestE2EDeleteDefect(t *testing.T) {
	e := newAviary(t)
	d := decode[ops.DefectDetail](t, e.mustAs("bo", "", "defect", "create", "--title", "Deleted soon", "-o", "json").stdout)

	if r := e.run("", "", "defect", "delete", d.Identifier); r.code != cli.ExitUsage || !strings.Contains(r.stderr, "--yes") {
		t.Errorf("delete without --yes: exit %d, stderr %q", r.code, r.stderr)
	}
	wantForbidden(t, "tester deleting a defect", e.as("bo", "", "defect", "delete", d.Identifier, "--yes"))
	wantForbidden(t, "developer deleting a defect", e.as("cy", "", "defect", "delete", d.Identifier, "--yes"))
	e.mustRun("", "defect", "show", d.Identifier) // still there

	out := e.mustRun("", "defect", "delete", d.Identifier, "--yes").stdout
	if !strings.Contains(out, d.Identifier) || !strings.Contains(out, "Deleted soon") {
		t.Errorf("delete output = %q", out)
	}
	if r := e.run("", "", "defect", "show", d.Identifier); r.code != cli.ExitNotFound {
		t.Errorf("defect show after delete: exit %d, want %d", r.code, cli.ExitNotFound)
	}
}
