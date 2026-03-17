package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

type scriptedToolRuntime struct {
	calls     int
	failUntil int
	result    string
	err       error
}

type blockingToolRuntime struct {
	delay time.Duration
}

func (r blockingToolRuntime) Call(_ context.Context, _ string, _ string) (string, error) {
	time.Sleep(r.delay)
	return "late", nil
}

func (r *scriptedToolRuntime) Call(_ context.Context, tool string, _ string) (string, error) {
	r.calls++
	if r.calls <= r.failUntil {
		if r.err != nil {
			return "", r.err
		}
		return "", fmt.Errorf("temporary failure")
	}
	return r.result + ":" + tool, nil
}

type staticToolLookup struct {
	items map[string]resources.Tool
}

func (l staticToolLookup) Get(name string) (resources.Tool, bool) {
	item, ok := l.items[name]
	return item, ok
}

type staticRoleLookup struct {
	items map[string]resources.AgentRole
}

func (l staticRoleLookup) Get(name string) (resources.AgentRole, bool) {
	item, ok := l.items[name]
	return item, ok
}

type staticToolPermissionLookup struct {
	items []resources.ToolPermission
}

func (l staticToolPermissionLookup) List() []resources.ToolPermission {
	out := make([]resources.ToolPermission, len(l.items))
	copy(out, l.items)
	return out
}

func TestGovernedToolRuntimeStrictUnsupportedTool(t *testing.T) {
	runtime := NewGovernedToolRuntime(&MockToolClient{}, nil, NewStaticToolCapabilityRegistry(nil), true)
	_, err := runtime.Call(context.Background(), "web_search", "input")
	if err == nil {
		t.Fatal("expected unsupported tool error")
	}
	if !errors.Is(err, ErrUnsupportedTool) {
		t.Fatalf("expected ErrUnsupportedTool, got %v", err)
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeUnsupportedTool {
		t.Fatalf("expected code %q, got %q", ToolCodeUnsupportedTool, code)
	}
	if reason != ToolReasonToolUnsupported {
		t.Fatalf("expected reason %q, got %q", ToolReasonToolUnsupported, reason)
	}
	if retryable {
		t.Fatal("expected unsupported tool to be non-retryable")
	}
}

func TestGovernedToolRuntimeRetriesPerPolicy(t *testing.T) {
	base := &scriptedToolRuntime{failUntil: 2, result: "ok", err: errors.New("transient")}
	runtime := NewGovernedToolRuntime(
		base,
		nil,
		NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
			"web_search": {
				Runtime: resources.ToolRuntimePolicy{
					Timeout: "1s",
					Retry: resources.ToolRetryPolicy{
						MaxAttempts: 3,
						Backoff:     "0s",
						MaxBackoff:  "1s",
						Jitter:      "none",
					},
				},
			},
		}),
		true,
	)

	out, err := runtime.Call(context.Background(), "web_search", "q=orloj")
	if err != nil {
		t.Fatalf("expected retry success, got error: %v", err)
	}
	if out != "ok:web_search" {
		t.Fatalf("unexpected output %q", out)
	}
	if base.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", base.calls)
	}
}

func TestGovernedToolRuntimeRoutesHighRiskToolsToIsolationRuntime(t *testing.T) {
	base := &scriptedToolRuntime{result: "base"}
	isolated := &scriptedToolRuntime{result: "isolated"}
	runtime := NewGovernedToolRuntime(
		base,
		isolated,
		NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
			"db_write": {
				Runtime: resources.ToolRuntimePolicy{
					Timeout:       "1s",
					IsolationMode: "sandboxed",
					Retry: resources.ToolRetryPolicy{
						MaxAttempts: 1,
						Backoff:     "0s",
						MaxBackoff:  "1s",
						Jitter:      "none",
					},
				},
			},
		}),
		true,
	)

	out, err := runtime.Call(context.Background(), "db_write", "payload")
	if err != nil {
		t.Fatalf("isolated call failed: %v", err)
	}
	if out != "isolated:db_write" {
		t.Fatalf("expected isolated runtime output, got %q", out)
	}
	if isolated.calls != 1 {
		t.Fatalf("expected isolated runtime calls=1, got %d", isolated.calls)
	}
	if base.calls != 0 {
		t.Fatalf("expected base runtime calls=0, got %d", base.calls)
	}
}

