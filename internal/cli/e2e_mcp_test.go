package cli_test

// `nexmap mcp` end to end: the command runs in-process through cli.Run with
// piped stdin/stdout, and a real MCP client from the SDK talks to it.

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"nexmap.net/internal/cli"
	cliconfig "nexmap.net/internal/cli/config"
	"nexmap.net/internal/cli/ops"
)

// startMCP runs `nexmap <args>` (an mcp command) and connects a client to it.
// Closing the session ends the server, whose exit code is checked on cleanup.
func (e *e2e) startMCP(args ...string) *sdk.ClientSession {
	t := e.t
	t.Helper()
	inR, inW := io.Pipe()   // client → server
	outR, outW := io.Pipe() // server → client
	var stderr strings.Builder
	done := make(chan int, 1)
	go func() {
		code := cli.Run(context.Background(), args, cli.IO{
			Stdin:  inR,
			Stdout: outW,
			Stderr: &stderr,
			Getenv: func(k string) string { return e.env[k] },
			Cwd:    e.repoDir,
		})
		outW.Close()
		done <- code
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := sdk.NewClient(&sdk.Implementation{Name: "e2e", Version: "test"}, nil)
	session, err := c.Connect(ctx, &sdk.IOTransport{Reader: outR, Writer: inW}, nil)
	if err != nil {
		t.Fatalf("connect to nexmap mcp: %v (stderr %q)", err, stderr.String())
	}
	t.Cleanup(func() {
		_ = session.Close()
		select {
		case code := <-done:
			if code != cli.ExitOK {
				t.Errorf("nexmap mcp exited %d, stderr %q", code, stderr.String())
			}
		case <-time.After(10 * time.Second):
			t.Error("nexmap mcp did not stop after the client disconnected")
		}
	})
	return session
}

func callTool(t *testing.T, s *sdk.ClientSession, name string, args any) *sdk.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := s.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error: %v", name, err)
	}
	return res
}

