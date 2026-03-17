package resources

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/cronexpr"
)

// AgentSystem defines a multi-agent architecture and execution graph.
type AgentSystem struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   ObjectMeta        `json:"metadata"`
	Spec       AgentSystemSpec   `json:"spec"`
	Status     AgentSystemStatus `json:"status,omitempty"`
}

type AgentSystemSpec struct {
	Agents []string             `json:"agents,omitempty"`
	Graph  map[string]GraphEdge `json:"graph,omitempty"`
}

type GraphEdge struct {
	// Legacy single-hop edge. Preserved for backward compatibility.
	Next string `json:"next,omitempty"`
	// Rich edge list for fan-out and per-edge metadata.
	Edges []GraphRoute `json:"edges,omitempty"`
	// Join semantics for this downstream node.
	Join GraphJoin `json:"join,omitempty"`
}

type GraphRoute struct {
	To string `json:"to,omitempty"`
	// Optional labels used by routing/observability layers.
	Labels map[string]string `json:"labels,omitempty"`
	// Optional policy key/value bag for edge-level controls.
	Policy map[string]string `json:"policy,omitempty"`
}

type GraphJoin struct {
	// Mode: wait_for_all | quorum.
	Mode string `json:"mode,omitempty"`
	// QuorumCount is an absolute minimum number of upstream branches.
	QuorumCount int `json:"quorum_count,omitempty"`
	// QuorumPercent is percentage-based minimum of expected branches (0-100).
	QuorumPercent int `json:"quorum_percent,omitempty"`
	// OnFailure: deadletter | skip | continue_partial.
	OnFailure string `json:"on_failure,omitempty"`
}

type AgentSystemStatus struct {
	Phase              string `json:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

type AgentSystemList struct {
	Items []AgentSystem `json:"items"`
}

func (a *AgentSystem) Normalize() error {
	if a.APIVersion == "" {
		a.APIVersion = "orloj.dev/v1"
	}
	if a.Kind == "" {
		a.Kind = "AgentSystem"
	}
	if !strings.EqualFold(a.Kind, "AgentSystem") {
		return fmt.Errorf("unsupported kind %q for AgentSystem", a.Kind)
	}
	NormalizeObjectMetaNamespace(&a.Metadata)
	if a.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if a.Spec.Graph == nil {
		a.Spec.Graph = make(map[string]GraphEdge)
	}
	for name, node := range a.Spec.Graph {
		node.Next = strings.TrimSpace(node.Next)
		if len(node.Edges) > 0 {
			normalized := make([]GraphRoute, 0, len(node.Edges))
			for _, route := range node.Edges {
				route.To = strings.TrimSpace(route.To)
				if route.To == "" {
					continue
				}
				normalized = append(normalized, route)
			}
			node.Edges = normalized
		}
		a.Spec.Graph[name] = node
	}
	if a.Status.Phase == "" {
		a.Status.Phase = "Pending"
	}
	return nil
}

// Tool defines an external capability that agents can call.
type Tool struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       ToolSpec   `json:"spec"`
	Status     ToolStatus `json:"status,omitempty"`
}

type ToolSpec struct {
	Type             string            `json:"type,omitempty"`
	Endpoint         string            `json:"endpoint,omitempty"`
	Capabilities     []string          `json:"capabilities,omitempty"`
	OperationClasses []string          `json:"operation_classes,omitempty"`
	RiskLevel        string            `json:"risk_level,omitempty"`
	Runtime          ToolRuntimePolicy `json:"runtime,omitempty"`
	Auth             ToolAuth          `json:"auth,omitempty"`
}

type ToolAuth struct {
	Profile    string   `json:"profile,omitempty"`
	SecretRef  string   `json:"secretRef,omitempty"`
	HeaderName string   `json:"headerName,omitempty"`
	TokenURL   string   `json:"tokenURL,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
}

// Secret stores sensitive values for runtime tool auth.
// Data values are base64-encoded (Kubernetes style).
type Secret struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   ObjectMeta   `json:"metadata"`
	Spec       SecretSpec   `json:"spec"`
	Status     SecretStatus `json:"status,omitempty"`
}

type SecretSpec struct {
	Data       map[string]string `json:"data,omitempty"`
	StringData map[string]string `json:"stringData,omitempty"`
}

