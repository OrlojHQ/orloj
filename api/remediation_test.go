package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestNamespaceBodyMismatchRejected(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	resp, err := http.Post(server.URL+"/v1/tools", "application/json", bytes.NewBufferString(`{
		"apiVersion":"orloj.dev/v1",
		"kind":"Tool",
		"metadata":{"name":"cross-ns-tool","namespace":"production"},
		"spec":{"type":"http","endpoint":"https://example.com"}
	}`))
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 for body namespace mismatch, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestPaginationEmitsScopedContinueToken(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/tools?namespace=team-a", resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "alpha", Namespace: "team-a"},
		Spec:       resources.ToolSpec{Type: "http", Endpoint: "https://a.example"},
	})
	postJSON(t, server.URL+"/v1/tools?namespace=team-b", resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "beta", Namespace: "team-b"},
		Spec:       resources.ToolSpec{Type: "http", Endpoint: "https://b.example"},
	})

	resp, err := http.Get(server.URL + "/v1/tools?limit=1")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	defer resp.Body.Close()
	var list resources.ToolList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if list.Continue == "" {
		t.Fatal("expected continue token")
	}
	if list.Continue != "default/alpha" && list.Continue != "team-a/alpha" && list.Continue != "team-b/beta" && list.Continue != "default/beta" {
		t.Fatalf("expected scoped continue token, got %q", list.Continue)
	}
}

func TestLabelSelectorPaginationDoesNotReturnEmptyPageWithContinue(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	for _, name := range []string{"plain-a", "plain-b", "tagged-c"} {
		labels := map[string]string{}
		if name == "tagged-c" {
			labels["env"] = "prod"
		}
		postJSON(t, server.URL+"/v1/tools", resources.Tool{
			APIVersion: "orloj.dev/v1",
			Kind:       "Tool",
			Metadata:   resources.ObjectMeta{Name: name, Labels: labels},
			Spec:       resources.ToolSpec{Type: "http", Endpoint: "https://example.com"},
		})
	}

	resp, err := http.Get(server.URL + "/v1/tools?limit=1&labelSelector=env=prod")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	defer resp.Body.Close()
	var list resources.ToolList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(list.Items) == 0 {
		t.Fatal("expected at least one matching item")
	}
	if list.Continue != "" && len(list.Items) == 0 {
		t.Fatalf("unexpected empty page with continue=%q", list.Continue)
	}
}

func TestUnsupportedMutationContentTypeReturns415(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/tools", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 415, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestTaskWebhookListPaginationEmitsContinue(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	for _, name := range []string{"hook-alpha", "hook-beta"} {
		postJSON(t, server.URL+"/v1/task-webhooks", resources.TaskWebhook{
			APIVersion: "orloj.dev/v1",
			Kind:       "TaskWebhook",
			Metadata:   resources.ObjectMeta{Name: name},
			Spec: resources.TaskWebhookSpec{
				TaskRef: "weekly-report-template",
				Auth:    resources.TaskWebhookAuthSpec{SecretRef: "build-webhook-secret"},
			},
		})
	}

	resp, err := http.Get(server.URL + "/v1/task-webhooks?limit=1")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	defer resp.Body.Close()
	var list resources.TaskWebhookList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list.Items))
	}
	if list.Continue == "" {
		t.Fatal("expected continue token for task-webhooks list")
	}
	if !strings.Contains(list.Continue, "/") {
		t.Fatalf("expected scoped continue token, got %q", list.Continue)
	}
}
