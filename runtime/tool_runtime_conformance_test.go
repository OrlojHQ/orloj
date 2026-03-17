package agentruntime_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/crds"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/runtime/conformance"
	conformancecases "github.com/OrlojHQ/orloj/runtime/conformance/cases"
)

type funcRuntime struct {
	call func(ctx context.Context, tool string, input string) (string, error)
}

func (r funcRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
	return r.call(ctx, tool, input)
}

type denyAuthorizer struct{}

func (a denyAuthorizer) Authorize(tool string, _ crds.ToolSpec) error {
	if !strings.EqualFold(strings.TrimSpace(tool), "vector_db") {
		return nil
	}
	return agentruntime.NewToolDeniedError(
		"policy permission denied for tool=vector_db",
		map[string]string{
			"tool":     "vector_db",
			"required": "tool:vector_db:invoke",
		},
		agentruntime.ErrToolPermissionDenied,
	)
}

type fakeContainerRunner struct {
	stdout       string
	stderr       string
	err          error
	delay        time.Duration
	delayOnToken string
}

func (r *fakeContainerRunner) Run(_ context.Context, _ string, _ []string, stdin string, _ map[string]string) (string, string, error) {
	if r.delay > 0 {
		if strings.TrimSpace(r.delayOnToken) == "" || strings.Contains(stdin, r.delayOnToken) {
			time.Sleep(r.delay)
		}
	}
	return r.stdout, r.stderr, r.err
}

type fakeSecretResolver struct {
	values map[string]string
}

func (r fakeSecretResolver) Resolve(_ context.Context, secretRef string) (string, error) {
	value, ok := r.values[strings.TrimSpace(secretRef)]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

type fakeWASMExecutor struct {
	delay        time.Duration
	delayOnToken string
}

func (e fakeWASMExecutor) Execute(ctx context.Context, req agentruntime.WASMToolExecuteRequest) (agentruntime.WASMToolExecuteResponse, error) {
	if e.delay > 0 {
		if strings.TrimSpace(e.delayOnToken) == "" || strings.Contains(req.Input, e.delayOnToken) {
			time.Sleep(e.delay)
		}
	}
	return agentruntime.WASMToolExecuteResponse{Output: "ok:" + req.Tool}, nil
}

type fakeWASMFactory struct {
	executor agentruntime.WASMToolExecutor
}

func (f fakeWASMFactory) Build(_ context.Context, _ agentruntime.WASMToolRuntimeConfig) (agentruntime.WASMToolExecutor, error) {
	return f.executor, nil
}

func TestGovernedToolRuntimeConformanceSuite(t *testing.T) {
	base := funcRuntime{
		call: func(ctx context.Context, tool string, input string) (string, error) {
			switch strings.ToLower(strings.TrimSpace(tool)) {
			case "timeout_tool":
				<-ctx.Done()
				return "", ctx.Err()
			case "stuck_tool":
				time.Sleep(250 * time.Millisecond)
				return "late-result", nil
			default:
				return "ok:" + tool + ":" + input, nil
			}
		},
	}
	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {
			Runtime: crds.ToolRuntimePolicy{
				Timeout: "100ms",
				Retry: crds.ToolRetryPolicy{
					MaxAttempts: 1,
					Backoff:     "0s",
					MaxBackoff:  "1s",
					Jitter:      "none",
				},
			},
		},
		"vector_db": {
			Runtime: crds.ToolRuntimePolicy{
				Timeout: "100ms",
				Retry: crds.ToolRetryPolicy{
					MaxAttempts: 1,
					Backoff:     "0s",
					MaxBackoff:  "1s",
					Jitter:      "none",
				},
			},
		},
		"timeout_tool": {
			Runtime: crds.ToolRuntimePolicy{
				Timeout: "1ms",
				Retry: crds.ToolRetryPolicy{
					MaxAttempts: 1,
					Backoff:     "0s",
					MaxBackoff:  "1s",
					Jitter:      "none",
				},
			},
		},
		"stuck_tool": {
			Runtime: crds.ToolRuntimePolicy{
				Timeout: "10ms",
				Retry: crds.ToolRetryPolicy{
					MaxAttempts: 1,
					Backoff:     "0s",
					MaxBackoff:  "1s",
					Jitter:      "none",
				},
			},
		},
	})
	governed := agentruntime.NewGovernedToolRuntimeWithAuthorizer(base, nil, registry, denyAuthorizer{}, true)
	executor := agentruntime.NewToolContractExecutor(governed)

	cases := []conformance.Case{
		{
			Name:     "governed-success",
			Request:  conformancecases.BaseRequest("req-governed-success", "web_search"),
			Expected: conformance.Expected{Status: agentruntime.ToolExecutionStatusOK},
		},
		{
			Name:    "governed-denied",
			Request: conformancecases.BaseRequest("req-governed-denied", "vector_db"),
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusDenied,
				ErrorCode: agentruntime.ToolCodePermissionDenied,
				Reason:    agentruntime.ToolReasonPermissionDenied,
				Retryable: conformancecases.BoolPtr(false),
			},
		},
		{
			Name:    "governed-timeout",
			Request: conformancecases.BaseRequest("req-governed-timeout", "timeout_tool"),
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeTimeout,
				Reason:    agentruntime.ToolReasonExecutionTimeout,
				Retryable: conformancecases.BoolPtr(true),
			},
		},
		{
			Name:    "governed-unsupported",
			Request: conformancecases.BaseRequest("req-governed-unsupported", "missing_tool"),
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeUnsupportedTool,
				Reason:    agentruntime.ToolReasonToolUnsupported,
				Retryable: conformancecases.BoolPtr(false),
			},
		},
		conformancecases.BoundedTimeoutCase(
			"governed-bounded-timeout-latency",
			conformancecases.BaseRequest("req-governed-bounded-timeout", "stuck_tool"),
			0,
			120*time.Millisecond,
			conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeTimeout,
				Reason:    agentruntime.ToolReasonExecutionTimeout,
				Retryable: conformancecases.BoolPtr(true),
			},
		),
		conformancecases.ImmediateCancelCase(
			"governed-canceled",
			conformancecases.BaseRequest("req-governed-canceled", "timeout_tool"),
			80*time.Millisecond,
			conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeCanceled,
				Reason:    agentruntime.ToolReasonExecutionCanceled,
				Retryable: conformancecases.BoolPtr(false),
			},
		),
		conformancecases.UnknownVersionCase("req-governed-unknown-version", "web_search"),
	}
	failures := conformance.RunCases(context.Background(), executor, cases)
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Errorf("case=%s err=%v", failure.Case, failure.Err)
		}
		t.Fatalf("governed runtime conformance failures: %d", len(failures))
	}
}

