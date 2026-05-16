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
	"github.com/OrlojHQ/orloj/crds"
	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

func newTestServerWithConflictPolicy(t *testing.T, policy string) *httptest.Server {
	t.Helper()
	logger := log.New(io.Discard, "", 0)
	runtimeMgr := agentruntime.NewManager(logger)
	server := api.NewServerWithOptions(api.Stores{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		Tools:        store.NewToolStore(),
		Secrets:      store.NewSecretStore(),
		Memories:     store.NewMemoryStore(),
		Policies:     store.NewAgentPolicyStore(),
		McpServers:   store.NewMcpServerStore(),
		ModelEPs:     store.NewModelEndpointStore(),
		Tasks:        store.NewTaskStore(),
		Workers:      store.NewWorkerStore(),
	}, runtimeMgr, logger, api.ServerOptions{
		CRDConflictPolicy: policy,
	})
	return httptest.NewServer(server.Handler())
}

// TestCRDConflictGuard_POST_Reject verifies that POST (upsert) to a
// CRD-managed resource is rejected with 409 when policy is "reject".
func TestCRDConflictGuard_POST_Reject(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newTestServerWithConflictPolicy(t, "reject")
	defer server.Close()

	endpoints := []struct {
		name string
		path string
		body interface{}
	}{
		{"Agent", "/v1/agents", resources.Agent{
			APIVersion: "orloj.dev/v1", Kind: "Agent",
			Metadata: resources.ObjectMeta{Name: "crd-agent", Namespace: "default"},
			Spec:     resources.AgentSpec{ModelRef: "gpt-4", Prompt: "test"},
		}},
		{"AgentSystem", "/v1/agent-systems", resources.AgentSystem{
			APIVersion: "orloj.dev/v1", Kind: "AgentSystem",
			Metadata: resources.ObjectMeta{Name: "crd-sys", Namespace: "default"},
			Spec:     resources.AgentSystemSpec{Agents: []string{"a1"}},
		}},
		{"Tool", "/v1/tools", resources.Tool{
			APIVersion: "orloj.dev/v1", Kind: "Tool",
			Metadata: resources.ObjectMeta{Name: "crd-tool", Namespace: "default"},
			Spec:     resources.ToolSpec{Description: "test"},
		}},
		{"McpServer", "/v1/mcp-servers", resources.McpServer{
			APIVersion: "orloj.dev/v1", Kind: "McpServer",
			Metadata: resources.ObjectMeta{Name: "crd-mcp", Namespace: "default"},
			Spec:     resources.McpServerSpec{Transport: "stdio", Command: "echo"},
		}},
		{"ModelEndpoint", "/v1/model-endpoints", resources.ModelEndpoint{
			APIVersion: "orloj.dev/v1", Kind: "ModelEndpoint",
			Metadata: resources.ObjectMeta{Name: "crd-ep", Namespace: "default"},
			Spec:     resources.ModelEndpointSpec{Provider: "openai", DefaultModel: "gpt-4o"},
		}},
		{"Memory", "/v1/memories", resources.Memory{
			APIVersion: "orloj.dev/v1", Kind: "Memory",
			Metadata: resources.ObjectMeta{Name: "crd-mem", Namespace: "default"},
			Spec:     resources.MemoryConfig{Type: "vector", Provider: "builtin"},
		}},
		{"AgentPolicy", "/v1/agent-policies", resources.AgentPolicy{
			APIVersion: "orloj.dev/v1", Kind: "AgentPolicy",
			Metadata: resources.ObjectMeta{Name: "crd-policy", Namespace: "default"},
			Spec:     resources.AgentPolicySpec{},
		}},
		{"Secret", "/v1/secrets", resources.Secret{
			APIVersion: "orloj.dev/v1", Kind: "Secret",
			Metadata: resources.ObjectMeta{Name: "crd-secret", Namespace: "default"},
			Spec:     resources.SecretSpec{StringData: map[string]string{"key": "val"}},
		}},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			// First create the resource (no conflict yet)
			body, _ := json.Marshal(ep.body)
			resp, err := http.Post(server.URL+ep.path, "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("initial POST failed: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				t.Fatalf("initial POST: expected 200/201, got %d", resp.StatusCode)
			}

			// Now simulate CRD ownership by adding the annotation directly to the store
			markCRDManaged(t, server.URL, ep.path, ep.name)

			// POST again — should be rejected
			resp2, err := http.Post(server.URL+ep.path, "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("conflict POST failed: %v", err)
			}
			resp2.Body.Close()
			if resp2.StatusCode != http.StatusConflict {
				t.Errorf("expected 409 Conflict, got %d", resp2.StatusCode)
			}
			if resp2.Header.Get("X-Orloj-CRD-Managed") != "true" {
				t.Error("expected X-Orloj-CRD-Managed header")
			}
		})
	}
}

