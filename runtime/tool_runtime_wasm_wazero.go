package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"

	"github.com/OrlojHQ/orloj/telemetry"
)

var wasmInstanceCounter uint64

// WazeroExecutor runs WASM tool modules in-process using the wazero runtime.
// Compiled modules are cached by module reference to avoid redundant compilation.
type WazeroExecutor struct {
	engine       wazero.Runtime
	mu           sync.RWMutex
	compiled     map[string]wazero.CompiledModule
	wasiReady    bool
	wasiInitOnce sync.Once
	resolver     *WASMModuleResolver
}

// NewWazeroExecutor wraps an existing wazero.Runtime with a compilation cache.
func NewWazeroExecutor(engine wazero.Runtime) *WazeroExecutor {
	return &WazeroExecutor{
		engine:   engine,
		compiled: make(map[string]wazero.CompiledModule),
	}
}

// SetModuleResolver attaches a module resolver for remote (HTTPS/OCI) modules.
func (e *WazeroExecutor) SetModuleResolver(resolver *WASMModuleResolver) {
	e.resolver = resolver
}

func (e *WazeroExecutor) getCompiled(cacheKey string) (wazero.CompiledModule, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	mod, ok := e.compiled[cacheKey]
	return mod, ok
}

func (e *WazeroExecutor) compileAndCache(ctx context.Context, cacheKey string, diskPath string) (wazero.CompiledModule, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if mod, ok := e.compiled[cacheKey]; ok {
		return mod, nil
	}
	wasmBytes, err := os.ReadFile(diskPath)
	if err != nil {
		return nil, fmt.Errorf("read wasm module %q: %w", diskPath, err)
	}
	compiled, err := e.engine.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compile wasm module %q: %w", diskPath, err)
	}
	e.compiled[cacheKey] = compiled
	return compiled, nil
}

// getOrCompile returns a compiled module from cache or compiles from disk.
// cacheKey is the original module reference (path/URL/OCI ref); diskPath
// is the resolved local filesystem path to the .wasm file. The tool name
// is used for observability metrics.
func (e *WazeroExecutor) getOrCompile(ctx context.Context, cacheKey string, diskPath string) (wazero.CompiledModule, bool, error) {
	if mod, ok := e.getCompiled(cacheKey); ok {
		return mod, true, nil
	}
	mod, err := e.compileAndCache(ctx, cacheKey, diskPath)
	return mod, false, err
}

