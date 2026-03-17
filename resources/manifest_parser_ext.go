package resources

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// DetectKind extracts resource kind from a JSON or constrained YAML manifest.
func DetectKind(data []byte) (string, error) {
	if json.Valid(data) {
		var tm struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(data, &tm); err != nil {
			return "", fmt.Errorf("failed to decode manifest kind: %w", err)
		}
		if strings.TrimSpace(tm.Kind) == "" {
			return "", fmt.Errorf("kind is required")
		}
		return strings.TrimSpace(tm.Kind), nil
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := parseKeyValue(trimmed)
		if !ok {
			continue
		}
		if key == "kind" {
			kind := stripQuotes(value)
			if kind == "" {
				return "", fmt.Errorf("kind is required")
			}
			return kind, nil
		}
	}
	return "", fmt.Errorf("kind is required")
}

// ParseAgentSystemManifest parses AgentSystem resources from JSON or constrained YAML.
func ParseAgentSystemManifest(data []byte) (AgentSystem, error) {
	var out AgentSystem
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return AgentSystem{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return AgentSystem{}, err
		}
		return out, nil
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	section := ""
	subsection := ""
	currentGraphNode := ""
	graphNodeSection := ""
	edgeNestedSection := ""
	currentGraphEdgeIndex := -1
	out.Spec.Graph = make(map[string]GraphEdge)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(line)

		if section == "spec" && indent <= 2 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			subsection = ""
			currentGraphNode = ""
			graphNodeSection = ""
			edgeNestedSection = ""
			currentGraphEdgeIndex = -1
		}
		if section == "metadata" && indent <= 2 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			subsection = ""
		}
		if section == "spec" && subsection == "graph" && indent <= 4 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			graphNodeSection = ""
			edgeNestedSection = ""
			currentGraphEdgeIndex = -1
		}

		if strings.HasSuffix(trimmed, ":") {
			key := strings.TrimSuffix(trimmed, ":")
			switch {
			case key == "metadata":
				section = "metadata"
				subsection = ""
				currentGraphNode = ""
				graphNodeSection = ""
				edgeNestedSection = ""
				currentGraphEdgeIndex = -1
			case key == "spec":
				section = "spec"
				subsection = ""
				currentGraphNode = ""
				graphNodeSection = ""
				edgeNestedSection = ""
				currentGraphEdgeIndex = -1
			case section == "spec" && key == "agents":
				subsection = "agents"
				currentGraphNode = ""
				graphNodeSection = ""
				edgeNestedSection = ""
				currentGraphEdgeIndex = -1
			case section == "spec" && key == "graph":
				subsection = "graph"
				currentGraphNode = ""
				graphNodeSection = ""
				edgeNestedSection = ""
				currentGraphEdgeIndex = -1
			case section == "metadata" && key == "labels":
				subsection = "labels"
				currentGraphNode = ""
			case section == "spec" && subsection == "graph" && indent >= 4:
				if currentGraphNode != "" && indent >= 6 && key == "edges" {
					graphNodeSection = "edges"
					edgeNestedSection = ""
					currentGraphEdgeIndex = -1
					break
				}
				if currentGraphNode != "" && indent >= 6 && key == "join" {
					graphNodeSection = "join"
					edgeNestedSection = ""
					currentGraphEdgeIndex = -1
					break
				}
				if currentGraphNode != "" && graphNodeSection == "edges" && indent >= 8 && (key == "labels" || key == "policy") {
					edgeNestedSection = key
					break
				}

				currentGraphNode = stripQuotes(key)
				graphNodeSection = ""
				edgeNestedSection = ""
				currentGraphEdgeIndex = -1
				if _, ok := out.Spec.Graph[currentGraphNode]; !ok {
					out.Spec.Graph[currentGraphNode] = GraphEdge{}
				}
			}
			continue
		}

		if section == "spec" && subsection == "agents" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.Agents = append(out.Spec.Agents, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "graph" && currentGraphNode != "" && graphNodeSection == "edges" && strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			route := GraphRoute{}
			if item != "" {
				if k, v, ok := parseKeyValue(item); ok {
					k = strings.TrimSpace(k)
					v = stripQuotes(v)
					if k == "to" || k == "next" {
						route.To = v
					}
				} else {
					route.To = stripQuotes(item)
				}
			}
			node := out.Spec.Graph[currentGraphNode]
			node.Edges = append(node.Edges, route)
			out.Spec.Graph[currentGraphNode] = node
			currentGraphEdgeIndex = len(node.Edges) - 1
			edgeNestedSection = ""
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
				return AgentSystem{}, err
			}
		case section == "spec" && subsection == "graph" && currentGraphNode != "":
			node := out.Spec.Graph[currentGraphNode]
			switch {
			case graphNodeSection == "join":
				switch key {
				case "mode":
					node.Join.Mode = value
				case "on_failure", "onFailure":
					node.Join.OnFailure = value
				case "quorum_count", "quorumCount":
					v, err := strconv.Atoi(value)
					if err != nil {
						return AgentSystem{}, fmt.Errorf("invalid spec.graph.%s.join.quorum_count value %q", currentGraphNode, value)
					}
					node.Join.QuorumCount = v
				case "quorum_percent", "quorumPercent":
					v, err := strconv.Atoi(value)
					if err != nil {
						return AgentSystem{}, fmt.Errorf("invalid spec.graph.%s.join.quorum_percent value %q", currentGraphNode, value)
					}
					node.Join.QuorumPercent = v
				}
			case graphNodeSection == "edges":
				if currentGraphEdgeIndex < 0 {
					node.Edges = append(node.Edges, GraphRoute{})
					currentGraphEdgeIndex = len(node.Edges) - 1
				}
				route := node.Edges[currentGraphEdgeIndex]
				switch edgeNestedSection {
				case "labels":
					if route.Labels == nil {
						route.Labels = make(map[string]string)
					}
					route.Labels[key] = value
				case "policy":
					if route.Policy == nil {
						route.Policy = make(map[string]string)
					}
					route.Policy[key] = value
				default:
					if key == "to" || key == "next" {
						route.To = value
					}
				}
				node.Edges[currentGraphEdgeIndex] = route
			default:
				if key == "next" {
					node.Next = value
				}
			}
			out.Spec.Graph[currentGraphNode] = node
		}
	}

	if err := out.Normalize(); err != nil {
		return AgentSystem{}, err
	}
	return out, nil
}

