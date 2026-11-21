// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli_test

// Linking defects to components and capabilities (FR-147): on create, with
// defect link/unlink, and through the MCP create_defect tool.

import (
	"strings"
	"testing"

	"nexmap.net/internal/cli"
	"nexmap.net/internal/cli/ops"
)

// defectLinked reports whether d has a link of the given type whose label
// contains name.
func defectLinked(d ops.DefectDetail, typ, name string) bool {
	for _, l := range d.Links {
		if l.Type == typ && strings.Contains(l.Label, name) {
			return true
		}
	}
	return false
}

func countLinks(d ops.DefectDetail, typ string) int {
	n := 0
	for _, l := range d.Links {
		if l.Type == typ {
			n++
		}
	}
	return n
}

func TestE2EDefectLinks(t *testing.T) {
	e := newAviary(t)
	e.mustRun("", "component", "create", "--name", "Telemetry")
	e.mustRun("", "component", "create", "--name", "Radio")
	e.mustRun("", "capability", "create", "--name", "Crash reports", "--component", "Telemetry")
	e.mustRun("", "capability", "create", "--name", "Usage stats", "--component", "Telemetry")

	// Create: --component repeated, --capability comma-separated.
	d := decode[ops.DefectDetail](t, e.mustAs("bo", "", "defect", "create", "--title", "Reports lost offline",
		"--component", "Telemetry", "--component", "radio", "--capability", "Crash reports, Usage stats", "-o", "json").stdout)
	if countLinks(d, "component") != 2 || countLinks(d, "capability") != 2 ||
		!defectLinked(d, "component", "Radio") || !defectLinked(d, "capability", "Usage stats") {
		t.Fatalf("links after create = %+v", d.Links)
	}
	md := e.mustRun("", "defect", "show", d.Identifier, "-o", "md").stdout
	for _, want := range []string{"component: Telemetry", "capability: Crash reports"} {
		if !strings.Contains(md, want) {
			t.Errorf("defect show lacks %q:\n%s", want, md)
		}
	}
	// A bad reference creates nothing.
	if r := e.as("bo", "", "defect", "create", "--title", "Never created", "--capability", "Nope"); r.code != cli.ExitNotFound {
		t.Errorf("create with an unknown capability: exit %d, want %d (%s)", r.code, cli.ExitNotFound, r.stderr)
	}
	if list := e.mustRun("", "defect", "list", "--search", "Never created").stdout; strings.Contains(list, "Never created") {
		t.Error("defect created despite an unknown capability")
	}

	// Unlink (developer) and link again (tester); linking twice is a no-op.
	d = decode[ops.DefectDetail](t, e.mustAs("cy", "", "defect", "unlink", d.Identifier,
		"--component", "Radio", "--capability", "Usage stats", "-o", "json").stdout)
	if countLinks(d, "component") != 1 || countLinks(d, "capability") != 1 || defectLinked(d, "component", "Radio") {
		t.Fatalf("links after unlink = %+v", d.Links)
	}
	reqs := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "-o", "json").stdout)
	fr := reqs[0].Identifier
	for range 2 {
		d = decode[ops.DefectDetail](t, e.mustAs("bo", "", "defect", "link", d.Identifier,
			"--component", "Radio", "--capability", "Usage stats", "--req", fr, "-o", "json").stdout)
	}
	if countLinks(d, "component") != 2 || countLinks(d, "capability") != 2 || countLinks(d, "requirement") != 1 {
		t.Fatalf("links after linking twice = %+v", d.Links)
	}

	// Refusals and errors.
	if r := e.run("", "", "defect", "link", d.Identifier); r.code != cli.ExitUsage {
		t.Errorf("link without targets: exit %d, want %d", r.code, cli.ExitUsage)
	}
	if r := e.run("", "", "defect", "unlink", d.Identifier, "--capability", "Crash reports", "--component", "Nope"); r.code != cli.ExitNotFound {
		t.Errorf("unlink of an unknown component: exit %d, want %d", r.code, cli.ExitNotFound)
	}
	e.mustRun("", "capability", "create", "--name", "Unlinked")
	if r := e.run("", "", "defect", "unlink", d.Identifier, "--capability", "Crash reports", "--capability", "Unlinked"); r.code != cli.ExitUsage {
		t.Errorf("unlink of something not linked: exit %d, want %d", r.code, cli.ExitUsage)
	}
	// Nothing was removed by the failed unlinks.
	d = decode[ops.DefectDetail](t, e.mustRun("", "defect", "show", d.Identifier, "-o", "json").stdout)
	if !defectLinked(d, "capability", "Crash reports") {
		t.Errorf("a failed unlink removed a link: %+v", d.Links)
	}
	wantForbidden(t, "tester unlinking", e.as("bo", "", "defect", "unlink", d.Identifier, "--component", "Radio"))
	wantForbidden(t, "viewer linking", e.as("ada", "", "defect", "link", d.Identifier, "--capability", "Unlinked"))
	if r := e.run("", "", "defect", "link", "DF-9999", "--component", "Radio"); r.code != cli.ExitNotFound {
		t.Errorf("link of an unknown defect: exit %d, want %d", r.code, cli.ExitNotFound)
	}
}

func TestE2EMCPCreateDefectLinks(t *testing.T) {
	e := newAviary(t)
	e.mustRun("", "component", "create", "--name", "Telemetry")
	e.mustRun("", "capability", "create", "--name", "Crash reports", "--component", "Telemetry")
	s := e.startMCP("mcp")

	_, got := mustTool(t, s, "create_defect", map[string]any{
		"title": "Reports lost offline", "components": []string{"Telemetry"}, "capabilities": []string{"Crash reports"},
	})
	ident := got.(map[string]any)["identifier"].(string)
	d := decode[ops.DefectDetail](t, e.mustRun("", "defect", "show", ident, "-o", "json").stdout)
	if !defectLinked(d, "component", "Telemetry") || !defectLinked(d, "capability", "Crash reports") {
		t.Fatalf("create_defect links = %+v", d.Links)
	}
}
