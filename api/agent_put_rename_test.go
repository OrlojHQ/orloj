package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestAgentPutRenameFromYAMLBody(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/agents", resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "orig-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "hi",
		},
	})

	resp, err := http.Get(server.URL + "/v1/agents/orig-agent")
	if err != nil {
		t.Fatalf("get agent failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
	}
	var cur resources.Agent
	if err := json.NewDecoder(resp.Body).Decode(&cur); err != nil {
		t.Fatalf("decode: %v", err)
	}

	cur.Metadata.Name = "renamed-agent"
	body, err := json.Marshal(cur)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/agents/orig-agent", bytes.NewReader(body))
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
	var updated resources.Agent
	if err := json.NewDecoder(putResp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode put response: %v", err)
	}
	if updated.Metadata.Name != "renamed-agent" {
		t.Fatalf("expected renamed metadata.name %q, got %q", "renamed-agent", updated.Metadata.Name)
	}

	getOld, err := http.Get(server.URL + "/v1/agents/orig-agent")
	if err != nil {
		t.Fatalf("get old: %v", err)
	}
	defer getOld.Body.Close()
	if getOld.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for old name, got %d", getOld.StatusCode)
	}

	getNew, err := http.Get(server.URL + "/v1/agents/renamed-agent")
	if err != nil {
		t.Fatalf("get new: %v", err)
	}
	defer getNew.Body.Close()
	if getNew.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(getNew.Body)
		t.Fatalf("expected 200 for new name, got %d body=%s", getNew.StatusCode, string(b))
	}
}
