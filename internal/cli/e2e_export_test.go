// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli_test

// `nexmap export` (FR-137) against the seeded Aviary product.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nexmap.net/internal/access"
	"nexmap.net/internal/cli"
	"nexmap.net/internal/service"
)

func TestE2EExport(t *testing.T) {
	e := newAviary(t)

	// Viewers can export: it shows nothing they cannot read anyway.
	out := e.mustAs("ada", "", "export").stdout
	for _, want := range []string{"# Aviary Specification", "## Product Structure", "## Constraints", "## Requirements", "* **FR-"} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q", want)
		}
	}
	if strings.Contains(out, "## Tasks") {
		t.Error("tasks exported without --tasks")
	}
	if again := e.mustAs("ada", "", "export").stdout; again != out {
		t.Error("two exports of an unchanged product differ")
	}
	if withTasks := e.mustRun("", "export", "--tasks").stdout; !strings.Contains(withTasks, "## Tasks") {
		t.Error("--tasks did not export tasks")
	}

	// --file writes the document, relative to the working directory.
	if err := os.MkdirAll(filepath.Join(e.repoDir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := e.mustRun("", "export", "--file", "docs/spec.md", "-o", "json")
	if !strings.Contains(r.stdout, `"file"`) {
		t.Errorf("json output lacks the file: %s", r.stdout)
	}
	data, err := os.ReadFile(filepath.Join(e.repoDir, "docs", "spec.md"))
	if err != nil || string(data) != out {
		t.Errorf("written file differs from stdout export (%v)", err)
	}

	// The export spans the structure and the backlog: a CI token cannot.
	ci := e.mint("bo", service.CreatePATInput{Name: "ci", Grants: access.Grants{access.Tests: access.Write}})
	if r := e.withToken(ci, "", "export"); r.code != cli.ExitAPI || !strings.Contains(r.stderr, "scope") {
		t.Errorf("export with a CI token: exit %d, stderr %q", r.code, r.stderr)
	}
	if r := e.run("", "", "export", "extra"); r.code != cli.ExitUsage {
		t.Errorf("export with an argument: exit %d, want %d", r.code, cli.ExitUsage)
	}
}

// TestE2EMCPExport checks that the MCP tool exports exactly what the CLI
// does, also on a read-only server, and that its hash identifies the text.
func TestE2EMCPExport(t *testing.T) {
	e := newAviary(t)
	want := e.mustRun("", "export").stdout
	wantTasks := e.mustRun("", "export", "--tasks").stdout

	s := e.startMCP("mcp", "--read-only")
	text, got := mustTool(t, s, "export_specification", map[string]any{})
	if text != want {
		t.Fatal("export_specification differs from `nexmap export`")
	}
	m := got.(map[string]any)
	sum := sha256.Sum256([]byte(want))
	if m["sha256"] != hex.EncodeToString(sum[:]) || int(m["bytes"].(float64)) != len(want) || m["tasks"] != false {
		t.Errorf("structured result = %v", m)
	}
	if _, ok := m["markdown"]; ok {
		t.Error("the document is duplicated in the structured result")
	}
	if text, _ := mustTool(t, s, "export_specification", map[string]any{"tasks": true}); text != wantTasks {
		t.Error("export_specification with tasks differs from `nexmap export --tasks`")
	}
}
