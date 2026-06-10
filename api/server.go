package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/frontend"
	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
	"github.com/OrlojHQ/orloj/telemetry"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Stores groups typed state stores used by the API server.
type Stores struct {
	Agents          *store.AgentStore
	AgentSystems    *store.AgentSystemStore
	ModelEPs        *store.ModelEndpointStore
	Tools           *store.ToolStore
	Secrets         *store.SecretStore
	SealedSecrets   *store.SealedSecretStore
	SealingKeys     *store.SealingKeyStore
	Memories        *store.MemoryStore
	ContextAdapters *store.ContextAdapterStore
	Policies        *store.AgentPolicyStore
	AgentRoles      *store.AgentRoleStore
	ToolPerms       *store.ToolPermissionStore
	ToolApprovals   *store.ToolApprovalStore
	TaskApprovals   *store.TaskApprovalStore
	Tasks           *store.TaskStore
	TaskSchedules   *store.TaskScheduleStore
	TaskWebhooks    *store.TaskWebhookStore
	WebhookDedupe   *store.WebhookDedupeStore
	Workers         *store.WorkerStore
	McpServers      *store.McpServerStore
	EvalDatasets    *store.EvalDatasetStore
	EvalRuns        *store.EvalRunStore
	LocalAdmins     *store.LocalAdminStore
	APITokens       *store.APITokenStore
	AuthSessions    *store.AuthSessionStore
}

// ServerOptions configures optional extension points.
type ServerOptions struct {
	Authorizer               RequestAuthorizer
	ResourceAuthorizer       ResourceAuthorizer // optional authorization hook
	Extensions               agentruntime.Extensions
	AuthMode                 AuthMode
	SessionTTL               time.Duration
	UIBasePath               string // URL path prefix for the web console (default "/")
	TrustedProxies           string // comma-separated CIDRs whose forwarding headers are trusted
	ContainerResourceCeiling resources.ContainerResourceCeiling
	CRDConflictPolicy        string // "off", "warn" (default), or "reject"
	CORSAllowedOrigins       []string
}

// Server exposes CRUD endpoints for control plane resources.
type Server struct {
	stores                   Stores
	runtime                  *agentruntime.Manager
	logger                   *log.Logger
	mux                      *http.ServeMux
	authorizer               RequestAuthorizer
	resourceAuthorizer       ResourceAuthorizer // optional authorization hook
	authMode                 AuthMode
	sessionTTL               time.Duration
	bus                      eventbus.Bus
	extensions               agentruntime.Extensions
	memoryBackends           *agentruntime.PersistentMemoryBackendRegistry
	authRateLimiter          *authRateLimiter
	requestRateLimiter       *rate.Limiter // per-server; avoids test suites sharing one process-global bucket
	trustedProxies           []*net.IPNet
	uiBasePath               string
	containerResourceCeiling resources.ContainerResourceCeiling
	crdConflictPolicy        string
	a2aConfig                *A2AConfig
	a2aRateLimiter           *ipRateLimiter
	a2aSubscribeCount        atomic.Int32
	corsAllowedOrigins       []string
	watchSubscribeCount      atomic.Int32
	watchRateLimiter         *ipRateLimiter
}

func NewServer(stores Stores, runtime *agentruntime.Manager, logger *log.Logger) *Server {
	return NewServerWithOptions(stores, runtime, logger, ServerOptions{})
}

func NewServerWithOptions(stores Stores, runtime *agentruntime.Manager, logger *log.Logger, opts ServerOptions) *Server {
	if stores.ModelEPs == nil {
		stores.ModelEPs = store.NewModelEndpointStore()
	}
	if stores.AgentRoles == nil {
		stores.AgentRoles = store.NewAgentRoleStore()
	}
	if stores.ToolPerms == nil {
		stores.ToolPerms = store.NewToolPermissionStore()
	}
	if stores.ToolApprovals == nil {
		stores.ToolApprovals = store.NewToolApprovalStore()
	}
	if stores.Secrets == nil {
		stores.Secrets = store.NewSecretStore()
	}
	if stores.SealedSecrets == nil {
		stores.SealedSecrets = store.NewSealedSecretStore()
	}
	if stores.SealingKeys == nil {
		stores.SealingKeys = store.NewSealingKeyStore()
	}
	if stores.TaskSchedules == nil {
		stores.TaskSchedules = store.NewTaskScheduleStore()
	}
	if stores.TaskApprovals == nil {
		stores.TaskApprovals = store.NewTaskApprovalStore()
	}
	if stores.TaskWebhooks == nil {
		stores.TaskWebhooks = store.NewTaskWebhookStore()
	}
	if stores.WebhookDedupe == nil {
		stores.WebhookDedupe = store.NewWebhookDedupeStore()
	}
	if stores.ContextAdapters == nil {
		stores.ContextAdapters = store.NewContextAdapterStore()
	}
	if stores.McpServers == nil {
		stores.McpServers = store.NewMcpServerStore()
	}
	if stores.EvalDatasets == nil {
		stores.EvalDatasets = store.NewEvalDatasetStore()
	}
	if stores.EvalRuns == nil {
		stores.EvalRuns = store.NewEvalRunStore()
	}
	if stores.LocalAdmins == nil {
		stores.LocalAdmins = store.NewLocalAdminStore()
	}
	if stores.APITokens == nil {
		stores.APITokens = store.NewAPITokenStore()
	}
	if stores.AuthSessions == nil {
		stores.AuthSessions = store.NewAuthSessionStore()
	}
	rawAuthMode := strings.ToLower(strings.TrimSpace(string(opts.AuthMode)))
	authMode := normalizeAuthMode(rawAuthMode)
	authModeExplicit := rawAuthMode != ""
	sessionTTL := opts.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}
	extensions := agentruntime.NormalizeExtensions(opts.Extensions)
	authorizer := opts.Authorizer
	if authorizer == nil {
		if authMode == AuthModeOff && authModeExplicit {
			// auth-mode=off with no injected authorizer: stay open unless env/DB
			// tokens are configured (ORLOJ_API_TOKEN, ORLOJ_API_TOKENS, /v1/tokens).
			tokenAuth := newTokenAuthorizerWithStoreFromEnv(stores.APITokens)
			if ta, ok := tokenAuth.(tokenAuthorizer); ok {
				if enabled, statusCode, _ := ta.authEnabled(); enabled && statusCode == 0 {
					authorizer = tokenAuth
				} else {
					authorizer = noAuthAuthorizer{}
				}
			} else {
				authorizer = noAuthAuthorizer{}
			}
		} else {
			authorizer = newTokenAuthorizerWithStoreFromEnv(stores.APITokens)
		}
	}
	if authMode == AuthModeNative {
		authorizer = newNativeModeAuthorizer(authorizer, stores.LocalAdmins, stores.AuthSessions, sessionTTL)
	}
	uiBase := normalizeUIBasePath(opts.UIBasePath)
	trustedProxies, tpErr := parseTrustedProxies(opts.TrustedProxies)
	if tpErr != nil && logger != nil {
		logger.Printf("WARNING: invalid --trusted-proxies value: %v; forwarding headers will be ignored", tpErr)
	}
	s := &Server{
		stores:             stores,
		runtime:            runtime,
		logger:             logger,
		mux:                http.NewServeMux(),
		authorizer:         authorizer,
		resourceAuthorizer: opts.ResourceAuthorizer,
		authMode:           authMode,
		sessionTTL:         sessionTTL,
		bus:                eventbus.NewMemoryBus(4096),
		extensions:         extensions,
		authRateLimiter:    newAuthRateLimiter(trustedProxies, logger),
		// 500 r/s sustained, burst 100 — same as previous package-global limiter, but per Server instance
		// so concurrent httptest servers in tests do not share one token bucket.
		requestRateLimiter:       rate.NewLimiter(rate.Limit(500), 100),
		trustedProxies:           trustedProxies,
		uiBasePath:               uiBase,
		containerResourceCeiling: opts.ContainerResourceCeiling,
		crdConflictPolicy:        normalizeCRDConflictPolicy(opts.CRDConflictPolicy),
		corsAllowedOrigins:       append([]string(nil), opts.CORSAllowedOrigins...),
		watchRateLimiter:         newIPRateLimiter(rate.Limit(30), 30, trustedProxies),
	}
	s.routes()
	return s
}

