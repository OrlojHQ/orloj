package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

type agentStatusPatch struct {
	Metadata resources.ObjectMeta  `json:"metadata"`
	Status   resources.AgentStatus `json:"status"`
}

type agentSystemStatusPatch struct {
	Metadata resources.ObjectMeta        `json:"metadata"`
	Status   resources.AgentSystemStatus `json:"status"`
}

type toolStatusPatch struct {
	Metadata resources.ObjectMeta `json:"metadata"`
	Status   resources.ToolStatus `json:"status"`
}

type modelEndpointStatusPatch struct {
	Metadata resources.ObjectMeta          `json:"metadata"`
	Status   resources.ModelEndpointStatus `json:"status"`
}

type memoryStatusPatch struct {
	Metadata resources.ObjectMeta   `json:"metadata"`
	Status   resources.MemoryStatus `json:"status"`
}

type policyStatusPatch struct {
	Metadata resources.ObjectMeta   `json:"metadata"`
	Status   resources.PolicyStatus `json:"status"`
}

type taskStatusPatch struct {
	Metadata resources.ObjectMeta `json:"metadata"`
	Status   resources.TaskStatus `json:"status"`
}

type taskScheduleStatusPatch struct {
	Metadata resources.ObjectMeta         `json:"metadata"`
	Status   resources.TaskScheduleStatus `json:"status"`
}

type taskWebhookStatusPatch struct {
	Metadata resources.ObjectMeta        `json:"metadata"`
	Status   resources.TaskWebhookStatus `json:"status"`
}

type workerStatusPatch struct {
	Metadata resources.ObjectMeta   `json:"metadata"`
	Status   resources.WorkerStatus `json:"status"`
}

func decodeStatusPatch(r *http.Request, out any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("failed to decode JSON body: %w", err)
	}
	return nil
}

func writeStatus(w http.ResponseWriter, metadata resources.ObjectMeta, status any) {
	writeJSON(w, http.StatusOK, map[string]any{
		"metadata": metadata,
		"status":   status,
	})
}

func writeStoreError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	if store.IsConflict(err) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

// writeStoreFetchError writes a 503 Service Unavailable if err is non-nil and
// returns true, signalling the caller to abort the handler. Returns false when
// err is nil so callers can inline it: if writeStoreFetchError(w, err) { return }.
func writeStoreFetchError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	http.Error(w, "store unavailable: "+err.Error(), http.StatusServiceUnavailable)
	return true
}

