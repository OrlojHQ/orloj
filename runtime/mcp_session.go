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
}

// McpSessionManager maintains one session per McpServer, handling connection
// pooling, initialization, and graceful shutdown.
type McpSessionManager struct {
	mu             sync.Mutex
	sessions       map[string]*McpSession
	secretResolver SecretResolver
}

func NewMcpSessionManager(secretResolver SecretResolver) *McpSessionManager {
	return &McpSessionManager{
		sessions:       make(map[string]*McpSession),
		secretResolver: secretResolver,
	}
}

// GetOrCreate returns an existing session or creates a new one for the given
// McpServer spec. Sessions are keyed by namespace/name.
func (m *McpSessionManager) GetOrCreate(ctx context.Context, server resources.McpServer) (*McpSession, error) {
	key := sessionKey(server)

	m.mu.Lock()
	if session, ok := m.sessions[key]; ok {
		m.mu.Unlock()
		return session, nil
	}
	m.mu.Unlock()

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

	session := &McpSession{
		Transport:  transport,
		InitResult: initResult,
		ServerName: server.Metadata.Name,
	}

	m.mu.Lock()
	if existing, ok := m.sessions[key]; ok {
		m.mu.Unlock()
		_ = transport.Close()
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
	env, err := m.resolveEnv(ctx, server)
	if err != nil {
		return nil, err
	}
	return NewStdioMcpTransport(StdioMcpTransportConfig{
		Command: server.Spec.Command,
		Args:    server.Spec.Args,
		Env:     env,
	}), nil
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
