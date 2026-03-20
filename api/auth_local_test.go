package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/api"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

func newLocalAuthServer(t *testing.T) *httptest.Server {
	return newLocalAuthServerWithOptions(t, api.ServerOptions{AuthMode: api.AuthModeLocal}, true)
}

func newLocalAuthServerWithOptions(t *testing.T, opts api.ServerOptions, clearTokenEnv bool) *httptest.Server {
	t.Helper()
	if clearTokenEnv {
		t.Setenv("ORLOJ_API_TOKENS", "")
		t.Setenv("ORLOJ_API_TOKEN", "")
	}
	if opts.AuthMode == "" {
		opts.AuthMode = api.AuthModeLocal
	}
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
		LocalAdmins:  store.NewLocalAdminStore(),
		AuthSessions: store.NewAuthSessionStore(),
	}, runtimeMgr, logger, opts)
	return httptest.NewServer(srv.Handler())
}

func TestLocalAuthSetupLoginAndProtectedRoutes(t *testing.T) {
	server := newLocalAuthServer(t)
	defer server.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	resp, err := client.Get(server.URL + "/v1/auth/config")
	if err != nil {
		t.Fatalf("config request failed: %v", err)
	}
	var cfg map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode config failed: %v", err)
	}
	resp.Body.Close()
	if cfg["mode"] != "local" {
		t.Fatalf("expected mode=local, got %v", cfg["mode"])
	}
	if cfg["setup_required"] != true {
		t.Fatalf("expected setup_required=true, got %v", cfg["setup_required"])
	}

	resp, err = client.Get(server.URL + "/v1/tasks")
	if err != nil {
		t.Fatalf("tasks request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401 before setup, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	setupBody := []byte(`{"username":"admin","password":"very-strong-pass"}`)
	resp, err = client.Post(server.URL+"/v1/auth/setup", "application/json", bytes.NewReader(setupBody))
	if err != nil {
		t.Fatalf("setup request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201 for setup, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = client.Post(server.URL+"/v1/auth/setup", "application/json", bytes.NewReader(setupBody))
	if err != nil {
		t.Fatalf("second setup request failed: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 409 for second setup, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = client.Get(server.URL + "/v1/tasks")
	if err != nil {
		t.Fatalf("tasks request with session failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 after setup session, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = client.Post(server.URL+"/v1/auth/logout", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("logout failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 logout, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = client.Get(server.URL + "/v1/tasks")
	if err != nil {
		t.Fatalf("tasks after logout failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401 after logout, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	loginBody := []byte(`{"username":"admin","password":"very-strong-pass"}`)
	resp, err = client.Post(server.URL+"/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 login, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = client.Get(server.URL + "/v1/tasks")
	if err != nil {
		t.Fatalf("tasks after login failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 after login, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
}

func TestLocalAuthUIRouteRemainsAccessible(t *testing.T) {
	server := newLocalAuthServer(t)
	defer server.Close()

	noRedirectClient := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := noRedirectClient.Get(server.URL + "/ui")
	if err != nil {
		t.Fatalf("ui request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected /ui to bypass auth, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestLocalAuthAdminResetPassword(t *testing.T) {
	server := newLocalAuthServer(t)
	defer server.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	_, _ = client.Post(server.URL+"/v1/auth/setup", "application/json", bytes.NewReader([]byte(`{"username":"admin","password":"very-strong-pass"}`)))

	resetBody := []byte(`{"new_password":"another-strong-pass"}`)
	resp, err := client.Post(server.URL+"/v1/auth/admin/reset-password", "application/json", bytes.NewReader(resetBody))
	if err != nil {
		t.Fatalf("reset request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 reset, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = client.Post(server.URL+"/v1/auth/login", "application/json", bytes.NewReader([]byte(`{"username":"admin","password":"very-strong-pass"}`)))
	if err != nil {
		t.Fatalf("old password login request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401 old password, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = client.Post(server.URL+"/v1/auth/login", "application/json", bytes.NewReader([]byte(`{"username":"admin","password":"another-strong-pass"}`)))
	if err != nil {
		t.Fatalf("new password login request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 new password login, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
}

func TestLocalAuthChangePassword(t *testing.T) {
	server := newLocalAuthServer(t)
	defer server.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	_, _ = client.Post(server.URL+"/v1/auth/setup", "application/json", bytes.NewReader([]byte(`{"username":"admin","password":"very-strong-pass"}`)))

	// Reject unauthenticated change attempts even with known current password.
	plainClient := &http.Client{}
	resp, err := plainClient.Post(server.URL+"/v1/auth/change-password", "application/json", bytes.NewReader([]byte(`{"current_password":"very-strong-pass","new_password":"brand-new-strong-pass"}`)))
	if err != nil {
		t.Fatalf("unauthenticated change-password request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401 for unauthenticated change-password, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = client.Post(server.URL+"/v1/auth/change-password", "application/json", bytes.NewReader([]byte(`{"current_password":"wrong-pass","new_password":"brand-new-strong-pass"}`)))
	if err != nil {
		t.Fatalf("wrong current password request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401 for wrong current password, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = client.Post(server.URL+"/v1/auth/change-password", "application/json", bytes.NewReader([]byte(`{"current_password":"very-strong-pass","new_password":"short"}`)))
	if err != nil {
		t.Fatalf("weak password request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 for weak new password, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = client.Post(server.URL+"/v1/auth/change-password", "application/json", bytes.NewReader([]byte(`{"current_password":"very-strong-pass","new_password":"brand-new-strong-pass"}`)))
	if err != nil {
		t.Fatalf("change-password request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 for password change, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = client.Get(server.URL + "/v1/tasks")
	if err != nil {
		t.Fatalf("tasks after password change request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401 after password change session invalidation, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = client.Post(server.URL+"/v1/auth/login", "application/json", bytes.NewReader([]byte(`{"username":"admin","password":"very-strong-pass"}`)))
	if err != nil {
		t.Fatalf("old password login request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401 old password after password change, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = client.Post(server.URL+"/v1/auth/login", "application/json", bytes.NewReader([]byte(`{"username":"admin","password":"brand-new-strong-pass"}`)))
	if err != nil {
		t.Fatalf("new password login request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 with new password after password change, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
}

func TestLocalAuthChangePasswordRejectsNonLocalMode(t *testing.T) {
	server := newLocalAuthServerWithOptions(t, api.ServerOptions{AuthMode: api.AuthModeOff}, true)
	defer server.Close()

	resp, err := http.Post(server.URL+"/v1/auth/change-password", "application/json", bytes.NewReader([]byte(`{"current_password":"x","new_password":"another-very-strong-pass"}`)))
	if err != nil {
		t.Fatalf("change-password request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 for non-local auth mode, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
}

func TestLocalAuthBearerFallbackWhenConfigured(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "automation-token:reader")
	t.Setenv("ORLOJ_API_TOKEN", "")
	server := newLocalAuthServerWithOptions(t, api.ServerOptions{AuthMode: api.AuthModeLocal}, false)
	defer server.Close()

	jar, _ := cookiejar.New(nil)
	setupClient := &http.Client{Jar: jar}
	setupBody := []byte(`{"username":"admin","password":"very-strong-pass"}`)
	resp, err := setupClient.Post(server.URL+"/v1/auth/setup", "application/json", bytes.NewReader(setupBody))
	if err != nil {
		t.Fatalf("setup request failed: %v", err)
	}
	resp.Body.Close()

	plainClient := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/tasks", nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	resp, err = plainClient.Do(req)
	if err != nil {
		t.Fatalf("tasks request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401 without session/bearer, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	req, err = http.NewRequest(http.MethodGet, server.URL+"/v1/tasks", nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer automation-token")
	resp, err = plainClient.Do(req)
	if err != nil {
		t.Fatalf("tasks request with bearer failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 with bearer, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
}

func TestLocalAuthSessionExpiryEnforced(t *testing.T) {
	server := newLocalAuthServerWithOptions(t, api.ServerOptions{
		AuthMode:   api.AuthModeLocal,
		SessionTTL: 20 * time.Millisecond,
	}, true)
	defer server.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	setupBody := []byte(`{"username":"admin","password":"very-strong-pass"}`)
	resp, err := client.Post(server.URL+"/v1/auth/setup", "application/json", bytes.NewReader(setupBody))
	if err != nil {
		t.Fatalf("setup request failed: %v", err)
	}
	resp.Body.Close()

	time.Sleep(80 * time.Millisecond)
	resp, err = client.Get(server.URL + "/v1/tasks")
	if err != nil {
		t.Fatalf("tasks request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401 after session expiry, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
}
