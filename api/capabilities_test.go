package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OrlojHQ/orloj/api"
	"github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

type fixedCapabilityProvider struct {
	snapshot agentruntime.CapabilitySnapshot
}

func (p fixedCapabilityProvider) Capabilities(_ context.Context) agentruntime.CapabilitySnapshot {
	return p.snapshot
}

func TestCapabilitiesEndpointDefaultSnapshot(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	logger := log.New(io.Discard, "", 0)
	server := api.NewServer(api.Stores{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		Tasks:        store.NewTaskStore(),
		Workers:      store.NewWorkerStore(),
	}, agentruntime.NewManager(logger), logger)
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		body, _ := io.ReadAll(rr.Body)
		t.Fatalf("expected 200, got %d body=%s", rr.Code, string(body))
	}

	var payload agentruntime.CapabilitySnapshot
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode capabilities failed: %v", err)
	}
	if payload.GeneratedAt == "" {
		t.Fatal("expected generated_at to be set")
	}
	if len(payload.Capabilities) == 0 {
		t.Fatal("expected at least one capability")
	}
	foundCoreCRUD := false
	for _, item := range payload.Capabilities {
		if item.ID == "core.api.crud" && item.Enabled {
			foundCoreCRUD = true
			break
		}
	}
	if !foundCoreCRUD {
		t.Fatalf("expected core.api.crud capability in %+v", payload.Capabilities)
	}
}

func TestCapabilitiesEndpointSupportsCustomProvider(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	logger := log.New(io.Discard, "", 0)
	server := api.NewServerWithOptions(api.Stores{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		Tasks:        store.NewTaskStore(),
		Workers:      store.NewWorkerStore(),
	}, agentruntime.NewManager(logger), logger, api.ServerOptions{
		Extensions: agentruntime.Extensions{
			Capabilities: fixedCapabilityProvider{
				snapshot: agentruntime.CapabilitySnapshot{
					GeneratedAt: "2026-03-14T00:00:00Z",
					Capabilities: []agentruntime.Capability{
						{ID: "enterprise.sso", Enabled: true, Source: "extension"},
					},
				},
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		body, _ := io.ReadAll(rr.Body)
		t.Fatalf("expected 200, got %d body=%s", rr.Code, string(body))
	}

	var payload agentruntime.CapabilitySnapshot
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode capabilities failed: %v", err)
	}
	if payload.GeneratedAt != "2026-03-14T00:00:00Z" {
		t.Fatalf("expected fixed generated_at, got %q", payload.GeneratedAt)
	}
	if len(payload.Capabilities) != 1 || payload.Capabilities[0].ID != "enterprise.sso" {
		t.Fatalf("unexpected custom capabilities payload: %+v", payload.Capabilities)
	}
}
