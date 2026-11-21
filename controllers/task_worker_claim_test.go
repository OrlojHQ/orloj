package controllers

import (
	"context"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

func TestTaskClaimSingleExecutionAcrossWorkers(t *testing.T) {
	logger := log.New(io.Discard, "", 0)

	agentStore := store.NewAgentStore()
	agentSystemStore := store.NewAgentSystemStore()
	modelEPStore := store.NewModelEndpointStore()
	toolStore := store.NewToolStore()
	memoryStore := store.NewMemoryStore()
	policyStore := store.NewAgentPolicyStore()
	taskStore := store.NewTaskStore()
	workerStore := store.NewWorkerStore()
	if _, err := modelEPStore.Upsert(context.Background(), resources.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata:   resources.ObjectMeta{Name: "openai-default", Namespace: "default"},
		Spec: resources.ModelEndpointSpec{
			Provider:     "mock",
			DefaultModel: "gpt-4o",
		},
	}); err != nil {
		t.Fatalf("upsert model endpoint failed: %v", err)
	}

	agent := resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "research-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "You are a research assistant.",
			Tools:    []string{"web_search"},
			Limits:   resources.AgentLimits{MaxSteps: 2, Timeout: "1s"},
		},
	}
	if _, err := agentStore.Upsert(context.Background(), agent); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := toolStore.Upsert(context.Background(), resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "web_search"},
		Spec:       resources.ToolSpec{Type: "http", Endpoint: "https://example"},
	}); err != nil {
		t.Fatalf("upsert tool failed: %v", err)
	}
	if _, err := agentSystemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "report-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"research-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "weekly-report"},
		Spec:       resources.TaskSpec{System: "report-system", Input: map[string]string{"topic": "x"}},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	if _, err := workerStore.Upsert(context.Background(), resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-1"},
		Spec:       resources.WorkerSpec{Region: "default"},
		Status: resources.WorkerStatus{
			Phase:         "Ready",
			LastHeartbeat: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("upsert worker-1 failed: %v", err)
	}
	if _, err := workerStore.Upsert(context.Background(), resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-2"},
		Spec:       resources.WorkerSpec{Region: "default"},
		Status: resources.WorkerStatus{
			Phase:         "Ready",
			LastHeartbeat: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("upsert worker-2 failed: %v", err)
	}

	worker1 := NewTaskController(taskStore, agentSystemStore, agentStore, toolStore, memoryStore, policyStore, workerStore, logger, 5*time.Millisecond)
	worker1.ConfigureWorker("worker-1", 100*time.Millisecond, 20*time.Millisecond)
	worker1.SetModelEndpointStore(modelEPStore)
	worker2 := NewTaskController(taskStore, agentSystemStore, agentStore, toolStore, memoryStore, policyStore, workerStore, logger, 5*time.Millisecond)
	worker2.ConfigureWorker("worker-2", 100*time.Millisecond, 20*time.Millisecond)
	worker2.SetModelEndpointStore(modelEPStore)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = worker1.ReconcileOnce(context.Background())
	}()
	go func() {
		defer wg.Done()
		_ = worker2.ReconcileOnce(context.Background())
	}()
	wg.Wait()

	task, ok, err := taskStore.Get(context.Background(), "weekly-report")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected task Succeeded, got %q", task.Status.Phase)
	}
	if task.Status.Attempts != 1 {
		t.Fatalf("expected attempts=1, got %d", task.Status.Attempts)
	}
	if task.Status.ClaimedBy != "" {
		t.Fatalf("expected claim released, got claimedBy=%q", task.Status.ClaimedBy)
	}
}

func TestTaskClaimHonorsAssignedWorker(t *testing.T) {
	logger := log.New(io.Discard, "", 0)

	agentStore := store.NewAgentStore()
	agentSystemStore := store.NewAgentSystemStore()
	modelEPStore := store.NewModelEndpointStore()
	toolStore := store.NewToolStore()
	memoryStore := store.NewMemoryStore()
	policyStore := store.NewAgentPolicyStore()
	taskStore := store.NewTaskStore()
	workerStore := store.NewWorkerStore()
	if _, err := modelEPStore.Upsert(context.Background(), resources.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata:   resources.ObjectMeta{Name: "openai-default", Namespace: "default"},
		Spec: resources.ModelEndpointSpec{
			Provider:     "mock",
			DefaultModel: "gpt-4o",
		},
	}); err != nil {
		t.Fatalf("upsert model endpoint failed: %v", err)
	}

	agent := resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "research-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "You are a research assistant.",
			Tools:    []string{"web_search"},
			Limits:   resources.AgentLimits{MaxSteps: 2, Timeout: "1s"},
		},
	}
	if _, err := agentStore.Upsert(context.Background(), agent); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := toolStore.Upsert(context.Background(), resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "web_search"},
		Spec:       resources.ToolSpec{Type: "http", Endpoint: "https://example"},
	}); err != nil {
		t.Fatalf("upsert tool failed: %v", err)
	}
	if _, err := agentSystemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "report-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"research-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "weekly-report"},
		Spec:       resources.TaskSpec{System: "report-system", Input: map[string]string{"topic": "x"}},
		Status:     resources.TaskStatus{AssignedWorker: "worker-2"},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	if _, err := workerStore.Upsert(context.Background(), resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-1"},
		Spec:       resources.WorkerSpec{Region: "default"},
		Status: resources.WorkerStatus{
			Phase:         "Ready",
			LastHeartbeat: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("upsert worker-1 failed: %v", err)
	}
	if _, err := workerStore.Upsert(context.Background(), resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-2"},
		Spec:       resources.WorkerSpec{Region: "default"},
		Status: resources.WorkerStatus{
			Phase:         "Ready",
			LastHeartbeat: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("upsert worker-2 failed: %v", err)
	}

	worker1 := NewTaskController(taskStore, agentSystemStore, agentStore, toolStore, memoryStore, policyStore, workerStore, logger, 5*time.Millisecond)
	worker1.ConfigureWorker("worker-1", 100*time.Millisecond, 20*time.Millisecond)
	worker1.SetModelEndpointStore(modelEPStore)
	worker2 := NewTaskController(taskStore, agentSystemStore, agentStore, toolStore, memoryStore, policyStore, workerStore, logger, 5*time.Millisecond)
	worker2.ConfigureWorker("worker-2", 100*time.Millisecond, 20*time.Millisecond)
	worker2.SetModelEndpointStore(modelEPStore)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = worker1.ReconcileOnce(context.Background())
	}()
	go func() {
		defer wg.Done()
		_ = worker2.ReconcileOnce(context.Background())
	}()
	wg.Wait()

	task, ok, err := taskStore.Get(context.Background(), "weekly-report")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected task Succeeded, got %q", task.Status.Phase)
	}

	foundWorker2Claim := false
	for _, event := range task.Status.History {
		if event.Type == "claim" && event.Worker == "worker-2" {
			foundWorker2Claim = true
			break
		}
	}
	if !foundWorker2Claim {
		t.Fatalf("expected claim history to show worker-2 claim, got %+v", task.Status.History)
	}
}

