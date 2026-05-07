package agentruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var ErrModelGatewayConfiguration = errors.New("model gateway configuration error")

// ModelGatewayConfig configures a runtime model gateway provider.
type ModelGatewayConfig struct {
	Provider     string
	APIKey       string
	BaseURL      string
	DefaultModel string
	Options      map[string]string
	Timeout      time.Duration
	HTTPClient   *http.Client
	// AllowPrivate permits outbound gateway requests to trusted local/private
	// model endpoints, including loopback, RFC 1918 / ULA, and CGNAT
	// addresses. Only honored when HTTPClient is nil (otherwise the caller is
	// responsible for the supplied client's egress policy).
	AllowPrivate bool
}

// DefaultModelGatewayConfig returns conservative defaults that preserve existing behavior.
func DefaultModelGatewayConfig() ModelGatewayConfig {
	return ModelGatewayConfig{
		Provider: "mock",
		Options:  map[string]string{},
		Timeout:  30 * time.Second,
	}
}

// NewModelGatewayFromConfig returns a provider-backed model gateway.
func NewModelGatewayFromConfig(cfg ModelGatewayConfig) (ModelGateway, error) {
	return newModelGatewayFromConfigWithRegistry(cfg, DefaultModelProviderRegistry())
}

func newModelGatewayFromConfigWithRegistry(cfg ModelGatewayConfig, registry *ModelProviderRegistry) (ModelGateway, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = "mock"
	}
	cfg.Provider = provider
	if registry == nil {
		return nil, fmt.Errorf("%w: model provider registry is not configured", ErrModelGatewayConfiguration)
	}
	plugin, ok := registry.Lookup(provider)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported provider %q", ErrModelGatewayConfiguration, cfg.Provider)
	}
	gateway, err := plugin.BuildGateway(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrModelGatewayConfiguration, err)
	}
	return gateway, nil
}

type mockModelProviderPlugin struct{}

func (p *mockModelProviderPlugin) Name() string { return "mock" }

func (p *mockModelProviderPlugin) Aliases() []string { return nil }

func (p *mockModelProviderPlugin) RequiresAPIKey() bool { return false }

func (p *mockModelProviderPlugin) BuildGateway(_ ModelGatewayConfig) (ModelGateway, error) {
	return &MockModelGateway{}, nil
}

type openAIModelProviderPlugin struct{}

func (p *openAIModelProviderPlugin) Name() string { return "openai" }

func (p *openAIModelProviderPlugin) Aliases() []string { return nil }

func (p *openAIModelProviderPlugin) RequiresAPIKey() bool { return true }

func (p *openAIModelProviderPlugin) BuildGateway(cfg ModelGatewayConfig) (ModelGateway, error) {
	openaiCfg := DefaultOpenAIModelGatewayConfig()
	openaiCfg.RequireAPIKey = true
	if strings.TrimSpace(cfg.APIKey) != "" {
		openaiCfg.APIKey = strings.TrimSpace(cfg.APIKey)
	}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		openaiCfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	}
	if strings.TrimSpace(cfg.DefaultModel) != "" {
		openaiCfg.DefaultModel = strings.TrimSpace(cfg.DefaultModel)
	}
	if cfg.Timeout > 0 {
		openaiCfg.Timeout = cfg.Timeout
	}
	openaiCfg.HTTPClient = resolveGatewayHTTPClient(cfg)
	return NewOpenAIModelGateway(openaiCfg)
}

type openAICompatibleModelProviderPlugin struct{}

func (p *openAICompatibleModelProviderPlugin) Name() string { return "openai-compatible" }

func (p *openAICompatibleModelProviderPlugin) Aliases() []string {
	return []string{"openai_compatible"}
}

func (p *openAICompatibleModelProviderPlugin) RequiresAPIKey() bool { return false }

