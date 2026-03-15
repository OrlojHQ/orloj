package agentruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/crds"
)

type testWASMExecutor struct {
	call func(ctx context.Context, req WASMToolExecuteRequest) (WASMToolExecuteResponse, error)
}

func (e testWASMExecutor) Execute(ctx context.Context, req WASMToolExecuteRequest) (WASMToolExecuteResponse, error) {
	if e.call == nil {
		return WASMToolExecuteResponse{}, errors.New("missing wasm executor function")
	}
	return e.call(ctx, req)
}

type testWASMExecutorFactory struct {
	build func(ctx context.Context, cfg WASMToolRuntimeConfig) (WASMToolExecutor, error)
}

func (f testWASMExecutorFactory) Build(ctx context.Context, cfg WASMToolRuntimeConfig) (WASMToolExecutor, error) {
	if f.build == nil {
		return nil, errors.New("missing wasm factory build function")
	}
	return f.build(ctx, cfg)
}

func TestWASMToolRuntimeMissingRegistry(t *testing.T) {
	runtime := NewWASMToolRuntime(nil, nil)
	_, err := runtime.Call(context.Background(), "web_search", "payload")
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
		t.Fatal("expected non-retryable error")
	}
}

func TestWASMToolRuntimeUnsupportedTool(t *testing.T) {
	runtime := NewWASMToolRuntime(NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"known_tool": {RiskLevel: "low"},
	}), testWASMExecutor{
		call: func(_ context.Context, req WASMToolExecuteRequest) (WASMToolExecuteResponse, error) {
			return WASMToolExecuteResponse{Output: "ok:" + req.Tool}, nil
		},
	})
	_, err := runtime.Call(context.Background(), "missing_tool", "payload")
	if err == nil {
		t.Fatal("expected unsupported tool error")
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeUnsupportedTool || reason != ToolReasonToolUnsupported {
		t.Fatalf("unexpected metadata code=%s reason=%s", code, reason)
	}
	if retryable {
		t.Fatal("expected non-retryable unsupported tool error")
	}
}

func TestWASMToolRuntimeBoundsTimeout(t *testing.T) {
	runtime := NewWASMToolRuntime(NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"slow_tool": {RiskLevel: "high"},
	}), testWASMExecutor{
		call: func(_ context.Context, _ WASMToolExecuteRequest) (WASMToolExecuteResponse, error) {
			time.Sleep(250 * time.Millisecond)
			return WASMToolExecuteResponse{Output: "late"}, nil
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := runtime.Call(ctx, "slow_tool", "payload")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 120*time.Millisecond {
		t.Fatalf("expected bounded timeout return, elapsed=%s", elapsed)
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeTimeout || reason != ToolReasonExecutionTimeout {
		t.Fatalf("unexpected metadata code=%s reason=%s", code, reason)
	}
	if !retryable {
		t.Fatal("expected timeout to be retryable")
	}
}

func TestWASMToolRuntimeBuildsExecutorFromFactory(t *testing.T) {
	buildCalls := 0
	runtime := NewWASMToolRuntimeWithFactory(
		NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
			"wasm_tool": {RiskLevel: "high"},
		}),
		testWASMExecutorFactory{
			build: func(_ context.Context, cfg WASMToolRuntimeConfig) (WASMToolExecutor, error) {
				buildCalls++
				if cfg.ModulePath != "/tmp/tool.wasm" {
					t.Fatalf("expected module path /tmp/tool.wasm, got %q", cfg.ModulePath)
				}
				if cfg.Entrypoint != "run_tool" {
					t.Fatalf("expected entrypoint run_tool, got %q", cfg.Entrypoint)
				}
				if cfg.MaxMemoryBytes != 134217728 {
					t.Fatalf("expected max memory 134217728, got %d", cfg.MaxMemoryBytes)
				}
				if cfg.Fuel != 50000 {
					t.Fatalf("expected fuel 50000, got %d", cfg.Fuel)
				}
				if !cfg.EnableWASI {
					t.Fatal("expected EnableWASI=true")
				}
				return testWASMExecutor{
					call: func(_ context.Context, req WASMToolExecuteRequest) (WASMToolExecuteResponse, error) {
						if req.Runtime.ModulePath != "/tmp/tool.wasm" {
							t.Fatalf("expected request runtime module path /tmp/tool.wasm, got %q", req.Runtime.ModulePath)
						}
						if req.Runtime.Entrypoint != "run_tool" {
							t.Fatalf("expected request runtime entrypoint run_tool, got %q", req.Runtime.Entrypoint)
						}
						return WASMToolExecuteResponse{Output: "ok:" + req.Tool}, nil
					},
				}, nil
			},
		},
		WASMToolRuntimeConfig{
			ModulePath:     "/tmp/tool.wasm",
			Entrypoint:     "run_tool",
			MaxMemoryBytes: 134217728,
			Fuel:           50000,
			EnableWASI:     true,
		},
	)

	out, err := runtime.Call(context.Background(), "wasm_tool", "payload")
	if err != nil {
		t.Fatalf("expected wasm call success, got %v", err)
	}
	if out != "ok:wasm_tool" {
		t.Fatalf("unexpected output %q", out)
	}
	if buildCalls != 1 {
		t.Fatalf("expected one factory build call, got %d", buildCalls)
	}
	// Executor should be cached and reused.
	out, err = runtime.Call(context.Background(), "wasm_tool", "payload")
	if err != nil {
		t.Fatalf("expected second wasm call success, got %v", err)
	}
	if out != "ok:wasm_tool" {
		t.Fatalf("unexpected second output %q", out)
	}
	if buildCalls != 1 {
		t.Fatalf("expected factory build caching, got %d calls", buildCalls)
	}
}

