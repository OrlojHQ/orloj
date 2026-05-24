package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
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
			PublicBaseURL:    "https://test.example.com",
			ProtocolVersion:  "1.0",
			StreamingEnabled: true,
			AuthSchemes:      []string{"bearer"},
		})
	}
	return httptest.NewServer(server.Handler()), stores
}

func seedAgent(t *testing.T, stores api.Stores, name, prompt string, tools []string) {
	t.Helper()
	seedAgentInNamespace(t, stores, "default", name, prompt, tools)
}

func seedAgentInNamespace(t *testing.T, stores api.Stores, namespace, name, prompt string, tools []string) {
	t.Helper()
	agent := resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata: resources.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
	system := resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata: resources.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"orloj.dev/description": "Test agent " + name,
			},
		},
		Spec: resources.AgentSystemSpec{
			Agents: []string{name},
			A2A:    resources.AgentSystemA2ASpec{Enabled: true},
		},
	}
	if _, err := stores.AgentSystems.Upsert(context.Background(), system); err != nil {
		t.Fatalf("failed to seed agentsystem %s: %v", name, err)
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
	return postA2AJSONRPCWithToken(t, url, method, params, "")
}

func postA2AJSONRPCWithToken(t *testing.T, url, method string, params any, token string) *http.Response {
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
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new POST %s failed: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	return resp
}

func newA2AAuthTestServer(t *testing.T) (*httptest.Server, api.Stores) {
	t.Helper()
	t.Setenv("ORLOJ_API_TOKENS", "client:client-token:a2a:default/allowed|team/allowed,writer:writer-token:writer")
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
	server.SetA2AConfig(&api.A2AConfig{
		PublicBaseURL:    "https://test.example.com",
		ProtocolVersion:  "1.0",
		StreamingEnabled: true,
		AuthSchemes:      []string{"bearer"},
	})
	return httptest.NewServer(server.Handler()), stores
}

// newA2AExplicitAuthModeOffTestServer mirrors orlojd with --auth-mode=off set
// explicitly. Token auth must stay active when env or DB tokens are configured.
// Callers must configure ORLOJ_API_TOKEN / ORLOJ_API_TOKENS before invoking.
func newA2AExplicitAuthModeOffTestServer(t *testing.T) (*httptest.Server, api.Stores) {
	t.Helper()
	logger := log.New(io.Discard, "", 0)
	stores := api.Stores{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		Tools:        store.NewToolStore(),
		Tasks:        store.NewTaskStore(),
		Workers:      store.NewWorkerStore(),
		Memories:     store.NewMemoryStore(),
		Policies:     store.NewAgentPolicyStore(),
		APITokens:    store.NewAPITokenStore(),
	}
	server := api.NewServerWithOptions(stores, agentruntime.NewManager(logger), logger, api.ServerOptions{
		AuthMode: api.AuthModeOff,
	})
	server.SetA2AConfig(&api.A2AConfig{
		PublicBaseURL:    "https://test.example.com",
		ProtocolVersion:  "1.0",
		StreamingEnabled: true,
		AuthSchemes:      []string{"bearer"},
	})
	return httptest.NewServer(server.Handler()), stores
}

