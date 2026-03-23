package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestPutRequiresResourceVersionPrecondition(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/tools", resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "web"},
		Spec:       resources.ToolSpec{Type: "http", Endpoint: "https://example"},
	})

	body, _ := json.Marshal(resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "web"},
		Spec:       resources.ToolSpec{Type: "http", Endpoint: "https://example/v2"},
	})
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/tools/web", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 409 for missing precondition, got %d body=%s", resp.StatusCode, string(b))
	}
}

func TestToolStatusSubresourceAndConflict(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/tools", resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "web"},
		Spec:       resources.ToolSpec{Type: "http", Endpoint: "https://example"},
	})

	tool := getTool(t, server.URL+"/v1/tools/web")
	patch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": tool.Metadata.ResourceVersion,
		},
		"status": map[string]any{
			"phase": "Ready",
		},
	}
	body, _ := json.Marshal(patch)
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/tools/web/status", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("status update failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 for status update, got %d body=%s", resp.StatusCode, string(b))
	}

	updated := getTool(t, server.URL+"/v1/tools/web")
	if updated.Status.Phase != "Ready" {
		t.Fatalf("expected status.phase=Ready, got %q", updated.Status.Phase)
	}
	if updated.Status.ObservedGeneration != updated.Metadata.Generation {
		t.Fatalf("expected observedGeneration=%d, got %d", updated.Metadata.Generation, updated.Status.ObservedGeneration)
	}

	// stale resourceVersion should conflict.
	stalePatch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": "1",
		},
		"status": map[string]any{
			"phase": "Error",
		},
	}
	staleBody, _ := json.Marshal(stalePatch)
	req2, err := http.NewRequest(http.MethodPut, server.URL+"/v1/tools/web/status", bytes.NewReader(staleBody))
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("stale status update failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 409 for stale status update, got %d body=%s", resp2.StatusCode, string(b))
	}
}

func getTool(t *testing.T, url string) resources.Tool {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get tool failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("get tool status=%d body=%s", resp.StatusCode, string(b))
	}
	var out resources.Tool
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode tool failed: %v", err)
	}
	return out
}
