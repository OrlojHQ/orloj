package agentruntime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

// McpSession wraps one active connection to an MCP server.
type McpSession struct {
	Transport  McpTransport
	InitResult *McpInitResult
	ServerName string

	generation  int64
	idleTimeout time.Duration
	lastUsedAt  time.Time
}

// McpSessionManager maintains one session per McpServer, handling connection
// pooling, initialization, idle eviction, and graceful shutdown.
type McpSessionManager struct {
	mu              sync.Mutex
	sessions        map[string]*McpSession
	secretResolver  SecretResolver
	allowedCommands []string // if non-empty, only these binaries may be launched for stdio
	containerConfig *ContainerToolRuntimeConfig
	imageInspect    func(ctx context.Context, runtimeBinary, image string, env []string) (bool, error)
	imagePull       func(ctx context.Context, runtimeBinary, image string, env []string) error
}

func NewMcpSessionManager(secretResolver SecretResolver) *McpSessionManager {
	return &McpSessionManager{
		sessions:       make(map[string]*McpSession),
		secretResolver: secretResolver,
		imageInspect:   defaultMcpImageInspect,
		imagePull:      defaultMcpImagePull,
	}
}

// RegistryAuthResolver is an optional hook for resolving image pull secrets
// into temporary Docker config directories. When nil, the manager resolves
// secrets using its own secretResolver.
type RegistryAuthResolver func(ctx context.Context, resolver SecretResolver, secretRef string) (*registryCredentials, error)

// SetContainerConfig sets the container runtime configuration used when
// McpServer resources specify spec.image for containerised stdio transport.
func (m *McpSessionManager) SetContainerConfig(cfg ContainerToolRuntimeConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.containerConfig = &cfg
}

// SetAllowedCommands restricts the binaries that stdio MCP transports may
// execute. An empty list means "no restriction" (backwards-compatible). When
// set, only the basename (or full path) of spec.command must appear in the
// list for the transport to start.
func (m *McpSessionManager) SetAllowedCommands(cmds []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allowedCommands = cmds
}

// GetOrCreate returns an existing session or creates a new one for the given
// McpServer spec. Sessions are keyed by namespace/name. If the server's
// generation has changed since the cached session was created, the old session
// is torn down and a fresh one is built.
func (m *McpSessionManager) GetOrCreate(ctx context.Context, server resources.McpServer) (*McpSession, error) {
	key := sessionKey(server)

	m.mu.Lock()
	if session, ok := m.sessions[key]; ok {
		if session.generation == server.Metadata.Generation {
			session.lastUsedAt = time.Now()
			m.mu.Unlock()
			return session, nil
		}
		delete(m.sessions, key)
		m.mu.Unlock()
		if session.Transport != nil {
			_ = session.Transport.Close()
		}
	} else {
		m.mu.Unlock()
	}

	transport, err := m.buildTransport(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("mcp session %s: build transport failed: %w", key, err)
	}

	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	initResult, err := transport.Initialize(initCtx)
	if err != nil {
		_ = transport.Close()
		return nil, fmt.Errorf("mcp session %s: initialize failed: %w", key, err)
	}

	idleTimeout := parseIdleTimeout(server.Spec.IdleTimeout)

	session := &McpSession{
		Transport:   transport,
		InitResult:  initResult,
		ServerName:  server.Metadata.Name,
		generation:  server.Metadata.Generation,
		idleTimeout: idleTimeout,
		lastUsedAt:  time.Now(),
	}

	m.mu.Lock()
	if existing, ok := m.sessions[key]; ok {
		m.mu.Unlock()
		_ = transport.Close()
		existing.lastUsedAt = time.Now()
		return existing, nil
	}
	m.sessions[key] = session
	m.mu.Unlock()

	return session, nil
}

// Remove closes and removes the session for the given server.
func (m *McpSessionManager) Remove(server resources.McpServer) {
	key := sessionKey(server)
	m.mu.Lock()
	session, ok := m.sessions[key]
	if ok {
		delete(m.sessions, key)
	}
	m.mu.Unlock()

	if ok && session.Transport != nil {
		_ = session.Transport.Close()
	}
}

// Close shuts down all active sessions.
func (m *McpSessionManager) Close() {
	m.mu.Lock()
	sessions := make(map[string]*McpSession, len(m.sessions))
	for k, v := range m.sessions {
		sessions[k] = v
	}
	m.sessions = make(map[string]*McpSession)
	m.mu.Unlock()

	for _, session := range sessions {
		if session.Transport != nil {
			_ = session.Transport.Close()
		}
	}
}