func newA2ANativeAuthTestServer(t *testing.T) (*httptest.Server, api.Stores) {
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
		LocalAdmins:  store.NewLocalAdminStore(),
		AuthSessions: store.NewAuthSessionStore(),
	}
	hash, err := store.GeneratePasswordHash("very-strong-pass")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := stores.LocalAdmins.CreateUser("admin", hash, "admin"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	server := api.NewServerWithOptions(stores, agentruntime.NewManager(logger), logger, api.ServerOptions{AuthMode: api.AuthModeNative})
	server.SetA2AConfig(&api.A2AConfig{
		PublicBaseURL:    "https://test.example.com",
		ProtocolVersion:  "1.0",
		StreamingEnabled: true,
		AuthSchemes:      []string{"bearer"},
	})
	return httptest.NewServer(server.Handler()), stores
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
	if card.URL != "https://test.example.com/v1/agent-systems/assistant/a2a" {
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
	if rpcResp.Error.Code != a2a.ErrCodeInvalidParams {
		t.Errorf("expected error code %d, got %d", a2a.ErrCodeInvalidParams, rpcResp.Error.Code)
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

func TestA2ARegistry_NoEnabledSystemsReturnsEmptyList(t *testing.T) {
	ts, _ := newA2ATestServer(t, false)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/a2a/agents")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var registry a2a.RegistryResponse
	if err := json.NewDecoder(resp.Body).Decode(&registry); err != nil {
		t.Fatalf("decode registry: %v", err)
	}
	if len(registry.LocalAgents) != 0 {
		t.Fatalf("expected no local agents, got %d", len(registry.LocalAgents))
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
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()
	seedAgent(t, stores, "capability-system", "capability test", nil)

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

func TestA2AJSONRPC_ScopedTokenCanOnlyInvokeAllowedSystem(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedAgent(t, stores, "allowed", "allowed system", nil)
	seedAgent(t, stores, "blocked", "blocked system", nil)

	params := map[string]any{
		"id": "task-scoped",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	resp := postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/allowed/a2a", "tasks/send", params, "client-token")
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("expected allowed system invocation to succeed, got %+v", rpcResp.Error)
	}

	resp = postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/blocked/a2a", "tasks/send", params, "client-token")
	rpcResp = decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected scoped token to be denied for blocked system")
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/tools", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer client-token")
	toolsResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer toolsResp.Body.Close()
	if toolsResp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(toolsResp.Body)
		t.Fatalf("expected a2a token GET /v1/tools to be forbidden, got %d: %s", toolsResp.StatusCode, body)
	}
}

func TestA2AJSONRPC_TaskSendMetadataTargetCanUseScopedSystem(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedAgentInNamespace(t, stores, "team", "allowed", "allowed team system", nil)

	params := map[string]any{
		"id": "task-team-scoped",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
		"metadata": map[string]string{"target": "team/allowed"},
	}

	resp := postA2AJSONRPCWithToken(t, ts.URL+"/a2a", "tasks/send", params, "client-token")
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("expected scoped metadata target invocation to succeed, got %+v", rpcResp.Error)
	}

	tasks, err := stores.Tasks.List(context.Background())
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task in store, got %d", len(tasks))
	}
	if tasks[0].Metadata.Namespace != "team" {
		t.Fatalf("expected task namespace team, got %q", tasks[0].Metadata.Namespace)
	}
	if tasks[0].Spec.System != "allowed" {
		t.Fatalf("expected task system allowed, got %q", tasks[0].Spec.System)
	}
}

func TestA2ARegistry_WithAuthFiltersScope(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedAgent(t, stores, "allowed", "allowed system", nil)
	seedAgent(t, stores, "blocked", "blocked system", nil)

	// Unauthenticated: sees no systems (none are public)
	resp, err := http.Get(ts.URL + "/v1/a2a/agents")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected unauthenticated registry request to get 200, got %d: %s", resp.StatusCode, body)
	}
	var unauthRegistry a2a.RegistryResponse
	if err := json.NewDecoder(resp.Body).Decode(&unauthRegistry); err != nil {
		t.Fatalf("decode registry: %v", err)
	}
	if len(unauthRegistry.LocalAgents) != 0 {
		t.Fatalf("expected unauthenticated registry to show 0 systems, got %d", len(unauthRegistry.LocalAgents))
	}

	// Scoped token: sees only allowed system
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/a2a/agents", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer client-token")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected scoped registry request to get 200, got %d: %s", resp2.StatusCode, body)
	}
	var registry a2a.RegistryResponse
	if err := json.NewDecoder(resp2.Body).Decode(&registry); err != nil {
		t.Fatalf("decode registry: %v", err)
	}
	if len(registry.LocalAgents) != 1 || registry.LocalAgents[0].Name != "allowed" {
		t.Fatalf("expected registry to include only allowed system, got %+v", registry.LocalAgents)
	}
}