func TestGovernedToolRuntimeFailsClosedWhenIsolationRuntimeMissing(t *testing.T) {
	base := &scriptedToolRuntime{result: "base"}
	runtime := NewGovernedToolRuntime(
		base,
		nil,
		NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
			"shell_exec": {
				Runtime: resources.ToolRuntimePolicy{
					Timeout:       "1s",
					IsolationMode: "sandboxed",
					Retry: resources.ToolRetryPolicy{
						MaxAttempts: 1,
						Backoff:     "0s",
						MaxBackoff:  "1s",
						Jitter:      "none",
					},
				},
			},
		}),
		true,
	)

	_, err := runtime.Call(context.Background(), "shell_exec", "rm -rf /tmp")
	if err == nil {
		t.Fatal("expected isolation runtime unavailable error")
	}
	if !errors.Is(err, ErrToolIsolationUnavailable) {
		t.Fatalf("expected ErrToolIsolationUnavailable, got %v", err)
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeIsolationUnavailable {
		t.Fatalf("expected code %q, got %q", ToolCodeIsolationUnavailable, code)
	}
	if reason != ToolReasonIsolationUnavailable {
		t.Fatalf("expected reason %q, got %q", ToolReasonIsolationUnavailable, reason)
	}
	if retryable {
		t.Fatal("expected isolation unavailable to be non-retryable")
	}
	if base.calls != 0 {
		t.Fatalf("expected no base runtime calls when isolation is required, got %d", base.calls)
	}
}

func TestBuildGovernedToolRuntimeForAgentUsesScopedToolLookup(t *testing.T) {
	lookup := staticToolLookup{
		items: map[string]resources.Tool{
			"team-a/web_search": {
				Metadata: resources.ObjectMeta{Name: "web_search", Namespace: "team-a"},
				Spec: resources.ToolSpec{
					Runtime: resources.ToolRuntimePolicy{
						Timeout: "1s",
						Retry: resources.ToolRetryPolicy{
							MaxAttempts: 1,
							Backoff:     "0s",
							MaxBackoff:  "1s",
							Jitter:      "none",
						},
					},
				},
			},
		},
	}
	base := &scriptedToolRuntime{result: "ok"}
	runtime := BuildGovernedToolRuntimeForAgent(base, nil, lookup, "team-a", []string{"web_search"})
	if runtime == nil {
		t.Fatal("expected governed runtime")
	}
	out, err := runtime.Call(context.Background(), "web_search", "q=ai")
	if err != nil {
		t.Fatalf("governed call failed: %v", err)
	}
	if out != "ok:web_search" {
		t.Fatalf("unexpected output %q", out)
	}
}

func TestGovernedToolRuntimeWithGovernanceAllowsRolePermission(t *testing.T) {
	toolLookup := staticToolLookup{
		items: map[string]resources.Tool{
			"default/web_search": {
				Metadata: resources.ObjectMeta{Name: "web_search", Namespace: "default"},
				Spec: resources.ToolSpec{
					Capabilities: []string{"web.read"},
					Runtime: resources.ToolRuntimePolicy{
						Timeout: "1s",
						Retry: resources.ToolRetryPolicy{
							MaxAttempts: 1,
							Backoff:     "0s",
							MaxBackoff:  "1s",
							Jitter:      "none",
						},
					},
				},
			},
		},
	}
	roleLookup := staticRoleLookup{
		items: map[string]resources.AgentRole{
			"default/analyst": {
				Metadata: resources.ObjectMeta{Name: "analyst", Namespace: "default"},
				Spec: resources.AgentRoleSpec{
					Permissions: []string{"tool:web_search:invoke", "capability:web.read"},
				},
			},
		},
	}
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "researcher", Namespace: "default"},
		Spec: resources.AgentSpec{
			Tools: []string{"web_search"},
			Roles: []string{"analyst"},
		},
	}
	base := &scriptedToolRuntime{result: "ok"}
	runtime := BuildGovernedToolRuntimeForAgentWithGovernance(base, nil, toolLookup, roleLookup, nil, "default", agent)

	out, err := runtime.Call(context.Background(), "web_search", "q=orloj")
	if err != nil {
		t.Fatalf("expected authorized call, got %v", err)
	}
	if out != "ok:web_search" {
		t.Fatalf("unexpected output %q", out)
	}
}

