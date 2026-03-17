package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestLabelSelectorFiltersListEndpoints(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/agent-systems", resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata: resources.ObjectMeta{
			Name: "report-system",
			Labels: map[string]string{
				"orloj.dev/env":     "dev",
				"orloj.dev/usecase": "reporting",
			},
		},
		Spec: resources.AgentSystemSpec{Agents: []string{"research-agent"}},
	})
	postJSON(t, server.URL+"/v1/agent-systems", resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata: resources.ObjectMeta{
			Name: "chat-system",
			Labels: map[string]string{
				"orloj.dev/env": "prod",
			},
		},
		Spec: resources.AgentSystemSpec{Agents: []string{"chat-agent"}},
	})

	resp, err := http.Get(server.URL + "/v1/agent-systems?labelSelector=orloj.dev/env=dev")
	if err != nil {
		t.Fatalf("list with label selector failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
	}
	var list resources.AgentSystemList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list failed: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Metadata.Name != "report-system" {
		t.Fatalf("expected only report-system, got %+v", list.Items)
	}
}

func TestLabelSelectorRejectsInvalidFormat(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/agents?labelSelector=invalid")
	if err != nil {
		t.Fatalf("list with invalid label selector failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, string(body))
	}
}
