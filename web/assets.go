// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"sync"
)

// StaticPrefix is the URL path under which the router serves DistFS.
const StaticPrefix = "/static/"

// Assets hands out cache-busting URLs for static files: each URL carries a
// short hash of the file's content (?v=<hash>), so a new server version with
// changed assets yields new URLs and browsers fetch them at once instead of
// reusing a cached copy. Hashes are computed on first use and kept, since the
// embedded files cannot change while the server runs.
type Assets struct {
	fsys fs.FS

	mu     sync.Mutex
	hashes map[string]string
}

// NewAssets returns an Assets for the files in fsys, whose root corresponds
// to StaticPrefix.
func NewAssets(fsys fs.FS) *Assets {
	return &Assets{fsys: fsys, hashes: map[string]string{}}
}

// Version returns the content hash of the named file (a path relative to the
// static root, e.g. "css/styles.css"), or "" if it cannot be read.
func (a *Assets) Version(name string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if v, ok := a.hashes[name]; ok {
		return v
	}
	data, err := fs.ReadFile(a.fsys, name)
	if err != nil {
		// Not remembered: names come from request paths too, and caching
		// misses would let arbitrary requests grow the map.
		return ""
	}
	sum := sha256.Sum256(data)
	v := hex.EncodeToString(sum[:])[:12]
	a.hashes[name] = v
	return v
}

// URL returns the versioned URL of the named file, e.g.
// "/static/css/styles.css?v=1a2b3c4d5e6f". A file that cannot be read gets
// its plain URL.
func (a *Assets) URL(name string) string {
	if v := a.Version(name); v != "" {
		return StaticPrefix + name + "?v=" + v
	}
	return StaticPrefix + name
}

// StaticFS is DistFS with the "dist" prefix stripped, so that its paths match
// the URL paths below StaticPrefix.
var StaticFS = func() fs.FS {
	sub, err := fs.Sub(DistFS, "dist")
	if err != nil {
		panic("failed to create sub-filesystem for static assets: " + err.Error())
	}
	return sub
}()

// StaticAssets versions the embedded static files.
var StaticAssets = NewAssets(StaticFS)

// Asset returns the versioned URL of an embedded static file; templates use
// it for every stylesheet and script they reference.
func Asset(name string) string { return StaticAssets.URL(name) }
