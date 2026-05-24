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
	if err := rejectMultiDocumentYAML(data); err != nil {
		return AgentSystem{}, err
	}
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
	delegateNestedSection := ""
	currentDelegateIndex := -1
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
			delegateNestedSection = ""
			currentDelegateIndex = -1
		}
		if section == "metadata" && indent <= 2 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			subsection = ""
		}
		if section == "spec" && subsection == "graph" && indent <= 4 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			graphNodeSection = ""
			edgeNestedSection = ""
			currentGraphEdgeIndex = -1
			delegateNestedSection = ""
			currentDelegateIndex = -1
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
				delegateNestedSection = ""
				currentDelegateIndex = -1
			case key == "spec":
				section = "spec"
				subsection = ""
				currentGraphNode = ""
				graphNodeSection = ""
				edgeNestedSection = ""
				currentGraphEdgeIndex = -1
				delegateNestedSection = ""
				currentDelegateIndex = -1
			case section == "spec" && key == "agents":
				subsection = "agents"
				currentGraphNode = ""
				graphNodeSection = ""
				edgeNestedSection = ""
				currentGraphEdgeIndex = -1
				delegateNestedSection = ""
				currentDelegateIndex = -1
			case section == "spec" && key == "graph":
				subsection = "graph"
				currentGraphNode = ""
				graphNodeSection = ""
				edgeNestedSection = ""
				currentGraphEdgeIndex = -1
				delegateNestedSection = ""
				currentDelegateIndex = -1
			case section == "spec" && key == "completion_review":
				subsection = "completion_review"
				currentGraphNode = ""
				graphNodeSection = ""
				edgeNestedSection = ""
				currentGraphEdgeIndex = -1
				delegateNestedSection = ""
				currentDelegateIndex = -1
			case section == "spec" && key == "a2a":
				subsection = "a2a"
				currentGraphNode = ""
				graphNodeSection = ""
				edgeNestedSection = ""
				currentGraphEdgeIndex = -1
				delegateNestedSection = ""
				currentDelegateIndex = -1
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
				if currentGraphNode != "" && indent >= 6 && key == "delegates" {
					graphNodeSection = "delegates"
					delegateNestedSection = ""
					currentDelegateIndex = -1
					break
				}
				if currentGraphNode != "" && indent >= 6 && (key == "delegate_join" || key == "delegateJoin") {
					graphNodeSection = "delegate_join"
					delegateNestedSection = ""
					currentDelegateIndex = -1
					break
				}
				if currentGraphNode != "" && indent >= 6 && key == "review" {
					graphNodeSection = "review"
					edgeNestedSection = ""
					currentGraphEdgeIndex = -1
					delegateNestedSection = ""
					currentDelegateIndex = -1
					break
				}
				if currentGraphNode != "" && graphNodeSection == "edges" && indent >= 8 && (key == "labels" || key == "policy" || key == "condition") {
					edgeNestedSection = key
					break
				}
				if currentGraphNode != "" && graphNodeSection == "delegates" && indent >= 8 && (key == "labels" || key == "policy" || key == "condition") {
					delegateNestedSection = key
					break
				}

				currentGraphNode = stripQuotes(key)
				graphNodeSection = ""
				edgeNestedSection = ""
				currentGraphEdgeIndex = -1
				delegateNestedSection = ""
				currentDelegateIndex = -1
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
		if section == "spec" && subsection == "graph" && currentGraphNode != "" && graphNodeSection == "delegates" && strings.HasPrefix(trimmed, "- ") {
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
			node.Delegates = append(node.Delegates, route)
			out.Spec.Graph[currentGraphNode] = node
			currentDelegateIndex = len(node.Delegates) - 1
			delegateNestedSection = ""
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
		case section == "spec" && subsection == "" && (key == "context_adapter" || key == "contextAdapter"):
			out.Spec.ContextAdapter = value
		case section == "spec" && subsection == "a2a":
			switch key {
			case "enabled":
				out.Spec.A2A.Enabled = strings.EqualFold(value, "true") || value == "1"
			case "auth":
				out.Spec.A2A.Auth = value
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
				case "condition":
					if route.Condition == nil {
						route.Condition = &EdgeCondition{}
					}
					switch key {
					case "output_contains", "outputContains":
						route.Condition.OutputContains = value
					case "output_not_contains", "outputNotContains":
						route.Condition.OutputNotContains = value
					case "output_matches", "outputMatches":
						route.Condition.OutputMatches = value
					case "default":
						route.Condition.Default = strings.EqualFold(value, "true") || value == "1"
					case "output_json_path", "outputJsonPath":
						route.Condition.OutputJSONPath = value
					case "equals":
						route.Condition.Equals = value
					case "not_equals", "notEquals":
						route.Condition.NotEquals = value
					case "contains":
						route.Condition.Contains = value
					case "greater_than", "greaterThan":
						route.Condition.GreaterThan = value
					case "less_than", "lessThan":
						route.Condition.LessThan = value
					}
				default:
					if key == "to" || key == "next" {
						route.To = value
					}
				}
				node.Edges[currentGraphEdgeIndex] = route
			case graphNodeSection == "delegates":
				if currentDelegateIndex < 0 {
					node.Delegates = append(node.Delegates, GraphRoute{})
					currentDelegateIndex = len(node.Delegates) - 1
				}
				route := node.Delegates[currentDelegateIndex]
				switch delegateNestedSection {
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
				case "condition":
					if route.Condition == nil {
						route.Condition = &EdgeCondition{}
					}
					switch key {
					case "output_contains", "outputContains":
						route.Condition.OutputContains = value
					case "output_not_contains", "outputNotContains":
						route.Condition.OutputNotContains = value
					case "output_matches", "outputMatches":
						route.Condition.OutputMatches = value
					case "default":
						route.Condition.Default = strings.EqualFold(value, "true") || value == "1"
					case "output_json_path", "outputJsonPath":
						route.Condition.OutputJSONPath = value
					case "equals":
						route.Condition.Equals = value
					case "not_equals", "notEquals":
						route.Condition.NotEquals = value
					case "contains":
						route.Condition.Contains = value
					case "greater_than", "greaterThan":
						route.Condition.GreaterThan = value
					case "less_than", "lessThan":
						route.Condition.LessThan = value
					}
				default:
					if key == "to" || key == "next" {
						route.To = value
					}
				}
				node.Delegates[currentDelegateIndex] = route
			case graphNodeSection == "delegate_join":
				switch key {
				case "mode":
					node.DelegateJoin.Mode = value
				case "on_failure", "onFailure":
					node.DelegateJoin.OnFailure = value
				case "quorum_count", "quorumCount":
					v, err := strconv.Atoi(value)
					if err != nil {
						return AgentSystem{}, fmt.Errorf("invalid spec.graph.%s.delegate_join.quorum_count value %q", currentGraphNode, value)
					}
					node.DelegateJoin.QuorumCount = v
				case "quorum_percent", "quorumPercent":
					v, err := strconv.Atoi(value)
					if err != nil {
						return AgentSystem{}, fmt.Errorf("invalid spec.graph.%s.delegate_join.quorum_percent value %q", currentGraphNode, value)
					}
					node.DelegateJoin.QuorumPercent = v
				}
			case graphNodeSection == "review":
				if node.Review == nil {
					node.Review = &ReviewCheckpointSpec{}
				}
				if err := applyReviewCheckpointField(node.Review, key, value); err != nil {
					return AgentSystem{}, fmt.Errorf("spec.graph.%s.review: %w", currentGraphNode, err)
				}
			default:
				if key == "next" {
					node.Next = value
				}
			}
			out.Spec.Graph[currentGraphNode] = node
		case section == "spec" && subsection == "completion_review":
			if out.Spec.CompletionReview == nil {
				out.Spec.CompletionReview = &ReviewCheckpointSpec{}
			}
			if err := applyReviewCheckpointField(out.Spec.CompletionReview, key, value); err != nil {
				return AgentSystem{}, fmt.Errorf("spec.completion_review: %w", err)
			}
		}
	}

	if err := out.Normalize(); err != nil {
		return AgentSystem{}, err
	}
	return out, nil
}

