package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

type WASMToolRuntimeConfig struct {
	ModulePath     string
	Entrypoint     string
	MaxMemoryBytes int64
	Fuel           uint64
	EnableWASI     bool
	RuntimeBinary  string
	RuntimeArgs    []string
}

func DefaultWASMToolRuntimeConfig() WASMToolRuntimeConfig {
	return WASMToolRuntimeConfig{
		Entrypoint:     "run",
		MaxMemoryBytes: 64 * 1024 * 1024,
		EnableWASI:     true,
		RuntimeBinary:  "wasmtime",
	}
}

func (c WASMToolRuntimeConfig) normalized() WASMToolRuntimeConfig {
	out := c
	defaults := DefaultWASMToolRuntimeConfig()
	out.ModulePath = strings.TrimSpace(out.ModulePath)
	out.Entrypoint = strings.TrimSpace(out.Entrypoint)
	if out.Entrypoint == "" {
		out.Entrypoint = defaults.Entrypoint
	}
	if out.MaxMemoryBytes <= 0 {
		out.MaxMemoryBytes = defaults.MaxMemoryBytes
	}
	if out.Fuel == 0 {
		out.Fuel = defaults.Fuel
	}
	if !out.EnableWASI &&
		strings.TrimSpace(out.ModulePath) == "" &&
		strings.TrimSpace(c.ModulePath) == "" &&
		strings.TrimSpace(c.Entrypoint) == "" &&
		c.MaxMemoryBytes == 0 &&
		c.Fuel == 0 &&
		strings.TrimSpace(c.RuntimeBinary) == "" &&
		len(c.RuntimeArgs) == 0 {
		out.EnableWASI = defaults.EnableWASI
	}
	out.RuntimeBinary = strings.TrimSpace(out.RuntimeBinary)
	if out.RuntimeBinary == "" {
		out.RuntimeBinary = defaults.RuntimeBinary
	}
	if len(out.RuntimeArgs) > 0 {
		normalized := make([]string, 0, len(out.RuntimeArgs))
		for _, arg := range out.RuntimeArgs {
			arg = strings.TrimSpace(arg)
			if arg == "" {
				continue
			}
			normalized = append(normalized, arg)
		}
		out.RuntimeArgs = normalized
	}
	return out
}

// WASMToolExecuteRequest is the portable execution envelope used by wasm executors.
type WASMToolExecuteRequest struct {
	Namespace    string
	Tool         string
	Input        string
	Capabilities []string
	RiskLevel    string
	Runtime      WASMToolRuntimeConfig
}

type WASMToolExecuteResponse struct {
	Output string
}

// WASMToolExecutor is a pluggable wasm execution adapter.
type WASMToolExecutor interface {
	Execute(ctx context.Context, req WASMToolExecuteRequest) (WASMToolExecuteResponse, error)
}

// WASMToolExecutorFactory creates wasm executors from runtime config.
type WASMToolExecutorFactory interface {
	Build(ctx context.Context, cfg WASMToolRuntimeConfig) (WASMToolExecutor, error)
}

// WASMToolRuntime is a scaffold runtime for wasm-backed tool execution.
type WASMToolRuntime struct {
	registry  ToolCapabilityRegistry
	executor  WASMToolExecutor
	factory   WASMToolExecutorFactory
	config    WASMToolRuntimeConfig
	buildMu   sync.Mutex
	buildErr  error
	namespace string
}

func NewWASMToolRuntime(registry ToolCapabilityRegistry, executor WASMToolExecutor) *WASMToolRuntime {
	return &WASMToolRuntime{
		registry: registry,
		executor: executor,
		config:   DefaultWASMToolRuntimeConfig(),
	}
}

func NewWASMToolRuntimeWithFactory(
	registry ToolCapabilityRegistry,
	factory WASMToolExecutorFactory,
	config WASMToolRuntimeConfig,
) *WASMToolRuntime {
	return &WASMToolRuntime{
		registry: registry,
		factory:  factory,
		config:   config.normalized(),
	}
}

func (r *WASMToolRuntime) WithRegistry(registry ToolCapabilityRegistry) ToolRuntime {
	if r == nil {
		return NewWASMToolRuntime(registry, nil)
	}
	return &WASMToolRuntime{
		registry:  registry,
		executor:  r.executor,
		factory:   r.factory,
		config:    r.config,
		namespace: r.namespace,
	}
}

func (r *WASMToolRuntime) WithNamespace(namespace string) ToolRuntime {
	if r == nil {
		return NewWASMToolRuntime(nil, nil)
	}
	return &WASMToolRuntime{
		registry:  r.registry,
		executor:  r.executor,
		factory:   r.factory,
		config:    r.config,
		namespace: strings.TrimSpace(namespace),
	}
}

func (r *WASMToolRuntime) WithConfig(config WASMToolRuntimeConfig) *WASMToolRuntime {
	if r == nil {
		return NewWASMToolRuntimeWithFactory(nil, nil, config)
	}
	return &WASMToolRuntime{
		registry:  r.registry,
		executor:  r.executor,
		factory:   r.factory,
		config:    config.normalized(),
		namespace: r.namespace,
	}
}

