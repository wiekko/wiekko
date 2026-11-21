package templates

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"nexmap.net/web"
	"nexmap.net/web/templates"
)

// TestLayoutAssetURLsCarryVersion: every stylesheet and script the app
// layout references carries the content hash of the embedded file, so
// browsers fetch new assets right after a server upgrade.
func TestLayoutAssetURLsCarryVersion(t *testing.T) {
	var sb strings.Builder
	err := AppLayout(AppLayoutProps{Meta: templates.PageMeta{Title: "t"}}).
		Render(templ.WithChildren(context.Background(), templ.NopComponent), &sb)
	if err != nil {
		t.Fatal(err)
	}
	out := sb.String()

	refs := regexp.MustCompile(`(?:<link rel="stylesheet" href|<script src)="(/static/[^"]+)"`).FindAllStringSubmatch(out, -1)
	if len(refs) < 2 {
		t.Fatalf("expected stylesheet and script references, found %d:\n%s", len(refs), out)
	}
	sawCSS := false
	for _, m := range refs {
		path, query, _ := strings.Cut(strings.TrimPrefix(m[1], web.StaticPrefix), "?")
		want := web.StaticAssets.Version(path)
		if want == "" {
			t.Errorf("%s is not an embedded file", m[1])
			continue
		}
		if query != "v="+want {
			t.Errorf("%s does not carry the content version %s", m[1], want)
		}
		sawCSS = sawCSS || path == "css/styles.css"
	}
	if !sawCSS {
		t.Error("layout does not reference css/styles.css")
	}
}
