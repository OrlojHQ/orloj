package controllers

import (
	"context"
	"log"
	"time"

	"github.com/OrlojHQ/orloj/store"
)

// ToolController reconciles Tool resources.
type ToolController struct {
	store          *store.ToolStore
	reconcileEvery time.Duration
	logger         *log.Logger
}

func NewToolController(store *store.ToolStore, logger *log.Logger, reconcileEvery time.Duration) *ToolController {
	if reconcileEvery <= 0 {
		reconcileEvery = 5 * time.Second
	}
	return &ToolController{store: store, logger: logger, reconcileEvery: reconcileEvery}
}

func (c *ToolController) Start(ctx context.Context) {
	queue := newKeyQueue(1024)
	go c.runWorker(ctx, queue)

	ticker := time.NewTicker(c.reconcileEvery)
	defer ticker.Stop()

	for {
		c.enqueueAll(ctx, queue)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *ToolController) runWorker(ctx context.Context, queue *keyQueue) {
	for {
		key, ok := queue.Pop(ctx)
		if !ok {
			return
		}
		if err := c.reconcileByName(ctx, key); err != nil && c.logger != nil {
			c.logger.Printf("tool controller reconcile error: %v", err)
		}
		queue.Done(key)
	}
}

func (c *ToolController) enqueueAll(ctx context.Context, queue *keyQueue) {
	_itemList, err := c.store.List(ctx)
	if err != nil {
		return
	}
	for _, item := range _itemList {
		queue.Enqueue(store.ScopedName(item.Metadata.Namespace, item.Metadata.Name))
	}
}

func (c *ToolController) ReconcileOnce(ctx context.Context) error {
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

func (c *ToolController) reconcileByName(ctx context.Context, name string) error {
	item, ok, err := c.store.Get(ctx, name)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if item.Status.Phase == "Ready" && item.Status.ObservedGeneration == item.Metadata.Generation {
		return nil
	}
	item.Status.Phase = "Ready"
	item.Status.LastError = ""
	item.Status.ObservedGeneration = item.Metadata.Generation
	_, err = c.store.Upsert(ctx, item)
	return err
}
