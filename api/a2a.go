package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/runtime/a2a"
	"github.com/OrlojHQ/orloj/store"
	"github.com/OrlojHQ/orloj/telemetry"
)

// A2AConfig holds server-side A2A configuration.
type A2AConfig struct {
	PublicBaseURL          string
	ProtocolVersion        string
	StreamingEnabled       bool
	AuthSchemes            []string
	Registry               *a2a.Registry
	RateLimitRPM           int
	MaxConcurrentSubscribe int
}

// handleWellKnownAgentCard serves GET /.well-known/agent-card.json and legacy /.well-known/agent.json
func (s *Server) handleWellKnownAgentCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	systems, err := s.a2aEnabledSystems(r)
	if err != nil {
		http.Error(w, "failed to list agent systems", http.StatusInternalServerError)
		return
	}
	if len(systems) != 1 {
		http.Error(w, "A2A root card is available only when exactly one AgentSystem is enabled", http.StatusNotFound)
		return
	}

	tools, _ := s.stores.Tools.List(r.Context())
	agents, _ := s.stores.Agents.List(r.Context())

	config := s.buildCardConfig(systems[0].Metadata.Namespace)
	if systems[0].Spec.A2A.Auth == resources.A2AAuthPublic {
		config.AuthSchemes = nil
	}
	card := a2a.GenerateSystemCard(systems[0], agentsForSystem(systems[0], agents), tools, config)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(card)
}

// handleAgentCard serves GET /v1/agents/{name}/.well-known/agent-card.json
func (s *Server) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.Path
	name := extractA2ASystemNameFromPath(path)
	if name == "" {
		http.Error(w, "agentsystem name required", http.StatusBadRequest)
		return
	}

	system, ok, err := s.a2aEnabledSystemByName(r, name)
	if err != nil {
		http.Error(w, "failed to get agentsystem", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "agentsystem not found", http.StatusNotFound)
		return
	}

	tools, _ := s.stores.Tools.List(r.Context())
	agents, _ := s.stores.Agents.List(r.Context())

	config := s.buildCardConfig(system.Metadata.Namespace)
	if system.Spec.A2A.Auth == resources.A2AAuthPublic {
		config.AuthSchemes = nil
	}
	card := a2a.GenerateSystemCard(system, agentsForSystem(system, agents), tools, config)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(card)
}

// handleA2AJSONRPC handles POST /a2a and POST /v1/agents/{name}/a2a
func (s *Server) handleA2AJSONRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.a2aRateLimiter != nil && !s.a2aRateLimiter.Allow(r) {
		writeA2AError(w, nil, a2a.ErrCodeInternal, "rate limit exceeded")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
	if err != nil {
		writeA2AError(w, nil, a2a.ErrCodeParse, "failed to read request body")
		return
	}

	var req a2a.JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeA2AError(w, nil, a2a.ErrCodeParse, "invalid JSON-RPC request")
		return
	}

	if req.JSONRPC != "2.0" {
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidRequest, "jsonrpc must be \"2.0\"")
		return
	}

	systemName := extractA2ASystemNameFromPath(r.URL.Path)

	switch req.Method {
	case a2a.MethodTaskSend:
		s.handleA2ATaskSend(w, r, req, systemName)
	case a2a.MethodTaskGet:
		s.handleA2ATaskGet(w, r, req, systemName)
	case a2a.MethodTaskCancel:
		s.handleA2ATaskCancel(w, r, req, systemName)
	case a2a.MethodTaskSubscribe:
		s.handleA2ATaskSubscribe(w, r, req, systemName)
	default:
		writeA2AError(w, req.ID, a2a.ErrCodeMethodNotFound, fmt.Sprintf("unknown method: %s", req.Method))
	}
}

