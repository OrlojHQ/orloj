package api_test

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OrlojHQ/orloj/api"
	"github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

type denyAllAuthorizer struct{}

func (denyAllAuthorizer) Authorize(_ *http.Request, _ string) (bool, int, string) {
	return false, http.StatusForbidden, "blocked by injected authorizer"
}

func TestServerUsesInjectedAuthorizer(t *testing.T) {
	t.Setenv("ORLOJ_API_TOKENS", "reader-token:reader")
	t.Setenv("ORLOJ_API_TOKEN", "")

	logger := log.New(io.Discard, "", 0)
	server := api.NewServerWithOptions(api.Stores{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		Tasks:        store.NewTaskStore(),
		Workers:      store.NewWorkerStore(),
	}, agentruntime.NewManager(logger), logger, api.ServerOptions{
		Authorizer: denyAllAuthorizer{},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/tasks", nil)
	req.Header.Set("Authorization", "Bearer reader-token")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected injected authorizer to deny request with 403, got %d body=%s", rr.Code, rr.Body.String())
	}
}
