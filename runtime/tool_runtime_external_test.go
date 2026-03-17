package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/crds"
)

func TestExternalToolRuntimeDelegatesContractRequest(t *testing.T) {
	contractResp := ToolExecutionResponse{
		ToolContractVersion: "v1",
		RequestID:           "ext-test",
		Status:              "ok",
		Output:              ToolExecutionOutput{Result: "external result"},
		Usage:               ToolExecutionUsage{Attempt: 1},
	}
	body, _ := json.Marshal(contractResp)

	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"ext_tool": {
			Type:     "external",
			Endpoint: "https://ext.example.com/execute",
		},
	})
	doer := &fakeHTTPDoer{statusCode: 200, body: string(body)}
	runtime := NewExternalToolRuntime(registry, nil, doer)

	out, err := runtime.Call(context.Background(), "ext_tool", `{"key":"value"}`)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if out != "external result" {
		t.Fatalf("expected 'external result', got %q", out)
	}
	if doer.calls != 1 {
		t.Fatalf("expected 1 call, got %d", doer.calls)
	}
	if doer.lastReq.URL.String() != "https://ext.example.com/execute" {
		t.Fatalf("unexpected URL %q", doer.lastReq.URL.String())
	}
	if ct := doer.lastReq.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}
	if cv := doer.lastReq.Header.Get("X-Tool-Contract-Version"); cv != "v1" {
		t.Fatalf("expected X-Tool-Contract-Version v1, got %q", cv)
	}

	var sentReq ToolExecutionRequest
	reqBody, _ := io.ReadAll(doer.lastReq.Body)
	if err := json.Unmarshal(reqBody, &sentReq); err != nil {
		t.Fatalf("failed to parse sent request body: %v", err)
	}
	if sentReq.Tool.Name != "ext_tool" {
		t.Fatalf("expected tool.name=ext_tool in request, got %q", sentReq.Tool.Name)
	}
	if sentReq.ToolContractVersion != "v1" {
		t.Fatalf("expected tool_contract_version=v1, got %q", sentReq.ToolContractVersion)
	}
}

func TestExternalToolRuntimePropagatesContractError(t *testing.T) {
	contractResp := ToolExecutionResponse{
		ToolContractVersion: "v1",
		RequestID:           "ext-err",
		Status:              "error",
		Usage:               ToolExecutionUsage{Attempt: 1},
		Error: &ToolExecutionFailure{
			Code:      "execution_failed",
			Reason:    "tool_backend_failure",
			Retryable: true,
			Message:   "downstream error",
		},
	}
	body, _ := json.Marshal(contractResp)
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"ext_tool": {Type: "external", Endpoint: "https://ext.example.com/execute"},
	})
	doer := &fakeHTTPDoer{statusCode: 200, body: string(body)}
	runtime := NewExternalToolRuntime(registry, nil, doer)

	_, err := runtime.Call(context.Background(), "ext_tool", "input")
	if err == nil {
		t.Fatal("expected error from contract error response")
	}
	code, _, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeExecutionFailed {
		t.Fatalf("expected code %q, got %q", ToolCodeExecutionFailed, code)
	}
	if !retryable {
		t.Fatal("expected retryable=true")
	}
}

func TestExternalToolRuntimeInjectsAuth(t *testing.T) {
	contractResp := ToolExecutionResponse{
		ToolContractVersion: "v1",
		Status:              "ok",
		Output:              ToolExecutionOutput{Result: "authed"},
		Usage:               ToolExecutionUsage{Attempt: 1},
	}
	body, _ := json.Marshal(contractResp)
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"ext_tool": {
			Type:     "external",
			Endpoint: "https://ext.example.com/execute",
			Auth:     crds.ToolAuth{SecretRef: "ext-secret"},
		},
	})
	doer := &fakeHTTPDoer{statusCode: 200, body: string(body)}
	secrets := staticSecretResolver{values: map[string]string{"ext-secret": "ext-token-value"}}
	runtime := NewExternalToolRuntime(registry, secrets, doer)

	_, err := runtime.Call(context.Background(), "ext_tool", "input")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	authHeader := doer.lastReq.Header.Get("Authorization")
	if authHeader != "Bearer ext-token-value" {
		t.Fatalf("expected 'Bearer ext-token-value', got %q", authHeader)
	}
}

func TestExternalToolRuntimeFailsOnMissingEndpoint(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"ext_tool": {Type: "external"},
	})
	runtime := NewExternalToolRuntime(registry, nil, nil)

	_, err := runtime.Call(context.Background(), "ext_tool", "input")
	if err == nil {
		t.Fatal("expected missing endpoint error")
	}
	code, _, _, _ := ToolErrorMeta(err)
	if code != ToolCodeRuntimePolicyInvalid {
		t.Fatalf("expected code %q, got %q", ToolCodeRuntimePolicyInvalid, code)
	}
}

func TestExternalToolRuntimeFailsOnInvalidContractResponse(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"ext_tool": {Type: "external", Endpoint: "https://ext.example.com/execute"},
	})
	doer := &fakeHTTPDoer{statusCode: 200, body: "not-json"}
	runtime := NewExternalToolRuntime(registry, nil, doer)

	_, err := runtime.Call(context.Background(), "ext_tool", "input")
	if err == nil {
		t.Fatal("expected invalid contract response error")
	}
	code, _, _, _ := ToolErrorMeta(err)
	if code != ToolCodeExecutionFailed {
		t.Fatalf("expected code %q, got %q", ToolCodeExecutionFailed, code)
	}
}

