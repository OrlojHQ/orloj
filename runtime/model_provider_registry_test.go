package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type testProviderPlugin struct {
	name           string
	aliases        []string
	requiresAPIKey bool
	build          func(cfg ModelGatewayConfig) (ModelGateway, error)
}

func (p *testProviderPlugin) Name() string { return p.name }

func (p *testProviderPlugin) Aliases() []string { return p.aliases }

func (p *testProviderPlugin) RequiresAPIKey() bool { return p.requiresAPIKey }

func (p *testProviderPlugin) BuildGateway(cfg ModelGatewayConfig) (ModelGateway, error) {
	if p.build != nil {
		return p.build(cfg)
	}
	return &MockModelGateway{}, nil
}

type testStaticGateway struct {
	content string
}

func (g *testStaticGateway) Complete(_ context.Context, req ModelRequest) (ModelResponse, error) {
	return ModelResponse{Content: strings.TrimSpace(g.content) + ":" + strings.TrimSpace(req.Model)}, nil
}

func TestDefaultModelProviderRegistryBuiltins(t *testing.T) {
	registry := DefaultModelProviderRegistry()
	if _, ok := registry.Lookup("mock"); !ok {
		t.Fatal("expected mock provider to be registered")
	}
	if _, ok := registry.Lookup("openai"); !ok {
		t.Fatal("expected openai provider to be registered")
	}
	if _, ok := registry.Lookup("openai-compatible"); !ok {
		t.Fatal("expected openai-compatible alias to be registered")
	}
	if _, ok := registry.Lookup("anthropic"); !ok {
		t.Fatal("expected anthropic provider to be registered")
	}
	if _, ok := registry.Lookup("azure-openai"); !ok {
		t.Fatal("expected azure-openai provider to be registered")
	}
	if _, ok := registry.Lookup("azure"); !ok {
		t.Fatal("expected azure alias to be registered")
	}
	if _, ok := registry.Lookup("ollama"); !ok {
		t.Fatal("expected ollama provider to be registered")
	}
}

func TestModelProviderRegistryRegisterRejectsDuplicate(t *testing.T) {
	registry := NewModelProviderRegistry()
	if err := registry.Register(&testProviderPlugin{name: "provider-a", aliases: []string{"alias-a"}}); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	err := registry.Register(&testProviderPlugin{name: "provider-b", aliases: []string{"alias-a"}})
	if err == nil {
		t.Fatal("expected duplicate alias registration to fail")
	}
}

func TestNewModelGatewayFromConfigWithRegistryUsesPlugin(t *testing.T) {
	registry := NewModelProviderRegistry()
	err := registry.Register(&testProviderPlugin{
		name: "custom-provider",
		build: func(cfg ModelGatewayConfig) (ModelGateway, error) {
			if strings.TrimSpace(cfg.DefaultModel) == "" {
				return nil, fmt.Errorf("default model is required")
			}
			return &testStaticGateway{content: "custom"}, nil
		},
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	gateway, err := newModelGatewayFromConfigWithRegistry(ModelGatewayConfig{
		Provider:     "custom-provider",
		DefaultModel: "model-a",
	}, registry)
	if err != nil {
		t.Fatalf("newModelGatewayFromConfigWithRegistry failed: %v", err)
	}

	resp, err := gateway.Complete(context.Background(), ModelRequest{Model: "model-a"})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if !strings.HasPrefix(resp.Content, "custom") {
		t.Fatalf("expected custom plugin gateway output, got %q", resp.Content)
	}
}
