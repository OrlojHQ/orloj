package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func newChatCompletionsTestServer(t *testing.T) (*httptest.Server, api.Stores, *api.Server) {
	t.Helper()
	t.Setenv("ORLOJ_API_TOKENS", "")
	t.Setenv("ORLOJ_API_TOKEN", "")
	logger := log.New(io.Discard, "", 0)
	stores := api.Stores{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		Tools:        store.NewToolStore(),
		Tasks:        store.NewTaskStore(),
		Sessions:     store.NewSessionStore(),
		Workers:      store.NewWorkerStore(),
		Memories:     store.NewMemoryStore(),
		Policies:     store.NewAgentPolicyStore(),
	}
	server := api.NewServer(stores, agentruntime.NewManager(logger), logger)
	return httptest.NewServer(server.Handler()), stores, server
}

func seedChatCompletionSystem(t *testing.T, stores api.Stores, name string) {
	t.Helper()
	system := resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata: resources.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"demo-agent"},
		},
	}
	if _, err := stores.AgentSystems.Upsert(context.Background(), system); err != nil {
		t.Fatalf("seed AgentSystem: %v", err)
	}
}

func completeLatestChatTask(t *testing.T, server *api.Server, stores api.Stores, phase string, output map[string]string, lastError string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		sessions, err := stores.Sessions.List(context.Background())
		if err != nil {
			t.Fatalf("list sessions: %v", err)
		}
		var target *resources.Session
		for i := range sessions {
			session := sessions[i]
			if session.Metadata.Labels["orloj.dev/created-by"] == "chat-completions" {
				target = &session
				break
			}
		}
		if target != nil {
			claim, ok, _, err := stores.Sessions.ClaimNextTurn(context.Background(), "test-worker", time.Minute)
			if err != nil || !ok {
				t.Fatalf("claim session turn ok=%v err=%v", ok, err)
			}
			switch phase {
			case "Succeeded":
				content := output["last_output"]
				if content == "" {
					best := ""
					for _, key := range []string{"agent.1.message_content", "agent.2.message_content", "response", "result"} {
						if value := output[key]; value != "" && value != "executed" {
							best = value
						}
					}
					content = best
				}
				if rawDeltas := output["_stream_deltas"]; rawDeltas != "" {
					for _, delta := range strings.Split(rawDeltas, "|") {
						evt, appendErr := stores.Sessions.AppendEvent(
							context.Background(),
							store.ScopedName(claim.Session.Metadata.Namespace, claim.Session.Metadata.Name),
							claim.Turn.ID,
							claim.Turn.ClaimedBy,
							claim.Turn.Fence,
							resources.SessionEvent{
								Type:      resources.SessionEventMessageDelta,
								MessageID: claim.Turn.AssistantMessageID,
								Payload:   map[string]any{"role": "assistant", "delta": delta},
							},
						)
						if appendErr != nil {
							t.Fatalf("append session delta: %v", appendErr)
						}
						server.PublishResourceEventForTest("Session", claim.Session.Metadata.Name, evt.Type, evt)
					}
				}
				events, updated, err := stores.Sessions.CompleteTurn(context.Background(), claim, content, nil)
				if err != nil {
					t.Fatalf("complete session turn: %v", err)
				}
				for _, evt := range events {
					server.PublishResourceEventForTest("Session", updated.Metadata.Name, evt.Type, evt)
				}
			case "WaitingApproval":
				events, updated, err := stores.Sessions.SetApprovalState(
					context.Background(),
					claim,
					true,
					&resources.TaskBlockedOn{Kind: "Approval", Name: "test", Reason: "session requires approval"},
				)
				if err != nil {
					t.Fatalf("mark session waiting for approval: %v", err)
				}
				for _, evt := range events {
					server.PublishResourceEventForTest("Session", updated.Metadata.Name, evt.Type, evt)
				}
			default:
				if lastError == "" {
					lastError = "session failed"
				}
				events, updated, err := stores.Sessions.FailTurn(context.Background(), claim, fmt.Errorf("%s", lastError))
				if err != nil {
					t.Fatalf("fail session turn: %v", err)
				}
				for _, evt := range events {
					server.PublishResourceEventForTest("Session", updated.Metadata.Name, evt.Type, evt)
				}
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for chat-completions session")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestChatCompletionsMissingModel(t *testing.T) {
	ts, _, _ := newChatCompletionsTestServer(t)
	defer ts.Close()

	resp := postChatCompletions(t, ts.URL, map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "hello"}},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestChatCompletionsUnknownSystem(t *testing.T) {
	ts, _, _ := newChatCompletionsTestServer(t)
	defer ts.Close()

	resp := postChatCompletions(t, ts.URL, map[string]any{
		"model":    "missing-system",
		"messages": []map[string]string{{"role": "user", "content": "hello"}},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestChatCompletionsRejectsNonStringContent(t *testing.T) {
	ts, stores, _ := newChatCompletionsTestServer(t)
	defer ts.Close()
	seedChatCompletionSystem(t, stores, "demo-system")

	resp := postChatCompletions(t, ts.URL, map[string]any{
		"model": "demo-system",
		"messages": []map[string]any{
			{"role": "user", "content": []map[string]string{{"type": "text", "text": "hi"}}},
		},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestChatCompletionsSuccessUsesLastOutput(t *testing.T) {
	ts, stores, server := newChatCompletionsTestServer(t)
	defer ts.Close()
	seedChatCompletionSystem(t, stores, "demo-system")

	errCh := make(chan error, 1)
	var resp *http.Response
	go func() {
		var err error
		resp, err = postChatCompletionsAsync(ts.URL, map[string]any{
			"model": "demo-system",
			"messages": []map[string]string{
				{"role": "system", "content": "be brief"},
				{"role": "user", "content": "hello"},
			},
		})
		errCh <- err
	}()

	completeLatestChatTask(t, server, stores, "Succeeded", map[string]string{
		"result":      "executed",
		"last_output": "hello from orloj",
	}, "")

	if err := <-errCh; err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["object"] != "chat.completion" {
		t.Fatalf("unexpected object: %#v", payload["object"])
	}
	if payload["model"] != "demo-system" {
		t.Fatalf("unexpected model: %#v", payload["model"])
	}
	choices, _ := payload["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("expected 1 choice, got %#v", payload["choices"])
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	if message["content"] != "hello from orloj" {
		t.Fatalf("unexpected content: %#v", message["content"])
	}

	sessions, err := stores.Sessions.List(context.Background())
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	turns, err := stores.Sessions.ListTurns(context.Background(), sessions[0].Metadata.Name)
	if err != nil {
		t.Fatalf("list turns: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if !strings.Contains(turns[0].Content, "user: hello") {
		t.Fatalf("expected flattened user message in turn, got %q", turns[0].Content)
	}
}

func TestChatCompletionsIgnoresExecutedResult(t *testing.T) {
	ts, stores, server := newChatCompletionsTestServer(t)
	defer ts.Close()
	seedChatCompletionSystem(t, stores, "demo-system")

	errCh := make(chan error, 1)
	var resp *http.Response
	go func() {
		var err error
		resp, err = postChatCompletionsAsync(ts.URL, map[string]any{
			"model":    "demo-system",
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		})
		errCh <- err
	}()

	completeLatestChatTask(t, server, stores, "Succeeded", map[string]string{
		"result":                  "executed",
		"agent.1.message_content": "from agent one",
		"agent.2.message_content": "from agent two",
	}, "")

	if err := <-errCh; err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	choices, _ := payload["choices"].([]any)
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	if message["content"] != "from agent two" {
		t.Fatalf("expected agent.2 content, got %#v", message["content"])
	}
}

func TestChatCompletionsStreamFinalChunk(t *testing.T) {
	ts, stores, server := newChatCompletionsTestServer(t)
	defer ts.Close()
	seedChatCompletionSystem(t, stores, "demo-system")

	errCh := make(chan error, 1)
	var resp *http.Response
	go func() {
		var err error
		resp, err = postChatCompletionsAsync(ts.URL, map[string]any{
			"model":    "demo-system",
			"stream":   true,
			"messages": []map[string]string{{"role": "user", "content": "stream me"}},
		})
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("request failed before stream opened: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not send response headers before task completion")
	}
	if resp == nil {
		t.Fatal("expected streaming response")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected event-stream content type, got %q", ct)
	}

	completeLatestChatTask(t, server, stores, "Succeeded", map[string]string{
		"last_output":    "streamed answer",
		"_stream_deltas": "streamed |answer",
	}, "")

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"object":"chat.completion.chunk"`) && !strings.Contains(text, `"object": "chat.completion.chunk"`) {
		t.Fatalf("expected completion chunk in SSE body, got %s", text)
	}
	if !strings.Contains(text, `"content":"streamed "`) || !strings.Contains(text, `"content":"answer"`) {
		t.Fatalf("expected incremental content chunks in SSE body, got %s", text)
	}
	if !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("expected [DONE] terminator, got %s", text)
	}
}

func TestChatCompletionsWaitingApprovalConflict(t *testing.T) {
	ts, stores, server := newChatCompletionsTestServer(t)
	defer ts.Close()
	seedChatCompletionSystem(t, stores, "demo-system")

	errCh := make(chan error, 1)
	var resp *http.Response
	go func() {
		var err error
		resp, err = postChatCompletionsAsync(ts.URL, map[string]any{
			"model":    "demo-system",
			"messages": []map[string]string{{"role": "user", "content": "needs review"}},
		})
		errCh <- err
	}()

	completeLatestChatTask(t, server, stores, "WaitingApproval", map[string]string{
		"result": "executed",
	}, "")

	if err := <-errCh; err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 409, got %d body=%s", resp.StatusCode, body)
	}
}

func TestChatCompletionsFailedTask(t *testing.T) {
	ts, stores, server := newChatCompletionsTestServer(t)
	defer ts.Close()
	seedChatCompletionSystem(t, stores, "demo-system")

	errCh := make(chan error, 1)
	var resp *http.Response
	go func() {
		var err error
		resp, err = postChatCompletionsAsync(ts.URL, map[string]any{
			"model":    "demo-system",
			"messages": []map[string]string{{"role": "user", "content": "boom"}},
		})
		errCh <- err
	}()

	completeLatestChatTask(t, server, stores, "Failed", nil, "agent exploded")

	if err := <-errCh; err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 500, got %d body=%s", resp.StatusCode, body)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	errObj, _ := payload["error"].(map[string]any)
	if errObj["message"] != "agent exploded" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func TestIsStreamingWatchRequestIncludesChatCompletions(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if !api.IsLongLivedRequestForTest(req) {
		t.Fatal("expected /v1/chat/completions POST to bypass write timeout")
	}
}

func postChatCompletions(t *testing.T, baseURL string, payload map[string]any) *http.Response {
	t.Helper()
	resp, err := postChatCompletionsAsync(baseURL, payload)
	if err != nil {
		t.Fatalf("POST chat completions: %v", err)
	}
	return resp
}

func postChatCompletionsAsync(baseURL string, payload map[string]any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}
