package a2a

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
)

func TestToolRuntime_CallSuccess_Immediate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: TaskResult{
				ID:     "task-1",
				Status: TaskStatus{State: TaskStateCompleted},
				Artifacts: []TaskArtifact{{
					Name:  "output",
					Parts: []TaskPart{{Type: "text", Text: "done"}},
				}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"remote-tool": {
			Type: "a2a",
			A2A:  resources.ToolA2ASpec{AgentURL: srv.URL},
		},
	})

	client := newTestClient()
	rt := NewToolRuntime(client, registry, nil)

	result, err := rt.Call(context.Background(), "remote-tool", "summarize this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "done" {
		t.Errorf("expected 'done', got %q", result)
	}
}

func TestToolRuntime_CallSuccess_Polls(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		state := TaskStateWorking
		var artifacts []TaskArtifact
		if callCount >= 3 {
			state = TaskStateCompleted
			artifacts = []TaskArtifact{{
				Name:  "output",
				Parts: []TaskPart{{Type: "text", Text: "polled result"}},
			}}
		}

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: TaskResult{
				ID:        "task-poll",
				Status:    TaskStatus{State: state},
				Artifacts: artifacts,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"poll-tool": {
			Type: "a2a",
			A2A:  resources.ToolA2ASpec{AgentURL: srv.URL},
		},
	})

	client := newTestClient()
	rt := NewToolRuntime(client, registry, nil)

	result, err := rt.Call(context.Background(), "poll-tool", "work on this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "polled result" {
		t.Errorf("expected 'polled result', got %q", result)
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 calls (send + 2 polls), got %d", callCount)
	}
}

func TestToolRuntime_CallRemoteFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: TaskResult{
				ID: "fail-task",
				Status: TaskStatus{
					State: TaskStateFailed,
					Message: &TaskMessage{
						Role:  "agent",
						Parts: []TaskPart{{Type: "text", Text: "model crashed"}},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"fail-tool": {
			Type: "a2a",
			A2A:  resources.ToolA2ASpec{AgentURL: srv.URL},
		},
	})

	client := newTestClient()
	rt := NewToolRuntime(client, registry, nil)

	_, err := rt.Call(context.Background(), "fail-tool", "try this")
	if err == nil {
		t.Fatal("expected error for failed remote task")
	}
}

func TestToolRuntime_CallRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: TaskResult{
				ID:     "rej-task",
				Status: TaskStatus{State: TaskStateRejected},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"rej-tool": {
			Type: "a2a",
			A2A:  resources.ToolA2ASpec{AgentURL: srv.URL},
		},
	})

	client := newTestClient()
	rt := NewToolRuntime(client, registry, nil)

	_, err := rt.Call(context.Background(), "rej-tool", "forbidden")
	if err == nil {
		t.Fatal("expected error for rejected remote task")
	}
}

func TestToolRuntime_MissingTool(t *testing.T) {
	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{})
	client := newTestClient()
	rt := NewToolRuntime(client, registry, nil)

	_, err := rt.Call(context.Background(), "nonexistent", "input")
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
}

func TestToolRuntime_MissingAgentURL(t *testing.T) {
	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"bad-tool": {Type: "a2a", A2A: resources.ToolA2ASpec{}},
	})
	client := newTestClient()
	rt := NewToolRuntime(client, registry, nil)

	_, err := rt.Call(context.Background(), "bad-tool", "input")
	if err == nil {
		t.Fatal("expected error for missing agent_url")
	}
}

func TestToolRuntime_NilRegistry(t *testing.T) {
	client := newTestClient()
	rt := NewToolRuntime(client, nil, nil)

	_, err := rt.Call(context.Background(), "any-tool", "input")
	if err == nil {
		t.Fatal("expected error for nil registry")
	}
}

func TestToolRuntime_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay long enough for the context to expire.
		time.Sleep(200 * time.Millisecond)
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: TaskResult{
				ID:     "ctx-task",
				Status: TaskStatus{State: TaskStateWorking},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"slow-tool": {
			Type: "a2a",
			A2A:  resources.ToolA2ASpec{AgentURL: srv.URL},
		},
	})

	client := newTestClient()
	rt := NewToolRuntime(client, registry, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := rt.Call(ctx, "slow-tool", "will timeout")
	if err == nil {
		t.Fatal("expected context deadline error")
	}
}

func TestToolRuntime_WithRegistry(t *testing.T) {
	client := newTestClient()
	rt := NewToolRuntime(client, nil, nil)

	newRegistry := agentruntime.NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"tool-a": {Type: "a2a"},
	})
	scoped := rt.WithRegistry(newRegistry)
	if scoped == nil {
		t.Fatal("expected non-nil scoped runtime")
	}
}

