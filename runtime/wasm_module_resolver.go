package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OrlojHQ/orloj/telemetry"
)

// WASMModuleResolver resolves a module reference (local path, HTTPS URL,
// or OCI artifact reference) to a local filesystem path. Remote modules
// are cached on disk keyed by SHA-256 of the reference string.
type WASMModuleResolver struct {
	cacheDir       string
	httpClient     *http.Client
	ociPuller      OCIArtifactPuller
	mu             sync.Mutex
	allowPrivate   bool
	secretResolver SecretResolver
	skipURLCheck   bool // for testing only -- bypasses SSRF URL pre-check
}

// OCIArtifactPuller abstracts OCI artifact pulls so callers can inject
// a real oras-go implementation or a test double.
type OCIArtifactPuller interface {
	Pull(ctx context.Context, ref string, destPath string, auth *OCIAuth) error
}

// OCIAuth carries credentials for authenticating to a private OCI registry.
type OCIAuth struct {
	Username string
	Password string
}

// WASMModuleResolverConfig holds configuration for the module resolver.
type WASMModuleResolverConfig struct {
	CacheDir       string
	AllowPrivate   bool
	SecretResolver SecretResolver
	OCIPuller      OCIArtifactPuller
}

// NewWASMModuleResolver creates a resolver that can fetch WASM modules
// from local paths, HTTPS URLs, or OCI registries.
func NewWASMModuleResolver(cfg WASMModuleResolverConfig) (*WASMModuleResolver, error) {
	cacheDir := strings.TrimSpace(cfg.CacheDir)
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			cacheDir = filepath.Join(os.TempDir(), "orloj-wasm-cache")
		} else {
			cacheDir = filepath.Join(home, ".orloj", "wasm-cache")
		}
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create wasm cache dir %q: %w", cacheDir, err)
	}
	httpClient := SafeHTTPClient(cfg.AllowPrivate, 60*time.Second)
	return &WASMModuleResolver{
		cacheDir:       cacheDir,
		httpClient:     httpClient,
		ociPuller:      cfg.OCIPuller,
		allowPrivate:   cfg.AllowPrivate,
		secretResolver: cfg.SecretResolver,
	}, nil
}

// ModuleSource classifies a module reference string.
type ModuleSource int

const (
	ModuleSourceLocal ModuleSource = iota
	ModuleSourceHTTPS
	ModuleSourceOCI
)

// ClassifyModuleRef determines the source type of a module reference.
func ClassifyModuleRef(ref string) ModuleSource {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "http://") {
		return ModuleSourceLocal
	}
	if strings.HasPrefix(ref, "https://") {
		return ModuleSourceHTTPS
	}
	if strings.HasPrefix(ref, "oci://") {
		return ModuleSourceOCI
	}
	return ModuleSourceLocal
}

// Resolve returns a local filesystem path for the given module reference.
// For local paths it returns as-is; for HTTPS URLs and OCI refs it fetches
// and caches the module on disk.
func (r *WASMModuleResolver) Resolve(ctx context.Context, moduleRef string, imagePullSecret string) (string, error) {
	moduleRef = strings.TrimSpace(moduleRef)
	if moduleRef == "" {
		return "", fmt.Errorf("empty module reference")
	}
	if strings.HasPrefix(moduleRef, "http://") {
		return "", fmt.Errorf("plaintext http:// module references are not supported; use https:// or a relative local path")
	}

	switch ClassifyModuleRef(moduleRef) {
	case ModuleSourceLocal:
		return r.resolveLocal(moduleRef)
	case ModuleSourceHTTPS:
		return r.resolveHTTPS(ctx, moduleRef)
	case ModuleSourceOCI:
		return r.resolveOCI(ctx, moduleRef, imagePullSecret)
	default:
		return "", fmt.Errorf("unsupported module reference %q", moduleRef)
	}
}

// SourceLabel returns a label suitable for metrics/logging.
func (r *WASMModuleResolver) SourceLabel(moduleRef string) string {
	switch ClassifyModuleRef(moduleRef) {
	case ModuleSourceHTTPS:
		return "https"
	case ModuleSourceOCI:
		return "oci"
	default:
		return "local"
	}
}

func (r *WASMModuleResolver) cacheKey(ref string) string {
	h := sha256.Sum256([]byte(ref))
	return hex.EncodeToString(h[:])
}

func (r *WASMModuleResolver) cachePath(ref string) string {
	return filepath.Join(r.cacheDir, r.cacheKey(ref)+".wasm")
}

