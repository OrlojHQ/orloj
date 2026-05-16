package resources

import (
	"encoding/json"
	"fmt"
	"strings"
)

const DefaultNamespace = "default"

// TypeMeta mirrors Kubernetes-style resource identity fields.
type TypeMeta struct {
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       string `json:"kind" yaml:"kind"`
}

// ObjectMeta stores metadata for a resource.
type ObjectMeta struct {
	Name            string            `json:"name" yaml:"name"`
	Namespace       string            `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Labels          map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	ResourceVersion string            `json:"resourceVersion,omitempty" yaml:"resourceVersion,omitempty"`
	Generation      int64             `json:"generation,omitempty" yaml:"generation,omitempty"`
	CreatedAt       string            `json:"createdAt,omitempty" yaml:"createdAt,omitempty"`
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

// reservedSubresourceSuffixes are path segments that collide with API sub-
// resource routes (e.g. /agents/{name}/status).
var reservedSubresourceSuffixes = []string{"status", "logs", "exec", "scale", "proxy", "binding"}

// ValidateMetadataName checks that a resource name is safe for use as a URL
// path segment and as a store key component. It rejects empty names, names
// containing '/', whitespace, or reserved subresource suffixes.
func ValidateMetadataName(name string) error {
	if name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if strings.ContainsAny(name, "/ \t\n\r") {
		return fmt.Errorf("metadata.name %q must not contain '/', spaces, or whitespace", name)
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "..") {
		return fmt.Errorf("metadata.name %q must not start with '.'", name)
	}
	lower := strings.ToLower(name)
	for _, suffix := range reservedSubresourceSuffixes {
		if lower == suffix {
			return fmt.Errorf("metadata.name %q is a reserved subresource name", name)
		}
	}
	return nil
}

// ListMeta carries pagination metadata in list responses. Continue holds the
// cursor (last item's name) for the next page; empty means no more results.
type ListMeta struct {
	Continue string `json:"continue,omitempty" yaml:"continue,omitempty"`
}

// Agent represents the desired and observed state for a single agent runtime.
type Agent struct {
	APIVersion string      `json:"apiVersion" yaml:"apiVersion"`
	Kind       string      `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta  `json:"metadata" yaml:"metadata"`
	Spec       AgentSpec   `json:"spec" yaml:"spec"`
	Status     AgentStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// ResolvedModel returns the runtime-resolved model ID. This is populated
// after model ref resolution and should be used instead of reading Spec.Model directly.
func (a *Agent) ResolvedModel() string { return a.Spec.Model }

// AgentList is returned by list API calls.
type AgentList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []Agent `json:"items" yaml:"items"`
}

// AgentSpec defines desired runtime behavior.
type AgentSpec struct {
	// Model stores the resolved model id for runtime execution.
	// WARNING: This field is NOT part of the external Agent API (tagged json:"-").
	// It is populated at runtime from ModelRef resolution. Do not set directly
	// in struct literals — use ModelRef instead. Read via Agent.ResolvedModel().
	Model             string             `json:"-" yaml:"-"`
	ModelRef          string             `json:"model_ref,omitempty" yaml:"model_ref,omitempty"`
	FallbackModelRefs []string           `json:"fallback_model_refs,omitempty" yaml:"fallback_model_refs,omitempty"`
	Prompt            string             `json:"prompt" yaml:"prompt"`
	Tools        []string           `json:"tools,omitempty" yaml:"tools,omitempty"`
	AllowedTools []string           `json:"allowed_tools,omitempty" yaml:"allowed_tools,omitempty"`
	Roles        []string           `json:"roles,omitempty" yaml:"roles,omitempty"`
	Memory       MemorySpec         `json:"memory,omitempty" yaml:"memory,omitempty"`
	Execution    AgentExecutionSpec `json:"execution,omitempty" yaml:"execution,omitempty"`
	Limits       AgentLimits        `json:"limits,omitempty" yaml:"limits,omitempty"`
}

const (
	AgentExecutionProfileDynamic  = "dynamic"
	AgentExecutionProfileContract = "contract"

	AgentDuplicateToolCallPolicyShortCircuit = "short_circuit"
	AgentDuplicateToolCallPolicyDeny         = "deny"

	AgentContractViolationPolicyObserve           = "observe"
	AgentContractViolationPolicyNonRetryableError = "non_retryable_error"

	AgentToolUseBehaviorRunLLMAgain     = "run_llm_again"
	AgentToolUseBehaviorStopOnFirstTool = "stop_on_first_tool"
)

