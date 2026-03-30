package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/OrlojHQ/orloj/api"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

type captureAuditSink struct {
	mu     sync.Mutex
	events []agentruntime.AuditEvent
}

func (c *captureAuditSink) RecordAudit(_ context.Context, event agentruntime.AuditEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *captureAuditSink) snapshot() []agentruntime.AuditEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]agentruntime.AuditEvent, len(c.events))
	copy(out, c.events)
	return out
}

func newBearerAuthServer(t *testing.T, ext agentruntime.Extensions) *httptest.Server {
	t.Helper()
	logger := log.New(io.Discard, "", 0)
	runtimeMgr := agentruntime.NewManager(logger)
	srv := api.NewServerWithOptions(api.Stores{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		Tools:        store.NewToolStore(),
		Memories:     store.NewMemoryStore(),
		Policies:     store.NewAgentPolicyStore(),
		Tasks:        store.NewTaskStore(),
		Workers:      store.NewWorkerStore(),
		APITokens:    store.NewAPITokenStore(),
	}, runtimeMgr, logger, api.ServerOptions{Extensions: ext})
	return httptest.NewServer(srv.Handler())
}

func TestTokenCRUDStoreOnly(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "bootstrap:env-admin-token:admin")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newBearerAuthServer(t, agentruntime.DefaultExtensions())
	defer server.Close()

	createBody := []byte(`{"name":"ci-writer","role":"writer"}`)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/tokens", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("build create request failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer env-admin-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create token request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 201 create token, got %d body=%s", resp.StatusCode, string(body))
	}
	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		resp.Body.Close()
		t.Fatalf("decode create token response failed: %v", err)
	}
	resp.Body.Close()
	token, _ := created["token"].(string)
	if strings.TrimSpace(token) == "" {
		t.Fatal("expected token secret in create response")
	}

	req, err = http.NewRequest(http.MethodGet, server.URL+"/v1/tokens", nil)
	if err != nil {
		t.Fatalf("build list request failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer env-admin-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list tokens request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 list tokens, got %d body=%s", resp.StatusCode, string(body))
	}
	var listed struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		resp.Body.Close()
		t.Fatalf("decode list response failed: %v", err)
	}
	resp.Body.Close()
	if len(listed.Items) != 1 {
		t.Fatalf("expected exactly 1 store-managed token, got %d", len(listed.Items))
	}
	if _, hasTokenField := listed.Items[0]["token"]; hasTokenField {
		t.Fatal("list response must not include token secret")
	}
	if listed.Items[0]["name"] != "ci-writer" {
		t.Fatalf("expected listed token ci-writer, got %v", listed.Items[0]["name"])
	}

	req, err = http.NewRequest(http.MethodDelete, server.URL+"/v1/tokens/ci-writer", nil)
	if err != nil {
		t.Fatalf("build delete request failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer env-admin-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete token request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 delete token, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	req, err = http.NewRequest(http.MethodGet, server.URL+"/v1/tokens", nil)
	if err != nil {
		t.Fatalf("build list request failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer env-admin-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list tokens request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 list tokens, got %d body=%s", resp.StatusCode, string(body))
	}
	var listedAfterDelete struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listedAfterDelete); err != nil {
		resp.Body.Close()
		t.Fatalf("decode list-after-delete response failed: %v", err)
	}
	resp.Body.Close()
	if len(listedAfterDelete.Items) != 0 {
		t.Fatalf("expected no store-managed tokens after delete, got %d", len(listedAfterDelete.Items))
	}
}

func TestAuthMeBearerIdentity(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "ci-bot:whoami-token:writer")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newBearerAuthServer(t, agentruntime.DefaultExtensions())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/auth/me", nil)
	if err != nil {
		t.Fatalf("build whoami request failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer whoami-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("whoami request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 for whoami, got %d body=%s", resp.StatusCode, string(body))
	}
	var me struct {
		Authenticated bool   `json:"authenticated"`
		Name          string `json:"name"`
		Role          string `json:"role"`
		Method        string `json:"method"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		resp.Body.Close()
		t.Fatalf("decode whoami response failed: %v", err)
	}
	resp.Body.Close()
	if !me.Authenticated {
		t.Fatal("expected authenticated=true")
	}
	if me.Name != "ci-bot" || me.Role != "writer" || me.Method != "bearer" {
		t.Fatalf("unexpected whoami response: %+v", me)
	}
}

func TestBearerAuditPrincipalNamedAndLegacy(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "named-token:token-named:reader,token-legacy:reader")
	t.Setenv("ORLOJ_API_TOKEN", "")
	audit := &captureAuditSink{}

	server := newBearerAuthServer(t, agentruntime.Extensions{Audit: audit})
	defer server.Close()

	reqNamed, err := http.NewRequest(http.MethodGet, server.URL+"/v1/tasks", nil)
	if err != nil {
		t.Fatalf("build named token request failed: %v", err)
	}
	reqNamed.Header.Set("Authorization", "Bearer token-named")
	resp, err := http.DefaultClient.Do(reqNamed)
	if err != nil {
		t.Fatalf("named token request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for named token request, got %d", resp.StatusCode)
	}

	reqLegacy, err := http.NewRequest(http.MethodGet, server.URL+"/v1/tasks", nil)
	if err != nil {
		t.Fatalf("build legacy token request failed: %v", err)
	}
	reqLegacy.Header.Set("Authorization", "Bearer token-legacy")
	resp, err = http.DefaultClient.Do(reqLegacy)
	if err != nil {
		t.Fatalf("legacy token request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for legacy token request, got %d", resp.StatusCode)
	}

	events := audit.snapshot()
	if len(events) == 0 {
		t.Fatal("expected audit events")
	}
	var sawNamed bool
	var sawLegacy bool
	for _, event := range events {
		if event.Action != "api.request" {
			continue
		}
		if event.Principal == "named-token" {
			sawNamed = true
		}
		if event.Principal == "bearer:reader" {
			sawLegacy = true
		}
	}
	if !sawNamed {
		t.Fatalf("expected audit principal for named token, events=%+v", events)
	}
	if !sawLegacy {
		t.Fatalf("expected audit principal bearer:reader for legacy token, events=%+v", events)
	}
}
