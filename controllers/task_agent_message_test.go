package controllers

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/runtime"
)

type captureAgentMessageBus struct {
	mu         sync.Mutex
	published  []agentruntime.AgentMessage
	publishErr error
}

func (b *captureAgentMessageBus) Publish(_ context.Context, message agentruntime.AgentMessage) (agentruntime.AgentMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, message)
	if b.publishErr != nil {
		return agentruntime.AgentMessage{}, b.publishErr
	}
	return message, nil
}

func (b *captureAgentMessageBus) Consume(context.Context, agentruntime.AgentMessageSubscription, agentruntime.AgentMessageHandler) error {
	return nil
}

func (b *captureAgentMessageBus) Close() error {
	return nil
}

func (b *captureAgentMessageBus) Published() []agentruntime.AgentMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]agentruntime.AgentMessage, len(b.published))
	copy(out, b.published)
	return out
}

func TestTaskControllerPublishesAgentHandoffMessages(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	bus := &captureAgentMessageBus{}
	controller.SetAgentMessageBus(bus)

	for _, agent := range []resources.Agent{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "planner-agent"},
			Spec: resources.AgentSpec{
				ModelRef: "openai-default",
				Prompt:   "plan",
				Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "writer-agent"},
			Spec: resources.AgentSpec{
				ModelRef: "openai-default",
				Prompt:   "write",
				Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
			},
		},
	} {
		if _, err := stores.agentStore.Upsert(context.Background(), agent); err != nil {
			t.Fatalf("upsert agent %s: %v", agent.Metadata.Name, err)
		}
	}

	if _, err := stores.agentSystemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "message-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"planner-agent", "writer-agent"},
			Graph: map[string]resources.GraphEdge{
				"planner-agent": {Next: "writer-agent"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system: %v", err)
	}

	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "message-task"},
		Spec:       resources.TaskSpec{System: "message-system", Input: map[string]string{"topic": "agents"}},
	}); err != nil {
		t.Fatalf("upsert task: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	task, ok, err := stores.taskStore.Get(context.Background(), "message-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected task Succeeded, got %q", task.Status.Phase)
	}
	if len(task.Status.Messages) != 1 {
		t.Fatalf("expected 1 task message, got %d", len(task.Status.Messages))
	}
	message := task.Status.Messages[0]
	if message.FromAgent != "planner-agent" {
		t.Fatalf("expected from planner-agent, got %q", message.FromAgent)
	}
	if message.ToAgent != "writer-agent" {
		t.Fatalf("expected to writer-agent, got %q", message.ToAgent)
	}
	if message.Type != "task_handoff" {
		t.Fatalf("expected task_handoff message type, got %q", message.Type)
	}
	if message.TaskID != "default/message-task" {
		t.Fatalf("expected task id default/message-task, got %q", message.TaskID)
	}
	if message.Attempt != 1 {
		t.Fatalf("expected attempt=1, got %d", message.Attempt)
	}
	expectedID := deterministicTaskMessageID("default", "message-task", 1, 1, "planner-agent", "writer-agent")
	if message.MessageID != expectedID {
		t.Fatalf("expected deterministic message id %q, got %q", expectedID, message.MessageID)
	}
	if message.TraceID != "default/message-task/a001" {
		t.Fatalf("expected trace id default/message-task/a001, got %q", message.TraceID)
	}

	published := bus.Published()
	if len(published) != 1 {
		t.Fatalf("expected 1 published envelope, got %d", len(published))
	}
	if published[0].MessageID != message.MessageID {
		t.Fatalf("expected published message id %q, got %q", message.MessageID, published[0].MessageID)
	}
	if published[0].Namespace != "default" {
		t.Fatalf("expected published namespace default, got %q", published[0].Namespace)
	}
}

func TestTaskControllerFailsTaskWhenMessagePublishFails(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	controller.SetAgentMessageBus(&captureAgentMessageBus{publishErr: errors.New("broker unavailable")})

	for _, agent := range []resources.Agent{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "planner-agent"},
			Spec: resources.AgentSpec{
				ModelRef: "openai-default",
				Prompt:   "plan",
				Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "writer-agent"},
			Spec: resources.AgentSpec{
				ModelRef: "openai-default",
				Prompt:   "write",
				Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
			},
		},
	} {
		if _, err := stores.agentStore.Upsert(context.Background(), agent); err != nil {
			t.Fatalf("upsert agent %s: %v", agent.Metadata.Name, err)
		}
	}

	if _, err := stores.agentSystemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "message-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"planner-agent", "writer-agent"},
			Graph: map[string]resources.GraphEdge{
				"planner-agent": {Next: "writer-agent"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system: %v", err)
	}

	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "message-fail-task"},
		Spec:       resources.TaskSpec{System: "message-system"},
	}); err != nil {
		t.Fatalf("upsert task: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	task, ok, err := stores.taskStore.Get(context.Background(), "message-fail-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "Failed" {
		t.Fatalf("expected task Failed, got %q", task.Status.Phase)
	}
	if !strings.Contains(task.Status.LastError, "publish agent message") {
		t.Fatalf("expected publish agent message failure in last error, got %q", task.Status.LastError)
	}
}