func applyReviewCheckpointField(spec *ReviewCheckpointSpec, key, value string) error {
	if spec == nil {
		return nil
	}
	switch key {
	case "checkpoint_id", "checkpointId":
		spec.CheckpointID = value
	case "display_name", "displayName":
		spec.DisplayName = value
	case "reason":
		spec.Reason = value
	case "ttl":
		spec.TTL = value
	case "allow_request_changes", "allowRequestChanges":
		parsed := strings.EqualFold(value, "true") || value == "1"
		spec.AllowRequestChanges = &parsed
	case "max_review_cycles", "maxReviewCycles":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid %s value %q", key, value)
		}
		spec.MaxReviewCycles = v
	}
	return nil
}

// ParseToolManifest parses Tool resources from JSON or constrained YAML.
func ParseToolManifest(data []byte) (Tool, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return Tool{}, err
	}
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
		if section == "spec" && subsection == "cli" && indent <= 4 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			runtimeSubsection = ""
		}
		if section == "spec" && subsection == "wasm" && indent <= 4 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			runtimeSubsection = ""
		}
		if section == "spec" && subsection == "a2a" && indent <= 4 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
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
			case "input_schema", "inputSchema":
				if section == "spec" {
					subsection = "input_schema"
					runtimeSubsection = ""
				}
			case "wasm":
				if section == "spec" {
					subsection = "wasm"
					runtimeSubsection = ""
				}
			case "cli":
				if section == "spec" {
					subsection = "cli"
					runtimeSubsection = ""
				}
			case "a2a":
				if section == "spec" {
					subsection = "a2a"
					runtimeSubsection = ""
				}
			case "args":
				if section == "spec" && subsection == "cli" {
					runtimeSubsection = "args"
				}
			case "env":
				if section == "spec" && subsection == "cli" {
					runtimeSubsection = "env"
				}
			case "env_from", "envFrom":
				if section == "spec" && subsection == "cli" {
					runtimeSubsection = "env_from"
				}
			case "resources":
				if section == "spec" && subsection == "cli" {
					runtimeSubsection = "resources"
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

		if section == "spec" && subsection == "cli" && runtimeSubsection == "args" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.Cli.Args = append(out.Spec.Cli.Args, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}

		if section == "spec" && subsection == "cli" && runtimeSubsection == "env_from" && strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			k, v, ok := parseKeyValue(item)
			if ok && (k == "name") {
				out.Spec.Cli.EnvFrom = append(out.Spec.Cli.EnvFrom, ToolCliEnvRef{Name: stripQuotes(v)})
			}
			continue
		}

		if section == "spec" && subsection == "cli" && runtimeSubsection == "env_from" && !strings.HasPrefix(trimmed, "- ") {
			if len(out.Spec.Cli.EnvFrom) > 0 {
				k, v, ok := parseKeyValue(trimmed)
				if ok {
					last := &out.Spec.Cli.EnvFrom[len(out.Spec.Cli.EnvFrom)-1]
					switch k {
					case "name":
						last.Name = stripQuotes(v)
					case "secretRef", "secret_ref":
						last.SecretRef = stripQuotes(v)
					case "key":
						last.Key = stripQuotes(v)
					}
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
				return Tool{}, err
			}
		case section == "spec" && subsection == "" && key == "type":
			out.Spec.Type = value
		case section == "spec" && subsection == "" && key == "description":
			out.Spec.Description = value
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
		case section == "spec" && subsection == "cli" && runtimeSubsection == "env":
			if out.Spec.Cli.Env == nil {
				out.Spec.Cli.Env = make(map[string]string)
			}
			out.Spec.Cli.Env[key] = value
		case section == "spec" && subsection == "cli" && runtimeSubsection == "" && key == "command":
			out.Spec.Cli.Command = value
		case section == "spec" && subsection == "cli" && runtimeSubsection == "" && key == "image":
			out.Spec.Cli.Image = value
		case section == "spec" && subsection == "cli" && runtimeSubsection == "" && key == "network":
			out.Spec.Cli.Network = value
		case section == "spec" && subsection == "cli" && runtimeSubsection == "" && key == "output":
			out.Spec.Cli.Output = value
		case section == "spec" && subsection == "cli" && runtimeSubsection == "" && (key == "working_dir" || key == "workingDir"):
			out.Spec.Cli.WorkingDir = value
		case section == "spec" && subsection == "cli" && runtimeSubsection == "" && (key == "image_pull_secret" || key == "imagePullSecret"):
			out.Spec.Cli.ImagePullSecret = value
		case section == "spec" && subsection == "cli" && runtimeSubsection == "" && (key == "stdin_from_input" || key == "stdinFromInput"):
			out.Spec.Cli.StdinFromInput = strings.EqualFold(value, "true")
		case section == "spec" && subsection == "cli" && runtimeSubsection == "resources" && key == "memory":
			out.Spec.Cli.Resources.Memory = value
		case section == "spec" && subsection == "cli" && runtimeSubsection == "resources" && key == "cpus":
			out.Spec.Cli.Resources.CPUs = value
		case section == "spec" && subsection == "cli" && runtimeSubsection == "resources" && (key == "pids_limit" || key == "pidsLimit"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return Tool{}, fmt.Errorf("invalid spec.cli.resources.pids_limit value %q", value)
			}
			out.Spec.Cli.Resources.PidsLimit = v
		case section == "spec" && subsection == "wasm" && key == "module":
			out.Spec.Wasm.Module = value
		case section == "spec" && subsection == "wasm" && key == "entrypoint":
			out.Spec.Wasm.Entrypoint = value
		case section == "spec" && subsection == "wasm" && (key == "max_memory_bytes" || key == "maxMemoryBytes"):
			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return Tool{}, fmt.Errorf("invalid spec.wasm.max_memory_bytes value %q", value)
			}
			out.Spec.Wasm.MaxMemoryBytes = v
		case section == "spec" && subsection == "wasm" && key == "fuel":
			v, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return Tool{}, fmt.Errorf("invalid spec.wasm.fuel value %q", value)
			}
			out.Spec.Wasm.Fuel = v
		case section == "spec" && subsection == "wasm" && (key == "enable_wasi" || key == "enableWasi"):
			out.Spec.Wasm.EnableWASI = strings.EqualFold(value, "true")
		case section == "spec" && subsection == "wasm" && (key == "image_pull_secret" || key == "imagePullSecret"):
			out.Spec.Wasm.ImagePullSecret = value
		case section == "spec" && subsection == "a2a" && (key == "agent_url" || key == "agentUrl"):
			out.Spec.A2A.AgentURL = value
		case section == "spec" && subsection == "a2a" && (key == "protocol_version" || key == "protocolVersion"):
			out.Spec.A2A.ProtocolVersion = value
		case section == "spec" && subsection == "a2a" && (key == "prefer_streaming" || key == "preferStreaming"):
			out.Spec.A2A.PreferStreaming = strings.EqualFold(value, "true")
		}
	}

	if len(out.Spec.InputSchema) == 0 {
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			ind := leadingSpaces(line)
			if (trimmed == "input_schema:" || trimmed == "inputSchema:") && ind >= 2 {
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
					out.Spec.InputSchema = parseSimpleYAMLMap(blockLines, blockIndent)
				}
				break
			}
		}
	}

	if err := out.Normalize(); err != nil {
		return Tool{}, err
	}
	return out, nil
}

