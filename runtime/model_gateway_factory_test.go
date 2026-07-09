package agentruntime

import (
	"errors"
	"testing"
	"time"
)

func TestNewModelGatewayFromConfigDefaultsToMock(t *testing.T) {
	gateway, err := NewModelGatewayFromConfig(ModelGatewayConfig{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := gateway.(*MockModelGateway); !ok {
		t.Fatalf("expected *MockModelGateway, got %T", gateway)
	}
}

func TestNewModelGatewayFromConfigOpenAIMissingKey(t *testing.T) {
	_, err := NewModelGatewayFromConfig(ModelGatewayConfig{Provider: "openai"})
	if err == nil {
		t.Fatal("expected configuration error")
	}
	if !errors.Is(err, ErrModelGatewayConfiguration) {
		t.Fatalf("expected ErrModelGatewayConfiguration, got %v", err)
	}
}

func TestNewModelGatewayFromConfigOpenAICompatibleNoKey(t *testing.T) {
	_, err := NewModelGatewayFromConfig(ModelGatewayConfig{
		Provider: "openai-compatible",
		BaseURL:  "https://example.invalid/v1",
	})
	if err != nil {
		t.Fatalf("expected openai-compatible provider without key to be accepted, got %v", err)
	}
}

func TestNewModelGatewayFromConfigOpenAICompatibleUnderscoreAlias(t *testing.T) {
	_, err := NewModelGatewayFromConfig(ModelGatewayConfig{
		Provider: "openai_compatible",
		BaseURL:  "https://example.invalid/v1",
	})
	if err != nil {
		t.Fatalf("expected openai_compatible alias provider to be accepted, got %v", err)
	}
}

func TestNewModelGatewayFromConfigAnthropicMissingKey(t *testing.T) {
	_, err := NewModelGatewayFromConfig(ModelGatewayConfig{Provider: "anthropic"})
	if err == nil {
		t.Fatal("expected configuration error")
	}
	if !errors.Is(err, ErrModelGatewayConfiguration) {
		t.Fatalf("expected ErrModelGatewayConfiguration, got %v", err)
	}
}

func TestNewModelGatewayFromConfigAnthropicInvalidMaxTokens(t *testing.T) {
	_, err := NewModelGatewayFromConfig(ModelGatewayConfig{
		Provider: "anthropic",
		APIKey:   "test-key",
		BaseURL:  "https://example.invalid/v1",
		Options: map[string]string{
			"max_tokens": "not-a-number",
		},
	})
	if err == nil {
		t.Fatal("expected invalid max_tokens error")
	}
	if !errors.Is(err, ErrModelGatewayConfiguration) {
		t.Fatalf("expected ErrModelGatewayConfiguration, got %v", err)
	}
}

func TestNewModelGatewayFromConfigAzureOpenAIMissingKey(t *testing.T) {
	_, err := NewModelGatewayFromConfig(ModelGatewayConfig{
		Provider: "azure-openai",
		BaseURL:  "https://example.openai.azure.com",
	})
	if err == nil {
		t.Fatal("expected configuration error")
	}
	if !errors.Is(err, ErrModelGatewayConfiguration) {
		t.Fatalf("expected ErrModelGatewayConfiguration, got %v", err)
	}
}

func TestNewModelGatewayFromConfigOllama(t *testing.T) {
	gateway, err := NewModelGatewayFromConfig(ModelGatewayConfig{
		Provider:     "ollama",
		BaseURL:      "http://127.0.0.1:11434",
		DefaultModel: "llama3.2",
	})
	if err != nil {
		t.Fatalf("expected ollama gateway config to succeed, got %v", err)
	}
	if _, ok := gateway.(*OllamaModelGateway); !ok {
		t.Fatalf("expected *OllamaModelGateway, got %T", gateway)
	}
}

func TestNewModelGatewayFromConfigUnsupportedProvider(t *testing.T) {
	_, err := NewModelGatewayFromConfig(ModelGatewayConfig{Provider: "unknown"})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
	if !errors.Is(err, ErrModelGatewayConfiguration) {
		t.Fatalf("expected ErrModelGatewayConfiguration, got %v", err)
	}
}

func TestAnthropicProviderKeepsDefaultTimeoutOverRouterDefault(t *testing.T) {
	cfg := DefaultModelGatewayConfig()
	cfg.Provider = "anthropic"
	cfg.APIKey = "test-key"
	// Router default is 30s; Anthropic should keep its 120s default for tool-heavy turns.
	if cfg.Timeout != 30*time.Second {
		t.Fatalf("expected router default timeout 30s, got %s", cfg.Timeout)
	}

	gateway, err := NewModelGatewayFromConfig(cfg)
	if err != nil {
		t.Fatalf("expected anthropic gateway, got %v", err)
	}
	ag, ok := gateway.(*AnthropicModelGateway)
	if !ok {
		t.Fatalf("expected *AnthropicModelGateway, got %T", gateway)
	}
	if ag.client == nil {
		t.Fatal("expected HTTP client")
	}
	if ag.client.Timeout != 120*time.Second {
		t.Fatalf("expected anthropic HTTP timeout 120s, got %s", ag.client.Timeout)
	}
}

func TestAnthropicProviderHonorsLongerExplicitTimeout(t *testing.T) {
	gateway, err := NewModelGatewayFromConfig(ModelGatewayConfig{
		Provider: "anthropic",
		APIKey:   "test-key",
		Timeout:  5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("expected anthropic gateway, got %v", err)
	}
	ag := gateway.(*AnthropicModelGateway)
	if ag.client.Timeout != 5*time.Minute {
		t.Fatalf("expected explicit timeout 5m, got %s", ag.client.Timeout)
	}
}
