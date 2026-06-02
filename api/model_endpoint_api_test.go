package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestModelEndpointCRUDAndNamespaceScoping(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/model-endpoints?namespace=team-a", resources.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata: resources.ObjectMeta{
			Name:      "openai-shared",
			Namespace: "team-a",
		},
		Spec: resources.ModelEndpointSpec{Provider: "openai", DefaultModel: "gpt-4o-mini"},
	})
	postJSON(t, server.URL+"/v1/model-endpoints?namespace=team-b", resources.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata: resources.ObjectMeta{
			Name:      "openai-shared",
			Namespace: "team-b",
		},
		Spec: resources.ModelEndpointSpec{Provider: "openai", DefaultModel: "gpt-4o"},
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
	var endpoint resources.ModelEndpoint
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

	postJSON(t, server.URL+"/v1/model-endpoints", resources.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata:   resources.ObjectMeta{Name: "openai-default"},
		Spec:       resources.ModelEndpointSpec{Provider: "openai", DefaultModel: "gpt-4o-mini"},
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
	var endpoint resources.ModelEndpoint
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

func TestModelEndpointPutRenameFromYAMLBody(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/model-endpoints", resources.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata:   resources.ObjectMeta{Name: "orig-name"},
		Spec:       resources.ModelEndpointSpec{Provider: "openai", DefaultModel: "gpt-4o-mini"},
	})

	resp, err := http.Get(server.URL + "/v1/model-endpoints/orig-name")
	if err != nil {
		t.Fatalf("get endpoint failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
	}
	var cur resources.ModelEndpoint
	if err := json.NewDecoder(resp.Body).Decode(&cur); err != nil {
		t.Fatalf("decode: %v", err)
	}

	cur.Metadata.Name = "new-name"
	body, err := json.Marshal(cur)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/model-endpoints/orig-name", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", cur.Metadata.ResourceVersion)
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(putResp.Body)
		t.Fatalf("expected 200, got %d body=%s", putResp.StatusCode, string(b))
	}
	var updated resources.ModelEndpoint
	if err := json.NewDecoder(putResp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode put response: %v", err)
	}
	if updated.Metadata.Name != "new-name" {
		t.Fatalf("expected renamed metadata.name %q, got %q", "new-name", updated.Metadata.Name)
	}

	getOld, err := http.Get(server.URL + "/v1/model-endpoints/orig-name")
	if err != nil {
		t.Fatalf("get old: %v", err)
	}
	defer getOld.Body.Close()
	if getOld.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for old name, got %d", getOld.StatusCode)
	}

	getNew, err := http.Get(server.URL + "/v1/model-endpoints/new-name")
	if err != nil {
		t.Fatalf("get new: %v", err)
	}
	defer getNew.Body.Close()
	if getNew.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(getNew.Body)
		t.Fatalf("expected 200 for new name, got %d body=%s", getNew.StatusCode, string(b))
	}
}
