package web

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestAssetVersionFollowsContent: the version in an asset URL is derived from
// the file's content, so a changed file (a new server version) gets a new URL
// and browsers cannot keep using a cached copy.
func TestAssetVersionFollowsContent(t *testing.T) {
	oldURL := NewAssets(fstest.MapFS{"css/styles.css": {Data: []byte("a{}")}}).URL("css/styles.css")
	sameURL := NewAssets(fstest.MapFS{"css/styles.css": {Data: []byte("a{}")}}).URL("css/styles.css")
	newURL := NewAssets(fstest.MapFS{"css/styles.css": {Data: []byte("a{color:red}")}}).URL("css/styles.css")

	if !strings.HasPrefix(oldURL, "/static/css/styles.css?v=") || len(oldURL) == len("/static/css/styles.css?v=") {
		t.Fatalf("asset URL lacks a version: %q", oldURL)
	}
	if oldURL != sameURL {
		t.Errorf("same content, different URLs: %q, %q", oldURL, sameURL)
	}
	if oldURL == newURL {
		t.Errorf("changed content kept the URL %q", oldURL)
	}
}

// TestAssetMissingFileHasPlainURL: a file that is not embedded gets its
// unversioned URL instead of a bogus version.
func TestAssetMissingFileHasPlainURL(t *testing.T) {
	a := NewAssets(fstest.MapFS{})
	if got := a.URL("js/none.js"); got != "/static/js/none.js" {
		t.Errorf("URL = %q", got)
	}
	if v := a.Version("js/none.js"); v != "" {
		t.Errorf("Version = %q", v)
	}
}