// TestCRDConflictGuard_POST_Warn verifies warn mode allows writes but sets header.
func TestCRDConflictGuard_POST_Warn(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newTestServerWithConflictPolicy(t, "warn")
	defer server.Close()

	// Create an agent
	agent := resources.Agent{
		APIVersion: "orloj.dev/v1", Kind: "Agent",
		Metadata: resources.ObjectMeta{Name: "warn-agent", Namespace: "default"},
		Spec:     resources.AgentSpec{ModelRef: "gpt-4", Prompt: "test"},
	}
	body, _ := json.Marshal(agent)
	resp, err := http.Post(server.URL+"/v1/agents", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Mark as CRD-managed
	markCRDManaged(t, server.URL, "/v1/agents", "Agent")

	// POST again — should succeed with warning header
	resp2, err := http.Post(server.URL+"/v1/agents", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode == http.StatusConflict {
		t.Error("warn mode should not reject")
	}
	if resp2.Header.Get("X-Orloj-CRD-Managed") != "true" {
		t.Error("expected X-Orloj-CRD-Managed warning header")
	}
}

// TestCRDConflictGuard_POST_Off verifies off mode is fully transparent.
func TestCRDConflictGuard_POST_Off(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newTestServerWithConflictPolicy(t, "off")
	defer server.Close()

	agent := resources.Agent{
		APIVersion: "orloj.dev/v1", Kind: "Agent",
		Metadata: resources.ObjectMeta{Name: "off-agent", Namespace: "default"},
		Spec:     resources.AgentSpec{ModelRef: "gpt-4", Prompt: "test"},
	}
	body, _ := json.Marshal(agent)
	resp, err := http.Post(server.URL+"/v1/agents", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Mark as CRD-managed
	markCRDManaged(t, server.URL, "/v1/agents", "Agent")

	// POST again — no conflict, no header
	resp2, err := http.Post(server.URL+"/v1/agents", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode == http.StatusConflict {
		t.Error("off mode should not reject")
	}
	if resp2.Header.Get("X-Orloj-CRD-Managed") != "" {
		t.Error("off mode should not set header")
	}
}

// markCRDManaged uses a PUT to update the resource with the CRD-managed annotation.
// This simulates what the operator's reconciler does internally.
func markCRDManaged(t *testing.T, baseURL, collectionPath, kind string) {
	t.Helper()

	var name string
	switch kind {
	case "Agent":
		name = getResourceName(t, baseURL, collectionPath, kind)
	case "AgentSystem":
		name = getResourceName(t, baseURL, collectionPath, kind)
	case "Tool":
		name = getResourceName(t, baseURL, collectionPath, kind)
	case "McpServer":
		name = getResourceName(t, baseURL, collectionPath, kind)
	case "ModelEndpoint":
		name = getResourceName(t, baseURL, collectionPath, kind)
	case "Memory":
		name = getResourceName(t, baseURL, collectionPath, kind)
	case "AgentPolicy":
		name = getResourceName(t, baseURL, collectionPath, kind)
	case "Secret":
		name = getResourceName(t, baseURL, collectionPath, kind)
	default:
		t.Fatalf("unknown kind: %s", kind)
	}

	// GET the resource, add annotation, PUT back
	resp, err := http.Get(baseURL + collectionPath + "/" + name)
	if err != nil {
		t.Fatalf("GET %s: %v", name, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	metadata, _ := raw["metadata"].(map[string]interface{})
	if metadata == nil {
		metadata = make(map[string]interface{})
		raw["metadata"] = metadata
	}
	annotations, _ := metadata["annotations"].(map[string]interface{})
	if annotations == nil {
		annotations = make(map[string]interface{})
		metadata["annotations"] = annotations
	}
	annotations[crds.AnnotationManagedBy] = crds.ManagedByCRDSync

	putBody, _ := json.Marshal(raw)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, baseURL+collectionPath+"/"+name, bytes.NewReader(putBody))
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", name, err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT %s: expected 200, got %d", name, putResp.StatusCode)
	}
}

func getResourceName(t *testing.T, baseURL, collectionPath, _ string) string {
	t.Helper()
	resp, err := http.Get(baseURL + collectionPath)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var listResp struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		t.Fatalf("unmarshal list: %v (body: %s)", err, string(body))
	}
	if len(listResp.Items) == 0 {
		t.Fatal("no resources found")
	}
	return listResp.Items[0].Metadata.Name
}
