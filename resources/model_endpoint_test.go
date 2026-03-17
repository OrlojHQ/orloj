package resources

import "testing"

func TestModelEndpointNormalizeDefaults(t *testing.T) {
	endpoint := ModelEndpoint{
		Kind:     "ModelEndpoint",
		Metadata: ObjectMeta{Name: "openai-prod"},
		Spec:     ModelEndpointSpec{},
	}
	if err := endpoint.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if endpoint.Metadata.Namespace != DefaultNamespace {
		t.Fatalf("expected default namespace, got %q", endpoint.Metadata.Namespace)
	}
	if endpoint.Spec.Provider != "openai" {
		t.Fatalf("expected provider openai, got %q", endpoint.Spec.Provider)
	}
	if endpoint.Spec.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected default base URL %q", endpoint.Spec.BaseURL)
	}
}

func TestModelEndpointNormalizeAllowsCustomProvider(t *testing.T) {
	endpoint := ModelEndpoint{
		Kind:     "ModelEndpoint",
		Metadata: ObjectMeta{Name: "bad"},
		Spec: ModelEndpointSpec{
			Provider: "custom-llm",
		},
	}
	if err := endpoint.Normalize(); err != nil {
		t.Fatalf("expected custom provider to normalize, got %v", err)
	}
	if endpoint.Spec.Provider != "custom-llm" {
		t.Fatalf("expected normalized provider custom-llm, got %q", endpoint.Spec.Provider)
	}
}

func TestModelEndpointNormalizeAnthropicDefaults(t *testing.T) {
	endpoint := ModelEndpoint{
		Kind:     "ModelEndpoint",
		Metadata: ObjectMeta{Name: "anthropic-prod"},
		Spec: ModelEndpointSpec{
			Provider: "Anthropic",
			Options: map[string]string{
				" Anthropic_Version ": " 2023-06-01 ",
			},
		},
	}
	if err := endpoint.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if endpoint.Spec.Provider != "anthropic" {
		t.Fatalf("expected provider anthropic, got %q", endpoint.Spec.Provider)
	}
	if endpoint.Spec.BaseURL != "https://api.anthropic.com/v1" {
		t.Fatalf("unexpected anthropic base URL %q", endpoint.Spec.BaseURL)
	}
	if endpoint.Spec.Options["anthropic_version"] != "2023-06-01" {
		t.Fatalf("expected normalized option key/value, got %+v", endpoint.Spec.Options)
	}
}

func TestModelEndpointNormalizeOllamaDefaults(t *testing.T) {
	endpoint := ModelEndpoint{
		Kind:     "ModelEndpoint",
		Metadata: ObjectMeta{Name: "ollama-local"},
		Spec: ModelEndpointSpec{
			Provider: "ollama",
		},
	}
	if err := endpoint.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if endpoint.Spec.BaseURL != "http://127.0.0.1:11434" {
		t.Fatalf("unexpected ollama base URL %q", endpoint.Spec.BaseURL)
	}
}

func TestParseModelEndpointManifestYAML(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: openai-team-a
  namespace: team-a
spec:
  provider: openai
  base_url: https://api.openai.com/v1
  default_model: gpt-4o-mini
  options:
    api_variant: responses
  auth:
    secretRef: openai-api-key
`)
	endpoint, err := ParseModelEndpointManifest(raw)
	if err != nil {
		t.Fatalf("parse model endpoint failed: %v", err)
	}
	if endpoint.Metadata.Name != "openai-team-a" || endpoint.Metadata.Namespace != "team-a" {
		t.Fatalf("unexpected metadata: %+v", endpoint.Metadata)
	}
	if endpoint.Spec.Provider != "openai" {
		t.Fatalf("unexpected provider %q", endpoint.Spec.Provider)
	}
	if endpoint.Spec.Options["api_variant"] != "responses" {
		t.Fatalf("expected parsed options map, got %+v", endpoint.Spec.Options)
	}
	if endpoint.Spec.Auth.SecretRef != "openai-api-key" {
		t.Fatalf("unexpected auth secretRef %q", endpoint.Spec.Auth.SecretRef)
	}
}
