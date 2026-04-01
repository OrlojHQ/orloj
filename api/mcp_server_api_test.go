package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestMcpServerPutRenameFromBody(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/mcp-servers", resources.McpServer{
		APIVersion: "orloj.dev/v1",
		Kind:       "McpServer",
		Metadata:   resources.ObjectMeta{Name: "orig-mcp"},
		Spec: resources.McpServerSpec{
			Transport: "stdio",
			Command:   "npx",
			Args:      []string{"-y", "@modelcontextprotocol/server-everything"},
		},
	})

	getResp, err := http.Get(server.URL + "/v1/mcp-servers/orig-mcp")
	if err != nil {
		t.Fatalf("get mcp-server: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(getResp.Body)
		t.Fatalf("expected 200 get, got %d body=%s", getResp.StatusCode, string(b))
	}
	var cur resources.McpServer
	if err := json.NewDecoder(getResp.Body).Decode(&cur); err != nil {
		t.Fatalf("decode get response: %v", err)
	}

	cur.Metadata.Name = "renamed-mcp"
	cur.Spec.Command = "node"
	putBody, err := json.Marshal(cur)
	if err != nil {
		t.Fatalf("marshal put body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/mcp-servers/orig-mcp", bytes.NewReader(putBody))
	if err != nil {
		t.Fatalf("new put request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", cur.Metadata.ResourceVersion)
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put mcp-server: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(putResp.Body)
		t.Fatalf("expected 200 put, got %d body=%s", putResp.StatusCode, string(b))
	}
	var updated resources.McpServer
	if err := json.NewDecoder(putResp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode put response: %v", err)
	}
	if updated.Metadata.Name != "renamed-mcp" {
		t.Fatalf("expected metadata.name renamed-mcp, got %q", updated.Metadata.Name)
	}
	if updated.Spec.Command != "node" {
		t.Fatalf("expected updated command node, got %q", updated.Spec.Command)
	}

	getOld, err := http.Get(server.URL + "/v1/mcp-servers/orig-mcp")
	if err != nil {
		t.Fatalf("get old mcp-server: %v", err)
	}
	defer getOld.Body.Close()
	if getOld.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(getOld.Body)
		t.Fatalf("expected 404 old name, got %d body=%s", getOld.StatusCode, string(b))
	}

	getNew, err := http.Get(server.URL + "/v1/mcp-servers/renamed-mcp")
	if err != nil {
		t.Fatalf("get renamed mcp-server: %v", err)
	}
	defer getNew.Body.Close()
	if getNew.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(getNew.Body)
		t.Fatalf("expected 200 new name, got %d body=%s", getNew.StatusCode, string(b))
	}
}

func TestMcpServerPutRenameConflict(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/mcp-servers", resources.McpServer{
		APIVersion: "orloj.dev/v1",
		Kind:       "McpServer",
		Metadata:   resources.ObjectMeta{Name: "mcp-a"},
		Spec: resources.McpServerSpec{
			Transport: "stdio",
			Command:   "npx",
		},
	})
	postJSON(t, server.URL+"/v1/mcp-servers", resources.McpServer{
		APIVersion: "orloj.dev/v1",
		Kind:       "McpServer",
		Metadata:   resources.ObjectMeta{Name: "mcp-b"},
		Spec: resources.McpServerSpec{
			Transport: "stdio",
			Command:   "npx",
		},
	})

	getResp, err := http.Get(server.URL + "/v1/mcp-servers/mcp-a")
	if err != nil {
		t.Fatalf("get mcp-a: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(getResp.Body)
		t.Fatalf("expected 200 get mcp-a, got %d body=%s", getResp.StatusCode, string(b))
	}
	var cur resources.McpServer
	if err := json.NewDecoder(getResp.Body).Decode(&cur); err != nil {
		t.Fatalf("decode mcp-a: %v", err)
	}
	cur.Metadata.Name = "mcp-b"

	putBody, err := json.Marshal(cur)
	if err != nil {
		t.Fatalf("marshal put body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/mcp-servers/mcp-a", bytes.NewReader(putBody))
	if err != nil {
		t.Fatalf("new put request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", cur.Metadata.ResourceVersion)
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put rename conflict: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(putResp.Body)
		t.Fatalf("expected 409 conflict, got %d body=%s", putResp.StatusCode, string(b))
	}
}
