// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package templates

import (
	"strings"
	"testing"
)

// Comment bodies are authored by any user with tester or developer role and
// rendered as HTML for everyone else, so these guarantees are load-bearing.
func TestRenderMarkdownNeutralizesHostileInput(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		absent  []string
		present []string
	}{
		{
			name:   "raw script tag is not emitted",
			source: "<script>alert(1)</script>",
			absent: []string{"<script"},
		},
		{
			name:   "inline event handler is not emitted",
			source: `<img src=x onerror=alert(1)>`,
			absent: []string{"onerror"},
		},
		{
			name:   "javascript link scheme is stripped",
			source: "[click](javascript:alert(1))",
			absent: []string{"javascript:"},
		},
		{
			name:   "mixed case javascript scheme is stripped",
			source: "[click](JaVaScRiPt:alert(1))",
			absent: []string{"javascript:", "JaVaScRiPt:"},
		},
		{
			name:   "data image scheme is stripped",
			source: "![x](data:text/html;base64,PHNjcmlwdD4=)",
			absent: []string{"data:text/html"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := RenderMarkdown(tt.source)
			for _, s := range tt.absent {
				if strings.Contains(out, s) {
					t.Fatalf("output contains %q\ninput:  %s\noutput: %s", s, tt.source, out)
				}
			}
			for _, s := range tt.present {
				if !strings.Contains(out, s) {
					t.Fatalf("output missing %q\ninput:  %s\noutput: %s", s, tt.source, out)
				}
			}
		})
	}
}

func TestRenderMarkdownKeepsSafeContent(t *testing.T) {
	out := RenderMarkdown("**bold** and [a link](https://example.com)")
	for _, want := range []string{"<strong>bold</strong>", `href="https://example.com"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q, got: %s", want, out)
		}
	}
}

// TestRenderMarkdownGFM covers the GitHub Flavored Markdown features of
// UR-143.
func TestRenderMarkdownGFM(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		present []string
		absent  []string
	}{
		{
			name:    "pipe table in a scroll box",
			source:  "| Name | Value |\n|------|-------|\n| a    | 1     |\n",
			present: []string{`<div class="md-table"><table>`, "<th>Name</th>", "<td>1</td>", "</table></div>"},
		},
		{
			name:    "raw HTML in a table cell is dropped",
			source:  "| a |\n|---|\n| <script>x</script> |\n",
			present: []string{"<td><!-- raw HTML omitted -->x<!-- raw HTML omitted --></td>"},
			absent:  []string{"<script"},
		},
		{
			name:    "strikethrough",
			source:  "~~gone~~",
			present: []string{"<del>gone</del>"},
		},
		{
			name:    "task list",
			source:  "- [x] done\n- [ ] open\n",
			present: []string{`<input checked="" disabled="" type="checkbox"> done`, `<input disabled="" type="checkbox"> open`},
		},
		{
			name:    "autolinks",
			source:  "see www.example.com and https://example.org/x",
			present: []string{`<a href="http://www.example.com">www.example.com</a>`, `<a href="https://example.org/x">https://example.org/x</a>`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := RenderMarkdown(tt.source)
			for _, s := range tt.present {
				if !strings.Contains(out, s) {
					t.Errorf("output missing %q:\n%s", s, out)
				}
			}
			for _, s := range tt.absent {
				if strings.Contains(out, s) {
					t.Errorf("output contains %q:\n%s", s, out)
				}
			}
		})
	}
}

// TestRenderMarkdownHighlightsCode covers UR-151: fenced blocks in each
// supported language get highlighted tokens, others stay plain code.
func TestRenderMarkdownHighlightsCode(t *testing.T) {
	samples := map[string]string{
		"c":          "int main(void) { return 0; }",
		"cpp":        "int main() { return 0; }",
		"go":         "func main() {}",
		"python":     "def f():\n    return 1",
		"php":        "<?php function f() { return 1; }",
		"java":       "class A { int f() { return 1; } }",
		"rust":       "fn main() { let x = 1; }",
		"javascript": "function f() { return 1; }",
		"typescript": "function f(): number { return 1; }",
		"sh":         "if true; then echo hi; fi",
		"bash":       "if true; then echo hi; fi",
		"bat":        "@echo off\nset X=1",
		"cmd":        "@echo off\nset X=1",
		"powershell": "function F { return 1 }",
		"html":       "<p class=\"x\">hi</p>",
		"css":        "p { color: red; }",
		"xml":        "<a b=\"c\"/>",
		"json":       `{"a": 1}`,
		"toml":       "a = 1",
		"yaml":       "a: 1",
	}
	for lang, code := range samples {
		t.Run(lang, func(t *testing.T) {
			out := RenderMarkdown("```" + lang + "\n" + code + "\n```\n")
			if !strings.Contains(out, `<pre class="chroma">`) || !strings.Contains(out, `<span class="`) {
				t.Errorf("not highlighted:\n%s", out)
			}
			if strings.Contains(out, "style=") {
				t.Errorf("highlighting uses inline styles instead of classes:\n%s", out)
			}
		})
	}

	for name, source := range map[string]string{
		"unknown language": "```nosuchlang\n<b>x</b>\n```\n",
		"no language":      "```\n<b>x</b>\n```\n",
	} {
		t.Run(name, func(t *testing.T) {
			out := RenderMarkdown(source)
			if strings.Contains(out, "chroma") || strings.Contains(out, "<span") {
				t.Errorf("highlighted although the language is unknown:\n%s", out)
			}
			if !strings.Contains(out, "<pre><code") || !strings.Contains(out, "&lt;b&gt;x&lt;/b&gt;") {
				t.Errorf("not rendered as plain, escaped code:\n%s", out)
			}
		})
	}
}

func TestHighlightCode(t *testing.T) {
	out := HighlightCode("json", `{"a": "<b>"}`)
	if !strings.Contains(out, `<pre class="chroma"><code>`) || !strings.Contains(out, `<span class="`) {
		t.Errorf("json document not highlighted:\n%s", out)
	}
	if strings.Contains(out, "<b>") {
		t.Errorf("json document not escaped:\n%s", out)
	}
	if got, want := HighlightCode("text", "<b>"), `<pre class="chroma"><code>&lt;b&gt;</code></pre>`; got != want {
		t.Errorf("text document: got %s, want %s", got, want)
	}
}
