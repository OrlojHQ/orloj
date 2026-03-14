package agentruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestToolErrorEnvelopeIncludesCanonicalFields(t *testing.T) {
	err := NewToolError(
		ToolStatusError,
		ToolCodeExecutionFailed,
		ToolReasonBackendFailure,
		true,
		"temporary upstream failure",
		errors.New("upstream unavailable"),
		map[string]string{"tool": "web_search"},
	)
	toolErr, ok := AsToolError(err)
	if !ok {
		t.Fatal("expected ToolError")
	}
	if !toolErr.Retryable {
		t.Fatal("expected retryable=true")
	}
	text := err.Error()
	required := []string{
		"tool_status=error",
		"tool_code=execution_failed",
		"tool_reason=tool_backend_failure",
		"retryable=true",
		"tool=web_search",
	}
	for _, marker := range required {
		if !strings.Contains(text, marker) {
			t.Fatalf("expected marker %q in %q", marker, text)
		}
	}
}

func TestIsToolDeniedErrorRecognizesDeniedEnvelope(t *testing.T) {
	err := NewToolDeniedError(
		"policy permission denied for tool=vector_db required=tool:vector_db:invoke",
		map[string]string{"tool": "vector_db"},
		ErrToolPermissionDenied,
	)
	if !IsToolDeniedError(err) {
		t.Fatal("expected denied tool error")
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodePermissionDenied {
		t.Fatalf("expected code %q, got %q", ToolCodePermissionDenied, code)
	}
	if reason != ToolReasonPermissionDenied {
		t.Fatalf("expected reason %q, got %q", ToolReasonPermissionDenied, reason)
	}
	if retryable {
		t.Fatal("expected denied error retryable=false")
	}
}

func TestNormalizeToolErrorTimeout(t *testing.T) {
	err := normalizeToolError(context.DeadlineExceeded, "web_search", 1500*time.Millisecond)
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeTimeout {
		t.Fatalf("expected code %q, got %q", ToolCodeTimeout, code)
	}
	if reason != ToolReasonExecutionTimeout {
		t.Fatalf("expected reason %q, got %q", ToolReasonExecutionTimeout, reason)
	}
	if !retryable {
		t.Fatal("expected timeout to be retryable")
	}
}