// ParseToolManifest parses Tool resources from JSON or constrained YAML.
func ParseToolManifest(data []byte) (Tool, error) {
	var out Tool
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return Tool{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return Tool{}, err
		}
		return out, nil
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	section := ""
	subsection := ""
	runtimeSubsection := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(line)
		if section == "spec" && indent <= 2 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			subsection = ""
			runtimeSubsection = ""
		}
		if section == "metadata" && indent <= 2 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			subsection = ""
			runtimeSubsection = ""
		}
		if section == "spec" && subsection == "runtime" && indent <= 4 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			runtimeSubsection = ""
		}

		if strings.HasSuffix(trimmed, ":") {
			switch strings.TrimSuffix(trimmed, ":") {
			case "metadata":
				section = "metadata"
				subsection = ""
				runtimeSubsection = ""
			case "spec":
				section = "spec"
				subsection = ""
				runtimeSubsection = ""
			case "labels":
				if section == "metadata" {
					subsection = "labels"
					runtimeSubsection = ""
				}
			case "auth":
				if section == "spec" {
					subsection = "auth"
					runtimeSubsection = ""
				}
		case "capabilities":
			if section == "spec" {
				subsection = "capabilities"
				runtimeSubsection = ""
			}
		case "operation_classes", "operationClasses":
			if section == "spec" {
				subsection = "operation_classes"
				runtimeSubsection = ""
			}
		case "scopes":
			if section == "spec" && subsection == "auth" {
				runtimeSubsection = "scopes"
			}
		case "runtime":
			if section == "spec" {
				subsection = "runtime"
				runtimeSubsection = ""
			}
		case "retry":
			if section == "spec" && subsection == "runtime" {
				runtimeSubsection = "retry"
			}
		}
		continue
	}

	if section == "spec" && subsection == "capabilities" && strings.HasPrefix(trimmed, "- ") {
		out.Spec.Capabilities = append(out.Spec.Capabilities, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
		continue
	}

	if section == "spec" && subsection == "operation_classes" && strings.HasPrefix(trimmed, "- ") {
		out.Spec.OperationClasses = append(out.Spec.OperationClasses, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
		continue
	}

	if section == "spec" && subsection == "auth" && runtimeSubsection == "scopes" && strings.HasPrefix(trimmed, "- ") {
		out.Spec.Auth.Scopes = append(out.Spec.Auth.Scopes, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
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
				return Tool{}, err
			}
		case section == "spec" && subsection == "" && key == "type":
			out.Spec.Type = value
		case section == "spec" && subsection == "" && key == "endpoint":
			out.Spec.Endpoint = value
		case section == "spec" && subsection == "" && (key == "risk_level" || key == "riskLevel"):
			out.Spec.RiskLevel = value
		case section == "spec" && subsection == "auth" && key == "profile":
			out.Spec.Auth.Profile = value
		case section == "spec" && subsection == "auth" && (key == "secretRef" || key == "secret_ref"):
			out.Spec.Auth.SecretRef = value
		case section == "spec" && subsection == "auth" && (key == "headerName" || key == "header_name"):
			out.Spec.Auth.HeaderName = value
		case section == "spec" && subsection == "auth" && (key == "tokenURL" || key == "token_url"):
			out.Spec.Auth.TokenURL = value
		case section == "spec" && subsection == "runtime" && runtimeSubsection == "" && key == "timeout":
			out.Spec.Runtime.Timeout = value
		case section == "spec" && subsection == "runtime" && runtimeSubsection == "" && (key == "isolation_mode" || key == "isolationMode"):
			out.Spec.Runtime.IsolationMode = value
		case section == "spec" && subsection == "runtime" && runtimeSubsection == "retry" && (key == "max_attempts" || key == "maxAttempts"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return Tool{}, fmt.Errorf("invalid spec.runtime.retry.max_attempts value %q", value)
			}
			out.Spec.Runtime.Retry.MaxAttempts = v
		case section == "spec" && subsection == "runtime" && runtimeSubsection == "retry" && key == "backoff":
			out.Spec.Runtime.Retry.Backoff = value
		case section == "spec" && subsection == "runtime" && runtimeSubsection == "retry" && (key == "max_backoff" || key == "maxBackoff"):
			out.Spec.Runtime.Retry.MaxBackoff = value
		case section == "spec" && subsection == "runtime" && runtimeSubsection == "retry" && key == "jitter":
			out.Spec.Runtime.Retry.Jitter = value
		}
	}

	if err := out.Normalize(); err != nil {
		return Tool{}, err
	}
	return out, nil
}