func (s *Server) handleA2ATaskSend(w http.ResponseWriter, r *http.Request, req a2a.JSONRPCRequest, systemName string) {
	start := time.Now()
	status := "ok"
	defer func() {
		telemetry.RecordA2AInbound(a2a.MethodTaskSend, status, systemName, time.Since(start).Seconds())
	}()

	paramsBytes, err := json.Marshal(req.Params)
	if err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "invalid params")
		return
	}

	var params a2a.TaskSendParams
	if err := json.Unmarshal(paramsBytes, &params); err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "invalid task send params")
		return
	}

	if params.ID == "" {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "task id is required")
		return
	}

	system := strings.TrimSpace(systemName)
	if system == "" {
		if params.Metadata != nil {
			if agent, ok := params.Metadata["agent"]; ok {
				system = agent
			} else if target, ok := params.Metadata["target"]; ok {
				system = target
			}
		}
	}
	if system == "" {
		defaultSystem, ok, err := s.defaultA2ASystem(r)
		if err != nil {
			status = "error"
			writeA2AError(w, req.ID, a2a.ErrCodeInternal, "failed to resolve target agentsystem")
			return
		}
		if ok {
			system = defaultSystem.Metadata.Name
		}
	}
	if system == "" {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "target AgentSystem must be specified via URL path or request metadata")
		return
	}

	target, ok, err := s.a2aEnabledSystemByName(r, system)
	if err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInternal, "failed to resolve target agentsystem")
		return
	}
	if !ok {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeAgentNotFound, "target AgentSystem is not A2A-enabled")
		return
	}
	if !a2aAuthorizeInvoke(w, r, req.ID, target) {
		status = "error"
		return
	}

	namespace := resources.NormalizeNamespace(target.Metadata.Namespace)
	task := a2a.CreateOrlojTaskFromA2A(params, target.Metadata.Name, namespace)
	task.Metadata.Name = a2aInternalTaskName(target, params.ID)

	if _, err := s.stores.Tasks.Upsert(r.Context(), task); err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInternal, "failed to create task")
		return
	}

	result := a2a.OrlojTaskToA2AResult(task)
	writeA2AResult(w, req.ID, result)
}

func (s *Server) handleA2ATaskGet(w http.ResponseWriter, r *http.Request, req a2a.JSONRPCRequest, systemName string) {
	start := time.Now()
	status := "ok"
	defer func() {
		telemetry.RecordA2AInbound(a2a.MethodTaskGet, status, systemName, time.Since(start).Seconds())
	}()

	paramsBytes, err := json.Marshal(req.Params)
	if err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "invalid params")
		return
	}

	var params a2a.TaskGetParams
	if err := json.Unmarshal(paramsBytes, &params); err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "invalid task get params")
		return
	}

	if params.ID == "" {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "task id is required")
		return
	}

	task, err := s.findTaskByA2AID(r, params.ID, systemName)
	if err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeTaskNotFound, "task not found")
		return
	}
	if !s.a2aIdentityAllowsTask(r, task) {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeTaskNotFound, "task not found")
		return
	}

	result := a2a.OrlojTaskToA2AResult(task)
	writeA2AResult(w, req.ID, result)
}

func (s *Server) handleA2ATaskCancel(w http.ResponseWriter, r *http.Request, req a2a.JSONRPCRequest, systemName string) {
	start := time.Now()
	status := "ok"
	defer func() {
		telemetry.RecordA2AInbound(a2a.MethodTaskCancel, status, systemName, time.Since(start).Seconds())
	}()

	paramsBytes, err := json.Marshal(req.Params)
	if err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "invalid params")
		return
	}

	var params a2a.TaskCancelParams
	if err := json.Unmarshal(paramsBytes, &params); err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "invalid task cancel params")
		return
	}

	if params.ID == "" {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "task id is required")
		return
	}

	task, err := s.findTaskByA2AID(r, params.ID, systemName)
	if err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeTaskNotFound, "task not found")
		return
	}
	if !s.a2aIdentityAllowsTask(r, task) {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeTaskNotFound, "task not found")
		return
	}

	reason := params.Reason
	if reason == "" {
		reason = "cancelled via A2A"
	}
	if len(reason) > 1024 {
		reason = reason[:1024]
		for !utf8.ValidString(reason) {
			reason = reason[:len(reason)-1]
		}
	}

	if task.Metadata.Labels == nil {
		task.Metadata.Labels = make(map[string]string)
	}
	task.Metadata.Labels[a2a.LabelA2ACancelled] = "true"
	task.Status.Phase = "Failed"
	task.Status.LastError = fmt.Sprintf("a2a:cancelled:%s", reason)
	task.Status.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)

	if _, err := s.stores.Tasks.Upsert(r.Context(), task); err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInternal, "failed to cancel task")
		return
	}

	result := a2a.OrlojTaskToA2AResult(task)
	writeA2AResult(w, req.ID, result)
}

