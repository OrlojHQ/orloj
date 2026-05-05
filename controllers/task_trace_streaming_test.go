package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
)

func TestTaskControllerPublishesTraceEventsToEventBus(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	bus := eventbus.NewMemoryBus(256)
	controller.SetEventBus(bus)

	if _, err := stores.agentStore.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "streamer-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "test streaming",
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "2s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := stores.agentSystemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "stream-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"streamer-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "stream-task"},
		Spec:       resources.TaskSpec{System: "stream-system", Input: map[string]string{"topic": "test"}},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := bus.Subscribe(ctx, eventbus.Filter{Type: "task.trace", Name: "stream-task"})

	if err := controller.ReconcileOnce(ctx); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	task, ok, _ := stores.taskStore.Get(ctx, "stream-task")
	if !ok {
		t.Fatal("task not found after reconcile")
	}
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected Succeeded, got %q (error: %s)", task.Status.Phase, task.Status.LastError)
	}

	var traceEvents []eventbus.Event
	timeout := time.After(2 * time.Second)
	for {
		select {
		case evt, ok := <-stream:
			if !ok {
				goto done
			}
			traceEvents = append(traceEvents, evt)
		case <-timeout:
			goto done
		}
	}
done:
	if len(traceEvents) == 0 {
		t.Fatal("expected task.trace events published to event bus, got none")
	}

	var hasModelCall bool
	for _, evt := range traceEvents {
		if evt.Type != "task.trace" {
			t.Fatalf("unexpected event type %q in trace stream", evt.Type)
		}
		if evt.Name != "stream-task" {
			t.Fatalf("expected event name stream-task, got %q", evt.Name)
		}
		if data, ok := evt.Data.(agentruntime.AgentStepEvent); ok {
			if data.Type == "model_call" {
				hasModelCall = true
			}
		}
	}
	if !hasModelCall {
		t.Fatal("expected model_call event in trace stream")
	}
}

func TestTaskControllerIntermediateUpsertMultiAgent(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	bus := eventbus.NewMemoryBus(256)
	controller.SetEventBus(bus)

	for _, name := range []string{"agent-a", "agent-b"} {
		if _, err := stores.agentStore.Upsert(context.Background(), resources.Agent{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: name},
			Spec: resources.AgentSpec{
				ModelRef: "openai-default",
				Prompt:   "do work",
				Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "2s"},
			},
		}); err != nil {
			t.Fatalf("upsert agent %s failed: %v", name, err)
		}
	}
	if _, err := stores.agentSystemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "multi-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"agent-a", "agent-b"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "multi-task"},
		Spec:       resources.TaskSpec{System: "multi-system", Input: map[string]string{"topic": "x"}},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	task, ok, _ := stores.taskStore.Get(context.Background(), "multi-task")
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected Succeeded, got %q (error: %s)", task.Status.Phase, task.Status.LastError)
	}
	if len(task.Status.Trace) == 0 {
		t.Fatal("expected trace events persisted on task")
	}

	var agentEndCount int
	for _, te := range task.Status.Trace {
		if te.Type == "agent_end" {
			agentEndCount++
		}
	}
	if agentEndCount < 2 {
		t.Fatalf("expected at least 2 agent_end trace events (one per agent), got %d", agentEndCount)
	}
}
