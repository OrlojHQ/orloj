package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

var (
	ErrUnsupportedTool          = errors.New("unsupported tool")
	ErrToolIsolationUnavailable = errors.New("tool isolation runtime unavailable")
	ErrInvalidToolRuntimePolicy = errors.New("invalid tool runtime policy")
	ErrToolPermissionDenied     = errors.New("tool permission denied")
	ErrToolApprovalRequired     = errors.New("tool approval required")
)

// ToolCapabilityRegistry resolves runtime policy/capability metadata for tools.
type ToolCapabilityRegistry interface {
	Resolve(tool string) (resources.ToolSpec, bool)
}

// ToolResourceLookup resolves Tool CRDs by name (optionally namespace scoped).
type ToolResourceLookup interface {
	Get(name string) (resources.Tool, bool)
}

type registryAwareToolRuntime interface {
	WithRegistry(registry ToolCapabilityRegistry) ToolRuntime
}

type namespaceAwareToolRuntime interface {
	WithNamespace(namespace string) ToolRuntime
}

// StaticToolCapabilityRegistry stores tool policies in-memory for runtime checks.
type StaticToolCapabilityRegistry struct {
	specs map[string]resources.ToolSpec
}

func NewStaticToolCapabilityRegistry(specs map[string]resources.ToolSpec) *StaticToolCapabilityRegistry {
	out := make(map[string]resources.ToolSpec, len(specs))
	for name, spec := range specs {
		key := normalizeToolKey(name)
		if key == "" {
			continue
		}
		out[key] = spec
	}
	return &StaticToolCapabilityRegistry{specs: out}
}

func NewToolCapabilityRegistryFromTools(tools []resources.Tool) *StaticToolCapabilityRegistry {
	specs := make(map[string]resources.ToolSpec, len(tools))
	for _, tool := range tools {
		key := normalizeToolKey(tool.Metadata.Name)
		if key == "" {
			continue
		}
		specs[key] = tool.Spec
	}
	return NewStaticToolCapabilityRegistry(specs)
}

func (r *StaticToolCapabilityRegistry) Resolve(tool string) (resources.ToolSpec, bool) {
	if r == nil {
		return resources.ToolSpec{}, false
	}
	spec, ok := r.specs[normalizeToolKey(tool)]
	return spec, ok
}

// UnsupportedIsolatedToolRuntime fails closed when isolation is required but no sandbox executor is wired.
type UnsupportedIsolatedToolRuntime struct{}

func (r *UnsupportedIsolatedToolRuntime) Call(_ context.Context, tool string, _ string) (string, error) {
	return "", NewToolError(
		ToolStatusError,
		ToolCodeIsolationUnavailable,
		ToolReasonIsolationUnavailable,
		false,
		fmt.Sprintf("tool isolation runtime unavailable for tool=%s", strings.TrimSpace(tool)),
		ErrToolIsolationUnavailable,
		map[string]string{"tool": strings.TrimSpace(tool)},
	)
}

// GovernedToolRuntime enforces per-tool policy (timeout/retry/isolation) using Tool CRD runtime metadata.
type GovernedToolRuntime struct {
	baseRuntime     ToolRuntime
	isolatedRuntime ToolRuntime
	mcpRuntime      ToolRuntime
	registry        ToolCapabilityRegistry
	authorizer      ToolCallAuthorizer
	strict          bool
}

func NewGovernedToolRuntime(
	baseRuntime ToolRuntime,
	isolatedRuntime ToolRuntime,
	registry ToolCapabilityRegistry,
	strict bool,
) *GovernedToolRuntime {
	return NewGovernedToolRuntimeWithAuthorizer(baseRuntime, isolatedRuntime, registry, nil, strict)
}

func NewGovernedToolRuntimeWithAuthorizer(
	baseRuntime ToolRuntime,
	isolatedRuntime ToolRuntime,
	registry ToolCapabilityRegistry,
	authorizer ToolCallAuthorizer,
	strict bool,
) *GovernedToolRuntime {
	if baseRuntime == nil {
		baseRuntime = NewHTTPToolClient(nil, nil, nil)
	}
	return &GovernedToolRuntime{
		baseRuntime:     baseRuntime,
		isolatedRuntime: isolatedRuntime,
		registry:        registry,
		authorizer:      authorizer,
		strict:          strict,
	}
}

// BuildGovernedToolRuntimeForAgent resolves tool policies for one agent in a namespace.
// Missing registry entries are treated as unsupported at call time when strict mode is enabled.
func BuildGovernedToolRuntimeForAgent(
	baseRuntime ToolRuntime,
	isolatedRuntime ToolRuntime,
	lookup ToolResourceLookup,
	namespace string,
	toolNames []string,
) ToolRuntime {
	return buildGovernedToolRuntime(baseRuntime, isolatedRuntime, lookup, namespace, toolNames, nil)
}

