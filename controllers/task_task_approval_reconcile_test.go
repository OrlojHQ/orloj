package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

func testEncodeResumeContext(ctx resources.TaskApprovalResumeContext) map[string]any {
	out, _ := resources.EncodeTaskApprovalResumeContext(ctx)
	return out
}

func TestReconcileWaitingTaskApprovalSequentialRequestChangesCreatesNextReviewCycle(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	approvals := store.NewTaskApprovalStore()
	controller.SetTaskApprovalStore(approvals)

	if _, err := stores.agentStore.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "writer-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "write",
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	allow := true
	if _, err := stores.agentSystemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "seq-review-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"writer-agent"},
			Graph: map[string]resources.GraphEdge{
				"writer-agent": {
					Review: &resources.ReviewCheckpointSpec{
						CheckpointID:        "writer-review",
						Reason:              "Editor signoff required.",
						AllowRequestChanges: &allow,
						MaxReviewCycles:     3,
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "seq-review-task"},
		Spec:       resources.TaskSpec{System: "seq-review-system", Input: map[string]string{"topic": "appeal draft"}},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	task, ok, err := stores.taskStore.Get(context.Background(), "seq-review-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "WaitingApproval" {
		t.Fatalf("expected WaitingApproval after first review checkpoint, got %q", task.Status.Phase)
	}
	if task.Status.BlockedOn == nil || task.Status.BlockedOn.Name == "" {
		t.Fatalf("expected task to be blocked on a task approval, got %+v", task.Status.BlockedOn)
	}

	firstApproval, ok, err := approvals.Get(context.Background(), store.ScopedName("default", task.Status.BlockedOn.Name))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("first task approval %q not found", task.Status.BlockedOn.Name)
	}
	firstApproval.Status.Phase = "ChangesRequested"
	firstApproval.Status.Decision = "request_changes"
	firstApproval.Status.DecidedBy = "reviewer@example.com"
	firstApproval.Status.DecidedAt = time.Now().UTC().Format(time.RFC3339)
	firstApproval.Status.Comment = "Tighten the disclaimer and be more specific."
	if _, err := approvals.Upsert(context.Background(), firstApproval); err != nil {
		t.Fatalf("update first approval failed: %v", err)
	}

	if err := controller.reconcileWaitingApprovalSweep(context.Background()); err != nil {
		t.Fatalf("waiting approval sweep failed: %v", err)
	}

	task, ok, err = stores.taskStore.Get(context.Background(), "seq-review-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found after rerun")
	}
	if task.Status.Phase != "WaitingApproval" {
		t.Fatalf("expected task to pause again for next review cycle, got %q", task.Status.Phase)
	}
	if task.Status.BlockedOn == nil || task.Status.BlockedOn.Name == "" {
		t.Fatalf("expected task to block on a second approval, got %+v", task.Status.BlockedOn)
	}
	if task.Status.BlockedOn.Name == firstApproval.Metadata.Name {
		t.Fatalf("expected a new approval cycle, still blocked on %q", task.Status.BlockedOn.Name)
	}

	secondApproval, ok, err := approvals.Get(context.Background(), store.ScopedName("default", task.Status.BlockedOn.Name))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("second task approval %q not found", task.Status.BlockedOn.Name)
	}
	if secondApproval.Spec.ReviewCycle != 2 {
		t.Fatalf("expected review cycle 2, got %d", secondApproval.Spec.ReviewCycle)
	}
	if secondApproval.Spec.Supersedes != firstApproval.Metadata.Name {
		t.Fatalf("expected supersedes %q, got %q", firstApproval.Metadata.Name, secondApproval.Spec.Supersedes)
	}
	resume, err := resources.DecodeTaskApprovalResumeContext(secondApproval.Spec.ResumeContext)
	if err != nil {
		t.Fatalf("decode resume context failed: %v", err)
	}
	if resume.RuntimeInput["review.feedback"] != firstApproval.Status.Comment {
		t.Fatalf("expected review feedback to be injected, got %+v", resume.RuntimeInput)
	}
	if resume.RuntimeInput["review.previous_output"] != firstApproval.Spec.Output {
		t.Fatalf("expected previous output %v, got %+v", firstApproval.Spec.Output, resume.RuntimeInput)
	}
	if resume.RuntimeInput["review.checkpoint_id"] != firstApproval.Spec.CheckpointID {
		t.Fatalf("expected checkpoint id %q, got %+v", firstApproval.Spec.CheckpointID, resume.RuntimeInput)
	}
	if resume.RuntimeInput["review.cycle"] != "2" {
		t.Fatalf("expected review cycle 2 in runtime input, got %+v", resume.RuntimeInput)
	}
	if resume.RuntimeInput["review.requested_by"] != firstApproval.Status.DecidedBy {
		t.Fatalf("expected requested_by %q, got %+v", firstApproval.Status.DecidedBy, resume.RuntimeInput)
	}
	if resume.RuntimeInput["review.supersedes"] != firstApproval.Metadata.Name {
		t.Fatalf("expected supersedes %q in runtime input, got %+v", firstApproval.Metadata.Name, resume.RuntimeInput)
	}
}

