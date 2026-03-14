package agentruntime

import (
	"errors"
	"testing"
)

func TestSelectAuthorizedToolCallsFromStructuredResponse(t *testing.T) {
	calls, err := selectAuthorizedToolCalls(ModelResponse{
		ToolCalls: []ModelToolCall{
			{Name: "web_search", Input: "latest ai news"},
		},
	}, []string{"web_search", "vector_db"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(calls))
	}
	if calls[0].Name != "web_search" {
		t.Fatalf("unexpected tool call name %q", calls[0].Name)
	}
	if calls[0].Input != "latest ai news" {
		t.Fatalf("unexpected tool input %q", calls[0].Input)
	}
}

func TestSelectAuthorizedToolCallsFromJSONContent(t *testing.T) {
	calls, err := selectAuthorizedToolCalls(ModelResponse{
		Content: `{"tool":"vector_db","input":"query=agents"}`,
	}, []string{"web_search", "vector_db"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(calls))
	}
	if calls[0].Name != "vector_db" {
		t.Fatalf("unexpected tool call name %q", calls[0].Name)
	}
}

func TestSelectAuthorizedToolCallsUnauthorized(t *testing.T) {
	_, err := selectAuthorizedToolCalls(ModelResponse{
		ToolCalls: []ModelToolCall{
			{Name: "shell_exec", Input: "rm -rf /"},
		},
	}, []string{"web_search"})
	if err == nil {
		t.Fatal("expected unauthorized tool error")
	}
	if !errors.Is(err, ErrToolPermissionDenied) {
		t.Fatalf("expected ErrToolPermissionDenied, got %v", err)
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodePermissionDenied || reason != ToolReasonPermissionDenied {
		t.Fatalf("unexpected metadata code=%s reason=%s", code, reason)
	}
	if retryable {
		t.Fatal("expected unauthorized request to be non-retryable")
	}
}
