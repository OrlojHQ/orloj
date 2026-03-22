package controllers

import (
	"context"
	"sync"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/runtime"
)

type captureMeteringSink struct {
	mu     sync.Mutex
	events []agentruntime.MeteringEvent
}

func (c *captureMeteringSink) RecordMetering(_ context.Context, event agentruntime.MeteringEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *captureMeteringSink) snapshot() []agentruntime.MeteringEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]agentruntime.MeteringEvent, len(c.events))
	copy(out, c.events)
	return out
}

type captureAuditSink struct {
	mu     sync.Mutex
	events []agentruntime.AuditEvent
}

func (c *captureAuditSink) RecordAudit(_ context.Context, event agentruntime.AuditEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *captureAuditSink) snapshot() []agentruntime.AuditEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]agentruntime.AuditEvent, len(c.events))
	copy(out, c.events)
	return out
}

func TestTaskControllerEmitsExtensionEventsSequential(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	metering := &captureMeteringSink{}
	audit := &captureAuditSink{}
	controller.SetExtensions(agentruntime.Extensions{
		Metering: metering,
		Audit:    audit,
	})

	if _, err := stores.agentStore.Upsert(resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "planner-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "plan a short update",
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := stores.agentSystemStore.Upsert(resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "report-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"planner-agent"},
		},
	}); err != nil {
		t.Fatalf("upsert agentsystem failed: %v", err)
	}
	if _, err := stores.taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "report-task"},
		Spec:       resources.TaskSpec{System: "report-system", Input: map[string]string{"topic": "status"}},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	task, ok, err := stores.taskStore.Get("report-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected succeeded task phase, got %q", task.Status.Phase)
	}

	meteringEvents := metering.snapshot()
	if len(meteringEvents) == 0 {
		t.Fatal("expected metering events")
	}
	if !containsMeteringType(meteringEvents, "task.attempt_started") {
		t.Fatalf("expected task.attempt_started metering event, got %+v", meteringEvents)
	}
	if !containsMeteringType(meteringEvents, "agent.execution") {
		t.Fatalf("expected agent.execution metering event, got %+v", meteringEvents)
	}
	if !containsMeteringType(meteringEvents, "task.completed") {
		t.Fatalf("expected task.completed metering event, got %+v", meteringEvents)
	}

	auditEvents := audit.snapshot()
	if len(auditEvents) == 0 {
		t.Fatal("expected audit events")
	}
	if !containsAuditAction(auditEvents, "task.completed") {
		t.Fatalf("expected task.completed audit event, got %+v", auditEvents)
	}
}

func containsMeteringType(events []agentruntime.MeteringEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func containsAuditAction(events []agentruntime.AuditEvent, action string) bool {
	for _, event := range events {
		if event.Action == action {
			return true
		}
	}
	return false
}
