package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OrlojHQ/orloj/api"
	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/runtime/a2a"
	"github.com/OrlojHQ/orloj/store"
)

func newA2ATestServer(t *testing.T, enabled bool) (*httptest.Server, api.Stores) {
	t.Helper()
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")
	logger := log.New(io.Discard, "", 0)
	stores := api.Stores{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		Tools:        store.NewToolStore(),
		Tasks:        store.NewTaskStore(),
		Workers:      store.NewWorkerStore(),
		Memories:     store.NewMemoryStore(),
		Policies:     store.NewAgentPolicyStore(),
	}
	server := api.NewServerWithOptions(stores, agentruntime.NewManager(logger), logger, api.ServerOptions{})
	if enabled {
		server.SetA2AConfig(&api.A2AConfig{
			Enabled:          true,
			PublicBaseURL:     "https://test.example.com",
			ProtocolVersion:  "1.0",
			StreamingEnabled: true,
			AuthSchemes:      []string{"bearer"},
		})
	}
	return httptest.NewServer(server.Handler()), stores
}

func seedAgent(t *testing.T, stores api.Stores, name, prompt string, tools []string) {
	t.Helper()
	agent := resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata: resources.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Annotations: map[string]string{
				"orloj.dev/description": "Test agent " + name,
			},
		},
		Spec: resources.AgentSpec{
			ModelRef: "test-model",
			Prompt:   prompt,
			Tools:    tools,
		},
	}
	if _, err := stores.Agents.Upsert(context.Background(), agent); err != nil {
		t.Fatalf("failed to seed agent %s: %v", name, err)
	}
}

func seedTool(t *testing.T, stores api.Stores, name, description string) {
	t.Helper()
	tool := resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata: resources.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: resources.ToolSpec{
			Type:        "http",
			Description: description,
		},
	}
	if _, err := stores.Tools.Upsert(context.Background(), tool); err != nil {
		t.Fatalf("failed to seed tool %s: %v", name, err)
	}
}

func postA2AJSONRPC(t *testing.T, url, method string, params any) *http.Response {
	t.Helper()
	rpcReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}
	body, err := json.Marshal(rpcReq)
	if err != nil {
		t.Fatalf("marshal JSON-RPC request: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	return resp
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func decodeJSONRPCResponse(t *testing.T, resp *http.Response) jsonrpcResponse {
	t.Helper()
	defer resp.Body.Close()
	var rpcResp jsonrpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		t.Fatalf("failed to decode JSON-RPC response: %v", err)
	}
	return rpcResp
}

// --- Well-known card routes ---

func TestWellKnownAgentCard_DisabledReturns404(t *testing.T) {
	ts, _ := newA2ATestServer(t, false)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/.well-known/agent-card.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestWellKnownAgentCard_EnabledReturnsCard(t *testing.T) {
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()

	seedTool(t, stores, "search", "Search the web")
	seedAgent(t, stores, "assistant", "You are helpful", []string{"search"})

	resp, err := http.Get(ts.URL + "/.well-known/agent-card.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var card a2a.AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode card: %v", err)
	}
	if card.Name != "assistant" {
		t.Errorf("expected card name 'assistant', got %q", card.Name)
	}
	if card.ProtocolVersion != "1.0" {
		t.Errorf("expected protocol version '1.0', got %q", card.ProtocolVersion)
	}
	if !card.Capabilities.Streaming {
		t.Error("expected streaming capability to be true")
	}
	if card.Authentication == nil || len(card.Authentication.Schemes) == 0 {
		t.Error("expected authentication schemes")
	}
	if len(card.Skills) != 1 || card.Skills[0].ID != "search" {
		t.Errorf("expected 1 skill 'search', got %+v", card.Skills)
	}
	if card.URL != "https://test.example.com/v1/agents/assistant/a2a" {
		t.Errorf("unexpected card URL: %s", card.URL)
	}
}

func TestWellKnownAgentCard_LegacyPathWorks(t *testing.T) {
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()

	seedAgent(t, stores, "bot", "A bot", nil)

	resp, err := http.Get(ts.URL + "/.well-known/agent.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 for legacy path, got %d: %s", resp.StatusCode, body)
	}

	var card a2a.AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode card: %v", err)
	}
	if card.Name != "bot" {
		t.Errorf("expected card name 'bot', got %q", card.Name)
	}
}

