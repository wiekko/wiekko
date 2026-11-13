package cli_test

// `nexmap export` (FR-905) against the seeded Aviary product.

import (
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
