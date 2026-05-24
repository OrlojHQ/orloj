package resources

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/cronexpr"
)

// normalizeGovernanceConfigPhase sets status for declarative governance resources (policy, role,
// tool permission). They are enforced on read paths and have no controller to move Pending → Ready;
// defaulting empty or legacy Pending to Ready avoids a permanently "stuck" UI phase.
func normalizeGovernanceConfigPhase(phase *string) {
	if phase == nil {
		return
	}
	p := strings.TrimSpace(*phase)
	if p == "" || strings.EqualFold(p, "Pending") {
		*phase = "Ready"
	}
}

// AgentSystem defines a multi-agent architecture and execution graph.
type AgentSystem struct {
	APIVersion string            `json:"apiVersion" yaml:"apiVersion"`
	Kind       string            `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta        `json:"metadata" yaml:"metadata"`
	Spec       AgentSystemSpec   `json:"spec" yaml:"spec"`
	Status     AgentSystemStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type AgentSystemSpec struct {
	Agents           []string              `json:"agents,omitempty" yaml:"agents,omitempty"`
	Graph            map[string]GraphEdge  `json:"graph,omitempty" yaml:"graph,omitempty"`
	CompletionReview *ReviewCheckpointSpec `json:"completion_review,omitempty" yaml:"completion_review,omitempty"`
	ContextAdapter   string                `json:"context_adapter,omitempty" yaml:"context_adapter,omitempty"`
	A2A              AgentSystemA2ASpec    `json:"a2a,omitempty" yaml:"a2a,omitempty"`
}

type AgentSystemA2ASpec struct {
	Enabled bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Auth    string `json:"auth,omitempty" yaml:"auth,omitempty"`
}

// A2AAuthPublic means unauthenticated callers may invoke this system.
const A2AAuthPublic = "public"

// A2AAuthBearer means a valid bearer token is required (default).
const A2AAuthBearer = "bearer"

type GraphEdge struct {
	// Legacy single-hop edge. Preserved for backward compatibility.
	Next string `json:"next,omitempty" yaml:"next,omitempty"`
	// Rich edge list for fan-out and per-edge metadata.
	Edges []GraphRoute `json:"edges,omitempty" yaml:"edges,omitempty"`
	// Join semantics for this downstream node.
	Join GraphJoin `json:"join,omitempty" yaml:"join,omitempty"`
	// Delegates dispatched after the node's first execution. Reports
	// flow back and trigger a review re-execution before edges fire.
	Delegates []GraphRoute `json:"delegates,omitempty" yaml:"delegates,omitempty"`
	// DelegateJoin controls how delegate returns are collected.
	DelegateJoin GraphJoin `json:"delegate_join,omitempty" yaml:"delegate_join,omitempty"`
	// Review gates this node's output behind a human approval checkpoint.
	Review *ReviewCheckpointSpec `json:"review,omitempty" yaml:"review,omitempty"`
}

type GraphRoute struct {
	To string `json:"to,omitempty" yaml:"to,omitempty"`
	// Optional labels used by routing/observability layers.
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	// Optional policy key/value bag for edge-level controls.
	Policy map[string]string `json:"policy,omitempty" yaml:"policy,omitempty"`
	// Optional condition that must match the completing agent's output for this edge to fire.
	Condition *EdgeCondition `json:"condition,omitempty" yaml:"condition,omitempty"`
}

// EdgeCondition defines a predicate evaluated against the completing agent's output
// to determine whether a graph edge should fire. All non-empty fields must match
// (logical AND). Use Default to mark a fallback edge.
type EdgeCondition struct {
	// OutputContains matches if the output contains this string (case-insensitive).
	OutputContains string `json:"output_contains,omitempty" yaml:"output_contains,omitempty"`
	// OutputNotContains matches if the output does NOT contain this string (case-insensitive).
	OutputNotContains string `json:"output_not_contains,omitempty" yaml:"output_not_contains,omitempty"`
	// OutputMatches matches if the output matches this regex pattern.
	OutputMatches string `json:"output_matches,omitempty" yaml:"output_matches,omitempty"`
	// CompiledOutputMatches is the pre-compiled regex from OutputMatches, populated during normalization.
	CompiledOutputMatches *regexp.Regexp `json:"-" yaml:"-"`
	// Default marks this edge as the fallback when no conditional edge matches.
	Default bool `json:"default,omitempty" yaml:"default,omitempty"`

	// OutputJSONPath extracts a value from JSON output using dot-notation (e.g. "$.route").
	// When set, one of the comparison operators (Equals, NotEquals, Contains, GreaterThan,
	// LessThan) must also be set.
	OutputJSONPath string `json:"output_json_path,omitempty" yaml:"output_json_path,omitempty"`
	// Equals matches when the extracted JSON value equals this string.
	Equals string `json:"equals,omitempty" yaml:"equals,omitempty"`
	// NotEquals matches when the extracted JSON value does NOT equal this string.
	NotEquals string `json:"not_equals,omitempty" yaml:"not_equals,omitempty"`
	// Contains matches when the extracted JSON value (string or array) contains this value.
	Contains string `json:"contains,omitempty" yaml:"contains,omitempty"`
	// GreaterThan matches when the extracted numeric JSON value is greater than this threshold.
	GreaterThan string `json:"greater_than,omitempty" yaml:"greater_than,omitempty"`
	// LessThan matches when the extracted numeric JSON value is less than this threshold.
	LessThan string `json:"less_than,omitempty" yaml:"less_than,omitempty"`
}

type GraphJoin struct {
	// Mode: wait_for_all | quorum.
	Mode string `json:"mode,omitempty" yaml:"mode,omitempty"`
	// QuorumCount is an absolute minimum number of upstream branches.
	QuorumCount int `json:"quorum_count,omitempty" yaml:"quorum_count,omitempty"`
	// QuorumPercent is percentage-based minimum of expected branches (0-100).
	QuorumPercent int `json:"quorum_percent,omitempty" yaml:"quorum_percent,omitempty"`
	// OnFailure: deadletter | skip | continue_partial.
	OnFailure string `json:"on_failure,omitempty" yaml:"on_failure,omitempty"`
}

type ReviewCheckpointSpec struct {
	CheckpointID        string `json:"checkpoint_id,omitempty" yaml:"checkpoint_id,omitempty"`
	DisplayName         string `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Reason              string `json:"reason,omitempty" yaml:"reason,omitempty"`
	TTL                 string `json:"ttl,omitempty" yaml:"ttl,omitempty"`
	AllowRequestChanges *bool  `json:"allow_request_changes,omitempty" yaml:"allow_request_changes,omitempty"`
	MaxReviewCycles     int    `json:"max_review_cycles,omitempty" yaml:"max_review_cycles,omitempty"`
}

