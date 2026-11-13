package cli_test

// Defect identifiers (FR-043) through the CLI.

import (
	"regexp"
	"strings"
	"testing"

	"nexmap.net/internal/cli/ops"
)

var defectIdentifierRE = regexp.MustCompile(`^DF-[1-9][0-9]*$`)

func TestE2EDefectIdentifiers(t *testing.T) {
	e := newAviary(t)
	defects := decode[[]ops.DefectSummary](t, e.mustRun("", "defect", "list", "-o", "json").stdout)
	if len(defects) < 2 {
		t.Fatal("seed has too few defects")
	}
	seen := map[string]bool{}
	for _, d := range defects {
		if !defectIdentifierRE.MatchString(d.Identifier) || seen[d.Identifier] {
			t.Fatalf("defect %q has identifier %q; want unique DF-n", d.Title, d.Identifier)
		}
		seen[d.Identifier] = true
	}
	d := defects[1]

	// Lookup by identifier, in any case.
	shown := decode[ops.DefectDetail](t, e.mustRun("", "defect", "show", strings.ToLower(d.Identifier), "-o", "json").stdout)
	if shown.ID != d.ID {
		t.Fatalf("defect show %s = %s, want %s", d.Identifier, shown.ID, d.ID)
	}
	// The table shows identifiers.
	if out := e.mustRun("", "defect", "list").stdout; !strings.Contains(out, d.Identifier) {
		t.Errorf("defect list does not show %s:\n%s", d.Identifier, out)
	}

	// An identifier names its item without --type.
	c := decode[ops.CommentResult](t, e.mustAs("bo", "", "comment", "add", d.Identifier, "--body", "Reproduced", "-o", "json").stdout)
	if c.OwnerType != "defect" || c.Owner.ID != d.ID || c.Owner.Identifier != d.Identifier {
		t.Fatalf("comment owner = %s %+v; want defect %s", c.OwnerType, c.Owner, d.Identifier)
	}
	if r := e.run("", "", "defect", "show", "DF-9999"); r.code == 0 {
		t.Error("an unknown identifier resolved")
	}
}
