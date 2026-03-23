package controllers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

const (
	mcpOwnerLabel     = "orloj.dev/mcp-server"
	mcpGeneratedLabel = "orloj.dev/mcp-generated"
)

// McpServerController reconciles McpServer resources. When a session manager
// is configured, it connects to MCP servers, discovers tools via tools/list,
// and auto-generates Tool resources for each discovered tool.
type McpServerController struct {
	store          *store.McpServerStore
	toolStore      *store.ToolStore
	sessionManager *agentruntime.McpSessionManager
	reconcileEvery time.Duration
	logger         *log.Logger
}

func NewMcpServerController(mcpStore *store.McpServerStore, toolStore *store.ToolStore, logger *log.Logger, reconcileEvery time.Duration) *McpServerController {
	if reconcileEvery <= 0 {
		reconcileEvery = 10 * time.Second
	}
	return &McpServerController{
		store:          mcpStore,
		toolStore:      toolStore,
		reconcileEvery: reconcileEvery,
		logger:         logger,
	}
}

func (c *McpServerController) SetSessionManager(sm *agentruntime.McpSessionManager) {
	c.sessionManager = sm
}

func (c *McpServerController) Start(ctx context.Context) {
	queue := newKeyQueue(256)
	go c.runWorker(ctx, queue)

	ticker := time.NewTicker(c.reconcileEvery)
	defer ticker.Stop()

	for {
		c.enqueueAll(ctx, queue)
		select {
		case <-ctx.Done():
			if c.sessionManager != nil {
				c.sessionManager.Close()
			}
			return
		case <-ticker.C:
		}
	}
}

func (c *McpServerController) runWorker(ctx context.Context, queue *keyQueue) {
	for {
		key, ok := queue.Pop(ctx)
		if !ok {
			return
		}
		if err := c.reconcileByName(ctx, key); err != nil && c.logger != nil {
			c.logger.Printf("mcp-server controller reconcile error: %v", err)
		}
		queue.Done(key)
	}
}

func (c *McpServerController) enqueueAll(ctx context.Context, queue *keyQueue) {
	_itemList, err := c.store.List(ctx)
	if err != nil {
		return
	}
	for _, item := range _itemList {
		queue.Enqueue(store.ScopedName(item.Metadata.Namespace, item.Metadata.Name))
	}
}

func (c *McpServerController) ReconcileOnce(ctx context.Context) error {
	_itemList, err := c.store.List(ctx)
	if err != nil {
		return err
	}
	for _, item := range _itemList {
		if err := c.reconcileByName(ctx, store.ScopedName(item.Metadata.Namespace, item.Metadata.Name)); err != nil {
			return err
		}
	}
	return nil
}

func (c *McpServerController) reconcileByName(ctx context.Context, name string) error {
	server, ok, err := c.store.Get(ctx, name)
	if err != nil {
		return err
	}
	if !ok {
		c.garbageCollectTools(ctx, name)
		return nil
	}

	if server.Status.Phase == "Ready" && server.Status.ObservedGeneration == server.Metadata.Generation {
		return nil
	}

	if c.sessionManager == nil {
		server.Status.Phase = "Ready"
		server.Status.LastError = ""
		server.Status.ObservedGeneration = server.Metadata.Generation
		_, err = c.store.Upsert(ctx, server)
		return err
	}

	server.Status.Phase = "Connecting"
	server.Status.LastError = ""
	if updated, err := c.store.Upsert(ctx, server); err == nil {
		server = updated
	}

	session, err := c.sessionManager.GetOrCreate(ctx, server)
	if err != nil {
		server.Status.Phase = "Error"
		server.Status.LastError = err.Error()
		if updated, uErr := c.store.Upsert(ctx, server); uErr == nil {
			server = updated
		}
		return fmt.Errorf("connect to mcp server %s: %w", name, err)
	}

	tools, err := session.Transport.ListTools(ctx)
	if err != nil {
		server.Status.Phase = "Error"
		server.Status.LastError = fmt.Sprintf("tools/list: %s", err.Error())
		if updated, uErr := c.store.Upsert(ctx, server); uErr == nil {
			server = updated
		}
		return fmt.Errorf("list tools from mcp server %s: %w", name, err)
	}

	discovered := make([]string, 0, len(tools))
	for _, t := range tools {
		discovered = append(discovered, t.Name)
	}

	filteredTools := filterTools(tools, server.Spec.ToolFilter.Include)
	generatedNames, err := c.syncTools(ctx, server, filteredTools)
	if err != nil {
		server.Status.Phase = "Error"
		server.Status.LastError = fmt.Sprintf("sync tools: %s", err.Error())
		if updated, uErr := c.store.Upsert(ctx, server); uErr == nil {
			server = updated
		}
		return err
	}

	c.deleteOrphanedTools(ctx, server, generatedNames)

	server.Status.Phase = "Ready"
	server.Status.LastError = ""
	server.Status.DiscoveredTools = discovered
	server.Status.GeneratedTools = generatedNames
	server.Status.LastSyncedAt = time.Now().UTC().Format(time.RFC3339)
	server.Status.ObservedGeneration = server.Metadata.Generation
	_, err = c.store.Upsert(ctx, server)
	return err
}