func (s *Server) handleA2ATaskSubscribe(w http.ResponseWriter, r *http.Request, req a2a.JSONRPCRequest, systemName string) {
	start := time.Now()
	status := "ok"
	defer func() {
		telemetry.RecordA2AInbound(a2a.MethodTaskSubscribe, status, systemName, time.Since(start).Seconds())
	}()

	paramsBytes, err := json.Marshal(req.Params)
	if err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "invalid params")
		return
	}

	var params a2a.TaskSendParams
	if err := json.Unmarshal(paramsBytes, &params); err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "invalid subscribe params")
		return
	}

	if params.ID == "" {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "task id is required")
		return
	}

	system := strings.TrimSpace(systemName)
	if system == "" && params.Metadata != nil {
		if agent, ok := params.Metadata["agent"]; ok {
			system = agent
		} else if target, ok := params.Metadata["target"]; ok {
			system = target
		}
	}
	if system == "" {
		defaultSystem, ok, err := s.defaultA2ASystem(r)
		if err != nil {
			status = "error"
			writeA2AError(w, req.ID, a2a.ErrCodeInternal, "failed to resolve target agentsystem")
			return
		}
		if ok {
			system = defaultSystem.Metadata.Name
		}
	}
	if system == "" {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "target AgentSystem required")
		return
	}

	target, ok, err := s.a2aEnabledSystemByName(r, system)
	if err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInternal, "failed to resolve target agentsystem")
		return
	}
	if !ok {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeAgentNotFound, "target AgentSystem is not A2A-enabled")
		return
	}
	if !a2aAuthorizeInvoke(w, r, req.ID, target) {
		status = "error"
		return
	}

	if s.a2aConfig != nil && !s.a2aConfig.StreamingEnabled {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInternal, "streaming is not enabled")
		return
	}

	if s.a2aConfig != nil && s.a2aConfig.MaxConcurrentSubscribe > 0 {
		cur := s.a2aSubscribeCount.Add(1)
		if int(cur) > s.a2aConfig.MaxConcurrentSubscribe {
			s.a2aSubscribeCount.Add(-1)
			status = "error"
			writeA2AError(w, req.ID, a2a.ErrCodeInternal, "too many concurrent subscribe connections")
			return
		}
	} else {
		s.a2aSubscribeCount.Add(1)
	}
	defer s.a2aSubscribeCount.Add(-1)

	flusher, ok := w.(http.Flusher)
	if !ok {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInternal, "streaming not supported")
		return
	}

	namespace := resources.NormalizeNamespace(target.Metadata.Namespace)
	task := a2a.CreateOrlojTaskFromA2A(params, target.Metadata.Name, namespace)
	task.Metadata.Name = a2aInternalTaskName(target, params.ID)

	if _, err := s.stores.Tasks.Upsert(r.Context(), task); err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInternal, "failed to create task")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	telemetry.A2AActiveSubscriptions.WithLabelValues(target.Metadata.Name).Inc()
	defer telemetry.A2AActiveSubscriptions.WithLabelValues(target.Metadata.Name).Dec()

	initialResult := a2a.OrlojTaskToA2AResult(task)
	if sendSSEEvent(w, flusher, "status", initialResult) != nil {
		status = "client_disconnected"
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	taskKey := scopedNameForRequest(r, task.Metadata.Name)
	for {
		select {
		case <-r.Context().Done():
			status = "client_disconnected"
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprintf(w, ": heartbeat\n\n"); err != nil {
				status = "client_disconnected"
				return
			}
			flusher.Flush()
		case <-ticker.C:
			current, ok, err := s.stores.Tasks.Get(r.Context(), taskKey)
			if err != nil || !ok {
				return
			}
			result := a2a.OrlojTaskToA2AResult(current)
			if sendSSEEvent(w, flusher, "status", result) != nil {
				status = "client_disconnected"
				return
			}
			if a2a.IsTerminal(result.Status.State) {
				return
			}
		}
	}
}

