package resources

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// rejectMultiDocumentYAML returns an error if the input contains a YAML
// document separator (`---` on its own line). Multi-document streams are not
// supported; each resource must be applied individually.
func rejectMultiDocumentYAML(data []byte) error {
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "---" {
			return fmt.Errorf("multi-document YAML is not supported; split documents and apply individually")
		}
	}
	return nil
}

// yamlScalarToTyped converts a bare YAML scalar string to its natural Go type:
// "true"/"false" → bool, integer strings → int64, float strings → float64,
// inline arrays "[a, b, c]" → []any of typed elements,
// everything else → string. This ensures JSON Schema booleans (e.g.
// additionalProperties: false) and numbers round-trip correctly.
func yamlScalarToTyped(s string) any {
	switch strings.ToLower(s) {
	case "true":
		return true
	case "false":
		return false
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	// Inline flow array: [a, b, c]
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if inner == "" {
			return []any{}
		}
		parts := splitFlowArray(inner)
		arr := make([]any, 0, len(parts))
		for _, p := range parts {
			arr = append(arr, yamlScalarToTyped(stripQuotes(strings.TrimSpace(p))))
		}
		return arr
	}
	return s
}

// splitFlowArray splits a YAML flow sequence body on commas, respecting
// quoted strings so that commas inside quotes are not treated as separators.
func splitFlowArray(inner string) []string {
	var parts []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
			current.WriteByte(ch)
		case ch == '"' && !inSingle:
			inDouble = !inDouble
			current.WriteByte(ch)
		case ch == ',' && !inSingle && !inDouble:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}
	parts = append(parts, current.String())
	return parts
}

