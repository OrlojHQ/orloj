package controllers

import (
	"io"
	"log"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

func TestTaskSchedulerSkipsTemplateTasks(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	taskStore := store.NewTaskStore()
	workerStore := store.NewWorkerStore()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if _, err := workerStore.Upsert(resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-a"},
		Status:     resources.WorkerStatus{Phase: "Ready", LastHeartbeat: now},
	}); err != nil {
		t.Fatalf("upsert worker failed: %v", err)
	}
	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "template-task"},
		Spec:       resources.TaskSpec{Mode: "template"},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	controller := NewTaskSchedulerController(taskStore, workerStore, logger, 5*time.Millisecond, 30*time.Second)
	if err := controller.ReconcileOnce(); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	task, ok, err := taskStore.Get("template-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.AssignedWorker != "" {
		t.Fatalf("expected no worker assignment for template task, got %q", task.Status.AssignedWorker)
	}
}

func TestTaskControllerTaskMatchesWorkerRejectsTemplate(t *testing.T) {
	controller := NewTaskController(nil, nil, nil, nil, nil, nil, nil, log.New(io.Discard, "", 0), time.Second)
	task := resources.Task{Spec: resources.TaskSpec{Mode: "template"}}
	if controller.taskMatchesWorker(task) {
		t.Fatal("expected template task to be rejected by worker matcher")
	}
}
