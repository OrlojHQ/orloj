package api_test

import (
	"bufio"
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
	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

func newSessionAPIServer(t *testing.T) (*httptest.Server, *store.SessionStore) {
	t.Helper()
	systems := store.NewAgentSystemStore()
	if _, err := systems.Upsert(context.Background(), resources.AgentSystem{
		Metadata: resources.ObjectMeta{Name: "support"},
		Spec:     resources.AgentSystemSpec{Agents: []string{"assistant"}},
	}); err != nil {
		t.Fatal(err)
	}
	sessions := store.NewSessionStore()
	server := api.NewServer(api.Stores{
		AgentSystems: systems,
		Sessions:     sessions,
		Tasks:        store.NewTaskStore(),
	}, agentruntime.NewManager(log.New(io.Discard, "", 0)), log.New(io.Discard, "", 0))
	return httptest.NewServer(server.Handler()), sessions
}

func sessionRequest(t *testing.T, method, url string, body any, headers map[string]string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestSessionAPIEnqueuesIdempotentTurnsAndListsEvents(t *testing.T) {
	server, _ := newSessionAPIServer(t)
	defer server.Close()

	resp := sessionRequest(t, http.MethodPost, server.URL+"/v1/sessions", map[string]any{
		"apiVersion": "orloj.dev/v1",
		"kind":       "Session",
		"metadata":   map[string]any{"name": "chat"},
		"spec":       map[string]any{"system": "support"},
	}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create status=%d body=%s", resp.StatusCode, body)
	}

	headers := map[string]string{"Idempotency-Key": "message-1"}
	resp = sessionRequest(t, http.MethodPost, server.URL+"/v1/sessions/chat/turns", map[string]any{"content": "hello"}, headers)
	var first map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("turn status=%d response=%#v", resp.StatusCode, first)
	}

	resp = sessionRequest(t, http.MethodPost, server.URL+"/v1/sessions/chat/turns", map[string]any{"content": "ignored duplicate"}, headers)
	var duplicate map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&duplicate); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || duplicate["created"] != false {
		t.Fatalf("duplicate status=%d response=%#v", resp.StatusCode, duplicate)
	}

	resp = sessionRequest(t, http.MethodGet, server.URL+"/v1/sessions/chat/events?after=1", nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events status=%d", resp.StatusCode)
	}
	var events struct {
		Items []resources.SessionEvent `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	if len(events.Items) != 2 {
		t.Fatalf("events=%d, want turn.queued and message.created", len(events.Items))
	}
	if events.Items[0].Sequence != 2 || events.Items[1].Sequence != 3 {
		t.Fatalf("event sequences=%d,%d", events.Items[0].Sequence, events.Items[1].Sequence)
	}
}

func TestSessionStreamResumesFromLastEventID(t *testing.T) {
	server, _ := newSessionAPIServer(t)
	defer server.Close()
	resp := sessionRequest(t, http.MethodPost, server.URL+"/v1/sessions", map[string]any{
		"metadata": map[string]any{"name": "stream-chat"},
		"spec":     map[string]any{"system": "support"},
	}, nil)
	resp.Body.Close()
	resp = sessionRequest(t, http.MethodPost, server.URL+"/v1/sessions/stream-chat/turns", map[string]any{"content": "hello"}, map[string]string{
		"Idempotency-Key": "stream-1",
	})
	resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/sessions/stream-chat/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Last-Event-ID", "1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content type=%q", ct)
	}
	scanner := bufio.NewScanner(resp.Body)
	foundID := false
	foundEvent := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "id: 2" {
			foundID = true
		}
		if line == "event: turn.queued" {
			foundEvent = true
		}
		if foundID && foundEvent {
			break
		}
	}
	if !foundID || !foundEvent {
		t.Fatalf("resume stream found id=%v event=%v", foundID, foundEvent)
	}
}

func TestSessionPauseAndResumeActions(t *testing.T) {
	server, _ := newSessionAPIServer(t)
	defer server.Close()
	resp := sessionRequest(t, http.MethodPost, server.URL+"/v1/sessions", map[string]any{
		"metadata": map[string]any{"name": "controlled"},
		"spec":     map[string]any{"system": "support"},
	}, nil)
	resp.Body.Close()

	resp = sessionRequest(t, http.MethodPost, server.URL+"/v1/sessions/controlled/pause", map[string]any{"reason": "review"}, nil)
	var paused resources.Session
	if err := json.NewDecoder(resp.Body).Decode(&paused); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || paused.Status.Phase != resources.SessionPhasePaused {
		t.Fatalf("pause status=%d session=%#v", resp.StatusCode, paused.Status)
	}

	resp = sessionRequest(t, http.MethodPost, server.URL+"/v1/sessions/controlled/resume", nil, nil)
	var resumed resources.Session
	if err := json.NewDecoder(resp.Body).Decode(&resumed); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resumed.Status.Phase != resources.SessionPhaseWaitingInput {
		t.Fatalf("resume status=%d session=%#v", resp.StatusCode, resumed.Status)
	}
}
