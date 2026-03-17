package api_test

import (
	"bufio"
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
	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/eventbus"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

func TestAPIEmitsResourceEventsToBus(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	server := api.NewServer(api.Stores{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		Tools:        store.NewToolStore(),
		Memories:     store.NewMemoryStore(),
		Policies:     store.NewAgentPolicyStore(),
		Tasks:        store.NewTaskStore(),
		Workers:      store.NewWorkerStore(),
	}, agentruntime.NewManager(logger), logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := server.EventBus().Subscribe(ctx, eventbus.Filter{Source: "apiserver", Kind: "Tool", Name: "web-search"})

	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	postJSON(t, httpServer.URL+"/v1/tools", resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "web-search"},
		Spec:       resources.ToolSpec{Type: "http", Endpoint: "https://example"},
	})

	select {
	case evt := <-ch:
		if evt.Type != "resource.created" {
			t.Fatalf("expected resource.created, got %q", evt.Type)
		}
		if evt.Kind != "Tool" || evt.Name != "web-search" {
			t.Fatalf("unexpected event target kind=%q name=%q", evt.Kind, evt.Name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for API event")
	}
}

func TestEventsWatchStreamReceivesPublishedEvents(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	server := api.NewServer(api.Stores{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		Tools:        store.NewToolStore(),
		Memories:     store.NewMemoryStore(),
		Policies:     store.NewAgentPolicyStore(),
		Tasks:        store.NewTaskStore(),
		Workers:      store.NewWorkerStore(),
	}, agentruntime.NewManager(logger), logger)

	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	req, err := http.NewRequest(http.MethodGet, httpServer.URL+"/v1/events/watch?source=apiserver&kind=Task", nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("events watch request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("events watch failed status=%d body=%s", resp.StatusCode, string(b))
	}

	postJSON(t, httpServer.URL+"/v1/tasks", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "stream-task"},
		Spec:       resources.TaskSpec{System: "sys"},
	})

	scanner := bufio.NewScanner(resp.Body)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for stream data")
		default:
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				t.Fatalf("scanner error: %v", err)
			}
			t.Fatal("event stream closed before event arrived")
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		var evt map[string]any
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			continue
		}
		if strings.EqualFold(asString(evt["kind"]), "Task") && strings.EqualFold(asString(evt["name"]), "stream-task") {
			if !strings.EqualFold(asString(evt["type"]), "resource.created") {
				t.Fatalf("expected resource.created event type, got %q", asString(evt["type"]))
			}
			return
		}
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