func BuildGovernedToolRuntimeForAgentWithGovernance(
	baseRuntime ToolRuntime,
	isolatedRuntime ToolRuntime,
	toolLookup ToolResourceLookup,
	roleLookup AgentRoleLookup,
	permissionLookup ToolPermissionLookup,
	namespace string,
	agent resources.Agent,
	approvalCtx *GovernedToolApprovalContext,
) ToolRuntime {
	inner := NewAgentToolAuthorizer(namespace, agent, roleLookup, permissionLookup)
	auth := ToolCallAuthorizer(inner)
	if approvalCtx != nil && approvalCtx.Getter != nil &&
		strings.TrimSpace(approvalCtx.TaskKey) != "" && strings.TrimSpace(approvalCtx.MessageID) != "" {
		auth = NewAuthorizerWithApprovedToolGrant(inner, approvalCtx.Getter, approvalCtx.TaskKey, approvalCtx.MessageID)
	}
	return buildGovernedToolRuntime(
		baseRuntime,
		isolatedRuntime,
		toolLookup,
		namespace,
		agent.Spec.Tools,
		auth,
	)
}

func buildGovernedToolRuntime(
	baseRuntime ToolRuntime,
	isolatedRuntime ToolRuntime,
	lookup ToolResourceLookup,
	namespace string,
	toolNames []string,
	authorizer ToolCallAuthorizer,
) ToolRuntime {
	if len(toolNames) == 0 {
		return nil
	}
	specs := make(map[string]resources.ToolSpec, len(toolNames))
	seen := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		trimmed := strings.TrimSpace(name)
		key := normalizeToolKey(trimmed)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if lookup == nil {
			continue
		}
		item, ok := lookup.Get(scopedRuntimeName(namespace, trimmed))
		if !ok && strings.Contains(trimmed, "/") {
			item, ok = lookup.Get(trimmed)
		}
		if ok {
			specs[key] = item.Spec
		}
	}
	if isolatedRuntime == nil {
		isolatedRuntime = &UnsupportedIsolatedToolRuntime{}
	} else {
		if scoped, ok := isolatedRuntime.(namespaceAwareToolRuntime); ok {
			isolatedRuntime = scoped.WithNamespace(namespace)
		}
		if aware, ok := isolatedRuntime.(registryAwareToolRuntime); ok {
			isolatedRuntime = aware.WithRegistry(NewStaticToolCapabilityRegistry(specs))
		}
	}
	governed := NewGovernedToolRuntimeWithAuthorizer(baseRuntime, isolatedRuntime, NewStaticToolCapabilityRegistry(specs), authorizer, true)
	return governed
}

// SetMcpRuntime configures the MCP tool runtime used for type=mcp tools.
func (r *GovernedToolRuntime) SetMcpRuntime(mcpRuntime ToolRuntime) {
	if r != nil {
		r.mcpRuntime = mcpRuntime
	}
}

// ConfigureMcpRuntime builds and attaches an MCP runtime using the given
// session manager and server store. The runtime is scoped to the governed
// runtime's registry and the provided namespace.
func ConfigureMcpRuntime(rt ToolRuntime, sessionManager *McpSessionManager, mcpServerStore McpServerLookup, namespace string) {
	governed, ok := rt.(*GovernedToolRuntime)
	if !ok || governed == nil {
		return
	}
	var mcpRT ToolRuntime = NewMCPToolRuntime(governed.registry, sessionManager, mcpServerStore)
	if scoped, ok := mcpRT.(namespaceAwareToolRuntime); ok {
		mcpRT = scoped.WithNamespace(namespace)
	}
	if aware, ok := mcpRT.(registryAwareToolRuntime); ok && governed.registry != nil {
		mcpRT = aware.WithRegistry(governed.registry)
	}
	governed.mcpRuntime = mcpRT
}

func (r *GovernedToolRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeInvalidInput,
			ToolReasonInvalidInput,
			false,
			"missing tool name",
			ErrInvalidToolRuntimePolicy,
			map[string]string{"field": "tool"},
		)
	}
	spec, ok := r.resolve(tool)
	if !ok {
		if r.strict {
			return "", NewToolError(
				ToolStatusError,
				ToolCodeUnsupportedTool,
				ToolReasonToolUnsupported,
				false,
				fmt.Sprintf("unsupported tool %s", tool),
				ErrUnsupportedTool,
				map[string]string{"tool": tool},
			)
		}
		return r.baseRuntime.Call(ctx, tool, input)
	}
	if r.authorizer != nil {
		result, err := r.authorizer.Authorize(tool, spec)
		if err != nil {
			if IsToolDeniedError(err) {
				return "", err
			}
			if errors.Is(err, ErrToolPermissionDenied) {
				return "", NewToolDeniedError(
					fmt.Sprintf("policy permission denied for tool=%s", tool),
					map[string]string{"tool": tool},
					err,
				)
			}
			return "", err
		}
		if result != nil && result.Verdict == AuthorizeVerdictApprovalRequired {
			return "", fmt.Errorf("%w: tool=%s reason=%s", ErrToolApprovalRequired, tool, result.Reason)
		}
	}
	return r.callWithPolicy(ctx, tool, input, spec)
}