func TestPerAgentCard_ReturnsCardForSpecificAgent(t *testing.T) {
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()

	seedTool(t, stores, "calculator", "Does math")
	seedAgent(t, stores, "math-agent", "I do math", []string{"calculator"})
	seedAgent(t, stores, "writer-agent", "I write", nil)

	resp, err := http.Get(ts.URL + "/v1/agents/math-agent/.well-known/agent-card.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var card a2a.AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode card: %v", err)
	}
	if card.Name != "math-agent" {
		t.Errorf("expected card name 'math-agent', got %q", card.Name)
	}
	if len(card.Skills) != 1 || card.Skills[0].ID != "calculator" {
		t.Errorf("expected 1 skill 'calculator', got %+v", card.Skills)
	}
}

func TestPerAgentCard_NonexistentAgentReturns404(t *testing.T) {
	ts, _ := newA2ATestServer(t, true)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/agents/does-not-exist/.well-known/agent-card.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent agent, got %d", resp.StatusCode)
	}
}

// --- JSON-RPC endpoint ---

func TestA2AJSONRPC_DisabledReturnsError(t *testing.T) {
	ts, _ := newA2ATestServer(t, false)
	defer ts.Close()

	resp := postA2AJSONRPC(t, ts.URL+"/a2a", "tasks/send", map[string]any{"id": "t1"})
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected error when A2A disabled")
	}
	if rpcResp.Error.Code != a2a.ErrCodeInternal {
		t.Errorf("expected error code %d, got %d", a2a.ErrCodeInternal, rpcResp.Error.Code)
	}
}

func TestA2AJSONRPC_InvalidJSONReturnsParseError(t *testing.T) {
	ts, _ := newA2ATestServer(t, true)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/a2a", "application/json", bytes.NewReader([]byte("{invalid")))
	if err != nil {
		t.Fatal(err)
	}
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected parse error")
	}
	if rpcResp.Error.Code != a2a.ErrCodeParse {
		t.Errorf("expected error code %d, got %d", a2a.ErrCodeParse, rpcResp.Error.Code)
	}
}

func TestA2AJSONRPC_MissingJsonrpcFieldReturnsInvalidRequest(t *testing.T) {
	ts, _ := newA2ATestServer(t, true)
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"id":     1,
		"method": "tasks/send",
	})
	resp, err := http.Post(ts.URL+"/a2a", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected invalid request error")
	}
	if rpcResp.Error.Code != a2a.ErrCodeInvalidRequest {
		t.Errorf("expected error code %d, got %d", a2a.ErrCodeInvalidRequest, rpcResp.Error.Code)
	}
}

func TestA2AJSONRPC_UnknownMethodReturnsMethodNotFound(t *testing.T) {
	ts, _ := newA2ATestServer(t, true)
	defer ts.Close()

	resp := postA2AJSONRPC(t, ts.URL+"/a2a", "nonexistent/method", nil)
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected method not found error")
	}
	if rpcResp.Error.Code != a2a.ErrCodeMethodNotFound {
		t.Errorf("expected error code %d, got %d", a2a.ErrCodeMethodNotFound, rpcResp.Error.Code)
	}
}

func TestA2AJSONRPC_TaskSendCreatesTask(t *testing.T) {
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()

	seedAgent(t, stores, "my-agent", "helpful agent", nil)

	params := map[string]any{
		"id": "task-001",
		"message": map[string]any{
			"role": "user",
			"parts": []map[string]any{
				{"type": "text", "text": "Hello, do something useful"},
			},
		},
		"metadata": map[string]string{
			"agent": "my-agent",
		},
	}

	resp := postA2AJSONRPC(t, ts.URL+"/a2a", "tasks/send", params)
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("unexpected error: %+v", rpcResp.Error)
	}

	var result a2a.TaskResult
	if err := json.Unmarshal(rpcResp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ID != "task-001" {
		t.Errorf("expected task ID 'task-001', got %q", result.ID)
	}
	if result.Status.State != a2a.TaskStateSubmitted {
		t.Errorf("expected state %q, got %q", a2a.TaskStateSubmitted, result.Status.State)
	}

	tasks, err := stores.Tasks.List(context.Background())
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task in store, got %d", len(tasks))
	}
	if tasks[0].Spec.System != "my-agent" {
		t.Errorf("expected task system 'my-agent', got %q", tasks[0].Spec.System)
	}
	if tasks[0].Spec.Input["prompt"] != "Hello, do something useful" {
		t.Errorf("unexpected task input: %v", tasks[0].Spec.Input)
	}
}