func TestA2AJSONRPC_NativeSessionCookieRejected(t *testing.T) {
	ts, stores := newA2ANativeAuthTestServer(t)
	defer ts.Close()

	seedAgent(t, stores, "native-system", "native system", nil)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginBody := []byte(`{"username":"admin","password":"very-strong-pass"}`)
	resp, err := client.Post(ts.URL+"/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected login 200, got %d", resp.StatusCode)
	}

	rpcReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  a2a.MethodTaskSend,
		"params": map[string]any{
			"id": "session-task",
			"message": map[string]any{
				"role":  "user",
				"parts": []map[string]any{{"type": "text", "text": "Hello"}},
			},
		},
	}
	body, err := json.Marshal(rpcReq)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/agent-systems/native-system/a2a", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected native session A2A request to be rejected with 401, got %d: %s", resp.StatusCode, body)
	}
}

func seedPublicAgent(t *testing.T, stores api.Stores, name, prompt string) {
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
		},
	}
	if _, err := stores.Agents.Upsert(context.Background(), agent); err != nil {
		t.Fatalf("failed to seed agent %s: %v", name, err)
	}
	system := resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata: resources.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Annotations: map[string]string{
				"orloj.dev/description": "Test agent " + name,
			},
		},
		Spec: resources.AgentSystemSpec{
			Agents: []string{name},
			A2A:    resources.AgentSystemA2ASpec{Enabled: true, Auth: resources.A2AAuthPublic},
		},
	}
	if _, err := stores.AgentSystems.Upsert(context.Background(), system); err != nil {
		t.Fatalf("failed to seed agentsystem %s: %v", name, err)
	}
}

func TestA2A_PublicSystemInvokeWithoutToken(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedPublicAgent(t, stores, "public-sys", "public system")

	params := map[string]any{
		"id": "task-public",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	// No token — should succeed because system is public
	resp := postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/public-sys/a2a", "tasks/send", params)
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("expected public system invoke without token to succeed, got %+v", rpcResp.Error)
	}
}

func TestA2A_PublicSystemInvokeWithValidToken(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedPublicAgent(t, stores, "public-sys", "public system")

	params := map[string]any{
		"id": "task-public-auth",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	// Valid token — should also succeed
	resp := postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/public-sys/a2a", "tasks/send", params, "writer-token")
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("expected public system invoke with valid token to succeed, got %+v", rpcResp.Error)
	}
}

func TestA2A_PublicSystemInvokeWithInvalidToken(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedPublicAgent(t, stores, "public-sys", "public system")

	params := map[string]any{
		"id": "task-bad-token",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	// Bad token — should be rejected at middleware (401)
	resp := postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/public-sys/a2a", "tasks/send", params, "bad-token-xyz")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected invalid token to get 401, got %d: %s", resp.StatusCode, body)
	}
}

func TestA2A_BearerSystemInvokeWithoutTokenDenied(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedAgent(t, stores, "bearer-sys", "bearer system", nil)

	params := map[string]any{
		"id": "task-bearer-notoken",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	// No token on a bearer-required system — denied via JSON-RPC error
	resp := postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/bearer-sys/a2a", "tasks/send", params)
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected bearer system invoke without token to be denied")
	}
}

func TestA2A_MixedPublicAndBearerSystems(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedPublicAgent(t, stores, "open-sys", "open system")
	seedAgent(t, stores, "allowed", "bearer system", nil)

	params := map[string]any{
		"id": "task-mixed",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	// Public system — no token needed
	resp := postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/open-sys/a2a", "tasks/send", params)
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("public system invoke failed: %+v", rpcResp.Error)
	}

	// Bearer system — no token → denied
	params["id"] = "task-mixed-bearer"
	resp = postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/allowed/a2a", "tasks/send", params)
	rpcResp = decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected bearer system invoke without token to be denied")
	}

	// Bearer system — scoped token → succeeds
	params["id"] = "task-mixed-bearer-auth"
	resp = postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/allowed/a2a", "tasks/send", params, "client-token")
	rpcResp = decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("bearer system invoke with valid token failed: %+v", rpcResp.Error)
	}
}

func TestA2A_BearerGetDeniedWithoutToken(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedAgent(t, stores, "allowed", "bearer system", nil)

	sendParams := map[string]any{
		"id": "task-bearer-get-auth",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	resp := postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/allowed/a2a", "tasks/send", sendParams, "client-token")
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("send with valid token failed: %+v", rpcResp.Error)
	}

	getParams := map[string]any{"id": "task-bearer-get-auth"}
	resp = postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/allowed/a2a", "tasks/get", getParams)
	rpcResp = decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected bearer system get without token to be denied")
	}
	if rpcResp.Error.Code != a2a.ErrCodeTaskNotFound {
		t.Errorf("expected error code %d (TaskNotFound), got %d", a2a.ErrCodeTaskNotFound, rpcResp.Error.Code)
	}
}

