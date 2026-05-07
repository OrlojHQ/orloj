package resources

import (
	"encoding/json"
	"fmt"

	yaml "go.yaml.in/yaml/v2"
)

// ParseContextAdapterManifest parses ContextAdapter resources from JSON or YAML (Kubernetes-style).
func ParseContextAdapterManifest(data []byte) (ContextAdapter, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return ContextAdapter{}, err
	}
	var out ContextAdapter
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return ContextAdapter{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return ContextAdapter{}, err
		}
		return out, nil
	}

	if err := yaml.Unmarshal(data, &out); err != nil {
		return ContextAdapter{}, fmt.Errorf("failed to decode YAML manifest: %w", err)
	}
	if err := out.Normalize(); err != nil {
		return ContextAdapter{}, err
	}
	return out, nil
}