func TestContainerToolRuntimeConformanceSuite(t *testing.T) {
	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {
			Type:     "http",
			Endpoint: "https://api.example/search",
		},
		"missing_endpoint": {
			Type: "http",
		},
		"unsupported_rpc": {
			Type:     "rpc",
			Endpoint: "https://rpc.example/query",
		},
		"secret_lookup": {
			Type:     "http",
			Endpoint: "https://api.example/private",
			Auth: crds.ToolAuth{
				SecretRef: "missing-secret",
			},
		},
		"slow_tool": {
			Type:     "http",
			Endpoint: "https://api.example/slow",
		},
	})
	runtime := agentruntime.NewContainerToolRuntimeWithRunnerAndSecrets(
		registry,
		agentruntime.DefaultContainerToolRuntimeConfig(),
		&fakeContainerRunner{
			stdout:       "ok",
			delay:        250 * time.Millisecond,
			delayOnToken: "slow_tool",
		},
		fakeSecretResolver{values: map[string]string{}},
	)
	executor := agentruntime.NewToolContractExecutor(runtime)

	cases := []conformance.Case{
		{
			Name:     "container-success",
			Request:  conformancecases.BaseRequest("req-container-success", "web_search"),
			Expected: conformance.Expected{Status: agentruntime.ToolExecutionStatusOK},
		},
		{
			Name:    "container-runtime-policy-invalid",
			Request: conformancecases.BaseRequest("req-container-runtime-policy-invalid", "missing_endpoint"),
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeRuntimePolicyInvalid,
				Reason:    agentruntime.ToolReasonRuntimePolicyInvalid,
				Retryable: conformancecases.BoolPtr(false),
			},
		},
		{
			Name:    "container-unsupported",
			Request: conformancecases.BaseRequest("req-container-unsupported", "unsupported_rpc"),
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeUnsupportedTool,
				Reason:    agentruntime.ToolReasonToolUnsupported,
				Retryable: conformancecases.BoolPtr(false),
			},
		},
		{
			Name:    "container-secret-resolution",
			Request: conformancecases.BaseRequest("req-container-secret-resolution", "secret_lookup"),
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeSecretResolution,
				Reason:    agentruntime.ToolReasonSecretResolution,
				Retryable: conformancecases.BoolPtr(false),
			},
		},
		conformancecases.BoundedTimeoutCase(
			"container-bounded-timeout-latency",
			conformancecases.BaseRequest("req-container-bounded-timeout", "slow_tool"),
			15*time.Millisecond,
			120*time.Millisecond,
			conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeTimeout,
				Reason:    agentruntime.ToolReasonExecutionTimeout,
				Retryable: conformancecases.BoolPtr(true),
			},
		),
		conformancecases.ImmediateCancelCase(
			"container-canceled",
			conformancecases.BaseRequest("req-container-canceled", "web_search"),
			80*time.Millisecond,
			conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeCanceled,
				Reason:    agentruntime.ToolReasonExecutionCanceled,
				Retryable: conformancecases.BoolPtr(false),
			},
		),
		conformancecases.UnknownVersionCase("req-container-unknown-version", "web_search"),
	}
	failures := conformance.RunCases(context.Background(), executor, cases)
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Errorf("case=%s err=%v", failure.Case, failure.Err)
		}
		t.Fatalf("container runtime conformance failures: %d", len(failures))
	}
}