func toolText(res *sdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// mustTool calls a tool that must succeed and returns its markdown text and
// structured content (re-decoded into generic JSON).
func mustTool(t *testing.T, s *sdk.ClientSession, name string, args any) (string, any) {
	t.Helper()
	res := callTool(t, s, name, args)
	if res.IsError {
		t.Fatalf("%s(%v): tool error: %s", name, args, toolText(res))
	}
	return toolText(res), generic(t, res.StructuredContent)
}

// generic round-trips v through JSON so values of different Go types compare.
func generic(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

var readTools = []string{
	"campaign_status", "get_capability", "get_constraint", "get_context", "get_defect", "get_requirement",
	"get_structure", "get_test_case", "list_constraints", "list_defects", "list_requirements", "list_test_cases",
}

var writeTools = []string{"add_comment", "create_backlog_item", "create_defect", "set_defect_status", "set_requirement_status"}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func TestE2EMCPTools(t *testing.T) {
	e := newAviary(t)
	for _, args := range [][]string{{"mcp"}, {"mcp", "--read-only"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			s := e.startMCP(args...)
			list, err := s.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			readOnly := len(args) > 1
			want := append([]string{}, readTools...)
			if !readOnly {
				want = append(want, writeTools...)
			}
			sort.Strings(want)
			var names []string
			for _, tool := range list.Tools {
				names = append(names, tool.Name)
				isWrite := contains(writeTools, tool.Name)
				if tool.Annotations == nil || tool.Annotations.ReadOnlyHint == isWrite {
					t.Errorf("%s: read-only annotation wrong (write tool: %v)", tool.Name, isWrite)
				}
				if isWrite && (tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint) {
					t.Errorf("%s: write tools only add or update, so they must not be marked destructive", tool.Name)
				}
				if tool.Description == "" {
					t.Errorf("%s has no description", tool.Name)
				}
			}
			sort.Strings(names)
			if !reflect.DeepEqual(names, want) {
				t.Errorf("tools = %v, want %v", names, want)
			}
			if init := s.InitializeResult(); init == nil || !strings.Contains(init.Instructions, "get_context") {
				t.Error("server instructions do not explain the tools")
			}
		})
	}
}

func TestE2EMCPMatchesCLI(t *testing.T) {
	e := newAviary(t)
	s := e.startMCP("mcp", "--read-only")

	cliJSON := func(args ...string) any {
		t.Helper()
		return generic(t, decode[any](t, e.mustRun("", append(args, "-o", "json")...).stdout))
	}
	same := func(what string, got, want any) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			gb, _ := json.MarshalIndent(got, "", " ")
			wb, _ := json.MarshalIndent(want, "", " ")
			t.Errorf("%s: MCP and CLI differ\nMCP: %s\nCLI: %s", what, gb, wb)
		}
	}
	items := func(v any) any { return v.(map[string]any)["items"] }

	reqs := decode[[]ops.RequirementSummary](t, e.mustRun("", "req", "list", "-o", "json").stdout)
	sighting := findRequirement(t, reqs, "Record a sighting")

	t.Run("get_context", func(t *testing.T) {
		text, got := mustTool(t, s, "get_context", map[string]any{})
		same("context", got, cliJSON("context"))
		if !strings.Contains(text, "Aviary") || !strings.HasPrefix(text, "#") {
			t.Errorf("context text is not the markdown briefing: %.200q", text)
		}
	})
	t.Run("get_requirement", func(t *testing.T) {
		text, got := mustTool(t, s, "get_requirement", map[string]any{"ref": strings.ToLower(sighting.Identifier)})
		same("requirement", got, cliJSON("req", "show", sighting.Identifier))
		if !strings.Contains(text, sighting.Identifier) || !strings.Contains(text, "Record a sighting") {
			t.Errorf("requirement text: %.200q", text)
		}
	})
	t.Run("list_requirements", func(t *testing.T) {
		_, got := mustTool(t, s, "list_requirements", map[string]any{"kind": "task", "open": true})
		same("tasks", items(got), cliJSON("req", "list", "--kind", "task", "--open"))
		_, got = mustTool(t, s, "list_requirements", nil)
		same("all requirements", items(got), cliJSON("req", "list"))
	})
	t.Run("other reads", func(t *testing.T) {
		_, got := mustTool(t, s, "list_constraints", map[string]any{})
		same("constraints", items(got), cliJSON("constraint", "list"))
		_, got = mustTool(t, s, "get_structure", map[string]any{})
		same("structure", got, cliJSON("component", "list"))
		_, got = mustTool(t, s, "list_defects", map[string]any{"limit": 3})
		same("defects", items(got), cliJSON("defect", "list", "--limit", "3"))
		_, got = mustTool(t, s, "list_test_cases", map[string]any{})
		same("test cases", items(got), cliJSON("test", "list"))
		_, got = mustTool(t, s, "campaign_status", map[string]any{})
		same("campaign", got, cliJSON("campaign", "status"))

		defects := decode[[]ops.DefectSummary](t, e.mustRun("", "defect", "list", "-o", "json").stdout)
		if len(defects) == 0 {
			t.Fatal("seed has no defects")
		}
		_, got = mustTool(t, s, "get_defect", map[string]any{"ref": defects[0].ShortID})
		same("defect", got, cliJSON("defect", "show", defects[0].ShortID))
	})
}

func TestE2EMCPErrors(t *testing.T) {
	e := newAviary(t)
	s := e.startMCP("mcp")

	for _, tc := range []struct {
		tool string
		args any
		want string
	}{
		{"get_requirement", map[string]any{"ref": "FR-9999"}, "FR-9999"},
		{"get_requirement", map[string]any{"ref": ""}, `"ref" is required`},
		{"get_requirement", map[string]any{}, "ref"},
		{"list_requirements", map[string]any{"status": "No Such Status"}, "No Such Status"},
		{"list_requirements", map[string]any{"bogus": true}, "bogus"},
	} {
		res := callTool(t, s, tc.tool, tc.args)
		if !res.IsError || !strings.Contains(toolText(res), tc.want) {
			t.Errorf("%s(%v): isError=%v, text %q; want an error mentioning %q", tc.tool, tc.args, res.IsError, toolText(res), tc.want)
		}
	}
	// The server keeps serving after tool errors.
	mustTool(t, s, "get_structure", map[string]any{})
}