// parseSecretManifestWithoutNormalize decodes JSON or constrained YAML into a Secret without Normalize().
func parseSecretManifestWithoutNormalize(data []byte) (Secret, error) {
	var out Secret
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return Secret{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		return out, nil
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	section := ""
	subsection := ""

	// Literal block scalar state (YAML `key: |` syntax inside stringData/data).
	lbKey := ""        // key whose value is being accumulated
	lbSubsection := "" // "stringData" or "data"
	lbKeyIndent := -1  // indent of the "key: |" line
	lbContentIndent := -1
	var lbLines []string

	flushLiteralBlock := func() {
		if lbKey == "" {
			return
		}
		val := strings.Join(lbLines, "\n")
		if len(val) > 0 {
			val += "\n"
		}
		if lbSubsection == "data" {
			if out.Spec.Data == nil {
				out.Spec.Data = make(map[string]string)
			}
			out.Spec.Data[lbKey] = val
		} else {
			if out.Spec.StringData == nil {
				out.Spec.StringData = make(map[string]string)
			}
			out.Spec.StringData[lbKey] = val
		}
		lbKey = ""
		lbSubsection = ""
		lbKeyIndent = -1
		lbContentIndent = -1
		lbLines = nil
	}

	for _, line := range lines {
		indent := leadingSpaces(line)
		trimmed := strings.TrimSpace(line)

		// While collecting a literal block, accumulate content lines.
		if lbKey != "" {
			if trimmed == "" {
				lbLines = append(lbLines, "")
				continue
			}
			if indent > lbKeyIndent {
				if lbContentIndent < 0 {
					lbContentIndent = indent
				}
				stripped := ""
				if len(line) >= lbContentIndent {
					stripped = line[lbContentIndent:]
				} else {
					stripped = trimmed
				}
				lbLines = append(lbLines, stripped)
				continue
			}
			// Indent dropped back to ≤ key indent — end of literal block.
			flushLiteralBlock()
			// Fall through to process the current line normally.
		}

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

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

		// Detect literal block scalar (`key: |`) inside stringData or data.
		if section == "spec" && (subsection == "stringData" || subsection == "data") && value == "|" {
			lbKey = key
			lbSubsection = subsection
			lbKeyIndent = indent
			lbContentIndent = -1
			lbLines = nil
			continue
		}

		switch {
		// Guard apiVersion/kind to document root (indent==0) so embedded YAML in
		// literal block values (e.g. a kubeconfig with its own kind: Config) cannot
		// overwrite the Orloj resource kind.
		case key == "apiVersion" && indent == 0:
			out.APIVersion = value
		case key == "kind" && indent == 0:
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

	// Flush any literal block that runs to end-of-file.
	flushLiteralBlock()

	return out, nil
}

// ParseSecretManifest parses Secret resources from JSON or constrained YAML.
func ParseSecretManifest(data []byte) (Secret, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return Secret{}, err
	}
	out, err := parseSecretManifestWithoutNormalize(data)
	if err != nil {
		return Secret{}, err
	}
	if err := out.Normalize(); err != nil {
		return Secret{}, err
	}
	return out, nil
}

// ParseMemoryManifest parses Memory resources from JSON or constrained YAML.
func ParseMemoryManifest(data []byte) (Memory, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return Memory{}, err
	}
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
			case "auth":
				if section == "spec" {
					section = "spec.auth"
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
		case section == "spec" && key == "endpoint":
			out.Spec.Endpoint = value
		case section == "spec" && (key == "endpoint_secret_ref" || key == "endpointSecretRef"):
			out.Spec.EndpointSecretRef = value
		case section == "spec.auth" && (key == "secretRef" || key == "secret_ref"):
			out.Spec.Auth.SecretRef = value
		}
	}

	if err := out.Normalize(); err != nil {
		return Memory{}, err
	}
	return out, nil
}

