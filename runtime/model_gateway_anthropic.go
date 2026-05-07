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
		AnthropicVersion: "2023-06-01",
		MaxTokens:        1024,
		Timeout:          120 * time.Second,
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
	var toolAliases providerToolAliases
	if len(req.Tools) > 0 {
		body.Tools, toolAliases = buildAnthropicToolsWithAliases(req.Tools, req.ToolSchemas)
	}
	if len(req.Messages) > 0 {
		body.System, body.Messages = chatMessagesToAnthropicWithAliases(req.Messages, toolAliases.RuntimeToProvider)
	} else {
		body.Messages = []anthropicMessagesInput{{
			Role:    "user",
			Content: buildOpenAIUserContent(req),
		}}
		if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
			body.System = []anthropicSystemBlock{{
				Type:         "text",
				Text:         prompt,
				CacheControl: &anthropicCacheControl{Type: "ephemeral"},
			}}
		}
	}
	if len(req.OutputSchema) > 0 {
		schema := ensureAdditionalPropertiesFalse(req.OutputSchema)
		body.OutputConfig = &anthropicOutputConfig{
			Format: &anthropicOutputFormat{
				Type:   "json_schema",
				Schema: schema,
			},
		}
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
		return ModelResponse{}, &ModelGatewayError{
			StatusCode: httpResp.StatusCode,
			Provider:   "anthropic",
			Message:    providerErr,
		}
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
			originalName := name
			if mapped, ok := toolAliases.ProviderToRuntime[name]; ok && strings.TrimSpace(mapped) != "" {
				originalName = strings.TrimSpace(mapped)
			}
			toolCalls = append(toolCalls, ModelToolCall{
				ID:           strings.TrimSpace(part.ID),
				Name:         originalName,
				Input:        parseAnthropicToolUseInput(part.Input),
				ProviderName: name,
			})
		}
	}
	content := strings.TrimSpace(strings.Join(texts, "\n"))
	if content == "" && len(toolCalls) == 0 {
		return ModelResponse{
			Content: "",
			Done:    true,
			Usage:   parseAnthropicUsage(parsed.Usage),
		}, nil
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
	Model        string                   `json:"model"`
	System       []anthropicSystemBlock   `json:"system,omitempty"`
	Messages     []anthropicMessagesInput `json:"messages"`
	MaxTokens    int                      `json:"max_tokens"`
	Tools        []anthropicToolSpec      `json:"tools,omitempty"`
	OutputConfig *anthropicOutputConfig   `json:"output_config,omitempty"`
}

type anthropicSystemBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicCacheControl struct {
	Type string `json:"type"`
}

type anthropicOutputConfig struct {
	Format *anthropicOutputFormat `json:"format,omitempty"`
}

type anthropicOutputFormat struct {
	Type   string         `json:"type"`
	Schema map[string]any `json:"schema,omitempty"`
}

type anthropicMessagesInput struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type anthropicMessagesResponse struct {
	Content []anthropicMessagesOutput `json:"content,omitempty"`
	Error   *anthropicProviderError   `json:"error,omitempty"`
	Usage   *anthropicMessagesUsage   `json:"usage,omitempty"`
}