// StartReaper runs a background goroutine that periodically evicts sessions
// whose idle time exceeds their configured idle_timeout. Sessions with
// idleTimeout == 0 are never evicted. The goroutine exits when ctx is done.
func (m *McpSessionManager) StartReaper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.evictIdle()
		}
	}
}

func (m *McpSessionManager) evictIdle() {
	now := time.Now()
	var toClose []*McpSession

	m.mu.Lock()
	for key, session := range m.sessions {
		if session.idleTimeout <= 0 {
			continue
		}
		if now.Sub(session.lastUsedAt) > session.idleTimeout {
			toClose = append(toClose, session)
			delete(m.sessions, key)
		}
	}
	m.mu.Unlock()

	for _, session := range toClose {
		if session.Transport != nil {
			_ = session.Transport.Close()
		}
	}
}

func (m *McpSessionManager) buildTransport(ctx context.Context, server resources.McpServer) (McpTransport, error) {
	switch strings.ToLower(strings.TrimSpace(server.Spec.Transport)) {
	case "stdio":
		return m.buildStdioTransport(ctx, server)
	case "http":
		return m.buildHTTPTransport(ctx, server)
	default:
		return nil, fmt.Errorf("unsupported transport %q", server.Spec.Transport)
	}
}

func (m *McpSessionManager) buildStdioTransport(ctx context.Context, server resources.McpServer) (McpTransport, error) {
	command := strings.TrimSpace(server.Spec.Command)
	image := strings.TrimSpace(server.Spec.Image)

	if command != "" {
		if err := m.validateStdioCommand(command); err != nil {
			return nil, err
		}
	}

	resolved, err := m.resolveEnv(ctx, server)
	if err != nil {
		return nil, err
	}

	if image != "" {
		var creds *registryCredentials
		if pullSecret := strings.TrimSpace(server.Spec.ImagePullSecret); pullSecret != "" {
			resolver := m.secretScopedToServer(server)
			var resolveErr error
			creds, resolveErr = resolveRegistryAuth(ctx, resolver, pullSecret)
			if resolveErr != nil {
				return nil, fmt.Errorf("resolve image_pull_secret %q: %w", pullSecret, resolveErr)
			}
		}
		var extraEnv []string
		if creds != nil {
			extraEnv = []string{creds.DockerConfigEnv()}
		}
		if err := m.ensureImagePulled(ctx, m.containerRuntimeBinary(), image, extraEnv); err != nil {
			if creds != nil {
				creds.Cleanup()
			}
			return nil, fmt.Errorf("pull image %q: %w", image, err)
		}
		return m.buildContainerStdioTransport(server, command, resolved, creds)
	}

	return NewStdioMcpTransport(StdioMcpTransportConfig{
		Command: command,
		Args:    server.Spec.Args,
		Env:     resolved.EnvVars,
	}), nil
}

