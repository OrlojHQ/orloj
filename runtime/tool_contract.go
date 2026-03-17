package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	ToolContractVersionV1 = "v1"
	ToolOperationInvoke   = "invoke"
)

const (
	ToolExecutionStatusOK     = "ok"
	ToolExecutionStatusError  = ToolStatusError
	ToolExecutionStatusDenied = ToolStatusDenied
)

// ToolExecutionRequest is the strict runtime contract envelope for one tool call.
type ToolExecutionRequest struct {
	ToolContractVersion string                    `json:"tool_contract_version,omitempty"`
	RequestID           string                    `json:"request_id,omitempty"`
	TaskID              string                    `json:"task_id,omitempty"`
	Namespace           string                    `json:"namespace,omitempty"`
	Agent               string                    `json:"agent,omitempty"`
	Tool                ToolExecutionRequestTool  `json:"tool,omitempty"`
	Input               map[string]string         `json:"input,omitempty"`
	InputRaw            string                    `json:"input_raw,omitempty"`
	Runtime             ToolExecutionRuntime      `json:"runtime,omitempty"`
	Auth                ToolExecutionAuth         `json:"auth,omitempty"`
	Trace               ToolExecutionTraceContext `json:"trace,omitempty"`
	Attempt             int                       `json:"attempt,omitempty"`
}

type ToolExecutionRequestTool struct {
	Name         string   `json:"name,omitempty"`
	Operation    string   `json:"operation,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	RiskLevel    string   `json:"risk_level,omitempty"`
}

type ToolExecutionRuntime struct {
	Mode         string `json:"mode,omitempty"`
	TimeoutMS    int    `json:"timeout_ms,omitempty"`
	MaxAttempts  int    `json:"max_attempts,omitempty"`
	Backoff      string `json:"backoff,omitempty"`
	MaxBackoffMS int    `json:"max_backoff_ms,omitempty"`
	Jitter       bool   `json:"jitter,omitempty"`
}

type ToolExecutionAuth struct {
	Profile   string   `json:"profile,omitempty"`
	SecretRef string   `json:"secret_ref,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
}

type ToolExecutionTraceContext struct {
	TraceID string `json:"trace_id,omitempty"`
	SpanID  string `json:"span_id,omitempty"`
}

type ToolExecutionResponse struct {
	ToolContractVersion string                    `json:"tool_contract_version,omitempty"`
	RequestID           string                    `json:"request_id,omitempty"`
	Status              string                    `json:"status,omitempty"`
	Output              ToolExecutionOutput       `json:"output,omitempty"`
	Usage               ToolExecutionUsage        `json:"usage,omitempty"`
	Trace               ToolExecutionTraceContext `json:"trace,omitempty"`
	Error               *ToolExecutionFailure     `json:"error,omitempty"`
}

type ToolExecutionOutput struct {
	Result string `json:"result,omitempty"`
}

type ToolExecutionUsage struct {
	DurationMS int64 `json:"duration_ms,omitempty"`
	Attempt    int   `json:"attempt,omitempty"`
}

