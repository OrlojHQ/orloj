package agentruntime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

type extensionCaptureMeteringSink struct {
	mu     sync.Mutex
	events []MeteringEvent
}

func (c *extensionCaptureMeteringSink) RecordMetering(_ context.Context, event MeteringEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *extensionCaptureMeteringSink) snapshot() []MeteringEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]MeteringEvent, len(c.events))
	copy(out, c.events)
	return out
}

type extensionCaptureAuditSink struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (c *extensionCaptureAuditSink) RecordAudit(_ context.Context, event AuditEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *extensionCaptureAuditSink) snapshot() []AuditEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]AuditEvent, len(c.events))
	copy(out, c.events)
	return out
}

func TestAgentMessageConsumerEmitsExtensionEvents(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()

	if _, err := agentStore.Upsert(resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "planner-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "plan",
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}

	if _, err := systemStore.Upsert(resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "report-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"planner-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}

	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "task-extensions"},
		Spec:       resources.TaskSpec{System: "report-system"},
		Status: resources.TaskStatus{
			Phase:     "Running",
			ClaimedBy: "worker-a",
			Attempts:  1,
		},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	metering := &extensionCaptureMeteringSink{}
	audit := &extensionCaptureAuditSink{}
	manager := NewAgentMessageConsumerManager(
		bus,
		agentStore,
		systemStore,
		taskStore,
		nil,
		AgentMessageConsumerOptions{ModelEndpoints: newTestModelEndpointStore(t),
			WorkerID:            "worker-a",
			RefreshEvery:        20 * time.Millisecond,
			DedupeWindow:        time.Minute,
			LeaseExtendDuration: 30 * time.Second,
			Extensions: Extensions{
				Metering: metering,
				Audit:    audit,
			},
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Start(ctx)
	time.Sleep(40 * time.Millisecond)

	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "msg-ext-1",
		TaskID:    "default/task-extensions",
		Namespace: "default",
		FromAgent: "system",
		ToAgent:   "planner-agent",
		Type:      "task_start",
		Payload:   "start",
		Attempt:   1,
		TraceID:   "default/task-extensions/a001",
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	waitForConsumer(t, 2*time.Second, func() bool {
		task, ok := taskStore.Get("task-extensions")
		return ok && task.Status.Phase == "Succeeded"
	})

	meteringEvents := metering.snapshot()
	if len(meteringEvents) == 0 {
		t.Fatal("expected metering events")
	}
	if !hasMeteringEventType(meteringEvents, "message.attempt_started") {
		t.Fatalf("expected message.attempt_started event, got %+v", meteringEvents)
	}
	if !hasMeteringEventType(meteringEvents, "message.completed") {
		t.Fatalf("expected message.completed event, got %+v", meteringEvents)
	}

	auditEvents := audit.snapshot()
	if len(auditEvents) == 0 {
		t.Fatal("expected audit events")
	}
	if !hasAuditAction(auditEvents, "message.completed") {
		t.Fatalf("expected message.completed audit action, got %+v", auditEvents)
	}
}

func hasMeteringEventType(events []MeteringEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func hasAuditAction(events []AuditEvent, action string) bool {
	for _, event := range events {
		if event.Action == action {
			return true
		}
	}
	return false
}