func (r *WASMModuleResolver) cacheHit(ref string) (string, bool) {
	path := r.cachePath(ref)
	if _, err := os.Stat(path); err == nil {
		return path, true
	}
	return "", false
}

func (r *WASMModuleResolver) resolveLocal(moduleRef string) (string, error) {
	if strings.Contains(moduleRef, "..") {
		return "", fmt.Errorf("local module path %q contains path traversal", moduleRef)
	}
	if filepath.IsAbs(moduleRef) {
		return "", fmt.Errorf("local module path %q must be relative", moduleRef)
	}
	clean := filepath.Clean(moduleRef)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local module path %q escapes base directory", moduleRef)
	}
	absBase, err := filepath.Abs(r.cacheDir)
	if err != nil {
		return "", fmt.Errorf("resolve wasm cache dir: %w", err)
	}
	absPath, err := filepath.Abs(filepath.Join(absBase, clean))
	if err != nil {
		return "", fmt.Errorf("resolve local module path: %w", err)
	}
	if absPath != absBase && !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) {
		return "", fmt.Errorf("local module path %q is outside wasm cache directory", moduleRef)
	}
	return absPath, nil
}

func (r *WASMModuleResolver) resolveHTTPS(ctx context.Context, rawURL string) (string, error) {
	if !r.skipURLCheck {
		if err := ValidateEndpointURL(rawURL, r.allowPrivate); err != nil {
			return "", fmt.Errorf("SSRF protection: %w", err)
		}
	}

	if path, ok := r.cacheHit(rawURL); ok {
		return path, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if path, ok := r.cacheHit(rawURL); ok {
		return path, nil
	}

	fetchStart := time.Now()
	defer func() {
		telemetry.RecordWASMModuleFetch("https", time.Since(fetchStart).Seconds())
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request for %q: %w", rawURL, err)
	}
	req.Header.Set("Accept", "application/wasm, application/octet-stream, */*")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch wasm module from %q: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch wasm module from %q: HTTP %d", rawURL, resp.StatusCode)
	}

	return r.writeToCache(rawURL, resp.Body)
}

func (r *WASMModuleResolver) resolveOCI(ctx context.Context, ref string, imagePullSecret string) (string, error) {
	if r.ociPuller == nil {
		return "", fmt.Errorf("OCI module loading is not available: no OCI puller configured")
	}

	// Strip oci:// prefix for the puller.
	ociRef := strings.TrimPrefix(ref, "oci://")

	if path, ok := r.cacheHit(ref); ok {
		return path, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if path, ok := r.cacheHit(ref); ok {
		return path, nil
	}

	fetchStart := time.Now()
	defer func() {
		telemetry.RecordWASMModuleFetch("oci", time.Since(fetchStart).Seconds())
	}()

	var auth *OCIAuth
	if imagePullSecret != "" && r.secretResolver != nil {
		resolved, err := r.resolveOCIAuth(ctx, imagePullSecret)
		if err != nil {
			return "", fmt.Errorf("resolve OCI auth for %q: %w", ref, err)
		}
		auth = resolved
	}

	destPath := r.cachePath(ref)
	if err := r.ociPuller.Pull(ctx, ociRef, destPath, auth); err != nil {
		_ = os.Remove(destPath)
		return "", fmt.Errorf("pull OCI artifact %q: %w", ref, err)
	}

	return destPath, nil
}

func (r *WASMModuleResolver) resolveOCIAuth(ctx context.Context, secretRef string) (*OCIAuth, error) {
	if r.secretResolver == nil {
		return nil, fmt.Errorf("no secret resolver available for image_pull_secret %q", secretRef)
	}
	username, uErr := r.secretResolver.Resolve(ctx, secretRef+":username")
	password, pErr := r.secretResolver.Resolve(ctx, secretRef+":password")
	if uErr != nil || pErr != nil {
		return nil, fmt.Errorf("image_pull_secret %q: missing username or password key", secretRef)
	}
	return &OCIAuth{Username: username, Password: password}, nil
}

func (r *WASMModuleResolver) writeToCache(ref string, body io.Reader) (string, error) {
	destPath := r.cachePath(ref)

	tmpFile, err := os.CreateTemp(r.cacheDir, "wasm-download-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp file for wasm download: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	if _, err := io.Copy(writer, body); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("download wasm module: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return "", fmt.Errorf("rename cached wasm module: %w", err)
	}

	return destPath, nil
}