func TestReconcileWaitingTaskApprovalSequentialCompletionReviewRequestChangesCreatesNextCycle(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	approvals := store.NewTaskApprovalStore()
	controller.SetTaskApprovalStore(approvals)

	if _, err := stores.agentStore.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "writer-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "write",
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	allow := true
	if _, err := stores.agentSystemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "seq-completion-review-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"writer-agent"},
			CompletionReview: &resources.ReviewCheckpointSpec{
				CheckpointID:        "final-review",
				Reason:              "Final signoff required.",
				AllowRequestChanges: &allow,
				MaxReviewCycles:     3,
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "seq-completion-review-task"},
		Spec:       resources.TaskSpec{System: "seq-completion-review-system", Input: map[string]string{"topic": "final memo"}},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	task, ok, err := stores.taskStore.Get(context.Background(), "seq-completion-review-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "WaitingApproval" {
		t.Fatalf("expected WaitingApproval at completion review, got %q", task.Status.Phase)
	}
	firstApproval, ok, err := approvals.Get(context.Background(), store.ScopedName("default", task.Status.BlockedOn.Name))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("first completion review approval %q not found", task.Status.BlockedOn.Name)
	}
	if firstApproval.Spec.CheckpointType != "task_output" {
		t.Fatalf("expected task_output checkpoint type, got %q", firstApproval.Spec.CheckpointType)
	}
	firstApproval.Status.Phase = "ChangesRequested"
	firstApproval.Status.Decision = "request_changes"
	firstApproval.Status.DecidedBy = "reviewer@example.com"
	firstApproval.Status.DecidedAt = time.Now().UTC().Format(time.RFC3339)
	firstApproval.Status.Comment = "One more revision before signoff."
	if _, err := approvals.Upsert(context.Background(), firstApproval); err != nil {
		t.Fatalf("update first approval failed: %v", err)
	}

	if err := controller.reconcileWaitingApprovalSweep(context.Background()); err != nil {
		t.Fatalf("waiting approval sweep failed: %v", err)
	}

	task, ok, err = stores.taskStore.Get(context.Background(), "seq-completion-review-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found after completion rerun")
	}
	if task.Status.Phase != "WaitingApproval" {
		t.Fatalf("expected task to pause again for completion review, got %q", task.Status.Phase)
	}
	if task.Status.BlockedOn == nil || task.Status.BlockedOn.Name == "" {
		t.Fatalf("expected blocked_on after completion rerun, got %+v", task.Status.BlockedOn)
	}
	if task.Status.BlockedOn.Name == firstApproval.Metadata.Name {
		t.Fatalf("expected second completion review approval, still blocked on %q", task.Status.BlockedOn.Name)
	}

	secondApproval, ok, err := approvals.Get(context.Background(), store.ScopedName("default", task.Status.BlockedOn.Name))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("second completion review approval %q not found", task.Status.BlockedOn.Name)
	}
	if secondApproval.Spec.CheckpointType != "task_output" {
		t.Fatalf("expected task_output checkpoint type, got %q", secondApproval.Spec.CheckpointType)
	}
	if secondApproval.Spec.ReviewCycle != 2 {
		t.Fatalf("expected completion review cycle 2, got %d", secondApproval.Spec.ReviewCycle)
	}
	if secondApproval.Spec.Supersedes != firstApproval.Metadata.Name {
		t.Fatalf("expected completion approval supersedes %q, got %q", firstApproval.Metadata.Name, secondApproval.Spec.Supersedes)
	}
	resume, err := resources.DecodeTaskApprovalResumeContext(secondApproval.Spec.ResumeContext)
	if err != nil {
		t.Fatalf("decode completion resume context failed: %v", err)
	}
	if resume.RuntimeInput["review.feedback"] != firstApproval.Status.Comment {
		t.Fatalf("expected review feedback in completion resume context, got %+v", resume.RuntimeInput)
	}
	if resume.RuntimeInput["review.supersedes"] != firstApproval.Metadata.Name {
		t.Fatalf("expected review supersedes %q, got %+v", firstApproval.Metadata.Name, resume.RuntimeInput)
	}
}

