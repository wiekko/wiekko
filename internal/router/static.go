// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package router

import (
	"net/http"

	"wiekko.net/web"
)

func mountStatic(mux *http.ServeMux) {
	// web.StaticFS has the "dist" prefix stripped, so that
	// /static/css/styles.css maps to dist/css/styles.css in the embed.
	fileServer := http.FileServerFS(web.StaticFS)
	mux.Handle(web.StaticPrefix, http.StripPrefix(web.StaticPrefix, cacheControl(web.StaticAssets, fileServer)))
}

// cacheControl wraps a handler to set Cache-Control headers for static assets.
// A request whose ?v= matches the file's current content hash (the URLs
// templates build with web.Asset) can be cached for good: a changed file gets
// a new URL. Anything else (unversioned URLs, fonts referenced from the
// stylesheet, a stale version from a page rendered before an upgrade) is
// cached briefly and revalidated.
func cacheControl(assets *web.Assets, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.URL.Query().Get("v"); v != "" && v == assets.Version(r.URL.Path) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
		}
		next.ServeHTTP(w, r)
	})
}
