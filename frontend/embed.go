package frontend

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var staticFS embed.FS

func Handler() http.Handler {
	subFS, err := fs.Sub(staticFS, "dist")
	if err != nil {
		panic("frontend dist assets missing")
	}
	fileServer := http.FileServer(http.FS(subFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Keep UI assets fresh during active development; embedded files update only on rebuild.
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		if !hasDistIndex() {
			http.Error(w, "frontend dist is not built; run `make ui-build` and rebuild orlojd", http.StatusServiceUnavailable)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func hasDistIndex() bool {
	file, err := staticFS.Open("dist/index.html")
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}
