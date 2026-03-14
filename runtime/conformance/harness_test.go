package conformance_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	agentruntime "github.com/AnonJon/orloj/runtime"
	"github.com/AnonJon/orloj/runtime/conformance"
	conformancecases "github.com/AnonJon/orloj/runtime/conformance/cases"
)

type runtimeFunc struct {
	call func(ctx context.Context, tool string, input string) (string, error)
}

func (r runtimeFunc) Call(ctx context.Context, tool string, input string) (string, error) {
	return r.call(ctx, tool, input)
}

func TestRunCasesPassesCanonicalSuite(t *testing.T) {
	executor := agentruntime.NewToolContractExecutor(runtimeFunc{
		call: func(_ context.Context, tool string, input string) (string, error) {
			switch tool {
			case "vector_db":
				return "", agentruntime.NewToolDeniedError("missing required permission", map[string]string{"required": "tool:vector_db:invoke"}, agentruntime.ErrToolPermissionDenied)
			default:
				return fmt.Sprintf("tool=%s input=%s", tool, input), nil
			}
		},
	})
	cases := []conformance.Case{
		{
			Name: "success",
			Request: func() agentruntime.ToolExecutionRequest {
				req := conformancecases.BaseRequest("req-success", "web_search")
				req.Input = map[string]string{"query": "orloj"}
				req.Trace = agentruntime.ToolExecutionTraceContext{TraceID: "trace-a", SpanID: "span-a"}
				return req
			}(),
			Expected: conformance.Expected{Status: agentruntime.ToolExecutionStatusOK},
		},
		{
			Name:    "denied",
			Request: conformancecases.BaseRequest("req-denied", "vector_db"),
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusDenied,
				ErrorCode: agentruntime.ToolCodePermissionDenied,
				Reason:    agentruntime.ToolReasonPermissionDenied,
				Retryable: conformancecases.BoolPtr(false),
			},
		},
		conformancecases.UnknownVersionCase("req-invalid-version", "web_search"),
	}
	failures := conformance.RunCases(context.Background(), executor, cases)
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Errorf("case=%s err=%v", failure.Case, failure.Err)
		}
		t.Fatalf("expected zero conformance failures, got %d", len(failures))
	}
}

func TestRunCasesReturnsFailures(t *testing.T) {
	executor := agentruntime.NewToolContractExecutor(runtimeFunc{
		call: func(_ context.Context, tool string, input string) (string, error) {
			return "ok", nil
		},
	})
	failures := conformance.RunCases(context.Background(), executor, []conformance.Case{
		{
			Name:    "intentional-mismatch",
			Request: conformancecases.BaseRequest("req-mismatch", "web_search"),
			Expected: conformance.Expected{
				Status: agentruntime.ToolExecutionStatusDenied,
			},
		},
	})
	if len(failures) != 1 {
		t.Fatalf("expected exactly one failure, got %d", len(failures))
	}
}

func TestRunCasesSupportsCancelAndLatencyGuards(t *testing.T) {
	executor := agentruntime.NewToolContractExecutor(runtimeFunc{
		call: func(ctx context.Context, _ string, _ string) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return "ok", nil
			}
		},
	})
	failures := conformance.RunCases(context.Background(), executor, []conformance.Case{
		{
			Name:        "latency-guard",
			Request:     conformancecases.BaseRequest("req-latency-guard", "web_search"),
			CallTimeout: 10 * time.Millisecond,
			MaxLatency:  120 * time.Millisecond,
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeTimeout,
				Reason:    agentruntime.ToolReasonExecutionTimeout,
				Retryable: conformancecases.BoolPtr(true),
			},
		},
	})
	if len(failures) != 0 {
		t.Fatalf("expected no failures, got %+v", failures)
	}
}
