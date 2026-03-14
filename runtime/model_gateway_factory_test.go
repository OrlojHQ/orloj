package agentruntime

import (
	"errors"
	"testing"
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

func TestNewModelGatewayFromConfigOpenAICompatibleAlias(t *testing.T) {
	_, err := NewModelGatewayFromConfig(ModelGatewayConfig{
		Provider: "openai-compatible",
		APIKey:   "test-key",
		BaseURL:  "https://example.invalid/v1",
	})
	if err != nil {
		t.Fatalf("expected alias provider to be accepted, got %v", err)
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
