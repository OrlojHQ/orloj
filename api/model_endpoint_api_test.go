package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/AnonJon/orloj/crds"
)

func TestModelEndpointCRUDAndNamespaceScoping(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/model-endpoints", crds.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata: crds.ObjectMeta{
			Name:      "openai-shared",
			Namespace: "team-a",
		},
		Spec: crds.ModelEndpointSpec{Provider: "openai", DefaultModel: "gpt-4o-mini"},
	})
	postJSON(t, server.URL+"/v1/model-endpoints", crds.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata: crds.ObjectMeta{
			Name:      "openai-shared",
			Namespace: "team-b",
		},
		Spec: crds.ModelEndpointSpec{Provider: "openai", DefaultModel: "gpt-4o"},
	})

	resp, err := http.Get(server.URL + "/v1/model-endpoints/openai-shared?namespace=team-b")
	if err != nil {
		t.Fatalf("get namespaced model endpoint failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
	}
	var endpoint crds.ModelEndpoint
	if err := json.NewDecoder(resp.Body).Decode(&endpoint); err != nil {
		t.Fatalf("decode model endpoint failed: %v", err)
	}
	if endpoint.Metadata.Namespace != "team-b" {
		t.Fatalf("expected team-b endpoint, got %q", endpoint.Metadata.Namespace)
	}
	if endpoint.Spec.DefaultModel != "gpt-4o" {
		t.Fatalf("unexpected default model %q", endpoint.Spec.DefaultModel)
	}

	respDefault, err := http.Get(server.URL + "/v1/model-endpoints/openai-shared")
	if err != nil {
		t.Fatalf("get default namespace model endpoint failed: %v", err)
	}
	defer respDefault.Body.Close()
	if respDefault.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(respDefault.Body)
		t.Fatalf("expected 404 for default namespace lookup, got %d body=%s", respDefault.StatusCode, string(body))
	}
}

func TestModelEndpointStatusSubresource(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/model-endpoints", crds.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata:   crds.ObjectMeta{Name: "openai-default"},
		Spec:       crds.ModelEndpointSpec{Provider: "openai", DefaultModel: "gpt-4o-mini"},
	})

	resp, err := http.Get(server.URL + "/v1/model-endpoints/openai-default")
	if err != nil {
		t.Fatalf("get endpoint failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
	}
	var endpoint crds.ModelEndpoint
	if err := json.NewDecoder(resp.Body).Decode(&endpoint); err != nil {
		t.Fatalf("decode endpoint failed: %v", err)
	}

	patch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": endpoint.Metadata.ResourceVersion,
		},
		"status": map[string]any{
			"phase": "Ready",
		},
	}
	body, _ := json.Marshal(patch)
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/model-endpoints/openai-default/status", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	statusResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("status put failed: %v", err)
	}
	defer statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(statusResp.Body)
		t.Fatalf("expected 200, got %d body=%s", statusResp.StatusCode, string(respBody))
	}
}
