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

// TestE2ECreateRequirementWithCapabilities links a new requirement to
// capabilities as it is created (FR-156); tasks reach capabilities through
// their parent and take none.
func TestE2ECreateRequirementWithCapabilities(t *testing.T) {
	e := newAviary(t)
	e.mustRun("", "capability", "create", "--name", "Crash reports", "-o", "json")
	e.mustRun("", "capability", "create", "--name", "Usage reports", "-o", "json")

	d := decode[ops.RequirementDetail](t, e.mustRun("", "req", "create", "--kind", "requirement", "--title", "Report crashes",
		"--capability", "Crash reports", "--capability", "usage reports", "-o", "json").stdout)
	got := map[string]bool{}
	for _, c := range d.Capabilities {
		got[c.Title] = true
	}
	if len(d.Capabilities) != 2 || !got["Crash reports"] || !got["Usage reports"] {
		t.Errorf("capabilities = %+v, want Crash reports and Usage reports", d.Capabilities)
	}

	before := len(decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "-o", "json").stdout))
	for _, bad := range [][]string{
		{"req", "create", "--kind", "requirement", "--title", "x", "--capability", "No such capability"},
		{"req", "create", "--kind", "task", "--title", "x", "--capability", "Crash reports"},
		{"task", "create", "--title", "x", "--capability", "Crash reports"},
	} {
		if r := e.as("root", "", bad...); r.code == cli.ExitOK {
			t.Errorf("%v succeeded", bad)
		}
	}
	if after := len(decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "-o", "json").stdout)); after != before {
		t.Errorf("failed creates changed the backlog: %d → %d items", before, after)
	}
}

