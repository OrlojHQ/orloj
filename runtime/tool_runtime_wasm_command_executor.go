package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// WASMCommandRunner executes wasm runtime binary commands.
type WASMCommandRunner interface {
	Run(ctx context.Context, binary string, args []string, stdin string, env map[string]string) (stdout string, stderr string, err error)
}

type osExecWASMCommandRunner struct{}

func (r *osExecWASMCommandRunner) Run(ctx context.Context, binary string, args []string, stdin string, env map[string]string) (string, string, error) {
	cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec
	cmd.Stdin = strings.NewReader(stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), mapToEnv(env)...)
	}
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

type WASMCommandExecutorFactory struct {
	Runner WASMCommandRunner
}

func NewWASMCommandExecutorFactory() *WASMCommandExecutorFactory {
	return &WASMCommandExecutorFactory{Runner: &osExecWASMCommandRunner{}}
}

func (f *WASMCommandExecutorFactory) Build(_ context.Context, cfg WASMToolRuntimeConfig) (WASMToolExecutor, error) {
	runner := f.Runner
	if runner == nil {
		runner = &osExecWASMCommandRunner{}
	}
	cfg = cfg.normalized()
	return &WASMCommandExecutor{
		config: cfg,
		runner: runner,
	}, nil
}

type WASMCommandExecutor struct {
	config WASMToolRuntimeConfig
	runner WASMCommandRunner
}

func (e *WASMCommandExecutor) Execute(ctx context.Context, req WASMToolExecuteRequest) (WASMToolExecuteResponse, error) {
	cfg := e.config.normalized()
	if strings.TrimSpace(cfg.ModulePath) == "" {
		return WASMToolExecuteResponse{}, NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			"wasm module_path is required",
			ErrInvalidToolRuntimePolicy,
			map[string]string{
				"isolation_mode": "wasm",
				"field":          "module_path",
			},
		)
	}
	binary := strings.TrimSpace(cfg.RuntimeBinary)
	if binary == "" {
		return WASMToolExecuteResponse{}, NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			"wasm runtime binary is required",
			ErrInvalidToolRuntimePolicy,
			map[string]string{
				"isolation_mode": "wasm",
				"field":          "runtime_binary",
			},
		)
	}

	args := make([]string, 0, len(cfg.RuntimeArgs)+4)
	args = append(args, "run")
	args = append(args, cfg.RuntimeArgs...)
	entrypoint := strings.TrimSpace(cfg.Entrypoint)
	if entrypoint != "" {
		args = append(args, "--invoke", entrypoint)
	}
	args = append(args, strings.TrimSpace(cfg.ModulePath))

	moduleReq := BuildWASMToolModuleRequest(req)
	payload, err := json.Marshal(moduleReq)
	if err != nil {
		return WASMToolExecuteResponse{}, NewToolError(
			ToolStatusError,
			ToolCodeInvalidInput,
			ToolReasonInvalidInput,
			false,
			"failed to encode wasm request payload",
			err,
			map[string]string{
				"isolation_mode": "wasm",
				"field":          "payload",
			},
		)
	}
	env := map[string]string{
		"ORLOJ_WASM_ENABLE_WASI":       strconv.FormatBool(cfg.EnableWASI),
		"ORLOJ_WASM_MAX_MEMORY_BYTES":  strconv.FormatInt(cfg.MaxMemoryBytes, 10),
		"ORLOJ_WASM_FUEL":              strconv.FormatUint(cfg.Fuel, 10),
		"ORLOJ_WASM_ENTRYPOINT":        entrypoint,
		"ORLOJ_WASM_TOOL":              strings.TrimSpace(req.Tool),
		"ORLOJ_WASM_NAMESPACE":         strings.TrimSpace(req.Namespace),
		"ORLOJ_WASM_CAPABILITIES_CSV":  strings.Join(req.Capabilities, ","),
		"ORLOJ_WASM_RUNTIME_BINARY":    binary,
		"ORLOJ_WASM_RUNTIME_ARGS_JSON": mustMarshalArgsJSON(cfg.RuntimeArgs),
	}
	stdout, stderr, runErr := e.runner.Run(ctx, binary, args, string(payload), env)
	if runErr != nil {
		lower := strings.ToLower(strings.TrimSpace(runErr.Error() + " " + stderr))
		if strings.Contains(lower, "executable file not found") || strings.Contains(lower, "no such file or directory") {
			return WASMToolExecuteResponse{}, NewToolError(
				ToolStatusError,
				ToolCodeRuntimePolicyInvalid,
				ToolReasonRuntimePolicyInvalid,
				false,
				fmt.Sprintf("wasm runtime binary %q is not available", binary),
				runErr,
				map[string]string{
					"isolation_mode": "wasm",
					"runtime_binary": binary,
				},
			)
		}
		return WASMToolExecuteResponse{}, NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("wasm command execution failed for tool=%s stderr=%s", strings.TrimSpace(req.Tool), RedactSensitive(compactStderr(stderr))),
			runErr,
			map[string]string{
				"isolation_mode": "wasm",
				"runtime_binary": binary,
				"module_path":    strings.TrimSpace(cfg.ModulePath),
			},
		)
	}
	moduleResp, decodeErr := DecodeWASMToolModuleResponse(stdout)
	if decodeErr != nil {
		code := ToolCodeExecutionFailed
		reason := ToolReasonBackendFailure
		retryable := true
		if IsWASMToolModuleContractError(decodeErr) {
			code = ToolCodeRuntimePolicyInvalid
			reason = ToolReasonRuntimePolicyInvalid
			retryable = false
		}
		return WASMToolExecuteResponse{}, NewToolError(
			ToolStatusError,
			code,
			reason,
			retryable,
			fmt.Sprintf("invalid wasm module response for tool=%s", strings.TrimSpace(req.Tool)),
			decodeErr,
			map[string]string{
				"isolation_mode": "wasm",
				"runtime_binary": binary,
				"module_path":    strings.TrimSpace(cfg.ModulePath),
			},
		)
	}

	switch moduleResp.Status {
	case wasmToolModuleStatusOK:
		return WASMToolExecuteResponse{Output: strings.TrimSpace(moduleResp.Output)}, nil
	case wasmToolModuleStatusDenied:
		return WASMToolExecuteResponse{}, wasmToolModuleFailureAsToolError(req.Tool, moduleResp, ToolStatusDenied)
	default:
		return WASMToolExecuteResponse{}, wasmToolModuleFailureAsToolError(req.Tool, moduleResp, ToolStatusError)
	}
}