type AgentSystemStatus struct {
	Phase              string `json:"phase,omitempty" yaml:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
}

type AgentSystemList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []AgentSystem `json:"items" yaml:"items"`
}

func normalizeGraphRoutes(routes []GraphRoute, node, field string) ([]GraphRoute, error) {
	normalized := make([]GraphRoute, 0, len(routes))
	defaultCount := 0
	for _, route := range routes {
		route.To = strings.TrimSpace(route.To)
		if route.To == "" {
			continue
		}
		if route.Condition != nil {
			route.Condition.OutputContains = strings.TrimSpace(route.Condition.OutputContains)
			route.Condition.OutputNotContains = strings.TrimSpace(route.Condition.OutputNotContains)
			route.Condition.OutputMatches = strings.TrimSpace(route.Condition.OutputMatches)
			route.Condition.OutputJSONPath = strings.TrimSpace(route.Condition.OutputJSONPath)
			route.Condition.Equals = strings.TrimSpace(route.Condition.Equals)
			route.Condition.NotEquals = strings.TrimSpace(route.Condition.NotEquals)
			route.Condition.Contains = strings.TrimSpace(route.Condition.Contains)
			route.Condition.GreaterThan = strings.TrimSpace(route.Condition.GreaterThan)
			route.Condition.LessThan = strings.TrimSpace(route.Condition.LessThan)
			if route.Condition.OutputMatches != "" {
				if len(route.Condition.OutputMatches) > 512 {
					return nil, fmt.Errorf("spec.graph.%s.%s[%s].condition.output_matches: pattern exceeds 512 chars", node, field, route.To)
				}
				compiled, err := regexp.Compile(route.Condition.OutputMatches)
				if err != nil {
					return nil, fmt.Errorf("spec.graph.%s.%s[%s].condition.output_matches: invalid regex %q: %w", node, field, route.To, route.Condition.OutputMatches, err)
				}
				route.Condition.CompiledOutputMatches = compiled
			}
			hasJSONOp := route.Condition.Equals != "" || route.Condition.NotEquals != "" ||
				route.Condition.Contains != "" || route.Condition.GreaterThan != "" || route.Condition.LessThan != ""
			if route.Condition.OutputJSONPath != "" {
				if !hasJSONOp {
					return nil, fmt.Errorf("spec.graph.%s.%s[%s].condition: output_json_path requires at least one comparison operator (equals, not_equals, contains, greater_than, less_than)", node, field, route.To)
				}
				if !strings.HasPrefix(route.Condition.OutputJSONPath, "$.") {
					return nil, fmt.Errorf("spec.graph.%s.%s[%s].condition: output_json_path must start with \"$.\"", node, field, route.To)
				}
			}
			if hasJSONOp && route.Condition.OutputJSONPath == "" {
				return nil, fmt.Errorf("spec.graph.%s.%s[%s].condition: comparison operators (equals, not_equals, etc.) require output_json_path", node, field, route.To)
			}
			if route.Condition.GreaterThan != "" {
				if _, err := strconv.ParseFloat(route.Condition.GreaterThan, 64); err != nil {
					return nil, fmt.Errorf("spec.graph.%s.%s[%s].condition.greater_than: must be a valid number: %w", node, field, route.To, err)
				}
			}
			if route.Condition.LessThan != "" {
				if _, err := strconv.ParseFloat(route.Condition.LessThan, 64); err != nil {
					return nil, fmt.Errorf("spec.graph.%s.%s[%s].condition.less_than: must be a valid number: %w", node, field, route.To, err)
				}
			}
			if route.Condition.Default {
				hasStringCond := route.Condition.OutputContains != "" || route.Condition.OutputNotContains != "" || route.Condition.OutputMatches != ""
				if hasStringCond || hasJSONOp || route.Condition.OutputJSONPath != "" {
					return nil, fmt.Errorf("spec.graph.%s.%s[%s].condition: default edge must not have other condition fields", node, field, route.To)
				}
				defaultCount++
			}
		}
		normalized = append(normalized, route)
	}
	if defaultCount > 1 {
		return nil, fmt.Errorf("spec.graph.%s.%s: at most one default edge is allowed, found %d", node, field, defaultCount)
	}
	return normalized, nil
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
	if err := ValidateMetadataName(a.Metadata.Name); err != nil {
		return err
	}
	if a.Spec.Graph == nil {
		a.Spec.Graph = make(map[string]GraphEdge)
	}
	seenCheckpoints := map[string]string{}
	for name, node := range a.Spec.Graph {
		node.Next = strings.TrimSpace(node.Next)
		if len(node.Edges) > 0 {
			normalized, err := normalizeGraphRoutes(node.Edges, name, "edges")
			if err != nil {
				return err
			}
			node.Edges = normalized
		}
		if len(node.Delegates) > 0 {
			normalized, err := normalizeGraphRoutes(node.Delegates, name, "delegates")
			if err != nil {
				return err
			}
			node.Delegates = normalized
		}
		if node.Review != nil {
			if err := normalizeReviewCheckpoint(node.Review, fmt.Sprintf("spec.graph.%s.review", name)); err != nil {
				return err
			}
			checkpointKey := strings.ToLower(strings.TrimSpace(node.Review.CheckpointID))
			if previous, exists := seenCheckpoints[checkpointKey]; exists {
				return fmt.Errorf("checkpoint_id %q must be unique within the system (already used by %s)", node.Review.CheckpointID, previous)
			}
			seenCheckpoints[checkpointKey] = fmt.Sprintf("spec.graph.%s.review", name)
		}
		normalizedJoin, err := NormalizeGraphJoin(node.Join)
		if err != nil {
			return fmt.Errorf("spec.graph.%s.join: %w", name, err)
		}
		node.Join = normalizedJoin
		normalizedDelegateJoin, err := NormalizeGraphJoin(node.DelegateJoin)
		if err != nil {
			return fmt.Errorf("spec.graph.%s.delegate_join: %w", name, err)
		}
		node.DelegateJoin = normalizedDelegateJoin
		a.Spec.Graph[name] = node
	}
	if a.Spec.CompletionReview != nil {
		if err := normalizeReviewCheckpoint(a.Spec.CompletionReview, "spec.completion_review"); err != nil {
			return err
		}
		checkpointKey := strings.ToLower(strings.TrimSpace(a.Spec.CompletionReview.CheckpointID))
		if previous, exists := seenCheckpoints[checkpointKey]; exists {
			return fmt.Errorf("checkpoint_id %q must be unique within the system (already used by %s)", a.Spec.CompletionReview.CheckpointID, previous)
		}
	}

	for name, node := range a.Spec.Graph {
		for _, route := range node.Delegates {
			if route.To == name {
				return fmt.Errorf("spec.graph.%s.delegates[%s]: a node cannot delegate to itself", name, route.To)
			}
		}
	}

	switch strings.ToLower(strings.TrimSpace(a.Spec.A2A.Auth)) {
	case "", A2AAuthBearer:
		a.Spec.A2A.Auth = ""
	case A2AAuthPublic:
		a.Spec.A2A.Auth = A2AAuthPublic
	default:
		return fmt.Errorf("unsupported spec.a2a.auth value %q (expected %q or %q)", a.Spec.A2A.Auth, A2AAuthPublic, A2AAuthBearer)
	}

	if a.Spec.ContextAdapter != "" {
		a.Spec.ContextAdapter = strings.TrimSpace(a.Spec.ContextAdapter)
	}
	if a.Status.Phase == "" {
		a.Status.Phase = "Pending"
	}
	return nil
}

// ContextAdapter configures a task-time hook that sanitizes task input via a Tool before any agent sees it.
type ContextAdapter struct {
	APIVersion string               `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
	Kind       string               `json:"kind,omitempty" yaml:"kind,omitempty"`
	Metadata   ObjectMeta           `json:"metadata" yaml:"metadata"`
	Spec       ContextAdapterSpec   `json:"spec" yaml:"spec"`
	Status     ContextAdapterStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type ContextAdapterSpec struct {
	ToolRef string `json:"tool_ref" yaml:"tool_ref"`
	OnError string `json:"on_error,omitempty" yaml:"on_error,omitempty"`
}

type ContextAdapterStatus struct {
	Phase   string `json:"phase,omitempty" yaml:"phase,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

type ContextAdapterList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []ContextAdapter `json:"items" yaml:"items"`
}

func (c *ContextAdapter) Normalize() error {
	if c.APIVersion == "" {
		c.APIVersion = "orloj.dev/v1"
	}
	if c.Kind == "" {
		c.Kind = "ContextAdapter"
	}
	if !strings.EqualFold(c.Kind, "ContextAdapter") {
		return fmt.Errorf("unsupported kind %q for ContextAdapter", c.Kind)
	}
	NormalizeObjectMetaNamespace(&c.Metadata)
	if err := ValidateMetadataName(c.Metadata.Name); err != nil {
		return err
	}
	c.Spec.ToolRef = strings.TrimSpace(c.Spec.ToolRef)
	if c.Spec.ToolRef == "" {
		return fmt.Errorf("spec.tool_ref is required")
	}
	if c.Spec.OnError == "" {
		c.Spec.OnError = "reject"
	}
	mode := strings.ToLower(strings.TrimSpace(c.Spec.OnError))
	if mode != "reject" && mode != "passthrough" {
		return fmt.Errorf("invalid spec.on_error %q: expected reject or passthrough", c.Spec.OnError)
	}
	c.Spec.OnError = mode
	c.Status.Phase = "Ready"
	return nil
}

func normalizeReviewCheckpoint(spec *ReviewCheckpointSpec, path string) error {
	if spec == nil {
		return nil
	}
	spec.CheckpointID = strings.TrimSpace(spec.CheckpointID)
	if spec.CheckpointID == "" {
		return fmt.Errorf("%s.checkpoint_id is required", path)
	}
	spec.DisplayName = strings.TrimSpace(spec.DisplayName)
	spec.Reason = strings.TrimSpace(spec.Reason)
	ttl := strings.TrimSpace(spec.TTL)
	if ttl == "" {
		ttl = "10m"
	}
	if _, err := time.ParseDuration(ttl); err != nil {
		return fmt.Errorf("invalid %s.ttl %q: %w", path, spec.TTL, err)
	}
	spec.TTL = ttl
	if spec.AllowRequestChanges == nil {
		allowed := true
		spec.AllowRequestChanges = &allowed
	}
	if spec.MaxReviewCycles <= 0 {
		spec.MaxReviewCycles = 3
	}
	return nil
}

// Tool defines an external capability that agents can call.
type Tool struct {
	APIVersion string     `json:"apiVersion" yaml:"apiVersion"`
	Kind       string     `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta `json:"metadata" yaml:"metadata"`
	Spec       ToolSpec   `json:"spec" yaml:"spec"`
	Status     ToolStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type ToolSpec struct {
	Type        string `json:"type,omitempty" yaml:"type,omitempty"`
	Endpoint    string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	InputSchema      map[string]any    `json:"input_schema,omitempty" yaml:"input_schema,omitempty"`
	McpServerRef     string            `json:"mcp_server_ref,omitempty" yaml:"mcp_server_ref,omitempty"`
	McpToolName      string            `json:"mcp_tool_name,omitempty" yaml:"mcp_tool_name,omitempty"`
	Cli              ToolCliSpec       `json:"cli,omitempty" yaml:"cli,omitempty"`
	Wasm             ToolWasmSpec      `json:"wasm,omitempty" yaml:"wasm,omitempty"`
	A2A              ToolA2ASpec       `json:"a2a,omitempty" yaml:"a2a,omitempty"`
	Capabilities     []string          `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	OperationClasses []string          `json:"operation_classes,omitempty" yaml:"operation_classes,omitempty"`
	RiskLevel        string            `json:"risk_level,omitempty" yaml:"risk_level,omitempty"`
	Runtime          ToolRuntimePolicy `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Auth             ToolAuth          `json:"auth,omitempty" yaml:"auth,omitempty"`
}

// ToolWasmSpec configures per-tool WASM module execution.
// Module may be a local path, HTTPS URL, or OCI artifact reference (oci://...).
type ToolWasmSpec struct {
	Module          string `json:"module,omitempty" yaml:"module,omitempty"`
	Entrypoint      string `json:"entrypoint,omitempty" yaml:"entrypoint,omitempty"`
	MaxMemoryBytes  int64  `json:"max_memory_bytes,omitempty" yaml:"max_memory_bytes,omitempty"`
	Fuel            uint64 `json:"fuel,omitempty" yaml:"fuel,omitempty"`
	EnableWASI      bool   `json:"enable_wasi" yaml:"enable_wasi"`
	ImagePullSecret string `json:"image_pull_secret,omitempty" yaml:"image_pull_secret,omitempty"`
}

// ToolA2ASpec configures per-tool A2A remote agent invocation.
type ToolA2ASpec struct {
	AgentURL        string `json:"agent_url,omitempty" yaml:"agent_url,omitempty"`
	ProtocolVersion string `json:"protocol_version,omitempty" yaml:"protocol_version,omitempty"`
	PreferStreaming bool   `json:"prefer_streaming,omitempty" yaml:"prefer_streaming,omitempty"`
}

// ContainerResources defines per-tool or per-McpServer container resource
// overrides. When set, these take precedence over the global
// --tool-container-{memory,cpus,pids-limit} flags.
type ContainerResources struct {
	Memory    string `json:"memory,omitempty" yaml:"memory,omitempty"`
	CPUs      string `json:"cpus,omitempty" yaml:"cpus,omitempty"`
	PidsLimit int    `json:"pids_limit,omitempty" yaml:"pids_limit,omitempty"`
}

// ToolCliSpec defines the configuration for CLI tool invocations.
type ToolCliSpec struct {
	Command         string             `json:"command,omitempty" yaml:"command,omitempty"`
	Args            []string           `json:"args,omitempty" yaml:"args,omitempty"`
	Image           string             `json:"image,omitempty" yaml:"image,omitempty"`
	ImagePullSecret string             `json:"image_pull_secret,omitempty" yaml:"image_pull_secret,omitempty"`
	Network         string             `json:"network,omitempty" yaml:"network,omitempty"`
	StdinFromInput  bool               `json:"stdin_from_input,omitempty" yaml:"stdin_from_input,omitempty"`
	Output          string             `json:"output,omitempty" yaml:"output,omitempty"`
	WorkingDir      string             `json:"working_dir,omitempty" yaml:"working_dir,omitempty"`
	Env             map[string]string  `json:"env,omitempty" yaml:"env,omitempty"`
	EnvFrom         []ToolCliEnvRef    `json:"env_from,omitempty" yaml:"env_from,omitempty"`
	Resources       ContainerResources `json:"resources,omitempty" yaml:"resources,omitempty"`
}

// ToolCliEnvRef maps an Orloj secret to a process environment variable.
type ToolCliEnvRef struct {
	Name      string `json:"name" yaml:"name"`
	SecretRef string `json:"secretRef" yaml:"secretRef"`
	Key       string `json:"key,omitempty" yaml:"key,omitempty"`
}

type ToolAuth struct {
	Profile    string   `json:"profile,omitempty" yaml:"profile,omitempty"`
	SecretRef  string   `json:"secretRef,omitempty" yaml:"secretRef,omitempty"`
	HeaderName string   `json:"headerName,omitempty" yaml:"headerName,omitempty"`
	TokenURL   string   `json:"tokenURL,omitempty" yaml:"tokenURL,omitempty"`
	Scopes     []string `json:"scopes,omitempty" yaml:"scopes,omitempty"`
}

// Secret stores sensitive values for runtime tool auth.
// Data values are base64-encoded (Kubernetes style).
type Secret struct {
	APIVersion string       `json:"apiVersion" yaml:"apiVersion"`
	Kind       string       `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta   `json:"metadata" yaml:"metadata"`
	Spec       SecretSpec   `json:"spec" yaml:"spec"`
	Status     SecretStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type SecretSpec struct {
	Data       map[string]string `json:"data,omitempty" yaml:"data,omitempty"`
	StringData map[string]string `json:"stringData,omitempty" yaml:"stringData,omitempty"`
}

type SecretStatus struct {
	Phase              string `json:"phase,omitempty" yaml:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
}

type SecretList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []Secret `json:"items" yaml:"items"`
}

type ToolRuntimePolicy struct {
	Timeout       string          `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	IsolationMode string          `json:"isolation_mode,omitempty" yaml:"isolation_mode,omitempty"`
	Retry         ToolRetryPolicy `json:"retry,omitempty" yaml:"retry,omitempty"`
}

type ToolRetryPolicy struct {
	MaxAttempts int    `json:"max_attempts,omitempty" yaml:"max_attempts,omitempty"`
	Backoff     string `json:"backoff,omitempty" yaml:"backoff,omitempty"`
	MaxBackoff  string `json:"max_backoff,omitempty" yaml:"max_backoff,omitempty"`
	Jitter      string `json:"jitter,omitempty" yaml:"jitter,omitempty"`
}

type ToolStatus struct {
	Phase              string `json:"phase,omitempty" yaml:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
}

type ToolList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []Tool `json:"items" yaml:"items"`
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
	if err := ValidateMetadataName(t.Metadata.Name); err != nil {
		return err
	}
	toolType := strings.ToLower(strings.TrimSpace(t.Spec.Type))
	if toolType == "" {
		toolType = "http"
	}
	switch toolType {
	case "http", "external", "grpc", "webhook-callback", "mcp", "wasm", "cli", "a2a":
		t.Spec.Type = toolType
	default:
		return fmt.Errorf("invalid spec.type %q: expected http, external, grpc, webhook-callback, mcp, wasm, cli, or a2a", t.Spec.Type)
	}
	t.Spec.McpServerRef = strings.TrimSpace(t.Spec.McpServerRef)
	t.Spec.McpToolName = strings.TrimSpace(t.Spec.McpToolName)
	if toolType == "mcp" {
		if t.Spec.McpServerRef == "" {
			return fmt.Errorf("spec.mcp_server_ref is required when spec.type is mcp")
		}
		if t.Spec.McpToolName == "" {
			return fmt.Errorf("spec.mcp_tool_name is required when spec.type is mcp")
		}
	}
	if toolType == "cli" {
		t.Spec.Cli.Command = strings.TrimSpace(t.Spec.Cli.Command)
		if t.Spec.Cli.Command == "" {
			return fmt.Errorf("spec.cli.command is required when spec.type is cli")
		}
		t.Spec.Cli.Image = strings.TrimSpace(t.Spec.Cli.Image)
		t.Spec.Cli.ImagePullSecret = strings.TrimSpace(t.Spec.Cli.ImagePullSecret)
		t.Spec.Cli.WorkingDir = strings.TrimSpace(t.Spec.Cli.WorkingDir)
		output := strings.ToLower(strings.TrimSpace(t.Spec.Cli.Output))
		if output == "" {
			output = "stdout"
		}
		switch output {
		case "stdout", "stderr", "both":
			t.Spec.Cli.Output = output
		default:
			return fmt.Errorf("invalid spec.cli.output %q: expected stdout, stderr, or both", t.Spec.Cli.Output)
		}
		network := strings.TrimSpace(t.Spec.Cli.Network)
		if network == "" {
			network = "bridge"
		}
		t.Spec.Cli.Network = network
		for i, ref := range t.Spec.Cli.EnvFrom {
			ref.Name = strings.TrimSpace(ref.Name)
			ref.SecretRef = strings.TrimSpace(ref.SecretRef)
			ref.Key = strings.TrimSpace(ref.Key)
			if ref.Name == "" {
				return fmt.Errorf("spec.cli.env_from[%d].name is required", i)
			}
			if ref.SecretRef == "" {
				return fmt.Errorf("spec.cli.env_from[%d].secretRef is required", i)
			}
			t.Spec.Cli.EnvFrom[i] = ref
		}
	}
	if toolType == "wasm" {
		t.Spec.Wasm.Module = strings.TrimSpace(t.Spec.Wasm.Module)
		if t.Spec.Wasm.Module == "" {
			return fmt.Errorf("spec.wasm.module is required when spec.type is wasm")
		}
		t.Spec.Wasm.ImagePullSecret = strings.TrimSpace(t.Spec.Wasm.ImagePullSecret)
		t.Spec.Wasm.Entrypoint = strings.TrimSpace(t.Spec.Wasm.Entrypoint)
		if t.Spec.Wasm.Entrypoint == "" {
			t.Spec.Wasm.Entrypoint = "run"
		}
		if t.Spec.Wasm.MaxMemoryBytes <= 0 {
			t.Spec.Wasm.MaxMemoryBytes = 64 * 1024 * 1024
		}
		if t.Spec.Wasm.Fuel == 0 {
			t.Spec.Wasm.Fuel = 1_000_000
		}
	}
	if toolType == "a2a" {
		t.Spec.A2A.AgentURL = strings.TrimSpace(t.Spec.A2A.AgentURL)
		if t.Spec.A2A.AgentURL == "" {
			return fmt.Errorf("spec.a2a.agent_url is required when spec.type is a2a")
		}
		t.Spec.A2A.ProtocolVersion = strings.TrimSpace(t.Spec.A2A.ProtocolVersion)
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
		if toolType == "cli" {
			mode = "container"
		} else if t.Spec.RiskLevel == "high" || t.Spec.RiskLevel == "critical" {
			mode = "sandboxed"
		} else {
			mode = "none"
		}
	}
	switch mode {
	case "none", "sandboxed", "container", "wasm", "kubernetes":
		t.Spec.Runtime.IsolationMode = mode
	default:
		return fmt.Errorf("invalid spec.runtime.isolation_mode %q: expected none, sandboxed, container, wasm, or kubernetes", t.Spec.Runtime.IsolationMode)
	}
	if toolType == "cli" && mode != "none" && t.Spec.Cli.Image == "" {
		return fmt.Errorf("spec.cli.image is required when spec.type is cli and isolation_mode is not none")
	}
	if t.Spec.Cli.ImagePullSecret != "" && t.Spec.Cli.Image == "" {
		return fmt.Errorf("spec.cli.image is required when spec.cli.image_pull_secret is set")
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
	if toolType == "cli" {
		if t.Spec.Auth.Profile != "" || t.Spec.Auth.SecretRef != "" {
			return fmt.Errorf("spec.auth is not supported for cli tools; use spec.cli.env_from for CLI tool credentials")
		}
	}
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
	if err := ValidateMetadataName(s.Metadata.Name); err != nil {
		return err
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
	APIVersion string       `json:"apiVersion" yaml:"apiVersion"`
	Kind       string       `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta   `json:"metadata" yaml:"metadata"`
	Spec       MemoryConfig `json:"spec" yaml:"spec"`
	Status     MemoryStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type MemoryConfig struct {
	Type              string           `json:"type,omitempty" yaml:"type,omitempty"`
	Provider          string           `json:"provider,omitempty" yaml:"provider,omitempty"`
	EmbeddingModel    string           `json:"embedding_model,omitempty" yaml:"embedding_model,omitempty"`
	Endpoint          string           `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	EndpointSecretRef string           `json:"endpoint_secret_ref,omitempty" yaml:"endpoint_secret_ref,omitempty"`
	Auth              MemoryAuthConfig `json:"auth,omitempty" yaml:"auth,omitempty"`
}

type MemoryAuthConfig struct {
	SecretRef string `json:"secretRef,omitempty" yaml:"secretRef,omitempty"`
}

type MemoryStatus struct {
	Phase              string `json:"phase,omitempty" yaml:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
}

type MemoryList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []Memory `json:"items" yaml:"items"`
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
	if err := ValidateMetadataName(m.Metadata.Name); err != nil {
		return err
	}
	if m.Status.Phase == "" {
		m.Status.Phase = "Pending"
	}
	return nil
}

// AgentPolicy defines governance limits for runtime behavior.
type AgentPolicy struct {
	APIVersion string          `json:"apiVersion" yaml:"apiVersion"`
	Kind       string          `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta      `json:"metadata" yaml:"metadata"`
	Spec       AgentPolicySpec `json:"spec" yaml:"spec"`
	Status     PolicyStatus    `json:"status,omitempty" yaml:"status,omitempty"`
}

type AgentPolicySpec struct {
	MaxTokensPerRun int      `json:"max_tokens_per_run,omitempty" yaml:"max_tokens_per_run,omitempty"`
	AllowedModels   []string `json:"allowed_models,omitempty" yaml:"allowed_models,omitempty"`
	BlockedTools    []string `json:"blocked_tools,omitempty" yaml:"blocked_tools,omitempty"`
	ApplyMode       string   `json:"apply_mode,omitempty" yaml:"apply_mode,omitempty"`
	TargetSystems   []string `json:"target_systems,omitempty" yaml:"target_systems,omitempty"`
	TargetTasks     []string `json:"target_tasks,omitempty" yaml:"target_tasks,omitempty"`
	TargetAgents    []string `json:"target_agents,omitempty" yaml:"target_agents,omitempty"`
	MaxChildDepth   int      `json:"max_child_depth,omitempty" yaml:"max_child_depth,omitempty"`
	MaxChildTasks   int      `json:"max_child_tasks,omitempty" yaml:"max_child_tasks,omitempty"`
}

type PolicyStatus struct {
	Phase              string `json:"phase,omitempty" yaml:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
}

type AgentPolicyList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []AgentPolicy `json:"items" yaml:"items"`
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
	if err := ValidateMetadataName(p.Metadata.Name); err != nil {
		return err
	}
	if p.Spec.ApplyMode == "" {
		p.Spec.ApplyMode = "scoped"
	}
	mode := strings.ToLower(strings.TrimSpace(p.Spec.ApplyMode))
	if mode != "scoped" && mode != "global" {
		return fmt.Errorf("unsupported spec.apply_mode %q: expected scoped or global", p.Spec.ApplyMode)
	}
	p.Spec.ApplyMode = mode
	normalizeGovernanceConfigPhase(&p.Status.Phase)
	return nil
}

// AgentRole defines reusable permission grants that can be bound to agents.
type AgentRole struct {
	APIVersion string          `json:"apiVersion" yaml:"apiVersion"`
	Kind       string          `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta      `json:"metadata" yaml:"metadata"`
	Spec       AgentRoleSpec   `json:"spec" yaml:"spec"`
	Status     AgentRoleStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type AgentRoleSpec struct {
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Permissions []string `json:"permissions,omitempty" yaml:"permissions,omitempty"`
}

type AgentRoleStatus struct {
	Phase              string `json:"phase,omitempty" yaml:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
}

type AgentRoleList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []AgentRole `json:"items" yaml:"items"`
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
	if err := ValidateMetadataName(r.Metadata.Name); err != nil {
		return err
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
	normalizeGovernanceConfigPhase(&r.Status.Phase)
	return nil
}

// ToolPermission defines required permissions for invoking a tool action.
type ToolPermission struct {
	APIVersion string               `json:"apiVersion" yaml:"apiVersion"`
	Kind       string               `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta           `json:"metadata" yaml:"metadata"`
	Spec       ToolPermissionSpec   `json:"spec" yaml:"spec"`
	Status     ToolPermissionStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type ToolPermissionSpec struct {
	ToolRef             string          `json:"tool_ref,omitempty" yaml:"tool_ref,omitempty"`
	Action              string          `json:"action,omitempty" yaml:"action,omitempty"`
	RequiredPermissions []string        `json:"required_permissions,omitempty" yaml:"required_permissions,omitempty"`
	MatchMode           string          `json:"match_mode,omitempty" yaml:"match_mode,omitempty"`
	ApplyMode           string          `json:"apply_mode,omitempty" yaml:"apply_mode,omitempty"`
	TargetAgents        []string        `json:"target_agents,omitempty" yaml:"target_agents,omitempty"`
	OperationRules      []OperationRule `json:"operation_rules,omitempty" yaml:"operation_rules,omitempty"`
}

type OperationRule struct {
	OperationClass string `json:"operation_class,omitempty" yaml:"operation_class,omitempty"`
	Verdict        string `json:"verdict,omitempty" yaml:"verdict,omitempty"`
}

type ToolPermissionStatus struct {
	Phase              string `json:"phase,omitempty" yaml:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
}

type ToolPermissionList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []ToolPermission `json:"items" yaml:"items"`
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
	if err := ValidateMetadataName(p.Metadata.Name); err != nil {
		return err
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

	normalizeGovernanceConfigPhase(&p.Status.Phase)
	return nil
}

// ToolApproval captures a pending human/system approval request for a tool invocation.
type ToolApproval struct {
	APIVersion string             `json:"apiVersion" yaml:"apiVersion"`
	Kind       string             `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta         `json:"metadata" yaml:"metadata"`
	Spec       ToolApprovalSpec   `json:"spec" yaml:"spec"`
	Status     ToolApprovalStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type ToolApprovalSpec struct {
	TaskRef        string `json:"task_ref" yaml:"task_ref"`
	Tool           string `json:"tool" yaml:"tool"`
	OperationClass string `json:"operation_class,omitempty" yaml:"operation_class,omitempty"`
	Agent          string `json:"agent,omitempty" yaml:"agent,omitempty"`
	Input          string `json:"input,omitempty" yaml:"input,omitempty"`
	Reason         string `json:"reason,omitempty" yaml:"reason,omitempty"`
	TTL            string `json:"ttl,omitempty" yaml:"ttl,omitempty"`
}

type ToolApprovalStatus struct {
	Phase     string `json:"phase,omitempty" yaml:"phase,omitempty"`
	Decision  string `json:"decision,omitempty" yaml:"decision,omitempty"`
	DecidedBy string `json:"decided_by,omitempty" yaml:"decided_by,omitempty"`
	DecidedAt string `json:"decided_at,omitempty" yaml:"decided_at,omitempty"`
	Comment   string `json:"comment,omitempty" yaml:"comment,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

type ToolApprovalList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []ToolApproval `json:"items" yaml:"items"`
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
	if err := ValidateMetadataName(a.Metadata.Name); err != nil {
		return err
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
	a.Status.Decision = strings.TrimSpace(a.Status.Decision)
	a.Status.DecidedBy = strings.TrimSpace(a.Status.DecidedBy)
	a.Status.DecidedAt = strings.TrimSpace(a.Status.DecidedAt)
	a.Status.Comment = strings.TrimSpace(a.Status.Comment)
	a.Status.ExpiresAt = strings.TrimSpace(a.Status.ExpiresAt)

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

// TaskApproval captures a pending human review checkpoint for agent or task output.
type TaskApproval struct {
	APIVersion string             `json:"apiVersion" yaml:"apiVersion"`
	Kind       string             `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta         `json:"metadata" yaml:"metadata"`
	Spec       TaskApprovalSpec   `json:"spec" yaml:"spec"`
	Status     TaskApprovalStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type TaskApprovalSpec struct {
	TaskRef             string         `json:"task_ref" yaml:"task_ref"`
	CheckpointID        string         `json:"checkpoint_id" yaml:"checkpoint_id"`
	CheckpointType      string         `json:"checkpoint_type,omitempty" yaml:"checkpoint_type,omitempty"`
	Agent               string         `json:"agent,omitempty" yaml:"agent,omitempty"`
	Reason              string         `json:"reason,omitempty" yaml:"reason,omitempty"`
	TTL                 string         `json:"ttl,omitempty" yaml:"ttl,omitempty"`
	AllowRequestChanges *bool          `json:"allow_request_changes,omitempty" yaml:"allow_request_changes,omitempty"`
	MaxReviewCycles     int            `json:"max_review_cycles,omitempty" yaml:"max_review_cycles,omitempty"`
	ReviewCycle         int            `json:"review_cycle,omitempty" yaml:"review_cycle,omitempty"`
	Supersedes          string         `json:"supersedes,omitempty" yaml:"supersedes,omitempty"`
	Output              any            `json:"output,omitempty" yaml:"output,omitempty"`
	OutputFormat        string         `json:"output_format,omitempty" yaml:"output_format,omitempty"`
	ResumeContext       map[string]any `json:"resume_context,omitempty" yaml:"resume_context,omitempty"`
}

type TaskApprovalResumeMessage struct {
	MessageID      string `json:"message_id,omitempty" yaml:"message_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty" yaml:"idempotency_key,omitempty"`
	TaskID         string `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	Attempt        int    `json:"attempt,omitempty" yaml:"attempt,omitempty"`
	System         string `json:"system,omitempty" yaml:"system,omitempty"`
	Namespace      string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	FromAgent      string `json:"from_agent,omitempty" yaml:"from_agent,omitempty"`
	ToAgent        string `json:"to_agent,omitempty" yaml:"to_agent,omitempty"`
	BranchID       string `json:"branch_id,omitempty" yaml:"branch_id,omitempty"`
	ParentBranchID string `json:"parent_branch_id,omitempty" yaml:"parent_branch_id,omitempty"`
	Type           string `json:"type,omitempty" yaml:"type,omitempty"`
	Payload        string `json:"payload,omitempty" yaml:"payload,omitempty"`
	Timestamp      string `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
	TraceID        string `json:"trace_id,omitempty" yaml:"trace_id,omitempty"`
	ParentID       string `json:"parent_id,omitempty" yaml:"parent_id,omitempty"`
	DelegateOf     string `json:"delegate_of,omitempty" yaml:"delegate_of,omitempty"`
}

type TaskApprovalResumeContext struct {
	Mode              string                      `json:"mode,omitempty" yaml:"mode,omitempty"`
	Action            string                      `json:"action,omitempty" yaml:"action,omitempty"`
	System            string                      `json:"system,omitempty" yaml:"system,omitempty"`
	ProducingAgent    string                      `json:"producing_agent,omitempty" yaml:"producing_agent,omitempty"`
	CurrentMessage    *TaskApprovalResumeMessage  `json:"current_message,omitempty" yaml:"current_message,omitempty"`
	NextMessages      []TaskApprovalResumeMessage `json:"next_messages,omitempty" yaml:"next_messages,omitempty"`
	RerunMessage      *TaskApprovalResumeMessage  `json:"rerun_message,omitempty" yaml:"rerun_message,omitempty"`
	RuntimeInput      map[string]string           `json:"runtime_input,omitempty" yaml:"runtime_input,omitempty"`
	NextRuntimeInput  map[string]string           `json:"next_runtime_input,omitempty" yaml:"next_runtime_input,omitempty"`
	Output            map[string]string           `json:"output,omitempty" yaml:"output,omitempty"`
	CurrentAgentIndex int                         `json:"current_agent_index,omitempty" yaml:"current_agent_index,omitempty"`
	NextAgentIndex    int                         `json:"next_agent_index,omitempty" yaml:"next_agent_index,omitempty"`
}

type TaskApprovalStatus struct {
	Phase     string `json:"phase,omitempty" yaml:"phase,omitempty"`
	Decision  string `json:"decision,omitempty" yaml:"decision,omitempty"`
	DecidedBy string `json:"decided_by,omitempty" yaml:"decided_by,omitempty"`
	DecidedAt string `json:"decided_at,omitempty" yaml:"decided_at,omitempty"`
	Comment   string `json:"comment,omitempty" yaml:"comment,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

func EncodeTaskApprovalResumeContext(ctx TaskApprovalResumeContext) (map[string]any, error) {
	raw, err := json.Marshal(ctx)
	if err != nil {
		return nil, fmt.Errorf("encode resume context: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode resume context: %w", err)
	}
	return out, nil
}

func DecodeTaskApprovalResumeContext(data map[string]any) (TaskApprovalResumeContext, error) {
	if len(data) == 0 {
		return TaskApprovalResumeContext{}, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return TaskApprovalResumeContext{}, err
	}
	var out TaskApprovalResumeContext
	if err := json.Unmarshal(raw, &out); err != nil {
		return TaskApprovalResumeContext{}, err
	}
	return out, nil
}

type TaskApprovalList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []TaskApproval `json:"items" yaml:"items"`
}

func (a *TaskApproval) Normalize() error {
	if a.APIVersion == "" {
		a.APIVersion = "orloj.dev/v1"
	}
	if a.Kind == "" {
		a.Kind = "TaskApproval"
	}
	if !strings.EqualFold(a.Kind, "TaskApproval") {
		return fmt.Errorf("unsupported kind %q for TaskApproval", a.Kind)
	}
	NormalizeObjectMetaNamespace(&a.Metadata)
	if err := ValidateMetadataName(a.Metadata.Name); err != nil {
		return err
	}
	a.Spec.TaskRef = strings.TrimSpace(a.Spec.TaskRef)
	if a.Spec.TaskRef == "" {
		return fmt.Errorf("spec.task_ref is required")
	}
	a.Spec.CheckpointID = strings.TrimSpace(a.Spec.CheckpointID)
	if a.Spec.CheckpointID == "" {
		return fmt.Errorf("spec.checkpoint_id is required")
	}
	checkpointType := strings.ToLower(strings.TrimSpace(a.Spec.CheckpointType))
	if checkpointType == "" {
		checkpointType = "agent_output"
	}
	switch checkpointType {
	case "agent_output", "task_output":
		a.Spec.CheckpointType = checkpointType
	default:
		return fmt.Errorf("invalid spec.checkpoint_type %q: expected agent_output or task_output", a.Spec.CheckpointType)
	}
	a.Spec.Agent = strings.TrimSpace(a.Spec.Agent)
	a.Spec.Reason = strings.TrimSpace(a.Spec.Reason)
	a.Spec.Supersedes = strings.TrimSpace(a.Spec.Supersedes)
	ttl := strings.TrimSpace(a.Spec.TTL)
	if ttl == "" {
		ttl = "10m"
	}
	if _, err := time.ParseDuration(ttl); err != nil {
		return fmt.Errorf("invalid spec.ttl %q: %w", a.Spec.TTL, err)
	}
	a.Spec.TTL = ttl
	if a.Spec.AllowRequestChanges == nil {
		allowed := true
		a.Spec.AllowRequestChanges = &allowed
	}
	if a.Spec.MaxReviewCycles <= 0 {
		a.Spec.MaxReviewCycles = 3
	}
	if a.Spec.ReviewCycle <= 0 {
		a.Spec.ReviewCycle = 1
	}
	if a.Spec.ResumeContext == nil {
		a.Spec.ResumeContext = map[string]any{}
	}
	outputFormat := strings.ToLower(strings.TrimSpace(a.Spec.OutputFormat))
	if outputFormat == "" {
		if _, ok := a.Spec.Output.(string); ok || a.Spec.Output == nil {
			outputFormat = "text"
		} else {
			outputFormat = "json"
		}
	}
	switch outputFormat {
	case "text", "json":
		a.Spec.OutputFormat = outputFormat
	default:
		return fmt.Errorf("invalid spec.output_format %q: expected text or json", a.Spec.OutputFormat)
	}

	phase := strings.TrimSpace(a.Status.Phase)
	if phase == "" {
		phase = "Pending"
	}
	switch phase {
	case "Pending", "Approved", "Denied", "ChangesRequested", "Expired":
		a.Status.Phase = phase
	default:
		return fmt.Errorf("invalid status.phase %q for TaskApproval: expected Pending, Approved, Denied, ChangesRequested, or Expired", a.Status.Phase)
	}
	decision := strings.ToLower(strings.TrimSpace(a.Status.Decision))
	switch decision {
	case "", "approved", "denied", "request_changes":
		a.Status.Decision = decision
	default:
		return fmt.Errorf("invalid status.decision %q for TaskApproval: expected approved, denied, or request_changes", a.Status.Decision)
	}
	a.Status.DecidedBy = strings.TrimSpace(a.Status.DecidedBy)
	a.Status.DecidedAt = strings.TrimSpace(a.Status.DecidedAt)
	a.Status.Comment = strings.TrimSpace(a.Status.Comment)
	a.Status.ExpiresAt = strings.TrimSpace(a.Status.ExpiresAt)
	if a.Status.Phase == "Pending" && a.Status.ExpiresAt == "" {
		dur, _ := time.ParseDuration(a.Spec.TTL)
		a.Status.ExpiresAt = time.Now().UTC().Add(dur).Format(time.RFC3339)
	}
	return nil
}

func TaskApprovalAllowsRequestChanges(a TaskApproval) bool {
	if a.Spec.AllowRequestChanges == nil {
		return true
	}
	return *a.Spec.AllowRequestChanges
}

func TaskApprovalMaxReviewCycles(a TaskApproval) int {
	if a.Spec.MaxReviewCycles <= 0 {
		return 3
	}
	return a.Spec.MaxReviewCycles
}

// Task defines one execution request routed to an AgentSystem.
type Task struct {
	APIVersion string     `json:"apiVersion" yaml:"apiVersion"`
	Kind       string     `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta `json:"metadata" yaml:"metadata"`
	Spec       TaskSpec   `json:"spec" yaml:"spec"`
	Status     TaskStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type TaskSpec struct {
	System       string                 `json:"system,omitempty" yaml:"system,omitempty"`
	Mode         string                 `json:"mode,omitempty" yaml:"mode,omitempty"`
	Input        map[string]string      `json:"input,omitempty" yaml:"input,omitempty"`
	Priority     string                 `json:"priority,omitempty" yaml:"priority,omitempty"`
	MaxTurns     int                    `json:"max_turns,omitempty" yaml:"max_turns,omitempty"`
	Retry        TaskRetryPolicy        `json:"retry,omitempty" yaml:"retry,omitempty"`
	MessageRetry TaskMessageRetryPolicy `json:"message_retry,omitempty" yaml:"message_retry,omitempty"`
	Requirements TaskRequirements       `json:"requirements,omitempty" yaml:"requirements,omitempty"`
}

type TaskRetryPolicy struct {
	MaxAttempts int    `json:"max_attempts,omitempty" yaml:"max_attempts,omitempty"`
	Backoff     string `json:"backoff,omitempty" yaml:"backoff,omitempty"`
}

type TaskMessageRetryPolicy struct {
	MaxAttempts  int      `json:"max_attempts,omitempty" yaml:"max_attempts,omitempty"`
	Backoff      string   `json:"backoff,omitempty" yaml:"backoff,omitempty"`
	MaxBackoff   string   `json:"max_backoff,omitempty" yaml:"max_backoff,omitempty"`
	Jitter       string   `json:"jitter,omitempty" yaml:"jitter,omitempty"`
	NonRetryable []string `json:"non_retryable,omitempty" yaml:"non_retryable,omitempty"`
}

type TaskRequirements struct {
	Region string `json:"region,omitempty" yaml:"region,omitempty"`
	GPU    bool   `json:"gpu,omitempty" yaml:"gpu,omitempty"`
	Model  string `json:"model,omitempty" yaml:"model,omitempty"`
}

type TaskTraceEvent struct {
	Timestamp           string `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
	StepID              string `json:"step_id,omitempty" yaml:"step_id,omitempty"`
	Attempt             int    `json:"attempt,omitempty" yaml:"attempt,omitempty"`
	Step                int    `json:"step,omitempty" yaml:"step,omitempty"`
	BranchID            string `json:"branch_id,omitempty" yaml:"branch_id,omitempty"`
	Type                string `json:"type,omitempty" yaml:"type,omitempty"`
	Agent               string `json:"agent,omitempty" yaml:"agent,omitempty"`
	Tool                string `json:"tool,omitempty" yaml:"tool,omitempty"`
	ToolContractVersion string `json:"tool_contract_version,omitempty" yaml:"tool_contract_version,omitempty"`
	ToolRequestID       string `json:"tool_request_id,omitempty" yaml:"tool_request_id,omitempty"`
	ToolAttempt         int    `json:"tool_attempt,omitempty" yaml:"tool_attempt,omitempty"`
	ErrorCode           string `json:"error_code,omitempty" yaml:"error_code,omitempty"`
	ErrorReason         string `json:"error_reason,omitempty" yaml:"error_reason,omitempty"`
	Retryable           *bool  `json:"retryable,omitempty" yaml:"retryable,omitempty"`
	Message             string `json:"message,omitempty" yaml:"message,omitempty"`
	LatencyMS           int64  `json:"latency_ms,omitempty" yaml:"latency_ms,omitempty"`
	Tokens              int    `json:"tokens,omitempty" yaml:"tokens,omitempty"`
	InputTokens         int    `json:"input_tokens,omitempty" yaml:"input_tokens,omitempty"`
	OutputTokens        int    `json:"output_tokens,omitempty" yaml:"output_tokens,omitempty"`
	TokenUsageSource    string `json:"token_usage_source,omitempty" yaml:"token_usage_source,omitempty"`
	ToolCalls           int    `json:"tool_calls,omitempty" yaml:"tool_calls,omitempty"`
	MemoryWrites        int    `json:"memory_writes,omitempty" yaml:"memory_writes,omitempty"`
	ToolAuthProfile     string `json:"tool_auth_profile,omitempty" yaml:"tool_auth_profile,omitempty"`
	ToolAuthSecretRef   string `json:"tool_auth_secret_ref,omitempty" yaml:"tool_auth_secret_ref,omitempty"`
}

type TaskHistoryEvent struct {
	Timestamp string `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
	Type      string `json:"type,omitempty" yaml:"type,omitempty"`
	Worker    string `json:"worker,omitempty" yaml:"worker,omitempty"`
	Message   string `json:"message,omitempty" yaml:"message,omitempty"`
}

type TaskMessage struct {
	Timestamp      string `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
	MessageID      string `json:"message_id,omitempty" yaml:"message_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty" yaml:"idempotency_key,omitempty"`
	TaskID         string `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	Attempt        int    `json:"attempt,omitempty" yaml:"attempt,omitempty"`
	System         string `json:"system,omitempty" yaml:"system,omitempty"`
	FromAgent      string `json:"from_agent,omitempty" yaml:"from_agent,omitempty"`
	ToAgent        string `json:"to_agent,omitempty" yaml:"to_agent,omitempty"`
	BranchID       string `json:"branch_id,omitempty" yaml:"branch_id,omitempty"`
	ParentBranchID string `json:"parent_branch_id,omitempty" yaml:"parent_branch_id,omitempty"`
	Type           string `json:"type,omitempty" yaml:"type,omitempty"`
	Content        string `json:"content,omitempty" yaml:"content,omitempty"`
	TraceID        string `json:"trace_id,omitempty" yaml:"trace_id,omitempty"`
	ParentID       string `json:"parent_id,omitempty" yaml:"parent_id,omitempty"`
	Phase          string `json:"phase,omitempty" yaml:"phase,omitempty"`
	Attempts       int    `json:"attempts,omitempty" yaml:"attempts,omitempty"`
	MaxAttempts    int    `json:"max_attempts,omitempty" yaml:"max_attempts,omitempty"`
	LastError      string `json:"last_error,omitempty" yaml:"last_error,omitempty"`
	Worker         string `json:"worker,omitempty" yaml:"worker,omitempty"`
	ProcessedAt    string `json:"processed_at,omitempty" yaml:"processed_at,omitempty"`
	NextAttemptAt  string `json:"next_attempt_at,omitempty" yaml:"next_attempt_at,omitempty"`
	DelegateOf     string `json:"delegate_of,omitempty" yaml:"delegate_of,omitempty"`
}

type TaskMessageIdempotency struct {
	Key       string `json:"key,omitempty" yaml:"key,omitempty"`
	MessageID string `json:"message_id,omitempty" yaml:"message_id,omitempty"`
	State     string `json:"state,omitempty" yaml:"state,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	Worker    string `json:"worker,omitempty" yaml:"worker,omitempty"`
}

type TaskJoinSource struct {
	MessageID string `json:"message_id,omitempty" yaml:"message_id,omitempty"`
	FromAgent string `json:"from_agent,omitempty" yaml:"from_agent,omitempty"`
	BranchID  string `json:"branch_id,omitempty" yaml:"branch_id,omitempty"`
	Timestamp string `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
	Payload   string `json:"payload,omitempty" yaml:"payload,omitempty"`
}

type TaskJoinState struct {
	Attempt        int              `json:"attempt,omitempty" yaml:"attempt,omitempty"`
	Node           string           `json:"node,omitempty" yaml:"node,omitempty"`
	Mode           string           `json:"mode,omitempty" yaml:"mode,omitempty"`
	Expected       int              `json:"expected,omitempty" yaml:"expected,omitempty"`
	QuorumRequired int              `json:"quorum_required,omitempty" yaml:"quorum_required,omitempty"`
	Activated      bool             `json:"activated,omitempty" yaml:"activated,omitempty"`
	ActivatedAt    string           `json:"activated_at,omitempty" yaml:"activated_at,omitempty"`
	ActivatedBy    string           `json:"activated_by,omitempty" yaml:"activated_by,omitempty"`
	Sources        []TaskJoinSource `json:"sources,omitempty" yaml:"sources,omitempty"`
}

type TaskDelegationState struct {
	Attempt        int              `json:"attempt,omitempty" yaml:"attempt,omitempty"`
	Node           string           `json:"node,omitempty" yaml:"node,omitempty"`
	Mode           string           `json:"mode,omitempty" yaml:"mode,omitempty"`
	Expected       int              `json:"expected,omitempty" yaml:"expected,omitempty"`
	QuorumRequired int              `json:"quorum_required,omitempty" yaml:"quorum_required,omitempty"`
	Activated      bool             `json:"activated,omitempty" yaml:"activated,omitempty"`
	ActivatedAt    string           `json:"activated_at,omitempty" yaml:"activated_at,omitempty"`
	ActivatedBy    string           `json:"activated_by,omitempty" yaml:"activated_by,omitempty"`
	Sources        []TaskJoinSource `json:"sources,omitempty" yaml:"sources,omitempty"`
}

type TaskBlockedOn struct {
	Kind   string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Name   string `json:"name,omitempty" yaml:"name,omitempty"`
	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type TaskStatus struct {
	Phase              string                   `json:"phase,omitempty" yaml:"phase,omitempty"`
	LastError          string                   `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	StartedAt          string                   `json:"startedAt,omitempty" yaml:"startedAt,omitempty"`
	CompletedAt        string                   `json:"completedAt,omitempty" yaml:"completedAt,omitempty"`
	NextAttemptAt      string                   `json:"nextAttemptAt,omitempty" yaml:"nextAttemptAt,omitempty"`
	Attempts           int                      `json:"attempts,omitempty" yaml:"attempts,omitempty"`
	Output             map[string]string        `json:"output,omitempty" yaml:"output,omitempty"`
	AssignedWorker     string                   `json:"assignedWorker,omitempty" yaml:"assignedWorker,omitempty"`
	ClaimedBy          string                   `json:"claimedBy,omitempty" yaml:"claimedBy,omitempty"`
	LeaseUntil         string                   `json:"leaseUntil,omitempty" yaml:"leaseUntil,omitempty"`
	LastHeartbeat      string                   `json:"lastHeartbeat,omitempty" yaml:"lastHeartbeat,omitempty"`
	Trace              []TaskTraceEvent         `json:"trace,omitempty" yaml:"trace,omitempty"`
	History            []TaskHistoryEvent       `json:"history,omitempty" yaml:"history,omitempty"`
	Messages           []TaskMessage            `json:"messages,omitempty" yaml:"messages,omitempty"`
	MessageIdempotency []TaskMessageIdempotency `json:"message_idempotency,omitempty" yaml:"message_idempotency,omitempty"`
	JoinStates         []TaskJoinState          `json:"join_states,omitempty" yaml:"join_states,omitempty"`
	DelegationStates   []TaskDelegationState    `json:"delegation_states,omitempty" yaml:"delegation_states,omitempty"`
	BlockedOn          *TaskBlockedOn           `json:"blocked_on,omitempty" yaml:"blocked_on,omitempty"`
	ObservedGeneration int64                    `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`

	// Agent Job fields: orchestrator↔pod communication for K8s agent execution.
	AgentJobInput     map[string]string `json:"agentJobInput,omitempty" yaml:"agentJobInput,omitempty"`
	AgentJobAgent     string            `json:"agentJobAgent,omitempty" yaml:"agentJobAgent,omitempty"`
	AgentJobMessageID string            `json:"agentJobMessageID,omitempty" yaml:"agentJobMessageID,omitempty"`
	AgentJobResult    *AgentJobResult   `json:"agentJobResult,omitempty" yaml:"agentJobResult,omitempty"`
}

// AgentJobResult captures execution output from an agent running as a K8s Job.
// Written by the agent pod, read by the orchestrator after Job completion.
type AgentJobResult struct {
	Output          string            `json:"output,omitempty" yaml:"output,omitempty"`
	LastEvent       string            `json:"lastEvent,omitempty" yaml:"lastEvent,omitempty"`
	Steps           int               `json:"steps,omitempty" yaml:"steps,omitempty"`
	ToolCalls       int               `json:"toolCalls,omitempty" yaml:"toolCalls,omitempty"`
	MemoryWrites    int               `json:"memoryWrites,omitempty" yaml:"memoryWrites,omitempty"`
	EstimatedTokens int               `json:"estimatedTokens,omitempty" yaml:"estimatedTokens,omitempty"`
	TokensUsed      int               `json:"tokensUsed,omitempty" yaml:"tokensUsed,omitempty"`
	TokenSource     string            `json:"tokenSource,omitempty" yaml:"tokenSource,omitempty"`
	DurationMS      int64             `json:"durationMS,omitempty" yaml:"durationMS,omitempty"`
	StepEvents      []AgentJobStepEvt `json:"stepEvents,omitempty" yaml:"stepEvents,omitempty"`
	Events          []string          `json:"events,omitempty" yaml:"events,omitempty"`
	Error           string            `json:"error,omitempty" yaml:"error,omitempty"`
	ExitReason      string            `json:"exitReason,omitempty" yaml:"exitReason,omitempty"`
}

// AgentJobStepEvt is the serializable form of a step event for cross-pod transfer.
type AgentJobStepEvt struct {
	Timestamp           string `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
	Type                string `json:"type,omitempty" yaml:"type,omitempty"`
	Step                int    `json:"step,omitempty" yaml:"step,omitempty"`
	Tool                string `json:"tool,omitempty" yaml:"tool,omitempty"`
	Message             string `json:"message,omitempty" yaml:"message,omitempty"`
	ErrorCode           string `json:"errorCode,omitempty" yaml:"errorCode,omitempty"`
	ErrorReason         string `json:"errorReason,omitempty" yaml:"errorReason,omitempty"`
	Retryable           *bool  `json:"retryable,omitempty" yaml:"retryable,omitempty"`
	ToolContractVersion string `json:"toolContractVersion,omitempty" yaml:"toolContractVersion,omitempty"`
	ToolRequestID       string `json:"toolRequestID,omitempty" yaml:"toolRequestID,omitempty"`
	ToolAttempt         int    `json:"toolAttempt,omitempty" yaml:"toolAttempt,omitempty"`
	LatencyMS           int64  `json:"latencyMS,omitempty" yaml:"latencyMS,omitempty"`
	Tokens              int    `json:"tokens,omitempty" yaml:"tokens,omitempty"`
	InputTokens         int    `json:"inputTokens,omitempty" yaml:"inputTokens,omitempty"`
	OutputTokens        int    `json:"outputTokens,omitempty" yaml:"outputTokens,omitempty"`
	UsageSource         string `json:"usageSource,omitempty" yaml:"usageSource,omitempty"`
	ToolAuthProfile     string `json:"toolAuthProfile,omitempty" yaml:"toolAuthProfile,omitempty"`
	ToolAuthSecretRef   string `json:"toolAuthSecretRef,omitempty" yaml:"toolAuthSecretRef,omitempty"`
}

type TaskList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []Task `json:"items" yaml:"items"`
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
	if err := ValidateMetadataName(t.Metadata.Name); err != nil {
		return err
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
	if mode == "run" {
		t.Spec.System = strings.TrimSpace(t.Spec.System)
		if t.Spec.System == "" {
			return fmt.Errorf("spec.system is required when spec.mode is run")
		}
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
	if t.Status.DelegationStates == nil {
		t.Status.DelegationStates = make([]TaskDelegationState, 0)
	}
	if t.Status.BlockedOn != nil {
		t.Status.BlockedOn.Kind = strings.TrimSpace(t.Status.BlockedOn.Kind)
		t.Status.BlockedOn.Name = strings.TrimSpace(t.Status.BlockedOn.Name)
		t.Status.BlockedOn.Reason = strings.TrimSpace(t.Status.BlockedOn.Reason)
		if t.Status.BlockedOn.Kind == "" && t.Status.BlockedOn.Name == "" && t.Status.BlockedOn.Reason == "" {
			t.Status.BlockedOn = nil
		}
	}
	return nil
}

// TaskSchedule defines recurring task creation from a template task.
type TaskSchedule struct {
	APIVersion string             `json:"apiVersion" yaml:"apiVersion"`
	Kind       string             `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta         `json:"metadata" yaml:"metadata"`
	Spec       TaskScheduleSpec   `json:"spec" yaml:"spec"`
	Status     TaskScheduleStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type TaskScheduleSpec struct {
	TaskRef                 string    `json:"task_ref,omitempty" yaml:"task_ref,omitempty"`
	TaskTemplate            *TaskSpec `json:"task_template,omitempty" yaml:"task_template,omitempty"`
	Schedule                string    `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	TimeZone                string    `json:"time_zone,omitempty" yaml:"time_zone,omitempty"`
	Suspend                 bool      `json:"suspend,omitempty" yaml:"suspend,omitempty"`
	StartingDeadlineSeconds int       `json:"starting_deadline_seconds,omitempty" yaml:"starting_deadline_seconds,omitempty"`
	ConcurrencyPolicy       string    `json:"concurrency_policy,omitempty" yaml:"concurrency_policy,omitempty"`
	SuccessfulHistoryLimit  int       `json:"successful_history_limit,omitempty" yaml:"successful_history_limit,omitempty"`
	FailedHistoryLimit      int       `json:"failed_history_limit,omitempty" yaml:"failed_history_limit,omitempty"`
}

type TaskScheduleStatus struct {
	Phase              string   `json:"phase,omitempty" yaml:"phase,omitempty"`
	LastError          string   `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	LastScheduleTime   string   `json:"lastScheduleTime,omitempty" yaml:"lastScheduleTime,omitempty"`
	LastSuccessfulTime string   `json:"lastSuccessfulTime,omitempty" yaml:"lastSuccessfulTime,omitempty"`
	NextScheduleTime   string   `json:"nextScheduleTime,omitempty" yaml:"nextScheduleTime,omitempty"`
	LastTriggeredTask  string   `json:"lastTriggeredTask,omitempty" yaml:"lastTriggeredTask,omitempty"`
	ActiveRuns         []string `json:"activeRuns,omitempty" yaml:"activeRuns,omitempty"`
	ObservedGeneration int64    `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
}

type TaskScheduleList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []TaskSchedule `json:"items" yaml:"items"`
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
	if err := ValidateMetadataName(t.Metadata.Name); err != nil {
		return err
	}

	t.Spec.TaskRef = strings.TrimSpace(t.Spec.TaskRef)
	hasRef := t.Spec.TaskRef != ""
	hasInline := t.Spec.TaskTemplate != nil
	if hasRef && hasInline {
		return fmt.Errorf("spec.task_ref and spec.task_template are mutually exclusive")
	}
	if !hasRef && !hasInline {
		return fmt.Errorf("one of spec.task_ref or spec.task_template is required")
	}
	if hasRef && strings.Contains(t.Spec.TaskRef, "/") {
		parts := strings.SplitN(t.Spec.TaskRef, "/", 2)
		if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("invalid spec.task_ref %q: expected name or namespace/name", t.Spec.TaskRef)
		}
	}
	if hasInline {
		tmpl := t.Spec.TaskTemplate
		tmpl.System = strings.TrimSpace(tmpl.System)
		if tmpl.System == "" {
			return fmt.Errorf("spec.task_template.system is required")
		}
		if tmpl.Input == nil {
			tmpl.Input = make(map[string]string)
		}
		if strings.TrimSpace(tmpl.Priority) == "" {
			tmpl.Priority = "normal"
		}
		if tmpl.Retry.MaxAttempts <= 0 {
			tmpl.Retry.MaxAttempts = 1
		}
		if tmpl.Retry.Backoff == "" {
			tmpl.Retry.Backoff = "0s"
		}
		if _, err := time.ParseDuration(tmpl.Retry.Backoff); err != nil {
			return fmt.Errorf("invalid spec.task_template.retry.backoff %q: %w", tmpl.Retry.Backoff, err)
		}
		if tmpl.MessageRetry.MaxAttempts <= 0 {
			tmpl.MessageRetry.MaxAttempts = tmpl.Retry.MaxAttempts
		}
		if strings.TrimSpace(tmpl.MessageRetry.Backoff) == "" {
			tmpl.MessageRetry.Backoff = tmpl.Retry.Backoff
		}
		if _, err := time.ParseDuration(tmpl.MessageRetry.Backoff); err != nil {
			return fmt.Errorf("invalid spec.task_template.message_retry.backoff %q: %w", tmpl.MessageRetry.Backoff, err)
		}
		if strings.TrimSpace(tmpl.MessageRetry.MaxBackoff) == "" {
			tmpl.MessageRetry.MaxBackoff = "24h"
		}
		if _, err := time.ParseDuration(tmpl.MessageRetry.MaxBackoff); err != nil {
			return fmt.Errorf("invalid spec.task_template.message_retry.max_backoff %q: %w", tmpl.MessageRetry.MaxBackoff, err)
		}
		jitter := strings.ToLower(strings.TrimSpace(tmpl.MessageRetry.Jitter))
		if jitter == "" {
			jitter = "full"
		}
		switch jitter {
		case "none", "full", "equal":
			tmpl.MessageRetry.Jitter = jitter
		default:
			return fmt.Errorf("invalid spec.task_template.message_retry.jitter %q: expected none, full, or equal", tmpl.MessageRetry.Jitter)
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
	APIVersion string            `json:"apiVersion" yaml:"apiVersion"`
	Kind       string            `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta        `json:"metadata" yaml:"metadata"`
	Spec       TaskWebhookSpec   `json:"spec" yaml:"spec"`
	Status     TaskWebhookStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type TaskWebhookSpec struct {
	TaskRef      string                 `json:"task_ref,omitempty" yaml:"task_ref,omitempty"`
	TaskTemplate *TaskSpec              `json:"task_template,omitempty" yaml:"task_template,omitempty"`
	Suspend      bool                   `json:"suspend,omitempty" yaml:"suspend,omitempty"`
	Auth         TaskWebhookAuthSpec    `json:"auth,omitempty" yaml:"auth,omitempty"`
	Idempotency  TaskWebhookIdempotency `json:"idempotency,omitempty" yaml:"idempotency,omitempty"`
	Payload      TaskWebhookPayloadSpec `json:"payload,omitempty" yaml:"payload,omitempty"`
}

type TaskWebhookAuthSpec struct {
	Profile           string `json:"profile,omitempty" yaml:"profile,omitempty"`
	SecretRef         string `json:"secret_ref,omitempty" yaml:"secret_ref,omitempty"`
	SignatureHeader   string `json:"signature_header,omitempty" yaml:"signature_header,omitempty"`
	SignaturePrefix   string `json:"signature_prefix,omitempty" yaml:"signature_prefix,omitempty"`
	TimestampHeader   string `json:"timestamp_header,omitempty" yaml:"timestamp_header,omitempty"`
	MaxSkewSeconds    int    `json:"max_skew_seconds,omitempty" yaml:"max_skew_seconds,omitempty"`
	Algorithm         string `json:"algorithm,omitempty" yaml:"algorithm,omitempty"`
	PayloadFormat     string `json:"payload_format,omitempty" yaml:"payload_format,omitempty"`
	PayloadPrefix     string `json:"payload_prefix,omitempty" yaml:"payload_prefix,omitempty"`
	PayloadSeparator  string `json:"payload_separator,omitempty" yaml:"payload_separator,omitempty"`
	SignatureEncoding string `json:"signature_encoding,omitempty" yaml:"signature_encoding,omitempty"`
	HeaderFormat      string `json:"header_format,omitempty" yaml:"header_format,omitempty"`
	SignatureKey      string `json:"signature_key,omitempty" yaml:"signature_key,omitempty"`
	TimestampKey      string `json:"timestamp_key,omitempty" yaml:"timestamp_key,omitempty"`
}

type TaskWebhookIdempotency struct {
	EventIDHeader       string `json:"event_id_header,omitempty" yaml:"event_id_header,omitempty"`
	EventIDFromBody     string `json:"event_id_from_body,omitempty" yaml:"event_id_from_body,omitempty"`
	DedupeWindowSeconds int    `json:"dedupe_window_seconds,omitempty" yaml:"dedupe_window_seconds,omitempty"`
}

type TaskWebhookPayloadSpec struct {
	Mode     string `json:"mode,omitempty" yaml:"mode,omitempty"`
	InputKey string `json:"input_key,omitempty" yaml:"input_key,omitempty"`
}

type TaskWebhookStatus struct {
	Phase              string `json:"phase,omitempty" yaml:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
	EndpointID         string `json:"endpointID,omitempty" yaml:"endpointID,omitempty"`
	EndpointPath       string `json:"endpointPath,omitempty" yaml:"endpointPath,omitempty"`
	LastDeliveryTime   string `json:"lastDeliveryTime,omitempty" yaml:"lastDeliveryTime,omitempty"`
	LastEventID        string `json:"lastEventID,omitempty" yaml:"lastEventID,omitempty"`
	LastTriggeredTask  string `json:"lastTriggeredTask,omitempty" yaml:"lastTriggeredTask,omitempty"`
	AcceptedCount      int64  `json:"acceptedCount,omitempty" yaml:"acceptedCount,omitempty"`
	DuplicateCount     int64  `json:"duplicateCount,omitempty" yaml:"duplicateCount,omitempty"`
	RejectedCount      int64  `json:"rejectedCount,omitempty" yaml:"rejectedCount,omitempty"`
}

type TaskWebhookList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []TaskWebhook `json:"items" yaml:"items"`
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
	if err := ValidateMetadataName(t.Metadata.Name); err != nil {
		return err
	}

	t.Spec.TaskRef = strings.TrimSpace(t.Spec.TaskRef)
	hasRef := t.Spec.TaskRef != ""
	hasInline := t.Spec.TaskTemplate != nil
	if hasRef && hasInline {
		return fmt.Errorf("spec.task_ref and spec.task_template are mutually exclusive")
	}
	if !hasRef && !hasInline {
		return fmt.Errorf("one of spec.task_ref or spec.task_template is required")
	}
	if hasRef && strings.Contains(t.Spec.TaskRef, "/") {
		parts := strings.SplitN(t.Spec.TaskRef, "/", 2)
		if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("invalid spec.task_ref %q: expected name or namespace/name", t.Spec.TaskRef)
		}
	}
	if hasInline {
		tmpl := t.Spec.TaskTemplate
		tmpl.System = strings.TrimSpace(tmpl.System)
		if tmpl.System == "" {
			return fmt.Errorf("spec.task_template.system is required")
		}
		if tmpl.Input == nil {
			tmpl.Input = make(map[string]string)
		}
		if strings.TrimSpace(tmpl.Priority) == "" {
			tmpl.Priority = "normal"
		}
		if tmpl.Retry.MaxAttempts <= 0 {
			tmpl.Retry.MaxAttempts = 1
		}
		if tmpl.Retry.Backoff == "" {
			tmpl.Retry.Backoff = "0s"
		}
		if _, err := time.ParseDuration(tmpl.Retry.Backoff); err != nil {
			return fmt.Errorf("invalid spec.task_template.retry.backoff %q: %w", tmpl.Retry.Backoff, err)
		}
		if tmpl.MessageRetry.MaxAttempts <= 0 {
			tmpl.MessageRetry.MaxAttempts = tmpl.Retry.MaxAttempts
		}
		if strings.TrimSpace(tmpl.MessageRetry.Backoff) == "" {
			tmpl.MessageRetry.Backoff = tmpl.Retry.Backoff
		}
		if _, err := time.ParseDuration(tmpl.MessageRetry.Backoff); err != nil {
			return fmt.Errorf("invalid spec.task_template.message_retry.backoff %q: %w", tmpl.MessageRetry.Backoff, err)
		}
		if strings.TrimSpace(tmpl.MessageRetry.MaxBackoff) == "" {
			tmpl.MessageRetry.MaxBackoff = "24h"
		}
		if _, err := time.ParseDuration(tmpl.MessageRetry.MaxBackoff); err != nil {
			return fmt.Errorf("invalid spec.task_template.message_retry.max_backoff %q: %w", tmpl.MessageRetry.MaxBackoff, err)
		}
		jitter := strings.ToLower(strings.TrimSpace(tmpl.MessageRetry.Jitter))
		if jitter == "" {
			jitter = "full"
		}
		switch jitter {
		case "none", "full", "equal":
			tmpl.MessageRetry.Jitter = jitter
		default:
			return fmt.Errorf("invalid spec.task_template.message_retry.jitter %q: expected none, full, or equal", tmpl.MessageRetry.Jitter)
		}
	}

	t.Spec.Auth.Profile = strings.ToLower(strings.TrimSpace(t.Spec.Auth.Profile))
	if t.Spec.Auth.Profile == "" {
		t.Spec.Auth.Profile = "generic"
	}
	switch t.Spec.Auth.Profile {
	case "generic", "github", "hmac", "shared_token":
	default:
		return fmt.Errorf("invalid spec.auth.profile %q: expected generic, github, hmac, or shared_token", t.Spec.Auth.Profile)
	}

	t.Spec.Auth.SecretRef = strings.TrimSpace(t.Spec.Auth.SecretRef)
	if t.Spec.Auth.SecretRef == "" {
		return fmt.Errorf("spec.auth.secret_ref is required")
	}

	t.Spec.Auth.Algorithm = strings.ToLower(strings.TrimSpace(t.Spec.Auth.Algorithm))
	t.Spec.Auth.PayloadFormat = strings.ToLower(strings.TrimSpace(t.Spec.Auth.PayloadFormat))
	t.Spec.Auth.SignatureEncoding = strings.ToLower(strings.TrimSpace(t.Spec.Auth.SignatureEncoding))
	t.Spec.Auth.HeaderFormat = strings.ToLower(strings.TrimSpace(t.Spec.Auth.HeaderFormat))
	t.Spec.Auth.PayloadPrefix = strings.TrimSpace(t.Spec.Auth.PayloadPrefix)
	t.Spec.Auth.PayloadSeparator = strings.TrimSpace(t.Spec.Auth.PayloadSeparator)
	t.Spec.Auth.SignatureKey = strings.TrimSpace(t.Spec.Auth.SignatureKey)
	t.Spec.Auth.TimestampKey = strings.TrimSpace(t.Spec.Auth.TimestampKey)

	switch t.Spec.Auth.Profile {
	case "github":
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
		if strings.TrimSpace(t.Spec.Idempotency.EventIDHeader) == "" && strings.TrimSpace(t.Spec.Idempotency.EventIDFromBody) == "" {
			t.Spec.Idempotency.EventIDHeader = "X-GitHub-Delivery"
		}

	case "generic":
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
		if strings.TrimSpace(t.Spec.Idempotency.EventIDHeader) == "" && strings.TrimSpace(t.Spec.Idempotency.EventIDFromBody) == "" {
			t.Spec.Idempotency.EventIDHeader = "X-Event-Id"
		}

	case "hmac":
		if t.Spec.Auth.Algorithm == "" {
			t.Spec.Auth.Algorithm = "sha256"
		}
		switch t.Spec.Auth.Algorithm {
		case "sha256", "sha1", "sha512":
		default:
			return fmt.Errorf("invalid spec.auth.algorithm %q: expected sha256, sha1, or sha512", t.Spec.Auth.Algorithm)
		}
		if t.Spec.Auth.PayloadFormat == "" {
			t.Spec.Auth.PayloadFormat = "body"
		}
		switch t.Spec.Auth.PayloadFormat {
		case "body", "timestamp_dot_body", "prefix_timestamp_body":
		default:
			return fmt.Errorf("invalid spec.auth.payload_format %q: expected body, timestamp_dot_body, or prefix_timestamp_body", t.Spec.Auth.PayloadFormat)
		}
		if t.Spec.Auth.SignatureEncoding == "" {
			t.Spec.Auth.SignatureEncoding = "hex"
		}
		switch t.Spec.Auth.SignatureEncoding {
		case "hex", "base64":
		default:
			return fmt.Errorf("invalid spec.auth.signature_encoding %q: expected hex or base64", t.Spec.Auth.SignatureEncoding)
		}
		if t.Spec.Auth.HeaderFormat == "" {
			t.Spec.Auth.HeaderFormat = "plain"
		}
		switch t.Spec.Auth.HeaderFormat {
		case "plain", "kv_pairs":
		default:
			return fmt.Errorf("invalid spec.auth.header_format %q: expected plain or kv_pairs", t.Spec.Auth.HeaderFormat)
		}
		if t.Spec.Auth.HeaderFormat == "kv_pairs" {
			if t.Spec.Auth.SignatureKey == "" {
				return fmt.Errorf("spec.auth.signature_key is required when header_format is kv_pairs")
			}
			payloadUsesTimestamp := t.Spec.Auth.PayloadFormat == "timestamp_dot_body" || t.Spec.Auth.PayloadFormat == "prefix_timestamp_body"
			if payloadUsesTimestamp && t.Spec.Auth.TimestampKey == "" {
				return fmt.Errorf("spec.auth.timestamp_key is required when header_format is kv_pairs and payload_format includes a timestamp")
			}
		}
		if t.Spec.Auth.PayloadFormat == "prefix_timestamp_body" && t.Spec.Auth.PayloadSeparator == "" {
			t.Spec.Auth.PayloadSeparator = "."
		}
		if strings.TrimSpace(t.Spec.Auth.SignatureHeader) == "" {
			return fmt.Errorf("spec.auth.signature_header is required for hmac profile")
		}
		payloadUsesTimestamp := t.Spec.Auth.PayloadFormat == "timestamp_dot_body" || t.Spec.Auth.PayloadFormat == "prefix_timestamp_body"
		if payloadUsesTimestamp && t.Spec.Auth.HeaderFormat != "kv_pairs" {
			if strings.TrimSpace(t.Spec.Auth.TimestampHeader) == "" {
				return fmt.Errorf("spec.auth.timestamp_header is required when payload_format includes a timestamp and header_format is plain")
			}
		}
		if t.Spec.Auth.MaxSkewSeconds < 0 {
			return fmt.Errorf("invalid spec.auth.max_skew_seconds %d: expected >= 0", t.Spec.Auth.MaxSkewSeconds)
		}
		if t.Spec.Auth.MaxSkewSeconds == 0 {
			t.Spec.Auth.MaxSkewSeconds = 300
		}
		if strings.TrimSpace(t.Spec.Idempotency.EventIDHeader) == "" && strings.TrimSpace(t.Spec.Idempotency.EventIDFromBody) == "" {
			t.Spec.Idempotency.EventIDHeader = "X-Event-Id"
		}

	case "shared_token":
		if strings.TrimSpace(t.Spec.Auth.SignatureHeader) == "" {
			return fmt.Errorf("spec.auth.signature_header is required for shared_token profile")
		}
		if strings.TrimSpace(t.Spec.Idempotency.EventIDHeader) == "" && strings.TrimSpace(t.Spec.Idempotency.EventIDFromBody) == "" {
			t.Spec.Idempotency.EventIDHeader = "X-Event-Id"
		}
	}

	t.Spec.Idempotency.EventIDFromBody = strings.TrimSpace(t.Spec.Idempotency.EventIDFromBody)

	if strings.TrimSpace(t.Spec.Auth.SignatureHeader) == "" {
		return fmt.Errorf("spec.auth.signature_header is required")
	}
	if strings.TrimSpace(t.Spec.Idempotency.EventIDHeader) == "" && t.Spec.Idempotency.EventIDFromBody == "" {
		return fmt.Errorf("spec.idempotency.event_id_header is required (or set event_id_from_body to extract from the request body)")
	}
	if t.Spec.Idempotency.DedupeWindowSeconds < 0 {
		return fmt.Errorf("invalid spec.idempotency.dedupe_window_seconds %d: expected >= 0", t.Spec.Idempotency.DedupeWindowSeconds)
	}
	if t.Spec.Idempotency.DedupeWindowSeconds == 0 {
		if t.Spec.Auth.Profile == "github" {
			// GitHub webhooks have no timestamp in the HMAC payload, so replay
			// protection relies entirely on dedup. 72h matches GitHub's max
			// retry window and provides adequate replay protection.
			t.Spec.Idempotency.DedupeWindowSeconds = 259200
		} else {
			t.Spec.Idempotency.DedupeWindowSeconds = 86400
		}
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
	APIVersion string       `json:"apiVersion" yaml:"apiVersion"`
	Kind       string       `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta   `json:"metadata" yaml:"metadata"`
	Spec       WorkerSpec   `json:"spec" yaml:"spec"`
	Status     WorkerStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type WorkerSpec struct {
	Region             string             `json:"region,omitempty" yaml:"region,omitempty"`
	Capabilities       WorkerCapabilities `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	MaxConcurrentTasks int                `json:"max_concurrent_tasks,omitempty" yaml:"max_concurrent_tasks,omitempty"`
}

type WorkerCapabilities struct {
	GPU             bool     `json:"gpu,omitempty" yaml:"gpu,omitempty"`
	SupportedModels []string `json:"supported_models,omitempty" yaml:"supported_models,omitempty"`
}

type WorkerStatus struct {
	Phase              string `json:"phase,omitempty" yaml:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	LastHeartbeat      string `json:"lastHeartbeat,omitempty" yaml:"lastHeartbeat,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
	CurrentTasks       int    `json:"currentTasks,omitempty" yaml:"currentTasks,omitempty"`
}

type WorkerList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []Worker `json:"items" yaml:"items"`
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
	if err := ValidateMetadataName(w.Metadata.Name); err != nil {
		return err
	}
	if w.Spec.MaxConcurrentTasks <= 0 {
		w.Spec.MaxConcurrentTasks = 1
	}
	if w.Status.Phase == "" {
		w.Status.Phase = "Pending"
	}
	return nil
}

// McpServer declares an MCP (Model Context Protocol) server connection.
// The controller connects to the server, discovers tools via tools/list,
// and auto-generates Tool resources for each discovered tool.
type McpServer struct {
	APIVersion string          `json:"apiVersion" yaml:"apiVersion"`
	Kind       string          `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta      `json:"metadata" yaml:"metadata"`
	Spec       McpServerSpec   `json:"spec" yaml:"spec"`
	Status     McpServerStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type McpServerSpec struct {
	Transport          string             `json:"transport" yaml:"transport"`
	Command            string             `json:"command,omitempty" yaml:"command,omitempty"`
	Args               []string           `json:"args,omitempty" yaml:"args,omitempty"`
	Env                []McpServerEnvVar  `json:"env,omitempty" yaml:"env,omitempty"`
	Endpoint           string             `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Image              string             `json:"image,omitempty" yaml:"image,omitempty"`
	ImagePullSecret    string             `json:"image_pull_secret,omitempty" yaml:"image_pull_secret,omitempty"`
	IdleTimeout        string             `json:"idle_timeout,omitempty" yaml:"idle_timeout,omitempty"`
	Auth               ToolAuth           `json:"auth,omitempty" yaml:"auth,omitempty"`
	ToolFilter         McpToolFilter      `json:"tool_filter,omitempty" yaml:"tool_filter,omitempty"`
	Reconnect          McpReconnectPolicy `json:"reconnect,omitempty" yaml:"reconnect,omitempty"`
	Resources          ContainerResources `json:"resources,omitempty" yaml:"resources,omitempty"`
	DefaultToolRuntime *ToolRuntimePolicy `json:"default_tool_runtime,omitempty" yaml:"default_tool_runtime,omitempty"`
	// AllowPrivate permits this MCP server's HTTP transport to connect to
	// RFC 1918 / ULA / CGNAT addresses (e.g. in-cluster Services). Loopback,
	// link-local, cloud metadata, and unspecified addresses remain blocked
	// regardless. Defaults to false; set true only for trusted internal MCP
	// servers. Has no effect on stdio transport.
	AllowPrivate *bool `json:"allowPrivate,omitempty" yaml:"allowPrivate,omitempty"`
}

type McpServerEnvVar struct {
	Name      string `json:"name" yaml:"name"`
	Value     string `json:"value,omitempty" yaml:"value,omitempty"`
	SecretRef string `json:"secretRef,omitempty" yaml:"secretRef,omitempty"`
	MountPath string `json:"mountPath,omitempty" yaml:"mountPath,omitempty"`
}

type McpToolFilter struct {
	Include []string `json:"include,omitempty" yaml:"include,omitempty"`
}

type McpReconnectPolicy struct {
	MaxAttempts int    `json:"max_attempts,omitempty" yaml:"max_attempts,omitempty"`
	Backoff     string `json:"backoff,omitempty" yaml:"backoff,omitempty"`
}

type McpServerStatus struct {
	Phase              string   `json:"phase,omitempty" yaml:"phase,omitempty"`
	DiscoveredTools    []string `json:"discoveredTools,omitempty" yaml:"discoveredTools,omitempty"`
	GeneratedTools     []string `json:"generatedTools,omitempty" yaml:"generatedTools,omitempty"`
	LastSyncedAt       string   `json:"lastSyncedAt,omitempty" yaml:"lastSyncedAt,omitempty"`
	LastError          string   `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	ObservedGeneration int64    `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
}

type McpServerList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []McpServer `json:"items" yaml:"items"`
}

func (m *McpServer) Normalize() error {
	if m.APIVersion == "" {
		m.APIVersion = "orloj.dev/v1"
	}
	if m.Kind == "" {
		m.Kind = "McpServer"
	}
	if !strings.EqualFold(m.Kind, "McpServer") {
		return fmt.Errorf("unsupported kind %q for McpServer", m.Kind)
	}
	NormalizeObjectMetaNamespace(&m.Metadata)
	if err := ValidateMetadataName(m.Metadata.Name); err != nil {
		return err
	}
	transport := strings.ToLower(strings.TrimSpace(m.Spec.Transport))
	if transport == "" {
		return fmt.Errorf("spec.transport is required (stdio or http)")
	}
	switch transport {
	case "stdio", "http":
		m.Spec.Transport = transport
	default:
		return fmt.Errorf("invalid spec.transport %q: expected stdio or http", m.Spec.Transport)
	}
	if transport == "stdio" && strings.TrimSpace(m.Spec.Command) == "" && strings.TrimSpace(m.Spec.Image) == "" {
		return fmt.Errorf("spec.command or spec.image is required for stdio transport")
	}
	if transport == "http" && strings.TrimSpace(m.Spec.Endpoint) == "" {
		return fmt.Errorf("spec.endpoint is required for http transport")
	}
	if transport == "http" && strings.TrimSpace(m.Spec.Image) != "" {
		return fmt.Errorf("spec.image is only supported for stdio transport")
	}
	m.Spec.Command = strings.TrimSpace(m.Spec.Command)
	m.Spec.Endpoint = strings.TrimSpace(m.Spec.Endpoint)
	m.Spec.Image = strings.TrimSpace(m.Spec.Image)
	m.Spec.ImagePullSecret = strings.TrimSpace(m.Spec.ImagePullSecret)
	if m.Spec.ImagePullSecret != "" && m.Spec.Image == "" {
		return fmt.Errorf("spec.image is required when spec.image_pull_secret is set")
	}
	for i, env := range m.Spec.Env {
		m.Spec.Env[i].Name = strings.TrimSpace(env.Name)
		m.Spec.Env[i].Value = strings.TrimSpace(env.Value)
		m.Spec.Env[i].SecretRef = strings.TrimSpace(env.SecretRef)
		m.Spec.Env[i].MountPath = strings.TrimSpace(env.MountPath)
		if m.Spec.Env[i].MountPath != "" {
			if m.Spec.Image == "" {
				return fmt.Errorf("env[%d].mountPath requires spec.image to be set", i)
			}
			if !strings.HasPrefix(m.Spec.Env[i].MountPath, "/") {
				return fmt.Errorf("env[%d].mountPath must be an absolute path, got %q", i, m.Spec.Env[i].MountPath)
			}
		}
	}
	normalized := make([]string, 0, len(m.Spec.ToolFilter.Include))
	for _, name := range m.Spec.ToolFilter.Include {
		name = strings.TrimSpace(name)
		if name != "" {
			normalized = append(normalized, name)
		}
	}
	m.Spec.ToolFilter.Include = normalized
	idleTimeout := strings.TrimSpace(m.Spec.IdleTimeout)
	if idleTimeout == "" {
		idleTimeout = "0"
	}
	if idleTimeout != "0" {
		if _, err := time.ParseDuration(idleTimeout); err != nil {
			return fmt.Errorf("invalid spec.idle_timeout %q: %w", m.Spec.IdleTimeout, err)
		}
	}
	m.Spec.IdleTimeout = idleTimeout
	if m.Spec.Reconnect.MaxAttempts <= 0 {
		m.Spec.Reconnect.MaxAttempts = 3
	}
	if strings.TrimSpace(m.Spec.Reconnect.Backoff) == "" {
		m.Spec.Reconnect.Backoff = "2s"
	}
	if r := m.Spec.DefaultToolRuntime; r != nil {
		if t := strings.TrimSpace(r.Timeout); t != "" {
			if _, err := time.ParseDuration(t); err != nil {
				return fmt.Errorf("invalid spec.default_tool_runtime.timeout %q: %w", t, err)
			}
			r.Timeout = t
		}
	}
	if m.Status.Phase == "" {
		m.Status.Phase = "Pending"
	}
	return nil
}
