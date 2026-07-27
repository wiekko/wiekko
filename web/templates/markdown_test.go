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
