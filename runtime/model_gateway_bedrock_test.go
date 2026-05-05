package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

type mockBedrockClient struct {
	converseFunc func(ctx context.Context, params *bedrockruntime.ConverseInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
}

func (m *mockBedrockClient) Converse(ctx context.Context, params *bedrockruntime.ConverseInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	return m.converseFunc(ctx, params, optFns...)
}

func TestBedrockModelGatewayCompleteBasicText(t *testing.T) {
	var capturedInput *bedrockruntime.ConverseInput

	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, params *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			capturedInput = params
			return &bedrockruntime.ConverseOutput{
				Output: &types.ConverseOutputMemberMessage{
					Value: types.Message{
						Role: types.ConversationRoleAssistant,
						Content: []types.ContentBlock{
							&types.ContentBlockMemberText{Value: "hello from bedrock"},
						},
					},
				},
				StopReason: types.StopReasonEndTurn,
				Usage: &types.TokenUsage{
					InputTokens:  aws.Int32(10),
					OutputTokens: aws.Int32(5),
					TotalTokens:  aws.Int32(15),
				},
			}, nil
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "anthropic.claude-3-5-sonnet-20241022-v2:0", 2048)

	resp, err := gateway.Complete(context.Background(), ModelRequest{
		Model:  "anthropic.claude-3-5-sonnet-20241022-v2:0",
		Prompt: "You are a planner.",
		Step:   3,
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.Content != "hello from bedrock" {
		t.Fatalf("unexpected content: %q", resp.Content)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 || resp.Usage.TotalTokens != 15 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
	if resp.Usage.Source != "provider" {
		t.Fatalf("expected usage source provider, got %q", resp.Usage.Source)
	}
	if capturedInput == nil {
		t.Fatal("expected captured input")
	}
	if aws.ToString(capturedInput.ModelId) != "anthropic.claude-3-5-sonnet-20241022-v2:0" {
		t.Fatalf("unexpected model: %q", aws.ToString(capturedInput.ModelId))
	}
	if len(capturedInput.System) != 2 {
		t.Fatalf("expected 2 system blocks (text + cache point), got %d", len(capturedInput.System))
	}
	sysBlock, ok := capturedInput.System[0].(*types.SystemContentBlockMemberText)
	if !ok || sysBlock.Value != "You are a planner." {
		t.Fatalf("unexpected system block: %+v", capturedInput.System[0])
	}
	if _, ok := capturedInput.System[1].(*types.SystemContentBlockMemberCachePoint); !ok {
		t.Fatalf("expected cache point as last system block, got %T", capturedInput.System[1])
	}
}

func TestBedrockModelGatewayCompleteUsesDefaultModel(t *testing.T) {
	var capturedModelId string

	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, params *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			capturedModelId = aws.ToString(params.ModelId)
			return &bedrockruntime.ConverseOutput{
				Output: &types.ConverseOutputMemberMessage{
					Value: types.Message{
						Role:    types.ConversationRoleAssistant,
						Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: "ok"}},
					},
				},
				StopReason: types.StopReasonEndTurn,
			}, nil
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "default-model-id", 1024)
	_, err := gateway.Complete(context.Background(), ModelRequest{Prompt: "test", Step: 1})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if capturedModelId != "default-model-id" {
		t.Fatalf("expected default model, got %q", capturedModelId)
	}
}