func (r *WASMToolRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeInvalidInput,
			ToolReasonInvalidInput,
			false,
			"missing tool name",
			ErrInvalidToolRuntimePolicy,
			map[string]string{"field": "tool"},
		)
	}
	if err := ctx.Err(); err != nil {
		return "", mapWASMContextError(tool, err)
	}
	if r == nil || r.registry == nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("missing tool registry for wasm runtime tool=%s", tool),
			ErrInvalidToolRuntimePolicy,
			map[string]string{
				"tool":           tool,
				"isolation_mode": "wasm",
			},
		)
	}
	spec, ok := r.registry.Resolve(tool)
	if !ok {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeUnsupportedTool,
			ToolReasonToolUnsupported,
			false,
			fmt.Sprintf("unsupported tool %s", tool),
			ErrUnsupportedTool,
			map[string]string{
				"tool":           tool,
				"isolation_mode": "wasm",
			},
		)
	}
	executor, execErr := r.resolveExecutor(ctx)
	if execErr != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("wasm executor initialization failed for tool=%s", tool),
			execErr,
			map[string]string{
				"tool":           tool,
				"isolation_mode": "wasm",
				"module_path":    strings.TrimSpace(r.config.ModulePath),
				"entrypoint":     strings.TrimSpace(r.config.Entrypoint),
			},
		)
	}
	if executor == nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeIsolationUnavailable,
			ToolReasonIsolationUnavailable,
			false,
			fmt.Sprintf("wasm isolation runtime unavailable for tool=%s", tool),
			ErrToolIsolationUnavailable,
			map[string]string{
				"tool":           tool,
				"isolation_mode": "wasm",
				"module_path":    strings.TrimSpace(r.config.ModulePath),
				"entrypoint":     strings.TrimSpace(r.config.Entrypoint),
			},
		)
	}
	response, err := executeWASMToolBounded(ctx, executor, WASMToolExecuteRequest{
		Namespace:    strings.TrimSpace(r.namespace),
		Tool:         tool,
		Input:        input,
		Capabilities: append([]string(nil), spec.Capabilities...),
		RiskLevel:    strings.ToLower(strings.TrimSpace(spec.RiskLevel)),
		Runtime:      r.config.normalized(),
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", mapWASMContextError(tool, err)
		}
		if _, ok := AsToolError(err); ok {
			return "", err
		}
		return "", NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("wasm tool execution failed for tool=%s", tool),
			err,
			map[string]string{
				"tool":           tool,
				"isolation_mode": "wasm",
			},
		)
	}
	return strings.TrimSpace(response.Output), nil
}

func (r *WASMToolRuntime) resolveExecutor(ctx context.Context) (WASMToolExecutor, error) {
	if r == nil {
		return nil, nil
	}
	if r.executor != nil {
		return r.executor, nil
	}
	if r.factory == nil {
		return nil, nil
	}
	r.buildMu.Lock()
	defer r.buildMu.Unlock()
	if r.executor != nil {
		return r.executor, nil
	}
	if r.buildErr != nil {
		return nil, r.buildErr
	}
	config := r.config.normalized()
	if strings.TrimSpace(config.ModulePath) == "" {
		r.buildErr = fmt.Errorf("wasm module path is required when a wasm executor factory is configured")
		return nil, r.buildErr
	}
	executor, err := r.factory.Build(ctx, config)
	if err != nil {
		r.buildErr = err
		return nil, r.buildErr
	}
	if executor == nil {
		r.buildErr = fmt.Errorf("wasm executor factory returned nil executor")
		return nil, r.buildErr
	}
	r.executor = executor
	return r.executor, nil
}

func executeWASMToolBounded(ctx context.Context, executor WASMToolExecutor, req WASMToolExecuteRequest) (WASMToolExecuteResponse, error) {
	type executeResult struct {
		resp WASMToolExecuteResponse
		err  error
	}
	resultCh := make(chan executeResult, 1)
	go func() {
		resp, err := executor.Execute(ctx, req)
		resultCh <- executeResult{resp: resp, err: err}
	}()
	select {
	case <-ctx.Done():
		return WASMToolExecuteResponse{}, ctx.Err()
	case result := <-resultCh:
		return result.resp, result.err
	}
}

func mapWASMContextError(tool string, err error) error {
	tool = strings.TrimSpace(tool)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return NewToolError(
			ToolStatusError,
			ToolCodeTimeout,
			ToolReasonExecutionTimeout,
			true,
			fmt.Sprintf("wasm tool execution timed out for tool=%s", tool),
			err,
			map[string]string{
				"tool":           tool,
				"isolation_mode": "wasm",
			},
		)
	case errors.Is(err, context.Canceled):
		return NewToolError(
			ToolStatusError,
			ToolCodeCanceled,
			ToolReasonExecutionCanceled,
			false,
			fmt.Sprintf("wasm tool execution canceled for tool=%s", tool),
			err,
			map[string]string{
				"tool":           tool,
				"isolation_mode": "wasm",
			},
		)
	default:
		return err
	}
}
