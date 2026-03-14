package crds

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseModelEndpointManifest parses ModelEndpoint resources from JSON or constrained YAML.
func ParseModelEndpointManifest(data []byte) (ModelEndpoint, error) {
	var out ModelEndpoint
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return ModelEndpoint{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return ModelEndpoint{}, err
		}
		return out, nil
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	section := ""
	subsection := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(line)
		if section == "spec" && indent <= 2 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			subsection = ""
		}
		if section == "metadata" && indent <= 2 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			subsection = ""
		}

		if strings.HasSuffix(trimmed, ":") {
			switch strings.TrimSuffix(trimmed, ":") {
			case "metadata":
				section = "metadata"
				subsection = ""
			case "spec":
				section = "spec"
				subsection = ""
			case "labels":
				if section == "metadata" {
					subsection = "labels"
				}
			case "auth":
				if section == "spec" {
					subsection = "auth"
				}
			case "options":
				if section == "spec" {
					subsection = "options"
				}
			}
			continue
		}

		key, value, ok := parseKeyValue(trimmed)
		if !ok {
			continue
		}
		value = stripQuotes(value)

		switch {
		case key == "apiVersion":
			out.APIVersion = value
		case key == "kind":
			out.Kind = value
		case section == "metadata" && subsection == "labels" && indent >= 4:
			if out.Metadata.Labels == nil {
				out.Metadata.Labels = make(map[string]string)
			}
			out.Metadata.Labels[key] = value
		case section == "metadata":
			if err := applyObjectMetaField(&out.Metadata, key, value); err != nil {
				return ModelEndpoint{}, err
			}
		case section == "spec" && subsection == "" && (key == "provider"):
			out.Spec.Provider = value
		case section == "spec" && subsection == "" && (key == "base_url" || key == "baseURL"):
			out.Spec.BaseURL = value
		case section == "spec" && subsection == "" && (key == "default_model" || key == "defaultModel"):
			out.Spec.DefaultModel = value
		case section == "spec" && subsection == "options" && indent >= 4:
			if out.Spec.Options == nil {
				out.Spec.Options = make(map[string]string)
			}
			out.Spec.Options[strings.ToLower(strings.TrimSpace(key))] = value
		case section == "spec" && subsection == "auth" && (key == "secretRef" || key == "secret_ref"):
			out.Spec.Auth.SecretRef = value
		}
	}

	if err := out.Normalize(); err != nil {
		return ModelEndpoint{}, err
	}
	return out, nil
}
