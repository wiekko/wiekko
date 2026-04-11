package templates

import (
	"bytes"

	"github.com/yuin/goldmark"
)

// RenderMarkdown converts a Markdown string to sanitized HTML.
// Raw HTML in the input is escaped by default (goldmark's safe mode).
func RenderMarkdown(source string) string {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(source), &buf); err != nil {
		return source
	}
	return buf.String()
}