func (r *GovernedToolRuntime) resolve(tool string) (resources.ToolSpec, bool) {
	if r.registry == nil {
		return resources.ToolSpec{}, false
	}
	return r.registry.Resolve(tool)
}

// ResolveToolSchemas returns description and input schema metadata for the
// given tool names, sourced from the underlying ToolCapabilityRegistry.
func (r *GovernedToolRuntime) ResolveToolSchemas(toolNames []string) map[string]ToolSchemaInfo {
	if r == nil || r.registry == nil {
		return nil
	}
	out := make(map[string]ToolSchemaInfo, len(toolNames))
	for _, name := range toolNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		spec, ok := r.registry.Resolve(name)
		if !ok {
			continue
		}
		if spec.Description == "" && len(spec.InputSchema) == 0 {
			continue
		}
		out[name] = ToolSchemaInfo{
			Description: spec.Description,
			InputSchema: spec.InputSchema,
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *GovernedToolRuntime) callWithPolicy(ctx context.Context, tool string, input string, spec resources.ToolSpec) (string, error) {
	target := r.baseRuntime
	toolType := strings.ToLower(strings.TrimSpace(spec.Type))
	if toolType == "mcp" {
		if r.mcpRuntime == nil {
			return "", NewToolError(
				ToolStatusError,
				ToolCodeIsolationUnavailable,
				ToolReasonIsolationUnavailable,
				false,
				fmt.Sprintf("mcp runtime unavailable for tool=%s", tool),
				ErrToolIsolationUnavailable,
				map[string]string{"tool": tool, "type": "mcp"},
			)
		}
		target = r.mcpRuntime
	} else {
		mode := strings.ToLower(strings.TrimSpace(spec.Runtime.IsolationMode))
		if mode == "" {
			risk := strings.ToLower(strings.TrimSpace(spec.RiskLevel))
			if risk == "high" || risk == "critical" {
				mode = "sandboxed"
			} else {
				mode = "none"
			}
		}
		if mode != "" && mode != "none" {
			if r.isolatedRuntime == nil {
				return "", NewToolError(
					ToolStatusError,
					ToolCodeIsolationUnavailable,
					ToolReasonIsolationUnavailable,
					false,
					fmt.Sprintf("tool isolation runtime unavailable for tool=%s mode=%s", tool, mode),
					ErrToolIsolationUnavailable,
					map[string]string{
						"tool":           tool,
						"isolation_mode": mode,
					},
				)
			}
			target = r.isolatedRuntime
		}
	}

	maxAttempts := spec.Runtime.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	timeoutRaw := strings.TrimSpace(spec.Runtime.Timeout)
	if timeoutRaw == "" {
		timeoutRaw = "30s"
	}
	timeout, err := time.ParseDuration(timeoutRaw)
	if err != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("invalid tool runtime timeout policy for tool=%s timeout=%q", tool, timeoutRaw),
			fmt.Errorf("%w: %v", ErrInvalidToolRuntimePolicy, err),
			map[string]string{
				"tool":    tool,
				"timeout": timeoutRaw,
			},
		)
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		callCtx := ctx
		cancel := func() {}
		if timeout > 0 {
			callCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		result, callErr := callToolRuntimeBounded(callCtx, target, tool, input)
		cancel()
		if callErr == nil {
			return result, nil
		}
		callErr = normalizeToolError(callErr, tool, timeout)
		lastErr = callErr
		if !shouldRetryToolError(callErr) || attempt >= maxAttempts {
			break
		}
		delay := computeToolRetryDelay(spec.Runtime.Retry, tool, attempt)
		if delay <= 0 {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr == nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			false,
			fmt.Sprintf("tool %s failed without runtime error details", tool),
			nil,
			map[string]string{
				"tool":         tool,
				"max_attempts": fmt.Sprintf("%d", maxAttempts),
			},
		)
	}
	return "", fmt.Errorf("tool %q failed after %d attempt(s): %w", tool, maxAttempts, lastErr)
}