type SecretStatus struct {
	Phase              string `json:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

type SecretList struct {
	Items []Secret `json:"items"`
}

type ToolRuntimePolicy struct {
	Timeout       string          `json:"timeout,omitempty"`
	IsolationMode string          `json:"isolation_mode,omitempty"`
	Retry         ToolRetryPolicy `json:"retry,omitempty"`
}

type ToolRetryPolicy struct {
	MaxAttempts int    `json:"max_attempts,omitempty"`
	Backoff     string `json:"backoff,omitempty"`
	MaxBackoff  string `json:"max_backoff,omitempty"`
	Jitter      string `json:"jitter,omitempty"`
}

type ToolStatus struct {
	Phase              string `json:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

type ToolList struct {
	Items []Tool `json:"items"`
}

func (t *Tool) Normalize() error {
	if t.APIVersion == "" {
		t.APIVersion = "orloj.dev/v1"
	}
	if t.Kind == "" {
		t.Kind = "Tool"
	}
	if !strings.EqualFold(t.Kind, "Tool") {
		return fmt.Errorf("unsupported kind %q for Tool", t.Kind)
	}
	NormalizeObjectMetaNamespace(&t.Metadata)
	if t.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	toolType := strings.ToLower(strings.TrimSpace(t.Spec.Type))
	if toolType == "" {
		toolType = "http"
	}
	switch toolType {
	case "http", "external", "grpc", "queue", "webhook-callback":
		t.Spec.Type = toolType
	default:
		return fmt.Errorf("invalid spec.type %q: expected http, external, grpc, queue, or webhook-callback", t.Spec.Type)
	}
	normalizedCaps := make([]string, 0, len(t.Spec.Capabilities))
	seenCaps := make(map[string]struct{}, len(t.Spec.Capabilities))
	for _, capability := range t.Spec.Capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			continue
		}
		lower := strings.ToLower(capability)
		if _, exists := seenCaps[lower]; exists {
			continue
		}
		seenCaps[lower] = struct{}{}
		normalizedCaps = append(normalizedCaps, capability)
	}
	t.Spec.Capabilities = normalizedCaps

	normalizedOps := make([]string, 0, len(t.Spec.OperationClasses))
	seenOps := make(map[string]struct{}, len(t.Spec.OperationClasses))
	for _, op := range t.Spec.OperationClasses {
		op = strings.ToLower(strings.TrimSpace(op))
		if op == "" {
			continue
		}
		switch op {
		case "read", "write", "delete", "admin":
		default:
			return fmt.Errorf("invalid spec.operation_classes value %q: expected read, write, delete, or admin", op)
		}
		if _, exists := seenOps[op]; exists {
			continue
		}
		seenOps[op] = struct{}{}
		normalizedOps = append(normalizedOps, op)
	}
	t.Spec.OperationClasses = normalizedOps

	risk := strings.ToLower(strings.TrimSpace(t.Spec.RiskLevel))
	if risk == "" {
		risk = "low"
	}
	switch risk {
	case "low", "medium", "high", "critical":
		t.Spec.RiskLevel = risk
	default:
		return fmt.Errorf("invalid spec.risk_level %q: expected low, medium, high, or critical", t.Spec.RiskLevel)
	}

	if len(t.Spec.OperationClasses) == 0 {
		if t.Spec.RiskLevel == "high" || t.Spec.RiskLevel == "critical" {
			t.Spec.OperationClasses = []string{"write"}
		} else {
			t.Spec.OperationClasses = []string{"read"}
		}
	}

	if strings.TrimSpace(t.Spec.Runtime.Timeout) == "" {
		t.Spec.Runtime.Timeout = "30s"
	}
	if _, err := time.ParseDuration(t.Spec.Runtime.Timeout); err != nil {
		return fmt.Errorf("invalid spec.runtime.timeout %q: %w", t.Spec.Runtime.Timeout, err)
	}

	mode := strings.ToLower(strings.TrimSpace(t.Spec.Runtime.IsolationMode))
	if mode == "" {
		if t.Spec.RiskLevel == "high" || t.Spec.RiskLevel == "critical" {
			mode = "sandboxed"
		} else {
			mode = "none"
		}
	}
	switch mode {
	case "none", "sandboxed", "container", "wasm":
		t.Spec.Runtime.IsolationMode = mode
	default:
		return fmt.Errorf("invalid spec.runtime.isolation_mode %q: expected none, sandboxed, container, or wasm", t.Spec.Runtime.IsolationMode)
	}

	if t.Spec.Runtime.Retry.MaxAttempts <= 0 {
		t.Spec.Runtime.Retry.MaxAttempts = 1
	}
	if strings.TrimSpace(t.Spec.Runtime.Retry.Backoff) == "" {
		t.Spec.Runtime.Retry.Backoff = "0s"
	}
	if _, err := time.ParseDuration(t.Spec.Runtime.Retry.Backoff); err != nil {
		return fmt.Errorf("invalid spec.runtime.retry.backoff %q: %w", t.Spec.Runtime.Retry.Backoff, err)
	}
	if strings.TrimSpace(t.Spec.Runtime.Retry.MaxBackoff) == "" {
		t.Spec.Runtime.Retry.MaxBackoff = "30s"
	}
	if _, err := time.ParseDuration(t.Spec.Runtime.Retry.MaxBackoff); err != nil {
		return fmt.Errorf("invalid spec.runtime.retry.max_backoff %q: %w", t.Spec.Runtime.Retry.MaxBackoff, err)
	}
	jitter := strings.ToLower(strings.TrimSpace(t.Spec.Runtime.Retry.Jitter))
	if jitter == "" {
		jitter = "none"
	}
	switch jitter {
	case "none", "full", "equal":
		t.Spec.Runtime.Retry.Jitter = jitter
	default:
		return fmt.Errorf("invalid spec.runtime.retry.jitter %q: expected none, full, or equal", t.Spec.Runtime.Retry.Jitter)
	}
	t.Spec.Auth.SecretRef = strings.TrimSpace(t.Spec.Auth.SecretRef)
	t.Spec.Auth.HeaderName = strings.TrimSpace(t.Spec.Auth.HeaderName)
	t.Spec.Auth.TokenURL = strings.TrimSpace(t.Spec.Auth.TokenURL)
	authProfile := strings.ToLower(strings.TrimSpace(t.Spec.Auth.Profile))
	if authProfile == "" && t.Spec.Auth.SecretRef != "" {
		authProfile = "bearer"
	}
	if authProfile != "" {
		switch authProfile {
		case "bearer", "api_key_header", "basic", "oauth2_client_credentials":
			t.Spec.Auth.Profile = authProfile
		default:
			return fmt.Errorf("invalid spec.auth.profile %q: expected bearer, api_key_header, basic, or oauth2_client_credentials", t.Spec.Auth.Profile)
		}
		if t.Spec.Auth.SecretRef == "" {
			return fmt.Errorf("spec.auth.secretRef is required when auth.profile is set")
		}
		if authProfile == "api_key_header" && t.Spec.Auth.HeaderName == "" {
			return fmt.Errorf("spec.auth.headerName is required when auth.profile is api_key_header")
		}
		if authProfile == "oauth2_client_credentials" && t.Spec.Auth.TokenURL == "" {
			return fmt.Errorf("spec.auth.tokenURL is required when auth.profile is oauth2_client_credentials")
		}
	}
	normalizedScopes := make([]string, 0, len(t.Spec.Auth.Scopes))
	for _, scope := range t.Spec.Auth.Scopes {
		scope = strings.TrimSpace(scope)
		if scope != "" {
			normalizedScopes = append(normalizedScopes, scope)
		}
	}
	t.Spec.Auth.Scopes = normalizedScopes

	if t.Status.Phase == "" {
		t.Status.Phase = "Pending"
	}
	return nil
}

