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
	"github.com/OrlojHQ/orloj/resources"
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

	// Webhook delivery endpoint is exempt from global API auth; the handler
	// performs its own signature/token verification via the TaskWebhook auth
	// profile. An unauthenticated request reaches the handler (404 for
	// nonexistent endpoint, not 401 from the auth middleware).
	req, _ = http.NewRequest(http.MethodPost, httpServer.URL+"/v1/webhook-deliveries/nonexistent", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("webhook delivery unauthenticated request failed: %v", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected webhook delivery to bypass global auth, got %d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	// Writer can create spec resources.
	payload, _ := json.Marshal(resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "t1"},
		Spec:       resources.ToolSpec{Type: "http", Endpoint: "https://example"},
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

	// Token management endpoints are admin-only.
	req, _ = http.NewRequest(http.MethodGet, httpServer.URL+"/v1/tokens", nil)
	req.Header.Set("Authorization", "Bearer writer-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("writer tokens get failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected writer GET /v1/tokens 403, got %d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, httpServer.URL+"/v1/tokens", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin tokens get failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected admin GET /v1/tokens 200, got %d body=%s", resp.StatusCode, string(b))
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
	var tool resources.Tool
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

	// Writers must NOT be able to create cli tools that execute on the host
	// (spec.type=cli + spec.runtime.isolation_mode=none). Writers can still
	// create container/wasm CLI tools and other tool types; only the host
	// execution path is admin-gated.
	hostCliPayload, _ := json.Marshal(resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "host-cli"},
		Spec: resources.ToolSpec{
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "kubectl",
				Output:  "stdout",
			},
			Runtime: resources.ToolRuntimePolicy{
				IsolationMode: "none",
			},
		},
	})
	req, _ = http.NewRequest(http.MethodPost, httpServer.URL+"/v1/tools", bytes.NewReader(hostCliPayload))
	req.Header.Set("Authorization", "Bearer writer-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("writer host-cli post failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected writer POST host-cli tool 403, got %d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	// Admins can create the same host CLI tool.
	req, _ = http.NewRequest(http.MethodPost, httpServer.URL+"/v1/tools", bytes.NewReader(hostCliPayload))
	req.Header.Set("Authorization", "Bearer admin-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin host-cli post failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected admin POST host-cli tool 201, got %d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	// YAML manifests must be classified the same way: a YAML cli+none body
	// posted by a writer must also be rejected. This guards against a
	// JSON-only probe accidentally letting YAML payloads through.
	yamlHostCli := []byte(`apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: host-cli-yaml
spec:
  type: cli
  cli:
    command: kubectl
    output: stdout
  runtime:
    isolation_mode: none
`)
	req, _ = http.NewRequest(http.MethodPost, httpServer.URL+"/v1/tools", bytes.NewReader(yamlHostCli))
	req.Header.Set("Authorization", "Bearer writer-token")
	req.Header.Set("Content-Type", "application/yaml")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("writer yaml host-cli post failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected writer YAML POST host-cli tool 403, got %d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	// Writers can still create container-isolated CLI tools.
	containerCliPayload, _ := json.Marshal(resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "container-cli"},
		Spec: resources.ToolSpec{
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "kubectl",
				Image:   "bitnami/kubectl:1.30",
				Output:  "stdout",
			},
			Runtime: resources.ToolRuntimePolicy{
				IsolationMode: "container",
			},
		},
	})
	req, _ = http.NewRequest(http.MethodPost, httpServer.URL+"/v1/tools", bytes.NewReader(containerCliPayload))
	req.Header.Set("Authorization", "Bearer writer-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("writer container-cli post failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected writer POST container-cli tool 201, got %d body=%s", resp.StatusCode, string(b))
	}
	resp.Body.Close()
}
