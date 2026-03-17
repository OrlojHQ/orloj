package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

func TestWebhookCallbackToolRuntimeImmediateResponse(t *testing.T) {
	contractResp := ToolExecutionResponse{
		ToolContractVersion: "v1",
		Status:              "ok",
		Output:              ToolExecutionOutput{Result: "immediate result"},
		Usage:               ToolExecutionUsage{Attempt: 1},
	}
	body, _ := json.Marshal(contractResp)
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"wh_tool": {
			Type:     "webhook-callback",
			Endpoint: "https://wh.example.com/execute",
		},
	})
	doer := &fakeHTTPDoer{statusCode: 200, body: string(body)}
	runtime := NewWebhookCallbackToolRuntime(registry, nil, doer, 100*time.Millisecond)

	out, err := runtime.Call(context.Background(), "wh_tool", `{"key":"value"}`)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if out != "immediate result" {
		t.Fatalf("expected 'immediate result', got %q", out)
	}
}

type sequencedDoer struct {
	responses []fakeHTTPDoer
	callCount int32
}

func (d *sequencedDoer) Do(req *http.Request) (*http.Response, error) {
	idx := int(atomic.AddInt32(&d.callCount, 1)) - 1
	if idx >= len(d.responses) {
		idx = len(d.responses) - 1
	}
	r := d.responses[idx]
	if r.err != nil {
		return nil, r.err
	}
	return &http.Response{
		StatusCode: r.statusCode,
		Body:       io.NopCloser(bytes.NewReader([]byte(r.body))),
		Header:     http.Header{},
	}, nil
}

func TestWebhookCallbackToolRuntimePollsForResult(t *testing.T) {
	contractResp := ToolExecutionResponse{
		ToolContractVersion: "v1",
		Status:              "ok",
		Output:              ToolExecutionOutput{Result: "polled result"},
		Usage:               ToolExecutionUsage{Attempt: 1},
	}
	respBody, _ := json.Marshal(contractResp)

	doer := &sequencedDoer{
		responses: []fakeHTTPDoer{
			{statusCode: 202, body: `{"status":"accepted"}`},
			{statusCode: 202, body: ""},
			{statusCode: 200, body: string(respBody)},
		},
	}
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"wh_tool": {
			Type:     "webhook-callback",
			Endpoint: "https://wh.example.com/execute",
		},
	})
	runtime := NewWebhookCallbackToolRuntime(registry, nil, doer, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := runtime.Call(ctx, "wh_tool", "input")
	if err != nil {
		t.Fatalf("expected success after polling, got %v", err)
	}
	if out != "polled result" {
		t.Fatalf("expected 'polled result', got %q", out)
	}
	if atomic.LoadInt32(&doer.callCount) < 2 {
		t.Fatalf("expected at least 2 HTTP calls (submit + poll), got %d", doer.callCount)
	}
}

func TestWebhookCallbackToolRuntimeCallbackDelivery(t *testing.T) {
	doer := &sequencedDoer{
		responses: []fakeHTTPDoer{
			{statusCode: 202, body: `{"status":"accepted"}`},
			{statusCode: 202, body: ""},
			{statusCode: 202, body: ""},
			{statusCode: 202, body: ""},
		},
	}
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"wh_tool": {
			Type:     "webhook-callback",
			Endpoint: "https://wh.example.com/execute",
		},
	})
	runtime := NewWebhookCallbackToolRuntime(registry, nil, doer, 100*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	var callErr error

	go func() {
		_, callErr = runtime.Call(ctx, "wh_tool", "input")
		close(done)
	}()

	// The poll keeps getting 202 and no callback is delivered,
	// so the call should eventually time out via context.
	select {
	case <-done:
		if callErr == nil {
			t.Fatal("expected timeout or error, got success without callback delivery")
		}
	case <-time.After(3 * time.Second):
		cancel()
		<-done
		if callErr == nil {
			t.Fatal("expected error after cancel")
		}
	}
}

func TestWebhookCallbackToolRuntimeTimesOut(t *testing.T) {
	doer := &sequencedDoer{
		responses: []fakeHTTPDoer{
			{statusCode: 202, body: ""},
			{statusCode: 202, body: ""},
			{statusCode: 202, body: ""},
			{statusCode: 202, body: ""},
		},
	}
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"wh_tool": {
			Type:     "webhook-callback",
			Endpoint: "https://wh.example.com/execute",
		},
	})
	runtime := NewWebhookCallbackToolRuntime(registry, nil, doer, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := runtime.Call(ctx, "wh_tool", "input")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	code, _, _, _ := ToolErrorMeta(err)
	if code != ToolCodeTimeout {
		t.Fatalf("expected code %q, got %q", ToolCodeTimeout, code)
	}
}

func TestWebhookCallbackToolRuntimeFailsOnMissingEndpoint(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"wh_tool": {Type: "webhook-callback"},
	})
	runtime := NewWebhookCallbackToolRuntime(registry, nil, nil, 0)

	_, err := runtime.Call(context.Background(), "wh_tool", "input")
	if err == nil {
		t.Fatal("expected missing endpoint error")
	}
	code, _, _, _ := ToolErrorMeta(err)
	if code != ToolCodeRuntimePolicyInvalid {
		t.Fatalf("expected code %q, got %q", ToolCodeRuntimePolicyInvalid, code)
	}
}

func TestWebhookCallbackToolRuntimeHTTPSubmitError(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"wh_tool": {
			Type:     "webhook-callback",
			Endpoint: "https://wh.example.com/execute",
		},
	})
	doer := &fakeHTTPDoer{statusCode: 500, body: "server error"}
	runtime := NewWebhookCallbackToolRuntime(registry, nil, doer, 0)

	_, err := runtime.Call(context.Background(), "wh_tool", "input")
	if err == nil {
		t.Fatal("expected error")
	}
	_, _, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if !retryable {
		t.Fatal("expected 500 to be retryable")
	}
}

func TestWebhookCallbackToolRuntimeSecretFailure(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"wh_tool": {
			Type:     "webhook-callback",
			Endpoint: "https://wh.example.com/execute",
			Auth:     resources.ToolAuth{SecretRef: "missing"},
		},
	})
	secrets := staticSecretResolver{values: map[string]string{}}
	runtime := NewWebhookCallbackToolRuntime(registry, secrets, nil, 0)

	_, err := runtime.Call(context.Background(), "wh_tool", "input")
	if err == nil {
		t.Fatal("expected secret resolution failure")
	}
	if !errors.Is(err, ErrToolSecretResolution) {
		t.Fatalf("expected ErrToolSecretResolution, got %v", err)
	}
}

func TestWebhookCallbackToolRuntimeRegisteredInDefaultRegistry(t *testing.T) {
	registry := DefaultToolIsolationBackendRegistry()
	modes := registry.Modes()
	found := false
	for _, mode := range modes {
		if mode == "webhook-callback" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'webhook-callback' in registered modes, got %v", modes)
	}
}
