package cli_test

// Agent writes end to end, with real tokens of the demo users. In Aviary,
// ada is a viewer, bo a tester and cy a developer; root is an administrator
// and acts with owner rights. The CLI never checks roles itself, so these
// tests verify that the server's rules come through unchanged.

import (
	"reflect"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"nexmap.net/internal/cli"
	cliconfig "nexmap.net/internal/cli/config"
	"nexmap.net/internal/cli/ops"
)

// as runs the CLI with another user's token.
func (e *e2e) as(user, stdin string, args ...string) result {
	e.t.Helper()
	saved := e.env[cliconfig.EnvToken]
	defer func() { e.env[cliconfig.EnvToken] = saved }()
	if user != "root" {
		tok, ok := e.tokens[user]
		if !ok {
			e.t.Fatalf("no token for %s", user)
		}
		e.env[cliconfig.EnvToken] = tok
	}
	return e.run(stdin, "", args...)
}

func (e *e2e) mustAs(user, stdin string, args ...string) result {
	e.t.Helper()
	r := e.as(user, stdin, args...)
	if r.code != cli.ExitOK {
		e.t.Fatalf("%s: nexmap %s: exit %d\nstdout: %s\nstderr: %s", user, strings.Join(args, " "), r.code, r.stdout, r.stderr)
	}
	return r
}

// wantForbidden asserts the server refused the write for lack of a role.
func wantForbidden(t *testing.T, what string, r result) {
	t.Helper()
	if r.code != cli.ExitAPI || !strings.Contains(r.stderr, "role") {
		t.Errorf("%s: exit %d, stderr %q; want a role refusal (exit %d)", what, r.code, r.stderr, cli.ExitAPI)
	}
}

func TestE2ERequirementStatus(t *testing.T) {
	e := newAviary(t)
	reqs := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "-o", "json").stdout)
	item := findRequirement(t, reqs, "Record a sighting")
	if item.Status == "In Review" {
		t.Fatal("pick an item that is not In Review")
	}

	wantForbidden(t, "viewer", e.as("ada", "", "req", "status", item.Identifier, "In", "Review"))
	wantForbidden(t, "tester", e.as("bo", "", "req", "status", item.Identifier, "In", "Review"))

	// Status names may be given unquoted and in any case.
	d := decode[ops.RequirementDetail](t, e.mustAs("cy", "", "req", "status", item.Identifier, "in", "review", "-o", "json").stdout)
	if d.Status != "In Review" || d.ID != item.ID {
		t.Fatalf("after status change: %+v", d.RequirementSummary)
	}

	r := e.as("cy", "", "req", "status", item.Identifier, "Shipped")
	if r.code != cli.ExitUsage || !strings.Contains(r.stderr, "In Progress") {
		t.Errorf("unknown status: exit %d, stderr %q; want the known statuses", r.code, r.stderr)
	}
	if r := e.as("cy", "", "task", "status", item.Identifier, "Done"); r.code != cli.ExitNotFound {
		t.Errorf("task status on a requirement: exit %d, want %d", r.code, cli.ExitNotFound)
	}
	if r := e.as("cy", "", "req", "status", item.Identifier); r.code != cli.ExitUsage {
		t.Errorf("missing status: exit %d", r.code)
	}
}

