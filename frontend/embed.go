package frontend

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist
var staticFS embed.FS

// Handler returns an http.Handler that serves the embedded SPA assets.
// basePath is the URL prefix where the console is mounted (e.g. "/" or "/console/").
// Unknown paths fall back to index.html so client-side routing works on refresh.
// The base path is injected into index.html at serve time so React Router
// can pick it up without a rebuild.
func Handler(basePath string) http.Handler {
	subFS, err := fs.Sub(staticFS, "dist")
	if err != nil {
		panic("frontend dist assets missing")
	}
	fileServer := http.FileServer(http.FS(subFS))
	indexHTML := patchedIndex(subFS, basePath)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		if indexHTML == nil {
			http.Error(w, "frontend dist is not built; run `make ui-build` and rebuild orlojd", http.StatusServiceUnavailable)
			return
		}

		// Serve the actual file if it exists in the embedded FS.
		clean := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if clean != "" {
			if f, err := subFS.Open(clean); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// SPA fallback: serve the patched index.html for all other paths.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
}

// patchedIndex reads index.html from the embedded FS and injects a script
// tag that exposes the UI base path to the frontend at runtime.
func patchedIndex(fsys fs.FS, basePath string) []byte {
	raw, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		return nil
	}
	tag := []byte(`<script>window.__ORLOJ_UI_BASE="` + basePath + `";</script>`)
	return bytes.Replace(raw, []byte("</head>"), append(tag, []byte("</head>")...), 1)
}

func hasDistIndex() bool {
	file, err := staticFS.Open("dist/index.html")
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}
