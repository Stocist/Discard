package frontend

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:build
var buildFS embed.FS

// FS returns the frontend filesystem rooted at the build directory.
func FS() (fs.FS, error) {
	return fs.Sub(buildFS, "build")
}

// SPAHandler serves the embedded frontend with SPA fallback.
// Any request that does not match a real file serves index.html,
// allowing the client-side router to handle the path.
func SPAHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(fsys, path); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
