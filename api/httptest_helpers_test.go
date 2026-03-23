package api_test

import (
	"io"
	"log"
	"net/http/httptest"
	"testing"

	"github.com/OrlojHQ/orloj/api"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

// newTestServer returns an httptest server with a minimal in-memory API surface
// (shared by namespace, versioning, and edge-case tests).
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	logger := log.New(io.Discard, "", 0)
	runtimeMgr := agentruntime.NewManager(logger)
	server := api.NewServer(api.Stores{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		Tools:        store.NewToolStore(),
		Memories:     store.NewMemoryStore(),
		Policies:     store.NewAgentPolicyStore(),
		Tasks:        store.NewTaskStore(),
		Workers:      store.NewWorkerStore(),
	}, runtimeMgr, logger)
	return httptest.NewServer(server.Handler())
}
