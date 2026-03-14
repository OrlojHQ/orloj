package controllers

import (
	"context"
	"log"
	"time"

	"github.com/AnonJon/orloj/store"
)

// AgentSystemController reconciles AgentSystem resources.
type AgentSystemController struct {
	store          *store.AgentSystemStore
	reconcileEvery time.Duration
	logger         *log.Logger
}

func NewAgentSystemController(store *store.AgentSystemStore, logger *log.Logger, reconcileEvery time.Duration) *AgentSystemController {
	if reconcileEvery <= 0 {
		reconcileEvery = 2 * time.Second
	}
	return &AgentSystemController{store: store, logger: logger, reconcileEvery: reconcileEvery}
}

func (c *AgentSystemController) Start(ctx context.Context) {
	queue := newKeyQueue(1024)
	go c.runWorker(ctx, queue)

	ticker := time.NewTicker(c.reconcileEvery)
	defer ticker.Stop()

	for {
		c.enqueueAll(queue)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *AgentSystemController) runWorker(ctx context.Context, queue *keyQueue) {
	for {
		key, ok := queue.Pop(ctx)
		if !ok {
			return
		}
		if err := c.reconcileByName(key); err != nil && c.logger != nil {
			c.logger.Printf("agentsystem controller reconcile error: %v", err)
		}
		queue.Done(key)
	}
}

func (c *AgentSystemController) enqueueAll(queue *keyQueue) {
	for _, item := range c.store.List() {
		queue.Enqueue(store.ScopedName(item.Metadata.Namespace, item.Metadata.Name))
	}
}

func (c *AgentSystemController) ReconcileOnce(_ context.Context) error {
	for _, item := range c.store.List() {
		if err := c.reconcileByName(store.ScopedName(item.Metadata.Namespace, item.Metadata.Name)); err != nil {
			return err
		}
	}
	return nil
}

func (c *AgentSystemController) reconcileByName(name string) error {
	item, ok := c.store.Get(name)
	if !ok {
		return nil
	}
	if item.Status.Phase == "Ready" && item.Status.ObservedGeneration == item.Metadata.Generation {
		return nil
	}
	item.Status.Phase = "Ready"
	item.Status.LastError = ""
	item.Status.ObservedGeneration = item.Metadata.Generation
	_, err := c.store.Upsert(item)
	return err
}