// handleA2ARegistry serves GET /v1/a2a/agents
func (s *Server) handleA2ARegistry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var localCards []a2a.AgentCard

	systems, err := s.a2aEnabledSystems(r)
	if err != nil {
		http.Error(w, "failed to list agent systems", http.StatusInternalServerError)
		return
	}
	if len(systems) > 0 {
		agents, _ := s.stores.Agents.List(r.Context())
		tools, _ := s.stores.Tools.List(r.Context())

		identity, hasIdentity := AuthIdentityFromRequest(r)
		unauthenticated := !hasIdentity || (strings.EqualFold(identity.Method, "none") && !identity.AuthDisabled)

		for _, system := range systems {
			if unauthenticated && system.Spec.A2A.Auth != resources.A2AAuthPublic {
				continue
			}
			if !unauthenticated && !a2aIdentityAllowsSystem(r, system) && system.Spec.A2A.Auth != resources.A2AAuthPublic {
				continue
			}
			config := s.buildCardConfig(system.Metadata.Namespace)
			if system.Spec.A2A.Auth == resources.A2AAuthPublic {
				config.AuthSchemes = nil
			}
			card := a2a.GenerateSystemCard(system, agentsForSystem(system, agents), tools, config)
			localCards = append(localCards, card)
		}
	}

	var remoteAgents []a2a.RemoteAgentEntry
	if s.a2aConfig != nil && s.a2aConfig.Registry != nil {
		remoteAgents = s.a2aConfig.Registry.List()
	}

	resp := a2a.RegistryResponse{
		LocalAgents:  localCards,
		RemoteAgents: remoteAgents,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) buildCardConfig(namespace string) a2a.CardGeneratorConfig {
	if s.a2aConfig == nil {
		return a2a.CardGeneratorConfig{Namespace: resources.NormalizeNamespace(namespace)}
	}
	return a2a.CardGeneratorConfig{
		PublicBaseURL:    s.a2aConfig.PublicBaseURL,
		ProtocolVersion:  s.a2aConfig.ProtocolVersion,
		StreamingEnabled: s.a2aConfig.StreamingEnabled,
		AuthSchemes:      s.a2aConfig.AuthSchemes,
		Namespace:        resources.NormalizeNamespace(namespace),
	}
}

func (s *Server) a2aEnabledSystems(r *http.Request) ([]resources.AgentSystem, error) {
	systems, err := s.stores.AgentSystems.List(r.Context())
	if err != nil {
		return nil, err
	}
	namespace := requestNamespace(r)
	out := make([]resources.AgentSystem, 0, len(systems))
	for _, system := range systems {
		if !system.Spec.A2A.Enabled {
			continue
		}
		if !strings.EqualFold(resources.NormalizeNamespace(system.Metadata.Namespace), namespace) {
			continue
		}
		out = append(out, system)
	}
	sort.Slice(out, func(i, j int) bool {
		return store.ScopedName(out[i].Metadata.Namespace, out[i].Metadata.Name) < store.ScopedName(out[j].Metadata.Namespace, out[j].Metadata.Name)
	})
	return out, nil
}

func (s *Server) a2aEnabledSystemByName(r *http.Request, name string) (resources.AgentSystem, bool, error) {
	ref := scopedNameForRequest(r, name)
	if strings.Contains(strings.TrimSpace(name), "/") {
		parts := strings.SplitN(strings.TrimSpace(name), "/", 2)
		ref = store.ScopedName(parts[0], parts[1])
	}
	system, ok, err := s.stores.AgentSystems.Get(r.Context(), ref)
	if err != nil || !ok {
		return resources.AgentSystem{}, false, err
	}
	if !system.Spec.A2A.Enabled {
		return resources.AgentSystem{}, false, nil
	}
	return system, true, nil
}

func (s *Server) defaultA2ASystem(r *http.Request) (resources.AgentSystem, bool, error) {
	systems, err := s.a2aEnabledSystems(r)
	if err != nil || len(systems) != 1 {
		return resources.AgentSystem{}, false, err
	}
	return systems[0], true, nil
}

func agentsForSystem(system resources.AgentSystem, agents []resources.Agent) []resources.Agent {
	agentsByName := make(map[string]resources.Agent, len(agents))
	for _, agent := range agents {
		agentsByName[store.ScopedName(agent.Metadata.Namespace, agent.Metadata.Name)] = agent
	}
	out := make([]resources.Agent, 0, len(system.Spec.Agents))
	for _, agentName := range system.Spec.Agents {
		key := store.ScopedName(system.Metadata.Namespace, agentName)
		if agent, ok := agentsByName[key]; ok {
			out = append(out, agent)
			continue
		}
		key = store.ScopedName(resources.DefaultNamespace, agentName)
		if agent, ok := agentsByName[key]; ok {
			out = append(out, agent)
		}
	}
	return out
}

