package templates

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"nexmap.net/web/templates"
)

// TestAppLayoutProvidesConfirmDialog guards UR-154: modal.js shows hx-confirm
// questions in the confirmation template and falls back to the browser's
// confirm() when either is missing.
func TestAppLayoutProvidesConfirmDialog(t *testing.T) {
	var sb strings.Builder
	err := AppLayout(AppLayoutProps{Meta: templates.PageMeta{Title: "t"}}).
		Render(templ.WithChildren(context.Background(), templ.NopComponent), &sb)
	if err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, s := range []string{`id="confirm-dialog-template"`, `src="/static/js/modal.js?v=`} {
		if !strings.Contains(out, s) {
			t.Errorf("app layout lacks %s", s)
		}
	}
}

// TestModalInheritsCSRFHeader guards the modal mount point: htmx requests
// from modal content (link buttons, create forms) get their X-CSRF-Token
// header only from an ancestor's hx-headers, so #modal has to sit inside
// #app-shell. Outside it, every write from a modal is refused with 403.
func TestModalInheritsCSRFHeader(t *testing.T) {
	var sb strings.Builder
	err := AppLayout(AppLayoutProps{Meta: templates.PageMeta{Title: "t", CSRFToken: "tok"}}).
		Render(templ.WithChildren(context.Background(), templ.NopComponent), &sb)
	if err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	shell := strings.Index(out, `id="app-shell"`)
	modal := strings.Index(out, `id="modal"`)
	if shell < 0 || modal < 0 {
		t.Fatalf("app shell or modal container missing:\n%s", out)
	}
	if !strings.Contains(out[shell:modal], `X-CSRF-Token`) {
		t.Fatal("app shell does not carry the CSRF header")
	}
	// Walk the divs from the shell's opening tag: the shell must still be
	// open when the modal container starts.
	depth := 0
	start := strings.LastIndex(out[:shell], "<div")
	for i := start; i < modal; {
		open := strings.Index(out[i:modal], "<div")
		closing := strings.Index(out[i:modal], "</div>")
		switch {
		case open < 0 && closing < 0:
			i = modal
		case closing < 0 || (open >= 0 && open < closing):
			depth++
			i += open + len("<div")
		default:
			depth--
			i += closing + len("</div>")
			if depth == 0 {
				t.Fatal("the modal container is outside #app-shell, so modal requests lack the CSRF header")
			}
		}
	}
}
