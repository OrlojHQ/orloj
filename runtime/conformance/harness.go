package conformance

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
)

// Executor is a backend under tool-contract conformance verification.
type Executor interface {
	Execute(ctx context.Context, req agentruntime.ToolExecutionRequest) (agentruntime.ToolExecutionResponse, error)
}

type Case struct {
	Name              string
	Request           agentruntime.ToolExecutionRequest
	CallTimeout       time.Duration
	CancelImmediately bool
	MaxLatency        time.Duration
	Expected          Expected
}

type Expected struct {
	Status    string
	ErrorCode string
	Reason    string
	Retryable *bool
}

type Failure struct {
	Case string
	Err  error
}

func RunCases(ctx context.Context, executor Executor, cases []Case) []Failure {
	if executor == nil {
		return []Failure{{Case: "executor", Err: fmt.Errorf("executor is required")}}
	}
	failures := make([]Failure, 0, len(cases))
	for idx, item := range cases {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = fmt.Sprintf("case-%d", idx+1)
		}
		callCtx := ctx
		cancel := func() {}
		if item.CallTimeout > 0 {
			callCtx, cancel = context.WithTimeout(ctx, item.CallTimeout)
		}
		if item.CancelImmediately {
			var cancelNow context.CancelFunc
			callCtx, cancelNow = context.WithCancel(callCtx)
			cancelNow()
		}
		start := time.Now()
		response, err := executor.Execute(callCtx, item.Request)
		elapsed := time.Since(start)
		cancel()
		if err != nil {
			failures = append(failures, Failure{
				Case: name,
				Err:  fmt.Errorf("executor error: %w", err),
			})
			continue
		}
		if item.MaxLatency > 0 && elapsed > item.MaxLatency {
			failures = append(failures, Failure{
				Case: name,
				Err:  fmt.Errorf("max latency exceeded: elapsed=%s max=%s", elapsed, item.MaxLatency),
			})
			continue
		}
		if err := ValidateEnvelope(item.Request, response); err != nil {
			failures = append(failures, Failure{
				Case: name,
				Err:  err,
			})
			continue
		}
		if err := validateExpectations(response, item.Expected); err != nil {
			failures = append(failures, Failure{
				Case: name,
				Err:  err,
			})
			continue
		}
	}
	return failures
}

func ValidateEnvelope(req agentruntime.ToolExecutionRequest, response agentruntime.ToolExecutionResponse) error {
	if !strings.EqualFold(strings.TrimSpace(response.ToolContractVersion), agentruntime.ToolContractVersionV1) {
		return fmt.Errorf("unexpected tool contract version %q", response.ToolContractVersion)
	}
	if strings.TrimSpace(req.RequestID) != "" && strings.TrimSpace(response.RequestID) != strings.TrimSpace(req.RequestID) {
		return fmt.Errorf("request_id mismatch: expected %q got %q", strings.TrimSpace(req.RequestID), strings.TrimSpace(response.RequestID))
	}
	switch strings.ToLower(strings.TrimSpace(response.Status)) {
	case agentruntime.ToolExecutionStatusOK:
		if response.Error != nil {
			return fmt.Errorf("ok response must not include error envelope")
		}
	case agentruntime.ToolExecutionStatusError, agentruntime.ToolExecutionStatusDenied:
		if response.Error == nil {
			return fmt.Errorf("%s response missing error envelope", strings.TrimSpace(response.Status))
		}
		if strings.TrimSpace(response.Error.Code) == "" {
			return fmt.Errorf("%s response missing error.code", strings.TrimSpace(response.Status))
		}
		if strings.TrimSpace(response.Error.Reason) == "" {
			return fmt.Errorf("%s response missing error.reason", strings.TrimSpace(response.Status))
		}
		if strings.TrimSpace(response.Error.Message) == "" {
			return fmt.Errorf("%s response missing error.message", strings.TrimSpace(response.Status))
		}
	default:
		return fmt.Errorf("unknown response status %q", response.Status)
	}
	if response.Usage.Attempt <= 0 {
		return fmt.Errorf("usage.attempt must be positive")
	}
	if strings.TrimSpace(req.Trace.TraceID) != "" && strings.TrimSpace(response.Trace.TraceID) != strings.TrimSpace(req.Trace.TraceID) {
		return fmt.Errorf("trace.trace_id mismatch: expected %q got %q", strings.TrimSpace(req.Trace.TraceID), strings.TrimSpace(response.Trace.TraceID))
	}
	if strings.TrimSpace(req.Trace.SpanID) != "" && strings.TrimSpace(response.Trace.SpanID) != strings.TrimSpace(req.Trace.SpanID) {
		return fmt.Errorf("trace.span_id mismatch: expected %q got %q", strings.TrimSpace(req.Trace.SpanID), strings.TrimSpace(response.Trace.SpanID))
	}
	return nil
}

func validateExpectations(response agentruntime.ToolExecutionResponse, expected Expected) error {
	status := strings.ToLower(strings.TrimSpace(response.Status))
	expectedStatus := strings.ToLower(strings.TrimSpace(expected.Status))
	if expectedStatus != "" && status != expectedStatus {
		return fmt.Errorf("expected status=%s got %s", expectedStatus, status)
	}
	if response.Error == nil {
		return nil
	}
	if want := strings.TrimSpace(expected.ErrorCode); want != "" && !strings.EqualFold(strings.TrimSpace(response.Error.Code), want) {
		return fmt.Errorf("expected error.code=%s got %s", want, strings.TrimSpace(response.Error.Code))
	}
	if want := strings.TrimSpace(expected.Reason); want != "" && !strings.EqualFold(strings.TrimSpace(response.Error.Reason), want) {
		return fmt.Errorf("expected error.reason=%s got %s", want, strings.TrimSpace(response.Error.Reason))
	}
	if expected.Retryable != nil && response.Error.Retryable != *expected.Retryable {
		return fmt.Errorf("expected error.retryable=%t got %t", *expected.Retryable, response.Error.Retryable)
	}
	return nil
}
