package agentruntime

import (
	"context"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestGovernedToolRuntimeRoutesA2AToA2ARuntime(t *testing.T) {
	base := &recordingToolRuntime{result: "base-ok"}
	isolated := &recordingToolRuntime{result: "isolated-ok"}
	a2aRT := &recordingToolRuntime{result: "a2a-ok"}

	specs := map[string]resources.ToolSpec{
		"remote-agent-tool": {
			Type: "a2a",
			A2A: resources.ToolA2ASpec{
				AgentURL: "https://remote.example.com/a2a",
			},
		},
	}

	governed := NewGovernedToolRuntime(base, isolated, NewStaticToolCapabilityRegistry(specs), true)
	governed.a2aRuntime = a2aRT

	result, err := governed.Call(context.Background(), "remote-agent-tool", `{"prompt":"summarize"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "a2a-ok" {
		t.Fatalf("expected a2a result, got %q", result)
	}
	if a2aRT.callCount != 1 {
		t.Fatalf("expected 1 call to a2a runtime, got %d", a2aRT.callCount)
	}
	if base.callCount != 0 {
		t.Fatalf("expected 0 calls to base runtime, got %d", base.callCount)
	}
}

func TestGovernedToolRuntimeA2AUnavailable(t *testing.T) {
	base := &recordingToolRuntime{result: "base-ok"}
	isolated := &recordingToolRuntime{result: "isolated-ok"}

	specs := map[string]resources.ToolSpec{
		"remote-agent-tool": {
			Type: "a2a",
			A2A: resources.ToolA2ASpec{
				AgentURL: "https://remote.example.com/a2a",
			},
		},
	}

	governed := NewGovernedToolRuntime(base, isolated, NewStaticToolCapabilityRegistry(specs), true)
	// a2aRuntime is NOT set

	_, err := governed.Call(context.Background(), "remote-agent-tool", `{}`)
	if err == nil {
		t.Fatal("expected error when a2a runtime is not configured")
	}
	var toolErr *ToolError
	if !isToolError(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
}

func TestConfigureA2ARuntime(t *testing.T) {
	base := &recordingToolRuntime{result: "base-ok"}
	isolated := &recordingToolRuntime{result: "isolated-ok"}
	a2aRT := &recordingToolRuntime{result: "a2a-ok"}

	specs := map[string]resources.ToolSpec{
		"remote-tool": {
			Type: "a2a",
			A2A:  resources.ToolA2ASpec{AgentURL: "https://example.com/a2a"},
		},
	}

	governed := NewGovernedToolRuntime(base, isolated, NewStaticToolCapabilityRegistry(specs), true)
	ConfigureA2ARuntime(governed, a2aRT, "production")

	if governed.a2aRuntime == nil {
		t.Fatal("expected a2a runtime to be configured")
	}
}

func TestConfigureA2ARuntimeNilSafe(t *testing.T) {
	ConfigureA2ARuntime(nil, nil, "default")
	ConfigureA2ARuntime(&GovernedToolRuntime{}, nil, "default")

	rt := &recordingToolRuntime{result: "ok"}
	ConfigureA2ARuntime(rt, rt, "default")
}

func isToolError(err error, target **ToolError) bool {
	for e := err; e != nil; {
		if te, ok := e.(*ToolError); ok {
			*target = te
			return true
		}
		if u, ok := e.(interface{ Unwrap() error }); ok {
			e = u.Unwrap()
		} else {
			break
		}
	}
	return false
}
