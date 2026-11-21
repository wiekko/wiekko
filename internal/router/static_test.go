package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"nexmap.net/web"
)

// TestStaticCacheControl: requests for the current version of an asset may be
// cached for good; unversioned or stale versions are revalidated.
func TestStaticCacheControl(t *testing.T) {
	fsys := fstest.MapFS{"css/styles.css": {Data: []byte("a{}")}}
	assets := web.NewAssets(fsys)
	h := http.StripPrefix(web.StaticPrefix, cacheControl(assets, http.FileServerFS(fsys)))

	current := assets.URL("css/styles.css")
	for _, tc := range []struct {
		url, want string
	}{
		{current, "public, max-age=31536000, immutable"},
		{"/static/css/styles.css", "public, max-age=3600, must-revalidate"},
		{"/static/css/styles.css?v=000000000000", "public, max-age=3600, must-revalidate"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", tc.url, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != tc.want {
			t.Errorf("%s: Cache-Control = %q, want %q", tc.url, got, tc.want)
		}
	}
}