// ParseSecretManifest parses Secret resources from JSON or constrained YAML.
func ParseSecretManifest(data []byte) (Secret, error) {
	var out Secret
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return Secret{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return Secret{}, err
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
			case "data":
				if section == "spec" {
					subsection = "data"
				}
			case "stringData", "string_data":
				if section == "spec" {
					subsection = "stringData"
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
				return Secret{}, err
			}
		case section == "spec" && subsection == "data":
			if out.Spec.Data == nil {
				out.Spec.Data = make(map[string]string)
			}
			out.Spec.Data[key] = value
		case section == "spec" && subsection == "stringData":
			if out.Spec.StringData == nil {
				out.Spec.StringData = make(map[string]string)
			}
			out.Spec.StringData[key] = value
		}
	}

	if err := out.Normalize(); err != nil {
		return Secret{}, err
	}
	return out, nil
}

// ParseMemoryManifest parses Memory resources from JSON or constrained YAML.
func ParseMemoryManifest(data []byte) (Memory, error) {
	var out Memory
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return Memory{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return Memory{}, err
		}
		return out, nil
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	section := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(line)
		if section == "metadata.labels" && indent <= 2 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			section = "metadata"
		}
		if strings.HasSuffix(trimmed, ":") {
			switch strings.TrimSuffix(trimmed, ":") {
			case "metadata":
				section = "metadata"
			case "spec":
				section = "spec"
			case "labels":
				if section == "metadata" {
					section = "metadata.labels"
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
		case section == "metadata.labels" && indent >= 4:
			if out.Metadata.Labels == nil {
				out.Metadata.Labels = make(map[string]string)
			}
			out.Metadata.Labels[key] = value
		case section == "metadata":
			if err := applyObjectMetaField(&out.Metadata, key, value); err != nil {
				return Memory{}, err
			}
		case section == "spec" && key == "type":
			out.Spec.Type = value
		case section == "spec" && key == "provider":
			out.Spec.Provider = value
		case section == "spec" && (key == "embedding_model" || key == "embeddingModel"):
			out.Spec.EmbeddingModel = value
		}
	}

	if err := out.Normalize(); err != nil {
		return Memory{}, err
	}
	return out, nil
}

