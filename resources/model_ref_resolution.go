package resources

import (
	"fmt"
	"strings"
)

// ModelEndpointLookup resolves model endpoints by scoped name (namespace/name).
type ModelEndpointLookup interface {
	Get(name string) (ModelEndpoint, bool)
}

// ParseModelEndpointRef resolves model endpoint references in name or namespace/name form.
func ParseModelEndpointRef(defaultNamespace string, ref string) (namespace string, name string) {
	ref = strings.TrimSpace(ref)
	namespace = NormalizeNamespace(defaultNamespace)
	if strings.Contains(ref, "/") {
		parts := strings.SplitN(ref, "/", 2)
		return NormalizeNamespace(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])
	}
	return namespace, ref
}

// ResolveAgentModelRef resolves an agent model_ref to a concrete endpoint and model identifier.
func ResolveAgentModelRef(defaultNamespace string, modelRef string, endpoints ModelEndpointLookup) (ModelEndpoint, string, error) {
	if endpoints == nil {
		return ModelEndpoint{}, "", fmt.Errorf("no model endpoint store configured")
	}
	modelRef = strings.TrimSpace(modelRef)
	if modelRef == "" {
		return ModelEndpoint{}, "", fmt.Errorf("spec.model_ref is required")
	}
	refNamespace, refName := ParseModelEndpointRef(defaultNamespace, modelRef)
	key := NormalizeNamespace(refNamespace) + "/" + strings.TrimSpace(refName)
	endpoint, ok := endpoints.Get(key)
	if !ok {
		return ModelEndpoint{}, "", fmt.Errorf("model endpoint %q not found", modelRef)
	}
	model := strings.TrimSpace(endpoint.Spec.DefaultModel)
	if model == "" {
		return ModelEndpoint{}, "", fmt.Errorf("model endpoint %q has empty spec.default_model", modelRef)
	}
	return endpoint, model, nil
}
