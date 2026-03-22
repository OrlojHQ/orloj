package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// OpenAIModelGatewayConfig defines OpenAI-compatible model gateway settings.
type OpenAIModelGatewayConfig struct {
	APIKey       string
	BaseURL      string
	DefaultModel string
	Timeout      time.Duration
	HTTPClient   *http.Client
}

// DefaultOpenAIModelGatewayConfig returns OpenAI gateway defaults.
func DefaultOpenAIModelGatewayConfig() OpenAIModelGatewayConfig {
	return OpenAIModelGatewayConfig{
		BaseURL:      "https://api.openai.com/v1",
		DefaultModel: "gpt-4o-mini",
		Timeout:      30 * time.Second,
	}
}

// OpenAIModelGateway calls an OpenAI-compatible Chat Completions endpoint.
type OpenAIModelGateway struct {
	apiKey       string
	baseURL      string
	defaultModel string
	client       *http.Client
}

func NewOpenAIModelGateway(cfg OpenAIModelGatewayConfig) (*OpenAIModelGateway, error) {
	normalized := cfg.normalized()
	if strings.TrimSpace(normalized.APIKey) == "" {
		return nil, fmt.Errorf("openai api key is required")
	}
	if strings.TrimSpace(normalized.BaseURL) == "" {
		return nil, fmt.Errorf("openai base URL is required")
	}
	if normalized.client() == nil {
		return nil, fmt.Errorf("openai HTTP client is required")
	}
	return &OpenAIModelGateway{
		apiKey:       strings.TrimSpace(normalized.APIKey),
		baseURL:      strings.TrimRight(strings.TrimSpace(normalized.BaseURL), "/"),
		defaultModel: strings.TrimSpace(normalized.DefaultModel),
		client:       normalized.client(),
	}, nil
}

func (c OpenAIModelGatewayConfig) normalized() OpenAIModelGatewayConfig {
	out := c
	defaults := DefaultOpenAIModelGatewayConfig()
	if strings.TrimSpace(out.BaseURL) == "" {
		out.BaseURL = defaults.BaseURL
	}
	if strings.TrimSpace(out.DefaultModel) == "" {
		out.DefaultModel = defaults.DefaultModel
	}
	if out.Timeout <= 0 {
		out.Timeout = defaults.Timeout
	}
	return out
}

func (c OpenAIModelGatewayConfig) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	if c.Timeout <= 0 {
		return nil
	}
	return &http.Client{Timeout: c.Timeout}
}

func (g *OpenAIModelGateway) Complete(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	if g == nil {
		return ModelResponse{}, fmt.Errorf("openai model gateway is nil")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(g.defaultModel)
	}
	if model == "" {
		return ModelResponse{}, fmt.Errorf("model is required")
	}

	body := openAIChatCompletionRequest{
		Model: model,
	}
	if len(req.Messages) > 0 {
		body.Messages = chatMessagesToOpenAI(req.Messages)
	} else {
		body.Messages = []openAIChatCompletionMessage{
			{Role: "system", Content: strings.TrimSpace(req.Prompt)},
			{Role: "user", Content: buildOpenAIUserContent(req)},
		}
		if strings.TrimSpace(req.Prompt) == "" {
			body.Messages = body.Messages[1:]
		}
	}
	if len(req.Tools) > 0 {
		body.Tools = buildOpenAIChatTools(req.Tools, req.ToolSchemas)
		body.ToolChoice = "auto"
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("marshal model request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return ModelResponse{}, fmt.Errorf("build model request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)

	httpResp, err := g.client.Do(httpReq)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("model request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("read model response: %w", err)
	}

	if httpResp.StatusCode >= http.StatusBadRequest {
		providerErr := parseOpenAIError(respBody)
		if providerErr == "" {
			providerErr = strings.TrimSpace(string(respBody))
		}
		return ModelResponse{}, &ModelGatewayError{
			StatusCode: httpResp.StatusCode,
			Provider:   "openai",
			Message:    providerErr,
		}
	}

	parsed := openAIChatCompletionResponse{}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ModelResponse{}, fmt.Errorf("decode model response: %w", err)
	}
	if parsed.Error != nil {
		return ModelResponse{}, fmt.Errorf("model provider error: %s", strings.TrimSpace(parsed.Error.Message))
	}
	if len(parsed.Choices) == 0 {
		return ModelResponse{}, fmt.Errorf("model response missing choices")
	}
	choice := parsed.Choices[0]
	content := parseOpenAIMessageContent(choice.Message.Content)
	toolCalls := parseOpenAIModelToolCalls(choice.Message.ToolCalls)
	if content == "" && len(toolCalls) == 0 {
		return ModelResponse{}, fmt.Errorf("model response missing message content")
	}
	return ModelResponse{
		Content:   content,
		Done:      false,
		ToolCalls: toolCalls,
		Usage:     parseOpenAIUsage(parsed.Usage, "provider"),
	}, nil
}

func buildOpenAIUserContent(req ModelRequest) string {
	lines := make([]string, 0, 4)
	lines = append(lines, fmt.Sprintf("step=%d", req.Step))
	if len(req.Tools) > 0 {
		lines = append(lines, "tools="+strings.Join(req.Tools, ","))
	}
	if len(req.Context) > 0 {
		keys := make([]string, 0, len(req.Context))
		for key := range req.Context {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", key, req.Context[key]))
		}
		lines = append(lines, "context="+strings.Join(parts, ","))
	}
	return strings.Join(lines, "\n")
}

