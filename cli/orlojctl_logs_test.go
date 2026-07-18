package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLogsEndpoint(t *testing.T) {
	name, endpoint, err := logsEndpoint("http://127.0.0.1:8080/", "task/weekly-report")
	if err != nil {
		t.Fatalf("logsEndpoint returned error: %v", err)
	}
	if name != "weekly-report" {
		t.Fatalf("unexpected name: %q", name)
	}
	if endpoint != "http://127.0.0.1:8080/v1/tasks/weekly-report/logs" {
		t.Fatalf("unexpected task endpoint: %q", endpoint)
	}

	name, endpoint, err = logsEndpoint("http://127.0.0.1:8080", "my-agent")
	if err != nil {
		t.Fatalf("logsEndpoint returned error: %v", err)
	}
	if name != "my-agent" {
		t.Fatalf("unexpected name: %q", name)
	}
	if endpoint != "http://127.0.0.1:8080/v1/agents/my-agent/logs" {
		t.Fatalf("unexpected agent endpoint: %q", endpoint)
	}

	if _, _, err := logsEndpoint("http://127.0.0.1:8080", "task/"); err == nil {
		t.Fatal("expected error for empty task name")
	}
}

func TestFetchLogs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "weekly-report",
			"logs": []string{"line1", "line2"},
		})
	}))
	defer ts.Close()

	logs, err := fetchLogs(context.Background(), ts.URL+"/v1/tasks/weekly-report/logs")
	if err != nil {
		t.Fatalf("fetchLogs returned error: %v", err)
	}
	if len(logs) != 2 || logs[0] != "line1" || logs[1] != "line2" {
		t.Fatalf("unexpected logs: %v", logs)
	}
}

func TestFetchLogsErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "task not found", http.StatusNotFound)
	}))
	defer ts.Close()

	if _, err := fetchLogs(context.Background(), ts.URL+"/v1/tasks/missing/logs"); err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

// TestStreamLogsFollowsNewLines verifies --follow prints only newly-appeared
// lines on each poll, and stops cleanly once the context is cancelled
// (mirroring what happens on Ctrl+C via signal.NotifyContext).
func TestStreamLogsFollowsNewLines(t *testing.T) {
	var calls int32
	responses := [][]string{
		{"tick 1"},
		{"tick 1", "tick 2"},
		{"tick 1", "tick 2", "tick 3"},
	}
	fetch := func(ctx context.Context) ([]string, error) {
		n := atomic.AddInt32(&calls, 1) - 1
		if int(n) >= len(responses) {
			n = int32(len(responses) - 1)
		}
		return responses[n], nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- streamLogs(ctx, &out, fetch, time.Millisecond)
	}()

	deadline := time.After(2 * time.Second)
	for {
		if atomic.LoadInt32(&calls) >= int32(len(responses)) {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for streamLogs to poll enough times")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("streamLogs returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("streamLogs did not return after context cancellation")
	}

	printed := strings.TrimSpace(out.String())
	lines := strings.Split(printed, "\n")
	want := []string{"tick 1", "tick 2", "tick 3"}
	if len(lines) != len(want) {
		t.Fatalf("expected %d printed lines, got %d: %v", len(want), len(lines), lines)
	}
	for i, line := range want {
		if lines[i] != line {
			t.Fatalf("expected line %d to be %q, got %q", i, line, lines[i])
		}
	}
}

// TestStreamLogsPropagatesFetchError ensures a real fetch failure (not a
// context cancellation) is still surfaced to the caller.
func TestStreamLogsPropagatesFetchError(t *testing.T) {
	fetchErr := errors.New("boom")
	fetch := func(ctx context.Context) ([]string, error) {
		return nil, fetchErr
	}
	var out bytes.Buffer
	err := streamLogs(context.Background(), &out, fetch, time.Millisecond)
	if !errors.Is(err, fetchErr) {
		t.Fatalf("expected fetch error to propagate, got %v", err)
	}
}
