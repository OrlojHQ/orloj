package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/OrlojHQ/orloj/crds"
)

type fakeHTTPDoer struct {
	statusCode int
	body       string
	err        error
	calls      int
	lastReq    *http.Request
}

func (d *fakeHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	d.calls++
	d.lastReq = req
	if d.err != nil {
		return nil, d.err
	}
	return &http.Response{
		StatusCode: d.statusCode,
		Body:       io.NopCloser(bytes.NewReader([]byte(d.body))),
	}, nil
}

func TestHTTPToolClientExecutesHTTPToolEndpoint(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {
			Type:     "http",
			Endpoint: "https://api.example.com/search",
		},
	})
	doer := &fakeHTTPDoer{statusCode: 200, body: "search results"}
	client := NewHTTPToolClient(registry, nil, doer)

	out, err := client.Call(context.Background(), "web_search", `{"q":"orloj"}`)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if out != "search results" {
		t.Fatalf("expected 'search results', got %q", out)
	}
	if doer.calls != 1 {
		t.Fatalf("expected 1 call, got %d", doer.calls)
	}
	if doer.lastReq.URL.String() != "https://api.example.com/search" {
		t.Fatalf("unexpected URL %q", doer.lastReq.URL.String())
	}
	if doer.lastReq.Method != http.MethodPost {
		t.Fatalf("expected POST method, got %q", doer.lastReq.Method)
	}
}

func TestHTTPToolClientParsesToolContractResponse(t *testing.T) {
	contractResp := ToolExecutionResponse{
		ToolContractVersion: "v1",
		RequestID:           "req-1",
		Status:              "ok",
		Output:              ToolExecutionOutput{Result: "contract output"},
	}
	body, _ := json.Marshal(contractResp)
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {Endpoint: "https://api.example.com/search"},
	})
	doer := &fakeHTTPDoer{statusCode: 200, body: string(body)}
	client := NewHTTPToolClient(registry, nil, doer)

	out, err := client.Call(context.Background(), "web_search", "input")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if out != "contract output" {
		t.Fatalf("expected 'contract output', got %q", out)
	}
}

func TestHTTPToolClientParsesToolContractErrorResponse(t *testing.T) {
	contractResp := ToolExecutionResponse{
		ToolContractVersion: "v1",
		RequestID:           "req-1",
		Status:              "error",
		Error: &ToolExecutionFailure{
			Code:      "execution_failed",
			Reason:    "tool_backend_failure",
			Retryable: true,
			Message:   "backend is down",
		},
	}
	body, _ := json.Marshal(contractResp)
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {Endpoint: "https://api.example.com/search"},
	})
	doer := &fakeHTTPDoer{statusCode: 200, body: string(body)}
	client := NewHTTPToolClient(registry, nil, doer)

	_, err := client.Call(context.Background(), "web_search", "input")
	if err == nil {
		t.Fatal("expected error from contract error response")
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeExecutionFailed {
		t.Fatalf("expected code %q, got %q", ToolCodeExecutionFailed, code)
	}
	if reason != ToolReasonBackendFailure {
		t.Fatalf("expected reason %q, got %q", ToolReasonBackendFailure, reason)
	}
	if !retryable {
		t.Fatal("expected retryable=true")
	}
}

func TestHTTPToolClientInjectsAuthBearer(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {
			Endpoint: "https://api.example.com/search",
			Auth:     crds.ToolAuth{SecretRef: "search-key"},
		},
	})
	doer := &fakeHTTPDoer{statusCode: 200, body: "ok"}
	secrets := staticSecretResolver{values: map[string]string{"search-key": "my-token"}}
	client := NewHTTPToolClient(registry, secrets, doer)

	out, err := client.Call(context.Background(), "web_search", "input")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected 'ok', got %q", out)
	}
	authHeader := doer.lastReq.Header.Get("Authorization")
	if authHeader != "Bearer my-token" {
		t.Fatalf("expected 'Bearer my-token' auth header, got %q", authHeader)
	}
}

