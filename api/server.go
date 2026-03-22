package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
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
	Agents        *store.AgentStore
	AgentSystems  *store.AgentSystemStore
	ModelEPs      *store.ModelEndpointStore
	Tools         *store.ToolStore
	Secrets       *store.SecretStore
	Memories      *store.MemoryStore
	Policies      *store.AgentPolicyStore
	AgentRoles    *store.AgentRoleStore
	ToolPerms     *store.ToolPermissionStore
	ToolApprovals *store.ToolApprovalStore
	Tasks         *store.TaskStore
	TaskSchedules *store.TaskScheduleStore
	TaskWebhooks  *store.TaskWebhookStore
	WebhookDedupe *store.WebhookDedupeStore
	Workers       *store.WorkerStore
	McpServers    *store.McpServerStore
	LocalAdmins   *store.LocalAdminStore
	AuthSessions  *store.AuthSessionStore
}

// ServerOptions configures optional extension points.
type ServerOptions struct {
	Authorizer         RequestAuthorizer
	ResourceAuthorizer ResourceAuthorizer // optional; enterprise RBAC hook
	Extensions         agentruntime.Extensions
	AuthMode           AuthMode
	SessionTTL         time.Duration
}

// Server exposes CRUD endpoints for control plane resources.
type Server struct {
	stores             Stores
	runtime            *agentruntime.Manager
	logger             *log.Logger
	mux                *http.ServeMux
	authorizer         RequestAuthorizer
	resourceAuthorizer ResourceAuthorizer // nil in OSS; enterprise RBAC hook
	authMode           AuthMode
	sessionTTL         time.Duration
	bus                eventbus.Bus
	extensions         agentruntime.Extensions
	memoryBackends     *agentruntime.PersistentMemoryBackendRegistry
	authRateLimiter    *authRateLimiter
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
	if stores.Secrets == nil {
		stores.Secrets = store.NewSecretStore()
	}
	if stores.TaskSchedules == nil {
		stores.TaskSchedules = store.NewTaskScheduleStore()
	}
	if stores.TaskWebhooks == nil {
		stores.TaskWebhooks = store.NewTaskWebhookStore()
	}
	if stores.WebhookDedupe == nil {
		stores.WebhookDedupe = store.NewWebhookDedupeStore()
	}
	if stores.McpServers == nil {
		stores.McpServers = store.NewMcpServerStore()
	}
	if stores.LocalAdmins == nil {
		stores.LocalAdmins = store.NewLocalAdminStore()
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
	if authMode == AuthModeOff && authModeExplicit {
		authorizer = noAuthAuthorizer{}
	} else if authorizer == nil {
		authorizer = newTokenAuthorizerFromEnv()
	}
	if authMode == AuthModeNative {
		authorizer = newNativeModeAuthorizer(authorizer, stores.LocalAdmins, stores.AuthSessions, sessionTTL)
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
		authRateLimiter:    newAuthRateLimiter(),
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

// globalRateLimiter caps the API server to 500 requests/second with a burst
// of 100 to handle short spikes. This is a simple global limiter; deploy a
// reverse proxy for per-client or per-IP limiting at scale.
var globalRateLimiter = rate.NewLimiter(rate.Limit(500), 100)

func withRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !globalRateLimiter.Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Handler() http.Handler {
	return withRateLimit(s.withBodyLimit(s.withAuth(s.mux)))
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.Handle("/metrics", promhttp.Handler())
	s.mux.HandleFunc("/v1/capabilities", s.handleCapabilities)
	s.mux.HandleFunc("/v1/auth/config", s.handleAuthConfig)
	s.mux.HandleFunc("/v1/auth/setup", s.handleAuthSetup)
	s.mux.HandleFunc("/v1/auth/login", s.handleAuthLogin)
	s.mux.HandleFunc("/v1/auth/logout", s.handleAuthLogout)
	s.mux.HandleFunc("/v1/auth/me", s.handleAuthMe)
	s.mux.HandleFunc("/v1/auth/change-password", s.handleAuthChangePassword)
	s.mux.HandleFunc("/v1/auth/admin/reset-password", s.handleAuthAdminResetPassword)
	s.mux.HandleFunc("/ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
	})
	s.mux.Handle("/ui/", http.StripPrefix("/ui/", frontend.Handler()))

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

	s.mux.HandleFunc("/v1/memories", s.handleMemories)
	s.mux.HandleFunc("/v1/memories/", s.handleMemoryByName)

	s.mux.HandleFunc("/v1/agent-policies", s.handlePolicies)
	s.mux.HandleFunc("/v1/agent-policies/", s.handlePolicyByName)

	s.mux.HandleFunc("/v1/agent-roles", s.handleAgentRoles)
	s.mux.HandleFunc("/v1/agent-roles/", s.handleAgentRoleByName)

	s.mux.HandleFunc("/v1/tool-permissions", s.handleToolPermissions)
	s.mux.HandleFunc("/v1/tool-permissions/", s.handleToolPermissionByName)

	s.mux.HandleFunc("/v1/tool-approvals", s.handleToolApprovals)
	s.mux.HandleFunc("/v1/tool-approvals/", s.handleToolApprovalByName)

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
	if agents, err := s.stores.Agents.List(); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range agents {
			collect(item.Metadata.Namespace)
		}
	}
	if agentSystems, err := s.stores.AgentSystems.List(); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range agentSystems {
			collect(item.Metadata.Namespace)
		}
	}
	if modelEPs, err := s.stores.ModelEPs.List(); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range modelEPs {
			collect(item.Metadata.Namespace)
		}
	}
	if tools, err := s.stores.Tools.List(); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range tools {
			collect(item.Metadata.Namespace)
		}
	}
	if secrets, err := s.stores.Secrets.List(); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range secrets {
			collect(item.Metadata.Namespace)
		}
	}
	if memories, err := s.stores.Memories.List(); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range memories {
			collect(item.Metadata.Namespace)
		}
	}
	if policies, err := s.stores.Policies.List(); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range policies {
			collect(item.Metadata.Namespace)
		}
	}
	if tasks, err := s.stores.Tasks.List(); writeStoreFetchError(w, err) {
		return
	} else {
		for _, item := range tasks {
			collect(item.Metadata.Namespace)
		}
	}
	if workers, err := s.stores.Workers.List(); writeStoreFetchError(w, err) {
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
		s.getAgentLogs(w, scopedNameForRequest(r, name), name)
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
		s.getAgent(w, key, name)
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
		current, ok, err := s.stores.Agents.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("agent %q not found", name), http.StatusNotFound)
			return
		}
		agent.Metadata.Name = name
		if err := applyRequestNamespace(r, &agent.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &agent.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		agent.Status = current.Status
		agent, err = s.stores.Agents.Upsert(agent)
		if err != nil {
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
	existing, ok, err := s.stores.Agents.Get(store.ScopedName(agent.Metadata.Namespace, agent.Metadata.Name))
	if writeStoreFetchError(w, err) { return }
	if ok {
		agent.Status = existing.Status
	}
	agent, err = s.stores.Agents.Upsert(agent)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.logApply("Agent", agent.Metadata.Name)
	s.publishResourceEvent("Agent", agent.Metadata.Name, "created", s.withRuntimeStatus(agent))
	writeJSON(w, http.StatusCreated, s.withRuntimeStatus(agent))
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.stores.Agents.List()
	if writeStoreFetchError(w, err) { return }
	ns, hasNS := namespaceFilter(r)
	selector, err := labelSelectorFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if hasNS || len(selector) > 0 {
		filtered := make([]resources.Agent, 0, len(agents))
		for _, agent := range agents {
			if !matchMetadataFilters(agent.Metadata, ns, hasNS, selector) {
				continue
			}
			filtered = append(filtered, agent)
		}
		agents = filtered
	}
	for i := range agents {
		agents[i] = s.withRuntimeStatus(agents[i])
	}
	writeJSON(w, http.StatusOK, resources.AgentList{Items: agents})
}

func (s *Server) getAgent(w http.ResponseWriter, key, name string) {
	agent, ok, err := s.stores.Agents.Get(key)
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("agent %q not found", name), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, s.withRuntimeStatus(agent))
}

