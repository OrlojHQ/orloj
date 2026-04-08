package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

type mockCLICommandRunner struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
	lastCmd  string
	lastArgs []string
	lastEnv  []string
}

func (r *mockCLICommandRunner) Run(_ context.Context, command string, args []string, _ string, env []string, _ string) (string, string, int, error) {
	r.lastCmd = command
	r.lastArgs = args
	r.lastEnv = env
	return r.stdout, r.stderr, r.exitCode, r.err
}

// newCLITestRuntime builds a CLIToolRuntime preconfigured with a permissive
// allowlist covering every command name used by the tests in this file. The
// runtime fails closed when AllowedCommands is empty, so tests that exercise
// the happy path must provide an explicit allowlist.
func newCLITestRuntime(specs map[string]resources.ToolSpec, runner *mockCLICommandRunner) *CLIToolRuntime {
	cfg := DefaultCLIToolRuntimeConfig()
	cfg.AllowedCommands = []string{"kubectl", "failing", "cmd", "rm", "echo"}
	return NewCLIToolRuntime(
		NewStaticToolCapabilityRegistry(specs),
		NewEnvSecretResolver("TEST_SECRET_"),
		runner,
		cfg,
	)
}

func TestCLIToolRuntimeSuccess(t *testing.T) {
	runner := &mockCLICommandRunner{stdout: "pod1\npod2\n", exitCode: 0}
	rt := newCLITestRuntime(map[string]resources.ToolSpec{
		"kubectl-pods": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "kubectl",
				Args:    []string{"get", "pods", "-n", "{{ .namespace }}"},
				Output:  "stdout",
			},
		},
	}, runner)
	result, err := rt.Call(context.Background(), "kubectl-pods", `{"namespace": "default"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "pod1\npod2" {
		t.Fatalf("unexpected result: %q", result)
	}
	if runner.lastCmd != "kubectl" {
		t.Fatalf("expected command=kubectl, got %q", runner.lastCmd)
	}
	if len(runner.lastArgs) != 4 || runner.lastArgs[3] != "default" {
		t.Fatalf("unexpected args: %v", runner.lastArgs)
	}
}

func TestCLIToolRuntimeNonZeroExit(t *testing.T) {
	runner := &mockCLICommandRunner{stdout: "", stderr: "not found", exitCode: 1}
	rt := newCLITestRuntime(map[string]resources.ToolSpec{
		"failing-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "failing",
				Output:  "stdout",
			},
		},
	}, runner)
	_, err := rt.Call(context.Background(), "failing-tool", "")
	if err == nil {
		t.Fatal("expected error for non-zero exit code")
	}
	if !strings.Contains(err.Error(), "exited with code 1") {
		t.Fatalf("expected exit code in error, got: %v", err)
	}
}

func TestCLIToolRuntimeOutputStderr(t *testing.T) {
	runner := &mockCLICommandRunner{stdout: "out", stderr: "err", exitCode: 0}
	rt := newCLITestRuntime(map[string]resources.ToolSpec{
		"stderr-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "cmd",
				Output:  "stderr",
			},
		},
	}, runner)
	result, err := rt.Call(context.Background(), "stderr-tool", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "err" {
		t.Fatalf("expected stderr output, got %q", result)
	}
}

func TestCLIToolRuntimeOutputBoth(t *testing.T) {
	runner := &mockCLICommandRunner{stdout: "out", stderr: "err", exitCode: 0}
	rt := newCLITestRuntime(map[string]resources.ToolSpec{
		"both-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "cmd",
				Output:  "both",
			},
		},
	}, runner)
	result, err := rt.Call(context.Background(), "both-tool", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, `"stdout":"out"`) || !strings.Contains(result, `"stderr":"err"`) {
		t.Fatalf("expected JSON with both streams, got %q", result)
	}
}

func TestCLIToolRuntimeRejectsNonCLIType(t *testing.T) {
	runner := &mockCLICommandRunner{}
	rt := newCLITestRuntime(map[string]resources.ToolSpec{
		"http-tool": {
			Type:     "http",
			Endpoint: "https://example.com",
		},
	}, runner)
	_, err := rt.Call(context.Background(), "http-tool", "")
	if err == nil {
		t.Fatal("expected error for non-cli tool type")
	}
}

func TestCLIToolRuntimeRejectsUnknownTool(t *testing.T) {
	runner := &mockCLICommandRunner{}
	rt := newCLITestRuntime(map[string]resources.ToolSpec{}, runner)
	_, err := rt.Call(context.Background(), "unknown", "")
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestCLIToolRuntimeAllowedCommands(t *testing.T) {
	runner := &mockCLICommandRunner{stdout: "ok", exitCode: 0}
	rt := NewCLIToolRuntime(
		NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
			"blocked": {
				Type: "cli",
				Cli: resources.ToolCliSpec{
					Command: "rm",
					Output:  "stdout",
				},
			},
		}),
		NewEnvSecretResolver("TEST_SECRET_"),
		runner,
		CLIToolRuntimeConfig{
			AllowedCommands: []string{"kubectl", "gh"},
			MaxArgvLength:   4096,
		},
	)
	_, err := rt.Call(context.Background(), "blocked", "")
	if err == nil {
		t.Fatal("expected error for command not in allowlist")
	}
}

func TestCLIToolRuntimeMaxArgvLength(t *testing.T) {
	runner := &mockCLICommandRunner{stdout: "ok", exitCode: 0}
	rt := NewCLIToolRuntime(
		NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
			"long-args": {
				Type: "cli",
				Cli: resources.ToolCliSpec{
					Command: "cmd",
					Args:    []string{"{{ .value }}"},
					Output:  "stdout",
				},
			},
		}),
		NewEnvSecretResolver("TEST_SECRET_"),
		runner,
		CLIToolRuntimeConfig{MaxArgvLength: 10, AllowedCommands: []string{"cmd"}},
	)
	_, err := rt.Call(context.Background(), "long-args", fmt.Sprintf(`{"value": "%s"}`, strings.Repeat("x", 20)))
	if err == nil {
		t.Fatal("expected error for argv exceeding max length")
	}
}

// TestCLIToolRuntimeRefusesWhenAllowlistEmpty asserts the runtime fails closed
// when no command allowlist is configured. This is the defense-in-depth check
// that protects against stored Tool objects bypassing API admission.
func TestCLIToolRuntimeRefusesWhenAllowlistEmpty(t *testing.T) {
	runner := &mockCLICommandRunner{stdout: "ok", exitCode: 0}
	rt := NewCLIToolRuntime(
		NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
			"any-tool": {
				Type: "cli",
				Cli: resources.ToolCliSpec{
					Command: "kubectl",
					Output:  "stdout",
				},
			},
		}),
		NewEnvSecretResolver("TEST_SECRET_"),
		runner,
		DefaultCLIToolRuntimeConfig(), // AllowedCommands intentionally empty
	)
	_, err := rt.Call(context.Background(), "any-tool", "")
	if err == nil {
		t.Fatal("expected error when AllowedCommands is empty")
	}
	if !strings.Contains(err.Error(), "no command allowlist configured") {
		t.Fatalf("expected fail-closed error message, got: %v", err)
	}
	if runner.lastCmd != "" {
		t.Fatalf("runner should not have been invoked, but lastCmd=%q", runner.lastCmd)
	}
}

func TestCLIToolRuntimeEnvLiterals(t *testing.T) {
	runner := &mockCLICommandRunner{stdout: "ok", exitCode: 0}
	rt := newCLITestRuntime(map[string]resources.ToolSpec{
		"env-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "cmd",
				Output:  "stdout",
				Env:     map[string]string{"MY_VAR": "my_value"},
			},
		},
	}, runner)
	_, err := rt.Call(context.Background(), "env-tool", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range runner.lastEnv {
		if e == "MY_VAR=my_value" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected MY_VAR=my_value in env, got %v", runner.lastEnv)
	}
}