// ParseAgentPolicyManifest parses AgentPolicy resources from JSON or constrained YAML.
func ParseAgentPolicyManifest(data []byte) (AgentPolicy, error) {
	var out AgentPolicy
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return AgentPolicy{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return AgentPolicy{}, err
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
			case "allowed_models":
				if section == "spec" {
					subsection = "allowed_models"
				}
			case "blocked_tools":
				if section == "spec" {
					subsection = "blocked_tools"
				}
			case "target_systems":
				if section == "spec" {
					subsection = "target_systems"
				}
			case "target_tasks":
				if section == "spec" {
					subsection = "target_tasks"
				}
			}
			continue
		}

		if section == "spec" && subsection == "allowed_models" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.AllowedModels = append(out.Spec.AllowedModels, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "blocked_tools" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.BlockedTools = append(out.Spec.BlockedTools, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "target_systems" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.TargetSystems = append(out.Spec.TargetSystems, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "target_tasks" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.TargetTasks = append(out.Spec.TargetTasks, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
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
				return AgentPolicy{}, err
			}
		case section == "spec" && key == "max_tokens_per_run":
			v, err := strconv.Atoi(value)
			if err != nil {
				return AgentPolicy{}, fmt.Errorf("invalid spec.max_tokens_per_run value %q", value)
			}
			out.Spec.MaxTokensPerRun = v
		case section == "spec" && key == "apply_mode":
			out.Spec.ApplyMode = value
		}
	}

	if err := out.Normalize(); err != nil {
		return AgentPolicy{}, err
	}
	return out, nil
}

// ParseAgentRoleManifest parses AgentRole resources from JSON or constrained YAML.
func ParseAgentRoleManifest(data []byte) (AgentRole, error) {
	var out AgentRole
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return AgentRole{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return AgentRole{}, err
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
			case "permissions":
				if section == "spec" {
					subsection = "permissions"
				}
			}
			continue
		}

		if section == "spec" && subsection == "permissions" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.Permissions = append(out.Spec.Permissions, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
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
				return AgentRole{}, err
			}
		case section == "spec" && key == "description":
			out.Spec.Description = value
		}
	}

	if err := out.Normalize(); err != nil {
		return AgentRole{}, err
	}
	return out, nil
}

// ParseToolPermissionManifest parses ToolPermission resources from JSON or constrained YAML.
func ParseToolPermissionManifest(data []byte) (ToolPermission, error) {
	var out ToolPermission
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return ToolPermission{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return ToolPermission{}, err
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
			case "required_permissions", "requiredPermissions":
				if section == "spec" {
					subsection = "required_permissions"
				}
			case "target_agents", "targetAgents":
				if section == "spec" {
					subsection = "target_agents"
				}
			case "operation_rules", "operationRules":
				if section == "spec" {
					subsection = "operation_rules"
				}
			}
			continue
		}

		if section == "spec" && subsection == "required_permissions" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.RequiredPermissions = append(out.Spec.RequiredPermissions, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "target_agents" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.TargetAgents = append(out.Spec.TargetAgents, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "operation_rules" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.OperationRules = append(out.Spec.OperationRules, OperationRule{})
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if k, v, ok := parseKeyValue(rest); ok {
				idx := len(out.Spec.OperationRules) - 1
				switch k {
				case "operation_class", "operationClass":
					out.Spec.OperationRules[idx].OperationClass = stripQuotes(v)
				case "verdict":
					out.Spec.OperationRules[idx].Verdict = stripQuotes(v)
				}
			}
			continue
		}
		if section == "spec" && subsection == "operation_rules" && len(out.Spec.OperationRules) > 0 {
			if k, v, ok := parseKeyValue(trimmed); ok {
				idx := len(out.Spec.OperationRules) - 1
				switch k {
				case "operation_class", "operationClass":
					out.Spec.OperationRules[idx].OperationClass = stripQuotes(v)
				case "verdict":
					out.Spec.OperationRules[idx].Verdict = stripQuotes(v)
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
				return ToolPermission{}, err
			}
		case section == "spec" && (key == "tool_ref" || key == "toolRef"):
			out.Spec.ToolRef = value
		case section == "spec" && key == "action":
			out.Spec.Action = value
		case section == "spec" && (key == "match_mode" || key == "matchMode"):
			out.Spec.MatchMode = value
		case section == "spec" && (key == "apply_mode" || key == "applyMode"):
			out.Spec.ApplyMode = value
		}
	}

	if err := out.Normalize(); err != nil {
		return ToolPermission{}, err
	}
	return out, nil
}