// AgentExecutionSpec configures optional per-agent execution contracts.
type AgentExecutionSpec struct {
	Profile                 string         `json:"profile,omitempty" yaml:"profile,omitempty"`
	ToolSequence            []string       `json:"tool_sequence,omitempty" yaml:"tool_sequence,omitempty"`
	RequiredOutputMarkers   []string       `json:"required_output_markers,omitempty" yaml:"required_output_markers,omitempty"`
	DuplicateToolCallPolicy string         `json:"duplicate_tool_call_policy,omitempty" yaml:"duplicate_tool_call_policy,omitempty"`
	OnContractViolation     string         `json:"on_contract_violation,omitempty" yaml:"on_contract_violation,omitempty"`
	ToolUseBehavior         string         `json:"tool_use_behavior,omitempty" yaml:"tool_use_behavior,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	OutputSchema            map[string]any `json:"output_schema,omitempty" yaml:"output_schema,omitempty"`
}

// MemorySpec configures runtime memory backend.
type MemorySpec struct {
	Ref      string   `json:"ref,omitempty" yaml:"ref,omitempty"`
	Type     string   `json:"type,omitempty" yaml:"type,omitempty"`
	Provider string   `json:"provider,omitempty" yaml:"provider,omitempty"`
	Allow    []string `json:"allow,omitempty" yaml:"allow,omitempty"`
}

// AgentLimits configures execution safety bounds.
type AgentLimits struct {
	MaxSteps int    `json:"max_steps,omitempty" yaml:"max_steps,omitempty"`
	Timeout  string `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// AgentStatus represents current runtime state.
type AgentStatus struct {
	Phase              string `json:"phase,omitempty" yaml:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
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
	if err := ValidateMetadataName(a.Metadata.Name); err != nil {
		return err
	}
	a.Spec.Model = strings.TrimSpace(a.Spec.Model)
	a.Spec.ModelRef = strings.TrimSpace(a.Spec.ModelRef)
	if a.Spec.ModelRef == "" {
		return fmt.Errorf("spec.model_ref is required")
	}
	normalizedFallbacks := make([]string, 0, len(a.Spec.FallbackModelRefs))
	seenFallbacks := make(map[string]struct{}, len(a.Spec.FallbackModelRefs))
	for _, ref := range a.Spec.FallbackModelRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		key := strings.ToLower(ref)
		if _, exists := seenFallbacks[key]; exists {
			continue
		}
		seenFallbacks[key] = struct{}{}
		normalizedFallbacks = append(normalizedFallbacks, ref)
	}
	a.Spec.FallbackModelRefs = normalizedFallbacks
	a.Spec.Memory.Ref = strings.TrimSpace(a.Spec.Memory.Ref)
	a.Spec.Memory.Type = strings.TrimSpace(a.Spec.Memory.Type)
	a.Spec.Memory.Provider = strings.TrimSpace(a.Spec.Memory.Provider)
	normalizedMemoryAllow, err := NormalizeMemoryOperations(a.Spec.Memory.Allow)
	if err != nil {
		return fmt.Errorf("invalid spec.memory.allow: %w", err)
	}
	a.Spec.Memory.Allow = normalizedMemoryAllow
	if len(a.Spec.Memory.Allow) > 0 && a.Spec.Memory.Ref == "" {
		return fmt.Errorf("spec.memory.ref is required when spec.memory.allow is set")
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
	normalizedAllowed := make([]string, 0, len(a.Spec.AllowedTools))
	seenAllowed := make(map[string]struct{}, len(a.Spec.AllowedTools))
	for _, t := range a.Spec.AllowedTools {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if _, exists := seenAllowed[key]; exists {
			continue
		}
		seenAllowed[key] = struct{}{}
		normalizedAllowed = append(normalizedAllowed, t)
	}
	a.Spec.AllowedTools = normalizedAllowed
	a.Spec.Execution.Profile = strings.ToLower(strings.TrimSpace(a.Spec.Execution.Profile))
	if a.Spec.Execution.Profile == "" {
		a.Spec.Execution.Profile = AgentExecutionProfileDynamic
	}
	switch a.Spec.Execution.Profile {
	case AgentExecutionProfileDynamic, AgentExecutionProfileContract:
	default:
		return fmt.Errorf("invalid spec.execution.profile %q", a.Spec.Execution.Profile)
	}

	normalizedSequence := make([]string, 0, len(a.Spec.Execution.ToolSequence))
	seenSequence := make(map[string]struct{}, len(a.Spec.Execution.ToolSequence))
	for _, tool := range a.Spec.Execution.ToolSequence {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		key := strings.ToLower(tool)
		if _, exists := seenSequence[key]; exists {
			continue
		}
		seenSequence[key] = struct{}{}
		normalizedSequence = append(normalizedSequence, tool)
	}
	a.Spec.Execution.ToolSequence = normalizedSequence

	normalizedMarkers := make([]string, 0, len(a.Spec.Execution.RequiredOutputMarkers))
	seenMarkers := make(map[string]struct{}, len(a.Spec.Execution.RequiredOutputMarkers))
	for _, marker := range a.Spec.Execution.RequiredOutputMarkers {
		marker = strings.TrimSpace(marker)
		if marker == "" {
			continue
		}
		if _, exists := seenMarkers[marker]; exists {
			continue
		}
		seenMarkers[marker] = struct{}{}
		normalizedMarkers = append(normalizedMarkers, marker)
	}
	a.Spec.Execution.RequiredOutputMarkers = normalizedMarkers

	a.Spec.Execution.DuplicateToolCallPolicy = strings.ToLower(strings.TrimSpace(a.Spec.Execution.DuplicateToolCallPolicy))
	if a.Spec.Execution.DuplicateToolCallPolicy == "" {
		a.Spec.Execution.DuplicateToolCallPolicy = AgentDuplicateToolCallPolicyShortCircuit
	}
	switch a.Spec.Execution.DuplicateToolCallPolicy {
	case AgentDuplicateToolCallPolicyShortCircuit, AgentDuplicateToolCallPolicyDeny:
	default:
		return fmt.Errorf("invalid spec.execution.duplicate_tool_call_policy %q", a.Spec.Execution.DuplicateToolCallPolicy)
	}

	a.Spec.Execution.OnContractViolation = strings.ToLower(strings.TrimSpace(a.Spec.Execution.OnContractViolation))
	if a.Spec.Execution.OnContractViolation == "" {
		a.Spec.Execution.OnContractViolation = AgentContractViolationPolicyNonRetryableError
	}
	switch a.Spec.Execution.OnContractViolation {
	case AgentContractViolationPolicyObserve, AgentContractViolationPolicyNonRetryableError:
	default:
		return fmt.Errorf("invalid spec.execution.on_contract_violation %q", a.Spec.Execution.OnContractViolation)
	}
	a.Spec.Execution.ToolUseBehavior = strings.ToLower(strings.TrimSpace(a.Spec.Execution.ToolUseBehavior))
	if a.Spec.Execution.ToolUseBehavior == "" {
		a.Spec.Execution.ToolUseBehavior = AgentToolUseBehaviorRunLLMAgain
	}
	switch a.Spec.Execution.ToolUseBehavior {
	case AgentToolUseBehaviorRunLLMAgain, AgentToolUseBehaviorStopOnFirstTool:
	default:
		return fmt.Errorf("invalid spec.execution.tool_use_behavior %q", a.Spec.Execution.ToolUseBehavior)
	}
	if a.Spec.Execution.Profile == AgentExecutionProfileContract && len(a.Spec.Execution.ToolSequence) == 0 {
		return fmt.Errorf("spec.execution.tool_sequence is required when spec.execution.profile=contract")
	}
	if err := validateOutputSchema(a.Spec.Execution.OutputSchema); err != nil {
		return fmt.Errorf("spec.execution.output_schema: %w", err)
	}
	if a.Spec.Limits.MaxSteps <= 0 {
		a.Spec.Limits.MaxSteps = 10
	}
	if a.Status.Phase == "" {
		a.Status.Phase = "Pending"
	}
	return nil
}

const (
	maxOutputSchemaDepth = 10
	maxOutputSchemaSize  = 64 * 1024
)

func validateOutputSchema(schema map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	if _, ok := schema["type"]; !ok {
		return fmt.Errorf("root level must contain a \"type\" key")
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("cannot serialize: %w", err)
	}
	if len(raw) > maxOutputSchemaSize {
		return fmt.Errorf("serialized size %d exceeds limit of %d bytes", len(raw), maxOutputSchemaSize)
	}
	if depth := mapDepth(schema, 0); depth > maxOutputSchemaDepth {
		return fmt.Errorf("nesting depth %d exceeds limit of %d", depth, maxOutputSchemaDepth)
	}
	return nil
}

func mapDepth(m map[string]any, current int) int {
	max := current + 1
	for _, v := range m {
		switch child := v.(type) {
		case map[string]any:
			if d := mapDepth(child, current+1); d > max {
				max = d
			}
		}
	}
	return max
}
