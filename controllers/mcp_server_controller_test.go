package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

func TestMcpServerControllerStatusReconcileDoesNotBumpGeneration(t *testing.T) {
	ctx := context.Background()
	mcpStore := store.NewMcpServerStore()
	toolStore := store.NewToolStore()

	created, err := mcpStore.Upsert(ctx, resources.McpServer{
		APIVersion: "orloj.dev/v1",
		Kind:       "McpServer",
		Metadata:   resources.ObjectMeta{Name: "context7"},
		Spec: resources.McpServerSpec{
			Transport: "stdio",
			Command:   "npx",
			Args:      []string{"@upstash/context7-mcp"},
		},
	})
	if err != nil {
		t.Fatalf("create mcp server failed: %v", err)
	}

	controller := NewMcpServerController(mcpStore, toolStore, nil, time.Second)
	if err := controller.ReconcileOnce(ctx); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	updated, ok, err := mcpStore.Get(ctx, "context7")
	if err != nil {
		t.Fatalf("get mcp server failed: %v", err)
	}
	if !ok {
		t.Fatal("expected mcp server to exist")
	}
	if updated.Metadata.Generation != created.Metadata.Generation {
		t.Fatalf("reconcile bumped generation: before=%d after=%d", created.Metadata.Generation, updated.Metadata.Generation)
	}
	if updated.Status.Phase != "Ready" {
		t.Fatalf("expected Ready phase, got %q", updated.Status.Phase)
	}
	if updated.Status.ObservedGeneration != created.Metadata.Generation {
		t.Fatalf("expected observed generation=%d, got %d", created.Metadata.Generation, updated.Status.ObservedGeneration)
	}

	rvAfterFirstReconcile := updated.Metadata.ResourceVersion
	if err := controller.ReconcileOnce(ctx); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}
	unchanged, ok, err := mcpStore.Get(ctx, "context7")
	if err != nil {
		t.Fatalf("get mcp server after second reconcile failed: %v", err)
	}
	if !ok {
		t.Fatal("expected mcp server to exist after second reconcile")
	}
	if unchanged.Metadata.ResourceVersion != rvAfterFirstReconcile {
		t.Fatalf("ready/observed reconcile should be a no-op: before rv=%q after rv=%q", rvAfterFirstReconcile, unchanged.Metadata.ResourceVersion)
	}
}