func TestExternalToolRuntimeMapsHTTPTimeout(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"ext_tool": {Type: "external", Endpoint: "https://ext.example.com/execute"},
	})
	doer := &fakeHTTPDoer{err: context.DeadlineExceeded}
	runtime := NewExternalToolRuntime(registry, nil, doer)

	_, err := runtime.Call(context.Background(), "ext_tool", "input")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	code, _, _, _ := ToolErrorMeta(err)
	if code != ToolCodeTimeout {
		t.Fatalf("expected code %q, got %q", ToolCodeTimeout, code)
	}
}

func TestExternalToolRuntimeMapsHTTPCanceled(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"ext_tool": {Type: "external", Endpoint: "https://ext.example.com/execute"},
	})
	doer := &fakeHTTPDoer{err: context.Canceled}
	runtime := NewExternalToolRuntime(registry, nil, doer)

	_, err := runtime.Call(context.Background(), "ext_tool", "input")
	if err == nil {
		t.Fatal("expected canceled error")
	}
	code, _, _, _ := ToolErrorMeta(err)
	if code != ToolCodeCanceled {
		t.Fatalf("expected code %q, got %q", ToolCodeCanceled, code)
	}
}

func TestExternalToolRuntimeRegisteredInDefaultRegistry(t *testing.T) {
	registry := DefaultToolIsolationBackendRegistry()
	modes := registry.Modes()
	found := false
	for _, mode := range modes {
		if mode == "external" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'external' in registered modes, got %v", modes)
	}
}

func TestExternalToolRuntimeHTTPErrorMapping(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"ext_tool": {Type: "external", Endpoint: "https://ext.example.com/execute"},
	})

	tests := []struct {
		name      string
		status    int
		retryable bool
	}{
		{"server_error", 500, true},
		{"rate_limit", 429, true},
		{"bad_request", 400, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doer := &fakeHTTPDoer{statusCode: tt.status, body: "error"}
			runtime := NewExternalToolRuntime(registry, nil, doer)

			_, err := runtime.Call(context.Background(), "ext_tool", "input")
			if err == nil {
				t.Fatal("expected error")
			}
			_, _, retryable, ok := ToolErrorMeta(err)
			if !ok {
				t.Fatal("expected tool error metadata")
			}
			if retryable != tt.retryable {
				t.Fatalf("expected retryable=%t, got %t", tt.retryable, retryable)
			}
		})
	}
}

// fakeHTTPDoer.lastReq.Body is consumed after the first read; capture a copy
// for inspection by wrapping the fake's Do to tee the body.
func init() {
	// Patch the fakeHTTPDoer to capture request bodies for assertion.
	// The actual implementation reads the body in Do, so we need a
	// helper that clones it.
}

func TestExternalToolRuntimeSecretResolutionFailure(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"ext_tool": {
			Type:     "external",
			Endpoint: "https://ext.example.com/execute",
			Auth:     crds.ToolAuth{SecretRef: "missing"},
		},
	})
	secrets := staticSecretResolver{values: map[string]string{}}
	runtime := NewExternalToolRuntime(registry, secrets, nil)

	_, err := runtime.Call(context.Background(), "ext_tool", "input")
	if err == nil {
		t.Fatal("expected secret resolution failure")
	}
	if !errors.Is(err, ErrToolSecretResolution) {
		t.Fatalf("expected ErrToolSecretResolution, got %v", err)
	}
}

// Override fakeHTTPDoer.Do to capture the request body before it's read.
type bodyCapturingDoer struct {
	inner   *fakeHTTPDoer
	lastBody []byte
}

func (d *bodyCapturingDoer) Do(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		data, _ := io.ReadAll(req.Body)
		d.lastBody = data
		req.Body = io.NopCloser(bytes.NewReader(data))
	}
	return d.inner.Do(req)
}

func TestExternalToolRuntimeSendsContractPayload(t *testing.T) {
	contractResp := ToolExecutionResponse{
		ToolContractVersion: "v1",
		Status:              "ok",
		Output:              ToolExecutionOutput{Result: "ok"},
		Usage:               ToolExecutionUsage{Attempt: 1},
	}
	body, _ := json.Marshal(contractResp)
	inner := &fakeHTTPDoer{statusCode: 200, body: string(body)}
	doer := &bodyCapturingDoer{inner: inner}

	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"ext_tool": {
			Type:         "external",
			Endpoint:     "https://ext.example.com/execute",
			Capabilities: []string{"web.read"},
			RiskLevel:    "medium",
		},
	})
	runtime := NewExternalToolRuntime(registry, nil, doer)

	_, err := runtime.Call(context.Background(), "ext_tool", `{"query":"test"}`)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	var sentReq ToolExecutionRequest
	if err := json.Unmarshal(doer.lastBody, &sentReq); err != nil {
		t.Fatalf("failed to unmarshal sent request: %v", err)
	}
	if sentReq.Tool.Name != "ext_tool" {
		t.Fatalf("expected tool.name=ext_tool, got %q", sentReq.Tool.Name)
	}
	if sentReq.Runtime.Mode != "external" {
		t.Fatalf("expected runtime.mode=external, got %q", sentReq.Runtime.Mode)
	}
	if !strings.Contains(sentReq.InputRaw, "query") {
		t.Fatalf("expected input passthrough, got %q", sentReq.InputRaw)
	}
}