// ParseAgentPolicyManifest parses AgentPolicy resources from JSON or constrained YAML.
func ParseAgentPolicyManifest(data []byte) (AgentPolicy, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return AgentPolicy{}, err
	}
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
			case "target_agents":
				if section == "spec" {
					subsection = "target_agents"
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
		if section == "spec" && subsection == "target_agents" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.TargetAgents = append(out.Spec.TargetAgents, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
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
		case section == "spec" && (key == "max_child_depth" || key == "maxChildDepth"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return AgentPolicy{}, fmt.Errorf("invalid spec.max_child_depth value %q", value)
			}
			out.Spec.MaxChildDepth = v
		case section == "spec" && (key == "max_child_tasks" || key == "maxChildTasks"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return AgentPolicy{}, fmt.Errorf("invalid spec.max_child_tasks value %q", value)
			}
			out.Spec.MaxChildTasks = v
		}
	}

	if err := out.Normalize(); err != nil {
		return AgentPolicy{}, err
	}
	return out, nil
}

// ParseAgentRoleManifest parses AgentRole resources from JSON or constrained YAML.
func ParseAgentRoleManifest(data []byte) (AgentRole, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return AgentRole{}, err
	}
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
	if err := rejectMultiDocumentYAML(data); err != nil {
		return ToolPermission{}, err
	}
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

