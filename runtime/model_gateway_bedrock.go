package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// bedrockConverser abstracts the Bedrock SDK client for testing.
type bedrockConverser interface {
	Converse(ctx context.Context, params *bedrockruntime.ConverseInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
}

// BedrockModelGatewayConfig defines AWS Bedrock Converse API settings.
type BedrockModelGatewayConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Profile         string
	BaseURL         string
	DefaultModel    string
	MaxTokens       int
	Timeout         time.Duration
}

// DefaultBedrockModelGatewayConfig returns Bedrock gateway defaults.
func DefaultBedrockModelGatewayConfig() BedrockModelGatewayConfig {
	return BedrockModelGatewayConfig{
		MaxTokens: 1024,
		Timeout:   30 * time.Second,
	}
}

// BedrockModelGateway calls the AWS Bedrock Converse API.
type BedrockModelGateway struct {
	client       bedrockConverser
	defaultModel string
	maxTokens    int
}

// NewBedrockModelGateway builds a Bedrock gateway from the given config.
func NewBedrockModelGateway(cfg BedrockModelGatewayConfig) (*BedrockModelGateway, error) {
	if strings.TrimSpace(cfg.Region) == "" {
		return nil, fmt.Errorf("bedrock region is required (set options.region)")
	}
	if strings.TrimSpace(cfg.DefaultModel) == "" {
		return nil, fmt.Errorf("bedrock default model is required")
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(strings.TrimSpace(cfg.Region)),
	}
	if strings.TrimSpace(cfg.Profile) != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(strings.TrimSpace(cfg.Profile)))
	}
	if strings.TrimSpace(cfg.AccessKeyID) != "" && strings.TrimSpace(cfg.SecretAccessKey) != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				strings.TrimSpace(cfg.AccessKeyID),
				strings.TrimSpace(cfg.SecretAccessKey),
				strings.TrimSpace(cfg.SessionToken),
			),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	var clientOpts []func(*bedrockruntime.Options)
	if strings.TrimSpace(cfg.BaseURL) != "" {
		endpoint := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
		clientOpts = append(clientOpts, func(o *bedrockruntime.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}
	client := bedrockruntime.NewFromConfig(awsCfg, clientOpts...)

	return &BedrockModelGateway{
		client:       client,
		defaultModel: strings.TrimSpace(cfg.DefaultModel),
		maxTokens:    maxTokens,
	}, nil
}

// newBedrockModelGatewayWithClient creates a gateway with an injected client (for testing).
func newBedrockModelGatewayWithClient(client bedrockConverser, defaultModel string, maxTokens int) *BedrockModelGateway {
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	return &BedrockModelGateway{
		client:       client,
		defaultModel: defaultModel,
		maxTokens:    maxTokens,
	}
}

func (g *BedrockModelGateway) Complete(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	if g == nil {
		return ModelResponse{}, fmt.Errorf("bedrock model gateway is nil")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = g.defaultModel
	}
	if model == "" {
		return ModelResponse{}, fmt.Errorf("model is required")
	}

	var toolAliases providerToolAliases
	var tools []types.Tool
	if len(req.Tools) > 0 {
		tools, toolAliases = buildBedrockToolsWithAliases(req.Tools, req.ToolSchemas)
	}
	system, messages := chatMessagesToBedrockWithAliases(req, toolAliases.RuntimeToProvider)

	input := &bedrockruntime.ConverseInput{
		ModelId:  aws.String(model),
		Messages: messages,
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens: aws.Int32(int32(g.maxTokens)),
		},
	}
	if len(system) > 0 {
		input.System = append(system, &types.SystemContentBlockMemberCachePoint{
			Value: types.CachePointBlock{Type: types.CachePointTypeDefault},
		})
	}

	if len(req.Tools) > 0 {
		tools = append(tools, &types.ToolMemberCachePoint{
			Value: types.CachePointBlock{Type: types.CachePointTypeDefault},
		})
		input.ToolConfig = &types.ToolConfiguration{Tools: tools}
	}

	if len(req.OutputSchema) > 0 {
		schemaJSON, err := json.Marshal(ensureAdditionalPropertiesFalse(req.OutputSchema))
		if err == nil {
			input.OutputConfig = &types.OutputConfig{
				TextFormat: &types.OutputFormat{
					Type: types.OutputFormatTypeJsonSchema,
					Structure: &types.OutputFormatStructureMemberJsonSchema{
						Value: types.JsonSchemaDefinition{
							Name:   aws.String("agent_output"),
							Schema: aws.String(string(schemaJSON)),
						},
					},
				},
			}
		}
	}

	output, err := g.client.Converse(ctx, input)
	if err != nil {
		return ModelResponse{}, mapBedrockError(err)
	}

	return parseBedrockResponse(output, toolAliases.ProviderToRuntime)
}