func (s *Secret) Normalize() error {
	if s.APIVersion == "" {
		s.APIVersion = "orloj.dev/v1"
	}
	if s.Kind == "" {
		s.Kind = "Secret"
	}
	if !strings.EqualFold(s.Kind, "Secret") {
		return fmt.Errorf("unsupported kind %q for Secret", s.Kind)
	}
	NormalizeObjectMetaNamespace(&s.Metadata)
	if s.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if s.Spec.Data == nil {
		s.Spec.Data = make(map[string]string)
	}
	for key, value := range s.Spec.StringData {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		s.Spec.Data[key] = base64.StdEncoding.EncodeToString([]byte(value))
	}
	for key, value := range s.Spec.Data {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("spec.data.%s is empty", key)
		}
		if _, err := base64.StdEncoding.DecodeString(value); err != nil {
			return fmt.Errorf("spec.data.%s must be valid base64: %w", key, err)
		}
	}
	// Keep stringData write-only semantics.
	s.Spec.StringData = nil
	if s.Status.Phase == "" {
		s.Status.Phase = "Pending"
	}
	return nil
}

// Memory defines persistent storage configuration for agents.
type Memory struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   ObjectMeta   `json:"metadata"`
	Spec       MemoryConfig `json:"spec"`
	Status     MemoryStatus `json:"status,omitempty"`
}

type MemoryConfig struct {
	Type           string `json:"type,omitempty"`
	Provider       string `json:"provider,omitempty"`
	EmbeddingModel string `json:"embedding_model,omitempty"`
}

type MemoryStatus struct {
	Phase              string `json:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

type MemoryList struct {
	Items []Memory `json:"items"`
}

func (m *Memory) Normalize() error {
	if m.APIVersion == "" {
		m.APIVersion = "orloj.dev/v1"
	}
	if m.Kind == "" {
		m.Kind = "Memory"
	}
	if !strings.EqualFold(m.Kind, "Memory") {
		return fmt.Errorf("unsupported kind %q for Memory", m.Kind)
	}
	NormalizeObjectMetaNamespace(&m.Metadata)
	if m.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if m.Status.Phase == "" {
		m.Status.Phase = "Pending"
	}
	return nil
}

// AgentPolicy defines governance limits for runtime behavior.
type AgentPolicy struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Metadata   ObjectMeta      `json:"metadata"`
	Spec       AgentPolicySpec `json:"spec"`
	Status     PolicyStatus    `json:"status,omitempty"`
}

type AgentPolicySpec struct {
	MaxTokensPerRun int      `json:"max_tokens_per_run,omitempty"`
	AllowedModels   []string `json:"allowed_models,omitempty"`
	BlockedTools    []string `json:"blocked_tools,omitempty"`
	ApplyMode       string   `json:"apply_mode,omitempty"`
	TargetSystems   []string `json:"target_systems,omitempty"`
	TargetTasks     []string `json:"target_tasks,omitempty"`
}

type PolicyStatus struct {
	Phase              string `json:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

type AgentPolicyList struct {
	Items []AgentPolicy `json:"items"`
}

func (p *AgentPolicy) Normalize() error {
	if p.APIVersion == "" {
		p.APIVersion = "orloj.dev/v1"
	}
	if p.Kind == "" {
		p.Kind = "AgentPolicy"
	}
	if !strings.EqualFold(p.Kind, "AgentPolicy") {
		return fmt.Errorf("unsupported kind %q for AgentPolicy", p.Kind)
	}
	NormalizeObjectMetaNamespace(&p.Metadata)
	if p.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if p.Spec.ApplyMode == "" {
		p.Spec.ApplyMode = "scoped"
	}
	mode := strings.ToLower(strings.TrimSpace(p.Spec.ApplyMode))
	if mode != "scoped" && mode != "global" {
		return fmt.Errorf("unsupported spec.apply_mode %q: expected scoped or global", p.Spec.ApplyMode)
	}
	p.Spec.ApplyMode = mode
	if p.Status.Phase == "" {
		p.Status.Phase = "Pending"
	}
	return nil
}

// AgentRole defines reusable permission grants that can be bound to agents.
type AgentRole struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Metadata   ObjectMeta      `json:"metadata"`
	Spec       AgentRoleSpec   `json:"spec"`
	Status     AgentRoleStatus `json:"status,omitempty"`
}

type AgentRoleSpec struct {
	Description string   `json:"description,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

type AgentRoleStatus struct {
	Phase              string `json:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

type AgentRoleList struct {
	Items []AgentRole `json:"items"`
}

func (r *AgentRole) Normalize() error {
	if r.APIVersion == "" {
		r.APIVersion = "orloj.dev/v1"
	}
	if r.Kind == "" {
		r.Kind = "AgentRole"
	}
	if !strings.EqualFold(r.Kind, "AgentRole") {
		return fmt.Errorf("unsupported kind %q for AgentRole", r.Kind)
	}
	NormalizeObjectMetaNamespace(&r.Metadata)
	if r.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	normalized := make([]string, 0, len(r.Spec.Permissions))
	seen := make(map[string]struct{}, len(r.Spec.Permissions))
	for _, permission := range r.Spec.Permissions {
		permission = strings.TrimSpace(permission)
		if permission == "" {
			continue
		}
		key := strings.ToLower(permission)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, permission)
	}
	r.Spec.Permissions = normalized
	if r.Status.Phase == "" {
		r.Status.Phase = "Pending"
	}
	return nil
}

// ToolPermission defines required permissions for invoking a tool action.
type ToolPermission struct {
	APIVersion string               `json:"apiVersion"`
	Kind       string               `json:"kind"`
	Metadata   ObjectMeta           `json:"metadata"`
	Spec       ToolPermissionSpec   `json:"spec"`
	Status     ToolPermissionStatus `json:"status,omitempty"`
}

type ToolPermissionSpec struct {
	ToolRef             string          `json:"tool_ref,omitempty"`
	Action              string          `json:"action,omitempty"`
	RequiredPermissions []string        `json:"required_permissions,omitempty"`
	MatchMode           string          `json:"match_mode,omitempty"`
	ApplyMode           string          `json:"apply_mode,omitempty"`
	TargetAgents        []string        `json:"target_agents,omitempty"`
	OperationRules      []OperationRule `json:"operation_rules,omitempty"`
}

type OperationRule struct {
	OperationClass string `json:"operation_class,omitempty"`
	Verdict        string `json:"verdict,omitempty"`
}

