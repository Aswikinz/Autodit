package api

import (
	"io/fs"
	"net/http"
	"os"
	"strings"
)

// serveFrontend exposes only the built assets and the SPA entry point. os.Root
// prevents symlink escapes as well as traversal, including concurrent renames.
func serveFrontend(directory string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") {
			http.Error(w, "invalid asset path", http.StatusBadRequest)
			return
		}
		asset := strings.HasPrefix(name, "assets/")
		if !asset {
			name = "index.html"
		}
		root, err := os.OpenRoot(directory)
		if err != nil {
			http.Error(w, "frontend unavailable", http.StatusServiceUnavailable)
			return
		}
		defer root.Close()
		file, err := root.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		if asset {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(w, r, name, info.ModTime(), file)
	}
}