func TestA2A_BearerCancelDeniedWithoutToken(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedAgent(t, stores, "allowed", "bearer system", nil)

	sendParams := map[string]any{
		"id": "task-bearer-cancel-auth",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	resp := postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/allowed/a2a", "tasks/send", sendParams, "client-token")
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("send with valid token failed: %+v", rpcResp.Error)
	}

	cancelParams := map[string]any{"id": "task-bearer-cancel-auth", "reason": "test"}
	resp = postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/allowed/a2a", "tasks/cancel", cancelParams)
	rpcResp = decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected bearer system cancel without token to be denied")
	}
	if rpcResp.Error.Code != a2a.ErrCodeTaskNotFound {
		t.Errorf("expected error code %d (TaskNotFound), got %d", a2a.ErrCodeTaskNotFound, rpcResp.Error.Code)
	}
}

func TestA2A_BearerGetAllowedWithScopedToken(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedAgent(t, stores, "allowed", "bearer system", nil)

	sendParams := map[string]any{
		"id": "task-bearer-get-scoped",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	resp := postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/allowed/a2a", "tasks/send", sendParams, "client-token")
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("send with valid token failed: %+v", rpcResp.Error)
	}

	getParams := map[string]any{"id": "task-bearer-get-scoped"}
	resp = postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/allowed/a2a", "tasks/get", getParams, "client-token")
	rpcResp = decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("get with valid scoped token failed: %+v", rpcResp.Error)
	}

	var result a2a.TaskResult
	if err := json.Unmarshal(rpcResp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ID != "task-bearer-get-scoped" {
		t.Errorf("expected task ID 'task-bearer-get-scoped', got %q", result.ID)
	}
}

func TestA2A_PublicGetAllowedWithoutToken(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedPublicAgent(t, stores, "public-get-sys", "public system")

	sendParams := map[string]any{
		"id": "task-public-get",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	resp := postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/public-get-sys/a2a", "tasks/send", sendParams)
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("public send failed: %+v", rpcResp.Error)
	}

	getParams := map[string]any{"id": "task-public-get"}
	resp = postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/public-get-sys/a2a", "tasks/get", getParams)
	rpcResp = decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("public get without token failed: %+v", rpcResp.Error)
	}
}

// Regression: with auth-mode explicitly off (orlojd default), env/DB tokens must
// still be resolved for A2A invoke — otherwise bearer systems reject all callers.
func TestA2A_ExplicitAuthModeOff_BearerInvokeRequiresTokenResolution(t *testing.T) {
	taskParams := map[string]any{
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	t.Run("env scoped token", func(t *testing.T) {
		t.Setenv("ORLOJ_API_TOKENS", "lessee:lessee-token:a2a:default/premium")
		t.Setenv("ORLOJ_API_TOKEN", "")

		ts, stores := newA2AExplicitAuthModeOffTestServer(t)
		defer ts.Close()
		seedAgent(t, stores, "premium", "premium bearer system", nil)

		params := copyA2ATaskParams(taskParams, "task-env-notoken")
		resp := postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/premium/a2a", "tasks/send", params)
		rpcResp := decodeJSONRPCResponse(t, resp)
		if rpcResp.Error == nil {
			t.Fatal("expected bearer system invoke without token to be denied")
		}

		params = copyA2ATaskParams(taskParams, "task-env-scoped")
		resp = postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/premium/a2a", "tasks/send", params, "lessee-token")
		rpcResp = decodeJSONRPCResponse(t, resp)
		if rpcResp.Error != nil {
			t.Fatalf("expected scoped env token invoke to succeed, got %+v", rpcResp.Error)
		}
	})

	t.Run("db minted token", func(t *testing.T) {
		t.Setenv("ORLOJ_API_TOKENS", "")
		t.Setenv("ORLOJ_API_TOKEN", "ops-admin-token")

		ts, stores := newA2AExplicitAuthModeOffTestServer(t)
		defer ts.Close()
		seedAgent(t, stores, "premium", "premium bearer system", nil)

		createBody := []byte(`{"name":"lessee-db","role":"a2a","a2a_agent_systems":["premium"]}`)
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/tokens", bytes.NewReader(createBody))
		if err != nil {
			t.Fatalf("build mint request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer ops-admin-token")
		req.Header.Set("Content-Type", "application/json")
		mintResp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("mint token request failed: %v", err)
		}
		if mintResp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(mintResp.Body)
			mintResp.Body.Close()
			t.Fatalf("expected 201 mint token, got %d: %s", mintResp.StatusCode, body)
		}
		var minted map[string]any
		if err := json.NewDecoder(mintResp.Body).Decode(&minted); err != nil {
			mintResp.Body.Close()
			t.Fatalf("decode mint response: %v", err)
		}
		mintResp.Body.Close()
		subscriptionToken, _ := minted["token"].(string)
		if strings.TrimSpace(subscriptionToken) == "" {
			t.Fatal("expected subscription token secret in mint response")
		}

		params := copyA2ATaskParams(taskParams, "task-db-notoken")
		resp := postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/premium/a2a", "tasks/send", params)
		rpcResp := decodeJSONRPCResponse(t, resp)
		if rpcResp.Error == nil {
			t.Fatal("expected bearer system invoke without token to be denied")
		}

		params = copyA2ATaskParams(taskParams, "task-db-scoped")
		resp = postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/premium/a2a", "tasks/send", params, subscriptionToken)
		rpcResp = decodeJSONRPCResponse(t, resp)
		if rpcResp.Error != nil {
			t.Fatalf("expected DB-minted scoped token invoke to succeed, got %+v", rpcResp.Error)
		}
	})
}