// Execute runs a WASM tool module and returns its output via the stdin/stdout JSON contract.
func (e *WazeroExecutor) Execute(ctx context.Context, req WASMToolExecuteRequest) (WASMToolExecuteResponse, error) {
	startTime := time.Now()
	moduleRef := strings.TrimSpace(req.Runtime.ModulePath)
	if moduleRef == "" {
		return WASMToolExecuteResponse{}, NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			"wasm module_path is required",
			ErrInvalidToolRuntimePolicy,
			map[string]string{"isolation_mode": "wasm", "field": "module_path"},
		)
	}

	// Resolve remote references (HTTPS/OCI) to local paths via the module resolver.
	modulePath := moduleRef
	if e.resolver != nil && ClassifyModuleRef(moduleRef) != ModuleSourceLocal {
		resolved, resolveErr := e.resolver.Resolve(ctx, moduleRef, req.ImagePullSecret)
		if resolveErr != nil {
			return WASMToolExecuteResponse{}, NewToolError(
				ToolStatusError,
				ToolCodeExecutionFailed,
				ToolReasonBackendFailure,
				true,
				fmt.Sprintf("wasm module resolution failed for %q", moduleRef),
				resolveErr,
				map[string]string{"isolation_mode": "wasm", "module_ref": moduleRef},
			)
		}
		modulePath = resolved
	}

	// Cache key uses the original reference so URL/OCI refs share compiled modules.
	compiled, cacheHit, compileErr := e.getOrCompile(ctx, moduleRef, modulePath)
	if cacheHit {
		telemetry.RecordWASMCacheHit(strings.TrimSpace(req.Tool))
	} else if compileErr == nil {
		telemetry.RecordWASMCacheMiss(strings.TrimSpace(req.Tool))
	}
	if compileErr != nil {
		if os.IsNotExist(compileErr) || strings.Contains(compileErr.Error(), "no such file") {
			return WASMToolExecuteResponse{}, NewToolError(
				ToolStatusError,
				ToolCodeRuntimePolicyInvalid,
				ToolReasonRuntimePolicyInvalid,
				false,
				fmt.Sprintf("wasm module %q not found", moduleRef),
				compileErr,
				map[string]string{"isolation_mode": "wasm", "module_ref": moduleRef},
			)
		}
		return WASMToolExecuteResponse{}, NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			false,
			fmt.Sprintf("wasm module compilation failed for %q", moduleRef),
			compileErr,
			map[string]string{"isolation_mode": "wasm", "module_ref": moduleRef},
		)
	}

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
			map[string]string{"isolation_mode": "wasm", "field": "payload"},
		)
	}

	stdout := NewBoundedWriter(DefaultMaxToolOutputBytes)
	stderr := NewBoundedWriter(DefaultMaxToolOutputBytes)
	instanceID := atomic.AddUint64(&wasmInstanceCounter, 1)
	moduleName := strings.TrimSpace(req.Tool)
	if moduleName == "" {
		moduleName = "wasm-tool"
	}
	moduleName = fmt.Sprintf("%s-%d", moduleName, instanceID)

	modCfg := wazero.NewModuleConfig().
		WithStdin(bytes.NewReader(payload)).
		WithStdout(stdout).
		WithStderr(stderr).
		WithName(moduleName).
		WithStartFunctions("_start")

	if req.Runtime.EnableWASI {
		var wasiErr error
		e.wasiInitOnce.Do(func() {
			if _, err := wasi_snapshot_preview1.Instantiate(ctx, e.engine); err != nil {
				if !strings.Contains(err.Error(), "already been instantiated") {
					wasiErr = err
				}
			}
			if wasiErr == nil {
				e.wasiReady = true
			}
		})
		if wasiErr != nil {
			return WASMToolExecuteResponse{}, NewToolError(
				ToolStatusError,
				ToolCodeExecutionFailed,
				ToolReasonBackendFailure,
				false,
				"failed to instantiate WASI",
				wasiErr,
				map[string]string{"isolation_mode": "wasm", "module_ref": moduleRef},
			)
		}
		if !e.wasiReady {
			return WASMToolExecuteResponse{}, NewToolError(
				ToolStatusError,
				ToolCodeExecutionFailed,
				ToolReasonBackendFailure,
				false,
				"WASI initialization previously failed",
				nil,
				map[string]string{"isolation_mode": "wasm", "module_ref": moduleRef},
			)
		}
	}

	mod, instantiateErr := e.engine.InstantiateModule(ctx, compiled, modCfg)
	if mod != nil {
		defer mod.Close(ctx)
	}
	if instantiateErr != nil {
		var exitErr *sys.ExitError
		if !(errors.As(instantiateErr, &exitErr) && exitErr.ExitCode() == 0) {
			return WASMToolExecuteResponse{}, NewToolError(
				ToolStatusError,
				ToolCodeExecutionFailed,
				ToolReasonBackendFailure,
				true,
				fmt.Sprintf("wasm module instantiation failed for tool=%s stderr=%s", strings.TrimSpace(req.Tool), RedactSensitive(compactStderr(stderr.String()))),
				instantiateErr,
				map[string]string{"isolation_mode": "wasm", "module_ref": moduleRef},
			)
		}
	}

	moduleResp, decodeErr := DecodeWASMToolModuleResponse(stdout.String())
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
			map[string]string{"isolation_mode": "wasm", "module_ref": moduleRef},
		)
	}

	durationSec := time.Since(startTime).Seconds()
	toolName := strings.TrimSpace(req.Tool)

	switch moduleResp.Status {
	case wasmToolModuleStatusOK:
		telemetry.RecordToolExecution(toolName, "wasm", "ok", durationSec)
		return WASMToolExecuteResponse{Output: strings.TrimSpace(moduleResp.Output)}, nil
	case wasmToolModuleStatusDenied:
		telemetry.RecordToolExecution(toolName, "wasm", "denied", durationSec)
		return WASMToolExecuteResponse{}, wasmToolModuleFailureAsToolError(req.Tool, moduleResp, ToolStatusDenied)
	default:
		telemetry.RecordToolExecution(toolName, "wasm", "error", durationSec)
		return WASMToolExecuteResponse{}, wasmToolModuleFailureAsToolError(req.Tool, moduleResp, ToolStatusError)
	}
}

// Close releases all cached compiled modules and the underlying engine.
func (e *WazeroExecutor) Close(ctx context.Context) error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for k, mod := range e.compiled {
		_ = mod.Close(ctx)
		delete(e.compiled, k)
	}
	return nil
}

// WazeroExecutorFactory builds WazeroExecutor instances backed by a shared wazero.Runtime engine.
type WazeroExecutorFactory struct {
	executor *WazeroExecutor
}

// NewWazeroExecutorFactory creates a factory that produces executors sharing a single wazero.Runtime.
func NewWazeroExecutorFactory(engine wazero.Runtime) *WazeroExecutorFactory {
	return &WazeroExecutorFactory{
		executor: NewWazeroExecutor(engine),
	}
}

// NewWazeroExecutorFactoryWithResolver creates a factory with an attached module
// resolver for remote (HTTPS/OCI) WASM modules.
func NewWazeroExecutorFactoryWithResolver(engine wazero.Runtime, resolver *WASMModuleResolver) *WazeroExecutorFactory {
	exec := NewWazeroExecutor(engine)
	exec.resolver = resolver
	return &WazeroExecutorFactory{executor: exec}
}

// Build returns the shared WazeroExecutor (module path is per-request, not per-build).
func (f *WazeroExecutorFactory) Build(_ context.Context, _ WASMToolRuntimeConfig) (WASMToolExecutor, error) {
	if f == nil || f.executor == nil {
		return nil, fmt.Errorf("wazero executor factory is not initialized")
	}
	return f.executor, nil
}

