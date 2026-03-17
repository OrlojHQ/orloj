package agentruntime

import (
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestClassifyMessageRetryabilityUsesToolErrorMetadata(t *testing.T) {
	policy := resources.TaskMessageRetryPolicy{}
	err := NewToolDeniedError(
		"policy permission denied for tool=vector_db required=tool:vector_db:invoke",
		map[string]string{"tool": "vector_db"},
		ErrToolPermissionDenied,
	)
	classification := classifyMessageRetryability(policy, err)
	if classification.Retryable {
		t.Fatal("expected non-retryable classification for tool denied error")
	}
	if classification.Reason != ToolReasonPermissionDenied {
		t.Fatalf("expected reason %q, got %q", ToolReasonPermissionDenied, classification.Reason)
	}
}
