package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestNamespaceScopedResourceLookup(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/tools", resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata: resources.ObjectMeta{
			Name:      "shared-tool",
			Namespace: "team-a",
		},
		Spec: resources.ToolSpec{Type: "http", Endpoint: "https://a.example"},
	})
	postJSON(t, server.URL+"/v1/tools", resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata: resources.ObjectMeta{
			Name:      "shared-tool",
			Namespace: "team-b",
		},
		Spec: resources.ToolSpec{Type: "http", Endpoint: "https://b.example"},
	})

	respA, err := http.Get(server.URL + "/v1/tools/shared-tool?namespace=team-a")
	if err != nil {
		t.Fatalf("get team-a tool failed: %v", err)
	}
	defer respA.Body.Close()
	if respA.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respA.Body)
		t.Fatalf("expected 200 for team-a lookup, got %d body=%s", respA.StatusCode, string(body))
	}
	var toolA resources.Tool
	if err := json.NewDecoder(respA.Body).Decode(&toolA); err != nil {
		t.Fatalf("decode team-a tool failed: %v", err)
	}
	if toolA.Metadata.Namespace != "team-a" {
		t.Fatalf("expected namespace team-a, got %q", toolA.Metadata.Namespace)
	}

	respB, err := http.Get(server.URL + "/v1/tools?namespace=team-b")
	if err != nil {
		t.Fatalf("list team-b tools failed: %v", err)
	}
	defer respB.Body.Close()
	if respB.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respB.Body)
		t.Fatalf("expected 200 for team-b list, got %d body=%s", respB.StatusCode, string(body))
	}
	var list resources.ToolList
	if err := json.NewDecoder(respB.Body).Decode(&list); err != nil {
		t.Fatalf("decode team-b list failed: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Metadata.Namespace != "team-b" {
		t.Fatalf("expected exactly one team-b tool, got %+v", list.Items)
	}

	respDefault, err := http.Get(server.URL + "/v1/tools/shared-tool")
	if err != nil {
		t.Fatalf("get default namespace tool failed: %v", err)
	}
	defer respDefault.Body.Close()
	if respDefault.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(respDefault.Body)
		t.Fatalf("expected 404 for default namespace lookup, got %d body=%s", respDefault.StatusCode, string(body))
	}
}
