package agentruntime

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/AnonJon/orloj/crds"
)

type staticModelGateway struct {
	content string
}

func (s *staticModelGateway) Complete(_ context.Context, req ModelRequest) (ModelResponse, error) {
	content := s.content
	if content == "" {
		content = "fallback"
	}
	return ModelResponse{Content: content + " model=" + strings.TrimSpace(req.Model), Done: false}, nil
}

type stubModelEndpointLookup struct {
	items map[string]crds.ModelEndpoint
}

func (s *stubModelEndpointLookup) Get(name string) (crds.ModelEndpoint, bool) {
	item, ok := s.items[name]
	return item, ok
}

type stubSecretLookup struct {
	items map[string]crds.Secret
}

func (s *stubSecretLookup) Get(name string) (crds.Secret, bool) {
	item, ok := s.items[name]
	return item, ok
}

func TestModelRouterUsesFallbackWithoutModelRef(t *testing.T) {
	router := NewModelRouter(ModelRouterConfig{
		Fallback: &staticModelGateway{content: "fallback"},
	})
	resp, err := router.Complete(context.Background(), ModelRequest{Model: "gpt-test", Step: 1})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if !strings.Contains(resp.Content, "fallback") {
		t.Fatalf("expected fallback content, got %q", resp.Content)
	}
}