func (c *McpServerController) syncTools(ctx context.Context, server resources.McpServer, tools []agentruntime.McpToolDefinition) ([]string, error) {
	ns := resources.NormalizeNamespace(server.Metadata.Namespace)
	serverName := strings.TrimSpace(server.Metadata.Name)
	generated := make([]string, 0, len(tools))

	for _, mcpTool := range tools {
		toolName := generatedToolName(serverName, mcpTool.Name)
		generated = append(generated, toolName)

		desc := mcpTool.Description
		if len(desc) > 4096 {
			desc = desc[:4096]
		}

		tool := resources.Tool{
			APIVersion: "orloj.dev/v1",
			Kind:       "Tool",
			Metadata: resources.ObjectMeta{
				Name:      toolName,
				Namespace: ns,
				Labels: map[string]string{
					mcpOwnerLabel:     serverName,
					mcpGeneratedLabel: "true",
				},
			},
			Spec: resources.ToolSpec{
				Type:         "mcp",
				McpServerRef: serverName,
				McpToolName:  mcpTool.Name,
				Description:  desc,
				InputSchema:  mcpTool.InputSchema,
			},
		}

		if _, err := c.toolStore.Upsert(ctx, tool); err != nil {
			return generated, fmt.Errorf("upsert generated tool %s: %w", toolName, err)
		}
	}
	return generated, nil
}

func (c *McpServerController) deleteOrphanedTools(ctx context.Context, server resources.McpServer, currentNames []string) {
	serverName := strings.TrimSpace(server.Metadata.Name)
	current := make(map[string]struct{}, len(currentNames))
	for _, name := range currentNames {
		current[name] = struct{}{}
	}
	_toolList, err := c.toolStore.List(ctx)
	if err != nil {
		return
	}
	for _, tool := range _toolList {
		if tool.Metadata.Labels[mcpOwnerLabel] != serverName {
			continue
		}
		if _, ok := current[tool.Metadata.Name]; ok {
			continue
		}
		key := store.ScopedName(tool.Metadata.Namespace, tool.Metadata.Name)
		if err := c.toolStore.Delete(ctx, key); err != nil && c.logger != nil {
			c.logger.Printf("mcp-server controller: failed to delete orphaned tool %s: %v", key, err)
		}
	}
}

func (c *McpServerController) garbageCollectTools(ctx context.Context, serverKey string) {
	parts := strings.SplitN(serverKey, "/", 2)
	serverName := serverKey
	if len(parts) == 2 {
		serverName = parts[1]
	}
	_toolList, err := c.toolStore.List(ctx)
	if err != nil {
		return
	}
	for _, tool := range _toolList {
		if tool.Metadata.Labels[mcpOwnerLabel] != serverName {
			continue
		}
		key := store.ScopedName(tool.Metadata.Namespace, tool.Metadata.Name)
		if err := c.toolStore.Delete(ctx, key); err != nil && c.logger != nil {
			c.logger.Printf("mcp-server controller: gc tool %s failed: %v", key, err)
		}
	}
}

func filterTools(tools []agentruntime.McpToolDefinition, include []string) []agentruntime.McpToolDefinition {
	if len(include) == 0 {
		return tools
	}
	allowed := make(map[string]struct{}, len(include))
	for _, name := range include {
		allowed[strings.TrimSpace(name)] = struct{}{}
	}
	filtered := make([]agentruntime.McpToolDefinition, 0, len(tools))
	for _, t := range tools {
		if _, ok := allowed[t.Name]; ok {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func generatedToolName(serverName, mcpToolName string) string {
	safe := strings.ReplaceAll(strings.TrimSpace(mcpToolName), "_", "-")
	safe = strings.ReplaceAll(safe, ".", "-")
	return strings.TrimSpace(serverName) + "--" + safe
}