func TestTaskWorkerCapacitySkipsClaimWhenFull(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	taskStore := store.NewTaskStore()
	agentStore := store.NewAgentStore()
	agentSystemStore := store.NewAgentSystemStore()
	toolStore := store.NewToolStore()
	memoryStore := store.NewMemoryStore()
	policyStore := store.NewAgentPolicyStore()
	workerStore := store.NewWorkerStore()

	if _, err := taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "capacity-task"},
		Spec:       resources.TaskSpec{System: "unused"},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}
	if _, err := workerStore.Upsert(context.Background(), resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-1"},
		Spec: resources.WorkerSpec{
			Region:             "default",
			MaxConcurrentTasks: 1,
		},
		Status: resources.WorkerStatus{
			Phase:         "Ready",
			LastHeartbeat: time.Now().UTC().Format(time.RFC3339Nano),
			CurrentTasks:  1,
		},
	}); err != nil {
		t.Fatalf("upsert worker failed: %v", err)
	}

	if _, _, acquired, err := workerStore.AcquireLeasedSlot(context.Background(), "worker-1", time.Minute); err != nil || !acquired {
		t.Fatalf("reserve worker slot: acquired=%v err=%v", acquired, err)
	}

	controller := NewTaskController(taskStore, agentSystemStore, agentStore, toolStore, memoryStore, policyStore, workerStore, logger, 5*time.Millisecond)
	controller.ConfigureWorker("worker-1", 100*time.Millisecond, 20*time.Millisecond)

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	task, ok, err := taskStore.Get(context.Background(), "capacity-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "Pending" {
		t.Fatalf("expected task to remain Pending when worker is full, got %q", task.Status.Phase)
	}
	if task.Status.Attempts != 0 {
		t.Fatalf("expected attempts=0 when worker is full, got %d", task.Status.Attempts)
	}
}

// A canceled task context must not prevent persisted worker-slot cleanup.
func TestReleaseWorkerSlotAfterContextCanceled(t *testing.T) {
	workerStore := store.NewWorkerStore()
	ctx := context.Background()
	_, err := workerStore.Upsert(ctx, resources.Worker{
		Metadata: resources.ObjectMeta{Name: "cleanup-worker"},
		Spec:     resources.WorkerSpec{MaxConcurrentTasks: 1},
		Status:   resources.WorkerStatus{Phase: "Ready", CurrentTasks: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := &TaskController{workerStore: workerStore, workerID: "cleanup-worker"}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := c.releaseWorkerSlot(canceled); err != nil {
		t.Fatal(err)
	}
	worker, ok, err := workerStore.Get(ctx, "cleanup-worker")
	if err != nil || !ok || worker.Status.CurrentTasks != 0 {
		t.Fatalf("slot not released: worker=%+v ok=%v err=%v", worker.Status, ok, err)
	}
}

func TestLostSlotLeaseCancelsTask(t *testing.T) {
	workers := store.NewWorkerStore()
	ctx := context.Background()
	_, err := workers.Upsert(ctx, resources.Worker{APIVersion: "orloj.dev/v1", Kind: "Worker", Metadata: resources.ObjectMeta{Name: "worker"}, Spec: resources.WorkerSpec{MaxConcurrentTasks: 1}, Status: resources.WorkerStatus{Phase: "Ready"}})
	if err != nil {
		t.Fatal(err)
	}
	_, token, acquired, err := workers.AcquireLeasedSlot(ctx, "worker", time.Second)
	if err != nil || !acquired {
		t.Fatalf("acquire: acquired=%v err=%v", acquired, err)
	}
	controller := NewTaskController(nil, nil, nil, nil, nil, nil, workers, nil, time.Second)
	controller.ConfigureWorker("worker", time.Second, 10*time.Millisecond)
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := controller.startSlotHeartbeat(taskCtx, cancel, token)
	defer stop()
	if err := workers.ReleaseLeasedSlot(ctx, "worker", token); err != nil {
		t.Fatal(err)
	}
	select {
	case <-taskCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("lost slot did not cancel task")
	}
}