func TestWASMToolRuntimeFactoryRequiresModulePath(t *testing.T) {
	runtime := NewWASMToolRuntimeWithFactory(
		NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
			"wasm_tool": {RiskLevel: "high"},
		}),
		testWASMExecutorFactory{
			build: func(_ context.Context, _ WASMToolRuntimeConfig) (WASMToolExecutor, error) {
				return testWASMExecutor{
					call: func(_ context.Context, req WASMToolExecuteRequest) (WASMToolExecuteResponse, error) {
						return WASMToolExecuteResponse{Output: "ok:" + req.Tool}, nil
					},
				}, nil
			},
		},
		WASMToolRuntimeConfig{
			ModulePath: "",
		},
	)
	_, err := runtime.Call(context.Background(), "wasm_tool", "payload")
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
		t.Fatal("expected non-retryable runtime policy invalid error")
	}
}

func TestWASMToolRuntimeAllowsWASIDisabledConfig(t *testing.T) {
	buildCalls := 0
	runtime := NewWASMToolRuntimeWithFactory(
		NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
			"wasm_tool": {RiskLevel: "high"},
		}),
		testWASMExecutorFactory{
			build: func(_ context.Context, cfg WASMToolRuntimeConfig) (WASMToolExecutor, error) {
				buildCalls++
				if cfg.EnableWASI {
					t.Fatal("expected EnableWASI=false to be preserved")
				}
				return testWASMExecutor{
					call: func(_ context.Context, req WASMToolExecuteRequest) (WASMToolExecuteResponse, error) {
						if req.Runtime.EnableWASI {
							t.Fatal("expected request runtime EnableWASI=false")
						}
						return WASMToolExecuteResponse{Output: "ok"}, nil
					},
				}, nil
			},
		},
		WASMToolRuntimeConfig{
			ModulePath: "/tmp/tool.wasm",
			EnableWASI: false,
		},
	)
	out, err := runtime.Call(context.Background(), "wasm_tool", "payload")
	if err != nil {
		t.Fatalf("expected wasm call success, got %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected output %q", out)
	}
	if buildCalls != 1 {
		t.Fatalf("expected one factory build call, got %d", buildCalls)
	}
}
