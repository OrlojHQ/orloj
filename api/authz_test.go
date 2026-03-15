package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OrlojHQ/orloj/api"
	"github.com/OrlojHQ/orloj/crds"
	"github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

func TestAuthzEnforcement(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "reader-token:reader,writer-token:writer,controller-token:controller,admin-token:admin")
	t.Setenv("ORLOJ_API_TOKEN", "")

	logger := log.New(io.Discard, "", 0)
	server := api.NewServer(api.Stores{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		Tools:        store.NewToolStore(),
		Memories:     store.NewMemoryStore(),
		Policies:     store.NewAgentPolicyStore(),
		Tasks:        store.NewTaskStore(),
		Workers:      store.NewWorkerStore(),
	}, agentruntime.NewManager(logger), logger)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	// Health should remain open.
	resp, err := http.Get(httpServer.URL + "/healthz")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected healthz=200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Reader can read.
	req, _ := http.NewRequest(http.MethodGet, httpServer.URL+"/v1/tasks", nil)
	req.Header.Set("Authorization", "Bearer reader-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reader get failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected reader GET 200, got %d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	// Missing token is unauthorized.
	req, _ = http.NewRequest(http.MethodGet, httpServer.URL+"/v1/tasks", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unauthorized get failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected GET without token 401, got %d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, httpServer.URL+"/v1/task-webhooks", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unauthorized task-webhooks get failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected task-webhooks GET without token 401, got %d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	// Webhook delivery endpoint bypasses bearer auth and uses signature-based auth.
	req, _ = http.NewRequest(http.MethodPost, httpServer.URL+"/v1/webhook-deliveries/nonexistent", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("webhook delivery request failed: %v", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected webhook delivery auth bypass (non-401/403), got %d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	// Writer can create spec resources.
	payload, _ := json.Marshal(crds.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   crds.ObjectMeta{Name: "t1"},
		Spec:       crds.ToolSpec{Type: "http", Endpoint: "https://example"},
	})
	req, _ = http.NewRequest(http.MethodPost, httpServer.URL+"/v1/tools", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer writer-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("writer post failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected writer POST 201, got %d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, httpServer.URL+"/v1/tools/t1", nil)
	req.Header.Set("Authorization", "Bearer reader-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get tool failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected get tool 200, got %d body=%s", resp.StatusCode, string(b))
	}
	var tool crds.Tool
	if err := json.NewDecoder(resp.Body).Decode(&tool); err != nil {
		t.Fatalf("decode tool failed: %v", err)
	}
	resp.Body.Close()
	statusPatch := map[string]any{
		"metadata": map[string]any{"resourceVersion": tool.Metadata.ResourceVersion},
		"status":   map[string]any{"phase": "Ready"},
	}
	patchBytes, _ := json.Marshal(statusPatch)

	// Writer cannot write status.
	req, _ = http.NewRequest(http.MethodPut, httpServer.URL+"/v1/tools/t1/status", bytes.NewReader(patchBytes))
	req.Header.Set("Authorization", "Bearer writer-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("writer status put failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected writer status PUT 403, got %d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	// Controller can write status.
	req, _ = http.NewRequest(http.MethodPut, httpServer.URL+"/v1/tools/t1/status", bytes.NewReader(patchBytes))
	req.Header.Set("Authorization", "Bearer controller-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("controller status put failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected controller status PUT 200, got %d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()
}
