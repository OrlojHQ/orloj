package api_test

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/api"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

func TestUIRoutesDefaultRoot(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	respIndex, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get / failed: %v", err)
	}
	defer respIndex.Body.Close()
	body, err := io.ReadAll(respIndex.Body)
	if err != nil {
		t.Fatalf("read / body failed: %v", err)
	}

	switch respIndex.StatusCode {
	case http.StatusOK:
		html := string(body)
		if !strings.Contains(html, "id=\"root\"") {
			t.Fatalf("expected React root element in / body")
		}
		if !strings.Contains(html, "<script") {
			t.Fatalf("expected script tag in / body")
		}
		if !strings.Contains(html, `__ORLOJ_UI_BASE="/"`) {
			t.Fatalf("expected __ORLOJ_UI_BASE injection in / body")
		}
	case http.StatusServiceUnavailable:
		msg := string(body)
		if !strings.Contains(msg, "frontend dist is not built") {
			t.Fatalf("expected build-required message for unbuilt UI, got body=%s", msg)
		}
	default:
		t.Fatalf("expected / status 200 or 503, got %d body=%s", respIndex.StatusCode, string(body))
	}
}

func TestUIRoutesCustomBasePath(t *testing.T) {
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
	}, runtimeMgr, logger, api.ServerOptions{
		UIBasePath: "/console/",
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	respRedirect, err := noRedirectClient.Get(server.URL + "/console")
	if err != nil {
		t.Fatalf("get /console failed: %v", err)
	}
	defer respRedirect.Body.Close()
	if respRedirect.StatusCode != http.StatusTemporaryRedirect {
		body, _ := io.ReadAll(respRedirect.Body)
		t.Fatalf("expected 307 for /console redirect, got %d body=%s", respRedirect.StatusCode, string(body))
	}
	if location := respRedirect.Header.Get("Location"); location != "/console/" {
		t.Fatalf("expected redirect to /console/, got %q", location)
	}

	respIndex, err := http.Get(server.URL + "/console/")
	if err != nil {
		t.Fatalf("get /console/ failed: %v", err)
	}
	defer respIndex.Body.Close()
	body, err := io.ReadAll(respIndex.Body)
	if err != nil {
		t.Fatalf("read /console/ body failed: %v", err)
	}

	switch respIndex.StatusCode {
	case http.StatusOK:
		html := string(body)
		if !strings.Contains(html, "id=\"root\"") {
			t.Fatalf("expected React root element in /console/ body")
		}
		if !strings.Contains(html, `__ORLOJ_UI_BASE="/console/"`) {
			t.Fatalf("expected __ORLOJ_UI_BASE injection in /console/ body")
		}
	case http.StatusServiceUnavailable:
		msg := string(body)
		if !strings.Contains(msg, "frontend dist is not built") {
			t.Fatalf("expected build-required message for unbuilt UI, got body=%s", msg)
		}
	default:
		t.Fatalf("expected /console/ status 200 or 503, got %d body=%s", respIndex.StatusCode, string(body))
	}
}

func TestUISPAFallback(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/tasks/some-task")
	if err != nil {
		t.Fatalf("get /tasks/some-task failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		html := string(body)
		if !strings.Contains(html, "id=\"root\"") {
			t.Fatalf("expected SPA fallback to serve index.html for client-side route")
		}
	case http.StatusServiceUnavailable:
		// dist not built — acceptable in CI
	default:
		t.Fatalf("expected SPA fallback 200 or 503, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestUIUnbuiltModeConsistentAcrossPaths(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	respIndex, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get / failed: %v", err)
	}
	defer respIndex.Body.Close()
	if respIndex.StatusCode != http.StatusServiceUnavailable {
		t.Skip("ui dist is built; skipping unbuilt-mode consistency test")
	}

	respAsset, err := http.Get(server.URL + "/app.js")
	if err != nil {
		t.Fatalf("get /app.js failed: %v", err)
	}
	defer respAsset.Body.Close()
	body, err := io.ReadAll(respAsset.Body)
	if err != nil {
		t.Fatalf("read /app.js body failed: %v", err)
	}
	if respAsset.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for /app.js when dist is unbuilt, got %d body=%s", respAsset.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "frontend dist is not built") {
		t.Fatalf("expected build-required message in /app.js unbuilt response")
	}
}
