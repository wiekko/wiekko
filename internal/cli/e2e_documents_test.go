package cli_test

// `nexmap doc` and the documents in `nexmap export` (FR-044..FR-052,
// FR-907..FR-909) against the seeded Aviary product.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nexmap.net/internal/access"
	"nexmap.net/internal/cli"
	"nexmap.net/internal/service"
)

type docView struct {
	ID           string `json:"id"`
	Identifier   string `json:"identifier"`
	Body         string `json:"body"`
	Format       string `json:"format"`
	State        string `json:"state"`
	Revision     int    `json:"revision"`
	File         string `json:"file"`
	Requirements []struct {
		Identifier string `json:"identifier"`
	} `json:"requirements"`
}

func TestE2EDocuments(t *testing.T) {
	e := newAviary(t)

	// A body from a file is stored byte for byte, CRLF and markup included.
	body := "syntax = \"proto3\";\r\n\r\n// <script>alert(1)</script>\r\nmessage Sighting {}\r\n"
	if err := os.WriteFile(filepath.Join(e.repoDir, "sync.proto"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	d := decode[docView](t, e.mustRun("", "doc", "create", "--title", "Sync wire format", "--format", "proto",
		"--state", "current", "--body-file", "sync.proto", "-o", "json").stdout)
	if d.Body != body || d.Format != "proto" || d.Revision != 1 || !strings.HasSuffix(d.File, ".proto") {
		t.Fatalf("created = %+v", d)
	}
	ref := d.Identifier

	// Viewers read, but cannot write.
	if out := e.mustAs("ada", "", "doc", "show", ref).stdout; !strings.Contains(out, "```proto\n") || !strings.Contains(out, "message Sighting") {
		t.Errorf("doc show:\n%s", out)
	}
	if r := e.as("ada", "", "doc", "create", "--title", "x"); r.code != cli.ExitAPI {
		t.Errorf("viewer created a document: exit %d", r.code)
	}
	if out := e.mustRun("", "doc", "list", "--format", "proto").stdout; !strings.Contains(out, ref) {
		t.Errorf("doc list lacks %s:\n%s", ref, out)
	}

	// Changes make revisions; diff and restore work on them.
	e.mustRun("", "doc", "update", ref, "--body", "syntax = \"proto3\";\nmessage Sighting { string id = 1; }\n")
	if out := e.mustRun("", "doc", "diff", ref).stdout; !strings.Contains(out, "+message Sighting { string id = 1; }") {
		t.Errorf("doc diff:\n%s", out)
	}
	if out := e.mustRun("", "doc", "revisions", ref).stdout; !strings.Contains(out, "\n2 ") || !strings.Contains(out, "\n1 ") {
		t.Errorf("doc revisions:\n%s", out)
	}
	d = decode[docView](t, e.mustRun("", "doc", "restore", ref, "1", "-o", "json").stdout)
	if d.Revision != 3 || d.Body != body {
		t.Fatalf("restored = %+v", d)
	}
	if r := e.run("", "", "doc", "restore", ref, "x"); r.code != cli.ExitUsage {
		t.Errorf("restore with a bad revision: exit %d", r.code)
	}

	// doc get writes the body back out unchanged, of any revision.
	e.mustRun("", "doc", "get", ref, "--revision", "2", "--file", "out/r2.proto")
	if data, err := os.ReadFile(filepath.Join(e.repoDir, "out", "r2.proto")); err != nil || !strings.Contains(string(data), "string id = 1") {
		t.Errorf("doc get --revision 2: %q, %v", data, err)
	}
	if got := e.mustRun("", "doc", "get", ref).stdout; got != body {
		t.Errorf("doc get = %q", got)
	}

	// Links, named on the requirement too (FR-909).
	d = decode[docView](t, e.mustRun("", "doc", "link", ref, "FR-1", "-o", "json").stdout)
	if len(d.Requirements) != 1 || d.Requirements[0].Identifier != "FR-1" {
		t.Fatalf("linked = %+v", d)
	}
	if out := e.mustRun("", "req", "show", "FR-1").stdout; !strings.Contains(out, "## Documents") || !strings.Contains(out, ref+" Sync wire format") {
		t.Errorf("req show FR-1 does not name the document:\n%s", out)
	}
	// Read on every product scope but documents.
	noDocs := access.Grants{}
	for _, sc := range access.ProductScopes {
		if sc != access.Documents {
			noDocs[sc] = access.Read
		}
	}
	backlog := e.mint("cy", service.CreatePATInput{Name: "no-docs", Grants: noDocs})
	r := e.withToken(backlog, "", "req", "show", "FR-1", "-o", "json")
	if r.code != cli.ExitOK || !strings.Contains(r.stdout, `"identifier": "`+ref+`"`) || strings.Contains(r.stdout, "Sync wire format") {
		t.Errorf("FR-1 without documents:read: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if r := e.withToken(backlog, "", "doc", "show", ref); r.code != cli.ExitAPI || !strings.Contains(r.stderr, "scope") {
		t.Errorf("doc show without documents:read: exit %d, stderr %q", r.code, r.stderr)
	}
	if r := e.as("ada", "", "doc", "link", ref, "FR-1"); r.code != cli.ExitAPI {
		t.Errorf("viewer linked a document: exit %d", r.code)
	}

	// The export lists the document and writes its body byte for byte.
	draft := decode[docView](t, e.mustRun("", "doc", "create", "--title", "Next format", "-o", "json").stdout)
	r = e.mustRun("", "export", "--file", "spec.md", "--documents", "docs/defs")
	spec, err := os.ReadFile(filepath.Join(e.repoDir, "spec.md"))
	if err != nil || !strings.Contains(string(spec), "## Documents") || !strings.Contains(string(spec), "**"+ref+"**: Sync wire format") {
		t.Fatalf("spec lacks the document (%v):\n%s", err, spec)
	}
	if strings.Contains(string(spec), draft.Identifier) {
		t.Error("a draft was exported without --drafts")
	}
	written, err := os.ReadFile(filepath.Join(e.repoDir, "docs", "defs", d.File))
	if err != nil || string(written) != body {
		t.Errorf("exported %s = %q, %v", d.File, written, err)
	}
	if _, err := os.Stat(filepath.Join(e.repoDir, "docs", "defs", draft.File)); err == nil {
		t.Error("a draft's file was written without --drafts")
	}
	e.mustRun("", "export", "--drafts", "--documents", "docs/defs")
	if _, err := os.Stat(filepath.Join(e.repoDir, "docs", "defs", draft.File)); err != nil {
		t.Errorf("--drafts did not write the draft: %v", err)
	}

	// Deleting needs confirmation.
	if r := e.run("", "", "doc", "delete", draft.Identifier); r.code != cli.ExitUsage {
		t.Errorf("delete without --yes: exit %d", r.code)
	}
	e.mustRun("", "doc", "delete", draft.Identifier, "--yes")
	if r := e.run("", "", "doc", "show", draft.Identifier); r.code != cli.ExitNotFound {
		t.Errorf("deleted document still shows: exit %d", r.code)
	}
}

// TestE2EMCPDocuments checks the MCP document tools against the CLI.
func TestE2EMCPDocuments(t *testing.T) {
	e := newAviary(t)
	s := e.startMCP("mcp")
	_, created := mustTool(t, s, "create_document", map[string]any{
		"title": "Sighting schema", "format": "json", "state": "current", "body": "{\"type\": \"object\"}\n",
	})
	ref := created.(map[string]any)["identifier"].(string)
	if _, got := mustTool(t, s, "link_document", map[string]any{"document": ref, "items": []string{"FR-1"}}); len(got.(map[string]any)["requirements"].([]any)) != 1 {
		t.Errorf("link_document = %v", got)
	}
	_, viaMCP := mustTool(t, s, "get_document", map[string]any{"ref": ref})
	viaCLI := generic(t, decode[any](t, e.mustRun("", "doc", "show", ref, "-o", "json").stdout))
	if a, b := generic(t, viaMCP), viaCLI; !jsonEqual(a, b) {
		t.Errorf("get_document differs from `nexmap doc show`:\n%v\n%v", a, b)
	}
	mustTool(t, s, "update_document", map[string]any{"ref": ref, "body": "{}\n"})
	if text, _ := mustTool(t, s, "document_diff", map[string]any{"ref": ref}); !strings.Contains(text, "+{}") {
		t.Errorf("document_diff:\n%s", text)
	}
	if _, got := mustTool(t, s, "restore_document", map[string]any{"ref": ref, "revision": 1}); int(got.(map[string]any)["revision"].(float64)) != 3 {
		t.Errorf("restore_document = %v", got)
	}

	ro := e.startMCP("mcp", "--read-only")
	if text, _ := mustTool(t, ro, "list_documents", map[string]any{"state": "current"}); !strings.Contains(text, ref) {
		t.Errorf("list_documents:\n%s", text)
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
