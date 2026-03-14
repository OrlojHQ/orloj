package agentruntime

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	ToolStatusError  = "error"
	ToolStatusDenied = "denied"
)

const (
	ToolCodeInvalidInput         = "invalid_input"
	ToolCodeUnsupportedTool      = "unsupported_tool"
	ToolCodeRuntimePolicyInvalid = "runtime_policy_invalid"
	ToolCodeIsolationUnavailable = "isolation_unavailable"
	ToolCodePermissionDenied     = "permission_denied"
	ToolCodeSecretResolution     = "secret_resolution_failed"
	ToolCodeTimeout              = "timeout"
	ToolCodeCanceled             = "canceled"
	ToolCodeExecutionFailed      = "execution_failed"
)

const (
	ToolReasonInvalidInput         = "tool_invalid_input"
	ToolReasonToolUnsupported      = "tool_unsupported"
	ToolReasonRuntimePolicyInvalid = "tool_runtime_policy_invalid"
	ToolReasonIsolationUnavailable = "tool_isolation_unavailable"
	ToolReasonPermissionDenied     = "tool_permission_denied"
	ToolReasonSecretResolution     = "tool_secret_resolution_failed"
	ToolReasonExecutionTimeout     = "tool_execution_timeout"
	ToolReasonExecutionCanceled    = "tool_execution_canceled"
	ToolReasonBackendFailure       = "tool_backend_failure"
)

// ToolError is the canonical runtime tool error envelope.
// It is serialized into deterministic message text so controllers/UI can parse stable fields.
type ToolError struct {
	Status    string
	Code      string
	Reason    string
	Retryable bool
	Message   string
	Details   map[string]string
	Cause     error
}

func (e *ToolError) Error() string {
	if e == nil {
		return ""
	}
	status := strings.TrimSpace(strings.ToLower(e.Status))
	if status == "" {
		status = ToolStatusError
	}
	code := strings.TrimSpace(strings.ToLower(e.Code))
	if code == "" {
		code = ToolCodeExecutionFailed
	}
	reason := strings.TrimSpace(strings.ToLower(e.Reason))
	if reason == "" {
		reason = ToolReasonBackendFailure
	}
	msg := strings.TrimSpace(e.Message)
	if msg == "" && e.Cause != nil {
		msg = strings.TrimSpace(e.Cause.Error())
	}
	if msg == "" {
		msg = "tool execution failed"
	}

	parts := []string{
		fmt.Sprintf("tool_status=%s", status),
		fmt.Sprintf("tool_code=%s", code),
		fmt.Sprintf("tool_reason=%s", reason),
		fmt.Sprintf("retryable=%t", e.Retryable),
		fmt.Sprintf("message=%s", msg),
	}
	for _, detail := range formatToolDetails(e.Details) {
		parts = append(parts, detail)
	}
	return strings.Join(parts, " ")
}

func (e *ToolError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewToolError(
	status string,
	code string,
	reason string,
	retryable bool,
	message string,
	cause error,
	details map[string]string,
) error {
	status = strings.TrimSpace(strings.ToLower(status))
	if status != ToolStatusDenied {
		status = ToolStatusError
	}
	code = strings.TrimSpace(strings.ToLower(code))
	if code == "" {
		code = ToolCodeExecutionFailed
	}
	reason = strings.TrimSpace(strings.ToLower(reason))
	if reason == "" {
		reason = ToolReasonBackendFailure
	}
	return &ToolError{
		Status:    status,
		Code:      code,
		Reason:    reason,
		Retryable: retryable,
		Message:   strings.TrimSpace(message),
		Details:   copyToolDetailMap(details),
		Cause:     cause,
	}
}

func NewToolDeniedError(message string, details map[string]string, cause error) error {
	return NewToolError(
		ToolStatusDenied,
		ToolCodePermissionDenied,
		ToolReasonPermissionDenied,
		false,
		message,
		cause,
		details,
	)
}

func AsToolError(err error) (*ToolError, bool) {
	if err == nil {
		return nil, false
	}
	var target *ToolError
	if errors.As(err, &target) && target != nil {
		return target, true
	}
	return nil, false
}

func IsToolDeniedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrToolPermissionDenied) {
		return true
	}
	toolErr, ok := AsToolError(err)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(toolErr.Status), ToolStatusDenied) ||
		strings.EqualFold(strings.TrimSpace(toolErr.Code), ToolCodePermissionDenied) ||
		strings.EqualFold(strings.TrimSpace(toolErr.Reason), ToolReasonPermissionDenied)
}

func ToolErrorMeta(err error) (code string, reason string, retryable bool, ok bool) {
	toolErr, ok := AsToolError(err)
	if !ok {
		return "", "", false, false
	}
	return strings.TrimSpace(toolErr.Code), strings.TrimSpace(toolErr.Reason), toolErr.Retryable, true
}

func formatToolDetails(details map[string]string) []string {
	if len(details) == 0 {
		return nil
	}
	keys := make([]string, 0, len(details))
	for key := range details {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(details[key])
		if value == "" {
			continue
		}
		out = append(out, fmt.Sprintf("%s=%s", key, value))
	}
	return out
}

func copyToolDetailMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
