package agentruntime

import (
	"context"
	"fmt"
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
}

func NewMcpSessionManager(secretResolver SecretResolver) *McpSessionManager {
	return &McpSessionManager{
		sessions:       make(map[string]*McpSession),
		secretResolver: secretResolver,
	}
}

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

	env, err := m.resolveEnv(ctx, server)
	if err != nil {
		return nil, err
	}

	if image != "" {
		return m.buildContainerStdioTransport(server, command, env)
	}

	return NewStdioMcpTransport(StdioMcpTransportConfig{
		Command: command,
		Args:    server.Spec.Args,
		Env:     env,
	}), nil
}

func (m *McpSessionManager) buildContainerStdioTransport(server resources.McpServer, command string, env []string) (McpTransport, error) {
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
		"--read-only",
		"--cap-drop=ALL",
		"--security-opt", "no-new-privileges",
	}

	if cfg != nil && strings.TrimSpace(cfg.Network) != "" {
		dockerArgs = append(dockerArgs, "--network", strings.TrimSpace(cfg.Network))
	} else {
		dockerArgs = append(dockerArgs, "--network", "bridge")
	}
	if cfg != nil && strings.TrimSpace(cfg.Memory) != "" {
		dockerArgs = append(dockerArgs, "--memory", strings.TrimSpace(cfg.Memory))
	}
	if cfg != nil && strings.TrimSpace(cfg.CPUs) != "" {
		dockerArgs = append(dockerArgs, "--cpus", strings.TrimSpace(cfg.CPUs))
	}
	if cfg != nil && cfg.PidsLimit > 0 {
		dockerArgs = append(dockerArgs, "--pids-limit", fmt.Sprintf("%d", cfg.PidsLimit))
	}

	for _, e := range env {
		dockerArgs = append(dockerArgs, "-e", e)
	}

	if command != "" {
		dockerArgs = append(dockerArgs, "--entrypoint", command)
	}

	dockerArgs = append(dockerArgs, image)
	dockerArgs = append(dockerArgs, server.Spec.Args...)

	return NewStdioMcpTransport(StdioMcpTransportConfig{
		Command: runtimeBinary,
		Args:    dockerArgs,
	}), nil
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
		secret, err := m.secretResolver.Resolve(ctx, server.Spec.Auth.SecretRef)
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
	return NewStreamableHTTPMcpTransport(StreamableHTTPMcpTransportConfig{
		Endpoint: server.Spec.Endpoint,
		Headers:  headers,
	}), nil
}

func (m *McpSessionManager) resolveEnv(ctx context.Context, server resources.McpServer) ([]string, error) {
	if len(server.Spec.Env) == 0 {
		return nil, nil
	}
	env := make([]string, 0, len(server.Spec.Env))
	for _, e := range server.Spec.Env {
		name := strings.TrimSpace(e.Name)
		if name == "" {
			continue
		}
		value := e.Value
		if e.SecretRef != "" && m.secretResolver != nil {
			resolved, err := m.secretResolver.Resolve(ctx, e.SecretRef)
			if err != nil {
				return nil, fmt.Errorf("resolve env secret %q for %s: %w", e.SecretRef, name, err)
			}
			value = resolved
		}
		env = append(env, name+"="+value)
	}
	return env, nil
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
