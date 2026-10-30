package handlers

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// NewStaticHandler serves /static from the embedded FS, with files found in
// uploadDir overlaid under img/ first. Uploaded images are therefore served
// from /static/img/<name>, keeping the same URL space as existing logos.
func NewStaticHandler(embedFS fs.FS, uploadDir string) http.Handler {
	fileServer := http.FileServer(http.FS(embedFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Cleaning against a forced root removes any ".." traversal segments.
		cleaned := path.Clean("/" + r.URL.Path)
		if rel := strings.TrimPrefix(cleaned, "/static/"); strings.HasPrefix(rel, "img/") {
			diskPath := filepath.Join(uploadDir, filepath.FromSlash(strings.TrimPrefix(rel, "img/")))
			if info, err := os.Stat(diskPath); err == nil && info.Mode().IsRegular() {
				http.ServeFile(w, r, diskPath)
				return
			}
		}
		http.StripPrefix("/static", fileServer).ServeHTTP(w, r)
	})
}
