package cli_test

import (
	"regexp"
	"strings"
	"testing"

	"nexmap.net/internal/cli"
	cliconfig "nexmap.net/internal/cli/config"
	"nexmap.net/internal/cli/ops"
)

// newAviary returns a seeded environment configured for the Aviary product.
func newAviary(t *testing.T) *e2e {
	t.Helper()
	e := newSeededE2E(t)
	e.env[cliconfig.EnvServer] = e.serverURL
	e.env[cliconfig.EnvToken] = e.token
	for _, p := range e.products {
		if p.Name == "Aviary" {
			e.env[cliconfig.EnvProduct] = p.ID
			return e
		}
	}
	t.Fatal("seed has no Aviary product")
	return nil
}

var identifierRE = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[1-9][0-9]*$`)

func findRequirement(t *testing.T, items []ops.RequirementSummary, title string) ops.RequirementSummary {
	t.Helper()
	for _, r := range items {
		if r.Title == title {
			return r
		}
	}
	t.Fatalf("no requirement titled %q", title)
	return ops.RequirementSummary{}
}

func TestE2ERequirements(t *testing.T) {
	e := newAviary(t)

	all := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "-o", "json").stdout)
	if len(all) == 0 {
		t.Fatal("no requirements")
	}
	for _, r := range all {
		if !identifierRE.MatchString(r.Identifier) {
			t.Errorf("%q has identifier %q", r.Title, r.Identifier)
		}
		if r.Status == "" {
			t.Errorf("%s has no resolved status", r.Identifier)
		}
	}
	sighting := findRequirement(t, all, "Record a sighting")

	t.Run("show by identifier", func(t *testing.T) {
		// Identifiers are case-insensitive.
		d := decode[ops.RequirementDetail](t, e.mustRun("", "req", "show", strings.ToLower(sighting.Identifier), "-o", "json").stdout)
		if d.ID != sighting.ID || d.Status != "In Progress" || !d.Active || !d.Coarse {
			t.Fatalf("detail = %+v", d.RequirementSummary)
		}
		if len(d.Children) == 0 {
			t.Error("expected children")
		}
		found := false
		for _, c := range d.Capabilities {
			found = found || c.Title == "Quick Entry"
		}
		if !found {
			t.Errorf("capabilities = %+v, want Quick Entry", d.Capabilities)
		}
		if d.Description == "" || d.Milestone != "1.0 Public Launch" {
			t.Errorf("description %q, milestone %q", d.Description, d.Milestone)
		}
	})

	t.Run("show as markdown", func(t *testing.T) {
		out := e.mustRun("", "req", "show", sighting.Identifier, "-o", "md").stdout
		for _, want := range []string{"# " + sighting.Identifier + " Record a sighting", "## Description", "## Definition of done", "## Children", "Quick Entry"} {
			if !strings.Contains(out, want) {
				t.Errorf("markdown lacks %q:\n%s", want, out)
			}
		}
	})

	t.Run("show unknown", func(t *testing.T) {
		r := e.run("", "", "req", "show", "FR-9999")
		if r.code != cli.ExitNotFound {
			t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
		}
	})

	t.Run("filters", func(t *testing.T) {
		inProgress := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "--status", "in progress", "-o", "json").stdout)
		if len(inProgress) == 0 {
			t.Fatal("no items in progress")
		}
		for _, r := range inProgress {
			if r.Status != "In Progress" {
				t.Errorf("%s status %q", r.Identifier, r.Status)
			}
		}
		search := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "--search", "SIGHTING", "--limit", "1", "-o", "json").stdout)
		if len(search) != 1 || !strings.Contains(strings.ToLower(search[0].Title), "sighting") {
			t.Errorf("search = %+v", search)
		}
		for _, r := range decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "--open", "-o", "json").stdout) {
			if r.StatusClosed {
				t.Errorf("--open returned closed %s", r.Identifier)
			}
		}
		tasks := decode[[]ops.RequirementSummary](t, e.mustRun("", "task", "list", "-o", "json").stdout)
		for _, r := range tasks {
			if r.Kind != "task" {
				t.Errorf("task list returned %s %s", r.Kind, r.Identifier)
			}
		}
		if r := e.run("", "", "req", "list", "--status", "No Such Status"); r.code != cli.ExitUsage || !strings.Contains(r.stderr, "known:") {
			t.Errorf("unknown status: exit %d, stderr %q", r.code, r.stderr)
		}
	})

	t.Run("task show rejects requirements", func(t *testing.T) {
		if r := e.run("", "", "task", "show", sighting.Identifier); r.code != cli.ExitNotFound {
			t.Errorf("exit %d", r.code)
		}
	})

	t.Run("api accepts identifiers", func(t *testing.T) {
		out := e.mustRun("", "api", "/products/{product}/requirements/"+sighting.Identifier).stdout
		if !strings.Contains(out, `"identifier": "`+sighting.Identifier+`"`) {
			t.Errorf("api output lacks identifier: %s", out)
		}
	})
}

func TestE2EConstraintsAndStructure(t *testing.T) {
	e := newAviary(t)

	cons := decode[[]ops.ConstraintSummary](t, e.mustRun("", "constraint", "list", "-o", "json").stdout)
	if len(cons) == 0 {
		t.Fatal("no constraints")
	}
	d := decode[ops.ConstraintDetail](t, e.mustRun("", "constraint", "show", cons[0].Identifier, "-o", "json").stdout)
	if d.ID != cons[0].ID || d.Compliance == "" || !identifierRE.MatchString(d.Identifier) {
		t.Fatalf("constraint = %+v", d)
	}
	// A requirement identifier is not a constraint.
	reqs := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "--limit", "1", "-o", "json").stdout)
	if r := e.run("", "", "constraint", "show", reqs[0].Identifier); r.code != cli.ExitNotFound {
		t.Errorf("requirement as constraint: exit %d", r.code)
	}

	tree := e.mustRun("", "component", "list", "-o", "md").stdout
	if !strings.Contains(tree, "**Field Logging**") || !strings.Contains(tree, "Quick Entry") {
		t.Errorf("structure:\n%s", tree)
	}

	cp := decode[ops.CapabilityDetail](t, e.mustRun("", "capability", "show", "quick", "entry", "-o", "json").stdout)
	if cp.Name != "Quick Entry" || cp.Component != "Field Logging" {
		t.Fatalf("capability = %+v", cp.CapabilityView)
	}
	found := false
	for _, r := range cp.Requirements {
		found = found || r.Title == "Record a sighting"
	}
	if !found {
		t.Errorf("Quick Entry requirements = %+v", cp.Requirements)
	}
}

func TestE2EDefectsTestsCampaign(t *testing.T) {
	e := newAviary(t)

	defects := decode[[]ops.DefectSummary](t, e.mustRun("", "defect", "list", "-o", "json").stdout)
	if len(defects) == 0 {
		t.Fatal("no defects")
	}
	rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
	for i := 1; i < len(defects); i++ {
		if rank[defects[i-1].Criticality] > rank[defects[i].Criticality] {
			t.Errorf("defects not sorted by criticality: %s before %s", defects[i-1].Criticality, defects[i].Criticality)
		}
	}
	d := decode[ops.DefectDetail](t, e.mustRun("", "defect", "show", defects[0].ShortID, "-o", "json").stdout)
	if d.ID != defects[0].ID || d.Status == "" {
		t.Fatalf("defect = %+v", d.DefectSummary)
	}
	if r := e.run("", "", "defect", "show", "zzzzzzzz"); r.code != cli.ExitNotFound {
		t.Errorf("unknown defect: exit %d", r.code)
	}

	auto := decode[[]ops.TestCaseSummary](t, e.mustRun("", "test", "list", "--automatic", "-o", "json").stdout)
	var withIdent ops.TestCaseSummary
	for _, tc := range auto {
		if tc.Mode != "automatic" {
			t.Errorf("--automatic returned %s", tc.Mode)
		}
		if tc.TestIdentifier != "" && withIdent.ID == "" {
			withIdent = tc
		}
	}
	if withIdent.ID == "" {
		t.Fatal("no automatic test case with identifier")
	}
	tc := decode[ops.TestCaseDetail](t, e.mustRun("", "test", "show", withIdent.TestIdentifier, "-o", "json").stdout)
	if tc.ID != withIdent.ID {
		t.Fatalf("test show by identifier = %+v", tc.TestCaseSummary)
	}

	cs := decode[ops.CampaignStatusView](t, e.mustRun("", "campaign", "status", "-o", "json").stdout)
	if cs.Campaign != nil {
		if cs.Campaign.Status != "active" || cs.Campaign.Total == 0 || cs.Campaign.Verdict == "" {
			t.Errorf("campaign = %+v", cs.Campaign)
		}
	} else if cs.Message == "" {
		t.Error("no campaign and no message")
	}
}

func TestE2EContext(t *testing.T) {
	e := newAviary(t)

	v := decode[ops.ContextView](t, e.mustRun("", "context", "-o", "json").stdout)
	if v.Product.Name != "Aviary" || len(v.Workflow) == 0 || len(v.Structure.Components) == 0 {
		t.Fatalf("context = %+v", v.Product)
	}
	if v.ActiveSprint == nil || v.ActiveSprint.Name != "Sprint 13" {
		t.Errorf("active sprint = %+v", v.ActiveSprint)
	}
	if len(v.Constraints) == 0 || v.Backlog.OpenRequirements == 0 {
		t.Errorf("constraints %d, open requirements %d", len(v.Constraints), v.Backlog.OpenRequirements)
	}
	for _, r := range v.InProgress {
		if !r.Active || r.StatusClosed {
			t.Errorf("in progress contains %+v", r)
		}
	}

	md := e.mustRun("", "context").stdout // default format renders markdown
	for _, want := range []string{"# Product: Aviary", "## Structure", "## Constraints", "## Open defects", "## Next steps"} {
		if !strings.Contains(md, want) {
			t.Errorf("context markdown lacks %q", want)
		}
	}

	guide := e.mustRun("", "agent-guide").stdout
	if !strings.Contains(guide, "nexmap context") || !strings.Contains(guide, "nexmap req show") {
		t.Errorf("agent guide:\n%s", guide)
	}
}

// TestE2ECrossProductIDs checks product isolation through the real router: a
// Finch Tracker requirement must not be reachable under Aviary's URL, neither
// by ID nor by identifier.
func TestE2ECrossProductIDs(t *testing.T) {
	e := newAviary(t)
	var finchID string
	for _, p := range e.products {
		if p.Name == "Finch Tracker" {
			finchID = p.ID
		}
	}
	finchReqs := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "--product", finchID, "--limit", "1", "-o", "json").stdout)
	if len(finchReqs) == 0 {
		t.Fatal("no Finch requirements")
	}
	r := e.run("", "", "api", "/products/{product}/requirements/"+finchReqs[0].ID)
	if r.code != cli.ExitNotFound {
		t.Fatalf("foreign requirement by ID: exit %d, stdout %q", r.code, r.stdout)
	}
	r = e.run("", "", "api", "PATCH", "/products/{product}/requirements/"+finchReqs[0].ID, "--data", `{"title":"hijacked"}`)
	if r.code != cli.ExitNotFound {
		t.Fatalf("foreign requirement PATCH: exit %d", r.code)
	}
	after := e.mustRun("", "req", "show", finchReqs[0].Identifier, "--product", finchID, "-o", "json").stdout
	if strings.Contains(after, "hijacked") {
		t.Fatal("foreign requirement was modified")
	}
}

func TestE2EReadNeedsProduct(t *testing.T) {
	e := newAviary(t)
	delete(e.env, cliconfig.EnvProduct)
	if r := e.run("", "", "req", "list"); r.code != cli.ExitUsage || !strings.Contains(r.stderr, "no product configured") {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if r := e.run("", "", "req"); r.code != cli.ExitUsage || !strings.Contains(r.stderr, "Commands:") {
		t.Fatalf("bare group: exit %d, stderr %q", r.code, r.stderr)
	}
	if r := e.run("", "", "req", "frob"); r.code != cli.ExitUsage {
		t.Fatalf("unknown sub: exit %d", r.code)
	}
}