type ToolExecutionFailure struct {
	Code      string            `json:"code,omitempty"`
	Reason    string            `json:"reason,omitempty"`
	Retryable bool              `json:"retryable"`
	Message   string            `json:"message,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

// ToolContractExecutor executes tools against the v1 tool request/response contract.
type ToolContractExecutor interface {
	Execute(ctx context.Context, req ToolExecutionRequest) (ToolExecutionResponse, error)
}

type toolContractAdapter struct {
	runtime ToolRuntime
}

func NewToolContractExecutor(runtime ToolRuntime) ToolContractExecutor {
	if runtime == nil {
		runtime = NewHTTPToolClient(nil, nil, nil)
	}
	return &toolContractAdapter{runtime: runtime}
}

func ExecuteToolContract(ctx context.Context, runtime ToolRuntime, req ToolExecutionRequest) (ToolExecutionResponse, error) {
	return NewToolContractExecutor(runtime).Execute(ctx, req)
}

func (a *toolContractAdapter) Execute(ctx context.Context, req ToolExecutionRequest) (ToolExecutionResponse, error) {
	start := time.Now()
	normalized, err := NormalizeToolExecutionRequest(req)
	if err != nil {
		response := toolExecutionResponseFromError(req, err, time.Since(start))
		return response, nil
	}
	if direct, ok := a.runtime.(ToolContractExecutor); ok {
		response, execErr := direct.Execute(ctx, normalized)
		if execErr != nil {
			return toolExecutionResponseFromError(normalized, execErr, time.Since(start)), nil
		}
		return normalizeToolExecutionResponse(normalized, response, time.Since(start)), nil
	}
	input, inputErr := normalized.EncodedInput()
	if inputErr != nil {
		response := toolExecutionResponseFromError(normalized, inputErr, time.Since(start))
		return response, nil
	}
	output, callErr := a.runtime.Call(ctx, normalized.Tool.Name, input)
	if callErr != nil {
		return toolExecutionResponseFromError(normalized, callErr, time.Since(start)), nil
	}
	return ToolExecutionResponse{
		ToolContractVersion: normalized.ToolContractVersion,
		RequestID:           normalized.RequestID,
		Status:              ToolExecutionStatusOK,
		Output: ToolExecutionOutput{
			Result: output,
		},
		Usage: ToolExecutionUsage{
			DurationMS: time.Since(start).Milliseconds(),
			Attempt:    normalized.Attempt,
		},
		Trace: normalized.Trace,
	}, nil
}

func NormalizeToolExecutionRequest(req ToolExecutionRequest) (ToolExecutionRequest, error) {
	req.ToolContractVersion = strings.ToLower(strings.TrimSpace(req.ToolContractVersion))
	if req.ToolContractVersion == "" {
		req.ToolContractVersion = ToolContractVersionV1
	}
	if req.ToolContractVersion != ToolContractVersionV1 {
		return req, NewToolError(
			ToolStatusError,
			ToolCodeInvalidInput,
			ToolReasonInvalidInput,
			false,
			fmt.Sprintf("unsupported tool contract version %q", req.ToolContractVersion),
			nil,
			map[string]string{
				"field":    "tool_contract_version",
				"expected": ToolContractVersionV1,
			},
		)
	}
	req.RequestID = strings.TrimSpace(req.RequestID)
	if req.RequestID == "" {
		return req, NewToolError(
			ToolStatusError,
			ToolCodeInvalidInput,
			ToolReasonInvalidInput,
			false,
			"missing request_id",
			nil,
			map[string]string{"field": "request_id"},
		)
	}
	req.TaskID = strings.TrimSpace(req.TaskID)
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Agent = strings.TrimSpace(req.Agent)
	req.Tool.Name = strings.TrimSpace(req.Tool.Name)
	if req.Tool.Name == "" {
		return req, NewToolError(
			ToolStatusError,
			ToolCodeInvalidInput,
			ToolReasonInvalidInput,
			false,
			"missing tool.name",
			nil,
			map[string]string{"field": "tool.name"},
		)
	}
	req.Tool.Operation = strings.ToLower(strings.TrimSpace(req.Tool.Operation))
	if req.Tool.Operation == "" {
		req.Tool.Operation = ToolOperationInvoke
	}
	req.Tool.Capabilities = normalizeToolContractLabels(req.Tool.Capabilities)
	req.Tool.RiskLevel = strings.ToLower(strings.TrimSpace(req.Tool.RiskLevel))
	if req.Attempt <= 0 {
		req.Attempt = 1
	}
	req.Runtime.Mode = strings.ToLower(strings.TrimSpace(req.Runtime.Mode))
	req.Runtime.Backoff = strings.ToLower(strings.TrimSpace(req.Runtime.Backoff))
	req.Auth.Profile = strings.ToLower(strings.TrimSpace(req.Auth.Profile))
	req.Auth.SecretRef = strings.TrimSpace(req.Auth.SecretRef)
	req.Auth.Scopes = normalizeToolContractLabels(req.Auth.Scopes)
	req.Trace.TraceID = strings.TrimSpace(req.Trace.TraceID)
	req.Trace.SpanID = strings.TrimSpace(req.Trace.SpanID)
	req.InputRaw = strings.TrimSpace(req.InputRaw)
	return req, nil
}

func (r ToolExecutionRequest) EncodedInput() (string, error) {
	if strings.TrimSpace(r.InputRaw) != "" {
		return strings.TrimSpace(r.InputRaw), nil
	}
	if len(r.Input) == 0 {
		return "", nil
	}
	data, err := json.Marshal(r.Input)
	if err != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeInvalidInput,
			ToolReasonInvalidInput,
			false,
			"failed to encode tool input",
			err,
			map[string]string{"field": "input"},
		)
	}
	return string(data), nil
}

func (r ToolExecutionResponse) ToError() error {
	status := strings.ToLower(strings.TrimSpace(r.Status))
	if status == "" || status == ToolExecutionStatusOK {
		return nil
	}
	failure := r.Error
	if failure == nil {
		return NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			false,
			"tool response missing error envelope",
			nil,
			nil,
		)
	}
	toolStatus := ToolStatusError
	if status == ToolExecutionStatusDenied {
		toolStatus = ToolStatusDenied
	}
	return NewToolError(
		toolStatus,
		strings.TrimSpace(failure.Code),
		strings.TrimSpace(failure.Reason),
		failure.Retryable,
		strings.TrimSpace(failure.Message),
		nil,
		failure.Details,
	)
}

func normalizeToolExecutionResponse(req ToolExecutionRequest, resp ToolExecutionResponse, duration time.Duration) ToolExecutionResponse {
	resp.ToolContractVersion = ToolContractVersionV1
	resp.RequestID = req.RequestID
	if strings.TrimSpace(resp.Status) == "" {
		resp.Status = ToolExecutionStatusOK
	}
	if resp.Usage.Attempt <= 0 {
		resp.Usage.Attempt = req.Attempt
	}
	if resp.Usage.DurationMS <= 0 {
		resp.Usage.DurationMS = duration.Milliseconds()
	}
	if strings.TrimSpace(resp.Trace.TraceID) == "" {
		resp.Trace.TraceID = req.Trace.TraceID
	}
	if strings.TrimSpace(resp.Trace.SpanID) == "" {
		resp.Trace.SpanID = req.Trace.SpanID
	}
	return resp
}

func toolExecutionResponseFromError(req ToolExecutionRequest, err error, duration time.Duration) ToolExecutionResponse {
	status := ToolExecutionStatusError
	code := ToolCodeExecutionFailed
	reason := ToolReasonBackendFailure
	retryable := true
	message := strings.TrimSpace(err.Error())
	details := map[string]string(nil)

	if toolErr, ok := AsToolError(err); ok {
		if strings.EqualFold(strings.TrimSpace(toolErr.Status), ToolStatusDenied) {
			status = ToolExecutionStatusDenied
		}
		code = firstNonEmpty(strings.TrimSpace(toolErr.Code), code)
		reason = firstNonEmpty(strings.TrimSpace(toolErr.Reason), reason)
		retryable = toolErr.Retryable
		message = firstNonEmpty(strings.TrimSpace(toolErr.Message), message)
		details = copyToolDetailMap(toolErr.Details)
	} else {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			code = ToolCodeTimeout
			reason = ToolReasonExecutionTimeout
			retryable = true
		case errors.Is(err, context.Canceled):
			code = ToolCodeCanceled
			reason = ToolReasonExecutionCanceled
			retryable = false
		}
	}
	if message == "" {
		message = "tool execution failed"
	}
	return ToolExecutionResponse{
		ToolContractVersion: ToolContractVersionV1,
		RequestID:           strings.TrimSpace(req.RequestID),
		Status:              status,
		Usage: ToolExecutionUsage{
			DurationMS: duration.Milliseconds(),
			Attempt:    max(req.Attempt, 1),
		},
		Trace: req.Trace,
		Error: &ToolExecutionFailure{
			Code:      code,
			Reason:    reason,
			Retryable: retryable,
			Message:   message,
			Details:   details,
		},
	}
}

func normalizeToolContractLabels(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
