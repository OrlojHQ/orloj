package resources

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ParseAgentManifest accepts either JSON or a constrained YAML subset for Agent resources.
func ParseAgentManifest(data []byte) (Agent, error) {
	var agent Agent

	if json.Valid(data) {
		if err := json.Unmarshal(data, &agent); err != nil {
			return Agent{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := agent.Normalize(); err != nil {
			return Agent{}, err
		}
		return agent, nil
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	section := ""
	subsection := ""
	inPromptBlock := false
	promptIndent := 0
	promptLines := make([]string, 0)

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := leadingSpaces(line)

		if inPromptBlock {
			if indent > promptIndent {
				textStart := promptIndent + 2
				if len(line) > textStart {
					promptLines = append(promptLines, line[textStart:])
				} else {
					promptLines = append(promptLines, strings.TrimSpace(line))
				}
				continue
			}
			agent.Spec.Prompt = strings.TrimRight(strings.Join(promptLines, "\n"), "\n")
			inPromptBlock = false
			promptLines = promptLines[:0]
		}

		if section == "spec" && indent <= 2 && !strings.HasPrefix(trimmed, "- ") && !strings.HasSuffix(trimmed, ":") {
			subsection = ""
		}
		if section == "metadata" && indent <= 2 && !strings.HasPrefix(trimmed, "- ") && !strings.HasSuffix(trimmed, ":") {
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
			case "tools":
				if section == "spec" {
					subsection = "tools"
				}
			case "roles":
				if section == "spec" {
					subsection = "roles"
				}
			case "memory":
				if section == "spec" {
					subsection = "memory"
				}
			case "allow":
				if section == "spec" && subsection == "memory" {
					subsection = "memory_allow"
				}
			case "limits":
				if section == "spec" {
					subsection = "limits"
				}
			}
			continue
		}

		if section == "spec" && subsection == "tools" && strings.HasPrefix(trimmed, "- ") {
			agent.Spec.Tools = append(agent.Spec.Tools, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "roles" && strings.HasPrefix(trimmed, "- ") {
			agent.Spec.Roles = append(agent.Spec.Roles, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "memory_allow" && strings.HasPrefix(trimmed, "- ") {
			agent.Spec.Memory.Allow = append(agent.Spec.Memory.Allow, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}

		key, value, ok := parseKeyValue(trimmed)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)

		switch {
		case key == "apiVersion":
			agent.APIVersion = stripQuotes(value)
		case key == "kind":
			agent.Kind = stripQuotes(value)
		case section == "metadata" && subsection == "labels" && indent >= 4:
			if agent.Metadata.Labels == nil {
				agent.Metadata.Labels = make(map[string]string)
			}
			agent.Metadata.Labels[key] = stripQuotes(value)
		case section == "metadata":
			if err := applyObjectMetaField(&agent.Metadata, key, stripQuotes(value)); err != nil {
				return Agent{}, err
			}
		case section == "spec" && subsection == "" && key == "model":
			agent.Spec.Model = stripQuotes(value)
		case section == "spec" && subsection == "" && (key == "model_ref" || key == "modelRef"):
			agent.Spec.ModelRef = stripQuotes(value)
		case section == "spec" && subsection == "" && key == "prompt":
			if value == "|" || value == "|-" || value == "|+" {
				inPromptBlock = true
				promptIndent = indent
			} else {
				agent.Spec.Prompt = stripQuotes(value)
			}
		case section == "spec" && subsection == "memory" && key == "type":
			agent.Spec.Memory.Type = stripQuotes(value)
		case section == "spec" && subsection == "memory" && key == "provider":
			agent.Spec.Memory.Provider = stripQuotes(value)
		case section == "spec" && subsection == "memory" && key == "ref":
			agent.Spec.Memory.Ref = stripQuotes(value)
		case section == "spec" && subsection == "limits" && key == "max_steps":
			maxSteps, err := strconv.Atoi(stripQuotes(value))
			if err != nil {
				return Agent{}, fmt.Errorf("invalid spec.limits.max_steps value %q", value)
			}
			agent.Spec.Limits.MaxSteps = maxSteps
		case section == "spec" && subsection == "limits" && key == "timeout":
			agent.Spec.Limits.Timeout = stripQuotes(value)
		}
	}

	if inPromptBlock {
		agent.Spec.Prompt = strings.TrimRight(strings.Join(promptLines, "\n"), "\n")
	}

	if err := agent.Normalize(); err != nil {
		return Agent{}, err
	}
	return agent, nil
}

func leadingSpaces(s string) int {
	i := 0
	for i < len(s) && s[i] == ' ' {
		i++
	}
	return i
}

func parseKeyValue(line string) (key, value string, ok bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func stripQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func applyObjectMetaField(meta *ObjectMeta, key string, value string) error {
	switch key {
	case "name":
		meta.Name = value
	case "namespace":
		meta.Namespace = value
	case "resourceVersion", "resource_version":
		meta.ResourceVersion = value
	case "generation":
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid metadata.generation value %q", value)
		}
		meta.Generation = v
	case "createdAt", "created_at":
		meta.CreatedAt = value
	}
	return nil
}
