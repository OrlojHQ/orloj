package agentruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type captureWASMCommandRunner struct {
	binary string
	args   []string
	stdin  string
	env    map[string]string

	stdout string
	stderr string
	err    error
	calls  int
}

func (r *captureWASMCommandRunner) Run(_ context.Context, binary string, args []string, stdin string, env map[string]string) (string, string, error) {
	r.calls++
	r.binary = binary
	r.args = append([]string(nil), args...)
	r.stdin = stdin
	r.env = copyStringMap(env)
	return r.stdout, r.stderr, r.err
}

func TestWASMCommandExecutorBuildAndExecute(t *testing.T) {
	runner := &captureWASMCommandRunner{stdout: "{\"contract_version\":\"v1\",\"status\":\"ok\",\"output\":\"ok\"}"}
	factory := &WASMCommandExecutorFactory{Runner: runner}
	exec, err := factory.Build(context.Background(), WASMToolRuntimeConfig{
		ModulePath:    "/tmp/tool.wasm",
		Entrypoint:    "run_tool",
		RuntimeBinary: "wasmtime",
		RuntimeArgs:   []string{"--dir", "/tmp"},
	})
	if err != nil {
		t.Fatalf("factory build failed: %v", err)
	}
	response, err := exec.Execute(context.Background(), WASMToolExecuteRequest{
		Namespace:    "default",
		Tool:         "wasm_tool",
		Input:        "payload",
		Capabilities: []string{"network.read"},
		RiskLevel:    "high",
		Runtime: WASMToolRuntimeConfig{
			ModulePath: "/tmp/tool.wasm",
		},
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if response.Output != "ok" {
		t.Fatalf("unexpected response output %q", response.Output)
	}
	if runner.calls != 1 {
		t.Fatalf("expected one runner call, got %d", runner.calls)
	}
	if runner.binary != "wasmtime" {
		t.Fatalf("expected runtime binary wasmtime, got %q", runner.binary)
	}
	if !sliceContains(runner.args, "run") || !sliceContains(runner.args, "--invoke") || !sliceContains(runner.args, "run_tool") || !sliceContains(runner.args, "/tmp/tool.wasm") {
		t.Fatalf("unexpected runtime args: %v", runner.args)
	}
	if !strings.Contains(runner.stdin, "\"tool\":\"wasm_tool\"") {
		t.Fatalf("expected stdin payload to include tool field, got %q", runner.stdin)
	}
	if !strings.Contains(runner.stdin, "\"contract_version\":\"v1\"") {
		t.Fatalf("expected stdin payload to include contract version, got %q", runner.stdin)
	}
}

func TestWASMCommandExecutorMissingModulePath(t *testing.T) {
	executor := &WASMCommandExecutor{
		config: WASMToolRuntimeConfig{
			ModulePath:    "",
			RuntimeBinary: "wasmtime",
		},
		runner: &captureWASMCommandRunner{stdout: "ok"},
	}
	_, err := executor.Execute(context.Background(), WASMToolExecuteRequest{Tool: "wasm_tool"})
	if err == nil {
		t.Fatal("expected runtime policy invalid error")
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeRuntimePolicyInvalid || reason != ToolReasonRuntimePolicyInvalid {
		t.Fatalf("unexpected metadata code=%s reason=%s", code, reason)
	}
	if retryable {
		t.Fatal("expected non-retryable runtime policy invalid")
	}
}

func TestWASMCommandExecutorMapsMissingBinaryAsPolicyError(t *testing.T) {
	executor := &WASMCommandExecutor{
		config: WASMToolRuntimeConfig{
			ModulePath:    "/tmp/tool.wasm",
			RuntimeBinary: "wasmtime",
		},
		runner: &captureWASMCommandRunner{
			err:    errors.New("exec: \"wasmtime\": executable file not found in $PATH"),
			stderr: "",
		},
	}
	_, err := executor.Execute(context.Background(), WASMToolExecuteRequest{Tool: "wasm_tool"})
	if err == nil {
		t.Fatal("expected runtime policy invalid error")
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeRuntimePolicyInvalid || reason != ToolReasonRuntimePolicyInvalid {
		t.Fatalf("unexpected metadata code=%s reason=%s", code, reason)
	}
	if retryable {
		t.Fatal("expected non-retryable runtime policy invalid")
	}
}

func TestWASMCommandExecutorDeniedResponse(t *testing.T) {
	executor := &WASMCommandExecutor{
		config: WASMToolRuntimeConfig{
			ModulePath:    "/tmp/tool.wasm",
			RuntimeBinary: "wasmtime",
		},
		runner: &captureWASMCommandRunner{
			stdout: "{\"contract_version\":\"v1\",\"status\":\"denied\",\"error\":{\"code\":\"permission_denied\",\"reason\":\"tool_permission_denied\",\"retryable\":false,\"message\":\"blocked by policy\"}}",
		},
	}
	_, err := executor.Execute(context.Background(), WASMToolExecuteRequest{Tool: "wasm_tool"})
	if err == nil {
		t.Fatal("expected denied response to return error")
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodePermissionDenied || reason != ToolReasonPermissionDenied {
		t.Fatalf("unexpected metadata code=%s reason=%s", code, reason)
	}
	if retryable {
		t.Fatal("expected denied response to be non-retryable")
	}
}

func TestWASMCommandExecutorInvalidResponseContractVersion(t *testing.T) {
	executor := &WASMCommandExecutor{
		config: WASMToolRuntimeConfig{
			ModulePath:    "/tmp/tool.wasm",
			RuntimeBinary: "wasmtime",
		},
		runner: &captureWASMCommandRunner{
			stdout: "{\"contract_version\":\"v2\",\"status\":\"ok\",\"output\":\"ok\"}",
		},
	}
	_, err := executor.Execute(context.Background(), WASMToolExecuteRequest{Tool: "wasm_tool"})
	if err == nil {
		t.Fatal("expected invalid contract version error")
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeRuntimePolicyInvalid || reason != ToolReasonRuntimePolicyInvalid {
		t.Fatalf("unexpected metadata code=%s reason=%s", code, reason)
	}
	if retryable {
		t.Fatal("expected invalid contract version to be non-retryable")
	}
}
