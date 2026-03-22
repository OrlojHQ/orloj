package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

func TestReconcileWaitingApprovalSweepResumesAfterApprove(t *testing.T) {
	c, h := newTaskControllerHarness()
	approvals := store.NewToolApprovalStore()
	c.SetToolApprovalStore(approvals)

	ns := resources.NormalizeNamespace("default")
	taskName := "approval-wait-task"
	taskKey := store.ScopedName(ns, taskName)

	if _, err := h.taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Namespace: ns,
			Name:      taskName,
		},
		Spec: resources.TaskSpec{System: "noop-system", Input: map[string]string{"k": "v"}},
		Status: resources.TaskStatus{
			Phase:            "WaitingApproval",
			AssignedWorker:   "test-worker",
			ObservedGeneration: 1,
		},
	}); err != nil {
		t.Fatalf("upsert task: %v", err)
	}

	if _, err := approvals.Upsert(resources.ToolApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "ToolApproval",
		Metadata: resources.ObjectMeta{
			Namespace: ns,
			Name:      "ta-sweep-test",
		},
		Spec: resources.ToolApprovalSpec{
			TaskRef: taskKey,
			Tool:    "example-tool",
		},
		Status: resources.ToolApprovalStatus{
			Phase:     "Approved",
			Decision:  "approved",
			DecidedBy: "test",
			DecidedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}); err != nil {
		t.Fatalf("upsert tool approval: %v", err)
	}

	if err := c.reconcileWaitingApprovalSweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	task, ok, err := h.taskStore.Get(taskKey)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task missing after sweep")
	}
	if task.Status.Phase != "Running" {
		t.Fatalf("expected Running after approved ToolApproval, got %q", task.Status.Phase)
	}
	if task.Status.ClaimedBy != "test-worker" {
		t.Fatalf("expected ClaimedBy test-worker, got %q", task.Status.ClaimedBy)
	}
}