type ToolPermissionStatus struct {
	Phase              string `json:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

type ToolPermissionList struct {
	Items []ToolPermission `json:"items"`
}

func (p *ToolPermission) Normalize() error {
	if p.APIVersion == "" {
		p.APIVersion = "orloj.dev/v1"
	}
	if p.Kind == "" {
		p.Kind = "ToolPermission"
	}
	if !strings.EqualFold(p.Kind, "ToolPermission") {
		return fmt.Errorf("unsupported kind %q for ToolPermission", p.Kind)
	}
	NormalizeObjectMetaNamespace(&p.Metadata)
	if p.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	p.Spec.ToolRef = strings.TrimSpace(p.Spec.ToolRef)
	if p.Spec.ToolRef == "" {
		p.Spec.ToolRef = p.Metadata.Name
	}
	if p.Spec.ToolRef == "" {
		return fmt.Errorf("spec.tool_ref is required")
	}

	action := strings.ToLower(strings.TrimSpace(p.Spec.Action))
	if action == "" {
		action = "invoke"
	}
	p.Spec.Action = action

	matchMode := strings.ToLower(strings.TrimSpace(p.Spec.MatchMode))
	if matchMode == "" {
		matchMode = "all"
	}
	switch matchMode {
	case "all", "any":
		p.Spec.MatchMode = matchMode
	default:
		return fmt.Errorf("unsupported spec.match_mode %q: expected all or any", p.Spec.MatchMode)
	}

	applyMode := strings.ToLower(strings.TrimSpace(p.Spec.ApplyMode))
	if applyMode == "" {
		applyMode = "global"
	}
	switch applyMode {
	case "global", "scoped":
		p.Spec.ApplyMode = applyMode
	default:
		return fmt.Errorf("unsupported spec.apply_mode %q: expected global or scoped", p.Spec.ApplyMode)
	}

	normalizedPerms := make([]string, 0, len(p.Spec.RequiredPermissions))
	seenPerms := make(map[string]struct{}, len(p.Spec.RequiredPermissions))
	for _, permission := range p.Spec.RequiredPermissions {
		permission = strings.TrimSpace(permission)
		if permission == "" {
			continue
		}
		key := strings.ToLower(permission)
		if _, exists := seenPerms[key]; exists {
			continue
		}
		seenPerms[key] = struct{}{}
		normalizedPerms = append(normalizedPerms, permission)
	}
	p.Spec.RequiredPermissions = normalizedPerms

	normalizedAgents := make([]string, 0, len(p.Spec.TargetAgents))
	seenAgents := make(map[string]struct{}, len(p.Spec.TargetAgents))
	for _, agent := range p.Spec.TargetAgents {
		agent = strings.TrimSpace(agent)
		if agent == "" {
			continue
		}
		key := strings.ToLower(agent)
		if _, exists := seenAgents[key]; exists {
			continue
		}
		seenAgents[key] = struct{}{}
		normalizedAgents = append(normalizedAgents, agent)
	}
	p.Spec.TargetAgents = normalizedAgents
	if p.Spec.ApplyMode == "scoped" && len(p.Spec.TargetAgents) == 0 {
		return fmt.Errorf("spec.target_agents is required when spec.apply_mode=scoped")
	}

	for i, rule := range p.Spec.OperationRules {
		opClass := strings.ToLower(strings.TrimSpace(rule.OperationClass))
		if opClass == "" {
			opClass = "*"
		}
		switch opClass {
		case "read", "write", "delete", "admin", "*":
			p.Spec.OperationRules[i].OperationClass = opClass
		default:
			return fmt.Errorf("invalid operation_rules[%d].operation_class %q: expected read, write, delete, admin, or *", i, rule.OperationClass)
		}
		verdict := strings.ToLower(strings.TrimSpace(rule.Verdict))
		if verdict == "" {
			verdict = "allow"
		}
		switch verdict {
		case "allow", "deny", "approval_required":
			p.Spec.OperationRules[i].Verdict = verdict
		default:
			return fmt.Errorf("invalid operation_rules[%d].verdict %q: expected allow, deny, or approval_required", i, rule.Verdict)
		}
	}

	if p.Status.Phase == "" {
		p.Status.Phase = "Pending"
	}
	return nil
}

// ToolApproval captures a pending human/system approval request for a tool invocation.
type ToolApproval struct {
	APIVersion string             `json:"apiVersion"`
	Kind       string             `json:"kind"`
	Metadata   ObjectMeta         `json:"metadata"`
	Spec       ToolApprovalSpec   `json:"spec"`
	Status     ToolApprovalStatus `json:"status,omitempty"`
}

type ToolApprovalSpec struct {
	TaskRef        string `json:"task_ref"`
	Tool           string `json:"tool"`
	OperationClass string `json:"operation_class,omitempty"`
	Agent          string `json:"agent,omitempty"`
	Input          string `json:"input,omitempty"`
	Reason         string `json:"reason,omitempty"`
	TTL            string `json:"ttl,omitempty"`
}

type ToolApprovalStatus struct {
	Phase     string `json:"phase,omitempty"`
	Decision  string `json:"decision,omitempty"`
	DecidedBy string `json:"decided_by,omitempty"`
	DecidedAt string `json:"decided_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type ToolApprovalList struct {
	Items []ToolApproval `json:"items"`
}

func (a *ToolApproval) Normalize() error {
	if a.APIVersion == "" {
		a.APIVersion = "orloj.dev/v1"
	}
	if a.Kind == "" {
		a.Kind = "ToolApproval"
	}
	if !strings.EqualFold(a.Kind, "ToolApproval") {
		return fmt.Errorf("unsupported kind %q for ToolApproval", a.Kind)
	}
	NormalizeObjectMetaNamespace(&a.Metadata)
	if a.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	a.Spec.TaskRef = strings.TrimSpace(a.Spec.TaskRef)
	if a.Spec.TaskRef == "" {
		return fmt.Errorf("spec.task_ref is required")
	}
	a.Spec.Tool = strings.TrimSpace(a.Spec.Tool)
	if a.Spec.Tool == "" {
		return fmt.Errorf("spec.tool is required")
	}
	a.Spec.OperationClass = strings.ToLower(strings.TrimSpace(a.Spec.OperationClass))
	a.Spec.Agent = strings.TrimSpace(a.Spec.Agent)
	a.Spec.Reason = strings.TrimSpace(a.Spec.Reason)

	ttl := strings.TrimSpace(a.Spec.TTL)
	if ttl == "" {
		ttl = "10m"
	}
	if _, err := time.ParseDuration(ttl); err != nil {
		return fmt.Errorf("invalid spec.ttl %q: %w", a.Spec.TTL, err)
	}
	a.Spec.TTL = ttl

	phase := strings.TrimSpace(a.Status.Phase)
	if phase == "" {
		phase = "Pending"
	}
	switch phase {
	case "Pending", "Approved", "Denied", "Expired":
		a.Status.Phase = phase
	default:
		return fmt.Errorf("invalid status.phase %q for ToolApproval: expected Pending, Approved, Denied, or Expired", a.Status.Phase)
	}

	if a.Status.Phase == "Pending" && a.Status.ExpiresAt == "" {
		dur, _ := time.ParseDuration(a.Spec.TTL)
		a.Status.ExpiresAt = time.Now().UTC().Add(dur).Format(time.RFC3339)
	}
	return nil
}