func TestReconcileWaitingTaskApprovalSequentialCompletionReviewApproveSucceedsTask(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	approvals := store.NewTaskApprovalStore()
	controller.SetTaskApprovalStore(approvals)

	if _, err := stores.agentStore.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "writer-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "write",
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	allow := true
	if _, err := stores.agentSystemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "seq-completion-approve-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"writer-agent"},
			CompletionReview: &resources.ReviewCheckpointSpec{
				CheckpointID:        "final-review",
				Reason:              "Final signoff required.",
				AllowRequestChanges: &allow,
				MaxReviewCycles:     3,
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "seq-completion-approve-task"},
		Spec:       resources.TaskSpec{System: "seq-completion-approve-system", Input: map[string]string{"topic": "final memo"}},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	task, ok, err := stores.taskStore.Get(context.Background(), "seq-completion-approve-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "WaitingApproval" {
		t.Fatalf("expected WaitingApproval at completion review, got %q", task.Status.Phase)
	}
	if task.Status.BlockedOn == nil || task.Status.BlockedOn.Name == "" {
		t.Fatalf("expected blocked_on completion approval, got %+v", task.Status.BlockedOn)
	}

	approval, ok, err := approvals.Get(context.Background(), store.ScopedName("default", task.Status.BlockedOn.Name))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("completion review approval %q not found", task.Status.BlockedOn.Name)
	}
	approval.Status.Phase = "Approved"
	approval.Status.Decision = "approved"
	approval.Status.DecidedBy = "reviewer@example.com"
	approval.Status.DecidedAt = time.Now().UTC().Format(time.RFC3339)
	if _, err := approvals.Upsert(context.Background(), approval); err != nil {
		t.Fatalf("update approval failed: %v", err)
	}

	if err := controller.reconcileWaitingApprovalSweep(context.Background()); err != nil {
		t.Fatalf("waiting approval sweep failed: %v", err)
	}

	task, ok, err = stores.taskStore.Get(context.Background(), "seq-completion-approve-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found after approval")
	}
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected Succeeded after completion review approval, got %q", task.Status.Phase)
	}
	if task.Status.BlockedOn != nil {
		t.Fatalf("expected blocked_on cleared after approval, got %+v", task.Status.BlockedOn)
	}
	if task.Status.CompletedAt == "" {
		t.Fatal("expected completed_at to be populated")
	}
	if len(task.Status.Output) == 0 {
		t.Fatalf("expected completion review approval to preserve output, got %+v", task.Status.Output)
	}
}

func TestReconcileWaitingTaskApprovalSequentialCompletionReviewDeniedFailsTask(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	approvals := store.NewTaskApprovalStore()
	controller.SetTaskApprovalStore(approvals)

	if _, err := stores.agentStore.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "writer-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "write",
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := stores.agentSystemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "seq-completion-deny-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"writer-agent"},
			CompletionReview: &resources.ReviewCheckpointSpec{
				CheckpointID: "final-review",
				Reason:       "Final signoff required.",
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "seq-completion-deny-task"},
		Spec:       resources.TaskSpec{System: "seq-completion-deny-system", Input: map[string]string{"topic": "final memo"}},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	task, ok, err := stores.taskStore.Get(context.Background(), "seq-completion-deny-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	approval, ok, err := approvals.Get(context.Background(), store.ScopedName("default", task.Status.BlockedOn.Name))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("completion review approval %q not found", task.Status.BlockedOn.Name)
	}
	approval.Status.Phase = "Denied"
	approval.Status.Decision = "denied"
	approval.Status.DecidedBy = "reviewer@example.com"
	approval.Status.DecidedAt = time.Now().UTC().Format(time.RFC3339)
	if _, err := approvals.Upsert(context.Background(), approval); err != nil {
		t.Fatalf("update approval failed: %v", err)
	}

	if err := controller.reconcileWaitingApprovalSweep(context.Background()); err != nil {
		t.Fatalf("waiting approval sweep failed: %v", err)
	}

	task, ok, err = stores.taskStore.Get(context.Background(), "seq-completion-deny-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found after denial")
	}
	if task.Status.Phase != "Failed" {
		t.Fatalf("expected Failed after completion review denial, got %q", task.Status.Phase)
	}
	if task.Status.LastError != "task approval denied" {
		t.Fatalf("expected denial reason to be recorded, got %q", task.Status.LastError)
	}
	if task.Status.Output != nil {
		t.Fatalf("expected task output to be cleared on failure, got %+v", task.Status.Output)
	}
}

