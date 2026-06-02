package agentruntime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyModuleRef(t *testing.T) {
	tests := []struct {
		ref  string
		want ModuleSource
	}{
		{"/opt/tools/echo.wasm", ModuleSourceLocal},
		{"relative/path.wasm", ModuleSourceLocal},
		{"https://example.com/tool.wasm", ModuleSourceHTTPS},
		{"http://example.com/tool.wasm", ModuleSourceLocal},
		{"oci://ghcr.io/orloj/echo:v1", ModuleSourceOCI},
		{"", ModuleSourceLocal},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got := ClassifyModuleRef(tt.ref)
			if got != tt.want {
				t.Errorf("ClassifyModuleRef(%q) = %d, want %d", tt.ref, got, tt.want)
			}
		})
	}
}

func TestResolverLocalPassthrough(t *testing.T) {
	cacheDir := t.TempDir()
	resolver, err := NewWASMModuleResolver(WASMModuleResolverConfig{
		CacheDir: cacheDir,
	})
	if err != nil {
		t.Fatalf("NewWASMModuleResolver: %v", err)
	}

	localName := "echo.wasm"
	if err := os.WriteFile(filepath.Join(cacheDir, localName), []byte("wasm"), 0o644); err != nil {
		t.Fatalf("write local module: %v", err)
	}
	got, err := resolver.Resolve(context.Background(), localName, "")
	if err != nil {
		t.Fatalf("Resolve local: %v", err)
	}
	want := filepath.Join(cacheDir, localName)
	if got != want {
		t.Errorf("Resolve local = %q, want %q", got, want)
	}
}

func TestResolverHTTPSFetch(t *testing.T) {
	wasmContent := []byte("fake wasm content for testing")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		w.Write(wasmContent)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	resolver, err := NewWASMModuleResolver(WASMModuleResolverConfig{
		CacheDir:     cacheDir,
		AllowPrivate: true,
	})
	if err != nil {
		t.Fatalf("NewWASMModuleResolver: %v", err)
	}
	resolver.skipURLCheck = true
	resolver.httpClient = server.Client()

	path, err := resolver.Resolve(context.Background(), server.URL+"/tool.wasm", "")
	if err != nil {
		t.Fatalf("Resolve HTTPS: %v", err)
	}
	if path == "" {
		t.Fatal("Resolve HTTPS returned empty path")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile cached: %v", err)
	}
	if string(content) != string(wasmContent) {
		t.Errorf("cached content = %q, want %q", string(content), string(wasmContent))
	}
}

func TestResolverHTTPSCacheHit(t *testing.T) {
	callCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte("wasm bytes"))
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	resolver, err := NewWASMModuleResolver(WASMModuleResolverConfig{
		CacheDir:     cacheDir,
		AllowPrivate: true,
	})
	if err != nil {
		t.Fatalf("NewWASMModuleResolver: %v", err)
	}
	resolver.skipURLCheck = true
	resolver.httpClient = server.Client()

	url := server.URL + "/tool.wasm"

	// First fetch.
	path1, err := resolver.Resolve(context.Background(), url, "")
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}

	// Second fetch should hit cache.
	path2, err := resolver.Resolve(context.Background(), url, "")
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}

	if path1 != path2 {
		t.Errorf("cache miss: paths differ: %q vs %q", path1, path2)
	}
	if callCount != 1 {
		t.Errorf("server called %d times, want 1 (cache hit)", callCount)
	}
}

func TestResolverSSRFRejection(t *testing.T) {
	cacheDir := t.TempDir()
	resolver, err := NewWASMModuleResolver(WASMModuleResolverConfig{
		CacheDir:     cacheDir,
		AllowPrivate: false,
	})
	if err != nil {
		t.Fatalf("NewWASMModuleResolver: %v", err)
	}

	privateURLs := []string{
		"https://10.0.0.1/tool.wasm",
		"https://192.168.1.1/tool.wasm",
		"https://169.254.169.254/tool.wasm",
	}
	for _, url := range privateURLs {
		_, err := resolver.Resolve(context.Background(), url, "")
		if err == nil {
			t.Errorf("expected SSRF rejection for %q, got nil", url)
		}
	}
}

func TestResolverOCIWithoutPuller(t *testing.T) {
	cacheDir := t.TempDir()
	resolver, err := NewWASMModuleResolver(WASMModuleResolverConfig{
		CacheDir: cacheDir,
	})
	if err != nil {
		t.Fatalf("NewWASMModuleResolver: %v", err)
	}

	_, err = resolver.Resolve(context.Background(), "oci://ghcr.io/test/tool:v1", "")
	if err == nil {
		t.Error("expected error for OCI without puller, got nil")
	}
}

type mockOCIPuller struct {
	content  []byte
	pullErr  error
	lastAuth *OCIAuth
}

func (m *mockOCIPuller) Pull(_ context.Context, ref string, destPath string, auth *OCIAuth) error {
	m.lastAuth = auth
	if m.pullErr != nil {
		return m.pullErr
	}
	return os.WriteFile(destPath, m.content, 0o644)
}

