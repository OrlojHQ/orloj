package a2a

import (
	"encoding/json"
	"testing"
)

func TestJSONRPCRequestMarshal(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  MethodTaskSend,
		Params: TaskSendParams{
			ID: "task-1",
			Message: TaskMessage{
				Role: "user",
				Parts: []TaskPart{
					{Type: "text", Text: "Hello"},
				},
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded JSONRPCRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", decoded.JSONRPC)
	}
	if decoded.Method != MethodTaskSend {
		t.Errorf("expected method %s, got %s", MethodTaskSend, decoded.Method)
	}
}

func TestJSONRPCResponseWithError(t *testing.T) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      "req-1",
		Error: &JSONRPCError{
			Code:    ErrCodeMethodNotFound,
			Message: "method not found",
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded JSONRPCResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Error == nil {
		t.Fatal("expected error to be set")
	}
	if decoded.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("expected code %d, got %d", ErrCodeMethodNotFound, decoded.Error.Code)
	}
	if decoded.Result != nil {
		t.Error("expected result to be nil when error is set")
	}
}

func TestTaskStates(t *testing.T) {
	if TaskStateSubmitted != "submitted" {
		t.Errorf("unexpected submitted state: %q", TaskStateSubmitted)
	}
	if TaskStateCompleted != "completed" {
		t.Errorf("unexpected completed state: %q", TaskStateCompleted)
	}
	if TaskStateFailed != "failed" {
		t.Errorf("unexpected failed state: %q", TaskStateFailed)
	}
}

func TestMethodConstants(t *testing.T) {
	if MethodTaskSend != "tasks/send" {
		t.Errorf("unexpected method: %q", MethodTaskSend)
	}
	if MethodTaskGet != "tasks/get" {
		t.Errorf("unexpected method: %q", MethodTaskGet)
	}
	if MethodTaskCancel != "tasks/cancel" {
		t.Errorf("unexpected method: %q", MethodTaskCancel)
	}
	if MethodTaskSubscribe != "tasks/sendSubscribe" {
		t.Errorf("unexpected method: %q", MethodTaskSubscribe)
	}
}

func TestAgentCardJSON(t *testing.T) {
	card := AgentCard{
		Name:            "test",
		URL:             "https://example.com/a2a",
		ProtocolVersion: "1.0",
		Capabilities: CardCapabilities{
			Streaming: true,
		},
		Skills: []CardSkill{{
			ID:   "search",
			Name: "search",
		}},
	}

	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded AgentCard
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Name != "test" {
		t.Errorf("expected name test, got %s", decoded.Name)
	}
	if !decoded.Capabilities.Streaming {
		t.Error("expected streaming=true")
	}
	if len(decoded.Skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(decoded.Skills))
	}
}