func (s *Server) SetEventBus(bus eventbus.Bus) {
	if bus == nil {
		s.bus = eventbus.NewMemoryBus(4096)
		return
	}
	s.bus = bus
}

func (s *Server) EventBus() eventbus.Bus {
	return s.bus
}

// SetMemoryBackends configures the registry used to serve memory entry queries.
func (s *Server) SetMemoryBackends(registry *agentruntime.PersistentMemoryBackendRegistry) {
	s.memoryBackends = registry
}

// SetA2AConfig configures A2A protocol support on this server.
func (s *Server) SetA2AConfig(config *A2AConfig) {
	s.a2aConfig = config
	if config != nil && config.RateLimitRPM > 0 {
		s.a2aRateLimiter = newIPRateLimiter(
			rate.Limit(float64(config.RateLimitRPM)/60.0),
			config.RateLimitRPM,
			s.trustedProxies,
		)
	}
}

func (s *Server) validateToolContainerResources(tool resources.Tool) error {
	res := tool.Spec.Cli.Resources
	if err := resources.ValidateContainerResources(res, "spec.cli.resources"); err != nil {
		return err
	}
	return resources.EnforceContainerResourceCeiling(res, s.containerResourceCeiling, "tool", tool.Metadata.Name)
}

func (s *Server) validateMcpServerContainerResources(server resources.McpServer) error {
	res := server.Spec.Resources
	if err := resources.ValidateContainerResources(res, "spec.resources"); err != nil {
		return err
	}
	return resources.EnforceContainerResourceCeiling(res, s.containerResourceCeiling, "McpServer", server.Metadata.Name)
}

// maxRequestBodyBytes is the hard cap on incoming request bodies for all
// non-streaming endpoints. 4 MB is generous for any control-plane resource
// manifest while still preventing OOM from malicious or misconfigured clients.
const maxRequestBodyBytes = 4 * 1024 * 1024

