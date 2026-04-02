package router

import (
	"io/fs"
	"net/http"

	"nexmap.net/web"
)

func mountStatic(mux *http.ServeMux) {
	// Strip the "dist" prefix from the embedded filesystem so that
	// /static/css/styles.css maps to dist/css/styles.css in the embed.
	distFS, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		panic("failed to create sub-filesystem for static assets: " + err.Error())
	}

	fileServer := http.FileServerFS(distFS)
	mux.Handle("/static/", http.StripPrefix("/static/", cacheControl(fileServer)))
}

// cacheControl wraps a handler to set Cache-Control headers for static assets.
func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
		next.ServeHTTP(w, r)
	})
}
