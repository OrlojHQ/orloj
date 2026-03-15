package controllers

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/crds"
	"github.com/OrlojHQ/orloj/store"
)

func TestTaskRetrySchedulesNextAttemptOnTimeout(t *testing.T) {
	controller, stores := newTaskControllerHarness()

	agent := crds.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   crds.ObjectMeta{Name: "retry-agent"},
		Spec: crds.AgentSpec{
			Model:  "gpt-4o",
			Prompt: "retry test",
			Limits: crds.AgentLimits{MaxSteps: 5, Timeout: "1ms"},
		},
	}
	if _, err := stores.agentStore.Upsert(agent); err != nil {
		t.Fatalf("upsert agent: %v", err)
	}

	system := crds.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   crds.ObjectMeta{Name: "retry-system"},
		Spec:       crds.AgentSystemSpec{Agents: []string{"retry-agent"}},
	}
	if _, err := stores.agentSystemStore.Upsert(system); err != nil {
		t.Fatalf("upsert system: %v", err)
	}

	task := crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "retry-task"},
		Spec: crds.TaskSpec{
			System: "retry-system",
			Retry:  crds.TaskRetryPolicy{MaxAttempts: 3, Backoff: "1ms"},
		},
	}
	if _, err := stores.taskStore.Upsert(task); err != nil {
		t.Fatalf("upsert task: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	taskAfterFirst, ok := stores.taskStore.Get("retry-task")
	if !ok {
		t.Fatal("task not found after first reconcile")
	}
	if taskAfterFirst.Status.Phase != "Pending" {
		t.Fatalf("expected Pending after first reconcile timeout retry scheduling, got %q", taskAfterFirst.Status.Phase)
	}
	if taskAfterFirst.Status.Attempts != 1 {
		t.Fatalf("expected attempts=1 after first reconcile, got %d", taskAfterFirst.Status.Attempts)
	}
	if taskAfterFirst.Status.NextAttemptAt == "" {
		t.Fatal("expected nextAttemptAt to be set after first reconcile")
	}
	if !strings.Contains(strings.ToLower(taskAfterFirst.Status.LastError), "retry scheduled") {
		t.Fatalf("expected retry scheduled in lastError, got %q", taskAfterFirst.Status.LastError)
	}

	time.Sleep(3 * time.Millisecond)
	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	taskAfterSecond, ok := stores.taskStore.Get("retry-task")
	if !ok {
		t.Fatal("task not found after second reconcile")
	}
	if taskAfterSecond.Status.Phase != "Pending" {
		t.Fatalf("expected Pending after timeout retry scheduling, got %q", taskAfterSecond.Status.Phase)
	}
	if taskAfterSecond.Status.NextAttemptAt == "" {
		t.Fatal("expected nextAttemptAt to be set")
	}
	if !strings.Contains(strings.ToLower(taskAfterSecond.Status.LastError), "retry scheduled") {
		t.Fatalf("expected retry scheduled in lastError, got %q", taskAfterSecond.Status.LastError)
	}

	time.Sleep(3 * time.Millisecond)
	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("third reconcile: %v", err)
	}
	finalTask, _ := stores.taskStore.Get("retry-task")
	if finalTask.Status.Phase != "DeadLetter" {
		t.Fatalf("expected DeadLetter after max retries reached, got %q", finalTask.Status.Phase)
	}
	if finalTask.Status.Attempts != 3 {
		t.Fatalf("expected attempts=3 after retries exhausted, got %d", finalTask.Status.Attempts)
	}
}

func TestTaskNonRetryablePolicyViolationFailsImmediately(t *testing.T) {
	controller, stores := newTaskControllerHarness()

	agent := crds.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   crds.ObjectMeta{Name: "policy-agent"},
		Spec: crds.AgentSpec{
			Model:  "gpt-4o",
			Prompt: "policy test",
			Limits: crds.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	}
	if _, err := stores.agentStore.Upsert(agent); err != nil {
		t.Fatalf("upsert agent: %v", err)
	}

	system := crds.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   crds.ObjectMeta{Name: "policy-system"},
		Spec:       crds.AgentSystemSpec{Agents: []string{"policy-agent"}},
	}
	if _, err := stores.agentSystemStore.Upsert(system); err != nil {
		t.Fatalf("upsert system: %v", err)
	}

	policy := crds.AgentPolicy{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentPolicy",
		Metadata:   crds.ObjectMeta{Name: "strict-policy"},
		Spec: crds.AgentPolicySpec{
			ApplyMode:     "scoped",
			TargetSystems: []string{"policy-system"},
			AllowedModels: []string{"claude-3"},
		},
	}
	if _, err := stores.policyStore.Upsert(policy); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}

	task := crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "policy-task"},
		Spec: crds.TaskSpec{
			System: "policy-system",
			Retry:  crds.TaskRetryPolicy{MaxAttempts: 3, Backoff: "1ms"},
		},
	}
	if _, err := stores.taskStore.Upsert(task); err != nil {
		t.Fatalf("upsert task: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	finalTask, ok := stores.taskStore.Get("policy-task")
	if !ok {
		t.Fatal("task not found")
	}
	if finalTask.Status.Phase != "Failed" {
		t.Fatalf("expected Failed, got %q", finalTask.Status.Phase)
	}
	if finalTask.Status.NextAttemptAt != "" {
		t.Fatalf("expected no nextAttemptAt for non-retryable failure, got %q", finalTask.Status.NextAttemptAt)
	}
	if !strings.Contains(strings.ToLower(finalTask.Status.LastError), "disallows model") {
		t.Fatalf("expected policy violation in lastError, got %q", finalTask.Status.LastError)
	}
}

type taskControllerHarness struct {
	taskStore        *store.TaskStore
	agentSystemStore *store.AgentSystemStore
	agentStore       *store.AgentStore
	toolStore        *store.ToolStore
	memoryStore      *store.MemoryStore
	policyStore      *store.AgentPolicyStore
	workerStore      *store.WorkerStore
}

func newTaskControllerHarness() (*TaskController, taskControllerHarness) {
	logger := log.New(io.Discard, "", 0)
	h := taskControllerHarness{
		taskStore:        store.NewTaskStore(),
		agentSystemStore: store.NewAgentSystemStore(),
		agentStore:       store.NewAgentStore(),
		toolStore:        store.NewToolStore(),
		memoryStore:      store.NewMemoryStore(),
		policyStore:      store.NewAgentPolicyStore(),
		workerStore:      store.NewWorkerStore(),
	}
	if _, err := h.workerStore.Upsert(crds.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   crds.ObjectMeta{Name: "test-worker"},
		Spec:       crds.WorkerSpec{Region: "default"},
		Status: crds.WorkerStatus{
			Phase:         "Ready",
			LastHeartbeat: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}); err != nil {
		panic(err)
	}
	controller := NewTaskController(
		h.taskStore,
		h.agentSystemStore,
		h.agentStore,
		h.toolStore,
		h.memoryStore,
		h.policyStore,
		h.workerStore,
		logger,
		5*time.Millisecond,
	)
	controller.ConfigureWorker("test-worker", 30*time.Second, 10*time.Second)
	return controller, h
}