func TestToolRuntime_WithNamespace(t *testing.T) {
	client := newTestClient()
	rt := NewToolRuntime(client, nil, nil)

	scoped := rt.WithNamespace("production")
	if scoped == nil {
		t.Fatal("expected non-nil scoped runtime")
	}
	a2aScoped, ok := scoped.(*ToolRuntime)
	if !ok {
		t.Fatal("expected *ToolRuntime type")
	}
	if a2aScoped.namespace != "production" {
		t.Errorf("expected namespace 'production', got %q", a2aScoped.namespace)
	}
}

func TestToolRuntime_WithRegistryNil(t *testing.T) {
	var rt *ToolRuntime
	scoped := rt.WithRegistry(nil)
	if scoped != nil {
		t.Error("expected nil for nil receiver")
	}
}

func TestToolRuntime_WithNamespaceNil(t *testing.T) {
	var rt *ToolRuntime
	scoped := rt.WithNamespace("ns")
	if scoped != nil {
		t.Error("expected nil for nil receiver")
	}
}

func TestFormatResult_Artifacts(t *testing.T) {
	result := TaskResult{
		Artifacts: []TaskArtifact{
			{Parts: []TaskPart{{Type: "text", Text: "Part A"}}},
			{Parts: []TaskPart{{Type: "text", Text: "Part B"}}},
		},
	}
	got := formatResult(result)
	if got != "Part A\nPart B" {
		t.Errorf("expected 'Part A\\nPart B', got %q", got)
	}
}

func TestFormatResult_StatusMessage(t *testing.T) {
	result := TaskResult{
		Status: TaskStatus{
			State: TaskStateCompleted,
			Message: &TaskMessage{
				Parts: []TaskPart{{Type: "text", Text: "status msg"}},
			},
		},
	}
	got := formatResult(result)
	if got != "status msg" {
		t.Errorf("expected 'status msg', got %q", got)
	}
}

func TestFormatResult_JSONFallback(t *testing.T) {
	result := TaskResult{
		ID:     "empty",
		Status: TaskStatus{State: TaskStateCompleted},
	}
	got := formatResult(result)
	if got == "" {
		t.Error("expected non-empty JSON fallback")
	}
}

func TestToolRuntime_PollToFailed(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		state := TaskStateWorking
		if req.Method == MethodTaskGet {
			state = TaskStateFailed
		}

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: TaskResult{
				ID: "task-poll-fail",
				Status: TaskStatus{
					State: state,
					Message: &TaskMessage{
						Role:  "agent",
						Parts: []TaskPart{{Type: "text", Text: "processing error"}},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"poll-fail-tool": {
			Type: "a2a",
			A2A:  resources.ToolA2ASpec{AgentURL: srv.URL},
		},
	})

	client := newTestClient()
	rt := NewToolRuntime(client, registry, nil)

	_, err := rt.Call(context.Background(), "poll-fail-tool", "do work")
	if err == nil {
		t.Fatal("expected error for task that transitions to failed during polling")
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 calls (send + poll), got %d", callCount)
	}
}

func TestToolRuntime_PollToCanceled(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		state := TaskStateWorking
		if req.Method == MethodTaskGet {
			state = TaskStateCanceled
		}

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: TaskResult{
				ID:     "task-poll-cancel",
				Status: TaskStatus{State: state},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"poll-cancel-tool": {
			Type: "a2a",
			A2A:  resources.ToolA2ASpec{AgentURL: srv.URL},
		},
	})

	client := newTestClient()
	rt := NewToolRuntime(client, registry, nil)

	_, err := rt.Call(context.Background(), "poll-cancel-tool", "do work")
	if err == nil {
		t.Fatal("expected error for task that transitions to canceled during polling")
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 calls (send + poll), got %d", callCount)
	}
}

func TestToolRuntime_PollToRejected(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		state := TaskStateWorking
		if req.Method == MethodTaskGet {
			state = TaskStateRejected
		}

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: TaskResult{
				ID:     "task-poll-reject",
				Status: TaskStatus{State: state},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"poll-reject-tool": {
			Type: "a2a",
			A2A:  resources.ToolA2ASpec{AgentURL: srv.URL},
		},
	})

	client := newTestClient()
	rt := NewToolRuntime(client, registry, nil)

	_, err := rt.Call(context.Background(), "poll-reject-tool", "do work")
	if err == nil {
		t.Fatal("expected error for task that transitions to rejected during polling")
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 calls (send + poll), got %d", callCount)
	}
}

func TestToolRuntime_PollContextTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: TaskResult{
				ID:     "task-poll-timeout",
				Status: TaskStatus{State: TaskStateWorking},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"timeout-tool": {
			Type: "a2a",
			A2A:  resources.ToolA2ASpec{AgentURL: srv.URL},
		},
	})

	client := newTestClient()
	rt := NewToolRuntime(client, registry, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := rt.Call(ctx, "timeout-tool", "will timeout during polling")
	if err == nil {
		t.Fatal("expected context deadline error during polling")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("expected context deadline exceeded error, got: %v", err)
	}
}