func mustMarshalArgsJSON(args []string) string {
	data, err := json.Marshal(args)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func wasmToolModuleFailureAsToolError(tool string, response WASMToolModuleResponse, status string) error {
	defaultCode := ToolCodeExecutionFailed
	defaultReason := ToolReasonBackendFailure
	defaultRetryable := true
	if status == ToolStatusDenied {
		defaultCode = ToolCodePermissionDenied
		defaultReason = ToolReasonPermissionDenied
		defaultRetryable = false
	}

	code := defaultCode
	reason := defaultReason
	retryable := defaultRetryable
	message := "wasm module execution failed"
	details := map[string]string{
		"isolation_mode":   "wasm",
		"contract_version": strings.TrimSpace(response.ContractVersion),
		"module_status":    strings.TrimSpace(response.Status),
		"tool":             strings.TrimSpace(tool),
	}
	if response.Error != nil {
		if trimmed := strings.TrimSpace(response.Error.Code); trimmed != "" {
			code = trimmed
		}
		if trimmed := strings.TrimSpace(response.Error.Reason); trimmed != "" {
			reason = trimmed
		}
		retryable = response.Error.Retryable
		if trimmed := strings.TrimSpace(response.Error.Message); trimmed != "" {
			message = trimmed
		}
		for key, value := range response.Error.Details {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			details[key] = strings.TrimSpace(value)
		}
	}
	if strings.TrimSpace(tool) != "" {
		message = fmt.Sprintf("wasm module error for tool=%s: %s", strings.TrimSpace(tool), message)
	}
	return NewToolError(
		status,
		code,
		reason,
		retryable,
		message,
		nil,
		details,
	)
}
