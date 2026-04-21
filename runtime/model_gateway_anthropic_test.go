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

func TestAnthropicModelGatewayCompleteSuccess(t *testing.T) {
	type capturedRequest struct {
		Model     string `json:"model"`
		System    string `json:"system"`
		MaxTokens int    `json:"max_tokens"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	var capturedAPIKey string
	var capturedVersion string
	var capturedPath string
	captured := capturedRequest{}

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			capturedAPIKey = req.Header.Get("x-api-key")
			capturedVersion = req.Header.Get("anthropic-version")
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
				Body:       io.NopCloser(strings.NewReader(`{"content":[{"type":"text","text":"hello from claude"}],"usage":{"input_tokens":9,"output_tokens":4}}`)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultAnthropicModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.BaseURL = "https://example.invalid/v1"
	cfg.AnthropicVersion = "2023-06-01"
	cfg.MaxTokens = 2048
	cfg.HTTPClient = client

	gateway, err := NewAnthropicModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	resp, err := gateway.Complete(context.Background(), ModelRequest{
		Model:  "claude-test",
		Prompt: "You are a planner.",
		Step:   3,
		Tools:  []string{"web_search"},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.Content != "hello from claude" {
		t.Fatalf("unexpected model content: %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 13 {
		t.Fatalf("expected total usage tokens=13, got %d", resp.Usage.TotalTokens)
	}
	if resp.Usage.InputTokens != 9 || resp.Usage.OutputTokens != 4 {
		t.Fatalf("unexpected usage split input=%d output=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	}
	if resp.Usage.Source != "provider" {
		t.Fatalf("expected usage source provider, got %q", resp.Usage.Source)
	}
	if capturedAPIKey != "test-key" {
		t.Fatalf("unexpected x-api-key header: %q", capturedAPIKey)
	}
	if capturedVersion != "2023-06-01" {
		t.Fatalf("unexpected anthropic-version header: %q", capturedVersion)
	}
	if capturedPath != "/v1/messages" {
		t.Fatalf("unexpected path: %s", capturedPath)
	}
	if captured.Model != "claude-test" {
		t.Fatalf("expected model claude-test, got %q", captured.Model)
	}
	if captured.System != "You are a planner." {
		t.Fatalf("expected system prompt, got %q", captured.System)
	}
	if captured.MaxTokens != 2048 {
		t.Fatalf("expected max_tokens 2048, got %d", captured.MaxTokens)
	}
	if len(captured.Messages) != 1 || captured.Messages[0].Role != "user" {
		t.Fatalf("unexpected messages payload: %+v", captured.Messages)
	}
	if !strings.Contains(captured.Messages[0].Content, "step=3") {
		t.Fatalf("expected step in user content, got %q", captured.Messages[0].Content)
	}
}

func TestAnthropicModelGatewayCompleteUsesDefaultModel(t *testing.T) {
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
				Body:       io.NopCloser(strings.NewReader(`{"content":[{"type":"text","text":"ok"}]}`)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultAnthropicModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.BaseURL = "https://example.invalid/v1"
	cfg.DefaultModel = "claude-default"
	cfg.HTTPClient = client

	gateway, err := NewAnthropicModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}
	_, err = gateway.Complete(context.Background(), ModelRequest{Prompt: "test", Step: 1})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if capturedModel != "claude-default" {
		t.Fatalf("expected default model claude-default, got %q", capturedModel)
	}
}

func TestAnthropicModelGatewayCompleteProviderError(t *testing.T) {
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

	cfg := DefaultAnthropicModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.BaseURL = "https://example.invalid/v1"
	cfg.HTTPClient = client

	gateway, err := NewAnthropicModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	_, err = gateway.Complete(context.Background(), ModelRequest{Model: "claude-test", Prompt: "test", Step: 1})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("expected rate limit in error, got %v", err)
	}
}

func TestAnthropicModelGatewayCompleteRequestFailure(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("transport unavailable")
		}),
		Timeout: time.Second,
	}

	cfg := DefaultAnthropicModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.BaseURL = "https://example.invalid/v1"
	cfg.HTTPClient = client

	gateway, err := NewAnthropicModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	_, err = gateway.Complete(context.Background(), ModelRequest{Model: "claude-test", Prompt: "test", Step: 1})
	if err == nil {
		t.Fatal("expected transport error")
	}
	if !strings.Contains(err.Error(), "transport unavailable") {
		t.Fatalf("expected transport unavailable in error, got %v", err)
	}
}

func TestAnthropicModelGatewayCompleteToolCallResponse(t *testing.T) {
	type capturedRequest struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}

	captured := capturedRequest{}
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(body, &captured); err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"content":[{"type":"tool_use","name":"web_search","input":{"input":"latest ai"}},{"type":"text","text":"calling tool"}]}`,
				)),
				Header: make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultAnthropicModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.BaseURL = "https://example.invalid/v1"
	cfg.HTTPClient = client

	gateway, err := NewAnthropicModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	resp, err := gateway.Complete(context.Background(), ModelRequest{
		Model: "claude-test",
		Step:  1,
		Tools: []string{"web_search"},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(captured.Tools) != 1 || captured.Tools[0].Name != "web_search" {
		t.Fatalf("expected request tools payload, got %+v", captured.Tools)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "web_search" {
		t.Fatalf("unexpected tool call name %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Input != "latest ai" {
		t.Fatalf("unexpected tool call input %q", resp.ToolCalls[0].Input)
	}
}

func TestAnthropicModelGatewayCompleteMapsToolAliasesBackToRuntimeNames(t *testing.T) {
	type capturedRequest struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}

	captured := capturedRequest{}
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(body, &captured); err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"content":[{"type":"tool_use","name":"memory_write","input":{"input":"{\"key\":\"x\",\"value\":\"y\"}"}}]}`,
				)),
				Header: make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultAnthropicModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.BaseURL = "https://example.invalid/v1"
	cfg.HTTPClient = client

	gateway, err := NewAnthropicModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	resp, err := gateway.Complete(context.Background(), ModelRequest{
		Model: "claude-test",
		Step:  1,
		Tools: []string{"memory.write"},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(captured.Tools) != 1 || captured.Tools[0].Name != "memory_write" {
		t.Fatalf("expected sanitized provider tool name, got %+v", captured.Tools)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "memory.write" {
		t.Fatalf("expected runtime tool name memory.write, got %q", resp.ToolCalls[0].Name)
	}
}

func TestChatMessagesToAnthropicStructuredToolMessages(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "system", Content: "be helpful"},
		{Role: "user", Content: "step=1"},
		{Role: "assistant", Content: "thinking", ToolCalls: []ChatToolCall{
			{ID: "toolu_01A", Name: "search", Input: `{"q":"test"}`},
		}},
		{Role: "tool", Content: "result data", ToolCallID: "toolu_01A"},
		{Role: "user", Content: "step=2"},
	}

	system, anthropicMsgs := chatMessagesToAnthropic(msgs)
	if system != "be helpful" {
		t.Fatalf("expected system=be helpful, got %q", system)
	}
	if len(anthropicMsgs) != 4 {
		t.Fatalf("expected 4 messages (user, assistant, user-tool-result, user), got %d", len(anthropicMsgs))
	}

	assistantMsg := anthropicMsgs[1]
	if assistantMsg.Role != "assistant" {
		t.Fatalf("expected assistant role, got %q", assistantMsg.Role)
	}
	blocks, ok := assistantMsg.Content.([]map[string]interface{})
	if !ok {
		t.Fatalf("expected structured content blocks, got %T", assistantMsg.Content)
	}
	var foundToolUse bool
	for _, block := range blocks {
		if block["type"] == "tool_use" {
			if block["id"] != "toolu_01A" {
				t.Fatalf("expected tool_use id=toolu_01A, got %v", block["id"])
			}
			foundToolUse = true
		}
	}
	if !foundToolUse {
		t.Fatal("expected tool_use content block in assistant message")
	}

	toolResultMsg := anthropicMsgs[2]
	if toolResultMsg.Role != "user" {
		t.Fatalf("expected user role for tool_result, got %q", toolResultMsg.Role)
	}
	resultBlocks, ok := toolResultMsg.Content.([]map[string]interface{})
	if !ok {
		t.Fatalf("expected structured content blocks for tool result, got %T", toolResultMsg.Content)
	}
	if len(resultBlocks) != 1 {
		t.Fatalf("expected 1 tool_result block, got %d", len(resultBlocks))
	}
	if resultBlocks[0]["type"] != "tool_result" {
		t.Fatalf("expected type=tool_result, got %v", resultBlocks[0]["type"])
	}
	if resultBlocks[0]["tool_use_id"] != "toolu_01A" {
		t.Fatalf("expected tool_use_id=toolu_01A, got %v", resultBlocks[0]["tool_use_id"])
	}
}

func TestChatMessagesToAnthropicErrorToolResult(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "user", Content: "step=1"},
		{Role: "assistant", Content: "calling tools", ToolCalls: []ChatToolCall{
			{ID: "toolu_01A", Name: "kubectl-get", Input: `{"resource":"pods"}`},
			{ID: "toolu_01B", Name: "prometheus-query", Input: `{"query":"up"}`},
		}},
		{Role: "tool", Content: "<tool_result>\npod list\n</tool_result>", ToolCallID: "toolu_01A"},
		{Role: "tool", Content: "<tool_error>\nmcp tool prometheus-query returned error: connection refused\n</tool_error>", ToolCallID: "toolu_01B", IsError: true},
		{Role: "user", Content: "step=2"},
	}

	system, anthropicMsgs := chatMessagesToAnthropic(msgs)
	if system != "" {
		t.Fatalf("expected empty system, got %q", system)
	}
	if len(anthropicMsgs) != 5 {
		t.Fatalf("expected 5 messages (user, assistant, tool-result, tool-error-result, user), got %d", len(anthropicMsgs))
	}

	successResult := anthropicMsgs[2]
	successBlocks, ok := successResult.Content.([]map[string]interface{})
	if !ok || len(successBlocks) != 1 {
		t.Fatalf("expected 1 tool_result block for success, got %v", successResult.Content)
	}
	if successBlocks[0]["tool_use_id"] != "toolu_01A" {
		t.Fatalf("expected tool_use_id=toolu_01A, got %v", successBlocks[0]["tool_use_id"])
	}
	if _, hasIsError := successBlocks[0]["is_error"]; hasIsError {
		t.Fatal("successful tool_result should not have is_error field")
	}

	errorResult := anthropicMsgs[3]
	if errorResult.Role != "user" {
		t.Fatalf("expected user role for error tool_result, got %q", errorResult.Role)
	}
	errorBlocks, ok := errorResult.Content.([]map[string]interface{})
	if !ok || len(errorBlocks) != 1 {
		t.Fatalf("expected 1 tool_result block for error, got %v", errorResult.Content)
	}
	if errorBlocks[0]["type"] != "tool_result" {
		t.Fatalf("expected type=tool_result, got %v", errorBlocks[0]["type"])
	}
	if errorBlocks[0]["tool_use_id"] != "toolu_01B" {
		t.Fatalf("expected tool_use_id=toolu_01B, got %v", errorBlocks[0]["tool_use_id"])
	}
	if errorBlocks[0]["is_error"] != true {
		t.Fatalf("expected is_error=true on error tool_result, got %v", errorBlocks[0]["is_error"])
	}
}

func TestChatMessagesToAnthropicMixedToolResultsAllHaveMatchingIDs(t *testing.T) {
	toolUseIDs := []string{"toolu_01A", "toolu_01B", "toolu_01C", "toolu_01D"}
	msgs := []ChatMessage{
		{Role: "user", Content: "step=1"},
		{Role: "assistant", Content: "running 4 tools", ToolCalls: []ChatToolCall{
			{ID: toolUseIDs[0], Name: "kubectl-get", Input: `{"resource":"pods"}`},
			{ID: toolUseIDs[1], Name: "kubectl-get", Input: `{"resource":"nodes"}`},
			{ID: toolUseIDs[2], Name: "prometheus-query", Input: `{"query":"up"}`},
			{ID: toolUseIDs[3], Name: "prometheus-query", Input: `{"query":"down"}`},
		}},
		{Role: "tool", Content: "<tool_result>\npod list\n</tool_result>", ToolCallID: toolUseIDs[0]},
		{Role: "tool", Content: "<tool_result>\nnode list\n</tool_result>", ToolCallID: toolUseIDs[1]},
		{Role: "tool", Content: "<tool_error>\nconnection refused\n</tool_error>", ToolCallID: toolUseIDs[2], IsError: true},
		{Role: "tool", Content: "<tool_error>\nconnection refused\n</tool_error>", ToolCallID: toolUseIDs[3], IsError: true},
	}

	_, anthropicMsgs := chatMessagesToAnthropic(msgs)

	// Collect all tool_use IDs from assistant message
	assistantMsg := anthropicMsgs[1]
	blocks, _ := assistantMsg.Content.([]map[string]interface{})
	emittedToolUseIDs := map[string]bool{}
	for _, block := range blocks {
		if block["type"] == "tool_use" {
			emittedToolUseIDs[block["id"].(string)] = true
		}
	}

	// Collect all tool_result IDs from subsequent messages
	emittedToolResultIDs := map[string]bool{}
	for _, msg := range anthropicMsgs[2:] {
		resultBlocks, ok := msg.Content.([]map[string]interface{})
		if !ok {
			continue
		}
		for _, block := range resultBlocks {
			if block["type"] == "tool_result" {
				emittedToolResultIDs[block["tool_use_id"].(string)] = true
			}
		}
	}

	for _, id := range toolUseIDs {
		if !emittedToolUseIDs[id] {
			t.Errorf("tool_use ID %s missing from assistant message", id)
		}
		if !emittedToolResultIDs[id] {
			t.Errorf("tool_result for tool_use ID %s missing — would cause orphaned tool_use error", id)
		}
	}
}

func TestAnthropicModelGatewaySendsOutputConfig(t *testing.T) {
	var capturedBody map[string]any

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			json.Unmarshal(body, &capturedBody)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"content":[{"type":"text","text":"{\"route\":\"research\"}"}],"usage":{"input_tokens":5,"output_tokens":3}}`)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultAnthropicModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.HTTPClient = client

	gateway, err := NewAnthropicModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"route": map[string]any{"type": "string"},
		},
		"required":             []string{"route"},
		"additionalProperties": false,
	}

	_, err = gateway.Complete(context.Background(), ModelRequest{
		Model:        "claude-test",
		Prompt:       "Classify input",
		Step:         1,
		OutputSchema: schema,
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}

	oc, ok := capturedBody["output_config"]
	if !ok {
		t.Fatal("expected output_config in request body")
	}
	ocMap, ok := oc.(map[string]any)
	if !ok {
		t.Fatalf("output_config is not a map: %T", oc)
	}
	fmtMap, ok := ocMap["format"].(map[string]any)
	if !ok {
		t.Fatal("expected output_config.format to be a map")
	}
	if fmtMap["type"] != "json_schema" {
		t.Fatalf("expected format.type=json_schema, got %v", fmtMap["type"])
	}
	schemaMap, ok := fmtMap["schema"].(map[string]any)
	if !ok {
		t.Fatal("expected format.schema to be a map")
	}
	if schemaMap["type"] != "object" {
		t.Fatalf("expected schema.type=object, got %v", schemaMap["type"])
	}
}

func TestAnthropicModelGatewayOmitsOutputConfigWhenNoSchema(t *testing.T) {
	var capturedBody map[string]any

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			json.Unmarshal(body, &capturedBody)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":5,"output_tokens":3}}`)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultAnthropicModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.HTTPClient = client

	gateway, err := NewAnthropicModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	_, err = gateway.Complete(context.Background(), ModelRequest{
		Model:  "claude-test",
		Prompt: "Hello",
		Step:   1,
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}

	if _, ok := capturedBody["output_config"]; ok {
		t.Fatal("output_config should be omitted when no output schema is set")
	}
}
