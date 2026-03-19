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

func TestChatMessagesToOpenAIStructuredToolMessages(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "system", Content: "be helpful"},
		{Role: "user", Content: "step=1"},
		{Role: "assistant", Content: "calling tool", ToolCalls: []ChatToolCall{
			{ID: "call_abc", Name: "search", Input: `{"q":"test"}`},
		}},
		{Role: "tool", Content: "result data", ToolCallID: "call_abc"},
		{Role: "user", Content: "step=2"},
	}

	openaiMsgs := chatMessagesToOpenAI(msgs)
	if len(openaiMsgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(openaiMsgs))
	}

	assistantMsg := openaiMsgs[2]
	if assistantMsg.Role != "assistant" {
		t.Fatalf("expected assistant role, got %q", assistantMsg.Role)
	}
	if len(assistantMsg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call in assistant msg, got %d", len(assistantMsg.ToolCalls))
	}
	if assistantMsg.ToolCalls[0].ID != "call_abc" {
		t.Fatalf("expected tool call ID=call_abc, got %q", assistantMsg.ToolCalls[0].ID)
	}
	if assistantMsg.ToolCalls[0].Function.Name != "search" {
		t.Fatalf("expected tool name=search, got %q", assistantMsg.ToolCalls[0].Function.Name)
	}

	toolMsg := openaiMsgs[3]
	if toolMsg.Role != "tool" {
		t.Fatalf("expected tool role, got %q", toolMsg.Role)
	}
	if toolMsg.ToolCallID != "call_abc" {
		t.Fatalf("expected tool_call_id=call_abc, got %q", toolMsg.ToolCallID)
	}
	contentStr, ok := toolMsg.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", toolMsg.Content)
	}
	if contentStr != "result data" {
		t.Fatalf("expected content=result data, got %q", contentStr)
	}

	raw, err := json.Marshal(openaiMsgs)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	jsonStr := string(raw)
	if !strings.Contains(jsonStr, `"tool_call_id":"call_abc"`) {
		t.Fatalf("expected tool_call_id in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"tool_calls"`) {
		t.Fatalf("expected tool_calls in JSON, got %s", jsonStr)
	}
}

func TestParseOpenAIModelToolCallsPreservesID(t *testing.T) {
	raw := []openAIChatToolCall{
		{
			ID:   "call_xyz",
			Type: "function",
			Function: openAIChatToolFunctionCall{
				Name:      "get_weather",
				Arguments: `{"location":"Paris"}`,
			},
		},
	}

	calls := parseOpenAIModelToolCalls(raw)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].ID != "call_xyz" {
		t.Fatalf("expected ID=call_xyz, got %q", calls[0].ID)
	}
	if calls[0].Name != "get_weather" {
		t.Fatalf("expected Name=get_weather, got %q", calls[0].Name)
	}
}
