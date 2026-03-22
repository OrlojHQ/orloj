package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AzureOpenAIModelGatewayConfig defines Azure OpenAI chat completion settings.
type AzureOpenAIModelGatewayConfig struct {
	APIKey            string
	BaseURL           string
	DefaultDeployment string
	APIVersion        string
	Timeout           time.Duration
	HTTPClient        *http.Client
}

// DefaultAzureOpenAIModelGatewayConfig returns Azure OpenAI gateway defaults.
func DefaultAzureOpenAIModelGatewayConfig() AzureOpenAIModelGatewayConfig {
	return AzureOpenAIModelGatewayConfig{
		APIVersion: "2024-10-21",
		Timeout:    30 * time.Second,
	}
}

// AzureOpenAIModelGateway calls Azure OpenAI chat completions API.
type AzureOpenAIModelGateway struct {
	apiKey            string
	baseURL           string
	defaultDeployment string
	apiVersion        string
	client            *http.Client
}

func NewAzureOpenAIModelGateway(cfg AzureOpenAIModelGatewayConfig) (*AzureOpenAIModelGateway, error) {
	normalized := cfg.normalized()
	if strings.TrimSpace(normalized.APIKey) == "" {
		return nil, fmt.Errorf("azure openai api key is required")
	}
	if strings.TrimSpace(normalized.BaseURL) == "" {
		return nil, fmt.Errorf("azure openai base URL is required")
	}
	if strings.TrimSpace(normalized.APIVersion) == "" {
		return nil, fmt.Errorf("azure openai api version is required")
	}
	if normalized.client() == nil {
		return nil, fmt.Errorf("azure openai HTTP client is required")
	}
	return &AzureOpenAIModelGateway{
		apiKey:            strings.TrimSpace(normalized.APIKey),
		baseURL:           strings.TrimRight(strings.TrimSpace(normalized.BaseURL), "/"),
		defaultDeployment: strings.TrimSpace(normalized.DefaultDeployment),
		apiVersion:        strings.TrimSpace(normalized.APIVersion),
		client:            normalized.client(),
	}, nil
}

func (c AzureOpenAIModelGatewayConfig) normalized() AzureOpenAIModelGatewayConfig {
	out := c
	defaults := DefaultAzureOpenAIModelGatewayConfig()
	if strings.TrimSpace(out.APIVersion) == "" {
		out.APIVersion = defaults.APIVersion
	}
	if out.Timeout <= 0 {
		out.Timeout = defaults.Timeout
	}
	return out
}

func (c AzureOpenAIModelGatewayConfig) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	if c.Timeout <= 0 {
		return nil
	}
	return &http.Client{Timeout: c.Timeout}
}

func (g *AzureOpenAIModelGateway) Complete(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	if g == nil {
		return ModelResponse{}, fmt.Errorf("azure openai model gateway is nil")
	}
	deployment := strings.TrimSpace(req.Model)
	if deployment == "" {
		deployment = strings.TrimSpace(g.defaultDeployment)
	}
	if deployment == "" {
		return ModelResponse{}, fmt.Errorf("deployment is required")
	}

	body := openAIChatCompletionRequest{
		Model: deployment,
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

	endpoint := fmt.Sprintf(
		"%s/openai/deployments/%s/chat/completions?api-version=%s",
		g.baseURL,
		url.PathEscape(deployment),
		url.QueryEscape(g.apiVersion),
	)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return ModelResponse{}, fmt.Errorf("build model request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", g.apiKey)

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
			Provider:   "azure-openai",
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