// Task defines one execution request routed to an AgentSystem.
type Task struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       TaskSpec   `json:"spec"`
	Status     TaskStatus `json:"status,omitempty"`
}

type TaskSpec struct {
	System       string                 `json:"system,omitempty"`
	Mode         string                 `json:"mode,omitempty"`
	Input        map[string]string      `json:"input,omitempty"`
	Priority     string                 `json:"priority,omitempty"`
	MaxTurns     int                    `json:"max_turns,omitempty"`
	Retry        TaskRetryPolicy        `json:"retry,omitempty"`
	MessageRetry TaskMessageRetryPolicy `json:"message_retry,omitempty"`
	Requirements TaskRequirements       `json:"requirements,omitempty"`
}

type TaskRetryPolicy struct {
	MaxAttempts int    `json:"max_attempts,omitempty"`
	Backoff     string `json:"backoff,omitempty"`
}

type TaskMessageRetryPolicy struct {
	MaxAttempts  int      `json:"max_attempts,omitempty"`
	Backoff      string   `json:"backoff,omitempty"`
	MaxBackoff   string   `json:"max_backoff,omitempty"`
	Jitter       string   `json:"jitter,omitempty"`
	NonRetryable []string `json:"non_retryable,omitempty"`
}

type TaskRequirements struct {
	Region string `json:"region,omitempty"`
	GPU    bool   `json:"gpu,omitempty"`
	Model  string `json:"model,omitempty"`
}

type TaskTraceEvent struct {
	Timestamp           string `json:"timestamp,omitempty"`
	StepID              string `json:"step_id,omitempty"`
	Attempt             int    `json:"attempt,omitempty"`
	Step                int    `json:"step,omitempty"`
	BranchID            string `json:"branch_id,omitempty"`
	Type                string `json:"type,omitempty"`
	Agent               string `json:"agent,omitempty"`
	Tool                string `json:"tool,omitempty"`
	ToolContractVersion string `json:"tool_contract_version,omitempty"`
	ToolRequestID       string `json:"tool_request_id,omitempty"`
	ToolAttempt         int    `json:"tool_attempt,omitempty"`
	ErrorCode           string `json:"error_code,omitempty"`
	ErrorReason         string `json:"error_reason,omitempty"`
	Retryable           *bool  `json:"retryable,omitempty"`
	Message             string `json:"message,omitempty"`
	LatencyMS           int64  `json:"latency_ms,omitempty"`
	Tokens              int    `json:"tokens,omitempty"`
	TokenUsageSource    string `json:"token_usage_source,omitempty"`
	ToolCalls           int    `json:"tool_calls,omitempty"`
	MemoryWrites        int    `json:"memory_writes,omitempty"`
	ToolAuthProfile     string `json:"tool_auth_profile,omitempty"`
	ToolAuthSecretRef   string `json:"tool_auth_secret_ref,omitempty"`
}

type TaskHistoryEvent struct {
	Timestamp string `json:"timestamp,omitempty"`
	Type      string `json:"type,omitempty"`
	Worker    string `json:"worker,omitempty"`
	Message   string `json:"message,omitempty"`
}

type TaskMessage struct {
	Timestamp      string `json:"timestamp,omitempty"`
	MessageID      string `json:"message_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	Attempt        int    `json:"attempt,omitempty"`
	System         string `json:"system,omitempty"`
	FromAgent      string `json:"from_agent,omitempty"`
	ToAgent        string `json:"to_agent,omitempty"`
	BranchID       string `json:"branch_id,omitempty"`
	ParentBranchID string `json:"parent_branch_id,omitempty"`
	Type           string `json:"type,omitempty"`
	Content        string `json:"content,omitempty"`
	TraceID        string `json:"trace_id,omitempty"`
	ParentID       string `json:"parent_id,omitempty"`
	Phase          string `json:"phase,omitempty"`
	Attempts       int    `json:"attempts,omitempty"`
	MaxAttempts    int    `json:"max_attempts,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	Worker         string `json:"worker,omitempty"`
	ProcessedAt    string `json:"processed_at,omitempty"`
	NextAttemptAt  string `json:"next_attempt_at,omitempty"`
}

type TaskMessageIdempotency struct {
	Key       string `json:"key,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	State     string `json:"state,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Worker    string `json:"worker,omitempty"`
}