func TestE2ECreateTask(t *testing.T) {
	e := newAviary(t)
	reqs := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "-o", "json").stdout)
	parent := findRequirement(t, reqs, "Record a sighting")

	args := []string{"task", "create", "--title", "Persist draft sightings", "--parent", parent.Identifier, "--body-file", "-", "--dod", "Drafts survive an app restart", "-o", "json"}
	wantForbidden(t, "developer", e.as("cy", "Body", args...))

	d := decode[ops.RequirementDetail](t, e.mustAs("root", "Keep unsent sightings across restarts.\n", args...).stdout)
	if d.Kind != "task" || !identifierRE.MatchString(d.Identifier) || d.Title != "Persist draft sightings" {
		t.Fatalf("created: %+v", d.RequirementSummary)
	}
	if d.Parent == nil || d.Parent.ID != parent.ID {
		t.Errorf("parent = %+v, want %s", d.Parent, parent.Identifier)
	}
	if d.Description != "Keep unsent sightings across restarts.\n" || d.DefinitionOfDone != "Drafts survive an app restart" {
		t.Errorf("texts: %q / %q", d.Description, d.DefinitionOfDone)
	}
	if d.Status != "New" {
		t.Errorf("status = %q, want the default", d.Status)
	}

	// The new task is a child of its parent and readable by identifier.
	p := decode[ops.RequirementDetail](t, e.mustRun("", "req", "show", parent.Identifier, "-o", "json").stdout)
	found := false
	for _, c := range p.Children {
		found = found || c.ID == d.ID
	}
	if !found {
		t.Error("new task is not listed as a child of its parent")
	}

	// A category of the right kind is accepted by name; bad input creates nothing.
	r := e.mustAs("root", "", "req", "create", "--kind", "task", "--title", "Second", "--category", d.Category, "--status", "In Progress", "-o", "json")
	second := decode[ops.RequirementDetail](t, r.stdout)
	if second.Category != d.Category || second.Status != "In Progress" {
		t.Errorf("second task: %+v", second.RequirementSummary)
	}
	before := len(decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "-o", "json").stdout))
	for _, bad := range [][]string{
		{"task", "create", "--title", "x", "--category", "NOPE"},
		{"task", "create", "--title", "x", "--parent", "FR-9999"},
		{"task", "create", "--title", "x", "--status", "Shipped"},
		{"req", "create", "--kind", "epic", "--title", "x"},
		{"task", "create"},
	} {
		if r := e.as("root", "", bad...); r.code == cli.ExitOK {
			t.Errorf("%v succeeded", bad)
		}
	}
	if after := len(decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "-o", "json").stdout)); after != before {
		t.Errorf("failed creates changed the backlog: %d → %d items", before, after)
	}
}

func TestE2EDefectWrites(t *testing.T) {
	e := newAviary(t)
	reqs := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "-o", "json").stdout)
	req := findRequirement(t, reqs, "Record a sighting")
	const testID = "TestLocalStore_SightingRoundTrip"
	defectCount := func() int {
		return len(decode[[]ops.DefectSummary](t, e.mustRun("", "defect", "list", "-o", "json").stdout))
	}
	before := defectCount()

	args := []string{"defect", "create", "--title", "Round trip drops the note", "--criticality", "high",
		"--test", testID, "--req", req.Identifier, "--body", "Notes longer than 255 characters are cut.", "-o", "json"}
	wantForbidden(t, "viewer", e.as("ada", "", args...))

	d := decode[ops.DefectDetail](t, e.mustAs("bo", "", args...).stdout)
	if d.Criticality != "high" || d.Status != "New" || d.Description == "" {
		t.Fatalf("created: %+v", d)
	}
	var linkedTest, linkedReq bool
	for _, l := range d.Links {
		linkedTest = linkedTest || l.Type == "test_case"
		linkedReq = linkedReq || l.Type == "requirement" && strings.Contains(l.Label, req.Identifier)
	}
	if !linkedTest || !linkedReq {
		t.Errorf("links = %+v; want the test case and %s", d.Links, req.Identifier)
	}

	// A bad link target is caught before anything is created.
	for _, bad := range [][]string{
		{"defect", "create", "--title", "x", "--test", "TestNoSuchThing"},
		{"defect", "create", "--title", "x", "--req", "FR-9999"},
		{"defect", "create", "--title", "x", "--criticality", "catastrophic"},
	} {
		if r := e.as("bo", "", bad...); r.code == cli.ExitOK {
			t.Errorf("%v succeeded", bad)
		}
	}
	if got := defectCount(); got != before+1 {
		t.Errorf("defects: %d → %d, want exactly one new", before, got)
	}

	// Testers report defects, developers move them through the workflow.
	wantForbidden(t, "tester", e.as("bo", "", "defect", "status", d.ShortID, "In Progress"))
	moved := decode[ops.DefectDetail](t, e.mustAs("cy", "", "defect", "status", d.ShortID, "Done", "-o", "json").stdout)
	if moved.Status != "Done" || !moved.StatusClosed {
		t.Errorf("after status change: %s (closed %v)", moved.Status, moved.StatusClosed)
	}
}

