package agentruntime

import (
	"context"
	"errors"
	"testing"
)

type contractFuncRuntime struct {
	call func(ctx context.Context, tool string, input string) (string, error)
}

func (r contractFuncRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
	if r.call == nil {
		return "", errors.New("missing runtime function")
	}
	return r.call(ctx, tool, input)
}

func TestExecuteToolContractSuccess(t *testing.T) {
	executor := NewToolContractExecutor(&MockToolClient{})
	response, err := executor.Execute(context.Background(), ToolExecutionRequest{
		ToolContractVersion: ToolContractVersionV1,
		RequestID:           "req-1",
		Namespace:           "default",
		Agent:               "research-agent",
		Tool: ToolExecutionRequestTool{
			Name:      "web_search",
			Operation: ToolOperationInvoke,
		},
		Input: map[string]string{
			"query": "latest updates",
		},
		Trace: ToolExecutionTraceContext{
			TraceID: "trace-1",
			SpanID:  "span-1",
		},
	})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if response.Status != ToolExecutionStatusOK {
		t.Fatalf("expected status=%s got %s", ToolExecutionStatusOK, response.Status)
	}
	if response.ToolContractVersion != ToolContractVersionV1 {
		t.Fatalf("expected version=%s got %s", ToolContractVersionV1, response.ToolContractVersion)
	}
	if response.RequestID != "req-1" {
		t.Fatalf("expected request_id=req-1 got %s", response.RequestID)
	}
	if response.Output.Result == "" {
		t.Fatal("expected output result")
	}
	if response.Usage.Attempt != 1 {
		t.Fatalf("expected attempt=1 got %d", response.Usage.Attempt)
	}
	if response.Trace.TraceID != "trace-1" || response.Trace.SpanID != "span-1" {
		t.Fatalf("expected trace context to roundtrip, got trace_id=%s span_id=%s", response.Trace.TraceID, response.Trace.SpanID)
	}
}

func TestExecuteToolContractRejectsUnknownMajorVersion(t *testing.T) {
	response, err := ExecuteToolContract(context.Background(), &MockToolClient{}, ToolExecutionRequest{
		ToolContractVersion: "v9",
		RequestID:           "req-2",
		Tool: ToolExecutionRequestTool{
			Name: "web_search",
		},
	})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if response.Status != ToolExecutionStatusError {
		t.Fatalf("expected status=%s got %s", ToolExecutionStatusError, response.Status)
	}
	if response.Error == nil {
		t.Fatal("expected error envelope")
	}
	if response.Error.Code != ToolCodeInvalidInput {
		t.Fatalf("expected code=%s got %s", ToolCodeInvalidInput, response.Error.Code)
	}
	if response.Error.Reason != ToolReasonInvalidInput {
		t.Fatalf("expected reason=%s got %s", ToolReasonInvalidInput, response.Error.Reason)
	}
	if response.Error.Retryable {
		t.Fatal("expected unknown major version to be non-retryable")
	}
}

func TestExecuteToolContractMapsDeniedError(t *testing.T) {
	runtime := contractFuncRuntime{
		call: func(context.Context, string, string) (string, error) {
			return "", NewToolDeniedError("policy denied", map[string]string{"rule": "vector-db-invoke"}, ErrToolPermissionDenied)
		},
	}
	response, err := ExecuteToolContract(context.Background(), runtime, ToolExecutionRequest{
		RequestID: "req-3",
		Tool: ToolExecutionRequestTool{
			Name: "vector_db",
		},
	})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if response.Status != ToolExecutionStatusDenied {
		t.Fatalf("expected status=%s got %s", ToolExecutionStatusDenied, response.Status)
	}
	if response.Error == nil {
		t.Fatal("expected denied error envelope")
	}
	if response.Error.Code != ToolCodePermissionDenied {
		t.Fatalf("expected code=%s got %s", ToolCodePermissionDenied, response.Error.Code)
	}
	if response.Error.Reason != ToolReasonPermissionDenied {
		t.Fatalf("expected reason=%s got %s", ToolReasonPermissionDenied, response.Error.Reason)
	}
	if response.Error.Retryable {
		t.Fatal("expected denied error to be non-retryable")
	}
}

func TestToolExecutionResponseToError(t *testing.T) {
	resp := ToolExecutionResponse{
		Status: ToolExecutionStatusDenied,
		Error: &ToolExecutionFailure{
			Code:      ToolCodePermissionDenied,
			Reason:    ToolReasonPermissionDenied,
			Retryable: false,
			Message:   "denied by policy",
			Details:   map[string]string{"rule": "db-write"},
		},
	}
	err := resp.ToError()
	if err == nil {
		t.Fatal("expected error from denied response")
	}
	if !IsToolDeniedError(err) {
		t.Fatalf("expected denied error, got %v", err)
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodePermissionDenied || reason != ToolReasonPermissionDenied {
		t.Fatalf("unexpected metadata code=%s reason=%s", code, reason)
	}
	if retryable {
		t.Fatal("expected non-retryable denied error")
	}
}