type TaskJoinSource struct {
	MessageID string `json:"message_id,omitempty"`
	FromAgent string `json:"from_agent,omitempty"`
	BranchID  string `json:"branch_id,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Payload   string `json:"payload,omitempty"`
}

type TaskJoinState struct {
	Attempt        int              `json:"attempt,omitempty"`
	Node           string           `json:"node,omitempty"`
	Mode           string           `json:"mode,omitempty"`
	Expected       int              `json:"expected,omitempty"`
	QuorumRequired int              `json:"quorum_required,omitempty"`
	Activated      bool             `json:"activated,omitempty"`
	ActivatedAt    string           `json:"activated_at,omitempty"`
	ActivatedBy    string           `json:"activated_by,omitempty"`
	Sources        []TaskJoinSource `json:"sources,omitempty"`
}

type TaskStatus struct {
	Phase              string                   `json:"phase,omitempty"`
	LastError          string                   `json:"lastError,omitempty"`
	StartedAt          string                   `json:"startedAt,omitempty"`
	CompletedAt        string                   `json:"completedAt,omitempty"`
	NextAttemptAt      string                   `json:"nextAttemptAt,omitempty"`
	Attempts           int                      `json:"attempts,omitempty"`
	Output             map[string]string        `json:"output,omitempty"`
	AssignedWorker     string                   `json:"assignedWorker,omitempty"`
	ClaimedBy          string                   `json:"claimedBy,omitempty"`
	LeaseUntil         string                   `json:"leaseUntil,omitempty"`
	LastHeartbeat      string                   `json:"lastHeartbeat,omitempty"`
	Trace              []TaskTraceEvent         `json:"trace,omitempty"`
	History            []TaskHistoryEvent       `json:"history,omitempty"`
	Messages           []TaskMessage            `json:"messages,omitempty"`
	MessageIdempotency []TaskMessageIdempotency `json:"message_idempotency,omitempty"`
	JoinStates         []TaskJoinState          `json:"join_states,omitempty"`
	ObservedGeneration int64                    `json:"observedGeneration,omitempty"`
}

type TaskList struct {
	Items []Task `json:"items"`
}

func (t *Task) Normalize() error {
	if t.APIVersion == "" {
		t.APIVersion = "orloj.dev/v1"
	}
	if t.Kind == "" {
		t.Kind = "Task"
	}
	if !strings.EqualFold(t.Kind, "Task") {
		return fmt.Errorf("unsupported kind %q for Task", t.Kind)
	}
	NormalizeObjectMetaNamespace(&t.Metadata)
	if t.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if t.Spec.Input == nil {
		t.Spec.Input = make(map[string]string)
	}
	mode := strings.ToLower(strings.TrimSpace(t.Spec.Mode))
	if mode == "" {
		mode = "run"
	}
	switch mode {
	case "run", "template":
		t.Spec.Mode = mode
	default:
		return fmt.Errorf("invalid spec.mode %q: expected run or template", t.Spec.Mode)
	}
	if t.Spec.Priority == "" {
		t.Spec.Priority = "normal"
	}
	if t.Spec.MaxTurns < 0 {
		return fmt.Errorf("invalid spec.max_turns %d: expected >= 0", t.Spec.MaxTurns)
	}
	if t.Spec.Retry.MaxAttempts <= 0 {
		t.Spec.Retry.MaxAttempts = 1
	}
	if t.Spec.Retry.Backoff == "" {
		t.Spec.Retry.Backoff = "0s"
	}
	if _, err := time.ParseDuration(t.Spec.Retry.Backoff); err != nil {
		return fmt.Errorf("invalid spec.retry.backoff %q: %w", t.Spec.Retry.Backoff, err)
	}
	if t.Spec.MessageRetry.MaxAttempts <= 0 {
		t.Spec.MessageRetry.MaxAttempts = t.Spec.Retry.MaxAttempts
	}
	if t.Spec.MessageRetry.MaxAttempts <= 0 {
		t.Spec.MessageRetry.MaxAttempts = 1
	}
	if strings.TrimSpace(t.Spec.MessageRetry.Backoff) == "" {
		t.Spec.MessageRetry.Backoff = t.Spec.Retry.Backoff
	}
	if strings.TrimSpace(t.Spec.MessageRetry.Backoff) == "" {
		t.Spec.MessageRetry.Backoff = "0s"
	}
	if _, err := time.ParseDuration(t.Spec.MessageRetry.Backoff); err != nil {
		return fmt.Errorf("invalid spec.message_retry.backoff %q: %w", t.Spec.MessageRetry.Backoff, err)
	}
	if strings.TrimSpace(t.Spec.MessageRetry.MaxBackoff) == "" {
		t.Spec.MessageRetry.MaxBackoff = "24h"
	}
	if _, err := time.ParseDuration(t.Spec.MessageRetry.MaxBackoff); err != nil {
		return fmt.Errorf("invalid spec.message_retry.max_backoff %q: %w", t.Spec.MessageRetry.MaxBackoff, err)
	}
	jitter := strings.ToLower(strings.TrimSpace(t.Spec.MessageRetry.Jitter))
	if jitter == "" {
		jitter = "full"
	}
	switch jitter {
	case "none", "full", "equal":
		t.Spec.MessageRetry.Jitter = jitter
	default:
		return fmt.Errorf("invalid spec.message_retry.jitter %q: expected none, full, or equal", t.Spec.MessageRetry.Jitter)
	}
	if t.Status.Phase == "" {
		t.Status.Phase = "Pending"
	}
	if t.Status.Trace == nil {
		t.Status.Trace = make([]TaskTraceEvent, 0)
	}
	if t.Status.History == nil {
		t.Status.History = make([]TaskHistoryEvent, 0)
	}
	if t.Status.Messages == nil {
		t.Status.Messages = make([]TaskMessage, 0)
	}
	if t.Status.MessageIdempotency == nil {
		t.Status.MessageIdempotency = make([]TaskMessageIdempotency, 0)
	}
	if t.Status.JoinStates == nil {
		t.Status.JoinStates = make([]TaskJoinState, 0)
	}
	return nil
}

// TaskSchedule defines recurring task creation from a template task.
type TaskSchedule struct {
	APIVersion string             `json:"apiVersion"`
	Kind       string             `json:"kind"`
	Metadata   ObjectMeta         `json:"metadata"`
	Spec       TaskScheduleSpec   `json:"spec"`
	Status     TaskScheduleStatus `json:"status,omitempty"`
}

type TaskScheduleSpec struct {
	TaskRef                 string `json:"task_ref,omitempty"`
	Schedule                string `json:"schedule,omitempty"`
	TimeZone                string `json:"time_zone,omitempty"`
	Suspend                 bool   `json:"suspend,omitempty"`
	StartingDeadlineSeconds int    `json:"starting_deadline_seconds,omitempty"`
	ConcurrencyPolicy       string `json:"concurrency_policy,omitempty"`
	SuccessfulHistoryLimit  int    `json:"successful_history_limit,omitempty"`
	FailedHistoryLimit      int    `json:"failed_history_limit,omitempty"`
}

type TaskScheduleStatus struct {
	Phase              string   `json:"phase,omitempty"`
	LastError          string   `json:"lastError,omitempty"`
	LastScheduleTime   string   `json:"lastScheduleTime,omitempty"`
	LastSuccessfulTime string   `json:"lastSuccessfulTime,omitempty"`
	NextScheduleTime   string   `json:"nextScheduleTime,omitempty"`
	LastTriggeredTask  string   `json:"lastTriggeredTask,omitempty"`
	ActiveRuns         []string `json:"activeRuns,omitempty"`
	ObservedGeneration int64    `json:"observedGeneration,omitempty"`
}

type TaskScheduleList struct {
	Items []TaskSchedule `json:"items"`
}

func (t *TaskSchedule) Normalize() error {
	if t.APIVersion == "" {
		t.APIVersion = "orloj.dev/v1"
	}
	if t.Kind == "" {
		t.Kind = "TaskSchedule"
	}
	if !strings.EqualFold(t.Kind, "TaskSchedule") {
		return fmt.Errorf("unsupported kind %q for TaskSchedule", t.Kind)
	}
	NormalizeObjectMetaNamespace(&t.Metadata)
	if t.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	t.Spec.TaskRef = strings.TrimSpace(t.Spec.TaskRef)
	if t.Spec.TaskRef == "" {
		return fmt.Errorf("spec.task_ref is required")
	}
	if strings.Contains(t.Spec.TaskRef, "/") {
		parts := strings.SplitN(t.Spec.TaskRef, "/", 2)
		if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("invalid spec.task_ref %q: expected name or namespace/name", t.Spec.TaskRef)
		}
	}

	t.Spec.Schedule = strings.TrimSpace(t.Spec.Schedule)
	if t.Spec.Schedule == "" {
		return fmt.Errorf("spec.schedule is required")
	}
	if _, err := cronexpr.Parse(t.Spec.Schedule); err != nil {
		return fmt.Errorf("invalid spec.schedule %q: %w", t.Spec.Schedule, err)
	}

	t.Spec.TimeZone = strings.TrimSpace(t.Spec.TimeZone)
	if t.Spec.TimeZone == "" {
		t.Spec.TimeZone = "UTC"
	}
	if _, err := time.LoadLocation(t.Spec.TimeZone); err != nil {
		return fmt.Errorf("invalid spec.time_zone %q: %w", t.Spec.TimeZone, err)
	}

	if t.Spec.StartingDeadlineSeconds <= 0 {
		t.Spec.StartingDeadlineSeconds = 300
	}

	policy := strings.ToLower(strings.TrimSpace(t.Spec.ConcurrencyPolicy))
	if policy == "" {
		policy = "forbid"
	}
	switch policy {
	case "forbid":
		t.Spec.ConcurrencyPolicy = policy
	default:
		return fmt.Errorf("invalid spec.concurrency_policy %q: expected forbid", t.Spec.ConcurrencyPolicy)
	}

	if t.Spec.SuccessfulHistoryLimit <= 0 {
		t.Spec.SuccessfulHistoryLimit = 10
	}
	if t.Spec.FailedHistoryLimit <= 0 {
		t.Spec.FailedHistoryLimit = 3
	}

	if t.Status.Phase == "" {
		t.Status.Phase = "Pending"
	}
	if t.Status.ActiveRuns == nil {
		t.Status.ActiveRuns = make([]string, 0)
	}
	return nil
}

// TaskWebhook defines event-driven task creation from inbound webhook deliveries.
type TaskWebhook struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   ObjectMeta        `json:"metadata"`
	Spec       TaskWebhookSpec   `json:"spec"`
	Status     TaskWebhookStatus `json:"status,omitempty"`
}

type TaskWebhookSpec struct {
	TaskRef     string                 `json:"task_ref,omitempty"`
	Suspend     bool                   `json:"suspend,omitempty"`
	Auth        TaskWebhookAuthSpec    `json:"auth,omitempty"`
	Idempotency TaskWebhookIdempotency `json:"idempotency,omitempty"`
	Payload     TaskWebhookPayloadSpec `json:"payload,omitempty"`
}

type TaskWebhookAuthSpec struct {
	Profile         string `json:"profile,omitempty"`
	SecretRef       string `json:"secret_ref,omitempty"`
	SignatureHeader string `json:"signature_header,omitempty"`
	SignaturePrefix string `json:"signature_prefix,omitempty"`
	TimestampHeader string `json:"timestamp_header,omitempty"`
	MaxSkewSeconds  int    `json:"max_skew_seconds,omitempty"`
}

type TaskWebhookIdempotency struct {
	EventIDHeader       string `json:"event_id_header,omitempty"`
	DedupeWindowSeconds int    `json:"dedupe_window_seconds,omitempty"`
}

type TaskWebhookPayloadSpec struct {
	Mode     string `json:"mode,omitempty"`
	InputKey string `json:"input_key,omitempty"`
}

type TaskWebhookStatus struct {
	Phase              string `json:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	EndpointID         string `json:"endpointID,omitempty"`
	EndpointPath       string `json:"endpointPath,omitempty"`
	LastDeliveryTime   string `json:"lastDeliveryTime,omitempty"`
	LastEventID        string `json:"lastEventID,omitempty"`
	LastTriggeredTask  string `json:"lastTriggeredTask,omitempty"`
	AcceptedCount      int64  `json:"acceptedCount,omitempty"`
	DuplicateCount     int64  `json:"duplicateCount,omitempty"`
	RejectedCount      int64  `json:"rejectedCount,omitempty"`
}

