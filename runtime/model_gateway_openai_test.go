package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestOpenAIModelGatewayCompleteSuccess(t *testing.T) {
	type capturedRequest struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	var capturedAuth string
	var capturedPath string
	captured := capturedRequest{}

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			capturedAuth = req.Header.Get("Authorization")
			capturedPath = req.URL.Path
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(body, &captured); err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"hello from model"}}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultOpenAIModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.BaseURL = "https://example.invalid/v1"
	cfg.Timeout = time.Second
	cfg.HTTPClient = client

	gateway, err := NewOpenAIModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	resp, err := gateway.Complete(context.Background(), ModelRequest{
		Model:  "gpt-test",
		Prompt: "You are a planner.",
		Step:   2,
		Tools:  []string{"web_search"},
		Context: map[string]string{
			"agent": "planner",
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.Content != "hello from model" {
		t.Fatalf("unexpected model content: %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 18 {
		t.Fatalf("expected total usage tokens=18, got %d", resp.Usage.TotalTokens)
	}
	if resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 7 {
		t.Fatalf("unexpected usage split input=%d output=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	}
	if resp.Usage.Source != "provider" {
		t.Fatalf("expected usage source provider, got %q", resp.Usage.Source)
	}
	if capturedAuth != "Bearer test-key" {
		t.Fatalf("unexpected auth header: %q", capturedAuth)
	}
	if capturedPath != "/v1/chat/completions" {
		t.Fatalf("unexpected path: %s", capturedPath)
	}
	if captured.Model != "gpt-test" {
		t.Fatalf("expected model gpt-test, got %q", captured.Model)
	}
	if len(captured.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(captured.Messages))
	}
	if captured.Messages[0].Role != "system" || captured.Messages[1].Role != "user" {
		t.Fatalf("unexpected message roles: %+v", captured.Messages)
	}
	if !strings.Contains(captured.Messages[1].Content, "step=2") {
		t.Fatalf("expected step in user content, got %q", captured.Messages[1].Content)
	}
}

func TestOpenAIModelGatewayCompleteUsesDefaultModel(t *testing.T) {
	var capturedModel string
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			parsed := map[string]interface{}{}
			if err := json.Unmarshal(body, &parsed); err != nil {
				return nil, err
			}
			if model, ok := parsed["model"].(string); ok {
				capturedModel = model
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultOpenAIModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.BaseURL = "https://example.invalid/v1"
	cfg.DefaultModel = "gpt-default"
	cfg.HTTPClient = client

	gateway, err := NewOpenAIModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}
	_, err = gateway.Complete(context.Background(), ModelRequest{Prompt: "test", Step: 1})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if capturedModel != "gpt-default" {
		t.Fatalf("expected default model gpt-default, got %q", capturedModel)
	}
}

func TestOpenAIModelGatewayCompleteProviderError(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"rate limit"}}`)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultOpenAIModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.BaseURL = "https://example.invalid/v1"
	cfg.HTTPClient = client

	gateway, err := NewOpenAIModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	_, err = gateway.Complete(context.Background(), ModelRequest{Model: "gpt-test", Prompt: "test", Step: 1})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("expected rate limit in error, got %v", err)
	}
}

func TestOpenAIModelGatewayCompleteRequestFailure(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("transport unavailable")
		}),
		Timeout: time.Second,
	}

	cfg := DefaultOpenAIModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.BaseURL = "https://example.invalid/v1"
	cfg.HTTPClient = client

	gateway, err := NewOpenAIModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	_, err = gateway.Complete(context.Background(), ModelRequest{Model: "gpt-test", Prompt: "test", Step: 1})
	if err == nil {
		t.Fatal("expected transport error")
	}
	if !strings.Contains(err.Error(), "transport unavailable") {
		t.Fatalf("expected transport unavailable in error, got %v", err)
	}
}

func TestOpenAIModelGatewayCompleteToolCallResponse(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"choices":[{"message":{"content":null,"tool_calls":[{"type":"function","function":{"name":"web_search","arguments":"{\"input\":\"latest ai news\"}"}}]}}]}`,
				)),
				Header: make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultOpenAIModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.BaseURL = "https://example.invalid/v1"
	cfg.HTTPClient = client

	gateway, err := NewOpenAIModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	resp, err := gateway.Complete(context.Background(), ModelRequest{
		Model: "gpt-test",
		Step:  1,
		Tools: []string{"web_search"},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "web_search" {
		t.Fatalf("unexpected tool call name %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Input != "latest ai news" {
		t.Fatalf("unexpected tool call input %q", resp.ToolCalls[0].Input)
	}
}
