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

// OllamaModelGatewayConfig defines Ollama chat settings.
type OllamaModelGatewayConfig struct {
	BaseURL      string
	DefaultModel string
	Timeout      time.Duration
	HTTPClient   *http.Client
}

// DefaultOllamaModelGatewayConfig returns Ollama gateway defaults.
func DefaultOllamaModelGatewayConfig() OllamaModelGatewayConfig {
	return OllamaModelGatewayConfig{
		BaseURL:      "http://127.0.0.1:11434",
		DefaultModel: "llama3.1",
		Timeout:      30 * time.Second,
	}
}

// OllamaModelGateway calls Ollama's /api/chat endpoint.
type OllamaModelGateway struct {
	baseURL      string
	defaultModel string
	client       *http.Client
}

func NewOllamaModelGateway(cfg OllamaModelGatewayConfig) (*OllamaModelGateway, error) {
	normalized := cfg.normalized()
	if strings.TrimSpace(normalized.BaseURL) == "" {
		return nil, fmt.Errorf("ollama base URL is required")
	}
	if normalized.client() == nil {
		return nil, fmt.Errorf("ollama HTTP client is required")
	}
	return &OllamaModelGateway{
		baseURL:      strings.TrimRight(strings.TrimSpace(normalized.BaseURL), "/"),
		defaultModel: strings.TrimSpace(normalized.DefaultModel),
		client:       normalized.client(),
	}, nil
}

func (c OllamaModelGatewayConfig) normalized() OllamaModelGatewayConfig {
	out := c
	defaults := DefaultOllamaModelGatewayConfig()
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

func (c OllamaModelGatewayConfig) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	if c.Timeout <= 0 {
		return nil
	}
	return &http.Client{Timeout: c.Timeout}
}

func (g *OllamaModelGateway) Complete(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	if g == nil {
		return ModelResponse{}, fmt.Errorf("ollama model gateway is nil")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(g.defaultModel)
	}
	if model == "" {
		return ModelResponse{}, fmt.Errorf("model is required")
	}

	body := ollamaChatRequest{
		Model:  model,
		Stream: false,
	}
	if len(req.Messages) > 0 {
		body.Messages = chatMessagesToOllama(req.Messages)
	} else {
		body.Messages = []ollamaChatMessage{
			{Role: "system", Content: strings.TrimSpace(req.Prompt)},
			{Role: "user", Content: buildOpenAIUserContent(req)},
		}
		if strings.TrimSpace(req.Prompt) == "" {
			body.Messages = body.Messages[1:]
		}
	}
	if len(req.Tools) > 0 {
		body.Tools = buildOpenAIChatTools(req.Tools, req.ToolSchemas)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("marshal model request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return ModelResponse{}, fmt.Errorf("build model request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

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
		providerErr := parseOllamaError(respBody)
		if providerErr == "" {
			providerErr = strings.TrimSpace(string(respBody))
		}
		return ModelResponse{}, fmt.Errorf("model request failed status=%d: %s", httpResp.StatusCode, providerErr)
	}

	parsed := ollamaChatResponse{}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ModelResponse{}, fmt.Errorf("decode model response: %w", err)
	}
	if strings.TrimSpace(parsed.Error) != "" {
		return ModelResponse{}, fmt.Errorf("model provider error: %s", strings.TrimSpace(parsed.Error))
	}
	content := strings.TrimSpace(parsed.Message.Content)
	toolCalls := parseOllamaModelToolCalls(parsed.Message.ToolCalls)
	if content == "" && len(toolCalls) == 0 {
		return ModelResponse{}, fmt.Errorf("model response missing message content")
	}
	return ModelResponse{
		Content:   content,
		Done:      parsed.Done,
		ToolCalls: toolCalls,
		Usage: ModelUsage{
			InputTokens:  max(0, parsed.PromptEvalCount),
			OutputTokens: max(0, parsed.EvalCount),
			TotalTokens:  max(0, parsed.PromptEvalCount+parsed.EvalCount),
			Source:       "provider",
		},
	}, nil
}

func chatMessagesToOllama(msgs []ChatMessage) []ollamaChatMessage {
	out := make([]ollamaChatMessage, 0, len(msgs))
	for _, m := range msgs {
		role := strings.TrimSpace(m.Role)
		content := strings.TrimSpace(m.Content)

		if role == "tool" {
			out = append(out, ollamaChatMessage{Role: "tool", Content: content})
			continue
		}

		if role == "assistant" && len(m.ToolCalls) > 0 {
			calls := make([]ollamaToolCall, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				args, _ := json.Marshal(map[string]string{"input": tc.Input})
				calls[i] = ollamaToolCall{
					Function: ollamaToolCallFunction{
						Name:      tc.Name,
						Arguments: args,
					},
				}
			}
			out = append(out, ollamaChatMessage{Role: "assistant", Content: content, ToolCalls: calls})
			continue
		}

		if content == "" {
			continue
		}
		if role != "system" && role != "assistant" {
			role = "user"
		}
		out = append(out, ollamaChatMessage{Role: role, Content: content})
	}
	return out
}

func parseOllamaError(body []byte) string {
	parsed := ollamaChatResponse{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Error)
}

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Tools    []openAIChatTool    `json:"tools,omitempty"`
}

type ollamaChatMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaChatResponse struct {
	Message         ollamaChatMessage `json:"message,omitempty"`
	Done            bool              `json:"done,omitempty"`
	Error           string            `json:"error,omitempty"`
	PromptEvalCount int               `json:"prompt_eval_count,omitempty"`
	EvalCount       int               `json:"eval_count,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaToolCallFunction `json:"function"`
}

type ollamaToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

func parseOllamaModelToolCalls(raw []ollamaToolCall) []ModelToolCall {
	out := make([]ModelToolCall, 0, len(raw))
	for _, item := range raw {
		name := strings.TrimSpace(item.Function.Name)
		if name == "" {
			continue
		}
		out = append(out, ModelToolCall{
			Name:  name,
			Input: parseOllamaToolCallArguments(item.Function.Arguments),
		})
	}
	return out
}

func parseOllamaToolCallArguments(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err == nil {
		if value, ok := asMap["input"]; ok {
			if str, ok := value.(string); ok {
				return strings.TrimSpace(str)
			}
			encoded, err := json.Marshal(value)
			if err == nil {
				return strings.TrimSpace(string(encoded))
			}
		}
		encoded, err := json.Marshal(asMap)
		if err == nil {
			return strings.TrimSpace(string(encoded))
		}
	}
	return trimmed
}