func TestReconcileWaitingTaskApprovalSequentialCompletionReviewExpiryFailsTask(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	approvals := store.NewTaskApprovalStore()
	controller.SetTaskApprovalStore(approvals)

	if _, err := stores.agentStore.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "writer-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "write",
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := stores.agentSystemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "seq-completion-expire-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"writer-agent"},
			CompletionReview: &resources.ReviewCheckpointSpec{
				CheckpointID: "final-review",
				Reason:       "Final signoff required.",
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "seq-completion-expire-task"},
		Spec:       resources.TaskSpec{System: "seq-completion-expire-system", Input: map[string]string{"topic": "final memo"}},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	task, ok, err := stores.taskStore.Get(context.Background(), "seq-completion-expire-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	approvalName := task.Status.BlockedOn.Name
	approval, ok, err := approvals.Get(context.Background(), store.ScopedName("default", approvalName))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("completion review approval %q not found", approvalName)
	}
	approval.Status.ExpiresAt = time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339)
	if _, err := approvals.Upsert(context.Background(), approval); err != nil {
		t.Fatalf("update approval failed: %v", err)
	}

	if err := controller.reconcileWaitingApprovalSweep(context.Background()); err != nil {
		t.Fatalf("waiting approval sweep failed: %v", err)
	}

	task, ok, err = stores.taskStore.Get(context.Background(), "seq-completion-expire-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found after expiry")
	}
	if task.Status.Phase != "Failed" {
		t.Fatalf("expected Failed after completion review expiry, got %q", task.Status.Phase)
	}
	if task.Status.LastError != "task approval expired" {
		t.Fatalf("expected expiry reason to be recorded, got %q", task.Status.LastError)
	}
	approval, ok, err = approvals.Get(context.Background(), store.ScopedName("default", approvalName))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("completion review approval %q not found after expiry", approvalName)
	}
	if approval.Status.Phase != "Expired" {
		t.Fatalf("expected approval to transition to Expired, got %q", approval.Status.Phase)
	}
}

