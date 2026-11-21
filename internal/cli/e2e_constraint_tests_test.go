// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli_test

// Linking test cases to constraints from the CLI and MCP (FR-150): test link
// and unlink with --constraint, test sync --constraint, and constraint show
// and test show listing the other side.

import (
	"strings"
	"testing"

	"wiekko.net/internal/cli/ops"
)

func hasConstraint(refs []ops.Ref, identifier string) bool {
	for _, r := range refs {
		if r.Identifier == identifier {
			return true
		}
	}
	return false
}

func hasEvidence(items []ops.EvidenceView, title string) bool {
	for _, it := range items {
		if it.Title == title && len(it.Via) == 0 {
			return true
		}
	}
	return false
}

func TestE2ETestCaseConstraints(t *testing.T) {
	e := newAviary(t)
	cons := decode[[]ops.ConstraintSummary](t, e.mustRun("", "constraint", "list", "-o", "json").stdout)
	tests := decode[[]ops.TestCaseSummary](t, e.mustRun("", "test", "list", "-o", "json").stdout)
	if len(cons) < 2 || len(tests) == 0 {
		t.Fatal("seed lacks constraints or test cases")
	}
	first, second, tc := cons[0].Identifier, cons[1].Identifier, tests[0]

	wantForbidden(t, "viewer linking", e.as("ada", "", "test", "link", tc.ShortID, "--constraint", first))
	if r := e.run("", "", "test", "link", tc.ShortID, "--constraint", "ZZ-99"); r.code == 0 {
		t.Error("linked an unknown constraint")
	}

	d := decode[ops.TestCaseDetail](t, e.mustAs("cy", "", "test", "link", tc.ShortID, "--constraint", first+","+second, "-o", "json").stdout)
	if !hasConstraint(d.Constraints, first) || !hasConstraint(d.Constraints, second) {
		t.Fatalf("test link --constraint: constraints %+v, want %s and %s", d.Constraints, first, second)
	}
	if md := e.mustRun("", "test", "show", tc.ShortID, "-o", "md").stdout; !strings.Contains(md, "## Verifies constraints") || !strings.Contains(md, first) {
		t.Errorf("test show markdown lacks the constraints:\n%s", md)
	}

	c := decode[ops.ConstraintDetail](t, e.mustRun("", "constraint", "show", first, "-o", "json").stdout)
	if !hasEvidence(c.TestCases, tc.Title) {
		t.Fatalf("constraint show lists %+v, want the linked %q", c.TestCases, tc.Title)
	}
	if md := e.mustRun("", "constraint", "show", first, "-o", "md").stdout; !strings.Contains(md, "## Test cases") || !strings.Contains(md, tc.Title) {
		t.Errorf("constraint show markdown lacks the test cases:\n%s", md)
	}

	d = decode[ops.TestCaseDetail](t, e.mustAs("cy", "", "test", "unlink", tc.ShortID, "--constraint", second, "-o", "json").stdout)
	if hasConstraint(d.Constraints, second) || !hasConstraint(d.Constraints, first) {
		t.Errorf("test unlink --constraint: constraints %+v, want only %s", d.Constraints, first)
	}

	// test sync links new test cases to the constraints.
	run := `{"Action":"run","Package":"example.com/aviary/app","Test":"TestCookieFlags"}
{"Action":"pass","Package":"example.com/aviary/app","Test":"TestCookieFlags","Elapsed":0}
`
	e.mustAs("cy", run, "test", "sync", "--constraint", second, "-o", "json")
	synced := decode[ops.TestCaseDetail](t, e.mustRun("", "test", "show", "example.com/aviary/app.TestCookieFlags", "-o", "json").stdout)
	if !hasConstraint(synced.Constraints, second) {
		t.Errorf("synced test case not linked to %s: %+v", second, synced.Constraints)
	}

	// MCP: the read tools include the links, link_test_case takes constraints.
	s := e.startMCP("mcp")
	_, got := mustTool(t, s, "get_constraint", map[string]any{"ref": second})
	if tcs, _ := got.(map[string]any)["test_cases"].([]any); len(tcs) == 0 {
		t.Errorf("get_constraint has no test cases: %v", got)
	}
	_, got = mustTool(t, s, "link_test_case", map[string]any{"test_case": tc.ShortID, "constraints": []string{second}})
	if refs, _ := got.(map[string]any)["constraints"].([]any); len(refs) != 2 {
		t.Errorf("link_test_case with constraints: %v", got)
	}
	_, got = mustTool(t, s, "get_test_case", map[string]any{"ref": tc.ShortID})
	if refs, _ := got.(map[string]any)["constraints"].([]any); len(refs) != 2 {
		t.Errorf("get_test_case constraints: %v", got)
	}
}
