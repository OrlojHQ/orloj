package agentruntime

import (
	"context"
	"testing"
)

type noopWASMExecutorFactory struct {
	executor WASMToolExecutor
}

func (f noopWASMExecutorFactory) Build(_ context.Context, _ WASMToolRuntimeConfig) (WASMToolExecutor, error) {
	return f.executor, nil
}

type noopWASMExecutor struct{}

func (e noopWASMExecutor) Execute(_ context.Context, _ WASMToolExecuteRequest) (WASMToolExecuteResponse, error) {
	return WASMToolExecuteResponse{Output: "ok"}, nil
}

func TestToolIsolationBackendRegistryBuildsDefaults(t *testing.T) {
	noneRuntime, err := BuildToolIsolationRuntime(ToolIsolationBackendOptions{Mode: "none"})
	if err != nil {
		t.Fatalf("none backend build failed: %v", err)
	}
	if noneRuntime != nil {
		t.Fatalf("expected nil runtime for none backend, got %T", noneRuntime)
	}

	containerRuntime, err := BuildToolIsolationRuntime(ToolIsolationBackendOptions{
		Mode: "container",
		ContainerConfig: ContainerToolRuntimeConfig{
			RuntimeBinary: "docker",
			Image:         "curlimages/curl:8.8.0",
		},
	})
	if err != nil {
		t.Fatalf("container backend build failed: %v", err)
	}
	if _, ok := containerRuntime.(*ContainerToolRuntime); !ok {
		t.Fatalf("expected *ContainerToolRuntime, got %T", containerRuntime)
	}

	wasmRuntime, err := BuildToolIsolationRuntime(ToolIsolationBackendOptions{
		Mode:                "wasm",
		WASMExecutorFactory: noopWASMExecutorFactory{executor: noopWASMExecutor{}},
		WASMConfig: WASMToolRuntimeConfig{
			ModulePath: "/tmp/test.wasm",
			Entrypoint: "run",
		},
	})
	if err != nil {
		t.Fatalf("wasm backend build failed: %v", err)
	}
	if _, ok := wasmRuntime.(*WASMToolRuntime); !ok {
		t.Fatalf("expected *WASMToolRuntime, got %T", wasmRuntime)
	}
}

func TestToolIsolationBackendRegistrySupportsCustomBackend(t *testing.T) {
	registry := NewToolIsolationBackendRegistry()
	err := registry.Register("custom", func(_ ToolIsolationBackendOptions) (ToolRuntime, error) {
		return &UnsupportedIsolatedToolRuntime{}, nil
	})
	if err != nil {
		t.Fatalf("register custom backend failed: %v", err)
	}
	runtime, err := registry.Build(ToolIsolationBackendOptions{Mode: "custom"})
	if err != nil {
		t.Fatalf("build custom backend failed: %v", err)
	}
	if _, ok := runtime.(*UnsupportedIsolatedToolRuntime); !ok {
		t.Fatalf("expected UnsupportedIsolatedToolRuntime, got %T", runtime)
	}
}

func TestToolIsolationBackendRegistryErrorsOnUnknownMode(t *testing.T) {
	registry := NewToolIsolationBackendRegistry()
	if err := registry.Register("none", func(_ ToolIsolationBackendOptions) (ToolRuntime, error) { return nil, nil }); err != nil {
		t.Fatalf("register none failed: %v", err)
	}
	if _, err := registry.Build(ToolIsolationBackendOptions{Mode: "unknown"}); err == nil {
		t.Fatal("expected unsupported backend error")
	}
}