func callToolRuntimeBounded(ctx context.Context, runtime ToolRuntime, tool string, input string) (string, error) {
	if runtime == nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("missing runtime for tool=%s", strings.TrimSpace(tool)),
			ErrInvalidToolRuntimePolicy,
			map[string]string{"tool": strings.TrimSpace(tool)},
		)
	}
	type callResult struct {
		output string
		err    error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		output, err := runtime.Call(ctx, tool, input)
		resultCh <- callResult{
			output: output,
			err:    err,
		}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-resultCh:
		return result.output, result.err
	}
}

func shouldRetryToolError(err error) bool {
	if err == nil {
		return false
	}
	if toolErr, ok := AsToolError(err); ok {
		return toolErr.Retryable
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, ErrUnsupportedTool) || errors.Is(err, ErrToolIsolationUnavailable) || errors.Is(err, ErrInvalidToolRuntimePolicy) || errors.Is(err, ErrToolPermissionDenied) || errors.Is(err, ErrToolApprovalRequired) {
		return false
	}
	lower := strings.ToLower(err.Error())
	nonRetryableMarkers := []string{
		"unsupported tool",
		"policy ",
		"permission denied",
		"invalid ",
		"isolation runtime unavailable",
		"auth_invalid",
		"auth_forbidden",
		"approval_pending",
		"approval_denied",
		"approval_timeout",
		"approval required",
	}
	for _, marker := range nonRetryableMarkers {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}

func normalizeToolError(err error, tool string, timeout time.Duration) error {
	if err == nil {
		return nil
	}
	if _, ok := AsToolError(err); ok {
		return err
	}
	tool = strings.TrimSpace(tool)

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return NewToolError(
			ToolStatusError,
			ToolCodeTimeout,
			ToolReasonExecutionTimeout,
			true,
			fmt.Sprintf("tool execution exceeded timeout for tool=%s", tool),
			err,
			map[string]string{
				"tool":    tool,
				"timeout": timeout.String(),
			},
		)
	case errors.Is(err, context.Canceled):
		return NewToolError(
			ToolStatusError,
			ToolCodeCanceled,
			ToolReasonExecutionCanceled,
			false,
			fmt.Sprintf("tool execution canceled for tool=%s", tool),
			err,
			map[string]string{"tool": tool},
		)
	}

	retryable := shouldRetryToolError(err)
	reason := ToolReasonBackendFailure
	if strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "permission denied") {
		reason = ToolReasonPermissionDenied
		retryable = false
	}
	return NewToolError(
		ToolStatusError,
		ToolCodeExecutionFailed,
		reason,
		retryable,
		fmt.Sprintf("tool execution failed for tool=%s: %s", tool, strings.TrimSpace(err.Error())),
		err,
		map[string]string{"tool": tool},
	)
}

func computeToolRetryDelay(policy resources.ToolRetryPolicy, tool string, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	base := 0 * time.Second
	if strings.TrimSpace(policy.Backoff) != "" {
		if parsed, err := time.ParseDuration(policy.Backoff); err == nil {
			base = parsed
		}
	}
	if base <= 0 {
		return 0
	}
	max := 30 * time.Second
	if strings.TrimSpace(policy.MaxBackoff) != "" {
		if parsed, err := time.ParseDuration(policy.MaxBackoff); err == nil && parsed > 0 {
			max = parsed
		}
	}
	exp := attempt - 1
	if exp > 10 {
		exp = 10
	}
	delay := base * time.Duration(1<<exp)
	if delay > max {
		delay = max
	}

	switch strings.ToLower(strings.TrimSpace(policy.Jitter)) {
	case "full":
		return applyDeterministicJitter(delay, 0, tool, attempt)
	case "equal":
		return applyDeterministicJitter(delay, 0.5, tool, attempt)
	default:
		return delay
	}
}

func applyDeterministicJitter(base time.Duration, floorRatio float64, tool string, attempt int) time.Duration {
	if base <= 0 {
		return 0
	}
	if floorRatio < 0 {
		floorRatio = 0
	}
	if floorRatio > 1 {
		floorRatio = 1
	}
	floor := time.Duration(float64(base) * floorRatio)
	span := base - floor
	if span <= 0 {
		return floor
	}
	fraction := jitterFraction(tool, attempt)
	jitter := time.Duration(float64(span) * fraction)
	out := floor + jitter
	if out <= 0 {
		return time.Millisecond
	}
	return out
}

func jitterFraction(tool string, attempt int) float64 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strings.TrimSpace(tool)))
	_, _ = hash.Write([]byte(fmt.Sprintf(":%d", attempt)))
	return float64(hash.Sum32()%10000) / 10000.0
}

func normalizeToolKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func scopedRuntimeName(namespace string, name string) string {
	return resources.NormalizeNamespace(namespace) + "/" + strings.TrimSpace(name)
}
