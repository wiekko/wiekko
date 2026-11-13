package templates

import (
	"bytes"
	"html"

	"github.com/yuin/goldmark"
)

// RenderMarkdown converts a Markdown string to sanitized HTML.
// Raw HTML in the input is escaped by default (goldmark's safe mode).
func RenderMarkdown(source string) string {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(source), &buf); err != nil {
		// The result is written to the page unescaped, so the fallback must
		// not be the raw input.
		return html.EscapeString(source)
	}
	return buf.String()
}