func copyA2ATaskParams(base map[string]any, taskID string) map[string]any {
	out := make(map[string]any, len(base)+1)
	for k, v := range base {
		out[k] = v
	}
	out["id"] = taskID
	return out
}

func TestA2A_ExplicitAuthModeOff_NoTokens_AllowsPublicDeniesBearer(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	ts, stores := newA2AExplicitAuthModeOffTestServer(t)
	defer ts.Close()

	seedPublicAgent(t, stores, "pub-off", "public system")
	seedAgent(t, stores, "bearer-off", "bearer system", nil)

	params := map[string]any{
		"id": "task-off-public",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	resp := postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/pub-off/a2a", "tasks/send", params)
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("public system invoke on explicit auth-off should succeed: %+v", rpcResp.Error)
	}

	params["id"] = "task-off-bearer"
	resp = postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/bearer-off/a2a", "tasks/send", params)
	rpcResp = decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected bearer system invoke without token to be denied on explicit auth-off with no tokens")
	}
}

func TestA2A_PublicSystemCardOmitsAuthSchemes(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedPublicAgent(t, stores, "public-card-sys", "public card system")

	resp, err := http.Get(ts.URL + "/v1/agent-systems/public-card-sys/.well-known/agent-card.json")
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
	if card.Authentication != nil && len(card.Authentication.Schemes) > 0 {
		t.Fatalf("expected public system card to omit auth schemes, got %v", card.Authentication.Schemes)
	}
}

func TestA2A_BearerSystemCardIncludesAuthSchemes(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedAgent(t, stores, "bearer-card-sys", "bearer card system", nil)

	resp, err := http.Get(ts.URL + "/v1/agent-systems/bearer-card-sys/.well-known/agent-card.json")
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
	if card.Authentication == nil || len(card.Authentication.Schemes) == 0 {
		t.Fatal("expected bearer system card to include auth schemes")
	}
}

