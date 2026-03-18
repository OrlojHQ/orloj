package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AnthropicModelGatewayConfig defines Anthropic Messages API settings.
type AnthropicModelGatewayConfig struct {
	APIKey           string
	BaseURL          string
	DefaultModel     string
	AnthropicVersion string
	MaxTokens        int
	Timeout          time.Duration
	HTTPClient       *http.Client
}

// DefaultAnthropicModelGatewayConfig returns Anthropic gateway defaults.
func DefaultAnthropicModelGatewayConfig() AnthropicModelGatewayConfig {
	return AnthropicModelGatewayConfig{
		BaseURL:          "https://api.anthropic.com/v1",
		DefaultModel:     "claude-3-5-sonnet-latest",
		AnthropicVersion: "2023-06-01",
		MaxTokens:        1024,
		Timeout:          30 * time.Second,
	}
}

// AnthropicModelGateway calls the Anthropic Messages API.
type AnthropicModelGateway struct {
	apiKey           string
	baseURL          string
	defaultModel     string
	anthropicVersion string
	maxTokens        int
	client           *http.Client
}

func NewAnthropicModelGateway(cfg AnthropicModelGatewayConfig) (*AnthropicModelGateway, error) {
	normalized := cfg.normalized()
	if strings.TrimSpace(normalized.APIKey) == "" {
		return nil, fmt.Errorf("anthropic api key is required")
	}
	if strings.TrimSpace(normalized.BaseURL) == "" {
		return nil, fmt.Errorf("anthropic base URL is required")
	}
	if strings.TrimSpace(normalized.AnthropicVersion) == "" {
		return nil, fmt.Errorf("anthropic version is required")
	}
	if normalized.maxTokens() <= 0 {
		return nil, fmt.Errorf("anthropic max tokens must be greater than zero")
	}
	if normalized.client() == nil {
		return nil, fmt.Errorf("anthropic HTTP client is required")
	}
	return &AnthropicModelGateway{
		apiKey:           strings.TrimSpace(normalized.APIKey),
		baseURL:          strings.TrimRight(strings.TrimSpace(normalized.BaseURL), "/"),
		defaultModel:     strings.TrimSpace(normalized.DefaultModel),
		anthropicVersion: strings.TrimSpace(normalized.AnthropicVersion),
		maxTokens:        normalized.maxTokens(),
		client:           normalized.client(),
	}, nil
}

func (c AnthropicModelGatewayConfig) normalized() AnthropicModelGatewayConfig {
	out := c
	defaults := DefaultAnthropicModelGatewayConfig()
	if strings.TrimSpace(out.BaseURL) == "" {
		out.BaseURL = defaults.BaseURL
	}
	if strings.TrimSpace(out.DefaultModel) == "" {
		out.DefaultModel = defaults.DefaultModel
	}
	if strings.TrimSpace(out.AnthropicVersion) == "" {
		out.AnthropicVersion = defaults.AnthropicVersion
	}
	if out.MaxTokens <= 0 {
		out.MaxTokens = defaults.MaxTokens
	}
	if out.Timeout <= 0 {
		out.Timeout = defaults.Timeout
	}
	return out
}

func (c AnthropicModelGatewayConfig) maxTokens() int {
	if c.MaxTokens > 0 {
		return c.MaxTokens
	}
	return 0
}

func (c AnthropicModelGatewayConfig) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	if c.Timeout <= 0 {
		return nil
	}
	return &http.Client{Timeout: c.Timeout}
}

