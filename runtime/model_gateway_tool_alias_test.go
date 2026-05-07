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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

func TestHTTPModelGatewaysUseToolAliasesForToolsHistoryAndResponses(t *testing.T) {
	type testCase struct {
		name       string
		response   string
		newGateway func(*http.Client) (ModelGateway, error)
		wantPath   string
	}

	cases := []testCase{
		{
			name:     "openai-compatible",
			response: `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call_resp","type":"function","function":{"name":"memory_write","arguments":"{\"input\":\"store this\"}"}}]}}]}`,
			wantPath: "/v1/chat/completions",
			newGateway: func(client *http.Client) (ModelGateway, error) {
				cfg := DefaultOpenAIModelGatewayConfig()
				cfg.RequireAPIKey = false
				cfg.BaseURL = "https://example.invalid/v1"
				cfg.HTTPClient = client
				return NewOpenAIModelGateway(cfg)
			},
		},
		{
			name:     "azure-openai",
			response: `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call_resp","type":"function","function":{"name":"memory_write","arguments":"{\"input\":\"store this\"}"}}]}}]}`,
			wantPath: "/openai/deployments/test-model/chat/completions",
			newGateway: func(client *http.Client) (ModelGateway, error) {
				cfg := DefaultAzureOpenAIModelGatewayConfig()
				cfg.APIKey = "azure-key"
				cfg.BaseURL = "https://example.openai.azure.com"
				cfg.DefaultDeployment = "deployment"
				cfg.HTTPClient = client
				return NewAzureOpenAIModelGateway(cfg)
			},
		},
		{
			name:     "ollama",
			response: `{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"memory_write","arguments":{"input":"store this"}}}]},"done":false}`,
			wantPath: "/api/chat",
			newGateway: func(client *http.Client) (ModelGateway, error) {
				cfg := DefaultOllamaModelGatewayConfig()
				cfg.BaseURL = "http://localhost:11434"
				cfg.DefaultModel = "llama3.2"
				cfg.HTTPClient = client
				return NewOllamaModelGateway(cfg)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured map[string]any
			var capturedPath string
			client := &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
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
						Body:       io.NopCloser(strings.NewReader(tc.response)),
						Header:     make(http.Header),
					}, nil
				}),
				Timeout: time.Second,
			}

			gateway, err := tc.newGateway(client)
			if err != nil {
				t.Fatalf("new gateway failed: %v", err)
			}

			resp, err := gateway.Complete(context.Background(), aliasRegressionRequest())
			if err != nil {
				t.Fatalf("complete failed: %v", err)
			}
			if capturedPath != tc.wantPath {
				t.Fatalf("unexpected request path: got %q want %q", capturedPath, tc.wantPath)
			}
			assertJSONToolNames(t, captured, "memory_write", "github-mcp--create_issue")
			assertJSONAssistantHistoryToolName(t, captured, "memory_write")
			assertRuntimeToolCall(t, resp, "memory.write", "memory_write")
		})
	}
}

func TestAnthropicModelGatewayUsesToolAliasesForToolsHistoryAndResponses(t *testing.T) {
	var captured map[string]any
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
					`{"content":[{"type":"tool_use","id":"toolu_resp","name":"memory_write","input":{"input":"store this"}}]}`,
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

	resp, err := gateway.Complete(context.Background(), aliasRegressionRequest())
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	assertAnthropicToolNames(t, captured, "memory_write", "github-mcp--create_issue")
	assertAnthropicAssistantHistoryToolName(t, captured, "memory_write")
	assertRuntimeToolCall(t, resp, "memory.write", "memory_write")
}

func TestBedrockModelGatewayUsesToolAliasesForToolsHistoryAndResponses(t *testing.T) {
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
									ToolUseId: aws.String("toolu_resp"),
									Name:      aws.String("memory_write"),
									Input:     document.NewLazyDocument(map[string]any{"input": "store this"}),
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
	resp, err := gateway.Complete(context.Background(), aliasRegressionRequest())
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("expected captured Bedrock request")
	}
	assertBedrockToolNames(t, capturedInput, "memory_write", "github-mcp--create_issue")
	assertBedrockAssistantHistoryToolName(t, capturedInput, "memory_write")
	assertRuntimeToolCall(t, resp, "memory.write", "memory_write")
}

func TestProviderToolNameForHistoryPrefersCurrentAliasMap(t *testing.T) {
	got := providerToolNameForHistory(
		"memory.write",
		"stale_provider_name",
		map[string]string{"memory.write": "memory_write"},
	)
	if got != "memory_write" {
		t.Fatalf("expected current request alias memory_write, got %q", got)
	}
}

func aliasRegressionRequest() ModelRequest {
	return ModelRequest{
		Model: "test-model",
		Step:  2,
		Tools: []string{"memory.write", "github-mcp--create_issue"},
		Messages: []ChatMessage{
			{Role: "user", Content: "remember this"},
			{Role: "assistant", ToolCalls: []ChatToolCall{
				{ID: "call_prev", Name: "memory.write", Input: `{"key":"x","value":"y"}`},
			}},
			{Role: "tool", ToolCallID: "call_prev", Content: "stored"},
			{Role: "user", Content: "continue"},
		},
	}
}