// ParseTaskManifest parses Task resources from JSON or constrained YAML.
func ParseTaskManifest(data []byte) (Task, error) {
	var out Task
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return Task{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return Task{}, err
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
			case "input":
				if section == "spec" {
					subsection = "input"
				}
			case "retry":
				if section == "spec" {
					subsection = "retry"
				}
			case "message_retry":
				if section == "spec" {
					subsection = "message_retry"
				}
			case "requirements":
				if section == "spec" {
					subsection = "requirements"
				}
			case "non_retryable":
				if section == "spec" && subsection == "message_retry" {
					subsection = "message_retry_non_retryable"
				}
			}
			continue
		}

		if section == "spec" && subsection == "message_retry_non_retryable" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.MessageRetry.NonRetryable = append(out.Spec.MessageRetry.NonRetryable, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
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
				return Task{}, err
			}
		case section == "spec" && subsection == "" && key == "system":
			out.Spec.System = value
		case section == "spec" && subsection == "" && key == "mode":
			out.Spec.Mode = value
		case section == "spec" && subsection == "" && key == "priority":
			out.Spec.Priority = value
		case section == "spec" && subsection == "" && (key == "max_turns" || key == "maxTurns"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return Task{}, fmt.Errorf("invalid spec.max_turns value %q", value)
			}
			out.Spec.MaxTurns = v
		case section == "spec" && subsection == "retry" && key == "max_attempts":
			v, err := strconv.Atoi(value)
			if err != nil {
				return Task{}, fmt.Errorf("invalid spec.retry.max_attempts value %q", value)
			}
			out.Spec.Retry.MaxAttempts = v
		case section == "spec" && subsection == "retry" && key == "backoff":
			out.Spec.Retry.Backoff = value
		case section == "spec" && subsection == "message_retry" && key == "max_attempts":
			v, err := strconv.Atoi(value)
			if err != nil {
				return Task{}, fmt.Errorf("invalid spec.message_retry.max_attempts value %q", value)
			}
			out.Spec.MessageRetry.MaxAttempts = v
		case section == "spec" && subsection == "message_retry" && key == "backoff":
			out.Spec.MessageRetry.Backoff = value
		case section == "spec" && subsection == "message_retry" && (key == "max_backoff" || key == "maxBackoff"):
			out.Spec.MessageRetry.MaxBackoff = value
		case section == "spec" && subsection == "message_retry" && key == "jitter":
			out.Spec.MessageRetry.Jitter = value
		case section == "spec" && subsection == "requirements" && key == "region":
			out.Spec.Requirements.Region = value
		case section == "spec" && subsection == "requirements" && key == "gpu":
			out.Spec.Requirements.GPU = strings.EqualFold(value, "true") || value == "1"
		case section == "spec" && subsection == "requirements" && key == "model":
			out.Spec.Requirements.Model = value
		case section == "spec" && subsection == "input":
			if out.Spec.Input == nil {
				out.Spec.Input = make(map[string]string)
			}
			out.Spec.Input[key] = value
		}
	}

	if err := out.Normalize(); err != nil {
		return Task{}, err
	}
	return out, nil
}

// ParseTaskScheduleManifest parses TaskSchedule resources from JSON or constrained YAML.
func ParseTaskScheduleManifest(data []byte) (TaskSchedule, error) {
	var out TaskSchedule
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return TaskSchedule{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return TaskSchedule{}, err
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
				return TaskSchedule{}, err
			}
		case section == "spec" && subsection == "" && (key == "task_ref" || key == "taskRef"):
			out.Spec.TaskRef = value
		case section == "spec" && subsection == "" && key == "schedule":
			out.Spec.Schedule = value
		case section == "spec" && subsection == "" && (key == "time_zone" || key == "timeZone"):
			out.Spec.TimeZone = value
		case section == "spec" && subsection == "" && key == "suspend":
			out.Spec.Suspend = strings.EqualFold(value, "true") || value == "1"
		case section == "spec" && subsection == "" && (key == "starting_deadline_seconds" || key == "startingDeadlineSeconds"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return TaskSchedule{}, fmt.Errorf("invalid spec.starting_deadline_seconds value %q", value)
			}
			out.Spec.StartingDeadlineSeconds = v
		case section == "spec" && subsection == "" && (key == "concurrency_policy" || key == "concurrencyPolicy"):
			out.Spec.ConcurrencyPolicy = value
		case section == "spec" && subsection == "" && (key == "successful_history_limit" || key == "successfulHistoryLimit"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return TaskSchedule{}, fmt.Errorf("invalid spec.successful_history_limit value %q", value)
			}
			out.Spec.SuccessfulHistoryLimit = v
		case section == "spec" && subsection == "" && (key == "failed_history_limit" || key == "failedHistoryLimit"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return TaskSchedule{}, fmt.Errorf("invalid spec.failed_history_limit value %q", value)
			}
			out.Spec.FailedHistoryLimit = v
		}
	}

	if err := out.Normalize(); err != nil {
		return TaskSchedule{}, err
	}
	return out, nil
}