func TestGovernedToolRuntimeWithGovernanceDeniesMissingRole(t *testing.T) {
	toolLookup := staticToolLookup{
		items: map[string]resources.Tool{
			"default/web_search": {
				Metadata: resources.ObjectMeta{Name: "web_search", Namespace: "default"},
				Spec: resources.ToolSpec{
					Runtime: resources.ToolRuntimePolicy{
						Timeout: "1s",
						Retry: resources.ToolRetryPolicy{
							MaxAttempts: 1,
							Backoff:     "0s",
							MaxBackoff:  "1s",
							Jitter:      "none",
						},
					},
				},
			},
		},
	}
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "researcher", Namespace: "default"},
		Spec: resources.AgentSpec{
			Tools: []string{"web_search"},
			Roles: []string{"missing-role"},
		},
	}
	base := &scriptedToolRuntime{result: "ok"}
	runtime := BuildGovernedToolRuntimeForAgentWithGovernance(base, nil, toolLookup, staticRoleLookup{items: map[string]resources.AgentRole{}}, nil, "default", agent)

	_, err := runtime.Call(context.Background(), "web_search", "q=orloj")
	if err == nil {
		t.Fatal("expected role resolution denial")
	}
	if !errors.Is(err, ErrToolPermissionDenied) {
		t.Fatalf("expected ErrToolPermissionDenied, got %v", err)
	}
}

func TestGovernedToolRuntimeWithGovernanceAppliesToolPermissionRule(t *testing.T) {
	toolLookup := staticToolLookup{
		items: map[string]resources.Tool{
			"default/db_write": {
				Metadata: resources.ObjectMeta{Name: "db_write", Namespace: "default"},
				Spec: resources.ToolSpec{
					Runtime: resources.ToolRuntimePolicy{
						Timeout: "1s",
						Retry: resources.ToolRetryPolicy{
							MaxAttempts: 1,
							Backoff:     "0s",
							MaxBackoff:  "1s",
							Jitter:      "none",
						},
					},
				},
			},
		},
	}
	roleLookup := staticRoleLookup{
		items: map[string]resources.AgentRole{
			"default/readonly": {
				Metadata: resources.ObjectMeta{Name: "readonly", Namespace: "default"},
				Spec: resources.AgentRoleSpec{
					Permissions: []string{"tool:db_read:invoke"},
				},
			},
		},
	}
	permissionLookup := staticToolPermissionLookup{
		items: []resources.ToolPermission{
			{
				Metadata: resources.ObjectMeta{Name: "db-write", Namespace: "default"},
				Spec: resources.ToolPermissionSpec{
					ToolRef:             "db_write",
					ApplyMode:           "global",
					MatchMode:           "all",
					RequiredPermissions: []string{"tool:db_write:invoke"},
				},
			},
		},
	}
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "planner", Namespace: "default"},
		Spec: resources.AgentSpec{
			Tools: []string{"db_write"},
			Roles: []string{"readonly"},
		},
	}
	base := &scriptedToolRuntime{result: "ok"}
	runtime := BuildGovernedToolRuntimeForAgentWithGovernance(base, nil, toolLookup, roleLookup, permissionLookup, "default", agent)

	_, err := runtime.Call(context.Background(), "db_write", "payload")
	if err == nil {
		t.Fatal("expected permission denial")
	}
	if !errors.Is(err, ErrToolPermissionDenied) {
		t.Fatalf("expected ErrToolPermissionDenied, got %v", err)
	}
}

