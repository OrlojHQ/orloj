package controllers

import (
	"context"
	"log"
	"time"

	"github.com/OrlojHQ/orloj/store"
)

// PolicyController reconciles AgentPolicy resources.
type PolicyController struct {
	store          *store.AgentPolicyStore
	reconcileEvery time.Duration
	logger         *log.Logger
}

func NewPolicyController(store *store.AgentPolicyStore, logger *log.Logger, reconcileEvery time.Duration) *PolicyController {
	if reconcileEvery <= 0 {
		reconcileEvery = 5 * time.Second
	}
	return &PolicyController{store: store, logger: logger, reconcileEvery: reconcileEvery}
}

func (c *PolicyController) Start(ctx context.Context) {
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

func (c *PolicyController) runWorker(ctx context.Context, queue *keyQueue) {
	for {
		key, ok := queue.Pop(ctx)
		if !ok {
			return
		}
		if err := c.reconcileByName(key); err != nil && c.logger != nil {
			c.logger.Printf("policy controller reconcile error: %v", err)
		}
		queue.Done(key)
	}
}

func (c *PolicyController) enqueueAll(queue *keyQueue) {
	for _, item := range c.store.List() {
		queue.Enqueue(item.Metadata.Name)
	}
}

func (c *PolicyController) ReconcileOnce(_ context.Context) error {
	for _, item := range c.store.List() {
		if err := c.reconcileByName(item.Metadata.Name); err != nil {
			return err
		}
	}
	return nil
}

func (c *PolicyController) reconcileByName(name string) error {
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