// ParseToolApprovalManifest parses ToolApproval resources from JSON or constrained YAML.
func ParseToolApprovalManifest(data []byte) (ToolApproval, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return ToolApproval{}, err
	}
	var out ToolApproval
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return ToolApproval{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return ToolApproval{}, err
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
		if section == "spec" && indent <= 2 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			subsection = ""
		}
		if section == "status" && indent <= 2 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
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
			case "status":
				section = "status"
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
				return ToolApproval{}, err
			}
		case section == "spec" && (key == "task_ref" || key == "taskRef"):
			out.Spec.TaskRef = value
		case section == "spec" && key == "tool":
			out.Spec.Tool = value
		case section == "spec" && (key == "operation_class" || key == "operationClass"):
			out.Spec.OperationClass = value
		case section == "spec" && key == "agent":
			out.Spec.Agent = value
		case section == "spec" && key == "input":
			out.Spec.Input = value
		case section == "spec" && key == "reason":
			out.Spec.Reason = value
		case section == "spec" && key == "ttl":
			out.Spec.TTL = value
		case section == "status" && key == "phase":
			out.Status.Phase = value
		}
	}

	if err := out.Normalize(); err != nil {
		return ToolApproval{}, err
	}
	return out, nil
}

// ParseTaskApprovalManifest parses TaskApproval resources from JSON or constrained YAML.
func ParseTaskApprovalManifest(data []byte) (TaskApproval, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return TaskApproval{}, err
	}
	var out TaskApproval
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return TaskApproval{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return TaskApproval{}, err
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
		if (section == "spec" || section == "status") && indent <= 2 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
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
			case "status":
				section = "status"
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
				return TaskApproval{}, err
			}
		case section == "spec":
			switch key {
			case "task_ref", "taskRef":
				out.Spec.TaskRef = value
			case "checkpoint_id", "checkpointId":
				out.Spec.CheckpointID = value
			case "checkpoint_type", "checkpointType":
				out.Spec.CheckpointType = value
			case "agent":
				out.Spec.Agent = value
			case "reason":
				out.Spec.Reason = value
			case "ttl":
				out.Spec.TTL = value
			case "allow_request_changes", "allowRequestChanges":
				parsed := strings.EqualFold(value, "true")
				out.Spec.AllowRequestChanges = &parsed
			case "max_review_cycles", "maxReviewCycles":
				v, err := strconv.Atoi(value)
				if err != nil {
					return TaskApproval{}, fmt.Errorf("invalid spec.max_review_cycles value %q", value)
				}
				out.Spec.MaxReviewCycles = v
			case "review_cycle", "reviewCycle":
				v, err := strconv.Atoi(value)
				if err != nil {
					return TaskApproval{}, fmt.Errorf("invalid spec.review_cycle value %q", value)
				}
				out.Spec.ReviewCycle = v
			case "supersedes":
				out.Spec.Supersedes = value
			case "output":
				out.Spec.Output = value
			case "output_format", "outputFormat":
				out.Spec.OutputFormat = value
			}
		case section == "status":
			switch key {
			case "phase":
				out.Status.Phase = value
			case "decision":
				out.Status.Decision = value
			case "decided_by", "decidedBy":
				out.Status.DecidedBy = value
			case "decided_at", "decidedAt":
				out.Status.DecidedAt = value
			case "comment":
				out.Status.Comment = value
			case "expires_at", "expiresAt":
				out.Status.ExpiresAt = value
			}
		}
	}

	if len(out.Spec.ResumeContext) == 0 || out.Spec.Output == nil {
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			ind := leadingSpaces(line)
			var target *map[string]any
			switch {
			case trimmed == "resume_context:" || trimmed == "resumeContext:":
				target = &out.Spec.ResumeContext
			case trimmed == "output:" && out.Spec.Output == nil:
				target = nil
			default:
				continue
			}

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
			if blockIndent <= 0 || len(blockLines) == 0 {
				continue
			}
			parsed := parseSimpleYAMLMap(blockLines, blockIndent)
			if target != nil && len(*target) == 0 {
				*target = parsed
			} else if trimmed == "output:" && out.Spec.Output == nil {
				out.Spec.Output = parsed
			}
		}
	}

	if err := out.Normalize(); err != nil {
		return TaskApproval{}, err
	}
	return out, nil
}