func TestHTTPToolClientFailsWhenSecretResolutionFails(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {
			Endpoint: "https://api.example.com/search",
			Auth:     crds.ToolAuth{SecretRef: "missing"},
		},
	})
	doer := &fakeHTTPDoer{statusCode: 200, body: "ok"}
	secrets := staticSecretResolver{values: map[string]string{}}
	client := NewHTTPToolClient(registry, secrets, doer)

	_, err := client.Call(context.Background(), "web_search", "input")
	if err == nil {
		t.Fatal("expected secret resolution failure")
	}
	if !errors.Is(err, ErrToolSecretResolution) {
		t.Fatalf("expected ErrToolSecretResolution, got %v", err)
	}
	code, _, _, _ := ToolErrorMeta(err)
	if code != ToolCodeSecretResolution {
		t.Fatalf("expected code %q, got %q", ToolCodeSecretResolution, code)
	}
}

func TestHTTPToolClientFailsOnMissingEndpoint(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {Type: "http"},
	})
	client := NewHTTPToolClient(registry, nil, nil)

	_, err := client.Call(context.Background(), "web_search", "input")
	if err == nil {
		t.Fatal("expected missing endpoint error")
	}
	code, _, _, _ := ToolErrorMeta(err)
	if code != ToolCodeRuntimePolicyInvalid {
		t.Fatalf("expected code %q, got %q", ToolCodeRuntimePolicyInvalid, code)
	}
}

func TestHTTPToolClientFailsOnUnsupportedTool(t *testing.T) {
	client := NewHTTPToolClient(NewStaticToolCapabilityRegistry(nil), nil, nil)

	_, err := client.Call(context.Background(), "unknown_tool", "input")
	if err == nil {
		t.Fatal("expected unsupported tool error")
	}
	if !errors.Is(err, ErrUnsupportedTool) {
		t.Fatalf("expected ErrUnsupportedTool, got %v", err)
	}
}

func TestHTTPToolClientMapsHTTPErrorCodes(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {Endpoint: "https://api.example.com/search"},
	})

	tests := []struct {
		name      string
		status    int
		retryable bool
	}{
		{"client_error_400", 400, false},
		{"rate_limit_429", 429, true},
		{"server_error_500", 500, true},
		{"bad_gateway_502", 502, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doer := &fakeHTTPDoer{statusCode: tt.status, body: "error response"}
			client := NewHTTPToolClient(registry, nil, doer)

			_, err := client.Call(context.Background(), "web_search", "input")
			if err == nil {
				t.Fatal("expected error")
			}
			_, _, retryable, ok := ToolErrorMeta(err)
			if !ok {
				t.Fatal("expected tool error metadata")
			}
			if retryable != tt.retryable {
				t.Fatalf("expected retryable=%t for HTTP %d, got %t", tt.retryable, tt.status, retryable)
			}
		})
	}
}

func TestHTTPToolClientMapsContextTimeout(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {Endpoint: "https://api.example.com/search"},
	})
	doer := &fakeHTTPDoer{err: context.DeadlineExceeded}
	client := NewHTTPToolClient(registry, nil, doer)

	_, err := client.Call(context.Background(), "web_search", "input")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	code, _, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeTimeout {
		t.Fatalf("expected code %q, got %q", ToolCodeTimeout, code)
	}
	if !retryable {
		t.Fatal("expected timeout to be retryable")
	}
}

func TestHTTPToolClientMapsContextCanceled(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]crds.ToolSpec{
		"web_search": {Endpoint: "https://api.example.com/search"},
	})
	doer := &fakeHTTPDoer{err: context.Canceled}
	client := NewHTTPToolClient(registry, nil, doer)

	_, err := client.Call(context.Background(), "web_search", "input")
	if err == nil {
		t.Fatal("expected canceled error")
	}
	code, _, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeCanceled {
		t.Fatalf("expected code %q, got %q", ToolCodeCanceled, code)
	}
	if retryable {
		t.Fatal("expected canceled to be non-retryable")
	}
}
