package controllers

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

func TestTaskRetrySchedulesNextAttemptOnTimeout(t *testing.T) {
	controller, stores := newTaskControllerHarness()

	agent := resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "retry-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "retry test",
			Limits:   resources.AgentLimits{MaxSteps: 5, Timeout: "1ms"},
		},
	}
	if _, err := stores.agentStore.Upsert(context.Background(), agent); err != nil {
		t.Fatalf("upsert agent: %v", err)
	}

	system := resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "retry-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"retry-agent"}},
	}
	if _, err := stores.agentSystemStore.Upsert(context.Background(), system); err != nil {
		t.Fatalf("upsert system: %v", err)
	}

	task := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "retry-task"},
		Spec: resources.TaskSpec{
			System: "retry-system",
			Retry:  resources.TaskRetryPolicy{MaxAttempts: 3, Backoff: "1ms"},
		},
	}
	if _, err := stores.taskStore.Upsert(context.Background(), task); err != nil {
		t.Fatalf("upsert task: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	taskAfterFirst, ok, err := stores.taskStore.Get(context.Background(), "retry-task")
	if err != nil {
		t.Fatal(err)
	}
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

	taskAfterSecond, ok, err := stores.taskStore.Get(context.Background(), "retry-task")
	if err != nil {
		t.Fatal(err)
	}
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
	finalTask, _, _ := stores.taskStore.Get(context.Background(), "retry-task")
	if finalTask.Status.Phase != "DeadLetter" {
		t.Fatalf("expected DeadLetter after max retries reached, got %q", finalTask.Status.Phase)
	}
	if finalTask.Status.Attempts != 3 {
		t.Fatalf("expected attempts=3 after retries exhausted, got %d", finalTask.Status.Attempts)
	}
}

func TestTaskNonRetryablePolicyViolationFailsImmediately(t *testing.T) {
	controller, stores := newTaskControllerHarness()

	agent := resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "policy-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "policy test",
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	}
	if _, err := stores.agentStore.Upsert(context.Background(), agent); err != nil {
		t.Fatalf("upsert agent: %v", err)
	}

	system := resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "policy-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"policy-agent"}},
	}
	if _, err := stores.agentSystemStore.Upsert(context.Background(), system); err != nil {
		t.Fatalf("upsert system: %v", err)
	}

	policy := resources.AgentPolicy{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentPolicy",
		Metadata:   resources.ObjectMeta{Name: "strict-policy"},
		Spec: resources.AgentPolicySpec{
			ApplyMode:     "scoped",
			TargetSystems: []string{"policy-system"},
			AllowedModels: []string{"claude-3"},
		},
	}
	if _, err := stores.policyStore.Upsert(context.Background(), policy); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}

	task := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "policy-task"},
		Spec: resources.TaskSpec{
			System: "policy-system",
			Retry:  resources.TaskRetryPolicy{MaxAttempts: 3, Backoff: "1ms"},
		},
	}
	if _, err := stores.taskStore.Upsert(context.Background(), task); err != nil {
		t.Fatalf("upsert task: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	finalTask, ok, err := stores.taskStore.Get(context.Background(), "policy-task")
	if err != nil {
		t.Fatal(err)
	}
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
	modelEPStore     *store.ModelEndpointStore
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
		modelEPStore:     store.NewModelEndpointStore(),
		toolStore:        store.NewToolStore(),
		memoryStore:      store.NewMemoryStore(),
		policyStore:      store.NewAgentPolicyStore(),
		workerStore:      store.NewWorkerStore(),
	}
	if _, err := h.modelEPStore.Upsert(context.Background(), resources.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata:   resources.ObjectMeta{Name: "openai-default", Namespace: "default"},
		Spec: resources.ModelEndpointSpec{
			Provider:     "mock",
			DefaultModel: "gpt-4o",
		},
	}); err != nil {
		panic(err)
	}
	if _, err := h.workerStore.Upsert(context.Background(), resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "test-worker"},
		Spec:       resources.WorkerSpec{Region: "default"},
		Status: resources.WorkerStatus{
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
	controller.SetModelEndpointStore(h.modelEPStore)
	return controller, h
}