// ParseTaskManifest parses Task resources from JSON or constrained YAML.
func ParseTaskManifest(data []byte) (Task, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return Task{}, err
	}
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
	if err := rejectMultiDocumentYAML(data); err != nil {
		return TaskSchedule{}, err
	}
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
		if section == "spec" && strings.HasPrefix(subsection, "task_template_") && indent <= 4 && !strings.HasSuffix(trimmed, ":") {
			subsection = "task_template"
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
			case "task_template":
				if section == "spec" {
					subsection = "task_template"
					if out.Spec.TaskTemplate == nil {
						out.Spec.TaskTemplate = &TaskSpec{}
					}
				}
			case "input":
				if section == "spec" && strings.HasPrefix(subsection, "task_template") {
					subsection = "task_template_input"
				}
			case "retry":
				if section == "spec" && strings.HasPrefix(subsection, "task_template") {
					subsection = "task_template_retry"
				}
			case "message_retry":
				if section == "spec" && strings.HasPrefix(subsection, "task_template") {
					subsection = "task_template_message_retry"
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
		case section == "spec" && subsection == "task_template" && key == "system":
			out.Spec.TaskTemplate.System = value
		case section == "spec" && subsection == "task_template" && key == "mode":
			out.Spec.TaskTemplate.Mode = value
		case section == "spec" && subsection == "task_template" && key == "priority":
			out.Spec.TaskTemplate.Priority = value
		case section == "spec" && subsection == "task_template" && (key == "max_turns" || key == "maxTurns"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return TaskSchedule{}, fmt.Errorf("invalid spec.task_template.max_turns value %q", value)
			}
			out.Spec.TaskTemplate.MaxTurns = v
		case section == "spec" && subsection == "task_template_input":
			if out.Spec.TaskTemplate.Input == nil {
				out.Spec.TaskTemplate.Input = make(map[string]string)
			}
			out.Spec.TaskTemplate.Input[key] = value
		case section == "spec" && subsection == "task_template_retry" && (key == "max_attempts" || key == "maxAttempts"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return TaskSchedule{}, fmt.Errorf("invalid spec.task_template.retry.max_attempts value %q", value)
			}
			out.Spec.TaskTemplate.Retry.MaxAttempts = v
		case section == "spec" && subsection == "task_template_retry" && key == "backoff":
			out.Spec.TaskTemplate.Retry.Backoff = value
		case section == "spec" && subsection == "task_template_message_retry" && (key == "max_attempts" || key == "maxAttempts"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return TaskSchedule{}, fmt.Errorf("invalid spec.task_template.message_retry.max_attempts value %q", value)
			}
			out.Spec.TaskTemplate.MessageRetry.MaxAttempts = v
		case section == "spec" && subsection == "task_template_message_retry" && key == "backoff":
			out.Spec.TaskTemplate.MessageRetry.Backoff = value
		case section == "spec" && subsection == "task_template_message_retry" && (key == "max_backoff" || key == "maxBackoff"):
			out.Spec.TaskTemplate.MessageRetry.MaxBackoff = value
		case section == "spec" && subsection == "task_template_message_retry" && key == "jitter":
			out.Spec.TaskTemplate.MessageRetry.Jitter = value
		}
	}

	if err := out.Normalize(); err != nil {
		return TaskSchedule{}, err
	}
	return out, nil
}