func (m *McpSessionManager) buildContainerStdioTransport(server resources.McpServer, command string, resolved *resolvedMcpEnv, creds *registryCredentials) (McpTransport, error) {
	m.mu.Lock()
	cfg := m.containerConfig
	m.mu.Unlock()

	runtimeBinary := "docker"
	if cfg != nil && strings.TrimSpace(cfg.RuntimeBinary) != "" {
		runtimeBinary = strings.TrimSpace(cfg.RuntimeBinary)
	}

	image := strings.TrimSpace(server.Spec.Image)
	dockerArgs := []string{
		"run", "--rm", "-i",
	}

	if cfg != nil && strings.TrimSpace(cfg.Network) != "" {
		dockerArgs = append(dockerArgs, "--network", strings.TrimSpace(cfg.Network))
	} else {
		dockerArgs = append(dockerArgs, "--network", "bridge")
	}
	memory := strings.TrimSpace(server.Spec.Resources.Memory)
	if memory == "" && cfg != nil {
		memory = strings.TrimSpace(cfg.Memory)
	}
	if memory != "" {
		dockerArgs = append(dockerArgs, "--memory", memory)
	}
	cpus := strings.TrimSpace(server.Spec.Resources.CPUs)
	if cpus == "" && cfg != nil {
		cpus = strings.TrimSpace(cfg.CPUs)
	}
	if cpus != "" {
		dockerArgs = append(dockerArgs, "--cpus", cpus)
	}
	pidsLimit := server.Spec.Resources.PidsLimit
	if pidsLimit <= 0 && cfg != nil {
		pidsLimit = cfg.PidsLimit
	}
	if pidsLimit > 0 {
		dockerArgs = append(dockerArgs, "--pids-limit", fmt.Sprintf("%d", pidsLimit))
	}

	var cleanupDir string
	if len(resolved.Mounts) > 0 {
		tmpDir, err := os.MkdirTemp("", "orloj-mcp-*")
		if err != nil {
			return nil, fmt.Errorf("create temp dir for secret mounts: %w", err)
		}
		cleanupDir = tmpDir

		for i, mount := range resolved.Mounts {
			hostFile := filepath.Join(tmpDir, fmt.Sprintf("mount_%d", i))
			if err := os.WriteFile(hostFile, []byte(mount.Content), 0600); err != nil {
				_ = os.RemoveAll(tmpDir)
				return nil, fmt.Errorf("write secret mount %q: %w", mount.ContainerPath, err)
			}
			dockerArgs = append(dockerArgs,
				"--mount", fmt.Sprintf("type=bind,source=%s,target=%s,readonly", hostFile, mount.ContainerPath),
			)
		}
	}

	for _, e := range resolved.EnvVars {
		dockerArgs = append(dockerArgs, "-e", e)
	}

	if command != "" {
		dockerArgs = append(dockerArgs, "--entrypoint", command)
	}

	dockerArgs = append(dockerArgs, image)
	dockerArgs = append(dockerArgs, server.Spec.Args...)

	onClose := func() {
		if cleanupDir != "" {
			_ = os.RemoveAll(cleanupDir)
		}
		if creds != nil {
			creds.Cleanup()
		}
	}

	var transportEnv []string
	if creds != nil {
		transportEnv = []string{creds.DockerConfigEnv()}
	}

	return NewStdioMcpTransport(StdioMcpTransportConfig{
		Command: runtimeBinary,
		Args:    dockerArgs,
		Env:     transportEnv,
		OnClose: onClose,
	}), nil
}

func (m *McpSessionManager) containerRuntimeBinary() string {
	m.mu.Lock()
	cfg := m.containerConfig
	m.mu.Unlock()
	if cfg != nil && strings.TrimSpace(cfg.RuntimeBinary) != "" {
		return strings.TrimSpace(cfg.RuntimeBinary)
	}
	return "docker"
}

// ensureImagePulled runs "<runtime> image inspect" to check if the image
// exists locally and, if not, pulls it with a generous timeout so that the
// image-pull cost does not eat into the MCP initialize handshake timeout.
// The extraEnv slice is appended to the process environment for both inspect
// and pull commands (used to inject DOCKER_CONFIG for private registries).
func (m *McpSessionManager) ensureImagePulled(ctx context.Context, runtimeBinary, image string, extraEnv []string) error {
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	present, err := m.inspectImage(inspectCtx, runtimeBinary, image, extraEnv)
	if err != nil {
		return err
	}
	if present {
		return nil // already present
	}

	pullCtx, pullCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer pullCancel()
	return m.pullImage(pullCtx, runtimeBinary, image, extraEnv)
}

func (m *McpSessionManager) inspectImage(ctx context.Context, runtimeBinary, image string, extraEnv []string) (bool, error) {
	if m != nil && m.imageInspect != nil {
		return m.imageInspect(ctx, runtimeBinary, image, extraEnv)
	}
	return defaultMcpImageInspect(ctx, runtimeBinary, image, extraEnv)
}

func (m *McpSessionManager) pullImage(ctx context.Context, runtimeBinary, image string, extraEnv []string) error {
	if m != nil && m.imagePull != nil {
		return m.imagePull(ctx, runtimeBinary, image, extraEnv)
	}
	return defaultMcpImagePull(ctx, runtimeBinary, image, extraEnv)
}

func defaultMcpImageInspect(ctx context.Context, runtimeBinary, image string, extraEnv []string) (bool, error) {
	cmd := exec.CommandContext(ctx, runtimeBinary, "image", "inspect", image)
	if len(extraEnv) > 0 {
		cmd.Env = append(cmd.Environ(), extraEnv...)
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	lower := strings.ToLower(string(out))
	if strings.Contains(lower, "no such image") || strings.Contains(lower, "no such object") || strings.Contains(lower, "not found") {
		return false, nil
	}
	return false, fmt.Errorf("%s image inspect failed: %w\n%s", runtimeBinary, err, out)
}

func defaultMcpImagePull(ctx context.Context, runtimeBinary, image string, extraEnv []string) error {
	pullCmd := exec.CommandContext(ctx, runtimeBinary, "pull", "--quiet", image)
	if len(extraEnv) > 0 {
		pullCmd.Env = append(pullCmd.Environ(), extraEnv...)
	}
	if out, err := pullCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s pull failed: %w\n%s", runtimeBinary, err, out)
	}
	return nil
}