func TestHTTPToolRuntimeConformanceSuite(t *testing.T) {
	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {
			Type:     "http",
			Endpoint: "https://api.example/search",
		},
		"missing_endpoint": {
			Type: "http",
		},
		"secret_lookup": {
			Type:     "http",
			Endpoint: "https://api.example/private",
			Auth: crds.ToolAuth{
				SecretRef: "missing-secret",
			},
		},
	})
	doer := &httpConformanceDoer{}
	runtime := agentruntime.NewHTTPToolClient(
		registry,
		fakeSecretResolver{values: map[string]string{}},
		doer,
	)
	executor := agentruntime.NewToolContractExecutor(runtime)

	cases := []conformance.Case{
		{
			Name:     "http-success",
			Request:  conformancecases.BaseRequest("req-http-success", "web_search"),
			Expected: conformance.Expected{Status: agentruntime.ToolExecutionStatusOK},
		},
		{
			Name:    "http-runtime-policy-invalid",
			Request: conformancecases.BaseRequest("req-http-runtime-policy-invalid", "missing_endpoint"),
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeRuntimePolicyInvalid,
				Reason:    agentruntime.ToolReasonRuntimePolicyInvalid,
				Retryable: conformancecases.BoolPtr(false),
			},
		},
		{
			Name:    "http-unsupported",
			Request: conformancecases.BaseRequest("req-http-unsupported", "missing_tool"),
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeUnsupportedTool,
				Reason:    agentruntime.ToolReasonToolUnsupported,
				Retryable: conformancecases.BoolPtr(false),
			},
		},
		{
			Name:    "http-secret-resolution",
			Request: conformancecases.BaseRequest("req-http-secret-resolution", "secret_lookup"),
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeSecretResolution,
				Reason:    agentruntime.ToolReasonSecretResolution,
				Retryable: conformancecases.BoolPtr(false),
			},
		},
		conformancecases.UnknownVersionCase("req-http-unknown-version", "web_search"),
	}
	failures := conformance.RunCases(context.Background(), executor, cases)
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Errorf("case=%s err=%v", failure.Case, failure.Err)
		}
		t.Fatalf("HTTP runtime conformance failures: %d", len(failures))
	}
}

func TestExternalToolRuntimeConformanceSuite(t *testing.T) {
	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"ext_tool": {
			Type:     "external",
			Endpoint: "https://ext.example/execute",
		},
		"missing_endpoint": {
			Type: "external",
		},
	})
	doer := &externalConformanceDoer{}
	runtime := agentruntime.NewExternalToolRuntime(registry, nil, doer)
	executor := agentruntime.NewToolContractExecutor(runtime)

	cases := []conformance.Case{
		{
			Name:     "external-success",
			Request:  conformancecases.BaseRequest("req-ext-success", "ext_tool"),
			Expected: conformance.Expected{Status: agentruntime.ToolExecutionStatusOK},
		},
		{
			Name:    "external-runtime-policy-invalid",
			Request: conformancecases.BaseRequest("req-ext-runtime-policy-invalid", "missing_endpoint"),
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeRuntimePolicyInvalid,
				Reason:    agentruntime.ToolReasonRuntimePolicyInvalid,
				Retryable: conformancecases.BoolPtr(false),
			},
		},
		{
			Name:    "external-unsupported",
			Request: conformancecases.BaseRequest("req-ext-unsupported", "missing_tool"),
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeUnsupportedTool,
				Reason:    agentruntime.ToolReasonToolUnsupported,
				Retryable: conformancecases.BoolPtr(false),
			},
		},
		conformancecases.UnknownVersionCase("req-ext-unknown-version", "ext_tool"),
	}
	failures := conformance.RunCases(context.Background(), executor, cases)
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Errorf("case=%s err=%v", failure.Case, failure.Err)
		}
		t.Fatalf("external runtime conformance failures: %d", len(failures))
	}
}

