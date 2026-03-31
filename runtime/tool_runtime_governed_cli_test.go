package agentruntime

import (
	"context"
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
