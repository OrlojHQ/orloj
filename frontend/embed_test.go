package frontend

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerCacheHeaders(t *testing.T) {
	assetPath := firstEmbeddedAsset(t)
	handler := Handler("/")

	assetReq := httptest.NewRequest(http.MethodGet, "/"+strings.TrimPrefix(assetPath, "dist/"), nil)
	assetRes := httptest.NewRecorder()
	handler.ServeHTTP(assetRes, assetReq)
	if assetRes.Code != http.StatusOK {
		t.Fatalf("asset response status = %d, want %d", assetRes.Code, http.StatusOK)
	}
	if got, want := assetRes.Header().Get("Cache-Control"), "public, max-age=31536000, immutable"; got != want {
		t.Fatalf("asset Cache-Control = %q, want %q", got, want)
	}
	if got := assetRes.Header().Get("Pragma"); got != "" {
		t.Fatalf("asset Pragma = %q, want empty", got)
	}
	if got := assetRes.Header().Get("Expires"); got != "" {
		t.Fatalf("asset Expires = %q, want empty", got)
	}

	fallbackReq := httptest.NewRequest(http.MethodGet, "/systems/example", nil)
	fallbackRes := httptest.NewRecorder()
	handler.ServeHTTP(fallbackRes, fallbackReq)
	if fallbackRes.Code != http.StatusOK {
		t.Fatalf("fallback response status = %d, want %d", fallbackRes.Code, http.StatusOK)
	}
	if got, want := fallbackRes.Header().Get("Cache-Control"), "no-store, max-age=0"; got != want {
		t.Fatalf("fallback Cache-Control = %q, want %q", got, want)
	}
	if got, want := fallbackRes.Header().Get("Pragma"), "no-cache"; got != want {
		t.Fatalf("fallback Pragma = %q, want %q", got, want)
	}
	if got, want := fallbackRes.Header().Get("Expires"), "0"; got != want {
		t.Fatalf("fallback Expires = %q, want %q", got, want)
	}
}

func firstEmbeddedAsset(t *testing.T) string {
	t.Helper()

	var assetPath string
	err := fs.WalkDir(staticFS, "dist/assets", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || assetPath != "" {
			return err
		}
		assetPath = path
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded assets: %v", err)
	}
	if assetPath == "" {
		t.Fatal("no embedded asset found")
	}
	return assetPath
}
