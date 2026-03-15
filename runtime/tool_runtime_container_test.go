package agentruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/crds"
)

type captureContainerRunner struct {
	binary string
	args   []string
	stdin  string
	env    map[string]string

	stdout string
	stderr string
	err    error
	calls  int
}

type sleepingContainerRunner struct {
	delay time.Duration
}

func (r sleepingContainerRunner) Run(_ context.Context, _ string, _ []string, _ string, _ map[string]string) (string, string, error) {
	time.Sleep(r.delay)
	return "", "", nil
}

func (r *captureContainerRunner) Run(_ context.Context, binary string, args []string, stdin string, env map[string]string) (string, string, error) {
	r.calls++
	r.binary = binary
	r.args = append([]string(nil), args...)
	r.stdin = stdin
	r.env = copyStringMap(env)
	return r.stdout, r.stderr, r.err
}

type staticSecretResolver struct {
	values map[string]string
}

func (r staticSecretResolver) Resolve(_ context.Context, secretRef string) (string, error) {
	value, ok := r.values[secretRef]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

func TestContainerToolRuntimeExecutesHTTPInContainer(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {
			Type:     "http",
			Endpoint: "https://api.example/search",
		},
	})
	runner := &captureContainerRunner{stdout: "ok\n"}
	cfg := ContainerToolRuntimeConfig{
		RuntimeBinary: "docker",
		Image:         "curlimages/curl:8.8.0",
		Network:       "none",
		Memory:        "64m",
		CPUs:          "0.25",
		PidsLimit:     32,
		User:          "1000:1000",
		Shell:         "/bin/sh",
	}
	runtime := NewContainerToolRuntimeWithRunner(registry, cfg, runner)

	out, err := runtime.Call(context.Background(), "web_search", "topic=agents")
	if err != nil {
		t.Fatalf("container runtime call failed: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected trimmed stdout 'ok', got %q", out)
	}
	if runner.calls != 1 {
		t.Fatalf("expected 1 container command call, got %d", runner.calls)
	}
	if runner.binary != "docker" {
		t.Fatalf("expected runtime binary docker, got %q", runner.binary)
	}
	if runner.stdin != "topic=agents" {
		t.Fatalf("expected stdin passthrough, got %q", runner.stdin)
	}
	assertArgsContain(t, runner.args, []string{
		"run", "--rm", "-i",
		"--network", "none",
		"--read-only",
		"--cap-drop=ALL",
		"--security-opt", "no-new-privileges",
		"--memory", "64m",
		"--cpus", "0.25",
		"--pids-limit", "32",
		"-e", "TOOL_ENDPOINT=https://api.example/search",
		"--entrypoint", "/bin/sh",
		"curlimages/curl:8.8.0",
	})
}

func TestContainerToolRuntimeInjectsBearerSecretFromSecretRef(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {
			Type:     "http",
			Endpoint: "https://api.example/search",
			Auth:     crds.ToolAuth{SecretRef: "search-key"},
		},
	})
	runner := &captureContainerRunner{stdout: "ok"}
	runtime := NewContainerToolRuntimeWithRunnerAndSecrets(
		registry,
		DefaultContainerToolRuntimeConfig(),
		runner,
		staticSecretResolver{values: map[string]string{"search-key": "super-secret-token"}},
	)

	out, err := runtime.Call(context.Background(), "web_search", "topic=agents")
	if err != nil {
		t.Fatalf("expected successful secret-backed call, got %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected output ok, got %q", out)
	}
	assertArgsContain(t, runner.args, []string{"--env", "TOOL_AUTH_BEARER"})
	if got := runner.env["TOOL_AUTH_BEARER"]; got != "super-secret-token" {
		t.Fatalf("expected bearer env injection, got %q", got)
	}
}

