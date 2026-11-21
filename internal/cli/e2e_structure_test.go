// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli_test

// Structure, constraint and link writes (FR-134 – FR-136) end to end, as the
// product owner (root) unless noted.

import (
	"strings"
	"testing"

	"wiekko.net/internal/cli"
	"wiekko.net/internal/cli/ops"
)

func TestE2EStructureWrites(t *testing.T) {
	e := newAviary(t)

	comp := decode[ops.ComponentView](t, e.mustRun("", "component", "create", "--name", "Telemetry", "--body", "Metrics and crash reports.", "-o", "json").stdout)
	if comp.Name != "Telemetry" || comp.Description != "Metrics and crash reports." {
		t.Fatalf("component = %+v", comp)
	}
	capa := decode[ops.CapabilityDetail](t, e.mustRun("", "capability", "create", "--name", "Crash reports", "--component", "telemetry", "-o", "json").stdout)
	if capa.Component != "Telemetry" {
		t.Fatalf("capability component = %q, want Telemetry", capa.Component)
	}
	// Unquoted names; an empty --component moves it to product level.
	capa = decode[ops.CapabilityDetail](t, e.mustRun("", "capability", "update", "Crash", "reports", "--name", "Crash reporting", "--component", "", "-o", "json").stdout)
	if capa.Name != "Crash reporting" || capa.Component != "" {
		t.Fatalf("updated capability = %+v", capa)
	}
	comp = decode[ops.ComponentView](t, e.mustRun("", "component", "update", "Telemetry", "--body", "", "-o", "json").stdout)
	if comp.Description != "" || comp.Name != "Telemetry" {
		t.Fatalf("component after clearing the description = %+v", comp)
	}

	// Traceability: requirements and constraints link to the capability.
	reqs := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "--kind", "requirement", "-o", "json").stdout)
	cons := decode[[]ops.ConstraintSummary](t, e.mustRun("", "constraint", "list", "-o", "json").stdout)
	if len(reqs) == 0 || len(cons) == 0 {
		t.Fatal("seed has no requirements or constraints")
	}
	fr, ac := reqs[0].Identifier, cons[0].Identifier
	capa = decode[ops.CapabilityDetail](t, e.mustRun("", "capability", "link", "Crash", "reporting", fr, ac, "-o", "json").stdout)
	if len(capa.Requirements) != 1 || capa.Requirements[0].Identifier != fr {
		t.Fatalf("linked requirements = %+v", capa.Requirements)
	}
	// DF-1: capability show lists linked constraints, in JSON and markdown.
	if len(capa.Constraints) != 1 || capa.Constraints[0].Identifier != ac || capa.Constraints[0].Compliance == "" {
		t.Fatalf("linked constraints = %+v", capa.Constraints)
	}
	if md := e.mustRun("", "capability", "show", "Crash reporting", "-o", "md").stdout; !strings.Contains(md, "## Constraints") || !strings.Contains(md, ac) {
		t.Errorf("capability markdown lacks the linked constraint %s:\n%s", ac, md)
	}
	// DF-3: linking again is idempotent; an already linked item does not
	// abort the others in the same call.
	fr2 := reqs[len(reqs)-1].Identifier
	capa = decode[ops.CapabilityDetail](t, e.mustRun("", "capability", "link", "Crash reporting", fr, ac, fr2, "-o", "json").stdout)
	if len(capa.Requirements) != 2 || len(capa.Constraints) != 1 {
		t.Fatalf("after relinking: requirements %+v, constraints %+v", capa.Requirements, capa.Constraints)
	}
	e.mustRun("", "capability", "unlink", "Crash reporting", fr2)
	con := decode[ops.ConstraintDetail](t, e.mustRun("", "constraint", "show", ac, "-o", "json").stdout)
	if !hasRef(con.Capabilities, "Crash reporting") {
		t.Fatalf("%s not linked to the capability: %+v", ac, con.Capabilities)
	}
	capa = decode[ops.CapabilityDetail](t, e.mustRun("", "capability", "unlink", "Crash reporting", fr, "-o", "json").stdout)
	if len(capa.Requirements) != 0 {
		t.Fatalf("requirement still linked: %+v", capa.Requirements)
	}
	if r := e.run("", "", "capability", "link", "Crash reporting", "FR-9999"); r.code != cli.ExitNotFound {
		t.Errorf("link to an unknown identifier: exit %d, want %d (%s)", r.code, cli.ExitNotFound, r.stderr)
	}

	// Test cases link to requirements and capabilities (developer role).
	tests := decode[[]ops.TestCaseSummary](t, e.mustRun("", "test", "list", "--automatic", "-o", "json").stdout)
	if len(tests) == 0 {
		t.Fatal("seed has no automatic test cases")
	}
	tc := tests[0].TestIdentifier
	d := decode[ops.TestCaseDetail](t, e.mustAs("cy", "", "test", "link", tc, "--req", fr, "--capability", "Crash reporting", "-o", "json").stdout)
	if d.Capability != "Crash reporting" || !hasCoverage(d.Requirements, fr) {
		t.Fatalf("test case links = %q %+v", d.Capability, d.Requirements)
	}
	d = decode[ops.TestCaseDetail](t, e.mustAs("cy", "", "test", "unlink", tc, "--req", fr, "--capability", "Crash reporting", "-o", "json").stdout)
	if d.Capability != "" || hasCoverage(d.Requirements, fr) {
		t.Fatalf("test case links after unlink = %q %+v", d.Capability, d.Requirements)
	}

	// Constraints: create and record compliance (FR-135).
	c := decode[ops.ConstraintDetail](t, e.mustRun("", "constraint", "create", "--title", "Logs never contain tokens", "--category", "SC", "--compliance", "met", "-o", "json").stdout)
	if !strings.HasPrefix(c.Identifier, "SC-") || c.Compliance != "met" {
		t.Fatalf("constraint = %+v", c)
	}
	c = decode[ops.ConstraintDetail](t, e.mustRun("", "constraint", "update", c.Identifier, "--compliance", "Violated", "--body", "Found one in the sync log.", "-o", "json").stdout)
	if c.Compliance != "violated" || c.Description != "Found one in the sync log." {
		t.Fatalf("updated constraint = %+v", c)
	}
	if r := e.run("", "", "constraint", "update", c.Identifier, "--compliance", "maybe"); r.code != cli.ExitUsage {
		t.Errorf("bad compliance: exit %d, want %d", r.code, cli.ExitUsage)
	}
	if r := e.run("", "", "constraint", "update", c.Identifier); r.code != cli.ExitUsage {
		t.Errorf("update without changes: exit %d, want %d", r.code, cli.ExitUsage)
	}

	// Roles are the server's: a developer cannot change the structure.
	wantForbidden(t, "developer creating a capability", e.as("cy", "", "capability", "create", "--name", "Nope"))
	wantForbidden(t, "developer creating a constraint", e.as("cy", "", "constraint", "create", "--title", "Nope"))

	// Deletions need --yes.
	if r := e.run("", "", "capability", "delete", "Crash reporting"); r.code != cli.ExitUsage || !strings.Contains(r.stderr, "--yes") {
		t.Errorf("delete without --yes: exit %d, stderr %q", r.code, r.stderr)
	}
	e.mustRun("", "capability", "delete", "Crash reporting", "--yes")
	e.mustRun("", "component", "delete", "Telemetry", "--yes")
	st := decode[ops.Structure](t, e.mustRun("", "component", "list", "-o", "json").stdout)
	for _, comp := range st.Components {
		if comp.Name == "Telemetry" {
			t.Error("component still listed after delete")
		}
	}
}