func TestBedrockModelGatewayCompleteMultiTurn(t *testing.T) {
	var capturedInput *bedrockruntime.ConverseInput

	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, params *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			capturedInput = params
			return &bedrockruntime.ConverseOutput{
				Output: &types.ConverseOutputMemberMessage{
					Value: types.Message{
						Role:    types.ConversationRoleAssistant,
						Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: "turn 2 reply"}},
					},
				},
				StopReason: types.StopReasonEndTurn,
			}, nil
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "test-model", 1024)
	_, err := gateway.Complete(context.Background(), ModelRequest{
		Model: "test-model",
		Step:  2,
		Messages: []ChatMessage{
			{Role: "system", Content: "be helpful"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
			{Role: "user", Content: "second question"},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(capturedInput.System) != 2 {
		t.Fatalf("expected 2 system blocks (text + cache point), got %d", len(capturedInput.System))
	}
	if len(capturedInput.Messages) != 3 {
		t.Fatalf("expected 3 messages (user, assistant, user), got %d", len(capturedInput.Messages))
	}
	if capturedInput.Messages[0].Role != types.ConversationRoleUser {
		t.Fatalf("expected first message role=user, got %q", capturedInput.Messages[0].Role)
	}
	if capturedInput.Messages[1].Role != types.ConversationRoleAssistant {
		t.Fatalf("expected second message role=assistant, got %q", capturedInput.Messages[1].Role)
	}
}

func TestBedrockModelGatewayCompleteSystemPromptExtraction(t *testing.T) {
	var capturedInput *bedrockruntime.ConverseInput

	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, params *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			capturedInput = params
			return &bedrockruntime.ConverseOutput{
				Output: &types.ConverseOutputMemberMessage{
					Value: types.Message{
						Role:    types.ConversationRoleAssistant,
						Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: "ok"}},
					},
				},
				StopReason: types.StopReasonEndTurn,
			}, nil
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "test-model", 1024)
	_, err := gateway.Complete(context.Background(), ModelRequest{
		Model: "test-model",
		Step:  1,
		Messages: []ChatMessage{
			{Role: "system", Content: "first system"},
			{Role: "system", Content: "second system"},
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(capturedInput.System) != 3 {
		t.Fatalf("expected 3 system blocks (2 text + cache point), got %d", len(capturedInput.System))
	}
	sys0, ok := capturedInput.System[0].(*types.SystemContentBlockMemberText)
	if !ok || sys0.Value != "first system" {
		t.Fatalf("unexpected first system block")
	}
	sys1, ok := capturedInput.System[1].(*types.SystemContentBlockMemberText)
	if !ok || sys1.Value != "second system" {
		t.Fatalf("unexpected second system block")
	}
	if _, ok := capturedInput.System[2].(*types.SystemContentBlockMemberCachePoint); !ok {
		t.Fatalf("expected cache point as last system block, got %T", capturedInput.System[2])
	}
	if len(capturedInput.Messages) != 1 {
		t.Fatalf("expected 1 message (system stripped), got %d", len(capturedInput.Messages))
	}
}

func TestBedrockModelGatewayCompleteToolUseResponse(t *testing.T) {
	var capturedInput *bedrockruntime.ConverseInput

	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, params *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			capturedInput = params
			return &bedrockruntime.ConverseOutput{
				Output: &types.ConverseOutputMemberMessage{
					Value: types.Message{
						Role: types.ConversationRoleAssistant,
						Content: []types.ContentBlock{
							&types.ContentBlockMemberToolUse{
								Value: types.ToolUseBlock{
									ToolUseId: aws.String("tool_abc123"),
									Name:      aws.String("web_search"),
									Input:     document.NewLazyDocument(map[string]interface{}{"input": "latest ai"}),
								},
							},
							&types.ContentBlockMemberText{Value: "calling tool"},
						},
					},
				},
				StopReason: types.StopReasonToolUse,
			}, nil
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "test-model", 1024)
	resp, err := gateway.Complete(context.Background(), ModelRequest{
		Model: "test-model",
		Step:  1,
		Tools: []string{"web_search"},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if capturedInput.ToolConfig == nil {
		t.Fatal("expected tool config in request")
	}
	if len(capturedInput.ToolConfig.Tools) != 2 {
		t.Fatalf("expected 2 tools (spec + cache point), got %d", len(capturedInput.ToolConfig.Tools))
	}
	if _, ok := capturedInput.ToolConfig.Tools[1].(*types.ToolMemberCachePoint); !ok {
		t.Fatalf("expected cache point as last tool, got %T", capturedInput.ToolConfig.Tools[1])
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "web_search" {
		t.Fatalf("unexpected tool call name %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].ID != "tool_abc123" {
		t.Fatalf("unexpected tool call ID %q", resp.ToolCalls[0].ID)
	}
	if resp.ToolCalls[0].Input != "latest ai" {
		t.Fatalf("unexpected tool call input %q", resp.ToolCalls[0].Input)
	}
	if resp.Content != "calling tool" {
		t.Fatalf("unexpected content %q", resp.Content)
	}
}

func TestBedrockModelGatewayCompleteMapsToolAliases(t *testing.T) {
	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, params *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			// Verify the tool was sent with the sanitized name (first entry before cache point)
			if params.ToolConfig == nil || len(params.ToolConfig.Tools) < 1 {
				return nil, fmt.Errorf("expected at least 1 tool")
			}
			spec, ok := params.ToolConfig.Tools[0].(*types.ToolMemberToolSpec)
			if !ok {
				return nil, fmt.Errorf("expected ToolMemberToolSpec")
			}
			if aws.ToString(spec.Value.Name) != "memory_write" {
				return nil, fmt.Errorf("expected sanitized name memory_write, got %q", aws.ToString(spec.Value.Name))
			}
			return &bedrockruntime.ConverseOutput{
				Output: &types.ConverseOutputMemberMessage{
					Value: types.Message{
						Role: types.ConversationRoleAssistant,
						Content: []types.ContentBlock{
							&types.ContentBlockMemberToolUse{
								Value: types.ToolUseBlock{
									ToolUseId: aws.String("t1"),
									Name:      aws.String("memory_write"),
									Input:     document.NewLazyDocument(map[string]interface{}{"input": `{"key":"x","value":"y"}`}),
								},
							},
						},
					},
				},
				StopReason: types.StopReasonToolUse,
			}, nil
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "test-model", 1024)
	resp, err := gateway.Complete(context.Background(), ModelRequest{
		Model: "test-model",
		Step:  1,
		Tools: []string{"memory.write"},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "memory.write" {
		t.Fatalf("expected runtime name memory.write, got %q", resp.ToolCalls[0].Name)
	}
}

func TestBedrockModelGatewayCompleteToolResultInHistory(t *testing.T) {
	var capturedInput *bedrockruntime.ConverseInput

	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, params *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			capturedInput = params
			return &bedrockruntime.ConverseOutput{
				Output: &types.ConverseOutputMemberMessage{
					Value: types.Message{
						Role:    types.ConversationRoleAssistant,
						Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: "thanks for the result"}},
					},
				},
				StopReason: types.StopReasonEndTurn,
			}, nil
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "test-model", 1024)
	_, err := gateway.Complete(context.Background(), ModelRequest{
		Model: "test-model",
		Step:  2,
		Messages: []ChatMessage{
			{Role: "user", Content: "step=1"},
			{Role: "assistant", Content: "thinking", ToolCalls: []ChatToolCall{
				{ID: "toolu_01A", Name: "search", Input: `{"q":"test"}`},
			}},
			{Role: "tool", Content: "result data", ToolCallID: "toolu_01A"},
			{Role: "user", Content: "step=2"},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(capturedInput.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(capturedInput.Messages))
	}
	// Assistant message with tool use
	assistantMsg := capturedInput.Messages[1]
	if assistantMsg.Role != types.ConversationRoleAssistant {
		t.Fatalf("expected assistant role, got %q", assistantMsg.Role)
	}
	if len(assistantMsg.Content) < 2 {
		t.Fatalf("expected at least 2 content blocks in assistant message, got %d", len(assistantMsg.Content))
	}
	// Tool result message
	toolResultMsg := capturedInput.Messages[2]
	if toolResultMsg.Role != types.ConversationRoleUser {
		t.Fatalf("expected user role for tool result, got %q", toolResultMsg.Role)
	}
	toolResultBlock, ok := toolResultMsg.Content[0].(*types.ContentBlockMemberToolResult)
	if !ok {
		t.Fatalf("expected tool result content block, got %T", toolResultMsg.Content[0])
	}
	if aws.ToString(toolResultBlock.Value.ToolUseId) != "toolu_01A" {
		t.Fatalf("expected tool_use_id=toolu_01A, got %q", aws.ToString(toolResultBlock.Value.ToolUseId))
	}
}

func TestBedrockModelGatewayCompleteErrorThrottling(t *testing.T) {
	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, _ *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			return nil, &types.ThrottlingException{Message: aws.String("rate limit exceeded")}
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "test-model", 1024)
	_, err := gateway.Complete(context.Background(), ModelRequest{Model: "test-model", Prompt: "test", Step: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	mge, retryable := IsModelGatewayError(err)
	if mge == nil {
		t.Fatalf("expected ModelGatewayError, got %T: %v", err, err)
	}
	if mge.StatusCode != 429 {
		t.Fatalf("expected status 429, got %d", mge.StatusCode)
	}
	if !retryable {
		t.Fatal("expected retryable error for throttling")
	}
	if mge.Provider != "bedrock" {
		t.Fatalf("expected provider bedrock, got %q", mge.Provider)
	}
}

func TestBedrockModelGatewayCompleteErrorValidation(t *testing.T) {
	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, _ *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			return nil, &types.ValidationException{Message: aws.String("invalid param")}
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "test-model", 1024)
	_, err := gateway.Complete(context.Background(), ModelRequest{Model: "test-model", Prompt: "test", Step: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	mge, retryable := IsModelGatewayError(err)
	if mge == nil {
		t.Fatalf("expected ModelGatewayError, got %T", err)
	}
	if mge.StatusCode != 400 {
		t.Fatalf("expected status 400, got %d", mge.StatusCode)
	}
	if retryable {
		t.Fatal("expected non-retryable error for validation")
	}
}

func TestBedrockModelGatewayCompleteErrorAccessDenied(t *testing.T) {
	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, _ *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			return nil, &types.AccessDeniedException{Message: aws.String("not authorized")}
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "test-model", 1024)
	_, err := gateway.Complete(context.Background(), ModelRequest{Model: "test-model", Prompt: "test", Step: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	mge, _ := IsModelGatewayError(err)
	if mge == nil || mge.StatusCode != 403 {
		t.Fatalf("expected status 403, got %v", mge)
	}
}

func TestBedrockModelGatewayCompleteErrorResourceNotFound(t *testing.T) {
	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, _ *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			return nil, &types.ResourceNotFoundException{Message: aws.String("model not enabled")}
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "test-model", 1024)
	_, err := gateway.Complete(context.Background(), ModelRequest{Model: "test-model", Prompt: "test", Step: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	mge, _ := IsModelGatewayError(err)
	if mge == nil || mge.StatusCode != 404 {
		t.Fatalf("expected status 404, got %v", mge)
	}
}

func TestBedrockModelGatewayCompleteErrorTimeout(t *testing.T) {
	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, _ *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			return nil, &types.ModelTimeoutException{Message: aws.String("model timed out")}
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "test-model", 1024)
	_, err := gateway.Complete(context.Background(), ModelRequest{Model: "test-model", Prompt: "test", Step: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	mge, retryable := IsModelGatewayError(err)
	if mge == nil || mge.StatusCode != 504 {
		t.Fatalf("expected status 504, got %v", mge)
	}
	if !retryable {
		t.Fatal("expected retryable error for timeout")
	}
}

func TestBedrockModelGatewayRequiresModel(t *testing.T) {
	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, _ *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			return nil, fmt.Errorf("should not be called")
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "", 1024)
	_, err := gateway.Complete(context.Background(), ModelRequest{Prompt: "test", Step: 1})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("expected model is required error, got %v", err)
	}
}

func TestBedrockModelGatewayNilGateway(t *testing.T) {
	var gateway *BedrockModelGateway
	_, err := gateway.Complete(context.Background(), ModelRequest{Model: "test", Step: 1})
	if err == nil {
		t.Fatal("expected error for nil gateway")
	}
}

func TestBedrockModelGatewayPluginRegistration(t *testing.T) {
	registry := DefaultModelProviderRegistry()
	for _, name := range []string{"bedrock", "aws-bedrock", "aws_bedrock"} {
		plugin, ok := registry.Lookup(name)
		if !ok {
			t.Fatalf("expected provider %q to be registered", name)
		}
		if plugin.RequiresAPIKey() {
			t.Fatalf("bedrock provider should not require API key")
		}
	}
}

func TestBedrockModelGatewayPluginBuildGatewayWithRegion(t *testing.T) {
	plugin := &bedrockModelProviderPlugin{}
	_, err := plugin.BuildGateway(ModelGatewayConfig{
		DefaultModel: "anthropic.claude-3-5-sonnet-20241022-v2:0",
		Options:      map[string]string{"region": "us-east-1"},
	})
	if err != nil {
		t.Fatalf("build gateway failed: %v", err)
	}
}

func TestBedrockModelGatewayPluginBuildGatewayMissingRegion(t *testing.T) {
	plugin := &bedrockModelProviderPlugin{}
	_, err := plugin.BuildGateway(ModelGatewayConfig{
		DefaultModel: "some-model",
		Options:      map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing region")
	}
	if !strings.Contains(err.Error(), "region") {
		t.Fatalf("expected region error, got %v", err)
	}
}

func TestBedrockModelGatewayPluginBuildGatewayWithCredentials(t *testing.T) {
	plugin := &bedrockModelProviderPlugin{}
	creds := `{"access_key_id":"AKIATEST","secret_access_key":"secrettest","session_token":"tokentest"}`
	_, err := plugin.BuildGateway(ModelGatewayConfig{
		DefaultModel: "anthropic.claude-3-5-sonnet-20241022-v2:0",
		APIKey:       creds,
		Options:      map[string]string{"region": "us-west-2"},
	})
	if err != nil {
		t.Fatalf("build gateway with credentials failed: %v", err)
	}
}

func TestBedrockModelGatewayPluginBuildGatewayInvalidMaxTokens(t *testing.T) {
	plugin := &bedrockModelProviderPlugin{}
	_, err := plugin.BuildGateway(ModelGatewayConfig{
		DefaultModel: "some-model",
		Options:      map[string]string{"region": "us-east-1", "max_tokens": "not-a-number"},
	})
	if err == nil {
		t.Fatal("expected error for invalid max_tokens")
	}
	if !strings.Contains(err.Error(), "max_tokens") {
		t.Fatalf("expected max_tokens error, got %v", err)
	}
}

func TestBedrockModelGatewaySendsOutputConfig(t *testing.T) {
	var capturedInput *bedrockruntime.ConverseInput

	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, params *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			capturedInput = params
			return &bedrockruntime.ConverseOutput{
				Output: &types.ConverseOutputMemberMessage{
					Value: types.Message{
						Role:    types.ConversationRoleAssistant,
						Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: `{"route":"research"}`}},
					},
				},
				StopReason: types.StopReasonEndTurn,
				Usage:      &types.TokenUsage{InputTokens: aws.Int32(5), OutputTokens: aws.Int32(3), TotalTokens: aws.Int32(8)},
			}, nil
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "test-model", 1024)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"route": map[string]any{"type": "string"},
		},
		"required":             []string{"route"},
		"additionalProperties": false,
	}

	_, err := gateway.Complete(context.Background(), ModelRequest{
		Model:        "test-model",
		Prompt:       "Classify input",
		Step:         1,
		OutputSchema: schema,
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if capturedInput.OutputConfig == nil {
		t.Fatal("expected output config in request")
	}
	if capturedInput.OutputConfig.TextFormat == nil {
		t.Fatal("expected text format in output config")
	}
	if capturedInput.OutputConfig.TextFormat.Type != types.OutputFormatTypeJsonSchema {
		t.Fatalf("expected json_schema format type, got %q", capturedInput.OutputConfig.TextFormat.Type)
	}
}

func TestBedrockModelGatewayOmitsOutputConfigWhenNoSchema(t *testing.T) {
	var capturedInput *bedrockruntime.ConverseInput

	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, params *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			capturedInput = params
			return &bedrockruntime.ConverseOutput{
				Output: &types.ConverseOutputMemberMessage{
					Value: types.Message{
						Role:    types.ConversationRoleAssistant,
						Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: "hello"}},
					},
				},
				StopReason: types.StopReasonEndTurn,
			}, nil
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "test-model", 1024)
	_, err := gateway.Complete(context.Background(), ModelRequest{
		Model:  "test-model",
		Prompt: "Hello",
		Step:   1,
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if capturedInput.OutputConfig != nil {
		t.Fatal("output config should be nil when no schema is set")
	}
}

func TestBedrockModelGatewayCompleteTokenUsage(t *testing.T) {
	mock := &mockBedrockClient{
		converseFunc: func(_ context.Context, _ *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			return &bedrockruntime.ConverseOutput{
				Output: &types.ConverseOutputMemberMessage{
					Value: types.Message{
						Role:    types.ConversationRoleAssistant,
						Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: "reply"}},
					},
				},
				StopReason: types.StopReasonEndTurn,
				Usage: &types.TokenUsage{
					InputTokens:  aws.Int32(42),
					OutputTokens: aws.Int32(15),
					TotalTokens:  aws.Int32(57),
				},
			}, nil
		},
	}

	gateway := newBedrockModelGatewayWithClient(mock, "test-model", 1024)
	resp, err := gateway.Complete(context.Background(), ModelRequest{Model: "test-model", Prompt: "test", Step: 1})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.Usage.InputTokens != 42 {
		t.Fatalf("expected input tokens 42, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 15 {
		t.Fatalf("expected output tokens 15, got %d", resp.Usage.OutputTokens)
	}
	if resp.Usage.TotalTokens != 57 {
		t.Fatalf("expected total tokens 57, got %d", resp.Usage.TotalTokens)
	}
}
