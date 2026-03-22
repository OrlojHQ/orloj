package controllers

import (
	"context"
	"log"
	"time"

	"github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

// AgentController reconciles desired Agent resources to active runtime workers.
type AgentController struct {
	store          *store.AgentStore
	runtime        *agentruntime.Manager
	reconcileEvery time.Duration
	logger         *log.Logger
}

func NewAgentController(store *store.AgentStore, runtime *agentruntime.Manager, logger *log.Logger, reconcileEvery time.Duration) *AgentController {
	if reconcileEvery <= 0 {
		reconcileEvery = 2 * time.Second
	}
	return &AgentController{store: store, runtime: runtime, reconcileEvery: reconcileEvery, logger: logger}
}

func (c *AgentController) Start(ctx context.Context) {
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

func (c *AgentController) runWorker(ctx context.Context, queue *keyQueue) {
	for {
		key, ok := queue.Pop(ctx)
		if !ok {
			return
		}
		if err := c.reconcileByName(key); err != nil && c.logger != nil {
			c.logger.Printf("agent controller reconcile error: %v", err)
		}
		queue.Done(key)
	}
}

func (c *AgentController) enqueueAll(queue *keyQueue) {
	_agentList, err := c.store.List()
	if err != nil {
		return
	}
	for _, agent := range _agentList {
		queue.Enqueue(store.ScopedName(agent.Metadata.Namespace, agent.Metadata.Name))
	}
	for _, running := range c.runtime.RunningAgents() {
		queue.Enqueue(running)
	}
}

func (c *AgentController) ReconcileOnce(_ context.Context) error {
	desired := make(map[string]struct{})
	_agentList, err := c.store.List()
	if err != nil {
		return err
	}
	for _, agent := range _agentList {
		desired[store.ScopedName(agent.Metadata.Namespace, agent.Metadata.Name)] = struct{}{}
		c.runtime.EnsureRunning(agent)
	}

	for _, running := range c.runtime.RunningAgents() {
		if _, ok := desired[running]; !ok {
			c.runtime.Stop(running)
		}
	}
	return nil
}

func (c *AgentController) reconcileByName(name string) error {
	agent, ok, err := c.store.Get(name)
	if err != nil {
		return err
	}
	if !ok {
		c.runtime.Stop(name)
		return nil
	}
	c.runtime.EnsureRunning(agent)
	return nil
}