func (p *openAICompatibleModelProviderPlugin) BuildGateway(cfg ModelGatewayConfig) (ModelGateway, error) {
	openaiCfg := DefaultOpenAIModelGatewayConfig()
	openaiCfg.RequireAPIKey = false
	if strings.TrimSpace(cfg.APIKey) != "" {
		openaiCfg.APIKey = strings.TrimSpace(cfg.APIKey)
	}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		openaiCfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	}
	if strings.TrimSpace(cfg.DefaultModel) != "" {
		openaiCfg.DefaultModel = strings.TrimSpace(cfg.DefaultModel)
	}
	if cfg.Timeout > 0 {
		openaiCfg.Timeout = cfg.Timeout
	}
	openaiCfg.HTTPClient = resolveGatewayHTTPClient(cfg)
	return NewOpenAIModelGateway(openaiCfg)
}

type anthropicModelProviderPlugin struct{}

func (p *anthropicModelProviderPlugin) Name() string { return "anthropic" }

func (p *anthropicModelProviderPlugin) Aliases() []string { return nil }

func (p *anthropicModelProviderPlugin) RequiresAPIKey() bool { return true }

func (p *anthropicModelProviderPlugin) BuildGateway(cfg ModelGatewayConfig) (ModelGateway, error) {
	anthropicCfg := DefaultAnthropicModelGatewayConfig()
	if strings.TrimSpace(cfg.APIKey) != "" {
		anthropicCfg.APIKey = strings.TrimSpace(cfg.APIKey)
	}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		anthropicCfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	}
	if strings.TrimSpace(cfg.DefaultModel) != "" {
		anthropicCfg.DefaultModel = strings.TrimSpace(cfg.DefaultModel)
	}
	if cfg.Timeout > 0 {
		anthropicCfg.Timeout = cfg.Timeout
	}
	anthropicCfg.HTTPClient = resolveGatewayHTTPClient(cfg)

	options := normalizeModelProviderOptions(cfg.Options)
	if value, ok := options["anthropic_version"]; ok && strings.TrimSpace(value) != "" {
		anthropicCfg.AnthropicVersion = strings.TrimSpace(value)
	}
	if value, ok := options["max_tokens"]; ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid anthropic max_tokens %q", value)
		}
		anthropicCfg.MaxTokens = parsed
	}
	return NewAnthropicModelGateway(anthropicCfg)
}

type azureOpenAIModelProviderPlugin struct{}

func (p *azureOpenAIModelProviderPlugin) Name() string { return "azure-openai" }

func (p *azureOpenAIModelProviderPlugin) Aliases() []string {
	return []string{"azure_openai", "azure"}
}

func (p *azureOpenAIModelProviderPlugin) RequiresAPIKey() bool { return true }

func (p *azureOpenAIModelProviderPlugin) BuildGateway(cfg ModelGatewayConfig) (ModelGateway, error) {
	azureCfg := DefaultAzureOpenAIModelGatewayConfig()
	if strings.TrimSpace(cfg.APIKey) != "" {
		azureCfg.APIKey = strings.TrimSpace(cfg.APIKey)
	}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		azureCfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	}
	if strings.TrimSpace(cfg.DefaultModel) != "" {
		azureCfg.DefaultDeployment = strings.TrimSpace(cfg.DefaultModel)
	}
	if cfg.Timeout > 0 {
		azureCfg.Timeout = cfg.Timeout
	}
	azureCfg.HTTPClient = resolveGatewayHTTPClient(cfg)

	options := normalizeModelProviderOptions(cfg.Options)
	if value, ok := options["deployment"]; ok && strings.TrimSpace(value) != "" {
		azureCfg.DefaultDeployment = strings.TrimSpace(value)
	}
	if value, ok := options["api_version"]; ok && strings.TrimSpace(value) != "" {
		azureCfg.APIVersion = strings.TrimSpace(value)
	}
	return NewAzureOpenAIModelGateway(azureCfg)
}

type ollamaModelProviderPlugin struct{}

func (p *ollamaModelProviderPlugin) Name() string { return "ollama" }

func (p *ollamaModelProviderPlugin) Aliases() []string { return nil }