// validateStdioCommand checks the command against the allowed commands list.
// If no allowlist is configured, all commands are permitted (for backward
// compatibility). When an allowlist is set, only the first token of the
// command (the binary) is checked against the list.
func (m *McpSessionManager) validateStdioCommand(command string) error {
	m.mu.Lock()
	allowed := m.allowedCommands
	m.mu.Unlock()

	if len(allowed) == 0 {
		return nil
	}

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}
	binary := parts[0]

	for _, cmd := range allowed {
		if cmd == binary {
			return nil
		}
	}
	return fmt.Errorf("mcp stdio command %q is not in the allowed commands list", binary)
}

func (m *McpSessionManager) buildHTTPTransport(ctx context.Context, server resources.McpServer) (McpTransport, error) {
	headers := make(map[string]string)
	if server.Spec.Auth.SecretRef != "" && m.secretResolver != nil {
		resolver := m.secretScopedToServer(server)
		secret, err := resolver.Resolve(ctx, server.Spec.Auth.SecretRef)
		if err != nil {
			return nil, fmt.Errorf("resolve auth secret %q: %w", server.Spec.Auth.SecretRef, err)
		}
		profile := strings.ToLower(strings.TrimSpace(server.Spec.Auth.Profile))
		if profile == "" {
			profile = "bearer"
		}
		switch profile {
		case "bearer":
			headers["Authorization"] = "Bearer " + secret
		case "api_key_header":
			headerName := server.Spec.Auth.HeaderName
			if headerName == "" {
				headerName = "X-API-Key"
			}
			headers[headerName] = secret
		}
	}
	allowPrivate := false
	if server.Spec.AllowPrivate != nil {
		allowPrivate = *server.Spec.AllowPrivate
	}
	return NewStreamableHTTPMcpTransport(StreamableHTTPMcpTransportConfig{
		Endpoint:     server.Spec.Endpoint,
		Headers:      headers,
		AllowPrivate: allowPrivate,
	}), nil
}

type secretMount struct {
	Content       string
	ContainerPath string
}

type resolvedMcpEnv struct {
	EnvVars []string
	Mounts  []secretMount
}

// secretScopedToServer returns a SecretResolver scoped to the server's namespace
// so that bare secret names (without an explicit namespace prefix) resolve within
// the same namespace as the McpServer resource.
func (m *McpSessionManager) secretScopedToServer(server resources.McpServer) SecretResolver {
	if m.secretResolver == nil {
		return nil
	}
	if aware, ok := m.secretResolver.(interface {
		WithNamespace(string) SecretResolver
	}); ok {
		return aware.WithNamespace(resources.NormalizeNamespace(server.Metadata.Namespace))
	}
	return m.secretResolver
}

func (m *McpSessionManager) resolveEnv(ctx context.Context, server resources.McpServer) (*resolvedMcpEnv, error) {
	result := &resolvedMcpEnv{}
	if len(server.Spec.Env) == 0 {
		return result, nil
	}
	result.EnvVars = make([]string, 0, len(server.Spec.Env))
	resolver := m.secretScopedToServer(server)
	for _, e := range server.Spec.Env {
		name := strings.TrimSpace(e.Name)
		if name == "" {
			continue
		}
		value := e.Value
		if e.SecretRef != "" && resolver != nil {
			resolved, err := resolver.Resolve(ctx, e.SecretRef)
			if err != nil {
				return nil, fmt.Errorf("resolve env secret %q for %s: %w", e.SecretRef, name, err)
			}
			value = resolved
		}
		mountPath := strings.TrimSpace(e.MountPath)
		if mountPath != "" {
			result.Mounts = append(result.Mounts, secretMount{
				Content:       value,
				ContainerPath: mountPath,
			})
			result.EnvVars = append(result.EnvVars, name+"="+mountPath)
		} else {
			result.EnvVars = append(result.EnvVars, name+"="+value)
		}
	}
	return result, nil
}

func sessionKey(server resources.McpServer) string {
	ns := resources.NormalizeNamespace(server.Metadata.Namespace)
	return ns + "/" + strings.TrimSpace(server.Metadata.Name)
}

func parseIdleTimeout(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0
	}
	return d
}
