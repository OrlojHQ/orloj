package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/runtime/a2a"
	"github.com/OrlojHQ/orloj/telemetry"
)

// A2AConfig holds server-side A2A configuration.
type A2AConfig struct {
	Enabled                bool
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
	if s.a2aConfig == nil || !s.a2aConfig.Enabled {
		http.Error(w, "A2A protocol is not enabled", http.StatusNotFound)
		return
	}

	agents, err := s.stores.Agents.List(r.Context())
	if err != nil {
		http.Error(w, "failed to list agents", http.StatusInternalServerError)
		return
	}
	if len(agents) == 0 {
		http.Error(w, "no agents configured", http.StatusNotFound)
		return
	}

	tools, _ := s.stores.Tools.List(r.Context())

	config := s.buildCardConfig()
	card := a2a.GenerateAgentCard(agents[0], tools, config)

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
	if s.a2aConfig == nil || !s.a2aConfig.Enabled {
		http.Error(w, "A2A protocol is not enabled", http.StatusNotFound)
		return
	}

	path := r.URL.Path
	name := extractAgentNameFromCardPath(path)
	if name == "" {
		http.Error(w, "agent name required", http.StatusBadRequest)
		return
	}

	agents, err := s.stores.Agents.List(r.Context())
	if err != nil {
		http.Error(w, "failed to list agents", http.StatusInternalServerError)
		return
	}

	var agent *resources.Agent
	for i := range agents {
		if agents[i].Metadata.Name == name {
			agent = &agents[i]
			break
		}
	}

	if agent == nil {
		systems, err := s.stores.AgentSystems.List(r.Context())
		if err == nil {
			for _, sys := range systems {
				if sys.Metadata.Name == name {
					tools, _ := s.stores.Tools.List(r.Context())
					var sysAgents []resources.Agent
					for _, agentName := range sys.Spec.Agents {
						for i := range agents {
							if agents[i].Metadata.Name == agentName {
								sysAgents = append(sysAgents, agents[i])
							}
						}
					}
					config := s.buildCardConfig()
					card := a2a.GenerateSystemCard(sys, sysAgents, tools, config)
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Cache-Control", "public, max-age=300")
					json.NewEncoder(w).Encode(card)
					return
				}
			}
		}
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	tools, _ := s.stores.Tools.List(r.Context())
	var agentTools []resources.Tool
	for _, t := range tools {
		for _, tn := range agent.Spec.Tools {
			if t.Metadata.Name == tn {
				agentTools = append(agentTools, t)
			}
		}
	}

	config := s.buildCardConfig()
	card := a2a.GenerateAgentCard(*agent, agentTools, config)

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
	if s.a2aConfig == nil || !s.a2aConfig.Enabled {
		writeA2AError(w, nil, a2a.ErrCodeInternal, "A2A protocol is not enabled")
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

	agentName := extractAgentNameFromA2APath(r.URL.Path)

	switch req.Method {
	case a2a.MethodTaskSend:
		s.handleA2ATaskSend(w, r, req, agentName)
	case a2a.MethodTaskGet:
		s.handleA2ATaskGet(w, r, req)
	case a2a.MethodTaskCancel:
		s.handleA2ATaskCancel(w, r, req)
	case a2a.MethodTaskSubscribe:
		s.handleA2ATaskSubscribe(w, r, req, agentName)
	default:
		writeA2AError(w, req.ID, a2a.ErrCodeMethodNotFound, fmt.Sprintf("unknown method: %s", req.Method))
	}
}

func (s *Server) handleA2ATaskSend(w http.ResponseWriter, r *http.Request, req a2a.JSONRPCRequest, agentName string) {
	start := time.Now()
	status := "ok"
	defer func() {
		telemetry.RecordA2AInbound(a2a.MethodTaskSend, status, agentName, time.Since(start).Seconds())
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

	system := agentName
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
		agents, err := s.stores.Agents.List(r.Context())
		if err == nil && len(agents) == 1 {
			system = agents[0].Metadata.Name
		}
	}
	if system == "" {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "target agent must be specified via URL path or request metadata")
		return
	}

	namespace := resolveNamespace(r)
	task := a2a.CreateOrlojTaskFromA2A(params, system, namespace)

	if _, err := s.stores.Tasks.Upsert(r.Context(), task); err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInternal, "failed to create task")
		return
	}

	result := a2a.OrlojTaskToA2AResult(task)
	writeA2AResult(w, req.ID, result)
}

func (s *Server) handleA2ATaskGet(w http.ResponseWriter, r *http.Request, req a2a.JSONRPCRequest) {
	start := time.Now()
	status := "ok"
	defer func() {
		telemetry.RecordA2AInbound(a2a.MethodTaskGet, status, "", time.Since(start).Seconds())
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

	task, err := s.findTaskByA2AID(r, params.ID)
	if err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeTaskNotFound, "task not found")
		return
	}

	result := a2a.OrlojTaskToA2AResult(task)
	writeA2AResult(w, req.ID, result)
}

