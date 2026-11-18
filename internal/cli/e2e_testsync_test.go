package cli_test

// `nexmap test sync` options (FR-119) and the MCP tool, with package-style
// identifiers such as example.com/aviary/app.TestX.

import (
	"strings"
	"testing"

	"nexmap.net/internal/cli/ops"
)

const packageRun = `{"Action":"run","Package":"example.com/aviary/app","Test":"TestExport_Csv"}
{"Action":"run","Package":"example.com/aviary/app","Test":"TestExport_Csv/empty"}
{"Action":"pass","Package":"example.com/aviary/app","Test":"TestExport_Csv/empty","Elapsed":0}
{"Action":"pass","Package":"example.com/aviary/app","Test":"TestExport_Csv","Elapsed":0}
{"Action":"run","Package":"example.com/aviary/app","Test":"TestExport_Json"}
{"Action":"pass","Package":"example.com/aviary/app","Test":"TestExport_Json","Elapsed":0}
{"Action":"pass","Package":"example.com/aviary/app","Elapsed":0.01}
`

func TestE2ETestSyncOptions(t *testing.T) {
	e := newAviary(t)
	reqs := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "--kind", "requirement", "-o", "json").stdout)
	st := decode[ops.Structure](t, e.mustRun("", "component", "list", "-o", "json").stdout)
	if len(reqs) == 0 || len(st.Components) == 0 {
		t.Fatal("seed lacks requirements or components")
	}
	fr, comp := reqs[0].Identifier, st.Components[0].Name

	args := []string{"test", "sync", "--component", comp, "--pyramid-level", "unit", "--req", fr, "-o", "json"}
	// A bad option creates nothing.
	if r := e.as("cy", packageRun, append(args[:len(args)-2:len(args)-2], "--pyramid-level", "nonsense")...); r.code == 0 {
		t.Fatal("an unknown pyramid level was accepted")
	}
	res := decode[ops.SyncResult](t, e.mustAs("cy", packageRun, args...).stdout)
	want := map[string]bool{"example.com/aviary/app.TestExport_Csv": true, "example.com/aviary/app.TestExport_Json": true}
	if len(res.Created) != 2 {
		t.Fatalf("created %+v; want the two package-qualified tests and no subtest", res.Created)
	}
	for _, tc := range res.Created {
		if !want[tc.TestIdentifier] {
			t.Errorf("unexpected test case %q", tc.TestIdentifier)
		}
		if tc.Component != comp || !strings.EqualFold(tc.PyramidLevel, "unit") {
			t.Errorf("%s filed under %q / %q, want %q / unit", tc.TestIdentifier, tc.Component, tc.PyramidLevel, comp)
		}
	}
	d := decode[ops.TestCaseDetail](t, e.mustRun("", "test", "show", "example.com/aviary/app.TestExport_Csv", "-o", "json").stdout)
	if !hasCoverage(d.Requirements, fr) {
		t.Errorf("synced test case not linked to %s: %+v", fr, d.Requirements)
	}

	// The MCP tool reads a report as well.
	s := e.startMCP("mcp")
	_, got := mustTool(t, s, "sync_test_cases", map[string]any{"report": packageRun, "dry_run": true})
	m := got.(map[string]any)
	if m["existing"].(float64) != 2 || len(m["created"].([]any)) != 0 {
		t.Fatalf("sync_test_cases on a synced run = %v", m)
	}
	_, got = mustTool(t, s, "sync_test_cases", map[string]any{"test_identifiers": []string{"example.com/aviary/app.TestNew"}, "dry_run": true})
	if c := got.(map[string]any)["created"].([]any); len(c) != 1 {
		t.Fatalf("sync_test_cases with identifiers = %v", got)
	}
}