func (g *AnthropicModelGateway) Complete(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	if g == nil {
		return ModelResponse{}, fmt.Errorf("anthropic model gateway is nil")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(g.defaultModel)
	}
	if model == "" {
		return ModelResponse{}, fmt.Errorf("model is required")
	}

	body := anthropicMessagesRequest{
		Model:     model,
		MaxTokens: g.maxTokens,
	}
	if len(req.Messages) > 0 {
		body.System, body.Messages = chatMessagesToAnthropic(req.Messages)
	} else {
		body.Messages = []anthropicMessagesInput{{
			Role:    "user",
			Content: buildOpenAIUserContent(req),
		}}
		if strings.TrimSpace(req.Prompt) != "" {
			body.System = strings.TrimSpace(req.Prompt)
		}
	}
	if len(req.Tools) > 0 {
		body.Tools = buildAnthropicTools(req.Tools)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("marshal model request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/messages", bytes.NewReader(payload))
	if err != nil {
		return ModelResponse{}, fmt.Errorf("build model request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", g.apiKey)
	httpReq.Header.Set("anthropic-version", g.anthropicVersion)

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
		providerErr := parseAnthropicError(respBody)
		if providerErr == "" {
			providerErr = strings.TrimSpace(string(respBody))
		}
		return ModelResponse{}, fmt.Errorf("model request failed status=%d: %s", httpResp.StatusCode, providerErr)
	}

	parsed := anthropicMessagesResponse{}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ModelResponse{}, fmt.Errorf("decode model response: %w", err)
	}
	if parsed.Error != nil {
		return ModelResponse{}, fmt.Errorf("model provider error: %s", strings.TrimSpace(parsed.Error.Message))
	}

	texts := make([]string, 0, len(parsed.Content))
	toolCalls := make([]ModelToolCall, 0, len(parsed.Content))
	for _, part := range parsed.Content {
		switch strings.ToLower(strings.TrimSpace(part.Type)) {
		case "text":
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			texts = append(texts, text)
		case "tool_use":
			name := strings.TrimSpace(part.Name)
			if name == "" {
				continue
			}
			toolCalls = append(toolCalls, ModelToolCall{
				Name:  name,
				Input: parseAnthropicToolUseInput(part.Input),
			})
		}
	}
	content := strings.TrimSpace(strings.Join(texts, "\n"))
	if content == "" && len(toolCalls) == 0 {
		return ModelResponse{}, fmt.Errorf("model response missing message content")
	}
	return ModelResponse{
		Content:   content,
		Done:      false,
		ToolCalls: toolCalls,
		Usage:     parseAnthropicUsage(parsed.Usage),
	}, nil
}

func parseAnthropicError(body []byte) string {
	parsed := anthropicMessagesResponse{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	if parsed.Error == nil {
		return ""
	}
	return strings.TrimSpace(parsed.Error.Message)
}

type anthropicMessagesRequest struct {
	Model     string                   `json:"model"`
	System    string                   `json:"system,omitempty"`
	Messages  []anthropicMessagesInput `json:"messages"`
	MaxTokens int                      `json:"max_tokens"`
	Tools     []anthropicToolSpec      `json:"tools,omitempty"`
}

type anthropicMessagesInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicMessagesResponse struct {
	Content []anthropicMessagesOutput `json:"content,omitempty"`
	Error   *anthropicProviderError   `json:"error,omitempty"`
	Usage   *anthropicMessagesUsage   `json:"usage,omitempty"`
}

type anthropicMessagesOutput struct {
	Type  string         `json:"type"`
	Text  string         `json:"text,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
}

type anthropicProviderError struct {
	Message string `json:"message"`
}

type anthropicMessagesUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type anthropicToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

func buildAnthropicTools(toolNames []string) []anthropicToolSpec {
	deduped := dedupeStrings(toolNames)
	out := make([]anthropicToolSpec, 0, len(deduped))
	for _, name := range deduped {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, anthropicToolSpec{
			Name:        name,
			Description: "Invoke tool " + name,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{
						"type": "string",
					},
				},
				"additionalProperties": true,
			},
		})
	}
	return out
}

func parseAnthropicToolUseInput(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	if value, ok := input["input"]; ok {
		if str, ok := value.(string); ok {
			return strings.TrimSpace(str)
		}
		encoded, err := json.Marshal(value)
		if err == nil {
			return strings.TrimSpace(string(encoded))
		}
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(encoded))
}

func chatMessagesToAnthropic(msgs []ChatMessage) (string, []anthropicMessagesInput) {
	var system string
	out := make([]anthropicMessagesInput, 0, len(msgs))
	for _, m := range msgs {
		role := strings.TrimSpace(m.Role)
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		if role == "system" {
			if system == "" {
				system = content
			} else {
				system += "\n" + content
			}
			continue
		}
		apiRole := role
		if apiRole == "tool" {
			apiRole = "user"
		}
		if apiRole != "user" && apiRole != "assistant" {
			apiRole = "user"
		}
		out = append(out, anthropicMessagesInput{
			Role:    apiRole,
			Content: content,
		})
	}
	return system, out
}

func parseAnthropicUsage(raw *anthropicMessagesUsage) ModelUsage {
	usage := ModelUsage{Source: "provider"}
	if raw == nil {
		return usage
	}
	usage.InputTokens = max(0, raw.InputTokens+raw.CacheCreationInputTokens+raw.CacheReadInputTokens)
	usage.OutputTokens = max(0, raw.OutputTokens)
	usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	return usage
}