type anthropicMessagesOutput struct {
	Type  string         `json:"type"`
	ID    string         `json:"id,omitempty"`
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
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	InputSchema  map[string]any         `json:"input_schema,omitempty"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

func buildAnthropicTools(toolNames []string, schemas map[string]ToolSchemaInfo) ([]anthropicToolSpec, map[string]string) {
	tools, aliases := buildAnthropicToolsWithAliases(toolNames, schemas)
	return tools, aliases.ProviderToRuntime
}

func buildAnthropicToolsWithAliases(toolNames []string, schemas map[string]ToolSchemaInfo) ([]anthropicToolSpec, providerToolAliases) {
	deduped := dedupeStrings(toolNames)
	out := make([]anthropicToolSpec, 0, len(deduped))
	aliases := buildProviderToolAliases(deduped)
	for _, name := range deduped {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		providerName := aliases.RuntimeToProvider[name]
		description := "Invoke tool " + name
		inputSchema := map[string]any{
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
				inputSchema = info.InputSchema
			}
		}
		if schema, ok := builtinToolSchemaForName(name); ok {
			description = schema.Description
			inputSchema = schema.Parameters
		}
		out = append(out, anthropicToolSpec{
			Name:        providerName,
			Description: description,
			InputSchema: inputSchema,
		})
	}
	if len(out) > 0 {
		out[len(out)-1].CacheControl = &anthropicCacheControl{Type: "ephemeral"}
	}
	return out, aliases
}

func anthropicToolName(name string, used map[string]struct{}) string {
	return sanitizeToolName(name, used)
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

func chatMessagesToAnthropic(msgs []ChatMessage) ([]anthropicSystemBlock, []anthropicMessagesInput) {
	return chatMessagesToAnthropicWithAliases(msgs, nil)
}

func chatMessagesToAnthropicWithAliases(msgs []ChatMessage, aliases map[string]string) ([]anthropicSystemBlock, []anthropicMessagesInput) {
	var systemBlocks []anthropicSystemBlock
	out := make([]anthropicMessagesInput, 0, len(msgs))
	for _, m := range msgs {
		role := strings.TrimSpace(m.Role)
		content := strings.TrimSpace(m.Content)

		if role == "system" {
			if content == "" {
				continue
			}
			systemBlocks = append(systemBlocks, anthropicSystemBlock{
				Type: "text",
				Text: content,
			})
			continue
		}

		if role == "assistant" && len(m.ToolCalls) > 0 {
			blocks := make([]map[string]interface{}, 0, len(m.ToolCalls)+1)
			if content != "" {
				blocks = append(blocks, map[string]interface{}{
					"type": "text",
					"text": content,
				})
			}
			for _, tc := range m.ToolCalls {
				inputMap := map[string]interface{}{"input": tc.Input}
				if parsed := parseJSONLoose(tc.Input); parsed != nil {
					inputMap = parsed
				}
				blocks = append(blocks, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  providerToolNameForHistory(tc.Name, tc.ProviderName, aliases),
					"input": inputMap,
				})
			}
			out = append(out, anthropicMessagesInput{
				Role:    "assistant",
				Content: blocks,
			})
			continue
		}

		if role == "tool" && m.ToolCallID != "" {
			block := map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": m.ToolCallID,
				"content":     content,
			}
			if m.IsError {
				block["is_error"] = true
			}
			if len(out) > 0 && out[len(out)-1].Role == "user" {
				out[len(out)-1].Content = mergeAnthropicUserContent(out[len(out)-1].Content, block)
			} else {
				out = append(out, anthropicMessagesInput{
					Role:    "user",
					Content: []map[string]interface{}{block},
				})
			}
			continue
		}

		if content == "" {
			continue
		}
		apiRole := role
		if apiRole != "user" && apiRole != "assistant" {
			apiRole = "user"
		}
		if apiRole == "user" && len(out) > 0 && out[len(out)-1].Role == "user" {
			out[len(out)-1].Content = mergeAnthropicUserContent(out[len(out)-1].Content, content)
		} else {
			out = append(out, anthropicMessagesInput{
				Role:    apiRole,
				Content: content,
			})
		}
	}
	if len(systemBlocks) > 0 {
		systemBlocks[len(systemBlocks)-1].CacheControl = &anthropicCacheControl{Type: "ephemeral"}
	}
	return systemBlocks, out
}

// mergeAnthropicUserContent appends new content into an existing user
// message, converting to the array-of-blocks form when needed. The added
// content can be a plain string (converted to a text block) or a
// map[string]interface{} block (e.g. tool_result). This ensures
// consecutive user messages are collapsed into one, which the Anthropic
// Messages API requires (roles must alternate).
func mergeAnthropicUserContent(existing interface{}, added interface{}) interface{} {
	var newBlock map[string]interface{}
	switch a := added.(type) {
	case string:
		newBlock = map[string]interface{}{"type": "text", "text": a}
	case map[string]interface{}:
		newBlock = a
	default:
		newBlock = map[string]interface{}{"type": "text", "text": fmt.Sprintf("%v", added)}
	}
	switch v := existing.(type) {
	case []map[string]interface{}:
		return append(v, newBlock)
	case string:
		return []map[string]interface{}{
			{"type": "text", "text": v},
			newBlock,
		}
	default:
		return []map[string]interface{}{newBlock}
	}
}

func parseJSONLoose(s string) map[string]interface{} {
	s = strings.TrimSpace(s)
	if s == "" || s[0] != '{' {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}

// anthropicUnsupportedSchemaKeys lists JSON Schema validation keywords that
// Anthropic's structured output API rejects. They are silently stripped so
// users can write standard JSON Schemas without provider-specific workarounds.
var anthropicUnsupportedSchemaKeys = map[string]bool{
	"minimum":          true,
	"maximum":          true,
	"exclusiveMinimum": true,
	"exclusiveMaximum": true,
	"multipleOf":       true,
	"minLength":        true,
	"maxLength":        true,
	"pattern":          true,
	"minItems":         true,
	"maxItems":         true,
	"uniqueItems":      true,
	"minProperties":    true,
	"maxProperties":    true,
}

// ensureAdditionalPropertiesFalse recursively normalizes a JSON Schema for
// Anthropic's structured output API: sets "additionalProperties": false on
// every object-typed node (required by Anthropic) and strips validation
// keywords that Anthropic does not support (minimum, maximum, pattern, etc.).
func ensureAdditionalPropertiesFalse(schema map[string]any) map[string]any {
	if schema == nil {
		return schema
	}
	out := make(map[string]any, len(schema))
	for k, v := range schema {
		if anthropicUnsupportedSchemaKeys[k] {
			continue
		}
		out[k] = v
	}

	typ, _ := out["type"].(string)
	if typ == "object" {
		if _, exists := out["additionalProperties"]; !exists {
			out["additionalProperties"] = false
		}
	}

	if props, ok := out["properties"].(map[string]any); ok {
		patched := make(map[string]any, len(props))
		for k, v := range props {
			if sub, ok := v.(map[string]any); ok {
				patched[k] = ensureAdditionalPropertiesFalse(sub)
			} else {
				patched[k] = v
			}
		}
		out["properties"] = patched
	}

	if items, ok := out["items"].(map[string]any); ok {
		out["items"] = ensureAdditionalPropertiesFalse(items)
	}

	return out
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