func TestE2EMCPStructureWrites(t *testing.T) {
	e := newAviary(t)
	s := e.startMCP("mcp")

	_, got := mustTool(t, s, "create_component", map[string]any{"name": "Telemetry"})
	if got.(map[string]any)["name"] != "Telemetry" {
		t.Fatalf("create_component = %v", got)
	}
	_, got = mustTool(t, s, "create_capability", map[string]any{"name": "Crash reports", "component": "Telemetry"})
	if got.(map[string]any)["component"] != "Telemetry" {
		t.Fatalf("create_capability = %v", got)
	}
	_, got = mustTool(t, s, "create_constraint", map[string]any{"title": "Logs never contain tokens", "category": "SC"})
	ident := got.(map[string]any)["identifier"].(string)
	_, got = mustTool(t, s, "update_constraint", map[string]any{"ref": ident, "compliance": "met"})
	if got.(map[string]any)["compliance"] != "met" {
		t.Fatalf("update_constraint = %v", got)
	}
	_, got = mustTool(t, s, "link_capability", map[string]any{"capability": "Crash reports", "items": []string{ident}})
	if got.(map[string]any)["name"] != "Crash reports" {
		t.Fatalf("link_capability = %v", got)
	}
	// DF-1: get_capability lists the linked constraint.
	_, got = mustTool(t, s, "get_capability", map[string]any{"ref": "Crash reports"})
	linked, _ := got.(map[string]any)["constraints"].([]any)
	if len(linked) != 1 || linked[0].(map[string]any)["identifier"] != ident {
		t.Fatalf("get_capability constraints = %v", got.(map[string]any)["constraints"])
	}
	con := decode[ops.ConstraintDetail](t, e.mustRun("", "constraint", "show", ident, "-o", "json").stdout)
	if !hasRef(con.Capabilities, "Crash reports") {
		t.Fatalf("link_capability did not link %s: %+v", ident, con.Capabilities)
	}
}

func hasRef(refs []ops.Ref, title string) bool {
	for _, r := range refs {
		if r.Title == title {
			return true
		}
	}
	return false
}

func hasCoverage(refs []ops.CoverageRef, identifier string) bool {
	for _, r := range refs {
		if r.Identifier == identifier {
			return true
		}
	}
	return false
}