func TestResolverOCIMock(t *testing.T) {
	cacheDir := t.TempDir()
	puller := &mockOCIPuller{content: []byte("oci wasm content")}
	resolver, err := NewWASMModuleResolver(WASMModuleResolverConfig{
		CacheDir:  cacheDir,
		OCIPuller: puller,
	})
	if err != nil {
		t.Fatalf("NewWASMModuleResolver: %v", err)
	}

	path, err := resolver.Resolve(context.Background(), "oci://ghcr.io/test/tool:v1", "")
	if err != nil {
		t.Fatalf("Resolve OCI: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "oci wasm content" {
		t.Errorf("content = %q, want %q", string(content), "oci wasm content")
	}
}

func TestResolverOCIAuth(t *testing.T) {
	cacheDir := t.TempDir()
	puller := &mockOCIPuller{content: []byte("auth wasm")}

	mockSecrets := staticSecretResolver{
		values: map[string]string{
			"my-secret:username": "user",
			"my-secret:password": "pass",
		},
	}

	resolver, err := NewWASMModuleResolver(WASMModuleResolverConfig{
		CacheDir:       cacheDir,
		OCIPuller:      puller,
		SecretResolver: &mockSecrets,
	})
	if err != nil {
		t.Fatalf("NewWASMModuleResolver: %v", err)
	}

	_, err = resolver.Resolve(context.Background(), "oci://ghcr.io/private/tool:v1", "my-secret")
	if err != nil {
		t.Fatalf("Resolve OCI with auth: %v", err)
	}

	if puller.lastAuth == nil {
		t.Fatal("expected auth to be passed to puller")
	}
	if puller.lastAuth.Username != "user" || puller.lastAuth.Password != "pass" {
		t.Errorf("auth = %+v, want user/pass", puller.lastAuth)
	}
}

func TestResolverEmptyRef(t *testing.T) {
	cacheDir := t.TempDir()
	resolver, err := NewWASMModuleResolver(WASMModuleResolverConfig{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("NewWASMModuleResolver: %v", err)
	}
	_, err = resolver.Resolve(context.Background(), "", "")
	if err == nil {
		t.Error("expected error for empty ref, got nil")
	}
}

func TestResolverRejectsHTTPModuleRef(t *testing.T) {
	cacheDir := t.TempDir()
	resolver, err := NewWASMModuleResolver(WASMModuleResolverConfig{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("NewWASMModuleResolver: %v", err)
	}

	_, err = resolver.Resolve(context.Background(), "http://example.com/tool.wasm", "")
	if err == nil {
		t.Fatal("expected error for http:// ref, got nil")
	}
}

func TestResolverRejectsAbsoluteLocalPath(t *testing.T) {
	resolver, err := NewWASMModuleResolver(WASMModuleResolverConfig{CacheDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewWASMModuleResolver: %v", err)
	}
	_, err = resolver.Resolve(context.Background(), "/opt/tools/echo.wasm", "")
	if err == nil {
		t.Fatal("expected error for absolute local path, got nil")
	}
}

func TestResolverRejectsPathTraversal(t *testing.T) {
	resolver, err := NewWASMModuleResolver(WASMModuleResolverConfig{CacheDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewWASMModuleResolver: %v", err)
	}
	for _, ref := range []string{"../escape.wasm", "tools/../../escape.wasm"} {
		_, err = resolver.Resolve(context.Background(), ref, "")
		if err == nil {
			t.Fatalf("expected error for path traversal ref %q, got nil", ref)
		}
	}
}

func TestResolverSourceLabel(t *testing.T) {
	cacheDir := t.TempDir()
	resolver, err := NewWASMModuleResolver(WASMModuleResolverConfig{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("NewWASMModuleResolver: %v", err)
	}

	tests := []struct {
		ref  string
		want string
	}{
		{"/local/path.wasm", "local"},
		{"https://example.com/tool.wasm", "https"},
		{"oci://ghcr.io/test:v1", "oci"},
	}
	for _, tt := range tests {
		got := resolver.SourceLabel(tt.ref)
		if got != tt.want {
			t.Errorf("SourceLabel(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

func TestResolverCacheKeyConsistency(t *testing.T) {
	cacheDir := t.TempDir()
	resolver, err := NewWASMModuleResolver(WASMModuleResolverConfig{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("NewWASMModuleResolver: %v", err)
	}

	key1 := resolver.cacheKey("https://example.com/tool.wasm")
	key2 := resolver.cacheKey("https://example.com/tool.wasm")
	if key1 != key2 {
		t.Errorf("cache keys differ for same URL: %q vs %q", key1, key2)
	}

	key3 := resolver.cacheKey("https://example.com/other.wasm")
	if key1 == key3 {
		t.Error("cache keys should differ for different URLs")
	}

	path := resolver.cachePath("https://example.com/tool.wasm")
	if !filepath.IsAbs(path) || !pathEndsWith(path, ".wasm") {
		t.Errorf("cachePath = %q, want absolute path ending in .wasm", path)
	}
}

func pathEndsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