func chatMessagesToBedrock(req ModelRequest) ([]types.SystemContentBlock, []types.Message) {
	return chatMessagesToBedrockWithAliases(req, nil)
}

func chatMessagesToBedrockWithAliases(req ModelRequest, aliases map[string]string) ([]types.SystemContentBlock, []types.Message) {
	if len(req.Messages) == 0 {
		userContent := buildOpenAIUserContent(req)
		var system []types.SystemContentBlock
		if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
			system = append(system, &types.SystemContentBlockMemberText{Value: prompt})
		}
		msg := types.Message{
			Role:    types.ConversationRoleUser,
			Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: userContent}},
		}
		return system, []types.Message{msg}
	}

	var systemBlocks []types.SystemContentBlock
	out := make([]types.Message, 0, len(req.Messages))

	for _, m := range req.Messages {
		role := strings.TrimSpace(m.Role)
		content := strings.TrimSpace(m.Content)

		if role == "system" {
			if content != "" {
				systemBlocks = append(systemBlocks, &types.SystemContentBlockMemberText{Value: content})
			}
			continue
		}

		if role == "assistant" && len(m.ToolCalls) > 0 {
			blocks := make([]types.ContentBlock, 0, len(m.ToolCalls)+1)
			if content != "" {
				blocks = append(blocks, &types.ContentBlockMemberText{Value: content})
			}
			for _, tc := range m.ToolCalls {
				inputDoc := document.NewLazyDocument(parseToolCallInput(tc.Input))
				blocks = append(blocks, &types.ContentBlockMemberToolUse{
					Value: types.ToolUseBlock{
						ToolUseId: aws.String(tc.ID),
						Name:      aws.String(providerToolNameForHistory(tc.Name, tc.ProviderName, aliases)),
						Input:     inputDoc,
					},
				})
			}
			out = append(out, types.Message{
				Role:    types.ConversationRoleAssistant,
				Content: blocks,
			})
			continue
		}

		if role == "tool" && m.ToolCallID != "" {
			resultBlock := types.ToolResultBlock{
				ToolUseId: aws.String(m.ToolCallID),
				Content: []types.ToolResultContentBlock{
					&types.ToolResultContentBlockMemberText{Value: content},
				},
			}
			if m.IsError {
				resultBlock.Status = types.ToolResultStatusError
			}
			out = append(out, types.Message{
				Role: types.ConversationRoleUser,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberToolResult{
						Value: resultBlock,
					},
				},
			})
			continue
		}

		if content == "" {
			continue
		}
		apiRole := types.ConversationRoleUser
		if role == "assistant" {
			apiRole = types.ConversationRoleAssistant
		}
		out = append(out, types.Message{
			Role:    apiRole,
			Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: content}},
		})
	}

	return systemBlocks, out
}

// parseToolCallInput attempts JSON parse; falls back to wrapping as {"input": ...}.
func parseToolCallInput(raw string) interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]interface{}{}
	}
	if raw[0] == '{' {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &m); err == nil {
			return m
		}
	}
	return map[string]interface{}{"input": raw}
}

func buildBedrockTools(toolNames []string, schemas map[string]ToolSchemaInfo) ([]types.Tool, map[string]string) {
	tools, aliases := buildBedrockToolsWithAliases(toolNames, schemas)
	return tools, aliases.ProviderToRuntime
}

