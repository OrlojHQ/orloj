package controllers

import (
	"context"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestTaskControllerMessageDrivenKickoff(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	controller.SetExecutionMode("message-driven")
	bus := &captureAgentMessageBus{}
	controller.SetAgentMessageBus(bus)

	for _, agent := range []resources.Agent{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "planner-agent"},
			Spec: resources.AgentSpec{
				Model:  "gpt-4o",
				Prompt: "plan",
				Limits: resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "writer-agent"},
			Spec: resources.AgentSpec{
				Model:  "gpt-4o",
				Prompt: "write",
				Limits: resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
			},
		},
	} {
		if _, err := stores.agentStore.Upsert(agent); err != nil {
			t.Fatalf("upsert agent %s: %v", agent.Metadata.Name, err)
		}
	}

	if _, err := stores.agentSystemStore.Upsert(resources.AgentSystem{
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

	if _, err := stores.taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "message-driven-task"},
		Spec:       resources.TaskSpec{System: "message-system", Input: map[string]string{"topic": "runtime"}},
	}); err != nil {
		t.Fatalf("upsert task: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	task, ok := stores.taskStore.Get("message-driven-task")
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "Running" {
		t.Fatalf("expected Running phase in message-driven kickoff, got %q", task.Status.Phase)
	}
	if task.Status.Output["runtime.mode"] != "message-driven" {
		t.Fatalf("expected runtime.mode message-driven, got %q", task.Status.Output["runtime.mode"])
	}
	if task.Status.Output["runtime.entry_agent"] != "planner-agent" {
		t.Fatalf("expected runtime.entry_agent planner-agent, got %q", task.Status.Output["runtime.entry_agent"])
	}
	if len(task.Status.Messages) != 1 {
		t.Fatalf("expected one kickoff message, got %d", len(task.Status.Messages))
	}
	if task.Status.Messages[0].Type != "task_start" {
		t.Fatalf("expected kickoff message type task_start, got %q", task.Status.Messages[0].Type)
	}
	if task.Status.Messages[0].ToAgent != "planner-agent" {
		t.Fatalf("expected kickoff to planner-agent, got %q", task.Status.Messages[0].ToAgent)
	}
	if countHistoryEvents(task.Status.History, "runtime_kickoff") != 1 {
		t.Fatalf("expected one runtime_kickoff history event, got %d", countHistoryEvents(task.Status.History, "runtime_kickoff"))
	}

	published := bus.Published()
	if len(published) != 1 {
		t.Fatalf("expected one published kickoff envelope, got %d", len(published))
	}
	if published[0].ToAgent != "planner-agent" {
		t.Fatalf("expected published kickoff to planner-agent, got %q", published[0].ToAgent)
	}
	if published[0].Type != "task_start" {
		t.Fatalf("expected published kickoff type task_start, got %q", published[0].Type)
	}
}

func countHistoryEvents(history []resources.TaskHistoryEvent, eventType string) int {
	count := 0
	for _, event := range history {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func TestTaskControllerMessageDrivenRejectsCycleWithoutMaxTurns(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	controller.SetExecutionMode("message-driven")
	controller.SetAgentMessageBus(&captureAgentMessageBus{})

	for _, agent := range []resources.Agent{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "manager-agent"},
			Spec:       resources.AgentSpec{Model: "gpt-4o", Prompt: "manage"},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "research-agent"},
			Spec:       resources.AgentSpec{Model: "gpt-4o", Prompt: "research"},
		},
	} {
		if _, err := stores.agentStore.Upsert(agent); err != nil {
			t.Fatalf("upsert agent %s: %v", agent.Metadata.Name, err)
		}
	}

	if _, err := stores.agentSystemStore.Upsert(resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "cycle-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"manager-agent", "research-agent"},
			Graph: map[string]resources.GraphEdge{
				"manager-agent":  {Next: "research-agent"},
				"research-agent": {Next: "manager-agent"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system: %v", err)
	}

	if _, err := stores.taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "cycle-no-max-turns"},
		Spec:       resources.TaskSpec{System: "cycle-system"},
	}); err != nil {
		t.Fatalf("upsert task: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	task, ok := stores.taskStore.Get("cycle-no-max-turns")
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "Failed" {
		t.Fatalf("expected Failed phase, got %q", task.Status.Phase)
	}
	if task.Status.LastError == "" || !strings.Contains(task.Status.LastError, "spec.max_turns") {
		t.Fatalf("expected cycle/max_turns validation error, got %q", task.Status.LastError)
	}
}

func TestTaskControllerMessageDrivenAllowsCycleWithMaxTurns(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	controller.SetExecutionMode("message-driven")
	bus := &captureAgentMessageBus{}
	controller.SetAgentMessageBus(bus)

	for _, agent := range []resources.Agent{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "manager-agent"},
			Spec:       resources.AgentSpec{Model: "gpt-4o", Prompt: "manage"},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "research-agent"},
			Spec:       resources.AgentSpec{Model: "gpt-4o", Prompt: "research"},
		},
	} {
		if _, err := stores.agentStore.Upsert(agent); err != nil {
			t.Fatalf("upsert agent %s: %v", agent.Metadata.Name, err)
		}
	}

	if _, err := stores.agentSystemStore.Upsert(resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "cycle-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"manager-agent", "research-agent"},
			Graph: map[string]resources.GraphEdge{
				"manager-agent":  {Next: "research-agent"},
				"research-agent": {Next: "manager-agent"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system: %v", err)
	}

	if _, err := stores.taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "cycle-with-max-turns"},
		Spec: resources.TaskSpec{
			System:   "cycle-system",
			MaxTurns: 4,
		},
	}); err != nil {
		t.Fatalf("upsert task: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	task, ok := stores.taskStore.Get("cycle-with-max-turns")
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "Running" {
		t.Fatalf("expected Running phase, got %q lastError=%q", task.Status.Phase, task.Status.LastError)
	}
	if len(task.Status.Messages) != 1 {
		t.Fatalf("expected one kickoff message, got %d", len(task.Status.Messages))
	}
	if task.Status.Messages[0].ToAgent != "manager-agent" {
		t.Fatalf("expected kickoff to first listed agent manager-agent, got %q", task.Status.Messages[0].ToAgent)
	}
	published := bus.Published()
	if len(published) != 1 {
		t.Fatalf("expected one published kickoff envelope, got %d", len(published))
	}
}