func (s *Server) handleA2ATaskCancel(w http.ResponseWriter, r *http.Request, req a2a.JSONRPCRequest) {
	start := time.Now()
	status := "ok"
	defer func() {
		telemetry.RecordA2AInbound(a2a.MethodTaskCancel, status, "", time.Since(start).Seconds())
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

	task, err := s.findTaskByA2AID(r, params.ID)
	if err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeTaskNotFound, "task not found")
		return
	}

	reason := params.Reason
	if reason == "" {
		reason = "cancelled via A2A"
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

func (s *Server) handleA2ATaskSubscribe(w http.ResponseWriter, r *http.Request, req a2a.JSONRPCRequest, agentName string) {
	start := time.Now()
	status := "ok"
	defer func() {
		telemetry.RecordA2AInbound(a2a.MethodTaskSubscribe, status, agentName, time.Since(start).Seconds())
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

	system := agentName
	if system == "" && params.Metadata != nil {
		if agent, ok := params.Metadata["agent"]; ok {
			system = agent
		} else if target, ok := params.Metadata["target"]; ok {
			system = target
		}
	}
	if system == "" {
		agents, err := s.stores.Agents.List(r.Context())
		if err == nil && len(agents) == 1 {
			system = agents[0].Metadata.Name
		}
	}
	if system == "" {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInvalidParams, "target agent required")
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

	namespace := resolveNamespace(r)
	task := a2a.CreateOrlojTaskFromA2A(params, system, namespace)

	if _, err := s.stores.Tasks.Upsert(r.Context(), task); err != nil {
		status = "error"
		writeA2AError(w, req.ID, a2a.ErrCodeInternal, "failed to create task")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	telemetry.A2AActiveSubscriptions.WithLabelValues(agentName).Inc()
	defer telemetry.A2AActiveSubscriptions.WithLabelValues(agentName).Dec()

	initialResult := a2a.OrlojTaskToA2AResult(task)
	sendSSEEvent(w, flusher, "status", initialResult)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	taskKey := scopedNameForRequest(r, task.Metadata.Name)
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case <-ticker.C:
			current, ok, err := s.stores.Tasks.Get(r.Context(), taskKey)
			if err != nil || !ok {
				return
			}
			result := a2a.OrlojTaskToA2AResult(current)
			sendSSEEvent(w, flusher, "status", result)
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
	if s.a2aConfig == nil || !s.a2aConfig.Enabled {
		http.Error(w, "A2A protocol is not enabled", http.StatusNotFound)
		return
	}

	var localCards []a2a.AgentCard

	agents, err := s.stores.Agents.List(r.Context())
	if err == nil {
		tools, _ := s.stores.Tools.List(r.Context())
		config := s.buildCardConfig()
		for _, agent := range agents {
			var agentTools []resources.Tool
			for _, t := range tools {
				for _, tn := range agent.Spec.Tools {
					if t.Metadata.Name == tn {
						agentTools = append(agentTools, t)
					}
				}
			}
			card := a2a.GenerateAgentCard(agent, agentTools, config)
			localCards = append(localCards, card)
		}
	}

	var remoteAgents []a2a.RemoteAgentEntry
	if s.a2aConfig.Registry != nil {
		remoteAgents = s.a2aConfig.Registry.List()
	}

	resp := a2a.RegistryResponse{
		LocalAgents:  localCards,
		RemoteAgents: remoteAgents,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) buildCardConfig() a2a.CardGeneratorConfig {
	if s.a2aConfig == nil {
		return a2a.CardGeneratorConfig{}
	}
	return a2a.CardGeneratorConfig{
		PublicBaseURL:    s.a2aConfig.PublicBaseURL,
		ProtocolVersion: s.a2aConfig.ProtocolVersion,
		StreamingEnabled: s.a2aConfig.StreamingEnabled,
		AuthSchemes:     s.a2aConfig.AuthSchemes,
	}
}

func (s *Server) findTaskByA2AID(r *http.Request, a2aTaskID string) (resources.Task, error) {
	tasks, err := s.stores.Tasks.List(r.Context())
	if err != nil {
		return resources.Task{}, err
	}
	for _, task := range tasks {
		if task.Metadata.Labels != nil && task.Metadata.Labels[a2a.LabelA2ATaskID] == a2aTaskID {
			return task, nil
		}
	}
	return resources.Task{}, fmt.Errorf("task not found")
}

func extractAgentNameFromCardPath(path string) string {
	path = strings.TrimPrefix(path, "/v1/agents/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	return ""
}

func extractAgentNameFromA2APath(path string) string {
	if !strings.HasPrefix(path, "/v1/agents/") {
		return ""
	}
	path = strings.TrimPrefix(path, "/v1/agents/")
	if idx := strings.Index(path, "/a2a"); idx >= 0 {
		return strings.TrimSpace(path[:idx])
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

func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, eventType string, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(jsonData))
	flusher.Flush()
}