func (s *Server) withBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withRateLimit(next http.Handler) http.Handler {
	lim := s.requestRateLimiter
	if lim == nil {
		lim = rate.NewLimiter(rate.Limit(500), 100)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !lim.Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UIBasePath returns the normalized base path for the web console.
func (s *Server) UIBasePath() string { return s.uiBasePath }

func (s *Server) Handler() http.Handler {
	chain := s.withAuth(s.mux)
	chain = s.withContentTypeCheck(chain)
	chain = s.withBodyLimit(chain)
	chain = s.withRateLimit(chain)
	chain = withCORS(s.corsAllowedOrigins, chain)
	return withRequestTimeout(chain)
}

// normalizeUIBasePath ensures the path has a leading and trailing slash.
// An empty input defaults to "/".
func normalizeUIBasePath(raw string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasSuffix(p, "/") {
		p = p + "/"
	}
	return p
}

var (
	apiReadTimeout  = 30 * time.Second
	apiWriteTimeout = 60 * time.Second
)

// withRequestTimeout adds a deadline to every request's context. Read methods
// (GET, HEAD) get a shorter deadline than mutating methods.
func withRequestTimeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timeout, ok := requestTimeoutFor(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestTimeoutFor(r *http.Request) (time.Duration, bool) {
	if isStreamingWatchRequest(r) {
		return 0, false
	}
	timeout := apiWriteTimeout
	if r != nil && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		timeout = apiReadTimeout
	}
	return timeout, true
}

func isStreamingWatchRequest(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	path := strings.TrimSpace(r.URL.Path)

	// A2A endpoints use POST and may include long-lived SSE subscriptions;
	// the JSON-RPC handler manages its own timeouts.
	if r.Method == http.MethodPost && (path == "/a2a" || strings.HasSuffix(path, "/a2a")) {
		return true
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	return path == "/v1/events/watch" || strings.HasSuffix(path, "/watch")
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.Handle("/metrics", promhttp.Handler())
	s.mux.HandleFunc("/v1/capabilities", s.handleCapabilities)
	s.mux.HandleFunc("/v1/auth/config", s.handleAuthConfig)
	s.mux.HandleFunc("/v1/auth/setup", s.handleAuthSetup)
	s.mux.HandleFunc("/v1/auth/login", s.handleAuthLogin)
	s.mux.HandleFunc("/v1/auth/cli-token", s.handleAuthCLIToken)
	s.mux.HandleFunc("/v1/auth/logout", s.handleAuthLogout)
	s.mux.HandleFunc("/v1/auth/me", s.handleAuthMe)
	s.mux.HandleFunc("/v1/auth/change-password", s.handleAuthChangePassword)
	s.mux.HandleFunc("/v1/auth/users", s.handleAuthUsers)
	s.mux.HandleFunc("/v1/auth/users/", s.handleAuthUserByName)
	s.mux.HandleFunc("/v1/auth/admin/reset-password", s.handleAuthAdminResetPassword)
	s.mux.HandleFunc("/v1/tokens", s.handleTokens)
	s.mux.HandleFunc("/v1/tokens/", s.handleTokenByName)
	if s.uiBasePath != "/" {
		trimmed := strings.TrimSuffix(s.uiBasePath, "/")
		s.mux.HandleFunc(trimmed, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, s.uiBasePath, http.StatusTemporaryRedirect)
		})
	}
	s.mux.Handle(s.uiBasePath, http.StripPrefix(s.uiBasePath, frontend.Handler(s.uiBasePath)))

	s.mux.HandleFunc("/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/v1/agents/watch", s.watchAgents)
	s.mux.HandleFunc("/v1/agents/", s.handleAgentByName)

	s.mux.HandleFunc("/v1/agent-systems", s.handleAgentSystems)
	s.mux.HandleFunc("/v1/agent-systems/", s.handleAgentSystemByName)

	s.mux.HandleFunc("/v1/model-endpoints", s.handleModelEndpoints)
	s.mux.HandleFunc("/v1/model-endpoints/", s.handleModelEndpointByName)

	s.mux.HandleFunc("/v1/tools", s.handleTools)
	s.mux.HandleFunc("/v1/tools/", s.handleToolByName)

	s.mux.HandleFunc("/v1/secrets", s.handleSecrets)
	s.mux.HandleFunc("/v1/secrets/", s.handleSecretByName)
	s.mux.HandleFunc("/v1/sealed-secrets", s.handleSealedSecrets)
	s.mux.HandleFunc("/v1/sealed-secrets/", s.handleSealedSecretByName)
	s.mux.HandleFunc("/v1/sealing-key/public", s.handleSealingKeyPublic)

	s.mux.HandleFunc("/v1/memories", s.handleMemories)
	s.mux.HandleFunc("/v1/memories/", s.handleMemoryByName)

	s.mux.HandleFunc("/v1/context-adapters", s.handleContextAdapters)
	s.mux.HandleFunc("/v1/context-adapters/", s.handleContextAdapterByName)

	s.mux.HandleFunc("/v1/eval-datasets", s.handleEvalDatasets)
	s.mux.HandleFunc("/v1/eval-datasets/", s.handleEvalDatasetByName)
	s.mux.HandleFunc("/v1/eval-runs", s.handleEvalRuns)
	s.mux.HandleFunc("/v1/eval-runs/compare", s.handleEvalRunsCompare)
	s.mux.HandleFunc("/v1/eval-runs/", s.handleEvalRunByName)

	s.mux.HandleFunc("/v1/agent-policies", s.handlePolicies)
	s.mux.HandleFunc("/v1/agent-policies/", s.handlePolicyByName)

	s.mux.HandleFunc("/v1/agent-roles", s.handleAgentRoles)
	s.mux.HandleFunc("/v1/agent-roles/", s.handleAgentRoleByName)

	s.mux.HandleFunc("/v1/tool-permissions", s.handleToolPermissions)
	s.mux.HandleFunc("/v1/tool-permissions/", s.handleToolPermissionByName)

	s.mux.HandleFunc("/v1/tool-approvals", s.handleToolApprovals)
	s.mux.HandleFunc("/v1/tool-approvals/", s.handleToolApprovalByName)
	s.mux.HandleFunc("/v1/task-approvals", s.handleTaskApprovals)
	s.mux.HandleFunc("/v1/task-approvals/", s.handleTaskApprovalByName)

	s.mux.HandleFunc("/v1/tasks", s.handleTasks)
	s.mux.HandleFunc("/v1/tasks/watch", s.watchTasks)
	s.mux.HandleFunc("/v1/tasks/", s.handleTaskByName)
	s.mux.HandleFunc("/v1/task-schedules", s.handleTaskSchedules)
	s.mux.HandleFunc("/v1/task-schedules/watch", s.watchTaskSchedules)
	s.mux.HandleFunc("/v1/task-schedules/", s.handleTaskScheduleByName)
	s.mux.HandleFunc("/v1/task-webhooks", s.handleTaskWebhooks)
	s.mux.HandleFunc("/v1/task-webhooks/watch", s.watchTaskWebhooks)
	s.mux.HandleFunc("/v1/task-webhooks/", s.handleTaskWebhookByName)
	s.mux.HandleFunc("/v1/webhook-deliveries/", s.handleWebhookDelivery)
	s.mux.HandleFunc("/v1/events/watch", s.watchEvents)

	s.mux.HandleFunc("/v1/workers", s.handleWorkers)
	s.mux.HandleFunc("/v1/workers/", s.handleWorkerByName)

	s.mux.HandleFunc("/v1/mcp-servers", s.handleMcpServers)
	s.mux.HandleFunc("/v1/mcp-servers/", s.handleMcpServerByName)

	s.mux.HandleFunc("/v1/namespaces", s.handleNamespaces)

	// A2A Protocol routes
	s.mux.HandleFunc("/.well-known/agent-card.json", s.handleWellKnownAgentCard)
	s.mux.HandleFunc("/.well-known/agent.json", s.handleWellKnownAgentCard)
	s.mux.HandleFunc("/a2a", s.handleA2AJSONRPC)
	s.mux.HandleFunc("/v1/a2a/agents", s.handleA2ARegistry)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snapshot := s.extensions.Capabilities.Capabilities(r.Context())
	if strings.TrimSpace(snapshot.GeneratedAt) == "" {
		snapshot.GeneratedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	a2aSystems, _ := s.a2aEnabledSystems(r)
	if s.a2aConfig != nil && len(a2aSystems) > 0 {
		snapshot.Capabilities = append(snapshot.Capabilities,
			agentruntime.Capability{ID: "a2a", Enabled: true, Description: "A2A protocol interoperability", Source: "config"},
			agentruntime.Capability{ID: "a2a.streaming", Enabled: s.a2aConfig.StreamingEnabled, Description: "A2A streaming subscribe support", Source: "config"},
			agentruntime.Capability{ID: "a2a.registry", Enabled: s.a2aConfig.Registry != nil, Description: "Remote agent registry", Source: "config"},
		)
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	seen := make(map[string]struct{})
	collect := func(ns string) {
		ns = strings.TrimSpace(ns)
		if ns != "" {
			seen[ns] = struct{}{}
		}
	}
	if agents, err := s.stores.Agents.List(r.Context()); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range agents {
			collect(item.Metadata.Namespace)
		}
	}
	if agentSystems, err := s.stores.AgentSystems.List(r.Context()); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range agentSystems {
			collect(item.Metadata.Namespace)
		}
	}
	if modelEPs, err := s.stores.ModelEPs.List(r.Context()); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range modelEPs {
			collect(item.Metadata.Namespace)
		}
	}
	if tools, err := s.stores.Tools.List(r.Context()); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range tools {
			collect(item.Metadata.Namespace)
		}
	}
	if secrets, err := s.stores.Secrets.List(r.Context()); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range secrets {
			collect(item.Metadata.Namespace)
		}
	}
	if sealedSecrets, err := s.stores.SealedSecrets.List(r.Context()); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range sealedSecrets {
			collect(item.Metadata.Namespace)
		}
	}
	if memories, err := s.stores.Memories.List(r.Context()); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range memories {
			collect(item.Metadata.Namespace)
		}
	}
	if contextAdapters, err := s.stores.ContextAdapters.List(r.Context()); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range contextAdapters {
			collect(item.Metadata.Namespace)
		}
	}
	if policies, err := s.stores.Policies.List(r.Context()); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range policies {
			collect(item.Metadata.Namespace)
		}
	}
	if tasks, err := s.stores.Tasks.List(r.Context()); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range tasks {
			collect(item.Metadata.Namespace)
		}
	}
	if workers, err := s.stores.Workers.List(r.Context()); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range workers {
			collect(item.Metadata.Namespace)
		}
	}
	if len(seen) == 0 {
		seen[resources.DefaultNamespace] = struct{}{}
	}
	namespaces := make([]string, 0, len(seen))
	for ns := range seen {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)
	writeJSON(w, http.StatusOK, map[string]any{"namespaces": namespaces})
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listAgents(w, r)
	case http.MethodPost:
		s.createOrUpdateAgent(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgentByName(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/agents/"), "/")
	if path == "" {
		http.Error(w, "agent name is required", http.StatusBadRequest)
		return
	}
	if strings.Contains(r.URL.Path, "/.well-known/agent-card.json") || strings.HasSuffix(r.URL.Path, "/.well-known/agent.json") {
		s.handleAgentCard(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/a2a") {
		s.handleA2AJSONRPC(w, r)
		return
	}
	if path == "watch" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.watchAgents(w, r)
		return
	}
	if strings.HasSuffix(path, "/logs") {
		name := strings.Trim(strings.TrimSuffix(path, "/logs"), "/")
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.getAgentLogs(w, r.Context(), scopedNameForRequest(r, name), name)
		return
	}
	if strings.HasSuffix(path, "/status") {
		name := strings.Trim(strings.TrimSuffix(path, "/status"), "/")
		if name == "" {
			http.Error(w, "agent name is required", http.StatusBadRequest)
			return
		}
		s.handleAgentStatusByName(w, r, name)
		return
	}

	name := path
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		s.getAgent(w, r.Context(), key, name)
	case http.MethodDelete:
		s.deleteAgent(w, r, key, name)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		agent, err := resources.ParseAgentManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.Agents.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("agent %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &agent.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &agent.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if s.checkCRDConflict(w, current.Metadata, "Agent") {
			return
		}
		agent.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(agent.Metadata.Name))
		if bodyKey != key {
			agent, err = s.stores.Agents.UpsertMovingKey(r.Context(), key, agent)
		} else {
			agent, err = s.stores.Agents.Upsert(r.Context(), agent)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("Agent", agent.Metadata.Name, "updated", s.withRuntimeStatus(agent))
		writeJSON(w, http.StatusOK, s.withRuntimeStatus(agent))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) createOrUpdateAgent(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	agent, err := resources.ParseAgentManifest(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := applyRequestNamespace(r, &agent.Metadata); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	existing, ok, err := s.stores.Agents.Get(r.Context(), store.ScopedName(agent.Metadata.Namespace, agent.Metadata.Name))
	if writeStoreFetchError(w, err) {
		return
	}
	if ok {
		if s.checkCRDConflict(w, existing.Metadata, "Agent") {
			return
		}
		agent.Status = existing.Status
	}
	agent, err = s.stores.Agents.Upsert(r.Context(), agent)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.logApply("Agent", agent.Metadata.Name)
	s.publishResourceEvent("Agent", agent.Metadata.Name, "created", s.withRuntimeStatus(agent))
	writeJSON(w, http.StatusCreated, s.withRuntimeStatus(agent))
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	agents, cont, err := fetchListPage(r.Context(), r, s.stores.Agents.ListCursor, func(a resources.Agent) resources.ObjectMeta { return a.Metadata })
	if writeListPageError(w, err) {
		return
	}
	for i := range agents {
		agents[i] = s.withRuntimeStatus(agents[i])
	}
	writeJSON(w, http.StatusOK, resources.AgentList{ListMeta: resources.ListMeta{Continue: cont}, Items: agents})
}

func (s *Server) getAgent(w http.ResponseWriter, ctx context.Context, key, name string) {
	agent, ok, err := s.stores.Agents.Get(ctx, key)
	if writeStoreFetchError(w, err) {
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("agent %q not found", name), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, s.withRuntimeStatus(agent))
}

func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request, key, name string) {
	existing, ok, err := s.stores.Agents.Get(r.Context(), key)
	if writeStoreFetchError(w, err) {
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("agent %q not found", name), http.StatusNotFound)
		return
	}
	if s.checkCRDConflict(w, existing.Metadata, "Agent") {
		return
	}
	if err := s.stores.Agents.Delete(r.Context(), key); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.runtime.Stop(key)
	s.publishResourceEvent("Agent", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getAgentLogs(w http.ResponseWriter, ctx context.Context, key, name string) {
	_, ok, err := s.stores.Agents.Get(ctx, key)
	if writeStoreFetchError(w, err) {
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("agent %q not found", name), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "logs": s.runtime.Logs(key)})
}

func (s *Server) handleAgentSystems(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.AgentSystems.ListCursor, func(item resources.AgentSystem) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.AgentSystemList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseAgentSystemManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.AgentSystems.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			if s.checkCRDConflict(w, existing.Metadata, "AgentSystem") {
				return
			}
			obj.Status = existing.Status
		}
		obj, err = s.stores.AgentSystems.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("AgentSystem", obj.Metadata.Name)
		s.publishResourceEvent("AgentSystem", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgentSystemByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/agent-systems/"), "/")
	if name == "" {
		http.Error(w, "agentsystem name is required", http.StatusBadRequest)
		return
	}
	if strings.Contains(r.URL.Path, "/.well-known/agent-card.json") || strings.HasSuffix(r.URL.Path, "/.well-known/agent.json") {
		s.handleAgentCard(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/a2a") {
		s.handleA2AJSONRPC(w, r)
		return
	}
	if strings.HasSuffix(name, "/status") {
		base := strings.Trim(strings.TrimSuffix(name, "/status"), "/")
		if base == "" {
			http.Error(w, "agentsystem name is required", http.StatusBadRequest)
			return
		}
		s.handleAgentSystemStatusByName(w, r, base)
		return
	}
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.AgentSystems.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("agentsystem %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		existing, ok, err := s.stores.AgentSystems.Get(r.Context(), key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("agentsystem %q not found", name), http.StatusNotFound)
			return
		}
		if s.checkCRDConflict(w, existing.Metadata, "AgentSystem") {
			return
		}
		if err := s.stores.AgentSystems.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("AgentSystem", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseAgentSystemManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.AgentSystems.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("agentsystem %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if s.checkCRDConflict(w, current.Metadata, "AgentSystem") {
			return
		}
		obj.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		if bodyKey != key {
			obj, err = s.stores.AgentSystems.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.AgentSystems.Upsert(r.Context(), obj)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("AgentSystem", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleModelEndpoints(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.ModelEPs.ListCursor, func(item resources.ModelEndpoint) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.ModelEndpointList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseModelEndpointManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.ModelEPs.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			if s.checkCRDConflict(w, existing.Metadata, "ModelEndpoint") {
				return
			}
			obj.Status = existing.Status
		}
		obj, err = s.stores.ModelEPs.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("ModelEndpoint", obj.Metadata.Name)
		s.publishResourceEvent("ModelEndpoint", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleModelEndpointByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/model-endpoints/"), "/")
	if name == "" {
		http.Error(w, "model endpoint name is required", http.StatusBadRequest)
		return
	}
	if strings.HasSuffix(name, "/status") {
		base := strings.Trim(strings.TrimSuffix(name, "/status"), "/")
		if base == "" {
			http.Error(w, "model endpoint name is required", http.StatusBadRequest)
			return
		}
		s.handleModelEndpointStatusByName(w, r, base)
		return
	}
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.ModelEPs.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("modelendpoint %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		existing, ok, err := s.stores.ModelEPs.Get(r.Context(), key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("modelendpoint %q not found", name), http.StatusNotFound)
			return
		}
		if s.checkCRDConflict(w, existing.Metadata, "ModelEndpoint") {
			return
		}
		if err := s.stores.ModelEPs.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("ModelEndpoint", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseModelEndpointManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.ModelEPs.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("modelendpoint %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if s.checkCRDConflict(w, current.Metadata, "ModelEndpoint") {
			return
		}
		obj.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		if bodyKey != key {
			obj, err = s.stores.ModelEPs.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.ModelEPs.Upsert(r.Context(), obj)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("ModelEndpoint", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.Tools.ListCursor, func(item resources.Tool) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.ToolList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseToolManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.validateToolContainerResources(obj); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.Tools.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			if s.checkCRDConflict(w, existing.Metadata, "Tool") {
				return
			}
			obj.Status = existing.Status
		}
		obj, err = s.stores.Tools.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("Tool", obj.Metadata.Name)
		s.publishResourceEvent("Tool", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleToolByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/tools/"), "/")
	if name == "" {
		http.Error(w, "tool name is required", http.StatusBadRequest)
		return
	}
	if strings.HasSuffix(name, "/status") {
		base := strings.Trim(strings.TrimSuffix(name, "/status"), "/")
		if base == "" {
			http.Error(w, "tool name is required", http.StatusBadRequest)
			return
		}
		s.handleToolStatusByName(w, r, base)
		return
	}
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.Tools.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("tool %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		existing, ok, err := s.stores.Tools.Get(r.Context(), key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("tool %q not found", name), http.StatusNotFound)
			return
		}
		if s.checkCRDConflict(w, existing.Metadata, "Tool") {
			return
		}
		if err := s.stores.Tools.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("Tool", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseToolManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.validateToolContainerResources(obj); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.Tools.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("tool %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if s.checkCRDConflict(w, current.Metadata, "Tool") {
			return
		}
		obj.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		if bodyKey != key {
			obj, err = s.stores.Tools.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.Tools.Upsert(r.Context(), obj)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("Tool", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSecrets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.Secrets.ListCursor, func(item resources.Secret) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		for i := range items {
			redactSecretData(&items[i])
		}
		writeJSON(w, http.StatusOK, resources.SecretList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseSecretManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.Secrets.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			if s.checkCRDConflict(w, existing.Metadata, "Secret") {
				return
			}
			obj.Status = existing.Status
		}
		obj, err = s.stores.Secrets.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("Secret", obj.Metadata.Name)
		redacted := obj
		redactSecretData(&redacted)
		s.publishResourceEvent("Secret", obj.Metadata.Name, "created", redacted)
		writeJSON(w, http.StatusCreated, redacted)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSecretByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/secrets/"), "/")
	if name == "" {
		http.Error(w, "secret name is required", http.StatusBadRequest)
		return
	}
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.Secrets.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("secret %q not found", name), http.StatusNotFound)
			return
		}
		redactSecretData(&obj)
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		existing, ok, err := s.stores.Secrets.Get(r.Context(), key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("secret %q not found", name), http.StatusNotFound)
			return
		}
		if s.checkCRDConflict(w, existing.Metadata, "Secret") {
			return
		}
		if err := s.stores.Secrets.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("Secret", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.Secrets.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("secret %q not found", name), http.StatusNotFound)
			return
		}
		obj, err := resources.ParseSecretManifestForPut(body, current)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if s.checkCRDConflict(w, current.Metadata, "Secret") {
			return
		}
		obj.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		if bodyKey != key {
			obj, err = s.stores.Secrets.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.Secrets.Upsert(r.Context(), obj)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		redacted := obj
		redactSecretData(&redacted)
		s.publishResourceEvent("Secret", obj.Metadata.Name, "updated", redacted)
		writeJSON(w, http.StatusOK, redacted)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// redactSecretData replaces all data values with "***" so that secret
// contents are never returned over the API or published to the event bus.
func redactSecretData(secret *resources.Secret) {
	if secret == nil {
		return
	}
	if len(secret.Spec.Data) > 0 {
		redacted := make(map[string]string, len(secret.Spec.Data))
		for k := range secret.Spec.Data {
			redacted[k] = "***"
		}
		secret.Spec.Data = redacted
	}
	secret.Spec.StringData = nil
}

func (s *Server) handleMemories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.Memories.ListCursor, func(item resources.Memory) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.MemoryList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseMemoryManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.Memories.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			if s.checkCRDConflict(w, existing.Metadata, "Memory") {
				return
			}
			obj.Status = existing.Status
		}
		obj, err = s.stores.Memories.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("Memory", obj.Metadata.Name)
		s.publishResourceEvent("Memory", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMemoryByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/memories/"), "/")
	if name == "" {
		http.Error(w, "memory name is required", http.StatusBadRequest)
		return
	}
	if strings.HasSuffix(name, "/status") {
		base := strings.Trim(strings.TrimSuffix(name, "/status"), "/")
		if base == "" {
			http.Error(w, "memory name is required", http.StatusBadRequest)
			return
		}
		s.handleMemoryStatusByName(w, r, base)
		return
	}
	if strings.HasSuffix(name, "/entries") {
		base := strings.Trim(strings.TrimSuffix(name, "/entries"), "/")
		if base == "" {
			http.Error(w, "memory name is required", http.StatusBadRequest)
			return
		}
		s.handleMemoryEntries(w, r, base)
		return
	}
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.Memories.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("memory %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		existing, ok, err := s.stores.Memories.Get(r.Context(), key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("memory %q not found", name), http.StatusNotFound)
			return
		}
		if s.checkCRDConflict(w, existing.Metadata, "Memory") {
			return
		}
		if err := s.stores.Memories.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("Memory", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseMemoryManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.Memories.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("memory %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if s.checkCRDConflict(w, current.Metadata, "Memory") {
			return
		}
		obj.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		if bodyKey != key {
			obj, err = s.stores.Memories.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.Memories.Upsert(r.Context(), obj)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("Memory", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMemoryEntries(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := scopedNameForRequest(r, name)
	_, ok, err := s.stores.Memories.Get(r.Context(), key)
	if writeStoreFetchError(w, err) {
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("memory %q not found", name), http.StatusNotFound)
		return
	}
	if s.memoryBackends == nil {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []any{}, "count": 0})
		return
	}
	backend, ok := s.memoryBackends.Get(key)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []any{}, "count": 0})
		return
	}

	q := r.URL.Query()
	query := strings.TrimSpace(q.Get("q"))
	prefix := strings.TrimSpace(q.Get("prefix"))
	limitStr := strings.TrimSpace(q.Get("limit"))
	limit := 100
	if limitStr != "" {
		if v, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || v != 1 || limit <= 0 {
			limit = 100
		}
	}

	ctx := r.Context()
	if query != "" {
		results, err := backend.Search(ctx, query, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"entries": results, "count": len(results)})
		return
	}
	results, err := backend.List(ctx, prefix)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(results) > limit {
		results = results[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": results, "count": len(results)})
}

func (s *Server) handlePolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.Policies.ListCursor, func(item resources.AgentPolicy) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.AgentPolicyList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseAgentPolicyManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.Policies.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			if s.checkCRDConflict(w, existing.Metadata, "AgentPolicy") {
				return
			}
			obj.Status = existing.Status
		}
		obj, err = s.stores.Policies.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("AgentPolicy", obj.Metadata.Name)
		s.publishResourceEvent("AgentPolicy", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePolicyByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/agent-policies/"), "/")
	if name == "" {
		http.Error(w, "policy name is required", http.StatusBadRequest)
		return
	}
	if strings.HasSuffix(name, "/status") {
		base := strings.Trim(strings.TrimSuffix(name, "/status"), "/")
		if base == "" {
			http.Error(w, "policy name is required", http.StatusBadRequest)
			return
		}
		s.handlePolicyStatusByName(w, r, base)
		return
	}
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.Policies.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("agentpolicy %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		existing, ok, err := s.stores.Policies.Get(r.Context(), key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("agentpolicy %q not found", name), http.StatusNotFound)
			return
		}
		if s.checkCRDConflict(w, existing.Metadata, "AgentPolicy") {
			return
		}
		if err := s.stores.Policies.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("AgentPolicy", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseAgentPolicyManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.Policies.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("agentpolicy %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if s.checkCRDConflict(w, current.Metadata, "AgentPolicy") {
			return
		}
		obj.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		if bodyKey != key {
			obj, err = s.stores.Policies.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.Policies.Upsert(r.Context(), obj)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("AgentPolicy", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgentRoles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.AgentRoles.ListCursor, func(item resources.AgentRole) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.AgentRoleList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseAgentRoleManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.AgentRoles.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.AgentRoles.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("AgentRole", obj.Metadata.Name)
		s.publishResourceEvent("AgentRole", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgentRoleByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/agent-roles/"), "/")
	if name == "" {
		http.Error(w, "agent role name is required", http.StatusBadRequest)
		return
	}
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.AgentRoles.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("agentrole %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.AgentRoles.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("AgentRole", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseAgentRoleManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.AgentRoles.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("agentrole %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		if bodyKey != key {
			obj, err = s.stores.AgentRoles.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.AgentRoles.Upsert(r.Context(), obj)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("AgentRole", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleToolPermissions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.ToolPerms.ListCursor, func(item resources.ToolPermission) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.ToolPermissionList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseToolPermissionManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.ToolPerms.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.ToolPerms.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("ToolPermission", obj.Metadata.Name)
		s.publishResourceEvent("ToolPermission", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleToolPermissionByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/tool-permissions/"), "/")
	if name == "" {
		http.Error(w, "tool permission name is required", http.StatusBadRequest)
		return
	}
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.ToolPerms.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("toolpermission %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.ToolPerms.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("ToolPermission", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseToolPermissionManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.ToolPerms.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("toolpermission %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		if bodyKey != key {
			obj, err = s.stores.ToolPerms.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.ToolPerms.Upsert(r.Context(), obj)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("ToolPermission", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleToolApprovals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.ToolApprovals.ListCursor, func(item resources.ToolApproval) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.ToolApprovalList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		var obj resources.ToolApproval
		if err := json.Unmarshal(body, &obj); err != nil {
			http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.ToolApprovals.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.ToolApprovals.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("ToolApproval", obj.Metadata.Name)
		s.publishResourceEvent("ToolApproval", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleToolApprovalByName(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/tool-approvals/"), "/")
	if path == "" {
		http.Error(w, "tool approval name is required", http.StatusBadRequest)
		return
	}
	if strings.HasSuffix(path, "/approve") {
		name := strings.TrimSuffix(path, "/approve")
		s.handleToolApprovalDecision(w, r, name, "Approved", "approved")
		return
	}
	if strings.HasSuffix(path, "/deny") {
		name := strings.TrimSuffix(path, "/deny")
		s.handleToolApprovalDecision(w, r, name, "Denied", "denied")
		return
	}
	name := path
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.ToolApprovals.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("toolapproval %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.ToolApprovals.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("ToolApproval", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type approvalDecisionRequest struct {
	DecidedBy string `json:"decided_by"`
	Comment   string `json:"comment"`
	Reason    string `json:"reason"`
}

func readApprovalDecisionRequest(r *http.Request) approvalDecisionRequest {
	var body approvalDecisionRequest
	if r == nil || r.Body == nil {
		return body
	}
	raw, _ := io.ReadAll(r.Body)
	if len(raw) == 0 {
		return body
	}
	_ = json.Unmarshal(raw, &body)
	if strings.TrimSpace(body.Comment) == "" {
		body.Comment = strings.TrimSpace(body.Reason)
	}
	return body
}

func (s *Server) handleToolApprovalDecision(w http.ResponseWriter, r *http.Request, name, phase, decision string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body := readApprovalDecisionRequest(r)

	key := scopedNameForRequest(r, name)
	// Retry optimistic-concurrency loop: read, check phase, CAS-write.
	// This eliminates the TOCTOU race where two concurrent approve/deny
	// requests could both pass the Pending guard and both commit.
	for attempt := 0; attempt < 5; attempt++ {
		obj, ok, err := s.stores.ToolApprovals.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("toolapproval %q not found", name), http.StatusNotFound)
			return
		}
		if obj.Status.Phase != "Pending" {
			http.Error(w, fmt.Sprintf("toolapproval %q is already %s", name, obj.Status.Phase), http.StatusConflict)
			return
		}
		obj.Status.Phase = phase
		obj.Status.Decision = decision
		obj.Status.DecidedBy = body.DecidedBy
		obj.Status.DecidedAt = time.Now().UTC().Format(time.RFC3339)
		obj.Status.Comment = body.Comment
		updated, err := s.stores.ToolApprovals.Upsert(r.Context(), obj)
		if err != nil {
			if store.IsConflict(err) {
				continue
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("ToolApproval", updated.Metadata.Name, decision, updated)
		writeJSON(w, http.StatusOK, updated)
		return
	}
	http.Error(w, "conflict updating tool approval, please retry", http.StatusConflict)
}

func (s *Server) handleTaskApprovals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.TaskApprovals.ListCursor, func(item resources.TaskApproval) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.TaskApprovalList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		var obj resources.TaskApproval
		if err := json.Unmarshal(body, &obj); err != nil {
			http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.TaskApprovals.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.TaskApprovals.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("TaskApproval", obj.Metadata.Name)
		s.publishResourceEvent("TaskApproval", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTaskApprovalByName(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/task-approvals/"), "/")
	if path == "" {
		http.Error(w, "task approval name is required", http.StatusBadRequest)
		return
	}
	if strings.HasSuffix(path, "/approve") {
		name := strings.TrimSuffix(path, "/approve")
		s.handleTaskApprovalDecision(w, r, name, "Approved", "approved")
		return
	}
	if strings.HasSuffix(path, "/deny") {
		name := strings.TrimSuffix(path, "/deny")
		s.handleTaskApprovalDecision(w, r, name, "Denied", "denied")
		return
	}
	if strings.HasSuffix(path, "/request-changes") {
		name := strings.TrimSuffix(path, "/request-changes")
		s.handleTaskApprovalDecision(w, r, name, "ChangesRequested", "request_changes")
		return
	}
	name := path
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.TaskApprovals.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("taskapproval %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.TaskApprovals.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("TaskApproval", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTaskApprovalDecision(w http.ResponseWriter, r *http.Request, name, phase, decision string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body := readApprovalDecisionRequest(r)
	if decision == "request_changes" && strings.TrimSpace(body.Comment) == "" {
		http.Error(w, "comment is required for request_changes", http.StatusBadRequest)
		return
	}

	key := scopedNameForRequest(r, name)
	for attempt := 0; attempt < 5; attempt++ {
		obj, ok, err := s.stores.TaskApprovals.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("taskapproval %q not found", name), http.StatusNotFound)
			return
		}
		if obj.Status.Phase != "Pending" {
			http.Error(w, fmt.Sprintf("taskapproval %q is already %s", name, obj.Status.Phase), http.StatusConflict)
			return
		}
		if decision == "request_changes" {
			if !resources.TaskApprovalAllowsRequestChanges(obj) {
				http.Error(w, fmt.Sprintf("taskapproval %q does not allow request_changes", name), http.StatusConflict)
				return
			}
			if obj.Spec.ReviewCycle >= resources.TaskApprovalMaxReviewCycles(obj) {
				http.Error(w, fmt.Sprintf("taskapproval %q has reached max_review_cycles", name), http.StatusConflict)
				return
			}
		}
		obj.Status.Phase = phase
		obj.Status.Decision = decision
		obj.Status.DecidedBy = body.DecidedBy
		obj.Status.DecidedAt = time.Now().UTC().Format(time.RFC3339)
		obj.Status.Comment = body.Comment
		updated, err := s.stores.TaskApprovals.Upsert(r.Context(), obj)
		if err != nil {
			if store.IsConflict(err) {
				continue
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("TaskApproval", updated.Metadata.Name, decision, updated)
		writeJSON(w, http.StatusOK, updated)
		return
	}
	http.Error(w, "conflict updating task approval, please retry", http.StatusConflict)
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, offset := paginationParams(r)
		after := normalizeListCursor(cursorParam(r), requestNamespace(r))
		if after != "" || offset == 0 {
			items, cont, err := fetchListPage(r.Context(), r, s.stores.Tasks.ListCursor, func(item resources.Task) resources.ObjectMeta { return item.Metadata })
			if writeListPageError(w, err) {
				return
			}
			writeJSON(w, http.StatusOK, resources.TaskList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
			return
		}
		ns, hasNS := namespaceFilter(r)
		nsFilter := ""
		if hasNS {
			nsFilter = ns
		}
		items, err := s.stores.Tasks.ListPaged(r.Context(), limit, offset, nsFilter)
		if writeStoreFetchError(w, err) {
			return
		}
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(selector) > 0 {
			filtered := make([]resources.Task, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, "", false, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.TaskList{Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseTaskManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.Tasks.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		rerun := r.URL.Query().Get("rerun") == "true"
		if ok && rerun {
			phase := strings.ToLower(strings.TrimSpace(existing.Status.Phase))
			switch phase {
			case "deadletter", "failed", "succeeded":
				obj.Metadata.Name = rerunTaskName(obj.Metadata.Name)
				if obj.Metadata.Labels == nil {
					obj.Metadata.Labels = make(map[string]string)
				}
				obj.Metadata.Labels["orloj.dev/source-task"] = existing.Metadata.Name
				obj.Metadata.Labels["orloj.dev/source-task-namespace"] = resources.NormalizeNamespace(existing.Metadata.Namespace)
			case "pending", "running", "", "waitingapproval":
				http.Error(w, fmt.Sprintf("task/%s is still active (phase=%s); cannot rerun", existing.Metadata.Name, existing.Status.Phase), http.StatusConflict)
				return
			}
		} else if ok {
			obj.Status = existing.Status
		}
		// Stamp W3C trace context so task execution spans link back to this request.
		if obj.Metadata.Annotations == nil {
			obj.Metadata.Annotations = make(map[string]string)
		}
		telemetry.InjectTraceContext(r.Context(), obj.Metadata.Annotations)
		obj, err = s.stores.Tasks.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("Task", obj.Metadata.Name)
		s.publishResourceEvent("Task", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func rerunTaskName(name string) string {
	base := name
	if len(base) > 40 {
		base = strings.TrimRight(base[:40], "-")
	}
	return fmt.Sprintf("%s-run-%d", base, time.Now().UnixMilli())
}

func (s *Server) handleTaskByName(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/tasks/"), "/")
	if path == "" {
		http.Error(w, "task name is required", http.StatusBadRequest)
		return
	}
	if path == "watch" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.watchTasks(w, r)
		return
	}
	if strings.HasSuffix(path, "/logs") {
		name := strings.Trim(strings.TrimSuffix(path, "/logs"), "/")
		if name == "" {
			http.Error(w, "task name is required", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.getTaskLogs(w, r.Context(), scopedNameForRequest(r, name), name)
		return
	}
	if strings.HasSuffix(path, "/messages") {
		name := strings.Trim(strings.TrimSuffix(path, "/messages"), "/")
		if name == "" {
			http.Error(w, "task name is required", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.getTaskMessages(w, r, name)
		return
	}
	if strings.HasSuffix(path, "/metrics") {
		name := strings.Trim(strings.TrimSuffix(path, "/metrics"), "/")
		if name == "" {
			http.Error(w, "task name is required", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.getTaskMessageMetrics(w, r, name)
		return
	}
	if strings.HasSuffix(path, "/status") {
		name := strings.Trim(strings.TrimSuffix(path, "/status"), "/")
		if name == "" {
			http.Error(w, "task name is required", http.StatusBadRequest)
			return
		}
		s.handleTaskStatusByName(w, r, name)
		return
	}

	name := path
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.Tasks.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("task %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.Tasks.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("Task", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseTaskManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.Tasks.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("task %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		// Preserve origin trace context set at creation time.
		if tp, ok := current.Metadata.Annotations[telemetry.AnnotationTraceparent]; ok {
			if obj.Metadata.Annotations == nil {
				obj.Metadata.Annotations = make(map[string]string)
			}
			obj.Metadata.Annotations[telemetry.AnnotationTraceparent] = tp
			if ts, ok := current.Metadata.Annotations[telemetry.AnnotationTracestate]; ok {
				obj.Metadata.Annotations[telemetry.AnnotationTracestate] = ts
			}
		}
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		if bodyKey != key {
			obj, err = s.stores.Tasks.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.Tasks.Upsert(r.Context(), obj)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("Task", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getTaskLogs(w http.ResponseWriter, ctx context.Context, key, name string) {
	logs, err := s.stores.Tasks.GetLogs(ctx, key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name": name,
		"logs": logs,
	})
}

func (s *Server) handleTaskSchedules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.TaskSchedules.ListCursor, func(item resources.TaskSchedule) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.TaskScheduleList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseTaskScheduleManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.TaskSchedules.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.TaskSchedules.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("TaskSchedule", obj.Metadata.Name)
		s.publishResourceEvent("TaskSchedule", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTaskScheduleByName(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/task-schedules/"), "/")
	if path == "" {
		http.Error(w, "task schedule name is required", http.StatusBadRequest)
		return
	}
	if path == "watch" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.watchTaskSchedules(w, r)
		return
	}
	if strings.HasSuffix(path, "/status") {
		name := strings.Trim(strings.TrimSuffix(path, "/status"), "/")
		if name == "" {
			http.Error(w, "task schedule name is required", http.StatusBadRequest)
			return
		}
		s.handleTaskScheduleStatusByName(w, r, name)
		return
	}

	name := path
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.TaskSchedules.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("taskschedule %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.TaskSchedules.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("TaskSchedule", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseTaskScheduleManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.TaskSchedules.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("taskschedule %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		if bodyKey != key {
			obj, err = s.stores.TaskSchedules.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.TaskSchedules.Upsert(r.Context(), obj)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("TaskSchedule", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorkers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.Workers.ListCursor, func(item resources.Worker) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.WorkerList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseWorkerManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.Workers.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.Workers.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("Worker", obj.Metadata.Name)
		s.publishResourceEvent("Worker", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorkerByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/workers/"), "/")
	if name == "" {
		http.Error(w, "worker name is required", http.StatusBadRequest)
		return
	}
	if strings.HasSuffix(name, "/status") {
		base := strings.Trim(strings.TrimSuffix(name, "/status"), "/")
		if base == "" {
			http.Error(w, "worker name is required", http.StatusBadRequest)
			return
		}
		s.handleWorkerStatusByName(w, r, base)
		return
	}
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.Workers.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("worker %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.Workers.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("Worker", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseWorkerManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.Workers.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("worker %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		if bodyKey != key {
			obj, err = s.stores.Workers.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.Workers.Upsert(r.Context(), obj)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("Worker", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) withRuntimeStatus(agent resources.Agent) resources.Agent {
	if s.runtime.IsRunning(store.ScopedName(agent.Metadata.Namespace, agent.Metadata.Name)) {
		agent.Status.Phase = "Running"
	} else {
		agent.Status.Phase = "Ready"
	}
	return agent
}

func (s *Server) logApply(kind, name string) {
	if s.logger != nil {
		s.logger.Printf("applied %s/%s", kind, name)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleMcpServers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.McpServers.ListCursor, func(item resources.McpServer) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.McpServerList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseMcpServerManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.validateMcpServerContainerResources(obj); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.McpServers.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			if s.checkCRDConflict(w, existing.Metadata, "McpServer") {
				return
			}
			obj.Status = existing.Status
		}
		obj, err = s.stores.McpServers.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("McpServer", obj.Metadata.Name)
		s.publishResourceEvent("McpServer", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMcpServerByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/mcp-servers/"), "/")
	if name == "" {
		http.Error(w, "mcp-server name is required", http.StatusBadRequest)
		return
	}
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.McpServers.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("mcp-server %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseMcpServerManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.validateMcpServerContainerResources(obj); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.McpServers.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("mcp-server %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if s.checkCRDConflict(w, current.Metadata, "McpServer") {
			return
		}
		obj.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		if bodyKey != key {
			obj, err = s.stores.McpServers.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.McpServers.Upsert(r.Context(), obj)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("McpServer", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		existing, ok, err := s.stores.McpServers.Get(r.Context(), key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("mcp-server %q not found", name), http.StatusNotFound)
			return
		}
		if s.checkCRDConflict(w, existing.Metadata, "McpServer") {
			return
		}
		if err := s.stores.McpServers.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("McpServer", name, "deleted", nil)
		writeJSON(w, http.StatusOK, map[string]string{"deleted": name})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
