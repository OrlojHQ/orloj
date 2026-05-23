package a2a

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchCard_Success(t *testing.T) {
	card := AgentCard{
		Name:            "remote-agent",
		URL:             "https://remote.example.com/a2a",
		ProtocolVersion: "1.0",
		Capabilities:    CardCapabilities{Streaming: true},
		Skills: []CardSkill{{
			ID:   "summarize",
			Name: "summarize",
		}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent-card.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(card)
	}))
	defer srv.Close()

	client := newTestClient(func(c *Client) { c.cardCacheTTL = 1 * time.Minute })
	got, err := client.FetchCard(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("FetchCard failed: %v", err)
	}
	if got.Name != "remote-agent" {
		t.Errorf("expected name remote-agent, got %s", got.Name)
	}
	if !got.Capabilities.Streaming {
		t.Error("expected streaming=true")
	}
	if len(got.Skills) != 1 || got.Skills[0].ID != "summarize" {
		t.Errorf("unexpected skills: %+v", got.Skills)
	}
}

func TestFetchCard_CachesResult(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(AgentCard{Name: "cached"})
	}))
	defer srv.Close()

	client := newTestClient(func(c *Client) { c.cardCacheTTL = 10 * time.Minute })
	_, err := client.FetchCard(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}
	_, err = client.FetchCard(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 HTTP call (cached), got %d", calls)
	}
}

func TestFetchCard_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(func(c *Client) { c.cardCacheTTL = 1 * time.Minute })
	_, err := client.FetchCard(context.Background(), srv.URL, nil)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}

	ts, hasErr := client.CacheStatus(srv.URL)
	if ts.IsZero() {
		t.Error("expected cache entry even for error")
	}
	if !hasErr {
		t.Error("expected error flag in cache")
	}
}

func TestFetchCard_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := newTestClient(func(c *Client) { c.cardCacheTTL = 1 * time.Minute })
	_, err := client.FetchCard(context.Background(), srv.URL, nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFetchCard_SSRFBlocksPrivateByDefault(t *testing.T) {
	client := NewClient(ClientConfig{AllowPrivate: false, CardCacheTTL: 1 * time.Minute})
	_, err := client.FetchCard(context.Background(), "http://192.168.1.1/agent", nil)
	if err == nil {
		t.Fatal("expected SSRF error for private IP")
	}
}

func TestSendTask_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: TaskResult{
				ID:     "task-1",
				Status: TaskStatus{State: TaskStateCompleted},
				Artifacts: []TaskArtifact{{
					Name:  "output",
					Parts: []TaskPart{{Type: "text", Text: "Summary result"}},
					Index: 0,
				}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := newTestClient()
	result, err := client.SendTask(context.Background(), srv.URL, TaskSendParams{
		ID: "task-1",
		Message: TaskMessage{
			Role:  "user",
			Parts: []TaskPart{{Type: "text", Text: "Summarize this"}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("SendTask failed: %v", err)
	}
	if result.ID != "task-1" {
		t.Errorf("expected task ID task-1, got %s", result.ID)
	}
	if result.Status.State != TaskStateCompleted {
		t.Errorf("expected completed, got %s", result.Status.State)
	}
}

func TestSendTask_RemoteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Error: &JSONRPCError{
				Code:    ErrCodeInvalidParams,
				Message: "missing required field",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := newTestClient()
	_, err := client.SendTask(context.Background(), srv.URL, TaskSendParams{ID: "bad"}, nil)
	if err == nil {
		t.Fatal("expected error from remote")
	}
}

func TestGetTask_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result: TaskResult{
				ID:     "task-2",
				Status: TaskStatus{State: TaskStateWorking},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := newTestClient()
	result, err := client.GetTask(context.Background(), srv.URL, TaskGetParams{ID: "task-2"}, nil)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if result.Status.State != TaskStateWorking {
		t.Errorf("expected working, got %s", result.Status.State)
	}
}

func TestCancelTask_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result: TaskResult{
				ID:     "task-3",
				Status: TaskStatus{State: TaskStateCanceled},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := newTestClient()
	result, err := client.CancelTask(context.Background(), srv.URL, TaskCancelParams{
		ID:     "task-3",
		Reason: "no longer needed",
	}, nil)
	if err != nil {
		t.Fatalf("CancelTask failed: %v", err)
	}
	if result.Status.State != TaskStateCanceled {
		t.Errorf("expected canceled, got %s", result.Status.State)
	}
}

func TestFetchCard_CacheTTLExpiry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(AgentCard{Name: "ttl-agent"})
	}))
	defer srv.Close()

	client := newTestClient(func(c *Client) { c.cardCacheTTL = 100 * time.Millisecond })

	_, err := client.FetchCard(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 HTTP call after first fetch, got %d", calls)
	}

	_, err = client.FetchCard(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("second fetch (cached) failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 HTTP call after cached fetch, got %d", calls)
	}

	time.Sleep(150 * time.Millisecond)

	_, err = client.FetchCard(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("third fetch (after TTL) failed: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 HTTP calls after TTL expiry, got %d", calls)
	}
}

func TestSendTask_HTTP502Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}))
	defer srv.Close()

	client := newTestClient()
	_, err := client.SendTask(context.Background(), srv.URL, TaskSendParams{
		ID: "task-502",
		Message: TaskMessage{
			Role:  "user",
			Parts: []TaskPart{{Type: "text", Text: "test"}},
		},
	}, nil)
	if err == nil {
		t.Fatal("expected error for HTTP 502 response")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("expected error to contain '502', got: %v", err)
	}
}