func TestA2ARegistry_PublicSystemsVisibleWithoutAuth(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedPublicAgent(t, stores, "pub-sys", "public system")
	seedAgent(t, stores, "allowed", "bearer system", nil)

	// Unauthenticated: sees only the public system
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
	if len(registry.LocalAgents) != 1 || registry.LocalAgents[0].Name != "pub-sys" {
		names := make([]string, len(registry.LocalAgents))
		for i, c := range registry.LocalAgents {
			names[i] = c.Name
		}
		t.Fatalf("expected only pub-sys in unauthenticated registry, got %v", names)
	}

	// Authenticated with scoped token: sees public system + scoped system
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/a2a/agents", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer client-token")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var authRegistry a2a.RegistryResponse
	if err := json.NewDecoder(resp2.Body).Decode(&authRegistry); err != nil {
		t.Fatalf("decode registry: %v", err)
	}
	if len(authRegistry.LocalAgents) != 2 {
		names := make([]string, len(authRegistry.LocalAgents))
		for i, c := range authRegistry.LocalAgents {
			names[i] = c.Name
		}
		t.Fatalf("expected 2 systems in authenticated registry, got %v", names)
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

func TestA2A_BearerSubscribeDeniedWithoutToken(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedAgent(t, stores, "allowed", "bearer system", nil)

	params := map[string]any{
		"id": "task-subscribe-notoken",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	resp := postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/allowed/a2a", "tasks/sendSubscribe", params)
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected bearer system subscribe without token to be denied")
	}
	if rpcResp.Error.Code != a2a.ErrCodeAgentNotFound {
		t.Errorf("expected error code %d (AgentNotFound), got %d", a2a.ErrCodeAgentNotFound, rpcResp.Error.Code)
	}
}

func TestA2A_BearerCancelAllowedWithScopedToken(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedAgent(t, stores, "allowed", "bearer system", nil)

	sendParams := map[string]any{
		"id": "task-bearer-cancel-scoped",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	resp := postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/allowed/a2a", "tasks/send", sendParams, "client-token")
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("send with valid token failed: %+v", rpcResp.Error)
	}

	cancelParams := map[string]any{"id": "task-bearer-cancel-scoped", "reason": "done"}
	resp = postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/allowed/a2a", "tasks/cancel", cancelParams, "client-token")
	rpcResp = decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("cancel with valid scoped token failed: %+v", rpcResp.Error)
	}

	var result a2a.TaskResult
	if err := json.Unmarshal(rpcResp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ID != "task-bearer-cancel-scoped" {
		t.Errorf("expected task ID 'task-bearer-cancel-scoped', got %q", result.ID)
	}
	if result.Status.State != "canceled" {
		t.Errorf("expected cancelled task state 'canceled', got %q", result.Status.State)
	}
}

func TestA2A_PublicCancelAllowedWithoutToken(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedPublicAgent(t, stores, "public-cancel-sys", "public system")

	sendParams := map[string]any{
		"id": "task-public-cancel",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	resp := postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/public-cancel-sys/a2a", "tasks/send", sendParams)
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("public send failed: %+v", rpcResp.Error)
	}

	cancelParams := map[string]any{"id": "task-public-cancel", "reason": "test"}
	resp = postA2AJSONRPC(t, ts.URL+"/v1/agent-systems/public-cancel-sys/a2a", "tasks/cancel", cancelParams)
	rpcResp = decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("public cancel without token failed: %+v", rpcResp.Error)
	}
}

func TestA2A_OutOfScopeTokenGetDenied(t *testing.T) {
	ts, stores := newA2AAuthTestServer(t)
	defer ts.Close()

	seedAgent(t, stores, "allowed", "bearer system", nil)
	seedAgent(t, stores, "blocked", "blocked system", nil)

	sendParams := map[string]any{
		"id": "task-scope-test",
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"type": "text", "text": "Hello"}},
		},
	}

	// Create a task on "allowed" (in scope for client-token)
	resp := postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/allowed/a2a", "tasks/send", sendParams, "client-token")
	rpcResp := decodeJSONRPCResponse(t, resp)
	if rpcResp.Error != nil {
		t.Fatalf("send to allowed system failed: %+v", rpcResp.Error)
	}

	// Try to get via "blocked" system URL with client-token (not scoped for "blocked")
	getParams := map[string]any{"id": "task-scope-test"}
	resp = postA2AJSONRPCWithToken(t, ts.URL+"/v1/agent-systems/blocked/a2a", "tasks/get", getParams, "client-token")
	rpcResp = decodeJSONRPCResponse(t, resp)
	if rpcResp.Error == nil {
		t.Fatal("expected out-of-scope token get to be denied")
	}
	if rpcResp.Error.Code != a2a.ErrCodeTaskNotFound {
		t.Errorf("expected error code %d (TaskNotFound), got %d", a2a.ErrCodeTaskNotFound, rpcResp.Error.Code)
	}
}