func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request, key, name string) {
	if err := s.stores.Agents.Delete(key); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.runtime.Stop(key)
	s.publishResourceEvent("Agent", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getAgentLogs(w http.ResponseWriter, key, name string) {
	_, ok, err := s.stores.Agents.Get(key)
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("agent %q not found", name), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "logs": s.runtime.Logs(key)})
}

func (s *Server) handleAgentSystems(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.stores.AgentSystems.List()
		if writeStoreFetchError(w, err) { return }
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.AgentSystem, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.AgentSystemList{Items: items})
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
		existing, ok, err := s.stores.AgentSystems.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.AgentSystems.Upsert(obj)
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
		obj, ok, err := s.stores.AgentSystems.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("agentsystem %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.AgentSystems.Delete(key); err != nil {
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
		current, ok, err := s.stores.AgentSystems.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("agentsystem %q not found", name), http.StatusNotFound)
			return
		}
		obj.Metadata.Name = name
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.AgentSystems.Upsert(obj)
		if err != nil {
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
		items, err := s.stores.ModelEPs.List()
		if writeStoreFetchError(w, err) { return }
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.ModelEndpoint, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.ModelEndpointList{Items: items})
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
		existing, ok, err := s.stores.ModelEPs.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.ModelEPs.Upsert(obj)
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
		obj, ok, err := s.stores.ModelEPs.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("modelendpoint %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.ModelEPs.Delete(key); err != nil {
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
		current, ok, err := s.stores.ModelEPs.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("modelendpoint %q not found", name), http.StatusNotFound)
			return
		}
		obj.Metadata.Name = name
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.ModelEPs.Upsert(obj)
		if err != nil {
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
		items, err := s.stores.Tools.List()
		if writeStoreFetchError(w, err) { return }
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.Tool, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.ToolList{Items: items})
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
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.Tools.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.Tools.Upsert(obj)
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
		obj, ok, err := s.stores.Tools.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("tool %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.Tools.Delete(key); err != nil {
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
		current, ok, err := s.stores.Tools.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("tool %q not found", name), http.StatusNotFound)
			return
		}
		obj.Metadata.Name = name
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.Tools.Upsert(obj)
		if err != nil {
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
		items, err := s.stores.Secrets.List()
		if writeStoreFetchError(w, err) { return }
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.Secret, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		for i := range items {
			redactSecretData(&items[i])
		}
		writeJSON(w, http.StatusOK, resources.SecretList{Items: items})
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
		existing, ok, err := s.stores.Secrets.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.Secrets.Upsert(obj)
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
		obj, ok, err := s.stores.Secrets.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("secret %q not found", name), http.StatusNotFound)
			return
		}
		redactSecretData(&obj)
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.Secrets.Delete(key); err != nil {
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
		obj, err := resources.ParseSecretManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.Secrets.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("secret %q not found", name), http.StatusNotFound)
			return
		}
		obj.Metadata.Name = name
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.Secrets.Upsert(obj)
		if err != nil {
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
		items, err := s.stores.Memories.List()
		if writeStoreFetchError(w, err) { return }
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.Memory, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.MemoryList{Items: items})
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
		existing, ok, err := s.stores.Memories.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.Memories.Upsert(obj)
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
		obj, ok, err := s.stores.Memories.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("memory %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.Memories.Delete(key); err != nil {
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
		current, ok, err := s.stores.Memories.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("memory %q not found", name), http.StatusNotFound)
			return
		}
		obj.Metadata.Name = name
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.Memories.Upsert(obj)
		if err != nil {
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
	_, ok, err := s.stores.Memories.Get(key)
	if writeStoreFetchError(w, err) { return }
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
		items, err := s.stores.Policies.List()
		if writeStoreFetchError(w, err) { return }
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.AgentPolicy, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.AgentPolicyList{Items: items})
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
		existing, ok, err := s.stores.Policies.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.Policies.Upsert(obj)
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
		obj, ok, err := s.stores.Policies.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("agentpolicy %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.Policies.Delete(key); err != nil {
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
		current, ok, err := s.stores.Policies.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("agentpolicy %q not found", name), http.StatusNotFound)
			return
		}
		obj.Metadata.Name = name
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.Policies.Upsert(obj)
		if err != nil {
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
		items, err := s.stores.AgentRoles.List()
		if writeStoreFetchError(w, err) { return }
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.AgentRole, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.AgentRoleList{Items: items})
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
		existing, ok, err := s.stores.AgentRoles.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.AgentRoles.Upsert(obj)
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
		obj, ok, err := s.stores.AgentRoles.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("agentrole %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.AgentRoles.Delete(key); err != nil {
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
		current, ok, err := s.stores.AgentRoles.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("agentrole %q not found", name), http.StatusNotFound)
			return
		}
		obj.Metadata.Name = name
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.AgentRoles.Upsert(obj)
		if err != nil {
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
		items, err := s.stores.ToolPerms.List()
		if writeStoreFetchError(w, err) { return }
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.ToolPermission, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.ToolPermissionList{Items: items})
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
		existing, ok, err := s.stores.ToolPerms.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.ToolPerms.Upsert(obj)
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
		obj, ok, err := s.stores.ToolPerms.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("toolpermission %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.ToolPerms.Delete(key); err != nil {
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
		current, ok, err := s.stores.ToolPerms.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("toolpermission %q not found", name), http.StatusNotFound)
			return
		}
		obj.Metadata.Name = name
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.ToolPerms.Upsert(obj)
		if err != nil {
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
		items, err := s.stores.ToolApprovals.List()
		if writeStoreFetchError(w, err) { return }
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.ToolApproval, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.ToolApprovalList{Items: items})
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
		existing, ok, err := s.stores.ToolApprovals.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.ToolApprovals.Upsert(obj)
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
		obj, ok, err := s.stores.ToolApprovals.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("toolapproval %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.ToolApprovals.Delete(key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("ToolApproval", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleToolApprovalDecision(w http.ResponseWriter, r *http.Request, name, phase, decision string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		DecidedBy string `json:"decided_by"`
	}
	if r.Body != nil {
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
		}
	}

	key := scopedNameForRequest(r, name)
	// Retry optimistic-concurrency loop: read, check phase, CAS-write.
	// This eliminates the TOCTOU race where two concurrent approve/deny
	// requests could both pass the Pending guard and both commit.
	for attempt := 0; attempt < 5; attempt++ {
		obj, ok, err := s.stores.ToolApprovals.Get(key)
		if writeStoreFetchError(w, err) { return }
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
		updated, err := s.stores.ToolApprovals.Upsert(obj)
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

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, offset := paginationParams(r)
		items, err := s.stores.Tasks.ListPaged(limit, offset)
		if writeStoreFetchError(w, err) { return }
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.Task, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
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
		existing, ok, err := s.stores.Tasks.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		// Stamp W3C trace context so task execution spans link back to this request.
		if obj.Metadata.Annotations == nil {
			obj.Metadata.Annotations = make(map[string]string)
		}
		telemetry.InjectTraceContext(r.Context(), obj.Metadata.Annotations)
		obj, err = s.stores.Tasks.Upsert(obj)
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
		s.getTaskLogs(w, scopedNameForRequest(r, name), name)
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
		obj, ok, err := s.stores.Tasks.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("task %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.Tasks.Delete(key); err != nil {
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
		current, ok, err := s.stores.Tasks.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("task %q not found", name), http.StatusNotFound)
			return
		}
		obj.Metadata.Name = name
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
		obj, err = s.stores.Tasks.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("Task", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getTaskLogs(w http.ResponseWriter, key, name string) {
	logs, err := s.stores.Tasks.Logs(key)
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
		items, err := s.stores.TaskSchedules.List()
		if writeStoreFetchError(w, err) { return }
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.TaskSchedule, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.TaskScheduleList{Items: items})
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
		existing, ok, err := s.stores.TaskSchedules.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.TaskSchedules.Upsert(obj)
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
		obj, ok, err := s.stores.TaskSchedules.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("taskschedule %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.TaskSchedules.Delete(key); err != nil {
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
		current, ok, err := s.stores.TaskSchedules.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("taskschedule %q not found", name), http.StatusNotFound)
			return
		}
		obj.Metadata.Name = name
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.TaskSchedules.Upsert(obj)
		if err != nil {
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
		items, err := s.stores.Workers.List()
		if writeStoreFetchError(w, err) { return }
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.Worker, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.WorkerList{Items: items})
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
		existing, ok, err := s.stores.Workers.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.Workers.Upsert(obj)
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
		obj, ok, err := s.stores.Workers.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("worker %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.Workers.Delete(key); err != nil {
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
		current, ok, err := s.stores.Workers.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("worker %q not found", name), http.StatusNotFound)
			return
		}
		obj.Metadata.Name = name
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.Workers.Upsert(obj)
		if err != nil {
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
		items, err := s.stores.McpServers.List()
		if writeStoreFetchError(w, err) { return }
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.McpServer, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.McpServerList{Items: items})
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
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.McpServers.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.McpServers.Upsert(obj)
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
		obj, ok, err := s.stores.McpServers.Get(key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("mcp-server %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.McpServers.Delete(key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("McpServer", name, "deleted", nil)
		writeJSON(w, http.StatusOK, map[string]string{"deleted": name})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