// Without credentials the server still starts, so the agent sees why each
// call fails instead of a dead server.
func TestE2EMCPNotLoggedIn(t *testing.T) {
	e := newAviary(t)
	delete(e.env, cliconfig.EnvToken)
	s := e.startMCP("mcp")
	res := callTool(t, s, "get_context", map[string]any{})
	if !res.IsError || !strings.Contains(toolText(res), "nexmap login") {
		t.Fatalf("isError=%v, text %q", res.IsError, toolText(res))
	}

	e.env[cliconfig.EnvToken] = "nxp_invalid"
	s = e.startMCP("mcp")
	res = callTool(t, s, "get_context", map[string]any{})
	if !res.IsError || !strings.Contains(toolText(res), "token") {
		t.Fatalf("bad token: isError=%v, text %q", res.IsError, toolText(res))
	}
}

func TestE2EInitAgents(t *testing.T) {
	e := newE2E(t)
	e.env[cliconfig.EnvToken] = e.token
	finch := e.products[1]
	repoConfig := filepath.Join(e.repoDir, cliconfig.RepoFileName)
	mcpConfig := filepath.Join(e.repoDir, ".mcp.json")
	agentsMD := filepath.Join(e.repoDir, "AGENTS.md")

	// A fresh repository: config, MCP entry and guide are all created.
	r := e.mustRun("", "init", "--agents", "--server", e.serverURL, "--product", finch.ID, "-o", "json")
	got := decode[struct {
		MCP   map[string]string `json:"mcp"`
		Guide map[string]string `json:"guide"`
	}](t, r.stdout)
	if got.MCP["state"] != "created" || got.Guide["state"] != "created" {
		t.Fatalf("first init --agents: %s", r.stdout)
	}
	if decode[cliconfig.Repo](t, mustRead(t, repoConfig)).Product != finch.ID {
		t.Fatal(".nexmap.json not written")
	}
	type mcpFile struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	entry := decode[mcpFile](t, mustRead(t, mcpConfig)).MCPServers["nexmap"]
	if entry.Command != "nexmap" || !reflect.DeepEqual(entry.Args, []string{"mcp", "--read-only"}) {
		t.Fatalf("nexmap MCP entry = %+v", entry)
	}
	guide := mustRead(t, agentsMD)
	if !strings.Contains(guide, "nexmap context") || !strings.Contains(guide, "get_context") {
		t.Fatalf("AGENTS.md lacks the guide: %.200q", guide)
	}

	// An initialized repository with other MCP servers and a CLAUDE.md: the
	// repo config is kept, other servers survive, the guide goes to CLAUDE.md
	// once, and nothing needs the server.
	if err := os.WriteFile(mcpConfig, []byte(`{"mcpServers":{"other":{"command":"x"}},"extra":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	claudeMD := filepath.Join(e.repoDir, "CLAUDE.md")
	if err := os.WriteFile(claudeMD, []byte("# Project rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		r = e.mustRun("", "init", "--agents", "--server", "http://127.0.0.1:1")
		if !strings.Contains(r.stdout, "kept") {
			t.Fatalf("run %d: repo config not kept: %s", i, r.stdout)
		}
	}
	doc := decode[map[string]any](t, mustRead(t, mcpConfig))
	servers := doc["mcpServers"].(map[string]any)
	if servers["other"] == nil || servers["nexmap"] == nil || doc["extra"] == nil {
		t.Fatalf(".mcp.json lost entries: %v", doc)
	}
	claude := mustRead(t, claudeMD)
	if !strings.HasPrefix(claude, "# Project rules\n\n") || strings.Count(claude, "<!-- nexmap:agent-guide -->") != 1 {
		t.Fatalf("CLAUDE.md = %.300q", claude)
	}

	// Without --agents an existing config is still an error.
	if r := e.run("", "", "init", "--server", e.serverURL, "--product", finch.ID); r.code != cli.ExitUsage {
		t.Fatalf("init over existing config: exit %d", r.code)
	}
}