func assertRuntimeToolCall(t *testing.T, resp ModelResponse, runtimeName string, providerName string) {
	t.Helper()
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != runtimeName {
		t.Fatalf("expected runtime tool name %q, got %q", runtimeName, resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].ProviderName != providerName {
		t.Fatalf("expected provider tool name %q, got %q", providerName, resp.ToolCalls[0].ProviderName)
	}
}

func assertJSONToolNames(t *testing.T, body map[string]any, want ...string) {
	t.Helper()
	rawTools, ok := body["tools"].([]any)
	if !ok {
		t.Fatalf("expected tools array in request, got %T", body["tools"])
	}
	got := make([]string, 0, len(rawTools))
	for _, raw := range rawTools {
		tool, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("expected tool object, got %T", raw)
		}
		fn, ok := tool["function"].(map[string]any)
		if !ok {
			t.Fatalf("expected function object, got %T", tool["function"])
		}
		got = append(got, strings.TrimSpace(fmt.Sprint(fn["name"])))
	}
	assertContainsAll(t, got, want...)
}

func assertJSONAssistantHistoryToolName(t *testing.T, body map[string]any, want string) {
	t.Helper()
	rawMessages, ok := body["messages"].([]any)
	if !ok {
		t.Fatalf("expected messages array in request, got %T", body["messages"])
	}
	for _, raw := range rawMessages {
		msg, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(fmt.Sprint(msg["role"])) != "assistant" {
			continue
		}
		rawCalls, _ := msg["tool_calls"].([]any)
		for _, rawCall := range rawCalls {
			call, ok := rawCall.(map[string]any)
			if !ok {
				continue
			}
			fn, _ := call["function"].(map[string]any)
			if strings.TrimSpace(fmt.Sprint(fn["name"])) == want {
				return
			}
		}
	}
	t.Fatalf("expected assistant history tool name %q in request body: %+v", want, body)
}

func assertAnthropicToolNames(t *testing.T, body map[string]any, want ...string) {
	t.Helper()
	rawTools, ok := body["tools"].([]any)
	if !ok {
		t.Fatalf("expected tools array in request, got %T", body["tools"])
	}
	got := make([]string, 0, len(rawTools))
	for _, raw := range rawTools {
		tool, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("expected tool object, got %T", raw)
		}
		got = append(got, strings.TrimSpace(fmt.Sprint(tool["name"])))
	}
	assertContainsAll(t, got, want...)
}

func assertAnthropicAssistantHistoryToolName(t *testing.T, body map[string]any, want string) {
	t.Helper()
	rawMessages, ok := body["messages"].([]any)
	if !ok {
		t.Fatalf("expected messages array in request, got %T", body["messages"])
	}
	for _, raw := range rawMessages {
		msg, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(fmt.Sprint(msg["role"])) != "assistant" {
			continue
		}
		blocks, _ := msg["content"].([]any)
		for _, rawBlock := range blocks {
			block, ok := rawBlock.(map[string]any)
			if !ok || strings.TrimSpace(fmt.Sprint(block["type"])) != "tool_use" {
				continue
			}
			if strings.TrimSpace(fmt.Sprint(block["name"])) == want {
				return
			}
		}
	}
	t.Fatalf("expected Anthropic assistant history tool_use.name %q in request body: %+v", want, body)
}

func assertBedrockToolNames(t *testing.T, input *bedrockruntime.ConverseInput, want ...string) {
	t.Helper()
	if input.ToolConfig == nil {
		t.Fatal("expected Bedrock tool config")
	}
	got := make([]string, 0, len(input.ToolConfig.Tools))
	for _, raw := range input.ToolConfig.Tools {
		spec, ok := raw.(*types.ToolMemberToolSpec)
		if !ok {
			continue
		}
		got = append(got, aws.ToString(spec.Value.Name))
	}
	assertContainsAll(t, got, want...)
}

func assertBedrockAssistantHistoryToolName(t *testing.T, input *bedrockruntime.ConverseInput, want string) {
	t.Helper()
	for _, msg := range input.Messages {
		if msg.Role != types.ConversationRoleAssistant {
			continue
		}
		for _, raw := range msg.Content {
			block, ok := raw.(*types.ContentBlockMemberToolUse)
			if !ok {
				continue
			}
			if aws.ToString(block.Value.Name) == want {
				return
			}
		}
	}
	t.Fatalf("expected Bedrock assistant history tool_use.name %q in request", want)
}

func assertContainsAll(t *testing.T, got []string, want ...string) {
	t.Helper()
	seen := make(map[string]bool, len(got))
	for _, item := range got {
		seen[item] = true
	}
	for _, item := range want {
		if !seen[item] {
			t.Fatalf("expected %q in %v", item, got)
		}
	}
}