func (s *Server) handleAgentStatusByName(w http.ResponseWriter, r *http.Request, name string) {
	obj, ok, err := s.stores.Agents.Get(scopedNameForRequest(r, name))
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("agent %q not found", name), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeStatus(w, obj.Metadata, s.withRuntimeStatus(obj).Status)
	case http.MethodPut:
		var patch agentStatusPatch
		if err := decodeStatusPatch(r, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &patch.Metadata, obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = patch.Status
		if obj.Status.ObservedGeneration == 0 {
			obj.Status.ObservedGeneration = obj.Metadata.Generation
		}
		obj.Metadata.ResourceVersion = patch.Metadata.ResourceVersion
		updated, err := s.stores.Agents.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("Agent", updated.Metadata.Name, "status", map[string]any{"metadata": updated.Metadata, "status": updated.Status})
		writeStatus(w, updated.Metadata, updated.Status)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgentSystemStatusByName(w http.ResponseWriter, r *http.Request, name string) {
	obj, ok, err := s.stores.AgentSystems.Get(scopedNameForRequest(r, name))
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("agentsystem %q not found", name), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeStatus(w, obj.Metadata, obj.Status)
	case http.MethodPut:
		var patch agentSystemStatusPatch
		if err := decodeStatusPatch(r, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &patch.Metadata, obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = patch.Status
		if obj.Status.ObservedGeneration == 0 {
			obj.Status.ObservedGeneration = obj.Metadata.Generation
		}
		obj.Metadata.ResourceVersion = patch.Metadata.ResourceVersion
		updated, err := s.stores.AgentSystems.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("AgentSystem", updated.Metadata.Name, "status", map[string]any{"metadata": updated.Metadata, "status": updated.Status})
		writeStatus(w, updated.Metadata, updated.Status)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleToolStatusByName(w http.ResponseWriter, r *http.Request, name string) {
	obj, ok, err := s.stores.Tools.Get(scopedNameForRequest(r, name))
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("tool %q not found", name), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeStatus(w, obj.Metadata, obj.Status)
	case http.MethodPut:
		var patch toolStatusPatch
		if err := decodeStatusPatch(r, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &patch.Metadata, obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = patch.Status
		if obj.Status.ObservedGeneration == 0 {
			obj.Status.ObservedGeneration = obj.Metadata.Generation
		}
		obj.Metadata.ResourceVersion = patch.Metadata.ResourceVersion
		updated, err := s.stores.Tools.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("Tool", updated.Metadata.Name, "status", map[string]any{"metadata": updated.Metadata, "status": updated.Status})
		writeStatus(w, updated.Metadata, updated.Status)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleModelEndpointStatusByName(w http.ResponseWriter, r *http.Request, name string) {
	obj, ok, err := s.stores.ModelEPs.Get(scopedNameForRequest(r, name))
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("modelendpoint %q not found", name), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeStatus(w, obj.Metadata, obj.Status)
	case http.MethodPut:
		var patch modelEndpointStatusPatch
		if err := decodeStatusPatch(r, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &patch.Metadata, obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = patch.Status
		if obj.Status.ObservedGeneration == 0 {
			obj.Status.ObservedGeneration = obj.Metadata.Generation
		}
		obj.Metadata.ResourceVersion = patch.Metadata.ResourceVersion
		updated, err := s.stores.ModelEPs.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("ModelEndpoint", updated.Metadata.Name, "status", map[string]any{"metadata": updated.Metadata, "status": updated.Status})
		writeStatus(w, updated.Metadata, updated.Status)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMemoryStatusByName(w http.ResponseWriter, r *http.Request, name string) {
	obj, ok, err := s.stores.Memories.Get(scopedNameForRequest(r, name))
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("memory %q not found", name), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeStatus(w, obj.Metadata, obj.Status)
	case http.MethodPut:
		var patch memoryStatusPatch
		if err := decodeStatusPatch(r, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &patch.Metadata, obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = patch.Status
		if obj.Status.ObservedGeneration == 0 {
			obj.Status.ObservedGeneration = obj.Metadata.Generation
		}
		obj.Metadata.ResourceVersion = patch.Metadata.ResourceVersion
		updated, err := s.stores.Memories.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("Memory", updated.Metadata.Name, "status", map[string]any{"metadata": updated.Metadata, "status": updated.Status})
		writeStatus(w, updated.Metadata, updated.Status)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePolicyStatusByName(w http.ResponseWriter, r *http.Request, name string) {
	obj, ok, err := s.stores.Policies.Get(scopedNameForRequest(r, name))
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("agentpolicy %q not found", name), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeStatus(w, obj.Metadata, obj.Status)
	case http.MethodPut:
		var patch policyStatusPatch
		if err := decodeStatusPatch(r, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &patch.Metadata, obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = patch.Status
		if obj.Status.ObservedGeneration == 0 {
			obj.Status.ObservedGeneration = obj.Metadata.Generation
		}
		obj.Metadata.ResourceVersion = patch.Metadata.ResourceVersion
		updated, err := s.stores.Policies.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("AgentPolicy", updated.Metadata.Name, "status", map[string]any{"metadata": updated.Metadata, "status": updated.Status})
		writeStatus(w, updated.Metadata, updated.Status)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTaskStatusByName(w http.ResponseWriter, r *http.Request, name string) {
	obj, ok, err := s.stores.Tasks.Get(scopedNameForRequest(r, name))
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("task %q not found", name), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeStatus(w, obj.Metadata, obj.Status)
	case http.MethodPut:
		var patch taskStatusPatch
		if err := decodeStatusPatch(r, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &patch.Metadata, obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = patch.Status
		if obj.Status.ObservedGeneration == 0 {
			obj.Status.ObservedGeneration = obj.Metadata.Generation
		}
		obj.Metadata.ResourceVersion = patch.Metadata.ResourceVersion
		updated, err := s.stores.Tasks.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("Task", updated.Metadata.Name, "status", map[string]any{"metadata": updated.Metadata, "status": updated.Status})
		writeStatus(w, updated.Metadata, updated.Status)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTaskScheduleStatusByName(w http.ResponseWriter, r *http.Request, name string) {
	obj, ok, err := s.stores.TaskSchedules.Get(scopedNameForRequest(r, name))
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("taskschedule %q not found", name), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeStatus(w, obj.Metadata, obj.Status)
	case http.MethodPut:
		var patch taskScheduleStatusPatch
		if err := decodeStatusPatch(r, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &patch.Metadata, obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = patch.Status
		if obj.Status.ObservedGeneration == 0 {
			obj.Status.ObservedGeneration = obj.Metadata.Generation
		}
		obj.Metadata.ResourceVersion = patch.Metadata.ResourceVersion
		updated, err := s.stores.TaskSchedules.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("TaskSchedule", updated.Metadata.Name, "status", map[string]any{"metadata": updated.Metadata, "status": updated.Status})
		writeStatus(w, updated.Metadata, updated.Status)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTaskWebhookStatusByName(w http.ResponseWriter, r *http.Request, name string) {
	obj, ok, err := s.stores.TaskWebhooks.Get(scopedNameForRequest(r, name))
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("taskwebhook %q not found", name), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeStatus(w, obj.Metadata, obj.Status)
	case http.MethodPut:
		var patch taskWebhookStatusPatch
		if err := decodeStatusPatch(r, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &patch.Metadata, obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = patch.Status
		if obj.Status.ObservedGeneration == 0 {
			obj.Status.ObservedGeneration = obj.Metadata.Generation
		}
		obj.Metadata.ResourceVersion = patch.Metadata.ResourceVersion
		updated, err := s.stores.TaskWebhooks.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("TaskWebhook", updated.Metadata.Name, "status", map[string]any{"metadata": updated.Metadata, "status": updated.Status})
		writeStatus(w, updated.Metadata, updated.Status)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorkerStatusByName(w http.ResponseWriter, r *http.Request, name string) {
	obj, ok, err := s.stores.Workers.Get(scopedNameForRequest(r, name))
	if writeStoreFetchError(w, err) { return }
	if !ok {
		http.Error(w, fmt.Sprintf("worker %q not found", name), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeStatus(w, obj.Metadata, obj.Status)
	case http.MethodPut:
		var patch workerStatusPatch
		if err := decodeStatusPatch(r, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &patch.Metadata, obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = patch.Status
		if obj.Status.ObservedGeneration == 0 {
			obj.Status.ObservedGeneration = obj.Metadata.Generation
		}
		obj.Metadata.ResourceVersion = patch.Metadata.ResourceVersion
		updated, err := s.stores.Workers.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("Worker", updated.Metadata.Name, "status", map[string]any{"metadata": updated.Metadata, "status": updated.Status})
		writeStatus(w, updated.Metadata, updated.Status)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
