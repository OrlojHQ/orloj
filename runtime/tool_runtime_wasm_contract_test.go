package agentruntime

import (
	"strings"
	"testing"
)

func TestBuildWASMToolModuleRequest(t *testing.T) {
	req := BuildWASMToolModuleRequest(WASMToolExecuteRequest{
		Namespace:    "default",
		Tool:         "wasm_tool",
		Input:        "{\"q\":\"hi\"}",
		Capabilities: []string{"web.search", "web.search", " vector.db.invoke "},
		RiskLevel:    "HIGH",
		Runtime: WASMToolRuntimeConfig{
			Entrypoint:     "run_tool",
			MaxMemoryBytes: 32 * 1024 * 1024,
			Fuel:           1000,
			EnableWASI:     true,
		},
	})
	if req.ContractVersion != WASMToolModuleContractVersionV1 {
		t.Fatalf("unexpected contract version %q", req.ContractVersion)
	}
	if req.Tool != "wasm_tool" {
		t.Fatalf("unexpected tool %q", req.Tool)
	}
	if req.RiskLevel != "high" {
		t.Fatalf("unexpected risk level %q", req.RiskLevel)
	}
	if len(req.Capabilities) != 2 {
		t.Fatalf("expected deduped capabilities length 2, got %d (%v)", len(req.Capabilities), req.Capabilities)
	}
	if req.Runtime.Entrypoint != "run_tool" {
		t.Fatalf("unexpected runtime entrypoint %q", req.Runtime.Entrypoint)
	}
}

func TestDecodeWASMToolModuleResponse(t *testing.T) {
	resp, err := DecodeWASMToolModuleResponse(`{"contract_version":"v1","status":"ok","output":"ok"}`)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if resp.Status != wasmToolModuleStatusOK {
		t.Fatalf("unexpected status %q", resp.Status)
	}
	if strings.TrimSpace(resp.Output) != "ok" {
		t.Fatalf("unexpected output %q", resp.Output)
	}
}

func TestDecodeWASMToolModuleResponseInvalidVersion(t *testing.T) {
	_, err := DecodeWASMToolModuleResponse(`{"contract_version":"v2","status":"ok","output":"ok"}`)
	if err == nil {
		t.Fatal("expected contract error")
	}
	if !IsWASMToolModuleContractError(err) {
		t.Fatalf("expected contract error classification, got %v", err)
	}
}

func TestDecodeWASMToolModuleResponseInvalidStatus(t *testing.T) {
	_, err := DecodeWASMToolModuleResponse(`{"contract_version":"v1","status":"weird","output":"ok"}`)
	if err == nil {
		t.Fatal("expected contract error")
	}
	if !IsWASMToolModuleContractError(err) {
		t.Fatalf("expected contract error classification, got %v", err)
	}
}