func (p *ollamaModelProviderPlugin) RequiresAPIKey() bool { return false }

func (p *ollamaModelProviderPlugin) BuildGateway(cfg ModelGatewayConfig) (ModelGateway, error) {
	ollamaCfg := DefaultOllamaModelGatewayConfig()
	if strings.TrimSpace(cfg.BaseURL) != "" {
		ollamaCfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	}
	if strings.TrimSpace(cfg.DefaultModel) != "" {
		ollamaCfg.DefaultModel = strings.TrimSpace(cfg.DefaultModel)
	}
	if cfg.Timeout > 0 {
		ollamaCfg.Timeout = cfg.Timeout
	}
	ollamaCfg.HTTPClient = resolveGatewayHTTPClient(cfg)

	options := normalizeModelProviderOptions(cfg.Options)
	if value, ok := options["base_url"]; ok && strings.TrimSpace(value) != "" {
		ollamaCfg.BaseURL = strings.TrimSpace(value)
	}
	if value, ok := options["default_model"]; ok && strings.TrimSpace(value) != "" {
		ollamaCfg.DefaultModel = strings.TrimSpace(value)
	}
	return NewOllamaModelGateway(ollamaCfg)
}

type bedrockModelProviderPlugin struct{}

func (p *bedrockModelProviderPlugin) Name() string { return "bedrock" }

func (p *bedrockModelProviderPlugin) Aliases() []string {
	return []string{"aws-bedrock", "aws_bedrock"}
}

func (p *bedrockModelProviderPlugin) RequiresAPIKey() bool { return false }

func (p *bedrockModelProviderPlugin) BuildGateway(cfg ModelGatewayConfig) (ModelGateway, error) {
	bedrockCfg := DefaultBedrockModelGatewayConfig()
	if strings.TrimSpace(cfg.DefaultModel) != "" {
		bedrockCfg.DefaultModel = strings.TrimSpace(cfg.DefaultModel)
	}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		bedrockCfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	}
	if cfg.Timeout > 0 {
		bedrockCfg.Timeout = cfg.Timeout
	}

	options := normalizeModelProviderOptions(cfg.Options)
	if value, ok := options["region"]; ok && strings.TrimSpace(value) != "" {
		bedrockCfg.Region = strings.TrimSpace(value)
	}
	if value, ok := options["max_tokens"]; ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid bedrock max_tokens %q", value)
		}
		bedrockCfg.MaxTokens = parsed
	}
	if value, ok := options["profile"]; ok && strings.TrimSpace(value) != "" {
		bedrockCfg.Profile = strings.TrimSpace(value)
	}

	if strings.TrimSpace(cfg.APIKey) != "" {
		var creds struct {
			AccessKeyID     string `json:"access_key_id"`
			SecretAccessKey string `json:"secret_access_key"`
			SessionToken    string `json:"session_token"`
		}
		if err := json.Unmarshal([]byte(cfg.APIKey), &creds); err == nil && creds.AccessKeyID != "" {
			bedrockCfg.AccessKeyID = creds.AccessKeyID
			bedrockCfg.SecretAccessKey = creds.SecretAccessKey
			bedrockCfg.SessionToken = creds.SessionToken
		}
	}

	return NewBedrockModelGateway(bedrockCfg)
}

// resolveGatewayHTTPClient returns cfg.HTTPClient if non-nil, otherwise a
// model-gateway safe HTTP client with dial-time SSRF enforcement configured
// from cfg.AllowPrivate and cfg.Timeout.
func resolveGatewayHTTPClient(cfg ModelGatewayConfig) *http.Client {
	if cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}
	return SafeModelGatewayHTTPClient(cfg.AllowPrivate, cfg.Timeout)
}

func normalizeModelProviderOptions(options map[string]string) map[string]string {
	if len(options) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(options))
	for key, value := range options {
		k := strings.ToLower(strings.TrimSpace(key))
		if k == "" {
			continue
		}
		out[k] = strings.TrimSpace(value)
	}
	return out
}