func TestGovernedToolRuntimeBoundedTimeoutWhenRuntimeIgnoresContext(t *testing.T) {
	runtime := NewGovernedToolRuntime(
		blockingToolRuntime{delay: 250 * time.Millisecond},
		nil,
		NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
			"web_search": {
				Runtime: resources.ToolRuntimePolicy{
					Timeout: "10ms",
					Retry: resources.ToolRetryPolicy{
						MaxAttempts: 1,
						Backoff:     "0s",
						MaxBackoff:  "1s",
						Jitter:      "none",
					},
				},
			},
		}),
		true,
	)
	start := time.Now()
	_, err := runtime.Call(context.Background(), "web_search", "q=orloj")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 120*time.Millisecond {
		t.Fatalf("expected bounded timeout latency, elapsed=%s", elapsed)
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeTimeout {
		t.Fatalf("expected code %q, got %q", ToolCodeTimeout, code)
	}
	if reason != ToolReasonExecutionTimeout {
		t.Fatalf("expected reason %q, got %q", ToolReasonExecutionTimeout, reason)
	}
	if !retryable {
		t.Fatal("expected timeout to be retryable")
	}
}

func TestGovernedToolRuntimeMapsCanceledContext(t *testing.T) {
	runtime := NewGovernedToolRuntime(
		blockingToolRuntime{delay: 250 * time.Millisecond},
		nil,
		NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
			"web_search": {
				Runtime: resources.ToolRuntimePolicy{
					Timeout: "2s",
					Retry: resources.ToolRetryPolicy{
						MaxAttempts: 1,
						Backoff:     "0s",
						MaxBackoff:  "1s",
						Jitter:      "none",
					},
				},
			},
		}),
		true,
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, err := runtime.Call(ctx, "web_search", "q=orloj")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected canceled error")
	}
	if elapsed > 80*time.Millisecond {
		t.Fatalf("expected canceled return promptly, elapsed=%s", elapsed)
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeCanceled {
		t.Fatalf("expected code %q, got %q", ToolCodeCanceled, code)
	}
	if reason != ToolReasonExecutionCanceled {
		t.Fatalf("expected reason %q, got %q", ToolReasonExecutionCanceled, reason)
	}
	if retryable {
		t.Fatal("expected canceled to be non-retryable")
	}
}

type approvalRequiredAuthorizer struct{}

func (a approvalRequiredAuthorizer) Authorize(tool string, spec resources.ToolSpec) (*AuthorizeResult, error) {
	return &AuthorizeResult{
		Verdict: AuthorizeVerdictApprovalRequired,
		Reason:  "approval required for tool=" + tool,
		Details: map[string]string{"tool": tool},
	}, nil
}

func TestGovernedToolRuntimeApprovalRequired(t *testing.T) {
	base := &scriptedToolRuntime{result: "ok"}
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"deploy": {Type: "http", OperationClasses: []string{"write"}},
	})
	governed := NewGovernedToolRuntimeWithAuthorizer(base, nil, registry, approvalRequiredAuthorizer{}, true)
	_, err := governed.Call(context.Background(), "deploy", `{"action":"deploy"}`)
	if err == nil {
		t.Fatal("expected approval required error")
	}
	if !errors.Is(err, ErrToolApprovalRequired) {
		t.Errorf("expected ErrToolApprovalRequired, got %v", err)
	}
}

func TestGovernedToolRuntimeApprovalRequiredIsNonRetryable(t *testing.T) {
	err := fmt.Errorf("%w: tool=deploy reason=approval required", ErrToolApprovalRequired)
	if shouldRetryToolError(err) {
		t.Fatal("approval_required errors should be non-retryable")
	}
}

func TestApprovalErrorCodesAreNonRetryable(t *testing.T) {
	codes := []string{
		ToolCodeApprovalPending,
		ToolCodeApprovalDenied,
		ToolCodeApprovalTimeout,
	}
	for _, code := range codes {
		err := NewToolError(ToolStatusError, code, "test", false, "test", nil, nil)
		if shouldRetryToolError(err) {
			t.Errorf("expected code %q to be non-retryable", code)
		}
	}
}

func TestIsApprovalRequiredError(t *testing.T) {
	if IsApprovalRequiredError(nil) {
		t.Fatal("nil should not be approval required")
	}
	err := fmt.Errorf("%w: tool=x", ErrToolApprovalRequired)
	if !IsApprovalRequiredError(err) {
		t.Fatal("wrapped ErrToolApprovalRequired should be detected")
	}
	if IsApprovalRequiredError(errors.New("some other error")) {
		t.Fatal("unrelated error should not be approval required")
	}
}
