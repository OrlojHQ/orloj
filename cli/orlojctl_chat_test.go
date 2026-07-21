package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

func TestRunChatCreatesSessionStreamsTurnAndPreservesIt(t *testing.T) {
	var createdSession resources.Session
	var idempotencyKey string
	sessionName := "demo-system-fixed"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("namespace"); got != "team-a" {
			t.Errorf("namespace=%q, want team-a", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			if err := json.NewDecoder(r.Body).Decode(&createdSession); err != nil {
				t.Errorf("decode Session: %v", err)
			}
			writeChatTestJSON(t, w, http.StatusCreated, resources.Session{
				Metadata: resources.ObjectMeta{Name: sessionName, Namespace: "team-a"},
				Spec:     resources.SessionSpec{System: "demo-system"},
				Status: resources.SessionStatus{
					Phase:             resources.SessionPhaseWaitingInput,
					LastEventSequence: 1,
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions/"+sessionName+"/turns":
			idempotencyKey = r.Header.Get("Idempotency-Key")
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode turn: %v", err)
			}
			if body["content"] != "hello" {
				t.Errorf("turn content=%#v, want hello", body["content"])
			}
			writeChatTestJSON(t, w, http.StatusAccepted, chatTurnResponse{
				Turn:    resources.SessionTurn{ID: "turn-1"},
				Created: true,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/"+sessionName+"/stream":
			if got := r.URL.Query().Get("after"); got != "1" {
				t.Errorf("stream after=%q, want 1", got)
			}
			writeChatTestSSE(t, w,
				chatTestEvent(2, "turn-1", resources.SessionEventMessageDelta, map[string]any{"delta": "hello "}),
				chatTestEvent(3, "turn-1", resources.SessionEventMessageDelta, map[string]any{"delta": "world"}),
				chatTestEvent(4, "turn-1", resources.SessionEventMessageCompleted, map[string]any{"content": "hello world"}),
				chatTestEvent(5, "turn-1", resources.SessionEventTurnCompleted, nil),
			)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/"+sessionName:
			writeChatTestJSON(t, w, http.StatusOK, resources.Session{
				Metadata: resources.ObjectMeta{Name: sessionName, Namespace: "team-a"},
				Spec:     resources.SessionSpec{System: "demo-system"},
				Status: resources.SessionStatus{
					Phase:             resources.SessionPhaseWaitingInput,
					CompletedTurns:    1,
					LastEventSequence: 5,
				},
			})
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := runChat(context.Background(), chatOptions{
		Server:      server.URL,
		Namespace:   "team-a",
		System:      "demo-system",
		In:          strings.NewReader("hello\n/exit\n"),
		Out:         &out,
		ErrOut:      &out,
		Client:      server.Client(),
		Interactive: false,
		GenerateIdentifier: func(prefix string) string {
			if prefix == "chat-demo-system" {
				return sessionName
			}
			return "turn-key-fixed"
		},
	})
	if err != nil {
		t.Fatalf("runChat: %v", err)
	}
	if createdSession.Metadata.Name != sessionName || createdSession.Spec.System != "demo-system" {
		t.Fatalf("created Session=%+v", createdSession)
	}
	if createdSession.Spec.MaxTurns != 0 {
		t.Fatalf("max_turns=%d, want unlimited", createdSession.Spec.MaxTurns)
	}
	if idempotencyKey != "turn-key-fixed" {
		t.Fatalf("Idempotency-Key=%q", idempotencyKey)
	}
	for _, want := range []string{
		"chat: created session/" + sessionName,
		"assistant> hello world",
		"chat: session/" + sessionName + " preserved",
		"--session " + sessionName,
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunChatResumesActiveApprovalAndContinuesTurn(t *testing.T) {
	const (
		sessionName  = "incident-chat"
		approvalName = "ta-review"
	)
	var decisionBody map[string]string
	var decisionCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/"+sessionName:
			phase := resources.SessionPhaseWaitingApproval
			lastSequence := uint64(8)
			if decisionCalls.Load() > 0 {
				phase = resources.SessionPhaseWaitingInput
				lastSequence = 12
			}
			session := resources.Session{
				Metadata: resources.ObjectMeta{Name: sessionName, Namespace: "default"},
				Spec:     resources.SessionSpec{System: "incident-system"},
				Status: resources.SessionStatus{
					Phase:             phase,
					ActiveTurnID:      "turn-active",
					LastEventSequence: lastSequence,
					BlockedOn: &resources.TaskBlockedOn{
						Kind:   "ToolApproval",
						Name:   approvalName,
						Reason: "write operation",
					},
				},
			}
			if decisionCalls.Load() > 0 {
				session.Status.ActiveTurnID = ""
				session.Status.BlockedOn = nil
				session.Status.CompletedTurns = 1
			}
			writeChatTestJSON(t, w, http.StatusOK, session)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tool-approvals/"+approvalName:
			writeChatTestJSON(t, w, http.StatusOK, resources.ToolApproval{
				Metadata: resources.ObjectMeta{Name: approvalName},
				Spec: resources.ToolApprovalSpec{
					TaskRef:        "session-incident-turn",
					Tool:           "kubernetes.patch_service",
					Agent:          "remediation-agent",
					OperationClass: "write",
					Input:          `{"service":"payment-cache","version":"v43"}`,
					Reason:         "write operation",
				},
				Status: resources.ToolApprovalStatus{
					Phase:     "Pending",
					ExpiresAt: "2026-07-21T02:00:00Z",
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tool-approvals/"+approvalName+"/approve":
			if err := json.NewDecoder(r.Body).Decode(&decisionBody); err != nil {
				t.Errorf("decode decision: %v", err)
			}
			decisionCalls.Add(1)
			writeChatTestJSON(t, w, http.StatusOK, map[string]any{"ok": true})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/"+sessionName+"/stream":
			if got := r.URL.Query().Get("after"); got != "0" {
				t.Errorf("stream after=%q, want 0", got)
			}
			writeChatTestSSE(t, w,
				chatTestEvent(4, "turn-active", resources.SessionEventApprovalRequested, map[string]any{
					"kind":   "ToolApproval",
					"name":   "ta-already-resolved",
					"reason": "earlier operation",
				}),
				chatTestEvent(5, "turn-active", resources.SessionEventApprovalResolved, map[string]any{
					"name": "ta-already-resolved",
				}),
				chatTestEvent(8, "turn-active", resources.SessionEventApprovalRequested, map[string]any{
					"kind":   "ToolApproval",
					"name":   approvalName,
					"reason": "write operation",
				}),
				chatTestEvent(9, "turn-active", resources.SessionEventApprovalResolved, map[string]any{"name": approvalName}),
				chatTestEvent(10, "turn-active", resources.SessionEventMessageDelta, map[string]any{"delta": "change applied"}),
				chatTestEvent(11, "turn-active", resources.SessionEventMessageCompleted, map[string]any{"content": "change applied"}),
				chatTestEvent(12, "turn-active", resources.SessionEventTurnCompleted, nil),
			)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := runChat(context.Background(), chatOptions{
		Server:      server.URL,
		Namespace:   "default",
		System:      "incident-system",
		SessionName: sessionName,
		DecidedBy:   "operator@example.com",
		In:          strings.NewReader("approve\nverified selector\n/exit\n"),
		Out:         &out,
		ErrOut:      &out,
		Client:      server.Client(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("runChat: %v\noutput:\n%s", err, out.String())
	}
	if decisionCalls.Load() != 1 {
		t.Fatalf("decision calls=%d, want 1", decisionCalls.Load())
	}
	if decisionBody["decided_by"] != "operator@example.com" ||
		decisionBody["comment"] != "verified selector" {
		t.Fatalf("decision body=%v", decisionBody)
	}
	for _, want := range []string{
		"chat: resumed session/" + sessionName,
		"chat: reattaching to active turn",
		"tool: kubernetes.patch_service",
		`"service": "payment-cache"`,
		"approval: approved tool-approval/" + approvalName,
		"assistant> change applied",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunChatNonInteractiveApprovalStopsWithCommands(t *testing.T) {
	const sessionName = "approval-chat"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sessions/" + sessionName:
			writeChatTestJSON(t, w, http.StatusOK, resources.Session{
				Metadata: resources.ObjectMeta{Name: sessionName},
				Spec:     resources.SessionSpec{System: "demo"},
				Status: resources.SessionStatus{
					Phase:        resources.SessionPhaseWaitingApproval,
					ActiveTurnID: "turn-1",
					BlockedOn: &resources.TaskBlockedOn{
						Kind: "ToolApproval",
						Name: "approval-1",
					},
				},
			})
		case "/v1/tool-approvals/approval-1":
			writeChatTestJSON(t, w, http.StatusOK, resources.ToolApproval{
				Metadata: resources.ObjectMeta{Name: "approval-1"},
				Spec: resources.ToolApprovalSpec{
					TaskRef: "task-1",
					Tool:    "deploy",
				},
			})
		case "/v1/sessions/" + sessionName + "/stream":
			writeChatTestSSE(t, w,
				chatTestEvent(1, "turn-1", resources.SessionEventApprovalRequested, map[string]any{
					"kind": "ToolApproval",
					"name": "approval-1",
				}),
			)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	err := runChat(context.Background(), chatOptions{
		Server:      server.URL,
		System:      "demo",
		SessionName: sessionName,
		In:          strings.NewReader("approve\n"),
		Client:      server.Client(),
		Interactive: false,
	})
	if err == nil {
		t.Fatal("expected non-interactive approval error")
	}
	for _, want := range []string{
		"requires an interactive decision",
		"orlojctl approve tool-approval approval-1",
		"orlojctl deny tool-approval approval-1",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

func TestHandleChatApprovalSkipsAlreadyResolvedDecision(t *testing.T) {
	var decisionCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/tool-approvals/approval-1" {
			writeChatTestJSON(t, w, http.StatusOK, resources.ToolApproval{
				Metadata: resources.ObjectMeta{Name: "approval-1"},
				Spec: resources.ToolApprovalSpec{
					TaskRef: "task-1",
					Tool:    "deploy",
				},
				Status: resources.ToolApprovalStatus{Phase: "Approved"},
			})
			return
		}
		decisionCalls.Add(1)
		http.Error(w, "unexpected decision", http.StatusConflict)
	}))
	defer server.Close()

	var out bytes.Buffer
	input := newChatLineReader(strings.NewReader("approve\ncomment\n"))
	defer input.Close()
	err := handleChatApproval(context.Background(), chatOptions{
		Server:      server.URL,
		Namespace:   "default",
		Out:         &out,
		Client:      server.Client(),
		Interactive: true,
	}, input, "ToolApproval", "approval-1", "")
	if err != nil {
		t.Fatalf("handleChatApproval: %v", err)
	}
	if decisionCalls.Load() != 0 {
		t.Fatalf("decision calls=%d, want 0", decisionCalls.Load())
	}
	if !strings.Contains(out.String(), "already Approved") {
		t.Fatalf("missing resolved status:\n%s", out.String())
	}
}

func TestRunChatReconnectsFromLastEvent(t *testing.T) {
	const sessionName = "reconnect-chat"
	var streamCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			writeChatTestJSON(t, w, http.StatusCreated, resources.Session{
				Metadata: resources.ObjectMeta{Name: sessionName},
				Spec:     resources.SessionSpec{System: "demo"},
				Status:   resources.SessionStatus{Phase: resources.SessionPhaseWaitingInput, LastEventSequence: 1},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions/"+sessionName+"/turns":
			writeChatTestJSON(t, w, http.StatusAccepted, chatTurnResponse{
				Turn:    resources.SessionTurn{ID: "turn-1"},
				Created: true,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/"+sessionName+"/stream":
			call := streamCalls.Add(1)
			if call == 1 {
				if got := r.URL.Query().Get("after"); got != "1" {
					t.Errorf("first stream after=%q, want 1", got)
				}
				writeChatTestSSE(t, w,
					chatTestEvent(2, "turn-1", resources.SessionEventMessageDelta, map[string]any{"delta": "hel"}),
				)
				return
			}
			if got := r.URL.Query().Get("after"); got != "2" {
				t.Errorf("second stream after=%q, want 2", got)
			}
			writeChatTestSSE(t, w,
				chatTestEvent(3, "turn-1", resources.SessionEventMessageDelta, map[string]any{"delta": "lo"}),
				chatTestEvent(4, "turn-1", resources.SessionEventMessageCompleted, map[string]any{"content": "hello"}),
				chatTestEvent(5, "turn-1", resources.SessionEventTurnCompleted, nil),
			)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/"+sessionName:
			writeChatTestJSON(t, w, http.StatusOK, resources.Session{
				Metadata: resources.ObjectMeta{Name: sessionName},
				Spec:     resources.SessionSpec{System: "demo"},
				Status:   resources.SessionStatus{Phase: resources.SessionPhaseWaitingInput, LastEventSequence: 5},
			})
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := runChat(context.Background(), chatOptions{
		Server:           server.URL,
		System:           "demo",
		In:               strings.NewReader("hi\n/exit\n"),
		Out:              &out,
		ErrOut:           &errOut,
		Client:           server.Client(),
		InitialReconnect: time.Millisecond,
		MaxReconnect:     time.Millisecond,
		GenerateIdentifier: func(prefix string) string {
			if prefix == "chat-demo" {
				return sessionName
			}
			return "turn-key"
		},
	})
	if err != nil {
		t.Fatalf("runChat: %v", err)
	}
	if streamCalls.Load() != 2 {
		t.Fatalf("stream calls=%d, want 2", streamCalls.Load())
	}
	if !strings.Contains(out.String(), "assistant> hello") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
	if !strings.Contains(errOut.String(), "reconnecting from event 2") {
		t.Fatalf("missing reconnect output:\n%s", errOut.String())
	}
}

func TestRunChatMarksResetOutputAsTentative(t *testing.T) {
	const sessionName = "reset-chat"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			writeChatTestJSON(t, w, http.StatusCreated, resources.Session{
				Metadata: resources.ObjectMeta{Name: sessionName},
				Spec:     resources.SessionSpec{System: "demo"},
				Status:   resources.SessionStatus{Phase: resources.SessionPhaseWaitingInput, LastEventSequence: 1},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/turns"):
			writeChatTestJSON(t, w, http.StatusAccepted, chatTurnResponse{
				Turn:    resources.SessionTurn{ID: "turn-1"},
				Created: true,
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/stream"):
			writeChatTestSSE(t, w,
				chatTestEvent(2, "turn-1", resources.SessionEventMessageDelta, map[string]any{"delta": "old"}),
				chatTestEvent(3, "turn-1", resources.SessionEventMessageReset, map[string]any{"reason": "lease recovery"}),
				chatTestEvent(4, "turn-1", resources.SessionEventMessageDelta, map[string]any{"delta": "new"}),
				chatTestEvent(5, "turn-1", resources.SessionEventMessageCompleted, map[string]any{"content": "new"}),
				chatTestEvent(6, "turn-1", resources.SessionEventTurnCompleted, nil),
			)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/"+sessionName:
			writeChatTestJSON(t, w, http.StatusOK, resources.Session{
				Metadata: resources.ObjectMeta{Name: sessionName},
				Spec:     resources.SessionSpec{System: "demo"},
				Status:   resources.SessionStatus{Phase: resources.SessionPhaseWaitingInput, LastEventSequence: 6},
			})
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := runChat(context.Background(), chatOptions{
		Server: server.URL,
		System: "demo",
		In:     strings.NewReader("hi\n/exit\n"),
		Out:    &out,
		Client: server.Client(),
		GenerateIdentifier: func(prefix string) string {
			if prefix == "chat-demo" {
				return sessionName
			}
			return "turn-key"
		},
	})
	if err != nil {
		t.Fatalf("runChat: %v", err)
	}
	for _, want := range []string{
		"assistant> old",
		"tentative assistant output reset (lease recovery)",
		"assistant> new",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunChatStopsAfterTerminalTurnFailure(t *testing.T) {
	const sessionName = "failed-chat"
	var streamCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			writeChatTestJSON(t, w, http.StatusCreated, resources.Session{
				Metadata: resources.ObjectMeta{Name: sessionName},
				Spec:     resources.SessionSpec{System: "demo"},
				Status:   resources.SessionStatus{Phase: resources.SessionPhaseWaitingInput, LastEventSequence: 1},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/turns"):
			writeChatTestJSON(t, w, http.StatusAccepted, chatTurnResponse{
				Turn:    resources.SessionTurn{ID: "turn-1"},
				Created: true,
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/stream"):
			streamCalls.Add(1)
			writeChatTestSSE(t, w,
				chatTestEvent(2, "turn-1", resources.SessionEventTurnFailed, map[string]any{"error": "agent failed"}),
			)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	err := runChat(context.Background(), chatOptions{
		Server:           server.URL,
		System:           "demo",
		In:               strings.NewReader("hi\n"),
		Client:           server.Client(),
		InitialReconnect: time.Millisecond,
		MaxReconnect:     time.Millisecond,
		GenerateIdentifier: func(prefix string) string {
			if prefix == "chat-demo" {
				return sessionName
			}
			return "turn-key"
		},
	})
	if err == nil || !strings.Contains(err.Error(), "agent failed") {
		t.Fatalf("error=%v, want terminal failure", err)
	}
	if streamCalls.Load() != 1 {
		t.Fatalf("stream calls=%d, want no reconnect", streamCalls.Load())
	}
}

func TestChatLineReaderIsContextCancellable(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	input := newChatLineReader(reader)
	defer input.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, _, err := input.Next(ctx)
		errCh <- err
	}()

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("input read did not stop after context cancellation")
	}
}

func TestRunChatReplaysActiveTurnBeforeFollowingNewOutput(t *testing.T) {
	const sessionName = "active-chat"
	var sessionGets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/"+sessionName:
			if sessionGets.Add(1) == 1 {
				writeChatTestJSON(t, w, http.StatusOK, resources.Session{
					Metadata: resources.ObjectMeta{Name: sessionName},
					Spec:     resources.SessionSpec{System: "demo"},
					Status: resources.SessionStatus{
						Phase:             resources.SessionPhaseRunning,
						ActiveTurnID:      "turn-active",
						LastEventSequence: 2,
					},
				})
				return
			}
			writeChatTestJSON(t, w, http.StatusOK, resources.Session{
				Metadata: resources.ObjectMeta{Name: sessionName},
				Spec:     resources.SessionSpec{System: "demo"},
				Status:   resources.SessionStatus{Phase: resources.SessionPhaseWaitingInput, LastEventSequence: 5},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/"+sessionName+"/stream":
			if got := r.URL.Query().Get("after"); got != "0" {
				t.Errorf("stream after=%q, want active-turn replay from 0", got)
			}
			writeChatTestSSE(t, w,
				chatTestEvent(2, "turn-active", resources.SessionEventMessageDelta, map[string]any{"delta": "hello "}),
				chatTestEvent(3, "turn-active", resources.SessionEventMessageDelta, map[string]any{"delta": "world"}),
				chatTestEvent(4, "turn-active", resources.SessionEventMessageCompleted, map[string]any{"content": "hello world"}),
				chatTestEvent(5, "turn-active", resources.SessionEventTurnCompleted, nil),
			)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := runChat(context.Background(), chatOptions{
		Server:      server.URL,
		System:      "demo",
		SessionName: sessionName,
		In:          strings.NewReader("/exit\n"),
		Out:         &out,
		Client:      server.Client(),
	})
	if err != nil {
		t.Fatalf("runChat: %v", err)
	}
	if !strings.Contains(out.String(), "assistant> hello world") {
		t.Fatalf("missing reconstructed output:\n%s", out.String())
	}
	if strings.Contains(out.String(), "assistant (final)") {
		t.Fatalf("active output was duplicated as a divergent final:\n%s", out.String())
	}
}

func TestOpenChatSessionRejectsSystemMismatchAndTerminalSession(t *testing.T) {
	tests := []struct {
		name    string
		session resources.Session
		want    string
	}{
		{
			name: "system mismatch",
			session: resources.Session{
				Metadata: resources.ObjectMeta{Name: "existing"},
				Spec:     resources.SessionSpec{System: "other"},
				Status:   resources.SessionStatus{Phase: resources.SessionPhaseWaitingInput},
			},
			want: `belongs to AgentSystem "other"`,
		},
		{
			name: "terminal",
			session: resources.Session{
				Metadata: resources.ObjectMeta{Name: "existing"},
				Spec:     resources.SessionSpec{System: "demo"},
				Status:   resources.SessionStatus{Phase: resources.SessionPhaseCompleted},
			},
			want: "is terminal",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeChatTestJSON(t, w, http.StatusOK, test.session)
			}))
			defer server.Close()

			opts := chatOptions{
				Server:      server.URL,
				Namespace:   "default",
				System:      "demo",
				SessionName: "existing",
				Client:      server.Client(),
			}
			if err := normalizeChatOptions(&opts); err != nil {
				t.Fatal(err)
			}
			_, _, err := openChatSession(context.Background(), opts)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want substring %q", err, test.want)
			}
		})
	}
}

func TestScanChatEventStreamSupportsCommentsAndMultilineData(t *testing.T) {
	input := strings.NewReader(": keep-alive\nid: 7\nevent: custom\ndata: first\ndata: second\n\n")
	var frames []chatEventFrame
	err := scanChatEventStream(input, func(frame chatEventFrame) error {
		frames = append(frames, frame)
		return nil
	})
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("scanChatEventStream: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames=%d, want 1", len(frames))
	}
	if frames[0].ID != "7" || frames[0].Type != "custom" || frames[0].Data != "first\nsecond" {
		t.Fatalf("frame=%+v", frames[0])
	}
}

func writeChatTestJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func writeChatTestSSE(t *testing.T, w http.ResponseWriter, events ...resources.SessionEvent) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	for _, event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			t.Errorf("marshal event: %v", err)
			return
		}
		if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Sequence, event.Type, raw); err != nil {
			t.Errorf("write event: %v", err)
			return
		}
	}
}

func chatTestEvent(sequence uint64, turnID string, eventType string, payload map[string]any) resources.SessionEvent {
	return resources.SessionEvent{
		Sequence:    sequence,
		ID:          fmt.Sprintf("event-%d", sequence),
		SessionName: "chat",
		Namespace:   "default",
		TurnID:      turnID,
		Type:        eventType,
		Timestamp:   "2026-07-21T00:00:00Z",
		Payload:     payload,
	}
}
