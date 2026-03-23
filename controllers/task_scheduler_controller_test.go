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

func TestTaskSchedulerAssignsTasksByRequirementsAndCapacity(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	taskStore := store.NewTaskStore()
	workerStore := store.NewWorkerStore()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	workers := []resources.Worker{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Worker",
			Metadata:   resources.ObjectMeta{Name: "worker-a"},
			Spec: resources.WorkerSpec{
				Region:             "us-east",
				MaxConcurrentTasks: 1,
				Capabilities: resources.WorkerCapabilities{
					SupportedModels: []string{"gpt-4o"},
				},
			},
			Status: resources.WorkerStatus{Phase: "Ready", LastHeartbeat: now, CurrentTasks: 0},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Worker",
			Metadata:   resources.ObjectMeta{Name: "worker-b"},
			Spec: resources.WorkerSpec{
				Region:             "us-west",
				MaxConcurrentTasks: 1,
				Capabilities: resources.WorkerCapabilities{
					GPU:             true,
					SupportedModels: []string{"gpt-4o"},
				},
			},
			Status: resources.WorkerStatus{Phase: "Ready", LastHeartbeat: now, CurrentTasks: 1},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Worker",
			Metadata:   resources.ObjectMeta{Name: "worker-c"},
			Spec: resources.WorkerSpec{
				Region:             "us-west",
				MaxConcurrentTasks: 2,
				Capabilities: resources.WorkerCapabilities{
					GPU:             true,
					SupportedModels: []string{"gpt-4o"},
				},
			},
			Status: resources.WorkerStatus{Phase: "Ready", LastHeartbeat: now, CurrentTasks: 0},
		},
	}
	for _, worker := range workers {
		if _, err := workerStore.Upsert(context.Background(),worker); err != nil {
			t.Fatalf("upsert worker %s: %v", worker.Metadata.Name, err)
		}
	}

	tasks := []resources.Task{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Task",
			Metadata:   resources.ObjectMeta{Name: "task-west-gpu-1"},
			Spec: resources.TaskSpec{
				Requirements: resources.TaskRequirements{Region: "us-west", GPU: true, Model: "gpt-4o"},
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Task",
			Metadata:   resources.ObjectMeta{Name: "task-east"},
			Spec: resources.TaskSpec{
				Requirements: resources.TaskRequirements{Region: "us-east", Model: "gpt-4o"},
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Task",
			Metadata:   resources.ObjectMeta{Name: "task-west-gpu-2"},
			Spec: resources.TaskSpec{
				Requirements: resources.TaskRequirements{Region: "us-west", GPU: true, Model: "gpt-4o"},
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Task",
			Metadata:   resources.ObjectMeta{Name: "task-no-match"},
			Spec: resources.TaskSpec{
				Requirements: resources.TaskRequirements{Region: "eu-central", GPU: true, Model: "gpt-4o"},
			},
		},
	}
	for _, task := range tasks {
		if _, err := taskStore.Upsert(context.Background(),task); err != nil {
			t.Fatalf("upsert task %s: %v", task.Metadata.Name, err)
		}
	}

	controller := NewTaskSchedulerController(taskStore, workerStore, logger, 5*time.Millisecond, 30*time.Second)
	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	tWest1, _, _ := taskStore.Get(context.Background(),"task-west-gpu-1")
	if tWest1.Status.AssignedWorker != "worker-c" {
		t.Fatalf("expected task-west-gpu-1 assigned to worker-c, got %q", tWest1.Status.AssignedWorker)
	}
	tEast, _, _ := taskStore.Get(context.Background(),"task-east")
	if tEast.Status.AssignedWorker != "worker-a" {
		t.Fatalf("expected task-east assigned to worker-a, got %q", tEast.Status.AssignedWorker)
	}
	tWest2, _, _ := taskStore.Get(context.Background(),"task-west-gpu-2")
	if tWest2.Status.AssignedWorker != "worker-c" {
		t.Fatalf("expected task-west-gpu-2 assigned to worker-c, got %q", tWest2.Status.AssignedWorker)
	}
	tNoMatch, _, _ := taskStore.Get(context.Background(),"task-no-match")
	if tNoMatch.Status.AssignedWorker != "" {
		t.Fatalf("expected task-no-match to remain unassigned, got %q", tNoMatch.Status.AssignedWorker)
	}

	if len(tWest1.Status.History) == 0 || !strings.EqualFold(tWest1.Status.History[len(tWest1.Status.History)-1].Type, "assigned") {
		t.Fatalf("expected assignment history on task-west-gpu-1, got %+v", tWest1.Status.History)
	}
}

func TestTaskSchedulerClearsAndReassignsInvalidAssignment(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	taskStore := store.NewTaskStore()
	workerStore := store.NewWorkerStore()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if _, err := workerStore.Upsert(context.Background(),resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-ready"},
		Spec: resources.WorkerSpec{
			Region:             "default",
			MaxConcurrentTasks: 1,
			Capabilities:       resources.WorkerCapabilities{SupportedModels: []string{"gpt-4o"}},
		},
		Status: resources.WorkerStatus{Phase: "Ready", LastHeartbeat: now},
	}); err != nil {
		t.Fatalf("upsert worker-ready: %v", err)
	}
	if _, err := workerStore.Upsert(context.Background(),resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-stale"},
		Spec: resources.WorkerSpec{
			Region:             "default",
			MaxConcurrentTasks: 1,
			Capabilities:       resources.WorkerCapabilities{SupportedModels: []string{"gpt-4o"}},
		},
		Status: resources.WorkerStatus{Phase: "NotReady", LastHeartbeat: now},
	}); err != nil {
		t.Fatalf("upsert worker-stale: %v", err)
	}

	if _, err := taskStore.Upsert(context.Background(),resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "task-1"},
		Spec: resources.TaskSpec{
			Requirements: resources.TaskRequirements{Model: "gpt-4o"},
		},
		Status: resources.TaskStatus{AssignedWorker: "worker-stale"},
	}); err != nil {
		t.Fatalf("upsert task: %v", err)
	}

	controller := NewTaskSchedulerController(taskStore, workerStore, logger, 5*time.Millisecond, 30*time.Second)
	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	task, _, _ := taskStore.Get(context.Background(),"task-1")
	if task.Status.AssignedWorker != "worker-ready" {
		t.Fatalf("expected reassignment to worker-ready, got %q", task.Status.AssignedWorker)
	}
	if len(task.Status.History) < 2 {
		t.Fatalf("expected clear + assign history events, got %+v", task.Status.History)
	}
	if !strings.EqualFold(task.Status.History[len(task.Status.History)-2].Type, "assignment_cleared") {
		t.Fatalf("expected assignment_cleared history event, got %+v", task.Status.History)
	}
	if !strings.EqualFold(task.Status.History[len(task.Status.History)-1].Type, "assigned") {
		t.Fatalf("expected assigned history event, got %+v", task.Status.History)
	}
}
