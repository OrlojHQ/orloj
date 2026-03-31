package api_test

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
)

func TestTokenCreateInvalidJSON(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "admin-bot:admin-tok:admin")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newBearerAuthServer(t, agentruntime.DefaultExtensions())
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/tokens", bytes.NewReader([]byte(`not json`)))
	req.Header.Set("Authorization", "Bearer admin-tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestTokenCreateEmptyName(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "admin-bot:admin-tok:admin")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newBearerAuthServer(t, agentruntime.DefaultExtensions())
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/tokens", bytes.NewReader([]byte(`{"name":"","role":"writer"}`)))
	req.Header.Set("Authorization", "Bearer admin-tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty name, got %d", resp.StatusCode)
	}
}

func TestTokenCreateInvalidRole(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "admin-bot:admin-tok:admin")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newBearerAuthServer(t, agentruntime.DefaultExtensions())
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/tokens", bytes.NewReader([]byte(`{"name":"bad-role","role":"superadmin"}`)))
	req.Header.Set("Authorization", "Bearer admin-tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid role, got %d", resp.StatusCode)
	}
}

func TestTokenCreateDuplicateName(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "admin-bot:admin-tok:admin")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newBearerAuthServer(t, agentruntime.DefaultExtensions())
	defer server.Close()

	body := []byte(`{"name":"dup","role":"writer"}`)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/tokens", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin-tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 on first create, got %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodPost, server.URL+"/v1/tokens", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin-tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("second create failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate name, got %d", resp.StatusCode)
	}
}

func TestTokenDeleteNotFound(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "admin-bot:admin-tok:admin")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newBearerAuthServer(t, agentruntime.DefaultExtensions())
	defer server.Close()

	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/v1/tokens/no-such-token", nil)
	req.Header.Set("Authorization", "Bearer admin-tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing token, got %d", resp.StatusCode)
	}
}

func TestTokenByNameEncodedSlashRejected(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "admin-bot:admin-tok:admin")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newBearerAuthServer(t, agentruntime.DefaultExtensions())
	defer server.Close()

	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/v1/tokens/foo%2Fbar", nil)
	req.Header.Set("Authorization", "Bearer admin-tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 for encoded slash in token name, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestTokenByNameEmptyAfterUnescape(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "admin-bot:admin-tok:admin")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newBearerAuthServer(t, agentruntime.DefaultExtensions())
	defer server.Close()

	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/v1/tokens/%20%20", nil)
	req.Header.Set("Authorization", "Bearer admin-tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for whitespace-only token name, got %d", resp.StatusCode)
	}
}

func TestTokenMethodNotAllowed(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "admin-bot:admin-tok:admin")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newBearerAuthServer(t, agentruntime.DefaultExtensions())
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPut, server.URL+"/v1/tokens", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer admin-tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for PUT on /v1/tokens, got %d", resp.StatusCode)
	}
}

func TestTokenAuditCapturesPrincipal(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "audit-admin:audit-tok:admin")
	t.Setenv("ORLOJ_API_TOKEN", "")

	audit := &captureAuditSink{}
	server := newBearerAuthServer(t, agentruntime.Extensions{Audit: audit})
	defer server.Close()

	body := []byte(`{"name":"audited","role":"reader"}`)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/tokens", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer audit-tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	events := audit.snapshot()
	var found bool
	for _, ev := range events {
		if ev.Action == "token.create" && ev.Principal == "audit-admin" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected token.create audit event with principal audit-admin, events=%+v", events)
	}
}