// httpConformanceDoer returns a success response for any valid request.
type httpConformanceDoer struct{}

func (d *httpConformanceDoer) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("conformance-ok")),
	}, nil
}

// externalConformanceDoer returns a valid ToolExecutionResponse for any request.
type externalConformanceDoer struct{}

func (d *externalConformanceDoer) Do(req *http.Request) (*http.Response, error) {
	resp := agentruntime.ToolExecutionResponse{
		ToolContractVersion: "v1",
		RequestID:           "ext-conformance",
		Status:              "ok",
		Output:              agentruntime.ToolExecutionOutput{Result: "ext-conformance-ok"},
		Usage:               agentruntime.ToolExecutionUsage{Attempt: 1},
	}
	body, _ := json.Marshal(resp)
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}, nil
}

func TestWASMStubRuntimeFailsClosed(t *testing.T) {
	executor := agentruntime.NewToolContractExecutor(agentruntime.NewUnsupportedWASMToolRuntime())
	cases := []conformance.Case{
		{
			Name:     "wasm-stub-isolation-unavailable",
			Request:  conformancecases.BaseRequest("req-wasm-stub", "danger_tool"),
			Expected: conformance.Expected{Status: agentruntime.ToolExecutionStatusError, ErrorCode: agentruntime.ToolCodeIsolationUnavailable, Reason: agentruntime.ToolReasonIsolationUnavailable, Retryable: conformancecases.BoolPtr(false)},
		},
		conformancecases.UnknownVersionCase("req-wasm-stub-unknown-version", "danger_tool"),
	}
	failures := conformance.RunCases(context.Background(), executor, cases)
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Errorf("case=%s err=%v", failure.Case, failure.Err)
		}
		t.Fatalf("wasm stub conformance failures: %d", len(failures))
	}
}

func TestWASMRuntimeScaffoldConformanceSuite(t *testing.T) {
	registry := agentruntime.NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"wasm_tool": {
			Type:      "custom",
			RiskLevel: "high",
		},
		"slow_wasm_tool": {
			Type:      "custom",
			RiskLevel: "high",
		},
	})
	runtime := agentruntime.NewWASMToolRuntimeWithFactory(
		registry,
		fakeWASMFactory{
			executor: fakeWASMExecutor{
				delay:        250 * time.Millisecond,
				delayOnToken: "slow_wasm_tool",
			},
		},
		agentruntime.WASMToolRuntimeConfig{
			ModulePath: "/tmp/test.wasm",
			Entrypoint: "run",
		},
	)
	executor := agentruntime.NewToolContractExecutor(runtime)
	cases := []conformance.Case{
		{
			Name:     "wasm-scaffold-success",
			Request:  conformancecases.BaseRequest("req-wasm-success", "wasm_tool"),
			Expected: conformance.Expected{Status: agentruntime.ToolExecutionStatusOK},
		},
		{
			Name:    "wasm-scaffold-unsupported",
			Request: conformancecases.BaseRequest("req-wasm-unsupported", "missing_wasm_tool"),
			Expected: conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeUnsupportedTool,
				Reason:    agentruntime.ToolReasonToolUnsupported,
				Retryable: conformancecases.BoolPtr(false),
			},
		},
		conformancecases.BoundedTimeoutCase(
			"wasm-scaffold-timeout",
			conformancecases.BaseRequest("req-wasm-timeout", "slow_wasm_tool"),
			15*time.Millisecond,
			120*time.Millisecond,
			conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeTimeout,
				Reason:    agentruntime.ToolReasonExecutionTimeout,
				Retryable: conformancecases.BoolPtr(true),
			},
		),
		conformancecases.ImmediateCancelCase(
			"wasm-scaffold-canceled",
			conformancecases.BaseRequest("req-wasm-canceled", "wasm_tool"),
			80*time.Millisecond,
			conformance.Expected{
				Status:    agentruntime.ToolExecutionStatusError,
				ErrorCode: agentruntime.ToolCodeCanceled,
				Reason:    agentruntime.ToolReasonExecutionCanceled,
				Retryable: conformancecases.BoolPtr(false),
			},
		),
	}
	failures := conformance.RunCases(context.Background(), executor, cases)
	if len(failures) > 0 {
		for _, failure := range failures {
			t.Errorf("case=%s err=%v", failure.Case, failure.Err)
		}
		t.Fatalf("wasm runtime scaffold conformance failures: %d", len(failures))
	}
}