// ParseTaskWebhookManifest parses TaskWebhook resources from JSON or constrained YAML.
func ParseTaskWebhookManifest(data []byte) (TaskWebhook, error) {
	var out TaskWebhook
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return TaskWebhook{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return TaskWebhook{}, err
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
			case "auth", "idempotency", "payload":
				if section == "spec" {
					subsection = strings.TrimSuffix(trimmed, ":")
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
				return TaskWebhook{}, err
			}
		case section == "spec" && subsection == "" && (key == "task_ref" || key == "taskRef"):
			out.Spec.TaskRef = value
		case section == "spec" && subsection == "" && key == "suspend":
			out.Spec.Suspend = strings.EqualFold(value, "true") || value == "1"
		case section == "spec" && subsection == "auth" && key == "profile":
			out.Spec.Auth.Profile = value
		case section == "spec" && subsection == "auth" && (key == "secret_ref" || key == "secretRef"):
			out.Spec.Auth.SecretRef = value
		case section == "spec" && subsection == "auth" && (key == "signature_header" || key == "signatureHeader"):
			out.Spec.Auth.SignatureHeader = value
		case section == "spec" && subsection == "auth" && (key == "signature_prefix" || key == "signaturePrefix"):
			out.Spec.Auth.SignaturePrefix = value
		case section == "spec" && subsection == "auth" && (key == "timestamp_header" || key == "timestampHeader"):
			out.Spec.Auth.TimestampHeader = value
		case section == "spec" && subsection == "auth" && (key == "max_skew_seconds" || key == "maxSkewSeconds"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return TaskWebhook{}, fmt.Errorf("invalid spec.auth.max_skew_seconds value %q", value)
			}
			out.Spec.Auth.MaxSkewSeconds = v
		case section == "spec" && subsection == "idempotency" && (key == "event_id_header" || key == "eventIdHeader"):
			out.Spec.Idempotency.EventIDHeader = value
		case section == "spec" && subsection == "idempotency" && (key == "dedupe_window_seconds" || key == "dedupeWindowSeconds"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return TaskWebhook{}, fmt.Errorf("invalid spec.idempotency.dedupe_window_seconds value %q", value)
			}
			out.Spec.Idempotency.DedupeWindowSeconds = v
		case section == "spec" && subsection == "payload" && key == "mode":
			out.Spec.Payload.Mode = value
		case section == "spec" && subsection == "payload" && (key == "input_key" || key == "inputKey"):
			out.Spec.Payload.InputKey = value
		}
	}

	if err := out.Normalize(); err != nil {
		return TaskWebhook{}, err
	}
	return out, nil
}

// ParseWorkerManifest parses Worker resources from JSON or constrained YAML.
func ParseWorkerManifest(data []byte) (Worker, error) {
	var out Worker
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return Worker{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return Worker{}, err
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
			case "capabilities":
				if section == "spec" {
					subsection = "capabilities"
				}
			case "supported_models":
				if section == "spec" && subsection == "capabilities" {
					subsection = "supported_models"
				}
			}
			continue
		}

		if section == "spec" && subsection == "supported_models" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.Capabilities.SupportedModels = append(
				out.Spec.Capabilities.SupportedModels,
				stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))),
			)
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
				return Worker{}, err
			}
		case section == "spec" && subsection == "" && key == "region":
			out.Spec.Region = value
		case section == "spec" && subsection == "" && key == "max_concurrent_tasks":
			v, err := strconv.Atoi(value)
			if err != nil {
				return Worker{}, fmt.Errorf("invalid spec.max_concurrent_tasks value %q", value)
			}
			out.Spec.MaxConcurrentTasks = v
		case section == "spec" && subsection == "capabilities" && key == "gpu":
			out.Spec.Capabilities.GPU = strings.EqualFold(value, "true") || value == "1"
		}
	}

	if err := out.Normalize(); err != nil {
		return Worker{}, err
	}
	return out, nil
}
