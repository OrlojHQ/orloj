package agentruntime

import (
	"context"
	"fmt"
	"strings"
)

// ContextAdapterDeps supplies tool runtime machinery shared with TaskController and the message consumer.
type ContextAdapterDeps struct {
	Tools          ToolResourceLookup
	Isolated       ToolRuntime
	Wasm           ToolRuntime
	SecretResolver SecretResolver
	Cli            CLIToolRuntimeConfig
	McpMgr         *McpSessionManager
	McpStore       McpServerLookup
}

// BuildSanitizerToolRuntime constructs a governed tool runtime wired like agent execution,
// scoped to exactly one Tool name (used for ContextAdapter sanitization tools).
func BuildSanitizerToolRuntime(
	ctx context.Context,
	namespace string,
	toolRef string,
	tools ToolResourceLookup,
	isolated ToolRuntime,
	wasmRT ToolRuntime,
	secretResolver SecretResolver,
	cliConfig CLIToolRuntimeConfig,
	mcpMgr *McpSessionManager,
	mcpStore McpServerLookup,
) (ToolRuntime, error) {
	toolRef = strings.TrimSpace(toolRef)
	if toolRef == "" {
		return nil, fmt.Errorf("context adapter tool_ref is empty")
	}
	rt := BuildGovernedToolRuntimeForAgent(ctx, nil, isolated, tools, namespace, []string{toolRef})
	if rt == nil {
		return nil, fmt.Errorf("context adapter runtime: no tools resolved for tool_ref %q", toolRef)
	}
	if mcpMgr != nil && mcpStore != nil {
		ConfigureMcpRuntime(rt, mcpMgr, mcpStore, namespace)
	}
	ConfigureHttpRuntime(rt, secretResolver, namespace)
	ConfigureCliRuntime(rt, secretResolver, nil, cliConfig, namespace)
	ConfigureExternalRuntime(rt, secretResolver, namespace)
	ConfigureGRPCRuntime(rt, secretResolver, namespace)
	ConfigureWebhookCallbackRuntime(rt, secretResolver, namespace)
	ConfigureWasmRuntime(rt, wasmRT, namespace)
	return rt, nil
}

// AdaptTaskInputViaContextAdapter runs the ContextAdapter tool chain on raw task input maps.
func AdaptTaskInputViaContextAdapter(
	ctx context.Context,
	namespace string,
	adapterStoreKey string,
	lookup ContextAdapterGetter,
	deps ContextAdapterDeps,
	raw map[string]string,
) (map[string]string, error) {
	key := strings.TrimSpace(adapterStoreKey)
	if key == "" {
		return raw, nil
	}
	item, ok, err := lookup.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("context adapter %q not found", adapterStoreKey)
	}
	rt, err := BuildSanitizerToolRuntime(
		ctx,
		namespace,
		item.Spec.ToolRef,
		deps.Tools,
		deps.Isolated,
		deps.Wasm,
		deps.SecretResolver,
		deps.Cli,
		deps.McpMgr,
		deps.McpStore,
	)
	if err != nil {
		return nil, err
	}
	hook := NewToolBackedContextAdapter(item.Spec, rt)
	return hook.AdaptContext(ctx, raw)
}