func parseOpenAIError(body []byte) string {
	parsed := openAIChatCompletionResponse{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	if parsed.Error == nil {
		return ""
	}
	return strings.TrimSpace(parsed.Error.Message)
}

func parseOpenAIMessageContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}
	var asParts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &asParts); err == nil {
		texts := make([]string, 0, len(asParts))
		for _, part := range asParts {
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			texts = append(texts, text)
		}
		return strings.TrimSpace(strings.Join(texts, "\n"))
	}
	return ""
}

type openAIChatCompletionRequest struct {
	Model      string                        `json:"model"`
	Messages   []openAIChatCompletionMessage `json:"messages"`
	Tools      []openAIChatTool              `json:"tools,omitempty"`
	ToolChoice string                        `json:"tool_choice,omitempty"`
}

type openAIChatCompletionMessage struct {
	Role       string               `json:"role"`
	Content    interface{}          `json:"content"`
	ToolCalls  []openAIChatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
}

type openAIChatCompletionResponse struct {
	Choices []openAIChatCompletionChoice `json:"choices"`
	Error   *openAIProviderError         `json:"error,omitempty"`
	Usage   *openAIUsage                 `json:"usage,omitempty"`
}

type openAIChatCompletionChoice struct {
	Message openAIChatCompletionMessageResponse `json:"message"`
}

type openAIChatCompletionMessageResponse struct {
	Content   json.RawMessage      `json:"content"`
	ToolCalls []openAIChatToolCall `json:"tool_calls,omitempty"`
}

type openAIProviderError struct {
	Message string `json:"message"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIChatTool struct {
	Type     string                 `json:"type"`
	Function openAIChatToolFunction `json:"function"`
}

type openAIChatToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAIChatToolCall struct {
	ID       string                     `json:"id,omitempty"`
	Type     string                     `json:"type,omitempty"`
	Function openAIChatToolFunctionCall `json:"function"`
}

type openAIChatToolFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

func buildOpenAIChatTools(toolNames []string, schemas map[string]ToolSchemaInfo) []openAIChatTool {
	deduped := dedupeStrings(toolNames)
	out := make([]openAIChatTool, 0, len(deduped))
	for _, name := range deduped {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		description := "Invoke tool " + name
		parameters := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{
					"type": "string",
				},
			},
			"additionalProperties": true,
		}
		if info, ok := schemas[name]; ok {
			if info.Description != "" {
				description = info.Description
			}
			if len(info.InputSchema) > 0 {
				parameters = info.InputSchema
			}
		}
		if schema, ok := builtinToolSchemaForName(name); ok {
			description = schema.Description
			parameters = schema.Parameters
		}
		out = append(out, openAIChatTool{
			Type: "function",
			Function: openAIChatToolFunction{
				Name:        name,
				Description: description,
				Parameters:  parameters,
			},
		})
	}
	return out
}

func parseOpenAIModelToolCalls(raw []openAIChatToolCall) []ModelToolCall {
	out := make([]ModelToolCall, 0, len(raw))
	for _, item := range raw {
		name := strings.TrimSpace(item.Function.Name)
		if name == "" {
			continue
		}
		out = append(out, ModelToolCall{
			ID:    strings.TrimSpace(item.ID),
			Name:  name,
			Input: parseOpenAIToolCallInput(item.Function.Arguments),
		})
	}
	return out
}

func parseOpenAIToolCallInput(arguments string) string {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(arguments), &parsed); err != nil {
		return arguments
	}
	if value, ok := parsed["input"]; ok {
		if str, ok := value.(string); ok {
			return strings.TrimSpace(str)
		}
		encoded, err := json.Marshal(value)
		if err == nil {
			return strings.TrimSpace(string(encoded))
		}
	}
	encoded, err := json.Marshal(parsed)
	if err != nil {
		return arguments
	}
	return strings.TrimSpace(string(encoded))
}

func chatMessagesToOpenAI(msgs []ChatMessage) []openAIChatCompletionMessage {
	out := make([]openAIChatCompletionMessage, 0, len(msgs))
	for _, m := range msgs {
		role := strings.TrimSpace(m.Role)
		content := strings.TrimSpace(m.Content)

		if role == "tool" && m.ToolCallID != "" {
			out = append(out, openAIChatCompletionMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: m.ToolCallID,
			})
			continue
		}

		if role == "assistant" && len(m.ToolCalls) > 0 {
			calls := make([]openAIChatToolCall, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				calls[i] = openAIChatToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: openAIChatToolFunctionCall{
						Name:      tc.Name,
						Arguments: tc.Input,
					},
				}
			}
			var msgContent interface{}
			if content != "" {
				msgContent = content
			}
			out = append(out, openAIChatCompletionMessage{
				Role:      "assistant",
				Content:   msgContent,
				ToolCalls: calls,
			})
			continue
		}

		if content == "" {
			continue
		}
		out = append(out, openAIChatCompletionMessage{
			Role:    role,
			Content: content,
		})
	}
	return out
}

func parseOpenAIUsage(raw *openAIUsage, source string) ModelUsage {
	usage := ModelUsage{Source: strings.TrimSpace(source)}
	if raw == nil {
		return usage
	}
	usage.InputTokens = max(0, raw.PromptTokens)
	usage.OutputTokens = max(0, raw.CompletionTokens)
	usage.TotalTokens = max(0, raw.TotalTokens)
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	return usage
}
