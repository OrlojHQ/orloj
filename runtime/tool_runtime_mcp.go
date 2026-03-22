package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

// MCPToolRuntime executes tool calls against MCP servers.
// It resolves the mcp_server_ref from the tool's ToolSpec, retrieves the
// session from the McpSessionManager, and delegates to tools/call.
type MCPToolRuntime struct {
	registry       ToolCapabilityRegistry
	sessionManager *McpSessionManager
	mcpServerStore McpServerLookup
	namespace      string
}

// McpServerLookup resolves McpServer resources by scoped name.
type McpServerLookup interface {
	Get(name string) (resources.McpServer, bool, error)
}

func NewMCPToolRuntime(
	registry ToolCapabilityRegistry,
	sessionManager *McpSessionManager,
	mcpServerStore McpServerLookup,
) *MCPToolRuntime {
	return &MCPToolRuntime{
		registry:       registry,
		sessionManager: sessionManager,
		mcpServerStore: mcpServerStore,
	}
}

func (r *MCPToolRuntime) WithRegistry(registry ToolCapabilityRegistry) ToolRuntime {
	copy := *r
	copy.registry = registry
	return &copy
}

func (r *MCPToolRuntime) WithNamespace(namespace string) ToolRuntime {
	copy := *r
	copy.namespace = resources.NormalizeNamespace(strings.TrimSpace(namespace))
	return &copy
}

func (r *MCPToolRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
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

	spec, ok := r.resolveSpec(tool)
	if !ok {
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

	serverRef := strings.TrimSpace(spec.McpServerRef)
	mcpToolName := strings.TrimSpace(spec.McpToolName)
	if serverRef == "" || mcpToolName == "" {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("tool=%s missing mcp_server_ref or mcp_tool_name", tool),
			ErrInvalidToolRuntimePolicy,
			map[string]string{"tool": tool},
		)
	}

	server, err := r.resolveServer(serverRef)
	if err != nil {
		return "", err
	}

	session, err := r.sessionManager.GetOrCreate(ctx, server)
	if err != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("mcp session failed for tool=%s server=%s: %s", tool, serverRef, err.Error()),
			err,
			map[string]string{"tool": tool, "mcp_server": serverRef},
		)
	}

	arguments := parseToolInputAsArguments(input)

	result, err := session.Transport.CallTool(ctx, mcpToolName, arguments)
	if err != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("mcp tools/call failed for tool=%s mcp_tool=%s: %s", tool, mcpToolName, err.Error()),
			err,
			map[string]string{"tool": tool, "mcp_tool": mcpToolName, "mcp_server": serverRef},
		)
	}

	if result.IsError {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("mcp tool %s returned error: %s", mcpToolName, result.McpTextResult()),
			nil,
			map[string]string{"tool": tool, "mcp_tool": mcpToolName},
		)
	}

	return result.McpTextResult(), nil
}

func (r *MCPToolRuntime) resolveSpec(tool string) (resources.ToolSpec, bool) {
	if r.registry == nil {
		return resources.ToolSpec{}, false
	}
	return r.registry.Resolve(tool)
}

func (r *MCPToolRuntime) resolveServer(serverRef string) (resources.McpServer, error) {
	if r.mcpServerStore == nil {
		return resources.McpServer{}, NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			"mcp server store not configured",
			ErrInvalidToolRuntimePolicy,
			nil,
		)
	}

	scopedName := serverRef
	if r.namespace != "" && !strings.Contains(serverRef, "/") {
		scopedName = r.namespace + "/" + serverRef
	}
	server, ok, err := r.mcpServerStore.Get(scopedName)
	if err == nil && !ok && strings.Contains(serverRef, "/") {
		server, ok, err = r.mcpServerStore.Get(serverRef)
	}
	if err != nil {
		return resources.McpServer{}, NewToolError(
			ToolStatusError,
			ToolCodeUnsupportedTool,
			ToolReasonToolUnsupported,
			true,
			fmt.Sprintf("mcp server %q lookup failed: %v", serverRef, err),
			err,
			map[string]string{"mcp_server": serverRef},
		)
	}
	if !ok {
		return resources.McpServer{}, NewToolError(
			ToolStatusError,
			ToolCodeUnsupportedTool,
			ToolReasonToolUnsupported,
			false,
			fmt.Sprintf("mcp server %q not found", serverRef),
			ErrUnsupportedTool,
			map[string]string{"mcp_server": serverRef},
		)
	}
	return server, nil
}

func parseToolInputAsArguments(input string) map[string]any {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return map[string]any{"input": input}
	}
	return args
}