func TestReconcileWaitingTaskApprovalMessageDrivenRequestChangesPublishesReviewMessage(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	controller.SetExecutionMode("message-driven")
	approvals := store.NewTaskApprovalStore()
	controller.SetTaskApprovalStore(approvals)
	bus := &captureAgentMessageBus{}
	controller.SetAgentMessageBus(bus)

	taskName := "message-review-task"
	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: taskName},
		Spec:       resources.TaskSpec{System: "message-review-system"},
		Status: resources.TaskStatus{
			Phase:          "WaitingApproval",
			AssignedWorker: "test-worker",
			BlockedOn: &resources.TaskBlockedOn{
				Kind:   "TaskApproval",
				Name:   "msg-review-approval",
				Reason: "output_review",
			},
		},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	allow := true
	resumeContext := resources.TaskApprovalResumeContext{
		Mode:           "message-driven",
		Action:         "message_complete",
		System:         "message-review-system",
		ProducingAgent: "writer-agent",
		CurrentMessage: &resources.TaskApprovalResumeMessage{
			MessageID:      "msg-approval-1",
			IdempotencyKey: "msg-approval-1",
			TaskID:         "default/" + taskName,
			System:         "message-review-system",
			Namespace:      "default",
			FromAgent:      "system",
			ToAgent:        "writer-agent",
			Type:           "task_start",
			Payload:        "draft content",
			Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
			TraceID:        "default/" + taskName + "/a001",
		},
	}
	if _, err := approvals.Upsert(context.Background(), resources.TaskApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskApproval",
		Metadata: resources.ObjectMeta{
			Name:      "msg-review-approval",
			Namespace: "default",
		},
		Spec: resources.TaskApprovalSpec{
			TaskRef:             "default/" + taskName,
			CheckpointID:        "writer-review",
			CheckpointType:      "agent_output",
			Agent:               "writer-agent",
			AllowRequestChanges: &allow,
			MaxReviewCycles:     3,
			ReviewCycle:         1,
			Output:              "draft v1",
			OutputFormat:        "text",
			ResumeContext:       testEncodeResumeContext(resumeContext),
		},
		Status: resources.TaskApprovalStatus{
			Phase:     "ChangesRequested",
			Decision:  "request_changes",
			DecidedBy: "reviewer@example.com",
			DecidedAt: time.Now().UTC().Format(time.RFC3339),
			Comment:   "Please revise the conclusion.",
		},
	}); err != nil {
		t.Fatalf("upsert task approval failed: %v", err)
	}

	if err := controller.reconcileWaitingApprovalSweep(context.Background()); err != nil {
		t.Fatalf("waiting approval sweep failed: %v", err)
	}

	task, ok, err := stores.taskStore.Get(context.Background(), taskName)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found after message-driven request changes")
	}
	if task.Status.Phase != "Running" {
		t.Fatalf("expected task Running after request_changes rerun publish, got %q", task.Status.Phase)
	}
	if task.Status.BlockedOn != nil {
		t.Fatalf("expected blocked_on to be cleared after rerun publish, got %+v", task.Status.BlockedOn)
	}

	published := bus.Published()
	if len(published) != 1 {
		t.Fatalf("expected one published rerun message, got %d", len(published))
	}
	if published[0].Type != "review_request_changes" {
		t.Fatalf("expected review_request_changes message, got %q", published[0].Type)
	}
	if published[0].ToAgent != "writer-agent" {
		t.Fatalf("expected rerun target writer-agent, got %q", published[0].ToAgent)
	}
	payload, ok := agentruntime.DecodeReviewRequestPayload(published[0].Payload)
	if !ok {
		t.Fatalf("expected decodable review request payload, got %q", published[0].Payload)
	}
	if payload.Feedback != "Please revise the conclusion." {
		t.Fatalf("expected feedback in rerun payload, got %+v", payload)
	}
	if payload.PreviousOutput != "draft v1" {
		t.Fatalf("expected previous output in rerun payload, got %+v", payload)
	}
	if payload.CheckpointID != "writer-review" {
		t.Fatalf("expected checkpoint id writer-review, got %+v", payload)
	}
	if payload.Cycle != 2 {
		t.Fatalf("expected rerun cycle 2, got %+v", payload)
	}
	if payload.RequestedBy != "reviewer@example.com" {
		t.Fatalf("expected requested_by reviewer@example.com, got %+v", payload)
	}
	if payload.Supersedes != "msg-review-approval" {
		t.Fatalf("expected supersedes msg-review-approval, got %+v", payload)
	}
}