type TaskWebhookList struct {
	Items []TaskWebhook `json:"items"`
}

func (t *TaskWebhook) Normalize() error {
	if t.APIVersion == "" {
		t.APIVersion = "orloj.dev/v1"
	}
	if t.Kind == "" {
		t.Kind = "TaskWebhook"
	}
	if !strings.EqualFold(t.Kind, "TaskWebhook") {
		return fmt.Errorf("unsupported kind %q for TaskWebhook", t.Kind)
	}
	NormalizeObjectMetaNamespace(&t.Metadata)
	if t.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	t.Spec.TaskRef = strings.TrimSpace(t.Spec.TaskRef)
	if t.Spec.TaskRef == "" {
		return fmt.Errorf("spec.task_ref is required")
	}
	if strings.Contains(t.Spec.TaskRef, "/") {
		parts := strings.SplitN(t.Spec.TaskRef, "/", 2)
		if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("invalid spec.task_ref %q: expected name or namespace/name", t.Spec.TaskRef)
		}
	}

	t.Spec.Auth.Profile = strings.ToLower(strings.TrimSpace(t.Spec.Auth.Profile))
	if t.Spec.Auth.Profile == "" {
		t.Spec.Auth.Profile = "generic"
	}
	switch t.Spec.Auth.Profile {
	case "generic", "github":
	default:
		return fmt.Errorf("invalid spec.auth.profile %q: expected generic or github", t.Spec.Auth.Profile)
	}

	t.Spec.Auth.SecretRef = strings.TrimSpace(t.Spec.Auth.SecretRef)
	if t.Spec.Auth.SecretRef == "" {
		return fmt.Errorf("spec.auth.secret_ref is required")
	}

	if t.Spec.Auth.Profile == "github" {
		if strings.TrimSpace(t.Spec.Auth.SignatureHeader) == "" {
			t.Spec.Auth.SignatureHeader = "X-Hub-Signature-256"
		}
		if strings.TrimSpace(t.Spec.Auth.SignaturePrefix) == "" {
			t.Spec.Auth.SignaturePrefix = "sha256="
		}
		t.Spec.Auth.TimestampHeader = strings.TrimSpace(t.Spec.Auth.TimestampHeader)
		if t.Spec.Auth.MaxSkewSeconds < 0 {
			return fmt.Errorf("invalid spec.auth.max_skew_seconds %d: expected >= 0", t.Spec.Auth.MaxSkewSeconds)
		}
		if t.Spec.Auth.MaxSkewSeconds == 0 {
			t.Spec.Auth.MaxSkewSeconds = 300
		}
		if strings.TrimSpace(t.Spec.Idempotency.EventIDHeader) == "" {
			t.Spec.Idempotency.EventIDHeader = "X-GitHub-Delivery"
		}
	} else {
		if strings.TrimSpace(t.Spec.Auth.SignatureHeader) == "" {
			t.Spec.Auth.SignatureHeader = "X-Signature"
		}
		if strings.TrimSpace(t.Spec.Auth.SignaturePrefix) == "" {
			t.Spec.Auth.SignaturePrefix = "sha256="
		}
		if strings.TrimSpace(t.Spec.Auth.TimestampHeader) == "" {
			t.Spec.Auth.TimestampHeader = "X-Timestamp"
		}
		if t.Spec.Auth.MaxSkewSeconds < 0 {
			return fmt.Errorf("invalid spec.auth.max_skew_seconds %d: expected >= 0", t.Spec.Auth.MaxSkewSeconds)
		}
		if t.Spec.Auth.MaxSkewSeconds == 0 {
			t.Spec.Auth.MaxSkewSeconds = 300
		}
		if strings.TrimSpace(t.Spec.Idempotency.EventIDHeader) == "" {
			t.Spec.Idempotency.EventIDHeader = "X-Event-Id"
		}
	}
	if strings.TrimSpace(t.Spec.Auth.SignatureHeader) == "" {
		return fmt.Errorf("spec.auth.signature_header is required")
	}
	if strings.TrimSpace(t.Spec.Idempotency.EventIDHeader) == "" {
		return fmt.Errorf("spec.idempotency.event_id_header is required")
	}
	if t.Spec.Idempotency.DedupeWindowSeconds < 0 {
		return fmt.Errorf("invalid spec.idempotency.dedupe_window_seconds %d: expected >= 0", t.Spec.Idempotency.DedupeWindowSeconds)
	}
	if t.Spec.Idempotency.DedupeWindowSeconds == 0 {
		t.Spec.Idempotency.DedupeWindowSeconds = 86400
	}

	t.Spec.Payload.Mode = strings.ToLower(strings.TrimSpace(t.Spec.Payload.Mode))
	if t.Spec.Payload.Mode == "" {
		t.Spec.Payload.Mode = "raw"
	}
	if t.Spec.Payload.Mode != "raw" {
		return fmt.Errorf("invalid spec.payload.mode %q: expected raw", t.Spec.Payload.Mode)
	}
	t.Spec.Payload.InputKey = strings.TrimSpace(t.Spec.Payload.InputKey)
	if t.Spec.Payload.InputKey == "" {
		t.Spec.Payload.InputKey = "webhook_payload"
	}

	if t.Status.Phase == "" {
		t.Status.Phase = "Pending"
	}
	if strings.TrimSpace(t.Status.EndpointID) == "" {
		t.Status.EndpointID = taskWebhookEndpointID(t.Metadata.Namespace, t.Metadata.Name)
	}
	if strings.TrimSpace(t.Status.EndpointPath) == "" {
		t.Status.EndpointPath = "/v1/webhook-deliveries/" + t.Status.EndpointID
	}
	return nil
}