// a2aAuthorizeInvoke is the single centralized auth gate for A2A invoke.
// It checks the system's auth policy and, for bearer-required systems,
// validates the caller's identity and scope. Returns true if the request
// is authorized; writes an appropriate JSON-RPC error and returns false
// otherwise.
func a2aAuthorizeInvoke(w http.ResponseWriter, r *http.Request, reqID any, system resources.AgentSystem) bool {
	if system.Spec.A2A.Auth == resources.A2AAuthPublic {
		return true
	}
	identity, ok := AuthIdentityFromRequest(r)
	if !ok {
		writeA2AError(w, reqID, a2a.ErrCodeAgentNotFound, "target AgentSystem is not available")
		return false
	}
	if identity.AuthDisabled {
		return true
	}
	if strings.EqualFold(identity.Method, "none") {
		writeA2AError(w, reqID, a2a.ErrCodeAgentNotFound, "target AgentSystem is not available")
		return false
	}
	if !strings.EqualFold(identity.Method, "bearer") {
		writeA2AError(w, reqID, a2a.ErrCodeAgentNotFound, "target AgentSystem is not available")
		return false
	}
	if !a2aIdentityAllowsSystem(r, system) {
		writeA2AError(w, reqID, a2a.ErrCodeAgentNotFound, "target AgentSystem is not available")
		return false
	}
	return true
}

func a2aIdentityAllowsSystem(r *http.Request, system resources.AgentSystem) bool {
	identity, ok := AuthIdentityFromRequest(r)
	if !ok {
		return system.Spec.A2A.Auth == resources.A2AAuthPublic
	}
	if strings.EqualFold(identity.Method, "none") {
		return identity.AuthDisabled || system.Spec.A2A.Auth == resources.A2AAuthPublic
	}
	if strings.EqualFold(identity.Role, "admin") || strings.EqualFold(identity.Role, "writer") {
		return true
	}
	if !strings.EqualFold(identity.Role, "a2a") {
		return false
	}
	target := strings.ToLower(store.ScopedName(system.Metadata.Namespace, system.Metadata.Name))
	for _, allowed := range normalizeA2AAgentSystemRefs(identity.A2AAgentSystems) {
		if strings.ToLower(allowed) == target {
			return true
		}
	}
	return false
}

func (s *Server) a2aIdentityAllowsTask(r *http.Request, task resources.Task) bool {
	system, ok, err := s.stores.AgentSystems.Get(r.Context(), store.ScopedName(task.Metadata.Namespace, task.Spec.System))
	if err != nil || !ok || !system.Spec.A2A.Enabled {
		return false
	}
	if system.Spec.A2A.Auth == resources.A2AAuthPublic {
		return true
	}
	return a2aIdentityAllowsSystem(r, system)
}

func a2aInternalTaskName(system resources.AgentSystem, externalID string) string {
	ref := store.ScopedName(system.Metadata.Namespace, system.Metadata.Name)
	sum := sha256.Sum256([]byte(ref + "\x00" + externalID))
	suffix := sanitizeA2ANamePart(externalID)
	if suffix == "" {
		suffix = "task"
	}
	if len(suffix) > 48 {
		suffix = suffix[:48]
	}
	return fmt.Sprintf("a2a-%x-%s", sum[:6], suffix)
}

func sanitizeA2ANamePart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func (s *Server) findTaskByA2AID(r *http.Request, a2aTaskID string, systemFilter ...string) (resources.Task, error) {
	tasks, err := s.stores.Tasks.List(r.Context())
	if err != nil {
		return resources.Task{}, err
	}
	filter := ""
	if len(systemFilter) > 0 {
		filter = strings.TrimSpace(systemFilter[0])
	}
	for _, task := range tasks {
		if task.Metadata.Labels != nil && task.Metadata.Labels[a2a.LabelA2ATaskID] == a2aTaskID {
			if filter != "" && task.Spec.System != filter {
				continue
			}
			if s.a2aIdentityAllowsTask(r, task) {
				return task, nil
			}
		}
	}
	return resources.Task{}, fmt.Errorf("task not found")
}

func extractA2ASystemNameFromPath(path string) string {
	for _, prefix := range []string{"/v1/agent-systems/", "/v1/agents/"} {
		if strings.HasPrefix(path, prefix) {
			path = strings.TrimPrefix(path, prefix)
			parts := strings.SplitN(path, "/", 2)
			if len(parts) > 0 {
				if decoded, err := url.PathUnescape(parts[0]); err == nil {
					return strings.TrimSpace(decoded)
				}
				return strings.TrimSpace(parts[0])
			}
		}
	}
	return ""
}

func resolveNamespace(r *http.Request) string {
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "default"
	}
	return ns
}

func writeA2AError(w http.ResponseWriter, id any, code int, msg string) {
	resp := a2a.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &a2a.JSONRPCError{
			Code:    code,
			Message: msg,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func writeA2AResult(w http.ResponseWriter, id any, result any) {
	resp := a2a.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, eventType string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(jsonData)); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
