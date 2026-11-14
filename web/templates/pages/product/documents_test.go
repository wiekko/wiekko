package product

import (
	"context"
	"strings"
	"testing"

	"nexmap.net/internal/model"
)

// hostileBody tries every way in that the rendering must close.
const hostileBody = "<script>alert(1)</script>\n" +
	"<img src=x onerror=alert(1)>\n" +
	"[click](javascript:alert(1))\n" +
	"</code></pre><iframe src=//evil></iframe>\n" +
	"<svg onload=alert(1)>\n"

// activeContent lists what must not reach the page as markup. A
// "javascript:" left as escaped text in a code block is harmless; as a link
// target it is not, so links are checked separately.
var activeContent = []string{"<script", "<img", "<iframe", "<svg", "</code></pre><", "href=\"javascript:", "<a "}

func renderComponent(t *testing.T, render func(sb *strings.Builder) error) string {
	t.Helper()
	return renderToString(t, render)
}

// TestDocumentBodyIsInert checks that a document body, whatever its format,
// reaches the page without active content: Markdown through the safe
// renderer, every other format as escaped text.
func TestDocumentBodyIsInert(t *testing.T) {
	for _, format := range model.DocumentFormats {
		out := renderComponent(t, func(sb *strings.Builder) error {
			return DocumentBody(format, hostileBody).Render(context.Background(), sb)
		})
		for _, bad := range activeContent {
			if strings.Contains(out, bad) {
				t.Errorf("format %s: output contains %q:\n%s", format, bad, out)
			}
		}
		if format == model.DocumentFormatMarkdown && strings.Contains(out, "javascript:") {
			t.Errorf("markdown output keeps a javascript: link:\n%s", out)
		}
		if format != model.DocumentFormatMarkdown && !strings.Contains(out, "&lt;script&gt;") {
			t.Errorf("format %s: body not shown as escaped text:\n%s", format, out)
		}
	}
}

func TestDocumentDiffIsInert(t *testing.T) {
	lines := []DocumentDiffLine{
		{Op: "delete", Text: "<script>alert(1)</script>", Old: 1},
		{Op: "insert", Text: "<img src=x onerror=alert(1)>", New: 1},
	}
	out := renderComponent(t, func(sb *strings.Builder) error {
		return DocumentDiff(lines).Render(context.Background(), sb)
	})
	for _, bad := range activeContent {
		if strings.Contains(out, bad) {
			t.Errorf("diff output contains %q:\n%s", bad, out)
		}
	}
}

// TestDocumentFormKeepsLeadingNewline checks the textarea round trip: HTML
// drops one line feed right after <textarea>, so the form writes one extra.
func TestDocumentFormKeepsLeadingNewline(t *testing.T) {
	props := DocumentFormProps{Product: &model.Product{ID: "p1", Name: "Aviary"}, Body: "\nfirst line after a blank one"}
	out := renderComponent(t, func(sb *strings.Builder) error {
		return DocumentFormPage(props).Render(context.Background(), sb)
	})
	if !strings.Contains(out, ">\n\nfirst line after a blank one</textarea>") {
		t.Errorf("textarea does not preserve the leading line feed:\n%s", out)
	}
}