func TestA2AJSONRPC_TaskSendWithoutTargetReturnsError(t *testing.T) {
	ts, _ := newA2ATestServer(t, true)
	defer ts.Close()

	params := map[string]any{
		"id": "task-no-target",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hi"}},
		},
	}

	resp := postA2AJSONRPC(t, ts.URL+"/a2a", "tasks/send", params)
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected error for missing target agent")
	}
	if rpcResp.Error.Code != a2a.ErrCodeInvalidParams {
		t.Errorf("expected error code %d, got %d", a2a.ErrCodeInvalidParams, rpcResp.Error.Code)
	}
}

func TestA2AJSONRPC_PerAgentPathRoutesToAgent(t *testing.T) {
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()

	seedAgent(t, stores, "routed-agent", "I route", nil)

	params := map[string]any{
		"id": "task-routed",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Route me"}},
		},
	}

	resp := postA2AJSONRPC(t, ts.URL+"/v1/agents/routed-agent/a2a", "tasks/send", params)
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("unexpected error: %+v", rpcResp.Error)
	}

	var result a2a.TaskResult
	if err := json.Unmarshal(rpcResp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ID != "task-routed" {
		t.Errorf("expected task ID 'task-routed', got %q", result.ID)
	}

	tasks, err := stores.Tasks.List(context.Background())
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Spec.System != "routed-agent" {
		t.Errorf("expected system 'routed-agent', got %q", tasks[0].Spec.System)
	}
}

func TestA2AJSONRPC_TaskGetReturnsTask(t *testing.T) {
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()

	seedAgent(t, stores, "agent-x", "I exist", nil)

	sendParams := map[string]any{
		"id": "task-get-test",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Please do X"}},
		},
		"metadata": map[string]string{"agent": "agent-x"},
	}
	sendResp := postA2AJSONRPC(t, ts.URL+"/a2a", "tasks/send", sendParams)
	sendRPC := decodeJSONRPCResponse(t, sendResp)
	if sendRPC.Error != nil {
		t.Fatalf("send failed: %+v", sendRPC.Error)
	}

	getParams := map[string]any{
		"id": "task-get-test",
	}
	getResp := postA2AJSONRPC(t, ts.URL+"/a2a", "tasks/get", getParams)
	getRPC := decodeJSONRPCResponse(t, getResp)
	if getRPC.Error != nil {
		t.Fatalf("get failed: %+v", getRPC.Error)
	}

	var result a2a.TaskResult
	if err := json.Unmarshal(getRPC.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ID != "task-get-test" {
		t.Errorf("expected task ID 'task-get-test', got %q", result.ID)
	}
	if result.Status.State != a2a.TaskStateSubmitted {
		t.Errorf("expected state %q, got %q", a2a.TaskStateSubmitted, result.Status.State)
	}
}

func TestA2AJSONRPC_TaskGetMissingReturnsNotFound(t *testing.T) {
	ts, _ := newA2ATestServer(t, true)
	defer ts.Close()

	params := map[string]any{
		"id": "nonexistent-task",
	}
	resp := postA2AJSONRPC(t, ts.URL+"/a2a", "tasks/get", params)
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected task not found error")
	}
	if rpcResp.Error.Code != a2a.ErrCodeTaskNotFound {
		t.Errorf("expected error code %d, got %d", a2a.ErrCodeTaskNotFound, rpcResp.Error.Code)
	}
}

func TestA2AJSONRPC_TaskCancelSetsFailedWithLabel(t *testing.T) {
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()

	seedAgent(t, stores, "cancel-agent", "agent to cancel", nil)

	sendParams := map[string]any{
		"id": "task-to-cancel",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Do work"}},
		},
		"metadata": map[string]string{"agent": "cancel-agent"},
	}
	sendResp := postA2AJSONRPC(t, ts.URL+"/a2a", "tasks/send", sendParams)
	sendRPC := decodeJSONRPCResponse(t, sendResp)
	if sendRPC.Error != nil {
		t.Fatalf("send failed: %+v", sendRPC.Error)
	}

	cancelParams := map[string]any{
		"id":     "task-to-cancel",
		"reason": "no longer needed",
	}
	cancelResp := postA2AJSONRPC(t, ts.URL+"/a2a", "tasks/cancel", cancelParams)
	cancelRPC := decodeJSONRPCResponse(t, cancelResp)
	if cancelRPC.Error != nil {
		t.Fatalf("cancel failed: %+v", cancelRPC.Error)
	}

	var result a2a.TaskResult
	if err := json.Unmarshal(cancelRPC.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Status.State != a2a.TaskStateCanceled {
		t.Errorf("expected state %q, got %q", a2a.TaskStateCanceled, result.Status.State)
	}

	tasks, err := stores.Tasks.List(context.Background())
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	task := tasks[0]
	if task.Status.Phase != "Failed" {
		t.Errorf("expected phase 'Failed', got %q", task.Status.Phase)
	}
	if task.Metadata.Labels[a2a.LabelA2ACancelled] != "true" {
		t.Errorf("expected cancelled label, got labels: %v", task.Metadata.Labels)
	}
	if task.Status.CompletedAt == "" {
		t.Error("expected CompletedAt to be set after cancellation")
	}
}