func TestModelRouterRoutesByModelRef(t *testing.T) {
	lookup := &stubModelEndpointLookup{items: map[string]crds.ModelEndpoint{
		"team-a/openai-team-a": {
			Metadata: crds.ObjectMeta{Name: "openai-team-a", Namespace: "team-a", ResourceVersion: "2"},
			Spec: crds.ModelEndpointSpec{
				Provider:     "mock",
				DefaultModel: "router-default",
			},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{
		Fallback:  &staticModelGateway{content: "fallback"},
		Endpoints: lookup,
	})
	resp, err := router.Complete(context.Background(), ModelRequest{
		Namespace: "team-a",
		ModelRef:  "openai-team-a",
		Step:      4,
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if strings.Contains(resp.Content, "fallback") {
		t.Fatalf("expected routed endpoint gateway, got fallback response %q", resp.Content)
	}
	if !strings.Contains(resp.Content, "router-default") {
		t.Fatalf("expected routed default model in response, got %q", resp.Content)
	}
}

func TestModelRouterErrorsWhenEndpointMissing(t *testing.T) {
	router := NewModelRouter(ModelRouterConfig{
		Fallback:  &staticModelGateway{content: "fallback"},
		Endpoints: &stubModelEndpointLookup{items: map[string]crds.ModelEndpoint{}},
	})
	_, err := router.Complete(context.Background(), ModelRequest{
		Namespace: "team-a",
		ModelRef:  "missing-endpoint",
		Step:      1,
	})
	if err == nil {
		t.Fatal("expected missing endpoint error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("expected not found in error, got %v", err)
	}
}

func TestModelRouterResolvesEndpointSecret(t *testing.T) {
	secretValue := "sk-test-value"
	lookup := &stubModelEndpointLookup{items: map[string]crds.ModelEndpoint{
		"team-a/openai-team-a": {
			Metadata: crds.ObjectMeta{Name: "openai-team-a", Namespace: "team-a", ResourceVersion: "4"},
			Spec: crds.ModelEndpointSpec{
				Provider:     "openai",
				BaseURL:      "https://example.invalid/v1",
				DefaultModel: "gpt-test",
				Auth:         crds.ModelEndpointAuth{SecretRef: "openai-api-key"},
			},
		},
	}}
	secrets := &stubSecretLookup{items: map[string]crds.Secret{
		"team-a/openai-api-key": {
			Metadata: crds.ObjectMeta{Name: "openai-api-key", Namespace: "team-a"},
			Spec:     crds.SecretSpec{Data: map[string]string{"value": base64.StdEncoding.EncodeToString([]byte(secretValue))}},
		},
	}}

	router := NewModelRouter(ModelRouterConfig{
		Fallback:  &staticModelGateway{content: "fallback"},
		Endpoints: lookup,
		Secrets:   secrets,
	})

	_, err := router.gatewayForEndpoint(context.Background(), lookup.items["team-a/openai-team-a"], "team-a/openai-team-a")
	if err != nil {
		t.Fatalf("gatewayForEndpoint failed: %v", err)
	}

	router.mu.RLock()
	cached := router.cache["team-a/openai-team-a"]
	router.mu.RUnlock()
	if cached.Gateway == nil {
		t.Fatal("expected cached gateway")
	}
	_, isOpenAI := cached.Gateway.(*OpenAIModelGateway)
	if !isOpenAI {
		t.Fatalf("expected OpenAIModelGateway cache type, got %T", cached.Gateway)
	}
}

func TestParseModelEndpointRef(t *testing.T) {
	ns, name := parseModelEndpointRef("team-a", "shared")
	if ns != "team-a" || name != "shared" {
		t.Fatalf("unexpected parse result ns=%s name=%s", ns, name)
	}
	ns, name = parseModelEndpointRef("team-a", "ops/global")
	if ns != "ops" || name != "global" {
		t.Fatalf("unexpected explicit namespace parse ns=%s name=%s", ns, name)
	}
}

func TestModelRouterOpenAIFailsWithoutKey(t *testing.T) {
	lookup := &stubModelEndpointLookup{items: map[string]crds.ModelEndpoint{
		"team-a/openai-team-a": {
			Metadata: crds.ObjectMeta{Name: "openai-team-a", Namespace: "team-a", ResourceVersion: "5"},
			Spec: crds.ModelEndpointSpec{
				Provider:     "openai",
				BaseURL:      "https://example.invalid/v1",
				DefaultModel: "gpt-test",
			},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{
		Fallback:  &staticModelGateway{content: "fallback"},
		Endpoints: lookup,
	})
	_, err := router.Complete(context.Background(), ModelRequest{Namespace: "team-a", ModelRef: "openai-team-a", Step: 1})
	if err == nil {
		t.Fatal("expected missing key error")
	}
	if !strings.Contains(strings.ToLower(fmt.Sprint(err)), "key") {
		t.Fatalf("expected key error, got %v", err)
	}
}

func TestModelRouterAnthropicFailsWithoutKey(t *testing.T) {
	lookup := &stubModelEndpointLookup{items: map[string]crds.ModelEndpoint{
		"team-a/anthropic-team-a": {
			Metadata: crds.ObjectMeta{Name: "anthropic-team-a", Namespace: "team-a", ResourceVersion: "6"},
			Spec: crds.ModelEndpointSpec{
				Provider:     "anthropic",
				BaseURL:      "https://example.invalid/v1",
				DefaultModel: "claude-test",
			},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{
		Fallback:  &staticModelGateway{content: "fallback"},
		Endpoints: lookup,
	})
	_, err := router.Complete(context.Background(), ModelRequest{Namespace: "team-a", ModelRef: "anthropic-team-a", Step: 1})
	if err == nil {
		t.Fatal("expected missing key error")
	}
	if !strings.Contains(strings.ToLower(fmt.Sprint(err)), "key") {
		t.Fatalf("expected key error, got %v", err)
	}
}

func TestModelRouterAzureOpenAIFailsWithoutKey(t *testing.T) {
	lookup := &stubModelEndpointLookup{items: map[string]crds.ModelEndpoint{
		"team-a/azure-team-a": {
			Metadata: crds.ObjectMeta{Name: "azure-team-a", Namespace: "team-a", ResourceVersion: "9"},
			Spec: crds.ModelEndpointSpec{
				Provider:     "azure-openai",
				BaseURL:      "https://example.openai.azure.com",
				DefaultModel: "deployment-a",
				Options: map[string]string{
					"api_version": "2024-10-21",
				},
			},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{
		Fallback:  &staticModelGateway{content: "fallback"},
		Endpoints: lookup,
	})
	_, err := router.Complete(context.Background(), ModelRequest{Namespace: "team-a", ModelRef: "azure-team-a", Step: 1})
	if err == nil {
		t.Fatal("expected missing key error")
	}
	if !strings.Contains(strings.ToLower(fmt.Sprint(err)), "key") {
		t.Fatalf("expected key error, got %v", err)
	}
}

func TestModelRouterEndpointOptionsValidation(t *testing.T) {
	lookup := &stubModelEndpointLookup{items: map[string]crds.ModelEndpoint{
		"team-a/anthropic-team-a": {
			Metadata: crds.ObjectMeta{Name: "anthropic-team-a", Namespace: "team-a", ResourceVersion: "7"},
			Spec: crds.ModelEndpointSpec{
				Provider:     "anthropic",
				BaseURL:      "https://example.invalid/v1",
				DefaultModel: "claude-test",
				Options: map[string]string{
					"max_tokens": "invalid",
				},
			},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{
		Fallback:       &staticModelGateway{content: "fallback"},
		Endpoints:      lookup,
		FallbackAPIKey: "test-key",
	})
	_, err := router.Complete(context.Background(), ModelRequest{Namespace: "team-a", ModelRef: "anthropic-team-a", Step: 1})
	if err == nil {
		t.Fatal("expected endpoint options validation error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "max_tokens") {
		t.Fatalf("expected max_tokens in error, got %v", err)
	}
}

func TestModelRouterOllamaDoesNotRequireAPIKey(t *testing.T) {
	lookup := &stubModelEndpointLookup{items: map[string]crds.ModelEndpoint{
		"team-a/ollama-local": {
			Metadata: crds.ObjectMeta{Name: "ollama-local", Namespace: "team-a", ResourceVersion: "8"},
			Spec: crds.ModelEndpointSpec{
				Provider:     "ollama",
				BaseURL:      "http://127.0.0.1:11434",
				DefaultModel: "llama3.2",
			},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{
		Fallback:  &staticModelGateway{content: "fallback"},
		Endpoints: lookup,
	})
	_, err := router.gatewayForEndpoint(context.Background(), lookup.items["team-a/ollama-local"], "team-a/ollama-local")
	if err != nil {
		t.Fatalf("expected ollama gateway build to succeed without key, got %v", err)
	}

	router.mu.RLock()
	cached := router.cache["team-a/ollama-local"]
	router.mu.RUnlock()
	if cached.Gateway == nil {
		t.Fatal("expected cached gateway for ollama endpoint")
	}
	if _, ok := cached.Gateway.(*OllamaModelGateway); !ok {
		t.Fatalf("expected OllamaModelGateway cache type, got %T", cached.Gateway)
	}
}
