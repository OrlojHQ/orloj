package controllers

import (
	"context"
	"log"
	"time"

	"github.com/OrlojHQ/orloj/store"
)

// SecretController reconciles Secret resources.
type SecretController struct {
	store          *store.SecretStore
	reconcileEvery time.Duration
	logger         *log.Logger
}

func NewSecretController(store *store.SecretStore, logger *log.Logger, reconcileEvery time.Duration) *SecretController {
	if reconcileEvery <= 0 {
		reconcileEvery = 5 * time.Second
	}
	return &SecretController{store: store, logger: logger, reconcileEvery: reconcileEvery}
}

func (c *SecretController) Start(ctx context.Context) {
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

func (c *SecretController) runWorker(ctx context.Context, queue *keyQueue) {
	for {
		key, ok := queue.Pop(ctx)
		if !ok {
			return
		}
		if err := c.reconcileByName(key); err != nil && c.logger != nil {
			c.logger.Printf("secret controller reconcile error: %v", err)
		}
		queue.Done(key)
	}
}

func (c *SecretController) enqueueAll(queue *keyQueue) {
	_itemList, err := c.store.List()
	if err != nil {
		return
	}
	for _, item := range _itemList {
		queue.Enqueue(store.ScopedName(item.Metadata.Namespace, item.Metadata.Name))
	}
}

func (c *SecretController) ReconcileOnce(_ context.Context) error {
	_itemList, err := c.store.List()
	if err != nil {
		return err
	}
	for _, item := range _itemList {
		if err := c.reconcileByName(store.ScopedName(item.Metadata.Namespace, item.Metadata.Name)); err != nil {
			return err
		}
	}
	return nil
}

func (c *SecretController) reconcileByName(name string) error {
	item, ok, err := c.store.Get(name)
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
	_, err = c.store.Upsert(item)
	return err
}