// TestE2EMoveBacklogItem moves items below others and back to the top level
// (FR-152).
func TestE2EMoveBacklogItem(t *testing.T) {
	e := newAviary(t)
	reqs := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "-o", "json").stdout)
	parent := findRequirement(t, reqs, "Record a sighting")
	item := decode[ops.RequirementDetail](t, e.mustAs("root", "", "req", "create", "--kind", "requirement", "--title", "Moved around", "-o", "json").stdout)
	task := decode[ops.RequirementDetail](t, e.mustAs("root", "", "task", "create", "--title", "Some task", "-o", "json").stdout)

	wantForbidden(t, "developer", e.as("cy", "", "req", "update", item.Identifier, "--parent", parent.Identifier))

	d := decode[ops.RequirementDetail](t, e.mustAs("root", "", "req", "update", item.Identifier, "--parent", parent.Identifier, "-o", "json").stdout)
	if d.Parent == nil || d.Parent.ID != parent.ID {
		t.Fatalf("parent = %+v, want %s", d.Parent, parent.Identifier)
	}
	d = decode[ops.RequirementDetail](t, e.mustAs("root", "", "task", "update", task.Identifier, "--parent", item.Identifier, "-o", "json").stdout)
	if d.Parent == nil || d.Parent.ID != item.ID {
		t.Errorf("task parent = %+v, want %s", d.Parent, item.Identifier)
	}

	// The server's checks come through; a refused move changes nothing.
	for _, bad := range []struct {
		args []string
		code int
	}{
		{[]string{"req", "update", parent.Identifier, "--parent", item.Identifier}, cli.ExitAPI},       // cycle
		{[]string{"req", "update", item.Identifier, "--parent", task.Identifier}, cli.ExitAPI},         // requirement below a task
		{[]string{"req", "update", item.Identifier, "--parent", "FR-9999"}, cli.ExitNotFound},          // unknown parent
		{[]string{"task", "update", item.Identifier, "--parent", parent.Identifier}, cli.ExitNotFound}, // not a task
		{[]string{"req", "update", item.Identifier}, cli.ExitUsage},                                    // nothing to change
	} {
		if r := e.as("root", "", bad.args...); r.code != bad.code {
			t.Errorf("%v: exit %d, want %d\nstderr: %s", bad.args, r.code, bad.code, r.stderr)
		}
	}
	if p := decode[ops.RequirementDetail](t, e.mustRun("", "req", "show", parent.Identifier, "-o", "json").stdout); p.Parent != nil {
		t.Errorf("refused cycle moved %s below %s", parent.Identifier, p.Parent.Identifier)
	}

	d = decode[ops.RequirementDetail](t, e.mustAs("root", "", "req", "update", item.Identifier, "--parent", "", "-o", "json").stdout)
	if d.Parent != nil {
		t.Errorf("after moving to the top level: parent = %+v", d.Parent)
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
	wantForbidden(t, "tester", e.as("bo", "", "defect", "status", d.Identifier, "In Progress"))
	// FR-116: only once the defect has been activated.
	wantForbidden(t, "developer on an inactive defect", e.as("cy", "", "defect", "status", d.Identifier, "Done"))
	e.mustRun("", "api", "PATCH", "/products/{product}/defects/"+d.ID, "--data", `{"is_active":true}`)
	moved := decode[ops.DefectDetail](t, e.mustAs("cy", "", "defect", "status", d.Identifier, "Done", "-o", "json").stdout)
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
	wantForbidden(t, "viewer on a defect", e.as("ada", "", "comment", "add", defect.Identifier, "--type", "defect", "--body", "hi"))

	c := decode[ops.CommentResult](t, e.mustAs("cy", "Decided: store drafts locally.\n", "comment", "add", strings.ToLower(req.Identifier), "--body-file", "-", "-o", "json").stdout)
	if c.OwnerType != "requirement" || c.Owner.ID != req.ID || c.Comment.Author != "cy" {
		t.Fatalf("comment: %+v", c)
	}
	shown := decode[ops.RequirementDetail](t, e.mustRun("", "req", "show", req.Identifier, "-o", "json").stdout)
	if n := len(shown.Comments); n == 0 || !strings.Contains(shown.Comments[n-1].Body, "store drafts locally") {
		t.Errorf("comment not visible on %s: %+v", req.Identifier, shown.Comments)
	}

	e.mustAs("bo", "", "comment", "add", defect.Identifier, "--type", "defect", "--body", "Still happens on 1.2")
	e.mustAs("bo", "", "comment", "add", "TestLocalStore_SightingRoundTrip", "--type", "test_case", "--body", "Flaky on CI")

	for _, bad := range []struct {
		args []string
		code int
	}{
		{[]string{"comment", "add", ops.ShortID(defect.ID), "--body", "x"}, cli.ExitUsage}, // not an identifier, no type
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

	rootToken := e.env[cliconfig.EnvToken]
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
	res = callTool(t, s, "update_backlog_item", map[string]any{"ref": item.Identifier, "parent": ""})
	if !res.IsError || !strings.Contains(toolText(res), "role") {
		t.Errorf("update_backlog_item as developer: isError=%v, %q", res.IsError, toolText(res))
	}

	// An owner moves items below others and back (FR-152); the server's
	// checks reach the model as tool errors.
	e.env[cliconfig.EnvToken] = rootToken
	owner := e.startMCP("mcp")
	_, created := mustTool(t, owner, "create_backlog_item", map[string]any{"title": "Moved by an agent"})
	task := created.(map[string]any)["identifier"].(string)
	_, moved := mustTool(t, owner, "update_backlog_item", map[string]any{"ref": task, "parent": item.Identifier})
	if p, _ := moved.(map[string]any)["parent"].(map[string]any); p["identifier"] != item.Identifier {
		t.Errorf("update_backlog_item parent: %v", moved)
	}
	if res := callTool(t, owner, "update_backlog_item", map[string]any{"ref": item.Identifier, "parent": task}); !res.IsError {
		t.Errorf("requirement moved below a task: %q", toolText(res))
	}
	_, top := mustTool(t, owner, "update_backlog_item", map[string]any{"ref": task, "parent": ""})
	if _, has := top.(map[string]any)["parent"]; has {
		t.Errorf("after moving to the top level: %v", top)
	}
	e.env[cliconfig.EnvToken] = e.tokens["cy"]

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