func TestA2AJSONRPC_GETMethodNotAllowed(t *testing.T) {
	ts, _ := newA2ATestServer(t, true)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/a2a")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

// --- Registry endpoint ---

func TestA2ARegistry_DisabledReturns404(t *testing.T) {
	ts, _ := newA2ATestServer(t, false)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/a2a/agents")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestA2ARegistry_EnabledReturnsLocalCardsAndEmptyRemote(t *testing.T) {
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()

	seedTool(t, stores, "web-search", "Searches the web")
	seedAgent(t, stores, "searcher", "I search things", []string{"web-search"})
	seedAgent(t, stores, "writer", "I write things", nil)

	resp, err := http.Get(ts.URL + "/v1/a2a/agents")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var registry a2a.RegistryResponse
	if err := json.NewDecoder(resp.Body).Decode(&registry); err != nil {
		t.Fatalf("decode registry: %v", err)
	}
	if len(registry.LocalAgents) != 2 {
		t.Fatalf("expected 2 local agents, got %d", len(registry.LocalAgents))
	}
	if registry.RemoteAgents == nil {
		t.Log("remote agents is nil (acceptable: means no registry configured)")
	} else if len(registry.RemoteAgents) != 0 {
		t.Errorf("expected 0 remote agents, got %d", len(registry.RemoteAgents))
	}

	foundSearcher := false
	for _, card := range registry.LocalAgents {
		if card.Name == "searcher" {
			foundSearcher = true
			if len(card.Skills) != 1 || card.Skills[0].ID != "web-search" {
				t.Errorf("searcher card should have web-search skill, got %+v", card.Skills)
			}
		}
	}
	if !foundSearcher {
		t.Error("expected to find 'searcher' in local agents")
	}
}

// --- Capabilities ---

func TestCapabilities_A2AEnabledIncludesA2AEntries(t *testing.T) {
	ts, _ := newA2ATestServer(t, true)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var payload agentruntime.CapabilitySnapshot
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}

	foundA2A := false
	foundStreaming := false
	for _, cap := range payload.Capabilities {
		switch cap.ID {
		case "a2a":
			foundA2A = true
			if !cap.Enabled {
				t.Error("expected a2a capability to be enabled")
			}
		case "a2a.streaming":
			foundStreaming = true
			if !cap.Enabled {
				t.Error("expected a2a.streaming capability to be enabled")
			}
		}
	}
	if !foundA2A {
		t.Error("expected 'a2a' capability in response")
	}
	if !foundStreaming {
		t.Error("expected 'a2a.streaming' capability in response")
	}
}

func TestA2AJSONRPC_TaskSendWithoutTargetSingleAgentDefault(t *testing.T) {
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()

	seedAgent(t, stores, "only-agent", "the sole agent", nil)

	params := map[string]any{
		"id": "task-single-default",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	resp := postA2AJSONRPC(t, ts.URL+"/a2a", "tasks/send", params)
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("unexpected error: %+v", rpcResp.Error)
	}

	var result a2a.TaskResult
	if err := json.Unmarshal(rpcResp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ID != "task-single-default" {
		t.Errorf("expected task ID 'task-single-default', got %q", result.ID)
	}
	if result.Status.State != a2a.TaskStateSubmitted {
		t.Errorf("expected state %q, got %q", a2a.TaskStateSubmitted, result.Status.State)
	}

	tasks, err := stores.Tasks.List(context.Background())
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task in store, got %d", len(tasks))
	}
	if tasks[0].Spec.System != "only-agent" {
		t.Errorf("expected task system 'only-agent', got %q", tasks[0].Spec.System)
	}
}

func TestCapabilities_A2ADisabledOmitsA2AEntries(t *testing.T) {
	ts, _ := newA2ATestServer(t, false)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var payload agentruntime.CapabilitySnapshot
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}

	for _, cap := range payload.Capabilities {
		if cap.ID == "a2a" || cap.ID == "a2a.streaming" || cap.ID == "a2a.registry" {
			t.Errorf("unexpected A2A capability when disabled: %+v", cap)
		}
	}
}