func TestE2ECommentAdd(t *testing.T) {
	e := newAviary(t)
	reqs := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "-o", "json").stdout)
	req := findRequirement(t, reqs, "Record a sighting")
	defects := decode[[]ops.DefectSummary](t, e.mustRun("", "defect", "list", "-o", "json").stdout)
	if len(defects) == 0 {
		t.Fatal("seed has no defects")
	}
	defect := defects[0]

	// Requirements need developers; defects and test cases accept testers.
	wantForbidden(t, "tester on a requirement", e.as("bo", "", "comment", "add", req.Identifier, "--body", "hi"))
	wantForbidden(t, "viewer on a defect", e.as("ada", "", "comment", "add", defect.ShortID, "--type", "defect", "--body", "hi"))

	c := decode[ops.CommentResult](t, e.mustAs("cy", "Decided: store drafts locally.\n", "comment", "add", strings.ToLower(req.Identifier), "--body-file", "-", "-o", "json").stdout)
	if c.OwnerType != "requirement" || c.Owner.ID != req.ID || c.Comment.Author != "cy" {
		t.Fatalf("comment: %+v", c)
	}
	shown := decode[ops.RequirementDetail](t, e.mustRun("", "req", "show", req.Identifier, "-o", "json").stdout)
	if n := len(shown.Comments); n == 0 || !strings.Contains(shown.Comments[n-1].Body, "store drafts locally") {
		t.Errorf("comment not visible on %s: %+v", req.Identifier, shown.Comments)
	}

	e.mustAs("bo", "", "comment", "add", defect.ShortID, "--type", "defect", "--body", "Still happens on 1.2")
	e.mustAs("bo", "", "comment", "add", "TestLocalStore_SightingRoundTrip", "--type", "test_case", "--body", "Flaky on CI")

	for _, bad := range []struct {
		args []string
		code int
	}{
		{[]string{"comment", "add", defect.ShortID, "--body", "x"}, cli.ExitUsage}, // not an identifier, no type
		{[]string{"comment", "add", "FR-9999", "--body", "x"}, cli.ExitNotFound},
		{[]string{"comment", "add", req.Identifier}, cli.ExitUsage}, // empty body
		{[]string{"comment", "add", req.Identifier, "--type", "sprint", "--body", "x"}, cli.ExitUsage},
	} {
		if r := e.as("cy", "", bad.args...); r.code != bad.code {
			t.Errorf("%v: exit %d, want %d (stderr %q)", bad.args, r.code, bad.code, r.stderr)
		}
	}
}

func TestE2EMCPWrites(t *testing.T) {
	e := newAviary(t)
	reqs := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "-o", "json").stdout)
	item := findRequirement(t, reqs, "Record a sighting")

	e.env[cliconfig.EnvToken] = e.tokens["cy"]
	s := e.startMCP("mcp")
	_, got := mustTool(t, s, "set_requirement_status", map[string]any{"ref": item.Identifier, "status": "In Review"})
	if got.(map[string]any)["status"] != "In Review" {
		t.Fatalf("set_requirement_status result: %v", got)
	}
	cliView := generic(t, decode[any](t, e.mustRun("", "req", "show", item.Identifier, "-o", "json").stdout))
	_, again := mustTool(t, s, "get_requirement", map[string]any{"ref": item.Identifier})
	if !reflect.DeepEqual(again, cliView) {
		t.Error("MCP and CLI disagree after the write")
	}
	text, _ := mustTool(t, s, "add_comment", map[string]any{"ref": item.Identifier, "body": "Ready for review."})
	if !strings.Contains(text, item.Identifier) {
		t.Errorf("add_comment text: %q", text)
	}
	// Owner-only: the developer's token is refused, and the model is told why.
	res := callTool(t, s, "create_backlog_item", map[string]any{"title": "x"})
	if !res.IsError || !strings.Contains(toolText(res), "role") {
		t.Errorf("create_backlog_item as developer: isError=%v, %q", res.IsError, toolText(res))
	}

	// --read-only does not offer write tools at all.
	ro := e.startMCP("mcp", "--read-only")
	if res, err := ro.CallTool(t.Context(), &sdk.CallToolParams{Name: "set_requirement_status", Arguments: map[string]any{"ref": item.Identifier, "status": "Done"}}); err == nil && !res.IsError {
		t.Error("read-only server ran a write tool")
	}
	after := decode[ops.RequirementDetail](t, e.mustRun("", "req", "show", item.Identifier, "-o", "json").stdout)
	if after.Status != "In Review" {
		t.Errorf("status changed through the read-only server: %s", after.Status)
	}
}
