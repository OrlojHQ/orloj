package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStreamableHTTPMcpTransportConcurrentSessionAccess(t *testing.T) {
	var nextSession atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var envelope map[string]any
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if sid := nextSession.Add(1); sid > 0 {
			w.Header().Set("Mcp-Session-Id", fmt.Sprintf("sid-%d", sid))
		}
		time.Sleep(2 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")

		method, _ := envelope["method"].(string)
		if method == "notifications/initialized" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		idBytes, err := json.Marshal(envelope["id"])
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var result any
		switch method {
		case "initialize":
			result = McpInitResult{
				ProtocolVersion: "2025-03-26",
				ServerInfo:      McpServerInfo{Name: "test-mcp", Version: "1.0.0"},
				Capabilities:    McpCapabilities{Tools: &McpToolCapability{ListChanged: false}},
			}
		case "tools/list":
			result = mcpToolsListResult{
				Tools: []McpToolDefinition{{Name: "noop"}},
			}
		case "tools/call":
			result = McpToolResult{
				Content: []McpContent{{Type: "text", Text: "ok"}},
			}
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
			return
		}

		response := map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(idBytes),
			"result":  result,
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	endpointURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url failed: %v", err)
	}
	_, port, err := net.SplitHostPort(endpointURL.Host)
	if err != nil {
		t.Fatalf("split host port failed: %v", err)
	}
	endpointURL.Host = net.JoinHostPort("localhost", port)

	transport := NewStreamableHTTPMcpTransport(StreamableHTTPMcpTransportConfig{
		Endpoint:     endpointURL.String(),
		Client:       server.Client(),
		AllowPrivate: true,
	})

	if _, err := transport.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize failed: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 32)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				if idx%2 == 0 {
					if _, err := transport.ListTools(context.Background()); err != nil {
						errCh <- err
						return
					}
					continue
				}
				if _, err := transport.CallTool(context.Background(), "noop", map[string]any{"attempt": j}); err != nil {
					errCh <- err
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent MCP request failed: %v", err)
		}
	}
}