func TestDecodeWASMToolModuleResponseErrorStatus(t *testing.T) {
	raw := `{"contract_version":"v1","status":"error","error":{"code":"rate_limited","reason":"throttled","retryable":true,"message":"slow down"}}`
	resp, err := DecodeWASMToolModuleResponse(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if resp.Status != wasmToolModuleStatusError {
		t.Fatalf("expected error status, got %q", resp.Status)
	}
	if resp.Error == nil {
		t.Fatal("expected error object")
	}
	if resp.Error.Code != "rate_limited" {
		t.Fatalf("expected code 'rate_limited', got %q", resp.Error.Code)
	}
	if resp.Error.Reason != "throttled" {
		t.Fatalf("expected reason 'throttled', got %q", resp.Error.Reason)
	}
	if !resp.Error.Retryable {
		t.Fatal("expected retryable=true")
	}
	if resp.Error.Message != "slow down" {
		t.Fatalf("expected message 'slow down', got %q", resp.Error.Message)
	}
}

func TestDecodeWASMToolModuleResponseDeniedStatus(t *testing.T) {
	raw := `{"contract_version":"v1","status":"denied","error":{"code":"permission_denied","reason":"blocked","retryable":false}}`
	resp, err := DecodeWASMToolModuleResponse(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if resp.Status != wasmToolModuleStatusDenied {
		t.Fatalf("expected denied status, got %q", resp.Status)
	}
	if resp.Error == nil || resp.Error.Code != "permission_denied" {
		t.Fatal("expected permission_denied error code")
	}
	if resp.Error.Retryable {
		t.Fatal("expected non-retryable")
	}
}

func TestDecodeWASMToolModuleResponseEmptyString(t *testing.T) {
	_, err := DecodeWASMToolModuleResponse("")
	if err == nil {
		t.Fatal("expected contract error for empty response")
	}
	if !IsWASMToolModuleContractError(err) {
		t.Fatalf("expected contract error classification, got %v", err)
	}
}

func TestDecodeWASMToolModuleResponseInvalidJSON(t *testing.T) {
	_, err := DecodeWASMToolModuleResponse("not json at all")
	if err == nil {
		t.Fatal("expected contract error for invalid JSON")
	}
	if !IsWASMToolModuleContractError(err) {
		t.Fatalf("expected contract error classification, got %v", err)
	}
}

func TestDecodeWASMToolModuleResponseMissingVersion(t *testing.T) {
	_, err := DecodeWASMToolModuleResponse(`{"status":"ok","output":"hi"}`)
	if err == nil {
		t.Fatal("expected contract error for missing version")
	}
	if !IsWASMToolModuleContractError(err) {
		t.Fatalf("expected contract error classification, got %v", err)
	}
}

func TestBuildWASMToolModuleRequestAuthPropagation(t *testing.T) {
	req := BuildWASMToolModuleRequest(WASMToolExecuteRequest{
		Namespace:   "production",
		Tool:        "secure_tool",
		Input:       "payload",
		AuthProfile: "bearer",
		AuthHeaders: map[string]string{"Authorization": "Bearer sk-test-123"},
		Runtime: WASMToolRuntimeConfig{
			Entrypoint:     "run",
			MaxMemoryBytes: 64 * 1024 * 1024,
			Fuel:           1000000,
			EnableWASI:     true,
		},
	})
	if req.Auth.Profile != "bearer" {
		t.Fatalf("expected auth profile 'bearer', got %q", req.Auth.Profile)
	}
	if req.Auth.Headers == nil {
		t.Fatal("expected auth headers to be populated")
	}
	if req.Auth.Headers["Authorization"] != "Bearer sk-test-123" {
		t.Fatalf("expected Authorization header, got %q", req.Auth.Headers["Authorization"])
	}
}

func TestWasmToolModuleFailureAsToolError(t *testing.T) {
	resp := WASMToolModuleResponse{
		ContractVersion: "v1",
		Status:          "error",
		Error: &ToolExecutionFailure{
			Code:      "rate_limited",
			Reason:    "upstream throttled",
			Retryable: true,
			Message:   "try later",
			Details:   map[string]string{"endpoint": "api.example.com"},
		},
	}
	err := wasmToolModuleFailureAsToolError("my-tool", resp, ToolStatusError)
	if err == nil {
		t.Fatal("expected error")
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != "rate_limited" {
		t.Fatalf("expected code 'rate_limited', got %q", code)
	}
	if reason != "upstream throttled" {
		t.Fatalf("expected reason 'upstream throttled', got %q", reason)
	}
	if !retryable {
		t.Fatal("expected retryable")
	}
}

func TestWasmToolModuleFailureAsToolErrorDefaults(t *testing.T) {
	resp := WASMToolModuleResponse{
		ContractVersion: "v1",
		Status:          "error",
	}
	err := wasmToolModuleFailureAsToolError("my-tool", resp, ToolStatusError)
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeExecutionFailed {
		t.Fatalf("expected default code %q, got %q", ToolCodeExecutionFailed, code)
	}
	if reason != ToolReasonBackendFailure {
		t.Fatalf("expected default reason %q, got %q", ToolReasonBackendFailure, reason)
	}
	if !retryable {
		t.Fatal("expected default retryable=true for error status")
	}
}

func TestWasmToolModuleFailureAsToolErrorDenied(t *testing.T) {
	resp := WASMToolModuleResponse{
		ContractVersion: "v1",
		Status:          "denied",
	}
	err := wasmToolModuleFailureAsToolError("my-tool", resp, ToolStatusDenied)
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodePermissionDenied {
		t.Fatalf("expected denied code %q, got %q", ToolCodePermissionDenied, code)
	}
	if reason != ToolReasonPermissionDenied {
		t.Fatalf("expected denied reason %q, got %q", ToolReasonPermissionDenied, reason)
	}
	if retryable {
		t.Fatal("expected non-retryable for denied")
	}
}