// ParseAgentManifest accepts either JSON or a constrained YAML subset for Agent resources.
func ParseAgentManifest(data []byte) (Agent, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return Agent{}, err
	}
	var agent Agent

	if json.Valid(data) {
		if err := rejectLegacyModelFieldFromAgentJSON(data); err != nil {
			return Agent{}, err
		}
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
		if section == "spec" &&
			(subsection == "memory_allow" || subsection == "execution_tool_sequence" || subsection == "execution_required_output_markers") &&
			indent <= 4 &&
			!strings.HasPrefix(trimmed, "- ") &&
			!strings.HasSuffix(trimmed, ":") {
			if subsection == "memory_allow" {
				subsection = "memory"
			} else {
				subsection = "execution"
			}
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
		case "allowed_tools", "allowedTools":
			if section == "spec" {
				subsection = "allowed_tools"
			}
		case "fallback_model_refs", "fallbackModelRefs":
			if section == "spec" {
				subsection = "fallback_model_refs"
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
			case "execution":
				if section == "spec" {
					subsection = "execution"
				}
			case "tool_sequence":
				if section == "spec" && (subsection == "execution" || subsection == "execution_tool_sequence" || subsection == "execution_required_output_markers") {
					subsection = "execution_tool_sequence"
				}
			case "required_output_markers":
				if section == "spec" && (subsection == "execution" || subsection == "execution_tool_sequence" || subsection == "execution_required_output_markers") {
					subsection = "execution_required_output_markers"
				}
			}
			continue
		}

		if section == "spec" && subsection == "tools" && strings.HasPrefix(trimmed, "- ") {
			agent.Spec.Tools = append(agent.Spec.Tools, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "allowed_tools" && strings.HasPrefix(trimmed, "- ") {
			agent.Spec.AllowedTools = append(agent.Spec.AllowedTools, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "fallback_model_refs" && strings.HasPrefix(trimmed, "- ") {
			agent.Spec.FallbackModelRefs = append(agent.Spec.FallbackModelRefs, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
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
		if section == "spec" && subsection == "execution_tool_sequence" && strings.HasPrefix(trimmed, "- ") {
			agent.Spec.Execution.ToolSequence = append(agent.Spec.Execution.ToolSequence, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "execution_required_output_markers" && strings.HasPrefix(trimmed, "- ") {
			agent.Spec.Execution.RequiredOutputMarkers = append(agent.Spec.Execution.RequiredOutputMarkers, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
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
			return Agent{}, fmt.Errorf("spec.model has been removed; use spec.model_ref")
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
		case section == "spec" && subsection == "execution" && key == "profile":
			agent.Spec.Execution.Profile = stripQuotes(value)
		case section == "spec" && subsection == "execution" && (key == "duplicate_tool_call_policy" || key == "duplicateToolCallPolicy"):
			agent.Spec.Execution.DuplicateToolCallPolicy = stripQuotes(value)
		case section == "spec" && subsection == "execution" && (key == "on_contract_violation" || key == "onContractViolation"):
			agent.Spec.Execution.OnContractViolation = stripQuotes(value)
		case section == "spec" && subsection == "execution" && (key == "tool_use_behavior" || key == "toolUseBehavior"):
			agent.Spec.Execution.ToolUseBehavior = stripQuotes(value)
		}
	}

	if inPromptBlock {
		agent.Spec.Prompt = strings.TrimRight(strings.Join(promptLines, "\n"), "\n")
	}

	// Post-scan: extract output_schema block under execution (nested map, not a scalar).
	if len(agent.Spec.Execution.OutputSchema) == 0 {
		inExecution := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			ind := leadingSpaces(line)
			if trimmed == "execution:" && ind == 2 {
				inExecution = true
				continue
			}
			if inExecution && ind <= 2 && trimmed != "" {
				inExecution = false
			}
			if inExecution && (trimmed == "output_schema:" || trimmed == "outputSchema:") {
				blockIndent := -1
				var blockLines []string
				for j := i + 1; j < len(lines); j++ {
					ct := strings.TrimSpace(lines[j])
					if ct == "" || strings.HasPrefix(ct, "#") {
						continue
					}
					ci := leadingSpaces(lines[j])
					if ci <= ind {
						break
					}
					if blockIndent < 0 {
						blockIndent = ci
					}
					if ci < blockIndent {
						break
					}
					blockLines = append(blockLines, lines[j])
				}
				if blockIndent > 0 && len(blockLines) > 0 {
					agent.Spec.Execution.OutputSchema = parseSimpleYAMLMap(blockLines, blockIndent)
				}
				break
			}
		}
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
		quote := s[0]
		if quote == '"' || quote == '\'' {
			end := strings.LastIndexByte(s, quote)
			if end > 0 {
				return s[1:end]
			}
		}
	}
	return stripInlineComment(s)
}

// stripInlineComment removes a trailing ` # ...` YAML inline comment from an
// unquoted scalar value.
func stripInlineComment(s string) string {
	if idx := strings.Index(s, " #"); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

// parseSimpleYAMLMap parses a block of indented YAML lines into a map[string]any.
// It handles nested maps, arrays of strings, and scalar values — enough for
// JSON Schema objects used in tool input_schema definitions.
func parseSimpleYAMLMap(lines []string, baseIndent int) map[string]any {
	out := make(map[string]any)
	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}
		ind := leadingSpaces(line)
		if ind < baseIndent {
			break
		}
		if ind > baseIndent {
			i++
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			i++
			continue
		}
		k, v, ok := parseKeyValue(trimmed)
		if !ok {
			i++
			continue
		}
		v = strings.TrimSpace(v)
		if v == "" {
			childIndent := -1
			var childLines []string
			for j := i + 1; j < len(lines); j++ {
				ct := strings.TrimSpace(lines[j])
				if ct == "" || strings.HasPrefix(ct, "#") {
					continue
				}
				ci := leadingSpaces(lines[j])
				if ci <= baseIndent {
					break
				}
				if childIndent < 0 {
					childIndent = ci
				}
				if ci < childIndent {
					break
				}
				childLines = append(childLines, lines[j])
			}
			if len(childLines) > 0 && strings.HasPrefix(strings.TrimSpace(childLines[0]), "- ") {
				arr := make([]any, 0, len(childLines))
				for _, cl := range childLines {
					ct := strings.TrimSpace(cl)
					if strings.HasPrefix(ct, "- ") {
						arr = append(arr, stripQuotes(strings.TrimSpace(strings.TrimPrefix(ct, "- "))))
					}
				}
				out[k] = arr
			} else if childIndent > 0 {
				out[k] = parseSimpleYAMLMap(childLines, childIndent)
			}
		} else {
			out[k] = yamlScalarToTyped(stripQuotes(v))
		}
		i++
	}
	return out
}

func rejectLegacyModelFieldFromAgentJSON(data []byte) error {
	var probe struct {
		Spec map[string]json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("failed to decode JSON manifest: %w", err)
	}
	if probe.Spec == nil {
		return nil
	}
	if _, ok := probe.Spec["model"]; ok {
		return fmt.Errorf("spec.model has been removed; use spec.model_ref")
	}
	return nil
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
