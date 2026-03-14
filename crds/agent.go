package crds

import (
	"fmt"
	"strings"
)

const DefaultNamespace = "default"

// TypeMeta mirrors Kubernetes-style resource identity fields.
type TypeMeta struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// ObjectMeta stores metadata for a resource.
type ObjectMeta struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	Generation      int64             `json:"generation,omitempty"`
	CreatedAt       string            `json:"createdAt,omitempty"`
}

func NormalizeNamespace(namespace string) string {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return DefaultNamespace
	}
	return namespace
}

func NormalizeObjectMetaNamespace(meta *ObjectMeta) {
	if meta == nil {
		return
	}
	meta.Namespace = NormalizeNamespace(meta.Namespace)
}

// Agent represents the desired and observed state for a single agent runtime.
type Agent struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Metadata   ObjectMeta  `json:"metadata"`
	Spec       AgentSpec   `json:"spec"`
	Status     AgentStatus `json:"status,omitempty"`
}

// AgentList is returned by list API calls.
type AgentList struct {
	Items []Agent `json:"items"`
}

// AgentSpec defines desired runtime behavior.
type AgentSpec struct {
	Model    string      `json:"model,omitempty"`
	ModelRef string      `json:"model_ref,omitempty"`
	Prompt   string      `json:"prompt"`
	Tools    []string    `json:"tools,omitempty"`
	Roles    []string    `json:"roles,omitempty"`
	Memory   MemorySpec  `json:"memory,omitempty"`
	Limits   AgentLimits `json:"limits,omitempty"`
}

// MemorySpec configures runtime memory backend.
type MemorySpec struct {
	Ref      string `json:"ref,omitempty"`
	Type     string `json:"type,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// AgentLimits configures execution safety bounds.
type AgentLimits struct {
	MaxSteps int    `json:"max_steps,omitempty"`
	Timeout  string `json:"timeout,omitempty"`
}

// AgentStatus represents current runtime state.
type AgentStatus struct {
	Phase              string `json:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

// Normalize applies defaults and validates the resource.
func (a *Agent) Normalize() error {
	if a.APIVersion == "" {
		a.APIVersion = "orloj.dev/v1"
	}
	if a.Kind == "" {
		a.Kind = "Agent"
	}
	if !strings.EqualFold(a.Kind, "Agent") {
		return fmt.Errorf("unsupported kind %q: only Agent is supported in MVP", a.Kind)
	}
	NormalizeObjectMetaNamespace(&a.Metadata)
	if a.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	a.Spec.Model = strings.TrimSpace(a.Spec.Model)
	a.Spec.ModelRef = strings.TrimSpace(a.Spec.ModelRef)
	if a.Spec.Model == "" && a.Spec.ModelRef == "" {
		a.Spec.Model = "gpt-4o-mini"
	}
	normalizedRoles := make([]string, 0, len(a.Spec.Roles))
	seenRoles := make(map[string]struct{}, len(a.Spec.Roles))
	for _, role := range a.Spec.Roles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		key := strings.ToLower(role)
		if _, exists := seenRoles[key]; exists {
			continue
		}
		seenRoles[key] = struct{}{}
		normalizedRoles = append(normalizedRoles, role)
	}
	a.Spec.Roles = normalizedRoles
	if a.Spec.Limits.MaxSteps <= 0 {
		a.Spec.Limits.MaxSteps = 10
	}
	if a.Status.Phase == "" {
		a.Status.Phase = "Pending"
	}
	return nil
}