func TestContainerToolRuntimeFailsWhenSecretRefCannotResolve(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {
			Type:     "http",
			Endpoint: "https://api.example/search",
			Auth:     crds.ToolAuth{SecretRef: "missing-secret"},
		},
	})
	runtime := NewContainerToolRuntimeWithRunnerAndSecrets(
		registry,
		DefaultContainerToolRuntimeConfig(),
		&captureContainerRunner{},
		staticSecretResolver{values: map[string]string{}},
	)

	_, err := runtime.Call(context.Background(), "web_search", "topic=agents")
	if err == nil {
		t.Fatal("expected secret resolution failure")
	}
	if !errors.Is(err, ErrToolSecretResolution) {
		t.Fatalf("expected ErrToolSecretResolution, got %v", err)
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeSecretResolution {
		t.Fatalf("expected code %q, got %q", ToolCodeSecretResolution, code)
	}
	if reason != ToolReasonSecretResolution {
		t.Fatalf("expected reason %q, got %q", ToolReasonSecretResolution, reason)
	}
	if retryable {
		t.Fatal("expected secret resolution failure to be non-retryable")
	}
}

func TestBuildGovernedToolRuntimeUsesContainerIsolationBackend(t *testing.T) {
	lookup := staticToolLookup{
		items: map[string]crds.Tool{
			"default/danger_tool": {
				Metadata: crds.ObjectMeta{Name: "danger_tool", Namespace: "default"},
				Spec: crds.ToolSpec{
					Type:      "http",
					Endpoint:  "https://api.example/danger",
					RiskLevel: "high",
					Runtime: crds.ToolRuntimePolicy{
						Timeout:       "1s",
						IsolationMode: "sandboxed",
						Retry: crds.ToolRetryPolicy{
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
	base := &scriptedToolRuntime{result: "base"}
	runner := &captureContainerRunner{stdout: "isolated"}
	isolated := NewContainerToolRuntimeWithRunner(nil, DefaultContainerToolRuntimeConfig(), runner)

	governed := BuildGovernedToolRuntimeForAgent(base, isolated, lookup, "default", []string{"danger_tool"})
	if governed == nil {
		t.Fatal("expected governed runtime instance")
	}
	out, err := governed.Call(context.Background(), "danger_tool", "payload")
	if err != nil {
		t.Fatalf("governed call failed: %v", err)
	}
	if out != "isolated" {
		t.Fatalf("expected isolated result, got %q", out)
	}
	if runner.calls != 1 {
		t.Fatalf("expected container runner to be called once, got %d", runner.calls)
	}
	if base.calls != 0 {
		t.Fatalf("expected base runtime calls=0 for sandboxed tool, got %d", base.calls)
	}
}

func TestContainerToolRuntimeMapsContextDeadlineToTimeoutError(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {
			Type:     "http",
			Endpoint: "https://api.example/search",
		},
	})
	runtime := NewContainerToolRuntimeWithRunner(
		registry,
		DefaultContainerToolRuntimeConfig(),
		sleepingContainerRunner{delay: 250 * time.Millisecond},
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := runtime.Call(ctx, "web_search", "topic=agents")
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

func TestContainerToolRuntimeMapsCanceledContextToCanceledError(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {
			Type:     "http",
			Endpoint: "https://api.example/search",
		},
	})
	runtime := NewContainerToolRuntimeWithRunner(
		registry,
		DefaultContainerToolRuntimeConfig(),
		sleepingContainerRunner{delay: 250 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, err := runtime.Call(ctx, "web_search", "topic=agents")
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

func TestEnvSecretResolverSupportsPrefixedNormalizedKey(t *testing.T) {
	t.Setenv("ORLOJ_SECRET_SEARCH_API_KEY", "token-123")
	resolver := NewEnvSecretResolver("ORLOJ_SECRET_")
	value, err := resolver.Resolve(context.Background(), "search-api-key")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if value != "token-123" {
		t.Fatalf("unexpected resolved value %q", value)
	}
}

func assertArgsContain(t *testing.T, args []string, expected []string) {
	t.Helper()
	for i := 0; i < len(expected); i++ {
		if !sliceContains(args, expected[i]) {
			t.Fatalf("expected args to contain %q, args=%v", expected[i], args)
		}
	}
}

func sliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
