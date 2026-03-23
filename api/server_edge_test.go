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
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/api"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

func TestNamespacesMethodNotAllowed(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/namespaces", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 405 for POST /v1/namespaces, got %d body=%s", resp.StatusCode, string(b))
	}
}

func TestCapabilitiesMethodNotAllowed(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	logger := log.New(io.Discard, "", 0)
	srv := api.NewServer(api.Stores{
		Agents:  store.NewAgentStore(),
		Tasks:   store.NewTaskStore(),
		Workers: store.NewWorkerStore(),
	}, agentruntime.NewManager(logger), logger)

	req := httptest.NewRequest(http.MethodPost, "/v1/capabilities", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST capabilities, got %d", rr.Code)
	}
}

func TestHealthzGetOK(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newTestServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 healthz, got %d body=%s", resp.StatusCode, string(b))
	}
}

// maxBodyOverLimit is one byte larger than api.Server's non-streaming body cap (4 MiB).
const maxBodyOverLimit = 4*1024*1024 + 1

func TestCreateToolRejectsOversizeJSONBody(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newTestServer(t)
	defer server.Close()

	payload := bytes.Repeat([]byte("a"), maxBodyOverLimit)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/tools", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// MaxBytesReader causes ReadAll to fail; handler maps that to 400.
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 for oversized body, got %d body=%s", resp.StatusCode, string(b))
	}
}

func TestAgentsWatchStartsSSE(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newTestServer(t)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/agents/watch", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Client may return error when server closes on context cancel.
		if ctx.Err() != nil {
			return
		}
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q status=%d", ct, resp.StatusCode)
	}
	_, _ = io.CopyN(io.Discard, resp.Body, 64)
}

func TestListAgentsWrongMethod(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPatch, server.URL+"/v1/agents", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 405 for PATCH /v1/agents, got %d body=%s", resp.StatusCode, string(b))
	}
}

func TestCreateToolMalformedJSON(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/tools", bytes.NewReader([]byte(`{"broken":`)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 for malformed JSON, got %d body=%s", resp.StatusCode, string(b))
	}
}

func TestNamespacesIncludesDefaultWhenEmpty(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")

	server := newTestServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/namespaces")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(b))
	}
	var out struct {
		Namespaces []string `json:"namespaces"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ns := range out.Namespaces {
		if ns == "default" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected default namespace in list, got %#v", out.Namespaces)
	}
}
