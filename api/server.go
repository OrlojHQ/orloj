package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/crds"
	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/frontend"
	"github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
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
	Tasks         *store.TaskStore
	TaskSchedules *store.TaskScheduleStore
	TaskWebhooks  *store.TaskWebhookStore
	WebhookDedupe *store.WebhookDedupeStore
	Workers       *store.WorkerStore
}

// ServerOptions configures optional extension points.
type ServerOptions struct {
	Authorizer RequestAuthorizer
	Extensions agentruntime.Extensions
}

// Server exposes CRUD endpoints for control plane resources.
type Server struct {
	stores     Stores
	runtime    *agentruntime.Manager
	logger     *log.Logger
	mux        *http.ServeMux
	authorizer RequestAuthorizer
	bus        eventbus.Bus
	extensions agentruntime.Extensions
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
	extensions := agentruntime.NormalizeExtensions(opts.Extensions)
	authorizer := opts.Authorizer
	if authorizer == nil {
		authorizer = newTokenAuthorizerFromEnv()
	}
	s := &Server{
		stores:     stores,
		runtime:    runtime,
		logger:     logger,
		mux:        http.NewServeMux(),
		authorizer: authorizer,
		bus:        eventbus.NewMemoryBus(4096),
		extensions: extensions,
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

func (s *Server) Handler() http.Handler {
	return s.withAuth(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.Handle("/metrics", promhttp.Handler())
	s.mux.HandleFunc("/v1/capabilities", s.handleCapabilities)
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
		agent, err := crds.ParseAgentManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok := s.stores.Agents.Get(key)
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
	agent, err := crds.ParseAgentManifest(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := applyRequestNamespace(r, &agent.Metadata); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if existing, ok := s.stores.Agents.Get(store.ScopedName(agent.Metadata.Namespace, agent.Metadata.Name)); ok {
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
	agents := s.stores.Agents.List()
	ns, hasNS := namespaceFilter(r)
	selector, err := labelSelectorFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if hasNS || len(selector) > 0 {
		filtered := make([]crds.Agent, 0, len(agents))
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
	writeJSON(w, http.StatusOK, crds.AgentList{Items: agents})
}

func (s *Server) getAgent(w http.ResponseWriter, key, name string) {
	agent, ok := s.stores.Agents.Get(key)
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
	s.runtime.Stop(name)
	s.publishResourceEvent("Agent", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getAgentLogs(w http.ResponseWriter, key, name string) {
	if _, ok := s.stores.Agents.Get(key); !ok {
		http.Error(w, fmt.Sprintf("agent %q not found", name), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "logs": s.runtime.Logs(key)})
}

func (s *Server) handleAgentSystems(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items := s.stores.AgentSystems.List()
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]crds.AgentSystem, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, crds.AgentSystemList{Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := crds.ParseAgentSystemManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if existing, ok := s.stores.AgentSystems.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name)); ok {
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
		obj, ok := s.stores.AgentSystems.Get(key)
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
		obj, err := crds.ParseAgentSystemManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok := s.stores.AgentSystems.Get(key)
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
		items := s.stores.ModelEPs.List()
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]crds.ModelEndpoint, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, crds.ModelEndpointList{Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := crds.ParseModelEndpointManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if existing, ok := s.stores.ModelEPs.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name)); ok {
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
		obj, ok := s.stores.ModelEPs.Get(key)
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
		obj, err := crds.ParseModelEndpointManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok := s.stores.ModelEPs.Get(key)
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
		items := s.stores.Tools.List()
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]crds.Tool, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, crds.ToolList{Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := crds.ParseToolManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if existing, ok := s.stores.Tools.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name)); ok {
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
		obj, ok := s.stores.Tools.Get(key)
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
		obj, err := crds.ParseToolManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok := s.stores.Tools.Get(key)
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
		items := s.stores.Secrets.List()
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]crds.Secret, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, crds.SecretList{Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := crds.ParseSecretManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if existing, ok := s.stores.Secrets.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name)); ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.Secrets.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("Secret", obj.Metadata.Name)
		s.publishResourceEvent("Secret", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
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
		obj, ok := s.stores.Secrets.Get(key)
		if !ok {
			http.Error(w, fmt.Sprintf("secret %q not found", name), http.StatusNotFound)
			return
		}
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
		obj, err := crds.ParseSecretManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok := s.stores.Secrets.Get(key)
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
		s.publishResourceEvent("Secret", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMemories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items := s.stores.Memories.List()
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]crds.Memory, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, crds.MemoryList{Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := crds.ParseMemoryManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if existing, ok := s.stores.Memories.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name)); ok {
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
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok := s.stores.Memories.Get(key)
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
		obj, err := crds.ParseMemoryManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok := s.stores.Memories.Get(key)
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

func (s *Server) handlePolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items := s.stores.Policies.List()
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]crds.AgentPolicy, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, crds.AgentPolicyList{Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := crds.ParseAgentPolicyManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if existing, ok := s.stores.Policies.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name)); ok {
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
		obj, ok := s.stores.Policies.Get(key)
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
		obj, err := crds.ParseAgentPolicyManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok := s.stores.Policies.Get(key)
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
		items := s.stores.AgentRoles.List()
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]crds.AgentRole, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, crds.AgentRoleList{Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := crds.ParseAgentRoleManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if existing, ok := s.stores.AgentRoles.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name)); ok {
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
		obj, ok := s.stores.AgentRoles.Get(key)
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
		obj, err := crds.ParseAgentRoleManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok := s.stores.AgentRoles.Get(key)
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
		items := s.stores.ToolPerms.List()
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]crds.ToolPermission, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, crds.ToolPermissionList{Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := crds.ParseToolPermissionManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if existing, ok := s.stores.ToolPerms.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name)); ok {
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
		obj, ok := s.stores.ToolPerms.Get(key)
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
		obj, err := crds.ParseToolPermissionManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok := s.stores.ToolPerms.Get(key)
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

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items := s.stores.Tasks.List()
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]crds.Task, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, crds.TaskList{Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := crds.ParseTaskManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if existing, ok := s.stores.Tasks.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name)); ok {
			obj.Status = existing.Status
		}
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
		obj, ok := s.stores.Tasks.Get(key)
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
		obj, err := crds.ParseTaskManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok := s.stores.Tasks.Get(key)
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
		items := s.stores.TaskSchedules.List()
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]crds.TaskSchedule, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, crds.TaskScheduleList{Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := crds.ParseTaskScheduleManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if existing, ok := s.stores.TaskSchedules.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name)); ok {
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
		obj, ok := s.stores.TaskSchedules.Get(key)
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
		obj, err := crds.ParseTaskScheduleManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok := s.stores.TaskSchedules.Get(key)
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
		items := s.stores.Workers.List()
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]crds.Worker, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, crds.WorkerList{Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := crds.ParseWorkerManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if existing, ok := s.stores.Workers.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name)); ok {
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
		obj, ok := s.stores.Workers.Get(key)
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
		obj, err := crds.ParseWorkerManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok := s.stores.Workers.Get(key)
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

func (s *Server) withRuntimeStatus(agent crds.Agent) crds.Agent {
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
