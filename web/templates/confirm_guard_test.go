package templates

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// rawConfirm matches an hx-confirm attribute written directly into a template.
var rawConfirm = regexp.MustCompile(`(?m)^\s*hx-confirm\s*=`)

// TestConfirmationsUseTheApplicationModal guards UR-154: every confirmation
// goes through ui.Confirm, which sets the dialog's title, button label and
// danger or neutral styling next to hx-confirm. A bare hx-confirm would show
// a dialog with generic copy and neutral styling, even for a deletion.
func TestConfirmationsUseTheApplicationModal(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".templ") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, loc := range rawConfirm.FindAllIndex(src, -1) {
			line := 1 + strings.Count(string(src[:loc[0]]), "\n")
			t.Errorf("%s:%d: bare hx-confirm; use { ui.Confirm(...)... } instead", path, line)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