// ParseTaskWebhookManifest parses TaskWebhook resources from JSON or constrained YAML.
func ParseTaskWebhookManifest(data []byte) (TaskWebhook, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return TaskWebhook{}, err
	}
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
		if section == "spec" && strings.HasPrefix(subsection, "task_template_") && indent <= 4 && !strings.HasSuffix(trimmed, ":") {
			subsection = "task_template"
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
			case "task_template":
				if section == "spec" {
					subsection = "task_template"
					if out.Spec.TaskTemplate == nil {
						out.Spec.TaskTemplate = &TaskSpec{}
					}
				}
			case "input":
				if section == "spec" && strings.HasPrefix(subsection, "task_template") {
					subsection = "task_template_input"
				}
			case "retry":
				if section == "spec" && strings.HasPrefix(subsection, "task_template") {
					subsection = "task_template_retry"
				}
			case "message_retry":
				if section == "spec" && strings.HasPrefix(subsection, "task_template") {
					subsection = "task_template_message_retry"
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
		case section == "spec" && subsection == "task_template" && key == "system":
			out.Spec.TaskTemplate.System = value
		case section == "spec" && subsection == "task_template" && key == "mode":
			out.Spec.TaskTemplate.Mode = value
		case section == "spec" && subsection == "task_template" && key == "priority":
			out.Spec.TaskTemplate.Priority = value
		case section == "spec" && subsection == "task_template" && (key == "max_turns" || key == "maxTurns"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return TaskWebhook{}, fmt.Errorf("invalid spec.task_template.max_turns value %q", value)
			}
			out.Spec.TaskTemplate.MaxTurns = v
		case section == "spec" && subsection == "task_template_input":
			if out.Spec.TaskTemplate.Input == nil {
				out.Spec.TaskTemplate.Input = make(map[string]string)
			}
			out.Spec.TaskTemplate.Input[key] = value
		case section == "spec" && subsection == "task_template_retry" && (key == "max_attempts" || key == "maxAttempts"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return TaskWebhook{}, fmt.Errorf("invalid spec.task_template.retry.max_attempts value %q", value)
			}
			out.Spec.TaskTemplate.Retry.MaxAttempts = v
		case section == "spec" && subsection == "task_template_retry" && key == "backoff":
			out.Spec.TaskTemplate.Retry.Backoff = value
		case section == "spec" && subsection == "task_template_message_retry" && (key == "max_attempts" || key == "maxAttempts"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return TaskWebhook{}, fmt.Errorf("invalid spec.task_template.message_retry.max_attempts value %q", value)
			}
			out.Spec.TaskTemplate.MessageRetry.MaxAttempts = v
		case section == "spec" && subsection == "task_template_message_retry" && key == "backoff":
			out.Spec.TaskTemplate.MessageRetry.Backoff = value
		case section == "spec" && subsection == "task_template_message_retry" && (key == "max_backoff" || key == "maxBackoff"):
			out.Spec.TaskTemplate.MessageRetry.MaxBackoff = value
		case section == "spec" && subsection == "task_template_message_retry" && key == "jitter":
			out.Spec.TaskTemplate.MessageRetry.Jitter = value
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
		case section == "spec" && subsection == "auth" && key == "algorithm":
			out.Spec.Auth.Algorithm = value
		case section == "spec" && subsection == "auth" && (key == "payload_format" || key == "payloadFormat"):
			out.Spec.Auth.PayloadFormat = value
		case section == "spec" && subsection == "auth" && (key == "payload_prefix" || key == "payloadPrefix"):
			out.Spec.Auth.PayloadPrefix = value
		case section == "spec" && subsection == "auth" && (key == "payload_separator" || key == "payloadSeparator"):
			out.Spec.Auth.PayloadSeparator = value
		case section == "spec" && subsection == "auth" && (key == "signature_encoding" || key == "signatureEncoding"):
			out.Spec.Auth.SignatureEncoding = value
		case section == "spec" && subsection == "auth" && (key == "header_format" || key == "headerFormat"):
			out.Spec.Auth.HeaderFormat = value
		case section == "spec" && subsection == "auth" && (key == "signature_key" || key == "signatureKey"):
			out.Spec.Auth.SignatureKey = value
		case section == "spec" && subsection == "auth" && (key == "timestamp_key" || key == "timestampKey"):
			out.Spec.Auth.TimestampKey = value
		case section == "spec" && subsection == "idempotency" && (key == "event_id_header" || key == "eventIdHeader"):
			out.Spec.Idempotency.EventIDHeader = value
		case section == "spec" && subsection == "idempotency" && (key == "event_id_from_body" || key == "eventIdFromBody"):
			out.Spec.Idempotency.EventIDFromBody = value
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
	if err := rejectMultiDocumentYAML(data); err != nil {
		return Worker{}, err
	}
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

// ParseMcpServerManifest parses McpServer resources from JSON or constrained YAML.
func ParseMcpServerManifest(data []byte) (McpServer, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return McpServer{}, err
	}
	var out McpServer
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return McpServer{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return McpServer{}, err
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
			label := strings.TrimSuffix(trimmed, ":")
			switch label {
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
			case "env":
				if section == "spec" {
					subsection = "env"
				}
			case "auth":
				if section == "spec" {
					subsection = "auth"
				}
			case "scopes":
				if section == "spec" && subsection == "auth" {
					subsection = "auth_scopes"
				}
			case "tool_filter", "toolFilter":
				if section == "spec" {
					subsection = "tool_filter"
				}
			case "include":
				if section == "spec" && subsection == "tool_filter" {
					subsection = "tool_filter_include"
				}
			case "args":
				if section == "spec" {
					subsection = "args"
				}
			case "reconnect":
				if section == "spec" {
					subsection = "reconnect"
				}
			case "resources":
				if section == "spec" {
					subsection = "resources"
				}
			case "default_tool_runtime", "defaultToolRuntime":
				if section == "spec" {
					subsection = "default_tool_runtime"
					if out.Spec.DefaultToolRuntime == nil {
						out.Spec.DefaultToolRuntime = &ToolRuntimePolicy{}
					}
				}
			case "retry":
				if section == "spec" && subsection == "default_tool_runtime" {
					subsection = "default_tool_runtime_retry"
				}
			}
			continue
		}

		if section == "spec" && subsection == "args" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.Args = append(out.Spec.Args, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "auth_scopes" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.Auth.Scopes = append(out.Spec.Auth.Scopes, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "tool_filter_include" && strings.HasPrefix(trimmed, "- ") {
			out.Spec.ToolFilter.Include = append(out.Spec.ToolFilter.Include, stripQuotes(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		if section == "spec" && subsection == "env" && strings.HasPrefix(trimmed, "- ") {
			entry := McpServerEnvVar{}
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if k, v, ok := parseKeyValue(rest); ok {
				if k == "name" {
					entry.Name = stripQuotes(v)
				}
			}
			out.Spec.Env = append(out.Spec.Env, entry)
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
				return McpServer{}, err
			}
		case section == "spec" && subsection == "" && key == "transport":
			out.Spec.Transport = value
		case section == "spec" && subsection == "" && key == "command":
			out.Spec.Command = value
		case section == "spec" && subsection == "" && key == "endpoint":
			out.Spec.Endpoint = value
		case section == "spec" && subsection == "" && key == "image":
			out.Spec.Image = value
		case section == "spec" && subsection == "" && (key == "image_pull_secret" || key == "imagePullSecret"):
			out.Spec.ImagePullSecret = value
		case section == "spec" && subsection == "" && (key == "idle_timeout" || key == "idleTimeout"):
			out.Spec.IdleTimeout = value
		case section == "spec" && subsection == "" && (key == "allowPrivate" || key == "allow_private"):
			b := strings.EqualFold(strings.TrimSpace(value), "true")
			out.Spec.AllowPrivate = &b
		case section == "spec" && subsection == "auth" && (key == "secretRef" || key == "secret_ref"):
			out.Spec.Auth.SecretRef = value
		case section == "spec" && subsection == "auth" && key == "profile":
			out.Spec.Auth.Profile = value
		case section == "spec" && subsection == "auth" && (key == "headerName" || key == "header_name"):
			out.Spec.Auth.HeaderName = value
		case section == "spec" && subsection == "auth" && (key == "tokenURL" || key == "token_url"):
			out.Spec.Auth.TokenURL = value
		case section == "spec" && subsection == "reconnect" && (key == "max_attempts" || key == "maxAttempts"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return McpServer{}, fmt.Errorf("invalid spec.reconnect.max_attempts value %q", value)
			}
			out.Spec.Reconnect.MaxAttempts = v
		case section == "spec" && subsection == "reconnect" && key == "backoff":
			out.Spec.Reconnect.Backoff = value
		case section == "spec" && subsection == "resources" && key == "memory":
			out.Spec.Resources.Memory = value
		case section == "spec" && subsection == "resources" && key == "cpus":
			out.Spec.Resources.CPUs = value
		case section == "spec" && subsection == "resources" && (key == "pids_limit" || key == "pidsLimit"):
			v, err := strconv.Atoi(value)
			if err != nil {
				return McpServer{}, fmt.Errorf("invalid spec.resources.pids_limit value %q", value)
			}
			out.Spec.Resources.PidsLimit = v
		case section == "spec" && subsection == "env" && indent >= 6:
			if len(out.Spec.Env) > 0 {
				last := &out.Spec.Env[len(out.Spec.Env)-1]
				switch key {
				case "name":
					last.Name = value
				case "value":
					last.Value = value
				case "secretRef", "secret_ref":
					last.SecretRef = value
				case "mountPath", "mount_path":
					last.MountPath = value
				}
			}
		case section == "spec" && subsection == "default_tool_runtime":
			if out.Spec.DefaultToolRuntime == nil {
				out.Spec.DefaultToolRuntime = &ToolRuntimePolicy{}
			}
			switch key {
			case "timeout":
				out.Spec.DefaultToolRuntime.Timeout = value
			case "isolation_mode", "isolationMode":
				out.Spec.DefaultToolRuntime.IsolationMode = value
			}
		case section == "spec" && subsection == "default_tool_runtime_retry":
			if out.Spec.DefaultToolRuntime == nil {
				out.Spec.DefaultToolRuntime = &ToolRuntimePolicy{}
			}
			switch key {
			case "max_attempts", "maxAttempts":
				v, _ := strconv.Atoi(value)
				out.Spec.DefaultToolRuntime.Retry.MaxAttempts = v
			case "backoff":
				out.Spec.DefaultToolRuntime.Retry.Backoff = value
			case "max_backoff", "maxBackoff":
				out.Spec.DefaultToolRuntime.Retry.MaxBackoff = value
			case "jitter":
				out.Spec.DefaultToolRuntime.Retry.Jitter = value
			}
		}
	}

	if err := out.Normalize(); err != nil {
		return McpServer{}, err
	}
	return out, nil
}
