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

func TestFailureInjectionStaleHeartbeatReassignsPendingTask(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	taskStore := store.NewTaskStore()
	workerStore := store.NewWorkerStore()

	if _, err := workerStore.Upsert(resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-a"},
		Spec:       resources.WorkerSpec{Region: "default", MaxConcurrentTasks: 1},
		Status: resources.WorkerStatus{
			Phase:         "Ready",
			LastHeartbeat: time.Now().UTC().Add(-5 * time.Second).Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("upsert worker-a failed: %v", err)
	}
	if _, err := workerStore.Upsert(resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-b"},
		Spec:       resources.WorkerSpec{Region: "default", MaxConcurrentTasks: 1},
		Status: resources.WorkerStatus{
			Phase:         "Ready",
			LastHeartbeat: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("upsert worker-b failed: %v", err)
	}
	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "stale-task"},
		Spec:       resources.TaskSpec{System: "unused", Requirements: resources.TaskRequirements{Region: "default"}},
		Status:     resources.TaskStatus{AssignedWorker: "worker-a", Phase: "Pending"},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	workerController := NewWorkerController(workerStore, logger, 5*time.Millisecond, 500*time.Millisecond)
	if err := workerController.ReconcileOnce(); err != nil {
		t.Fatalf("worker controller reconcile failed: %v", err)
	}

	workerA, ok := workerStore.Get("worker-a")
	if !ok {
		t.Fatal("worker-a missing after reconcile")
	}
	if !strings.EqualFold(workerA.Status.Phase, "NotReady") {
		t.Fatalf("expected worker-a NotReady after stale heartbeat, got %q", workerA.Status.Phase)
	}

	scheduler := NewTaskSchedulerController(taskStore, workerStore, logger, 5*time.Millisecond, 500*time.Millisecond)
	if err := scheduler.ReconcileOnce(); err != nil {
		t.Fatalf("scheduler reconcile failed: %v", err)
	}

	task, ok := taskStore.Get("stale-task")
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.AssignedWorker != "worker-b" {
		t.Fatalf("expected reassignment to worker-b, got %q", task.Status.AssignedWorker)
	}
	assertHistoryType(t, task.Status.History, "assignment_cleared")
	assertHistoryType(t, task.Status.History, "assigned")
}

func TestFailureInjectionWorkerCrashLeaseTakeover(t *testing.T) {
	logger := log.New(io.Discard, "", 0)

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	toolStore := store.NewToolStore()
	memoryStore := store.NewMemoryStore()
	policyStore := store.NewAgentPolicyStore()
	taskStore := store.NewTaskStore()
	workerStore := store.NewWorkerStore()

	if _, err := toolStore.Upsert(resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "web_search"},
		Spec:       resources.ToolSpec{Type: "http", Endpoint: "https://example"},
	}); err != nil {
		t.Fatalf("upsert tool failed: %v", err)
	}
	if _, err := agentStore.Upsert(resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "agent-a"},
		Spec: resources.AgentSpec{
			Model:  "gpt-4o",
			Prompt: "run",
			Tools:  []string{"web_search"},
			Limits: resources.AgentLimits{MaxSteps: 2, Timeout: "1s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := systemStore.Upsert(resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "sys-a"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"agent-a"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "lease-takeover-task"},
		Spec:       resources.TaskSpec{System: "sys-a", Input: map[string]string{"topic": "x"}},
		Status:     resources.TaskStatus{AssignedWorker: "worker-a", Phase: "Pending"},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := workerStore.Upsert(resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-a"},
		Spec:       resources.WorkerSpec{Region: "default", MaxConcurrentTasks: 1},
		Status: resources.WorkerStatus{
			Phase:         "Ready",
			LastHeartbeat: now,
		},
	}); err != nil {
		t.Fatalf("upsert worker-a failed: %v", err)
	}
	if _, err := workerStore.Upsert(resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-b"},
		Spec:       resources.WorkerSpec{Region: "default", MaxConcurrentTasks: 1},
		Status: resources.WorkerStatus{
			Phase:         "Ready",
			LastHeartbeat: now,
		},
	}); err != nil {
		t.Fatalf("upsert worker-b failed: %v", err)
	}

	if _, ok, err := taskStore.ClaimIfDue("lease-takeover-task", "worker-a", 20*time.Millisecond); err != nil {
		t.Fatalf("initial claim by worker-a failed: %v", err)
	} else if !ok {
		t.Fatal("expected initial claim by worker-a")
	}

	workerA, ok := workerStore.Get("worker-a")
	if !ok {
		t.Fatal("worker-a not found")
	}
	workerA.Status.Phase = "NotReady"
	workerA.Status.LastHeartbeat = time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano)
	if _, err := workerStore.Upsert(workerA); err != nil {
		t.Fatalf("mark worker-a crashed failed: %v", err)
	}

	time.Sleep(30 * time.Millisecond)

	controllerB := NewTaskController(
		taskStore,
		systemStore,
		agentStore,
		toolStore,
		memoryStore,
		policyStore,
		workerStore,
		logger,
		5*time.Millisecond,
	)
	controllerB.ConfigureWorker("worker-b", 50*time.Millisecond, 10*time.Millisecond)
	if err := controllerB.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("worker-b reconcile failed: %v", err)
	}

	finalTask, ok := taskStore.Get("lease-takeover-task")
	if !ok {
		t.Fatal("task not found")
	}
	if finalTask.Status.Phase != "Succeeded" {
		t.Fatalf("expected task Succeeded after takeover, got %q", finalTask.Status.Phase)
	}
	assertHistoryType(t, finalTask.Status.History, "takeover")

	foundWorkerBClaim := false
	for _, item := range finalTask.Status.History {
		if strings.EqualFold(item.Type, "claim") && strings.EqualFold(item.Worker, "worker-b") {
			foundWorkerBClaim = true
			break
		}
	}
	if !foundWorkerBClaim {
		t.Fatalf("expected worker-b claim event after crash takeover, history=%+v", finalTask.Status.History)
	}
}

func assertHistoryType(t *testing.T, history []resources.TaskHistoryEvent, eventType string) {
	t.Helper()
	for _, item := range history {
		if strings.EqualFold(item.Type, eventType) {
			return
		}
	}
	t.Fatalf("expected history type %q not found in %+v", eventType, history)
}
