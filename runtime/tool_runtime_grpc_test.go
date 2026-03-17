package agentruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/OrlojHQ/orloj/crds"
)

func TestGRPCToolRuntimeFailsOnMissingEndpoint(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"grpc_tool": {Type: "grpc"},
	})
	runtime := NewGRPCToolRuntime(registry, nil, nil)

	_, err := runtime.Call(context.Background(), "grpc_tool", "input")
	if err == nil {
		t.Fatal("expected missing endpoint error")
	}
	code, _, _, _ := ToolErrorMeta(err)
	if code != ToolCodeRuntimePolicyInvalid {
		t.Fatalf("expected code %q, got %q", ToolCodeRuntimePolicyInvalid, code)
	}
}

func TestGRPCToolRuntimeFailsOnUnsupportedTool(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(nil)
	runtime := NewGRPCToolRuntime(registry, nil, nil)

	_, err := runtime.Call(context.Background(), "unknown_tool", "input")
	if err == nil {
		t.Fatal("expected unsupported tool error")
	}
	if !errors.Is(err, ErrUnsupportedTool) {
		t.Fatalf("expected ErrUnsupportedTool, got %v", err)
	}
}

func TestGRPCToolRuntimeFailsOnMissingRegistry(t *testing.T) {
	runtime := NewGRPCToolRuntime(nil, nil, nil)

	_, err := runtime.Call(context.Background(), "grpc_tool", "input")
	if err == nil {
		t.Fatal("expected missing registry error")
	}
	code, _, _, _ := ToolErrorMeta(err)
	if code != ToolCodeRuntimePolicyInvalid {
		t.Fatalf("expected code %q, got %q", ToolCodeRuntimePolicyInvalid, code)
	}
}

func TestGRPCToolRuntimeFailsOnEmptyToolName(t *testing.T) {
	runtime := NewGRPCToolRuntime(nil, nil, nil)

	_, err := runtime.Call(context.Background(), "", "input")
	if err == nil {
		t.Fatal("expected empty tool name error")
	}
	code, _, _, _ := ToolErrorMeta(err)
	if code != ToolCodeInvalidInput {
		t.Fatalf("expected code %q, got %q", ToolCodeInvalidInput, code)
	}
}

func TestGRPCToolRuntimeSecretResolutionFailure(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"grpc_tool": {
			Type:     "grpc",
			Endpoint: "localhost:50051",
			Auth:     crds.ToolAuth{SecretRef: "missing"},
		},
	})
	secrets := staticSecretResolver{values: map[string]string{}}
	runtime := NewGRPCToolRuntime(registry, secrets, nil)

	_, err := runtime.Call(context.Background(), "grpc_tool", "input")
	if err == nil {
		t.Fatal("expected secret resolution failure")
	}
	if !errors.Is(err, ErrToolSecretResolution) {
		t.Fatalf("expected ErrToolSecretResolution, got %v", err)
	}
}

func TestGRPCToolRuntimeRegisteredInDefaultRegistry(t *testing.T) {
	registry := DefaultToolIsolationBackendRegistry()
	modes := registry.Modes()
	found := false
	for _, mode := range modes {
		if mode == "grpc" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'grpc' in registered modes, got %v", modes)
	}
}
