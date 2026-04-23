package agentruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

type recordingToolRuntime struct {
	callCount int
	lastTool  string
	lastInput string
	result    string
	err       error
}

func (r *recordingToolRuntime) Call(_ context.Context, tool string, input string) (string, error) {
	r.callCount++
	r.lastTool = tool
	r.lastInput = input
	return r.result, r.err
}

func TestGovernedToolRuntimeRoutesCLIToIsolated(t *testing.T) {
	isolated := &recordingToolRuntime{result: "isolated-ok"}
	base := &recordingToolRuntime{result: "base-ok"}
	specs := map[string]resources.ToolSpec{
		"kubectl-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "kubectl",
				Image:   "bitnami/kubectl:1.30",
				Output:  "stdout",
			},
			Runtime: resources.ToolRuntimePolicy{
				IsolationMode: "container",
				Timeout:       "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, isolated, NewStaticToolCapabilityRegistry(specs), true)
	result, err := governed.Call(context.Background(), "kubectl-tool", `{"namespace":"default"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "isolated-ok" {
		t.Fatalf("expected isolated result, got %q", result)
	}
	if isolated.callCount != 1 {
		t.Fatalf("expected 1 call to isolated runtime, got %d", isolated.callCount)
	}
	if base.callCount != 0 {
		t.Fatalf("expected 0 calls to base runtime, got %d", base.callCount)
	}
}

func TestGovernedToolRuntimeRoutesCLINoneToCliRuntime(t *testing.T) {
	isolated := &recordingToolRuntime{result: "isolated-ok"}
	base := &recordingToolRuntime{result: "base-ok"}
	cli := &recordingToolRuntime{result: "cli-ok"}
	specs := map[string]resources.ToolSpec{
		"local-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "/usr/local/bin/tool",
				Output:  "stdout",
			},
			Runtime: resources.ToolRuntimePolicy{
				IsolationMode: "none",
				Timeout:       "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, isolated, NewStaticToolCapabilityRegistry(specs), true)
	governed.cliRuntime = cli
	result, err := governed.Call(context.Background(), "local-tool", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "cli-ok" {
		t.Fatalf("expected cli result, got %q", result)
	}
	if cli.callCount != 1 {
		t.Fatalf("expected 1 call to cli runtime, got %d", cli.callCount)
	}
	if isolated.callCount != 0 {
		t.Fatalf("expected 0 calls to isolated runtime, got %d", isolated.callCount)
	}
	if base.callCount != 0 {
		t.Fatalf("expected 0 calls to base runtime, got %d", base.callCount)
	}
}

func TestGovernedToolRuntimeCLIMissingCliRuntimeErrors(t *testing.T) {
	isolated := &recordingToolRuntime{}
	base := &recordingToolRuntime{}
	specs := map[string]resources.ToolSpec{
		"no-cli-rt": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "cmd",
				Output:  "stdout",
			},
			Runtime: resources.ToolRuntimePolicy{
				IsolationMode: "none",
				Timeout:       "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, isolated, NewStaticToolCapabilityRegistry(specs), true)
	_, err := governed.Call(context.Background(), "no-cli-rt", "")
	if err == nil {
		t.Fatal("expected error when cli runtime is nil and isolation=none")
	}
}

func TestGovernedToolRuntimeHTTPStillRoutesToBase(t *testing.T) {
	isolated := &recordingToolRuntime{result: "isolated-ok"}
	base := &recordingToolRuntime{result: "base-ok"}
	specs := map[string]resources.ToolSpec{
		"http-tool": {
			Type:     "http",
			Endpoint: "https://example.com",
			Runtime: resources.ToolRuntimePolicy{
				IsolationMode: "none",
				Timeout:       "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, isolated, NewStaticToolCapabilityRegistry(specs), true)
	result, err := governed.Call(context.Background(), "http-tool", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "base-ok" {
		t.Fatalf("expected base result, got %q", result)
	}
	if base.callCount != 1 {
		t.Fatalf("expected 1 call to base, got %d", base.callCount)
	}
}

func TestGovernedToolRuntimeRoutesExternalToExternalRuntime(t *testing.T) {
	base := &recordingToolRuntime{result: "base-ok"}
	ext := &recordingToolRuntime{result: "external-ok"}
	specs := map[string]resources.ToolSpec{
		"ext-tool": {
			Type:     "external",
			Endpoint: "https://ext.example.com/run",
			Runtime: resources.ToolRuntimePolicy{
				IsolationMode: "none",
				Timeout:       "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, nil, NewStaticToolCapabilityRegistry(specs), true)
	governed.externalRuntime = ext
	result, err := governed.Call(context.Background(), "ext-tool", `{"action":"run"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "external-ok" {
		t.Fatalf("expected external result, got %q", result)
	}
	if ext.callCount != 1 {
		t.Fatalf("expected 1 call to external runtime, got %d", ext.callCount)
	}
	if base.callCount != 0 {
		t.Fatalf("expected 0 calls to base runtime, got %d", base.callCount)
	}
}

func TestGovernedToolRuntimeExternalMissingRuntimeErrors(t *testing.T) {
	base := &recordingToolRuntime{}
	specs := map[string]resources.ToolSpec{
		"ext-tool": {
			Type:     "external",
			Endpoint: "https://ext.example.com/run",
			Runtime: resources.ToolRuntimePolicy{
				IsolationMode: "none",
				Timeout:       "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, nil, NewStaticToolCapabilityRegistry(specs), true)
	_, err := governed.Call(context.Background(), "ext-tool", "")
	if err == nil {
		t.Fatal("expected error when external runtime is nil")
	}
	if !errors.Is(err, ErrToolIsolationUnavailable) {
		t.Fatalf("expected ErrToolIsolationUnavailable, got %v", err)
	}
}

func TestGovernedToolRuntimeExternalHighRiskRoutesToIsolated(t *testing.T) {
	base := &recordingToolRuntime{result: "base-ok"}
	isolated := &recordingToolRuntime{result: "isolated-ok"}
	ext := &recordingToolRuntime{result: "external-ok"}
	specs := map[string]resources.ToolSpec{
		"ext-risky": {
			Type:      "external",
			Endpoint:  "https://ext.example.com/run",
			RiskLevel: "high",
			Runtime: resources.ToolRuntimePolicy{
				Timeout: "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, isolated, NewStaticToolCapabilityRegistry(specs), true)
	governed.externalRuntime = ext
	result, err := governed.Call(context.Background(), "ext-risky", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "isolated-ok" {
		t.Fatalf("expected isolated result for high-risk external tool, got %q", result)
	}
	if isolated.callCount != 1 {
		t.Fatalf("expected 1 call to isolated runtime, got %d", isolated.callCount)
	}
	if ext.callCount != 0 {
		t.Fatalf("expected 0 calls to external runtime, got %d", ext.callCount)
	}
}

func TestGovernedToolRuntimeRoutesGRPCToGRPCRuntime(t *testing.T) {
	base := &recordingToolRuntime{result: "base-ok"}
	grpc := &recordingToolRuntime{result: "grpc-ok"}
	specs := map[string]resources.ToolSpec{
		"grpc-tool": {
			Type:     "grpc",
			Endpoint: "grpc.example.com:443",
			Runtime: resources.ToolRuntimePolicy{
				IsolationMode: "none",
				Timeout:       "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, nil, NewStaticToolCapabilityRegistry(specs), true)
	governed.grpcRuntime = grpc
	result, err := governed.Call(context.Background(), "grpc-tool", `{"method":"Ping"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "grpc-ok" {
		t.Fatalf("expected grpc result, got %q", result)
	}
	if grpc.callCount != 1 {
		t.Fatalf("expected 1 call to grpc runtime, got %d", grpc.callCount)
	}
	if base.callCount != 0 {
		t.Fatalf("expected 0 calls to base runtime, got %d", base.callCount)
	}
}

func TestGovernedToolRuntimeGRPCMissingRuntimeErrors(t *testing.T) {
	base := &recordingToolRuntime{}
	specs := map[string]resources.ToolSpec{
		"grpc-tool": {
			Type:     "grpc",
			Endpoint: "grpc.example.com:443",
			Runtime: resources.ToolRuntimePolicy{
				IsolationMode: "none",
				Timeout:       "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, nil, NewStaticToolCapabilityRegistry(specs), true)
	_, err := governed.Call(context.Background(), "grpc-tool", "")
	if err == nil {
		t.Fatal("expected error when grpc runtime is nil")
	}
	if !errors.Is(err, ErrToolIsolationUnavailable) {
		t.Fatalf("expected ErrToolIsolationUnavailable, got %v", err)
	}
}

func TestGovernedToolRuntimeRoutesWebhookCallbackToWebhookRuntime(t *testing.T) {
	base := &recordingToolRuntime{result: "base-ok"}
	wh := &recordingToolRuntime{result: "webhook-ok"}
	specs := map[string]resources.ToolSpec{
		"wh-tool": {
			Type:     "webhook-callback",
			Endpoint: "https://hooks.example.com/callback",
			Runtime: resources.ToolRuntimePolicy{
				IsolationMode: "none",
				Timeout:       "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, nil, NewStaticToolCapabilityRegistry(specs), true)
	governed.webhookCallbackRuntime = wh
	result, err := governed.Call(context.Background(), "wh-tool", `{"event":"deploy"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "webhook-ok" {
		t.Fatalf("expected webhook result, got %q", result)
	}
	if wh.callCount != 1 {
		t.Fatalf("expected 1 call to webhook runtime, got %d", wh.callCount)
	}
	if base.callCount != 0 {
		t.Fatalf("expected 0 calls to base runtime, got %d", base.callCount)
	}
}

func TestGovernedToolRuntimeWebhookCallbackMissingRuntimeErrors(t *testing.T) {
	base := &recordingToolRuntime{}
	specs := map[string]resources.ToolSpec{
		"wh-tool": {
			Type:     "webhook-callback",
			Endpoint: "https://hooks.example.com/callback",
			Runtime: resources.ToolRuntimePolicy{
				IsolationMode: "none",
				Timeout:       "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, nil, NewStaticToolCapabilityRegistry(specs), true)
	_, err := governed.Call(context.Background(), "wh-tool", "")
	if err == nil {
		t.Fatal("expected error when webhook-callback runtime is nil")
	}
	if !errors.Is(err, ErrToolIsolationUnavailable) {
		t.Fatalf("expected ErrToolIsolationUnavailable, got %v", err)
	}
}

func TestGovernedToolRuntimeRoutesWASMToDedicatedRuntime(t *testing.T) {
	base := &recordingToolRuntime{result: "base-ok"}
	wasmRT := &recordingToolRuntime{result: "wasm-ok"}
	specs := map[string]resources.ToolSpec{
		"wasm-tool": {
			Type: "wasm",
			Runtime: resources.ToolRuntimePolicy{
				Timeout: "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, nil, NewStaticToolCapabilityRegistry(specs), true)
	governed.SetWasmRuntime(wasmRT)
	result, err := governed.Call(context.Background(), "wasm-tool", `{"input":"data"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "wasm-ok" {
		t.Fatalf("expected wasm result, got %q", result)
	}
	if wasmRT.callCount != 1 {
		t.Fatalf("expected 1 call to wasm runtime, got %d", wasmRT.callCount)
	}
	if base.callCount != 0 {
		t.Fatalf("expected 0 calls to base runtime, got %d", base.callCount)
	}
}

func TestGovernedToolRuntimeWASMMissingIsolatedRuntimeErrors(t *testing.T) {
	base := &recordingToolRuntime{}
	specs := map[string]resources.ToolSpec{
		"wasm-tool": {
			Type: "wasm",
			Runtime: resources.ToolRuntimePolicy{
				Timeout: "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, nil, NewStaticToolCapabilityRegistry(specs), true)
	_, err := governed.Call(context.Background(), "wasm-tool", "")
	if err == nil {
		t.Fatal("expected error when isolated runtime is nil for wasm tool")
	}
	if !errors.Is(err, ErrToolIsolationUnavailable) {
		t.Fatalf("expected ErrToolIsolationUnavailable, got %v", err)
	}
}

func TestGovernedToolRuntimeUnknownTypeFailsClosed(t *testing.T) {
	base := &recordingToolRuntime{result: "base-ok"}
	specs := map[string]resources.ToolSpec{
		"mystery-tool": {
			Type: "carrier-pigeon",
			Runtime: resources.ToolRuntimePolicy{
				Timeout: "30s",
			},
		},
	}
	governed := NewGovernedToolRuntime(base, nil, NewStaticToolCapabilityRegistry(specs), true)
	_, err := governed.Call(context.Background(), "mystery-tool", "")
	if err == nil {
		t.Fatal("expected error for unknown tool type")
	}
	if !errors.Is(err, ErrUnsupportedTool) {
		t.Fatalf("expected ErrUnsupportedTool, got %v", err)
	}
	if base.callCount != 0 {
		t.Fatalf("expected 0 calls to base runtime for unknown type, got %d", base.callCount)
	}
}