func TestReconcileWaitingTaskApprovalMessageDrivenApproveUsesFrozenResumeContextAfterSystemEdit(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	controller.SetExecutionMode("message-driven")
	approvals := store.NewTaskApprovalStore()
	controller.SetTaskApprovalStore(approvals)
	bus := &captureAgentMessageBus{}
	controller.SetAgentMessageBus(bus)

	for _, agent := range []resources.Agent{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "planner-agent"},
			Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "plan"},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "writer-agent"},
			Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "write"},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "compliance-agent"},
			Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "comply"},
		},
	} {
		if _, err := stores.agentStore.Upsert(context.Background(), agent); err != nil {
			t.Fatalf("upsert agent %s failed: %v", agent.Metadata.Name, err)
		}
	}

	if _, err := stores.agentSystemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "frozen-message-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"planner-agent", "writer-agent"},
			Graph: map[string]resources.GraphEdge{
				"planner-agent": {Next: "writer-agent"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert initial system failed: %v", err)
	}

	taskName := "frozen-message-task"
	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: taskName},
		Spec:       resources.TaskSpec{System: "frozen-message-system"},
		Status: resources.TaskStatus{
			Phase:          "WaitingApproval",
			AssignedWorker: "test-worker",
			BlockedOn: &resources.TaskBlockedOn{
				Kind:   "TaskApproval",
				Name:   "frozen-approval",
				Reason: "output_review",
			},
		},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	nextMessage := resources.TaskApprovalResumeMessage{
		MessageID:      "frozen-msg-2",
		IdempotencyKey: "frozen-msg-2",
		TaskID:         "default/" + taskName,
		System:         "frozen-message-system",
		Namespace:      "default",
		FromAgent:      "planner-agent",
		ToAgent:        "writer-agent",
		Type:           "task_handoff",
		Payload:        "draft to writer",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		TraceID:        "default/" + taskName + "/a001",
	}
	if _, err := approvals.Upsert(context.Background(), resources.TaskApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskApproval",
		Metadata: resources.ObjectMeta{
			Name:      "frozen-approval",
			Namespace: "default",
		},
		Spec: resources.TaskApprovalSpec{
			TaskRef:        "default/" + taskName,
			CheckpointID:   "writer-review",
			CheckpointType: "agent_output",
			Agent:          "planner-agent",
			ReviewCycle:    1,
			Output:         "draft v1",
			OutputFormat:   "text",
			ResumeContext: testEncodeResumeContext(resources.TaskApprovalResumeContext{
				Mode:           "message-driven",
				Action:         "message_forward",
				System:         "frozen-message-system",
				ProducingAgent: "planner-agent",
				CurrentMessage: &resources.TaskApprovalResumeMessage{
					MessageID:      "frozen-msg-1",
					IdempotencyKey: "frozen-msg-1",
					TaskID:         "default/" + taskName,
					System:         "frozen-message-system",
					Namespace:      "default",
					FromAgent:      "system",
					ToAgent:        "planner-agent",
					Type:           "task_start",
					Payload:        "start",
					Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
					TraceID:        "default/" + taskName + "/a001",
				},
				NextMessages: []resources.TaskApprovalResumeMessage{nextMessage},
			}),
		},
		Status: resources.TaskApprovalStatus{
			Phase:     "Approved",
			Decision:  "approved",
			DecidedBy: "reviewer@example.com",
			DecidedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}); err != nil {
		t.Fatalf("upsert task approval failed: %v", err)
	}

	if _, err := stores.agentSystemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "frozen-message-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"planner-agent", "compliance-agent"},
			Graph: map[string]resources.GraphEdge{
				"planner-agent": {Next: "compliance-agent"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert edited system failed: %v", err)
	}

	if err := controller.reconcileWaitingApprovalSweep(context.Background()); err != nil {
		t.Fatalf("waiting approval sweep failed: %v", err)
	}

	task, ok, err := stores.taskStore.Get(context.Background(), taskName)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found after approval")
	}
	if task.Status.Phase != "Running" {
		t.Fatalf("expected Running after approval, got %q", task.Status.Phase)
	}
	if task.Status.BlockedOn != nil {
		t.Fatalf("expected blocked_on cleared after approval, got %+v", task.Status.BlockedOn)
	}

	published := bus.Published()
	if len(published) != 1 {
		t.Fatalf("expected one published downstream message, got %d", len(published))
	}
	if published[0].ToAgent != "writer-agent" {
		t.Fatalf("expected frozen downstream target writer-agent, got %q", published[0].ToAgent)
	}
	if published[0].MessageID != nextMessage.MessageID {
		t.Fatalf("expected frozen message id %q, got %q", nextMessage.MessageID, published[0].MessageID)
	}
}