func buildBedrockToolsWithAliases(toolNames []string, schemas map[string]ToolSchemaInfo) ([]types.Tool, providerToolAliases) {
	deduped := dedupeStrings(toolNames)
	out := make([]types.Tool, 0, len(deduped))
	aliases := buildProviderToolAliases(deduped)

	for _, name := range deduped {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		providerName := aliases.RuntimeToProvider[name]

		description := "Invoke tool " + name
		inputSchema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input": map[string]interface{}{"type": "string"},
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

		out = append(out, &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        aws.String(providerName),
				Description: aws.String(description),
				InputSchema: &types.ToolInputSchemaMemberJson{
					Value: document.NewLazyDocument(inputSchema),
				},
			},
		})
	}
	return out, aliases
}

func parseBedrockResponse(output *bedrockruntime.ConverseOutput, toolAliases map[string]string) (ModelResponse, error) {
	msg, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return ModelResponse{}, fmt.Errorf("unexpected bedrock output type %T", output.Output)
	}

	texts := make([]string, 0, len(msg.Value.Content))
	toolCalls := make([]ModelToolCall, 0)

	for _, block := range msg.Value.Content {
		switch v := block.(type) {
		case *types.ContentBlockMemberText:
			text := strings.TrimSpace(v.Value)
			if text != "" {
				texts = append(texts, text)
			}
		case *types.ContentBlockMemberToolUse:
			name := aws.ToString(v.Value.Name)
			if name == "" {
				continue
			}
			originalName := name
			if mapped, ok := toolAliases[name]; ok && strings.TrimSpace(mapped) != "" {
				originalName = strings.TrimSpace(mapped)
			}
			toolCalls = append(toolCalls, ModelToolCall{
				ID:           aws.ToString(v.Value.ToolUseId),
				Name:         originalName,
				Input:        marshalToolUseInput(v.Value.Input),
				ProviderName: name,
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
		Usage:     parseBedrockUsage(output.Usage),
	}, nil
}

func marshalToolUseInput(doc document.Interface) string {
	if doc == nil {
		return ""
	}
	encoded, err := doc.MarshalSmithyDocument()
	if err != nil {
		return ""
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(encoded, &raw); err == nil {
		if v, ok := raw["input"]; ok {
			if str, ok := v.(string); ok {
				return strings.TrimSpace(str)
			}
		}
	}
	return strings.TrimSpace(string(encoded))
}

func parseBedrockUsage(raw *types.TokenUsage) ModelUsage {
	usage := ModelUsage{Source: "provider"}
	if raw == nil {
		return usage
	}
	usage.InputTokens = max(0, int(aws.ToInt32(raw.InputTokens)))
	usage.OutputTokens = max(0, int(aws.ToInt32(raw.OutputTokens)))
	usage.TotalTokens = max(0, int(aws.ToInt32(raw.TotalTokens)))
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	return usage
}

func mapBedrockError(err error) error {
	var throttling *types.ThrottlingException
	if errors.As(err, &throttling) {
		return &ModelGatewayError{StatusCode: 429, Provider: "bedrock", Message: throttling.ErrorMessage()}
	}
	var timeout *types.ModelTimeoutException
	if errors.As(err, &timeout) {
		return &ModelGatewayError{StatusCode: 504, Provider: "bedrock", Message: timeout.ErrorMessage()}
	}
	var modelErr *types.ModelErrorException
	if errors.As(err, &modelErr) {
		return &ModelGatewayError{StatusCode: 500, Provider: "bedrock", Message: modelErr.ErrorMessage()}
	}
	var validation *types.ValidationException
	if errors.As(err, &validation) {
		return &ModelGatewayError{StatusCode: 400, Provider: "bedrock", Message: validation.ErrorMessage()}
	}
	var accessDenied *types.AccessDeniedException
	if errors.As(err, &accessDenied) {
		return &ModelGatewayError{StatusCode: 403, Provider: "bedrock", Message: accessDenied.ErrorMessage()}
	}
	var notFound *types.ResourceNotFoundException
	if errors.As(err, &notFound) {
		return &ModelGatewayError{StatusCode: 404, Provider: "bedrock", Message: notFound.ErrorMessage()}
	}
	return fmt.Errorf("bedrock model request failed: %w", err)
}
