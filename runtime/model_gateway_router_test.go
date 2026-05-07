package agentruntime

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

type stubModelEndpointLookup struct {
	items map[string]resources.ModelEndpoint
}

func (s *stubModelEndpointLookup) Get(_ context.Context, name string) (resources.ModelEndpoint, bool, error) {
	item, ok := s.items[name]
	return item, ok, nil
}

type stubSecretLookup struct {
	items map[string]resources.Secret
}

func (s *stubSecretLookup) Get(_ context.Context, name string) (resources.Secret, bool, error) {
	item, ok := s.items[name]
	return item, ok, nil
}

func TestModelRouterErrorsWithoutModelRef(t *testing.T) {
	router := NewModelRouter(ModelRouterConfig{})
	_, err := router.Complete(context.Background(), ModelRequest{Model: "gpt-test", Step: 1})
	if err == nil {
		t.Fatal("expected error when model_ref is empty")
	}
	if !strings.Contains(err.Error(), "model_ref") {
		t.Fatalf("expected model_ref in error, got %v", err)
	}
}

func TestModelRouterRoutesByModelRef(t *testing.T) {
	lookup := &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{
		"team-a/openai-team-a": {
			Metadata: resources.ObjectMeta{Name: "openai-team-a", Namespace: "team-a", ResourceVersion: "2"},
			Spec: resources.ModelEndpointSpec{
				Provider:     "mock",
				DefaultModel: "router-default",
			},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{
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
		Endpoints: &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{}},
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
	lookup := &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{
		"team-a/openai-team-a": {
			Metadata: resources.ObjectMeta{Name: "openai-team-a", Namespace: "team-a", ResourceVersion: "4"},
			Spec: resources.ModelEndpointSpec{
				Provider:     "openai",
				BaseURL:      "https://example.invalid/v1",
				DefaultModel: "gpt-test",
				Auth:         resources.ModelEndpointAuth{SecretRef: "openai-api-key"},
			},
		},
	}}
	secrets := &stubSecretLookup{items: map[string]resources.Secret{
		"team-a/openai-api-key": {
			Metadata: resources.ObjectMeta{Name: "openai-api-key", Namespace: "team-a"},
			Spec:     resources.SecretSpec{Data: map[string]string{"value": base64.StdEncoding.EncodeToString([]byte(secretValue))}},
		},
	}}

	router := NewModelRouter(ModelRouterConfig{
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
	lookup := &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{
		"team-a/openai-team-a": {
			Metadata: resources.ObjectMeta{Name: "openai-team-a", Namespace: "team-a", ResourceVersion: "5"},
			Spec: resources.ModelEndpointSpec{
				Provider:     "openai",
				BaseURL:      "https://example.invalid/v1",
				DefaultModel: "gpt-test",
			},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{
		Endpoints: lookup,
	})
	_, err := router.Complete(context.Background(), ModelRequest{Namespace: "team-a", ModelRef: "openai-team-a", Step: 1})
	if err == nil {
		t.Fatal("expected missing auth error")
	}
	if !strings.Contains(strings.ToLower(fmt.Sprint(err)), "secretref") {
		t.Fatalf("expected secretRef in error, got %v", err)
	}
}

func TestModelRouterAnthropicFailsWithoutKey(t *testing.T) {
	lookup := &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{
		"team-a/anthropic-team-a": {
			Metadata: resources.ObjectMeta{Name: "anthropic-team-a", Namespace: "team-a", ResourceVersion: "6"},
			Spec: resources.ModelEndpointSpec{
				Provider:     "anthropic",
				BaseURL:      "https://example.invalid/v1",
				DefaultModel: "claude-test",
			},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{
		Endpoints: lookup,
	})
	_, err := router.Complete(context.Background(), ModelRequest{Namespace: "team-a", ModelRef: "anthropic-team-a", Step: 1})
	if err == nil {
		t.Fatal("expected missing auth error")
	}
	if !strings.Contains(strings.ToLower(fmt.Sprint(err)), "secretref") {
		t.Fatalf("expected secretRef in error, got %v", err)
	}
}

func TestModelRouterAzureOpenAIFailsWithoutKey(t *testing.T) {
	lookup := &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{
		"team-a/azure-team-a": {
			Metadata: resources.ObjectMeta{Name: "azure-team-a", Namespace: "team-a", ResourceVersion: "9"},
			Spec: resources.ModelEndpointSpec{
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
		Endpoints: lookup,
	})
	_, err := router.Complete(context.Background(), ModelRequest{Namespace: "team-a", ModelRef: "azure-team-a", Step: 1})
	if err == nil {
		t.Fatal("expected missing auth error")
	}
	if !strings.Contains(strings.ToLower(fmt.Sprint(err)), "secretref") {
		t.Fatalf("expected secretRef in error, got %v", err)
	}
}

func TestModelRouterEndpointOptionsValidation(t *testing.T) {
	lookup := &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{
		"team-a/anthropic-team-a": {
			Metadata: resources.ObjectMeta{Name: "anthropic-team-a", Namespace: "team-a", ResourceVersion: "7"},
			Spec: resources.ModelEndpointSpec{
				Provider:     "anthropic",
				BaseURL:      "https://example.invalid/v1",
				DefaultModel: "claude-test",
				Options: map[string]string{
					"max_tokens": "invalid",
				},
				Auth: resources.ModelEndpointAuth{SecretRef: "anthropic-api-key"},
			},
		},
	}}
	secrets := &stubSecretLookup{items: map[string]resources.Secret{
		"team-a/anthropic-api-key": {
			Metadata: resources.ObjectMeta{Name: "anthropic-api-key", Namespace: "team-a"},
			Spec:     resources.SecretSpec{Data: map[string]string{"value": base64.StdEncoding.EncodeToString([]byte("test-key"))}},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{
		Endpoints: lookup,
		Secrets:   secrets,
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
	lookup := &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{
		"team-a/ollama-local": {
			Metadata: resources.ObjectMeta{Name: "ollama-local", Namespace: "team-a", ResourceVersion: "8"},
			Spec: resources.ModelEndpointSpec{
				Provider:     "ollama",
				BaseURL:      "http://127.0.0.1:11434",
				DefaultModel: "llama3.2",
			},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{
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

func TestModelRouterOpenAICompatibleDoesNotRequireAPIKey(t *testing.T) {
	lookup := &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{
		"team-a/compatible-local": {
			Metadata: resources.ObjectMeta{Name: "compatible-local", Namespace: "team-a", ResourceVersion: "10"},
			Spec: resources.ModelEndpointSpec{
				Provider:     "openai-compatible",
				BaseURL:      "http://127.0.0.1:11434/v1",
				DefaultModel: "llama3.2",
			},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{
		Endpoints: lookup,
	})
	_, err := router.gatewayForEndpoint(context.Background(), lookup.items["team-a/compatible-local"], "team-a/compatible-local")
	if err != nil {
		t.Fatalf("expected openai-compatible gateway build to succeed without key, got %v", err)
	}

	router.mu.RLock()
	cached := router.cache["team-a/compatible-local"]
	router.mu.RUnlock()
	if cached.Gateway == nil {
		t.Fatal("expected cached gateway for openai-compatible endpoint")
	}
	gw, ok := cached.Gateway.(*OpenAIModelGateway)
	if !ok {
		t.Fatalf("expected OpenAIModelGateway cache type, got %T", cached.Gateway)
	}
	if gw.apiKey != "" {
		t.Fatalf("expected empty api key for openai-compatible without auth, got %q", gw.apiKey)
	}
}

func TestModelRouterOpenAICompatibleUsesProvidedSecretWhenPresent(t *testing.T) {
	secretValue := "compat-secret"
	lookup := &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{
		"team-a/compatible-auth": {
			Metadata: resources.ObjectMeta{Name: "compatible-auth", Namespace: "team-a", ResourceVersion: "11"},
			Spec: resources.ModelEndpointSpec{
				Provider:     "openai-compatible",
				BaseURL:      "https://example.invalid/v1",
				DefaultModel: "llama3.2",
				Auth:         resources.ModelEndpointAuth{SecretRef: "compatible-key"},
			},
		},
	}}
	secrets := &stubSecretLookup{items: map[string]resources.Secret{
		"team-a/compatible-key": {
			Metadata: resources.ObjectMeta{Name: "compatible-key", Namespace: "team-a"},
			Spec:     resources.SecretSpec{Data: map[string]string{"value": base64.StdEncoding.EncodeToString([]byte(secretValue))}},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{
		Endpoints: lookup,
		Secrets:   secrets,
	})
	_, err := router.gatewayForEndpoint(context.Background(), lookup.items["team-a/compatible-auth"], "team-a/compatible-auth")
	if err != nil {
		t.Fatalf("expected openai-compatible gateway build with secret to succeed, got %v", err)
	}

	router.mu.RLock()
	cached := router.cache["team-a/compatible-auth"]
	router.mu.RUnlock()
	gw, ok := cached.Gateway.(*OpenAIModelGateway)
	if !ok {
		t.Fatalf("expected OpenAIModelGateway cache type, got %T", cached.Gateway)
	}
	if gw.apiKey != secretValue {
		t.Fatalf("expected resolved api key %q, got %q", secretValue, gw.apiKey)
	}
}

func TestModelRouterOllamaDefaultAllowsLoopbackEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"local ollama ok"},"done":true}`))
	}))
	defer server.Close()

	lookup := &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{
		"team-a/ollama-local": {
			Metadata: resources.ObjectMeta{Name: "ollama-local", Namespace: "team-a", ResourceVersion: "12"},
			Spec: resources.ModelEndpointSpec{
				Provider:     "ollama",
				BaseURL:      server.URL,
				DefaultModel: "llama3.2",
			},
		},
	}}
	router := NewModelRouter(ModelRouterConfig{Endpoints: lookup})

	resp, err := router.Complete(context.Background(), ModelRequest{
		Namespace: "team-a",
		ModelRef:  "ollama-local",
		Step:      1,
	})
	if err != nil {
		t.Fatalf("expected ollama loopback endpoint to work by default, got %v", err)
	}
	if resp.Content != "local ollama ok" {
		t.Fatalf("unexpected response content %q", resp.Content)
	}
}

func TestModelRouterOpenAICompatibleLoopbackRequiresAllowPrivate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"local compatible ok"}}]}`))
	}))
	defer server.Close()

	baseEndpoint := resources.ModelEndpoint{
		Metadata: resources.ObjectMeta{Name: "compatible-local", Namespace: "team-a", ResourceVersion: "13"},
		Spec: resources.ModelEndpointSpec{
			Provider:     "openai-compatible",
			BaseURL:      server.URL,
			DefaultModel: "llama3.2",
		},
	}
	lookup := &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{
		"team-a/compatible-local": baseEndpoint,
	}}
	router := NewModelRouter(ModelRouterConfig{Endpoints: lookup})
	_, err := router.Complete(context.Background(), ModelRequest{
		Namespace: "team-a",
		ModelRef:  "compatible-local",
		Step:      1,
	})
	if err == nil {
		t.Fatal("expected openai-compatible loopback endpoint to fail without allowPrivate=true")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "loopback") {
		t.Fatalf("expected loopback block error, got %v", err)
	}

	allowPrivate := true
	allowedEndpoint := baseEndpoint
	allowedEndpoint.Metadata.ResourceVersion = "14"
	allowedEndpoint.Spec.AllowPrivate = &allowPrivate
	allowedLookup := &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{
		"team-a/compatible-local": allowedEndpoint,
	}}
	allowedRouter := NewModelRouter(ModelRouterConfig{Endpoints: allowedLookup})
	resp, err := allowedRouter.Complete(context.Background(), ModelRequest{
		Namespace: "team-a",
		ModelRef:  "compatible-local",
		Step:      1,
	})
	if err != nil {
		t.Fatalf("expected openai-compatible loopback endpoint with allowPrivate=true to work, got %v", err)
	}
	if resp.Content != "local compatible ok" {
		t.Fatalf("unexpected response content %q", resp.Content)
	}
}

func TestModelRouterCacheInvalidatesOnSecretChange(t *testing.T) {
	oldKey := "sk-old-key"
	newKey := "sk-new-key"
	secretName := "openai-api-key"
	endpointKey := "team-a/openai-team-a"

	secrets := &stubSecretLookup{items: map[string]resources.Secret{
		"team-a/" + secretName: {
			Metadata: resources.ObjectMeta{Name: secretName, Namespace: "team-a", ResourceVersion: "1"},
			Spec:     resources.SecretSpec{Data: map[string]string{"value": base64.StdEncoding.EncodeToString([]byte(oldKey))}},
		},
	}}
	lookup := &stubModelEndpointLookup{items: map[string]resources.ModelEndpoint{
		endpointKey: {
			Metadata: resources.ObjectMeta{Name: "openai-team-a", Namespace: "team-a", ResourceVersion: "10"},
			Spec: resources.ModelEndpointSpec{
				Provider:     "openai",
				BaseURL:      "https://example.invalid/v1",
				DefaultModel: "gpt-test",
				Auth:         resources.ModelEndpointAuth{SecretRef: secretName},
			},
		},
	}}

	router := NewModelRouter(ModelRouterConfig{
		Endpoints: lookup,
		Secrets:   secrets,
	})

	gw1, err := router.gatewayForEndpoint(context.Background(), lookup.items[endpointKey], endpointKey)
	if err != nil {
		t.Fatalf("first gatewayForEndpoint failed: %v", err)
	}
	oaiGW1, ok := gw1.(*OpenAIModelGateway)
	if !ok {
		t.Fatalf("expected OpenAIModelGateway, got %T", gw1)
	}
	if oaiGW1.apiKey != oldKey {
		t.Fatalf("expected apiKey=%q, got %q", oldKey, oaiGW1.apiKey)
	}

	// Call again with same endpoint and same secret -- should return cached gateway.
	gw2, err := router.gatewayForEndpoint(context.Background(), lookup.items[endpointKey], endpointKey)
	if err != nil {
		t.Fatalf("second gatewayForEndpoint failed: %v", err)
	}
	if gw2 != gw1 {
		t.Fatal("expected same cached gateway instance when nothing changed")
	}

	// Rotate the secret: update the value and bump its ResourceVersion.
	// The ModelEndpoint ResourceVersion stays the same -- this is the bug scenario.
	secrets.items["team-a/"+secretName] = resources.Secret{
		Metadata: resources.ObjectMeta{Name: secretName, Namespace: "team-a", ResourceVersion: "2"},
		Spec:     resources.SecretSpec{Data: map[string]string{"value": base64.StdEncoding.EncodeToString([]byte(newKey))}},
	}

	gw3, err := router.gatewayForEndpoint(context.Background(), lookup.items[endpointKey], endpointKey)
	if err != nil {
		t.Fatalf("third gatewayForEndpoint (after secret rotation) failed: %v", err)
	}
	if gw3 == gw1 {
		t.Fatal("expected new gateway instance after secret rotation, got same cached object")
	}
	oaiGW3, ok := gw3.(*OpenAIModelGateway)
	if !ok {
		t.Fatalf("expected OpenAIModelGateway, got %T", gw3)
	}
	if oaiGW3.apiKey != newKey {
		t.Fatalf("expected rotated apiKey=%q, got %q", newKey, oaiGW3.apiKey)
	}
}

// --- Fallback routing test helpers ---

type fallbackFailingGateway struct {
	err error
}

func (g *fallbackFailingGateway) Complete(_ context.Context, _ ModelRequest) (ModelResponse, error) {
	return ModelResponse{}, g.err
}

type countingModelGateway struct {
	calls   int
	wrapped ModelGateway
}

func (g *countingModelGateway) Complete(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	g.calls++
	return g.wrapped.Complete(ctx, req)
}

func newFallbackRouter(endpoints map[string]resources.ModelEndpoint) *ModelRouter {
	return NewModelRouter(ModelRouterConfig{
		Endpoints: &stubModelEndpointLookup{items: endpoints},
	})
}

func injectGateway(router *ModelRouter, key string, gw ModelGateway, rv string) {
	router.mu.Lock()
	router.cache[key] = cachedModelGateway{ResourceVersion: rv, Gateway: gw}
	router.mu.Unlock()
}

// --- Fallback routing tests ---

func TestModelRouterFallbackOnRetryableError(t *testing.T) {
	endpoints := map[string]resources.ModelEndpoint{
		"default/primary": {
			Metadata: resources.ObjectMeta{Name: "primary", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "primary-model"},
		},
		"default/fallback": {
			Metadata: resources.ObjectMeta{Name: "fallback", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "fallback-model"},
		},
	}
	router := newFallbackRouter(endpoints)
	injectGateway(router, "default/primary", &fallbackFailingGateway{
		err: &ModelGatewayError{StatusCode: 500, Provider: "mock", Message: "server error"},
	}, "1")

	resp, err := router.Complete(context.Background(), ModelRequest{
		Namespace:         "default",
		ModelRef:          "primary",
		FallbackModelRefs: []string{"fallback"},
		Step:              1,
	})
	if err != nil {
		t.Fatalf("expected fallback success, got error: %v", err)
	}
	if !strings.Contains(resp.Content, "fallback-model") {
		t.Fatalf("expected fallback-model in response, got %q", resp.Content)
	}
}

func TestModelRouterNoFallbackOnNonRetryableError(t *testing.T) {
	endpoints := map[string]resources.ModelEndpoint{
		"default/primary": {
			Metadata: resources.ObjectMeta{Name: "primary", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "primary-model"},
		},
		"default/fallback": {
			Metadata: resources.ObjectMeta{Name: "fallback", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "fallback-model"},
		},
	}
	router := newFallbackRouter(endpoints)
	injectGateway(router, "default/primary", &fallbackFailingGateway{
		err: &ModelGatewayError{StatusCode: 401, Provider: "mock", Message: "unauthorized"},
	}, "1")

	fallbackGW := &countingModelGateway{wrapped: &MockModelGateway{}}
	injectGateway(router, "default/fallback", fallbackGW, "1")

	_, err := router.Complete(context.Background(), ModelRequest{
		Namespace:         "default",
		ModelRef:          "primary",
		FallbackModelRefs: []string{"fallback"},
		Step:              1,
	})
	if err == nil {
		t.Fatal("expected 401 error, got nil")
	}
	mge, retryable := IsModelGatewayError(err)
	if mge == nil || retryable {
		t.Fatalf("expected non-retryable ModelGatewayError, got retryable=%v err=%v", retryable, err)
	}
	if fallbackGW.calls != 0 {
		t.Fatalf("expected fallback gateway to not be called, got %d calls", fallbackGW.calls)
	}
}

func TestModelRouterFallbackOn429(t *testing.T) {
	endpoints := map[string]resources.ModelEndpoint{
		"default/primary": {
			Metadata: resources.ObjectMeta{Name: "primary", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "primary-model"},
		},
		"default/fallback": {
			Metadata: resources.ObjectMeta{Name: "fallback", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "fallback-model"},
		},
	}
	router := newFallbackRouter(endpoints)
	injectGateway(router, "default/primary", &fallbackFailingGateway{
		err: &ModelGatewayError{StatusCode: 429, Provider: "mock", Message: "rate limited"},
	}, "1")

	resp, err := router.Complete(context.Background(), ModelRequest{
		Namespace:         "default",
		ModelRef:          "primary",
		FallbackModelRefs: []string{"fallback"},
		Step:              1,
	})
	if err != nil {
		t.Fatalf("expected fallback success on 429, got error: %v", err)
	}
	if !strings.Contains(resp.Content, "fallback-model") {
		t.Fatalf("expected fallback-model in response, got %q", resp.Content)
	}
}

func TestModelRouterAllEndpointsExhausted(t *testing.T) {
	endpoints := map[string]resources.ModelEndpoint{
		"default/primary": {
			Metadata: resources.ObjectMeta{Name: "primary", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "primary-model"},
		},
		"default/fallback1": {
			Metadata: resources.ObjectMeta{Name: "fallback1", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "fb1-model"},
		},
		"default/fallback2": {
			Metadata: resources.ObjectMeta{Name: "fallback2", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "fb2-model"},
		},
	}
	router := newFallbackRouter(endpoints)
	injectGateway(router, "default/primary", &fallbackFailingGateway{
		err: &ModelGatewayError{StatusCode: 500, Provider: "mock", Message: "primary down"},
	}, "1")
	injectGateway(router, "default/fallback1", &fallbackFailingGateway{
		err: &ModelGatewayError{StatusCode: 502, Provider: "mock", Message: "fallback1 down"},
	}, "1")
	injectGateway(router, "default/fallback2", &fallbackFailingGateway{
		err: &ModelGatewayError{StatusCode: 503, Provider: "mock", Message: "fallback2 down"},
	}, "1")

	_, err := router.Complete(context.Background(), ModelRequest{
		Namespace:         "default",
		ModelRef:          "primary",
		FallbackModelRefs: []string{"fallback1", "fallback2"},
		Step:              1,
	})
	if err == nil {
		t.Fatal("expected error when all endpoints exhausted")
	}
	if !strings.Contains(err.Error(), "fallback2 down") {
		t.Fatalf("expected last error from fallback2, got %v", err)
	}
}

func TestModelRouterFallbackSkippedOnContextCancel(t *testing.T) {
	endpoints := map[string]resources.ModelEndpoint{
		"default/primary": {
			Metadata: resources.ObjectMeta{Name: "primary", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "primary-model"},
		},
		"default/fallback": {
			Metadata: resources.ObjectMeta{Name: "fallback", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "fallback-model"},
		},
	}
	router := newFallbackRouter(endpoints)

	ctx, cancel := context.WithCancel(context.Background())
	primaryGW := &fallbackFailingGateway{
		err: &ModelGatewayError{StatusCode: 500, Provider: "mock", Message: "server error"},
	}
	injectGateway(router, "default/primary", primaryGW, "1")

	fallbackGW := &countingModelGateway{wrapped: &MockModelGateway{}}
	injectGateway(router, "default/fallback", fallbackGW, "1")

	// Cancel the context before calling Complete so the fallback is skipped.
	cancel()

	_, err := router.Complete(ctx, ModelRequest{
		Namespace:         "default",
		ModelRef:          "primary",
		FallbackModelRefs: []string{"fallback"},
		Step:              1,
	})
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
	if fallbackGW.calls != 0 {
		t.Fatalf("expected fallback gateway to not be called with cancelled context, got %d calls", fallbackGW.calls)
	}
}

func TestModelRouterFallbackEndpointNotFound(t *testing.T) {
	endpoints := map[string]resources.ModelEndpoint{
		"default/primary": {
			Metadata: resources.ObjectMeta{Name: "primary", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "primary-model"},
		},
		"default/fallback2": {
			Metadata: resources.ObjectMeta{Name: "fallback2", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "fb2-model"},
		},
	}
	router := newFallbackRouter(endpoints)
	injectGateway(router, "default/primary", &fallbackFailingGateway{
		err: &ModelGatewayError{StatusCode: 500, Provider: "mock", Message: "primary down"},
	}, "1")

	resp, err := router.Complete(context.Background(), ModelRequest{
		Namespace:         "default",
		ModelRef:          "primary",
		FallbackModelRefs: []string{"missing-endpoint", "fallback2"},
		Step:              1,
	})
	if err != nil {
		t.Fatalf("expected second fallback to succeed, got error: %v", err)
	}
	if !strings.Contains(resp.Content, "fb2-model") {
		t.Fatalf("expected fb2-model in response, got %q", resp.Content)
	}
}

func TestModelRouterNoFallbackRefsUsesOriginalBehavior(t *testing.T) {
	endpoints := map[string]resources.ModelEndpoint{
		"default/primary": {
			Metadata: resources.ObjectMeta{Name: "primary", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "primary-model"},
		},
	}
	router := newFallbackRouter(endpoints)
	injectGateway(router, "default/primary", &fallbackFailingGateway{
		err: &ModelGatewayError{StatusCode: 500, Provider: "mock", Message: "server error"},
	}, "1")

	_, err := router.Complete(context.Background(), ModelRequest{
		Namespace: "default",
		ModelRef:  "primary",
		Step:      1,
	})
	if err == nil {
		t.Fatal("expected error with no fallback refs")
	}
	if !strings.Contains(err.Error(), "server error") {
		t.Fatalf("expected original server error, got %v", err)
	}
}