func TestReconcileWaitingTaskApprovalMessageDrivenCompletionReviewApproveSucceedsTask(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	controller.SetExecutionMode("message-driven")
	approvals := store.NewTaskApprovalStore()
	controller.SetTaskApprovalStore(approvals)
	bus := &captureAgentMessageBus{}
	controller.SetAgentMessageBus(bus)

	taskName := "message-completion-approve-task"
	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: taskName},
		Spec:       resources.TaskSpec{System: "message-review-system"},
		Status: resources.TaskStatus{
			Phase:          "WaitingApproval",
			AssignedWorker: "test-worker",
			BlockedOn: &resources.TaskBlockedOn{
				Kind:   "TaskApproval",
				Name:   "message-completion-approval",
				Reason: "output_review",
			},
		},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	if _, err := approvals.Upsert(context.Background(), resources.TaskApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskApproval",
		Metadata: resources.ObjectMeta{
			Name:      "message-completion-approval",
			Namespace: "default",
		},
		Spec: resources.TaskApprovalSpec{
			TaskRef:        "default/" + taskName,
			CheckpointID:   "final-review",
			CheckpointType: "task_output",
			Agent:          "writer-agent",
			ReviewCycle:    1,
			Output: map[string]string{
				"final_report": "approved final output",
			},
			OutputFormat: "json",
			ResumeContext: testEncodeResumeContext(resources.TaskApprovalResumeContext{
				Mode:           "message-driven",
				Action:         "message_complete",
				System:         "message-review-system",
				ProducingAgent: "writer-agent",
				CurrentMessage: &resources.TaskApprovalResumeMessage{
					MessageID:      "msg-completion-1",
					IdempotencyKey: "msg-completion-1",
					TaskID:         "default/" + taskName,
					System:         "message-review-system",
					Namespace:      "default",
					FromAgent:      "system",
					ToAgent:        "writer-agent",
					Type:           "task_start",
					Payload:        "draft output",
					Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
					TraceID:        "default/" + taskName + "/a001",
				},
				Output: map[string]string{
					"final_report": "approved final output",
				},
			}),
		},
		Status: resources.TaskApprovalStatus{
			Phase:     "Approved",
			Decision:  "approved",
			DecidedBy: "reviewer@example.com",
			DecidedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}); err != nil {
		t.Fatalf("upsert task approval failed: %v", err)
	}

	if err := controller.reconcileWaitingApprovalSweep(context.Background()); err != nil {
		t.Fatalf("waiting approval sweep failed: %v", err)
	}

	task, ok, err := stores.taskStore.Get(context.Background(), taskName)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found after approval")
	}
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected Succeeded after message-driven completion approval, got %q", task.Status.Phase)
	}
	if task.Status.BlockedOn != nil {
		t.Fatalf("expected blocked_on cleared after message-driven completion approval, got %+v", task.Status.BlockedOn)
	}
	if task.Status.Output["final_report"] != "approved final output" {
		t.Fatalf("expected final output to be restored from resume context, got %+v", task.Status.Output)
	}
	if len(bus.Published()) != 0 {
		t.Fatalf("expected no downstream messages for completion approval, got %d", len(bus.Published()))
	}
	if len(task.Status.Messages) != 1 || task.Status.Messages[0].Phase != "Succeeded" {
		t.Fatalf("expected current message to be marked succeeded, got %+v", task.Status.Messages)
	}
}

func TestReconcileWaitingTaskApprovalMessageDrivenChangesRequestedDisabledFailsClosed(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	controller.SetExecutionMode("message-driven")
	approvals := store.NewTaskApprovalStore()
	controller.SetTaskApprovalStore(approvals)
	bus := &captureAgentMessageBus{}
	controller.SetAgentMessageBus(bus)

	taskName := "message-disabled-request-changes-task"
	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: taskName},
		Spec:       resources.TaskSpec{System: "message-review-system"},
		Status: resources.TaskStatus{
			Phase:          "WaitingApproval",
			AssignedWorker: "test-worker",
			BlockedOn: &resources.TaskBlockedOn{
				Kind:   "TaskApproval",
				Name:   "disabled-request-changes-approval",
				Reason: "output_review",
			},
		},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	allow := false
	if _, err := approvals.Upsert(context.Background(), resources.TaskApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskApproval",
		Metadata: resources.ObjectMeta{
			Name:      "disabled-request-changes-approval",
			Namespace: "default",
		},
		Spec: resources.TaskApprovalSpec{
			TaskRef:             "default/" + taskName,
			CheckpointID:        "writer-review",
			CheckpointType:      "agent_output",
			Agent:               "writer-agent",
			AllowRequestChanges: &allow,
			MaxReviewCycles:     3,
			ReviewCycle:         1,
			Output:              "draft v1",
			OutputFormat:        "text",
			ResumeContext: testEncodeResumeContext(resources.TaskApprovalResumeContext{
				Mode:           "message-driven",
				Action:         "message_complete",
				System:         "message-review-system",
				ProducingAgent: "writer-agent",
				CurrentMessage: &resources.TaskApprovalResumeMessage{
					MessageID:      "msg-disabled-request-changes-1",
					IdempotencyKey: "msg-disabled-request-changes-1",
					TaskID:         "default/" + taskName,
					System:         "message-review-system",
					Namespace:      "default",
					FromAgent:      "system",
					ToAgent:        "writer-agent",
					Type:           "task_start",
					Payload:        "draft content",
					Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
					TraceID:        "default/" + taskName + "/a001",
				},
			}),
		},
		Status: resources.TaskApprovalStatus{
			Phase:     "ChangesRequested",
			Decision:  "request_changes",
			DecidedBy: "reviewer@example.com",
			DecidedAt: time.Now().UTC().Format(time.RFC3339),
			Comment:   "Please revise the conclusion.",
		},
	}); err != nil {
		t.Fatalf("upsert task approval failed: %v", err)
	}

	if err := controller.reconcileWaitingApprovalSweep(context.Background()); err != nil {
		t.Fatalf("waiting approval sweep failed: %v", err)
	}

	task, ok, err := stores.taskStore.Get(context.Background(), taskName)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found after disabled request_changes")
	}
	if task.Status.Phase != "Failed" {
		t.Fatalf("expected Failed when request_changes is disabled, got %q", task.Status.Phase)
	}
	if task.Status.LastError != "task approval request_changes is disabled for this checkpoint" {
		t.Fatalf("expected fail-closed reason, got %q", task.Status.LastError)
	}
	if published := bus.Published(); len(published) != 0 {
		t.Fatalf("expected no rerun message to be published, got %d", len(published))
	}
}

