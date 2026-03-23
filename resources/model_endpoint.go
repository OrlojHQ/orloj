package resources

import (
	"fmt"
	"strings"
)

// ModelEndpoint declares a model-provider endpoint for agent runtime routing.
type ModelEndpoint struct {
	APIVersion string              `json:"apiVersion"`
	Kind       string              `json:"kind"`
	Metadata   ObjectMeta          `json:"metadata"`
	Spec       ModelEndpointSpec   `json:"spec"`
	Status     ModelEndpointStatus `json:"status,omitempty"`
}

type ModelEndpointSpec struct {
	Provider     string            `json:"provider,omitempty"`
	BaseURL      string            `json:"base_url,omitempty"`
	DefaultModel string            `json:"default_model,omitempty"`
	Options      map[string]string `json:"options,omitempty"`
	Auth         ModelEndpointAuth `json:"auth,omitempty"`
}

type ModelEndpointAuth struct {
	SecretRef string `json:"secretRef,omitempty"`
}

type ModelEndpointStatus struct {
	Phase              string `json:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

type ModelEndpointList struct {
	ListMeta `json:",inline"`
	Items    []ModelEndpoint `json:"items"`
}

func (m *ModelEndpoint) Normalize() error {
	if m.APIVersion == "" {
		m.APIVersion = "orloj.dev/v1"
	}
	if m.Kind == "" {
		m.Kind = "ModelEndpoint"
	}
	if !strings.EqualFold(strings.TrimSpace(m.Kind), "ModelEndpoint") {
		return fmt.Errorf("unsupported kind %q for ModelEndpoint", m.Kind)
	}
	NormalizeObjectMetaNamespace(&m.Metadata)
	if strings.TrimSpace(m.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}

	provider := strings.ToLower(strings.TrimSpace(m.Spec.Provider))
	if provider == "" {
		provider = "openai"
	}
	m.Spec.Provider = provider

	m.Spec.BaseURL = strings.TrimSpace(m.Spec.BaseURL)
	if m.Spec.BaseURL == "" {
		if m.Spec.Provider == "openai" {
			m.Spec.BaseURL = "https://api.openai.com/v1"
		} else if m.Spec.Provider == "anthropic" {
			m.Spec.BaseURL = "https://api.anthropic.com/v1"
		} else if m.Spec.Provider == "ollama" {
			m.Spec.BaseURL = "http://127.0.0.1:11434"
		}
	}
	m.Spec.DefaultModel = strings.TrimSpace(m.Spec.DefaultModel)
	if len(m.Spec.Options) > 0 {
		normalized := make(map[string]string, len(m.Spec.Options))
		for key, value := range m.Spec.Options {
			k := strings.ToLower(strings.TrimSpace(key))
			if k == "" {
				continue
			}
			normalized[k] = strings.TrimSpace(value)
		}
		m.Spec.Options = normalized
	}
	m.Spec.Auth.SecretRef = strings.TrimSpace(m.Spec.Auth.SecretRef)

	if m.Status.Phase == "" {
		m.Status.Phase = "Pending"
	}
	return nil
}
