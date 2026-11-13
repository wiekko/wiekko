package specdoc

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// golden compares got with testdata/name, or rewrites it with -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from the rendering:\n%s", path, got)
	}
}

func TestRender(t *testing.T) {
	golden(t, "sample.md", Render(sampleDocument()))
}

func TestRenderEmptyAndTasks(t *testing.T) {
	d := Document{Product: "Empty", Tasks: []Section{}}
	golden(t, "empty.md", Render(d))

	d.Tasks = nil
	if strings.Contains(Render(d), "## Tasks") {
		t.Error("tasks rendered although not requested")
	}
}

// TestRenderIsStable guards the promise that matters for repositories: the
// same input renders byte for byte the same, whatever line endings and
// trailing whitespace the stored text has.
func TestRenderIsStable(t *testing.T) {
	a := sampleDocument()
	b := sampleDocument()
	b.Description = strings.ReplaceAll(a.Description, "\r\n", "\n") + "\n\n"
	b.Requirements[0].Items[0].Description = "A user records   \r\nwhat they saw.\r\n"
	if Render(a) != Render(b) {
		t.Error("whitespace-only differences changed the rendering")
	}
}

func TestNormalizeAndIndent(t *testing.T) {
	if got := normalize("\n\n a  \r\n\r\n b\t\n\n"); got != " a\n\n b" {
		t.Errorf("normalize = %q", got)
	}
	if got := indent("a\n\nb", 4); got != "    a\n\n    b" {
		t.Errorf("indent = %q", got)
	}
	if got := oneLine(" two\n lines "); got != "two lines" {
		t.Errorf("oneLine = %q", got)
	}
}