func TestReconcileWaitingApprovalSweepUsesBlockedOnExactTaskApproval(t *testing.T) {
	controller, stores := newTaskControllerHarness()
	approvals := store.NewTaskApprovalStore()
	controller.SetTaskApprovalStore(approvals)
	toolApprovals := store.NewToolApprovalStore()
	controller.SetToolApprovalStore(toolApprovals)
	bus := &captureAgentMessageBus{}
	controller.SetAgentMessageBus(bus)

	taskName := "blocked-on-task"
	taskKey := store.ScopedName("default", taskName)
	if _, err := stores.taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: taskName},
		Spec:       resources.TaskSpec{System: "blocked-on-system"},
		Status: resources.TaskStatus{
			Phase:          "WaitingApproval",
			AssignedWorker: "test-worker",
			BlockedOn: &resources.TaskBlockedOn{
				Kind:   "TaskApproval",
				Name:   "target-approval",
				Reason: "output_review",
			},
		},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	if _, err := approvals.Upsert(context.Background(), resources.TaskApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskApproval",
		Metadata: resources.ObjectMeta{
			Name:      "target-approval",
			Namespace: "default",
		},
		Spec: resources.TaskApprovalSpec{
			TaskRef:        taskKey,
			CheckpointID:   "writer-review",
			CheckpointType: "agent_output",
		},
		Status: resources.TaskApprovalStatus{
			Phase: "Pending",
		},
	}); err != nil {
		t.Fatalf("upsert target approval failed: %v", err)
	}
	if _, err := approvals.Upsert(context.Background(), resources.TaskApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskApproval",
		Metadata: resources.ObjectMeta{
			Name:      "other-approved-approval",
			Namespace: "default",
		},
		Spec: resources.TaskApprovalSpec{
			TaskRef:        taskKey,
			CheckpointID:   "other-review",
			CheckpointType: "agent_output",
		},
		Status: resources.TaskApprovalStatus{
			Phase:     "Approved",
			Decision:  "approved",
			DecidedBy: "reviewer@example.com",
			DecidedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}); err != nil {
		t.Fatalf("upsert unrelated approved task approval failed: %v", err)
	}
	if _, err := toolApprovals.Upsert(context.Background(), resources.ToolApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "ToolApproval",
		Metadata: resources.ObjectMeta{
			Name:      "tool-approved",
			Namespace: "default",
		},
		Spec: resources.ToolApprovalSpec{
			TaskRef: taskKey,
			Tool:    "web_search",
		},
		Status: resources.ToolApprovalStatus{
			Phase:     "Approved",
			Decision:  "approved",
			DecidedBy: "reviewer@example.com",
			DecidedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}); err != nil {
		t.Fatalf("upsert unrelated approved tool approval failed: %v", err)
	}

	if err := controller.reconcileWaitingApprovalSweep(context.Background()); err != nil {
		t.Fatalf("waiting approval sweep failed: %v", err)
	}

	task, ok, err := stores.taskStore.Get(context.Background(), taskName)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found after sweep")
	}
	if task.Status.Phase != "WaitingApproval" {
		t.Fatalf("expected task to remain WaitingApproval, got %q", task.Status.Phase)
	}
	if task.Status.BlockedOn == nil || task.Status.BlockedOn.Name != "target-approval" {
		t.Fatalf("expected task to remain blocked on target approval, got %+v", task.Status.BlockedOn)
	}
	if published := bus.Published(); len(published) != 0 {
		t.Fatalf("expected no messages published while blocked approval is still pending, got %d", len(published))
	}
}
