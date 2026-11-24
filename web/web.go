package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist/*
var staticFiles embed.FS

// Handler returns an http.Handler that serves the embedded React SPA.
func Handler() http.Handler {
	sub, err := fs.Sub(staticFiles, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Web assets not found", http.StatusNotFound)
		})
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For SPA routing: if requesting an asset with an extension, serve as is;
		// otherwise serve root / (which maps to index.html)
		if !strings.Contains(r.URL.Path, ".") {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
