// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli_test

// Archiving backlog items and defects from the CLI (FR-155). Archived items
// leave the default lists; --archived include|only brings them back.

import (
	"testing"

	"nexmap.net/internal/cli"
	"nexmap.net/internal/cli/ops"
)

func TestE2EArchiveBacklogItems(t *testing.T) {
	e := newAviary(t)
	top := decode[ops.RequirementDetail](t, e.mustRun("", "req", "create", "--kind", "requirement", "--title", "Archive me", "-o", "json").stdout)
	task := decode[ops.RequirementDetail](t, e.mustRun("", "task", "create", "--title", "Below", "--parent", top.Identifier, "-o", "json").stdout)
	listed := func(args ...string) map[string]ops.RequirementSummary {
		t.Helper()
		items := decode[[]ops.RequirementSummary](t, e.mustRun("", append([]string{"req", "list", "-o", "json"}, args...)...).stdout)
		m := map[string]ops.RequirementSummary{}
		for _, it := range items {
			m[it.Identifier] = it
		}
		return m
	}

	e.mustRun("", "req", "status", top.Identifier, "Done")
	if r := e.run("", "", "req", "archive", top.Identifier); r.code == 0 {
		t.Fatal("archived a requirement with an open task below it")
	}
	e.mustRun("", "task", "status", task.Identifier, "Done")
	if r := e.run("", "", "task", "archive", top.Identifier); r.code != cli.ExitNotFound {
		t.Errorf("task archive on a requirement: exit %d, want %d", r.code, cli.ExitNotFound)
	}
	wantForbidden(t, "developer archiving", e.as("cy", "", "req", "archive", top.Identifier))

	got := decode[ops.RequirementDetail](t, e.mustRun("", "req", "archive", top.Identifier, "-o", "json").stdout)
	if !got.Archived {
		t.Fatalf("req archive: %+v not archived", got.RequirementSummary)
	}
	if _, ok := listed()[top.Identifier]; ok {
		t.Error("req list shows an archived requirement")
	}
	if _, ok := listed()[task.Identifier]; ok {
		t.Error("req list shows the archived task below it")
	}
	if it, ok := listed("--archived", "include")[task.Identifier]; !ok || !it.Archived {
		t.Errorf("req list --archived include: task %+v (listed %v)", it, ok)
	}
	for id, it := range listed("--archived", "only") {
		if !it.Archived {
			t.Errorf("req list --archived only shows %s", id)
		}
	}
	if r := e.run("", "", "req", "list", "--archived", "maybe"); r.code != cli.ExitUsage {
		t.Errorf("--archived maybe: exit %d, want %d", r.code, cli.ExitUsage)
	}

	got = decode[ops.RequirementDetail](t, e.mustRun("", "req", "unarchive", top.Identifier, "-o", "json").stdout)
	if got.Archived {
		t.Error("req unarchive left the requirement archived")
	}
	if _, ok := listed()[task.Identifier]; !ok {
		t.Error("unarchiving did not bring the task back")
	}
}

func TestE2EArchiveDefect(t *testing.T) {
	e := newAviary(t)
	d := decode[ops.DefectDetail](t, e.mustAs("bo", "", "defect", "create", "--title", "Old bug", "-o", "json").stdout)
	listed := func(args ...string) bool {
		t.Helper()
		for _, it := range decode[[]ops.DefectSummary](t, e.mustRun("", append([]string{"defect", "list", "-o", "json"}, args...)...).stdout) {
			if it.Identifier == d.Identifier {
				return true
			}
		}
		return false
	}

	if r := e.run("", "", "defect", "archive", d.Identifier); r.code == 0 {
		t.Fatal("archived an open defect")
	}
	e.mustRun("", "defect", "status", d.Identifier, "Done")
	wantForbidden(t, "tester archiving", e.as("bo", "", "defect", "archive", d.Identifier))
	got := decode[ops.DefectDetail](t, e.mustRun("", "defect", "archive", d.Identifier, "-o", "json").stdout)
	if !got.Archived {
		t.Fatal("defect archive did not archive")
	}
	if listed() || !listed("--archived", "include") || !listed("--archived", "only") {
		t.Errorf("defect list: default %v, include %v, only %v", listed(), listed("--archived", "include"), listed("--archived", "only"))
	}
	// The defect still resolves by identifier for every other command.
	e.mustRun("", "defect", "show", d.Identifier)
	e.mustRun("", "defect", "status", d.Identifier, "New")
	if !listed() {
		t.Error("reopening did not bring the defect back")
	}
}
