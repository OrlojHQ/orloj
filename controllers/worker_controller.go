package controllers

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/AnonJon/orloj/crds"
	"github.com/AnonJon/orloj/eventbus"
	"github.com/AnonJon/orloj/store"
)

// WorkerController reconciles Worker heartbeats into readiness status.
type WorkerController struct {
	store          *store.WorkerStore
	reconcileEvery time.Duration
	staleAfter     time.Duration
	logger         *log.Logger
	eventBus       eventbus.Bus
}

func NewWorkerController(store *store.WorkerStore, logger *log.Logger, reconcileEvery, staleAfter time.Duration) *WorkerController {
	if reconcileEvery <= 0 {
		reconcileEvery = 2 * time.Second
	}
	if staleAfter <= 0 {
		staleAfter = 20 * time.Second
	}
	return &WorkerController{
		store:          store,
		reconcileEvery: reconcileEvery,
		staleAfter:     staleAfter,
		logger:         logger,
	}
}

func (c *WorkerController) Start(ctx context.Context) {
	queue := newKeyQueue(1024)
	go c.runWorker(ctx, queue)
	var eventCh <-chan eventbus.Event
	if c.eventBus != nil {
		eventCh = c.eventBus.Subscribe(ctx, eventbus.Filter{
			Source: "apiserver",
			Kind:   "Worker",
		})
	}

	ticker := time.NewTicker(c.reconcileEvery)
	defer ticker.Stop()

	for {
		c.enqueueAll(queue)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-eventCh:
		}
	}
}

func (c *WorkerController) SetEventBus(bus eventbus.Bus) {
	c.eventBus = bus
}

func (c *WorkerController) ReconcileOnce() error {
	queue := newKeyQueue(1024)
	c.enqueueAll(queue)

	for {
		select {
		case key := <-queue.ch:
			if err := c.reconcileByName(key); err != nil {
				return err
			}
			queue.Done(key)
		default:
			return nil
		}
	}
}

func (c *WorkerController) runWorker(ctx context.Context, queue *keyQueue) {
	for {
		key, ok := queue.Pop(ctx)
		if !ok {
			return
		}
		if err := c.reconcileByName(key); err != nil && c.logger != nil {
			c.logger.Printf("worker controller reconcile error: %v", err)
		}
		queue.Done(key)
	}
}

func (c *WorkerController) enqueueAll(queue *keyQueue) {
	for _, item := range c.store.List() {
		queue.Enqueue(store.ScopedName(item.Metadata.Namespace, item.Metadata.Name))
	}
}

func (c *WorkerController) reconcileByName(name string) error {
	item, ok := c.store.Get(name)
	if !ok {
		return nil
	}

	now := time.Now().UTC()
	last := strings.TrimSpace(item.Status.LastHeartbeat)
	if last == "" {
		item.Status.Phase = "NotReady"
		item.Status.LastError = "no heartbeat received"
		item.Status.ObservedGeneration = item.Metadata.Generation
		_, err := c.store.Upsert(item)
		c.publishWorkerEvent(item, "worker.not_ready", item.Status.LastError)
		return err
	}

	lastHeartbeat, err := parseControllerTimestamp(last)
	if err != nil {
		item.Status.Phase = "NotReady"
		item.Status.LastError = "invalid heartbeat timestamp"
		item.Status.ObservedGeneration = item.Metadata.Generation
		_, upsertErr := c.store.Upsert(item)
		c.publishWorkerEvent(item, "worker.not_ready", item.Status.LastError)
		if upsertErr != nil {
			return upsertErr
		}
		return err
	}

	if now.Sub(lastHeartbeat) > c.staleAfter {
		if item.Status.Phase != "NotReady" {
			item.Status.Phase = "NotReady"
			item.Status.LastError = "worker heartbeat stale"
			item.Status.ObservedGeneration = item.Metadata.Generation
			_, err := c.store.Upsert(item)
			c.publishWorkerEvent(item, "worker.not_ready", item.Status.LastError)
			return err
		}
		return nil
	}

	if item.Status.Phase != "Ready" || item.Status.ObservedGeneration != item.Metadata.Generation {
		item.Status.Phase = "Ready"
		item.Status.LastError = ""
		item.Status.ObservedGeneration = item.Metadata.Generation
		_, err := c.store.Upsert(item)
		c.publishWorkerEvent(item, "worker.ready", "worker ready")
		return err
	}
	return nil
}

func (c *WorkerController) publishWorkerEvent(worker crds.Worker, eventType string, message string) {
	if c.eventBus == nil {
		return
	}
	c.eventBus.Publish(eventbus.Event{
		Source:    "worker-controller",
		Type:      strings.TrimSpace(eventType),
		Kind:      "Worker",
		Name:      worker.Metadata.Name,
		Namespace: crds.NormalizeNamespace(worker.Metadata.Namespace),
		Action:    strings.ToLower(strings.TrimSpace(worker.Status.Phase)),
		Message:   strings.TrimSpace(message),
		Data: map[string]any{
			"phase":         worker.Status.Phase,
			"lastHeartbeat": worker.Status.LastHeartbeat,
			"lastError":     worker.Status.LastError,
		},
	})
}
