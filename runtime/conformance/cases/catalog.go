package cases

import (
	"fmt"
	"strings"
	"time"

	agentruntime "github.com/AnonJon/orloj/runtime"
	"github.com/AnonJon/orloj/runtime/conformance"
)

func BaseRequest(requestID string, tool string) agentruntime.ToolExecutionRequest {
	return agentruntime.ToolExecutionRequest{
		ToolContractVersion: agentruntime.ToolContractVersionV1,
		RequestID:           strings.TrimSpace(requestID),
		Namespace:           "default",
		Agent:               "research-agent",
		Tool: agentruntime.ToolExecutionRequestTool{
			Name:      strings.TrimSpace(tool),
			Operation: agentruntime.ToolOperationInvoke,
		},
		InputRaw: fmt.Sprintf("tool=%s payload", strings.TrimSpace(tool)),
		Trace: agentruntime.ToolExecutionTraceContext{
			TraceID: "default/task/a001",
			SpanID:  strings.TrimSpace(requestID),
		},
		Attempt: 1,
	}
}

func UnknownVersionCase(requestID string, tool string) conformance.Case {
	req := BaseRequest(requestID, tool)
	req.ToolContractVersion = "v9"
	return conformance.Case{
		Name:     "unknown-version",
		Request:  req,
		Expected: UnknownVersionExpected(),
	}
}

func UnknownVersionExpected() conformance.Expected {
	return conformance.Expected{
		Status:    agentruntime.ToolExecutionStatusError,
		ErrorCode: agentruntime.ToolCodeInvalidInput,
		Reason:    agentruntime.ToolReasonInvalidInput,
		Retryable: BoolPtr(false),
	}
}

func ImmediateCancelCase(name string, req agentruntime.ToolExecutionRequest, maxLatency time.Duration, expected conformance.Expected) conformance.Case {
	return conformance.Case{
		Name:              strings.TrimSpace(name),
		Request:           req,
		CancelImmediately: true,
		MaxLatency:        maxLatency,
		Expected:          expected,
	}
}

func BoundedTimeoutCase(name string, req agentruntime.ToolExecutionRequest, callTimeout time.Duration, maxLatency time.Duration, expected conformance.Expected) conformance.Case {
	return conformance.Case{
		Name:        strings.TrimSpace(name),
		Request:     req,
		CallTimeout: callTimeout,
		MaxLatency:  maxLatency,
		Expected:    expected,
	}
}

func BoolPtr(value bool) *bool {
	return &value
}