func taskWebhookEndpointID(namespace, name string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(NormalizeNamespace(namespace)) + "/" + strings.TrimSpace(name)))
	return hex.EncodeToString(sum[:12])
}

// Worker defines a runtime worker that executes claimed tasks.
type Worker struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   ObjectMeta   `json:"metadata"`
	Spec       WorkerSpec   `json:"spec"`
	Status     WorkerStatus `json:"status,omitempty"`
}

type WorkerSpec struct {
	Region             string             `json:"region,omitempty"`
	Capabilities       WorkerCapabilities `json:"capabilities,omitempty"`
	MaxConcurrentTasks int                `json:"max_concurrent_tasks,omitempty"`
}

type WorkerCapabilities struct {
	GPU             bool     `json:"gpu,omitempty"`
	SupportedModels []string `json:"supported_models,omitempty"`
}

type WorkerStatus struct {
	Phase              string `json:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty"`
	LastHeartbeat      string `json:"lastHeartbeat,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	CurrentTasks       int    `json:"currentTasks,omitempty"`
}

type WorkerList struct {
	Items []Worker `json:"items"`
}

func (w *Worker) Normalize() error {
	if w.APIVersion == "" {
		w.APIVersion = "orloj.dev/v1"
	}
	if w.Kind == "" {
		w.Kind = "Worker"
	}
	if !strings.EqualFold(w.Kind, "Worker") {
		return fmt.Errorf("unsupported kind %q for Worker", w.Kind)
	}
	NormalizeObjectMetaNamespace(&w.Metadata)
	if w.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if w.Spec.MaxConcurrentTasks <= 0 {
		w.Spec.MaxConcurrentTasks = 1
	}
	if w.Status.Phase == "" {
		w.Status.Phase = "Pending"
	}
	return nil
}
