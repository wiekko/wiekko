// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package templates

import (
	"bytes"
	"html"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
)

// markdown renders GitHub Flavored Markdown (UR-143): pipe tables,
// strikethrough, task lists and autolinks, with fenced code blocks that name
// their language highlighted on the server (UR-151). Highlighting marks
// tokens with CSS classes (styled in web/static/css/input.css for light and
// dark mode, UC-68) instead of inline styles. Raw HTML stays escaped, since
// goldmark's unsafe mode is off.
var markdown = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		highlighting.NewHighlighting(
			highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			highlighting.WithGuessLanguage(false),
		),
	),
)

// tableWrapper lets a wide table scroll inside its own box instead of
// widening the page on narrow screens (UC-60). The renderer escapes raw HTML,
// so "<table>" in its output is always one it generated.
var tableWrapper = strings.NewReplacer(
	"<table>", `<div class="md-table"><table>`,
	"</table>", "</table></div>",
)

// RenderMarkdown converts a Markdown string to sanitized HTML.
// Every page that shows Markdown goes through this one renderer.
func RenderMarkdown(source string) string {
	var buf bytes.Buffer
	if err := markdown.Convert([]byte(source), &buf); err != nil {
		// The result is written to the page unescaped, so the fallback must
		// not be the raw input.
		return html.EscapeString(source)
	}
	return tableWrapper.Replace(buf.String())
}

// codeLanguages maps document formats other than Markdown and plain text to
// the language they are highlighted as.
var codeLanguages = map[string]string{
	"json":    "json",
	"yaml":    "yaml",
	"xml":     "xml",
	"proto":   "protobuf",
	"sql":     "sql",
	"graphql": "graphql",
}

// codeFormatter renders the tokens of a whole document; the surrounding
// <pre> comes from HighlightCode.
var codeFormatter = chromahtml.New(chromahtml.WithClasses(true), chromahtml.PreventSurroundingPre(true))

// HighlightCode renders source in the given document format as a highlighted
// code block, the same way RenderMarkdown renders a fenced block (UR-151).
// Formats without a highlighter come out as plain, escaped code.
func HighlightCode(format, source string) string {
	const open, closing = `<pre class="chroma"><code>`, "</code></pre>"
	plain := open + html.EscapeString(source) + closing
	lexer := lexers.Get(codeLanguages[format])
	if codeLanguages[format] == "" || lexer == nil {
		return plain
	}
	it, err := lexer.Tokenise(nil, source)
	if err != nil {
		return plain
	}
	var buf bytes.Buffer
	buf.WriteString(open)
	if err := codeFormatter.Format(&buf, styles.Fallback, it); err != nil {
		return plain
	}
	buf.WriteString(closing)
	return buf.String()
}
