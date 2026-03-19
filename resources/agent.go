package resources

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
	// Model stores the resolved model id for runtime execution and is not part of the external Agent API.
	Model        string             `json:"-"`
	ModelRef     string             `json:"model_ref,omitempty"`
	Prompt       string             `json:"prompt"`
	Tools        []string           `json:"tools,omitempty"`
	AllowedTools []string           `json:"allowed_tools,omitempty"`
	Roles        []string           `json:"roles,omitempty"`
	Memory       MemorySpec         `json:"memory,omitempty"`
	Execution    AgentExecutionSpec `json:"execution,omitempty"`
	Limits       AgentLimits        `json:"limits,omitempty"`
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
	Profile                 string   `json:"profile,omitempty"`
	ToolSequence            []string `json:"tool_sequence,omitempty"`
	RequiredOutputMarkers   []string `json:"required_output_markers,omitempty"`
	DuplicateToolCallPolicy string   `json:"duplicate_tool_call_policy,omitempty"`
	OnContractViolation     string   `json:"on_contract_violation,omitempty"`
	ToolUseBehavior         string   `json:"tool_use_behavior,omitempty"`
}

// MemorySpec configures runtime memory backend.
type MemorySpec struct {
	Ref      string   `json:"ref,omitempty"`
	Type     string   `json:"type,omitempty"`
	Provider string   `json:"provider,omitempty"`
	Allow    []string `json:"allow,omitempty"`
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
	if a.Spec.ModelRef == "" && a.Spec.Model == "" {
		return fmt.Errorf("spec.model_ref is required")
	}
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
	if a.Spec.Limits.MaxSteps <= 0 {
		a.Spec.Limits.MaxSteps = 10
	}
	if a.Status.Phase == "" {
		a.Status.Phase = "Pending"
	}
	return nil
}
