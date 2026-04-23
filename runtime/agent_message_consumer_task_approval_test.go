package agentruntime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

func TestAgentMessageConsumerReviewRequestChangesCreatesNextTaskApprovalCycle(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()
	taskApprovalStore := store.NewTaskApprovalStore()

	if _, err := agentStore.Upsert(context.Background(), resources.Agent{
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
	if _, err := systemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "message-review-system"},
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

	if _, err := taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "message-review-task"},
		Spec:       resources.TaskSpec{System: "message-review-system", Input: map[string]string{"topic": "claims letter"}},
		Status: resources.TaskStatus{
			Phase:     "Running",
			ClaimedBy: "worker-a",
			Attempts:  1,
		},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	manager := NewAgentMessageConsumerManager(
		bus,
		agentStore,
		systemStore,
		taskStore,
		nil,
		AgentMessageConsumerOptions{
			ModelEndpoints:      newTestModelEndpointStore(t),
			TaskApprovals:       taskApprovalStore,
			WorkerID:            "worker-a",
			RefreshEvery:        20 * time.Millisecond,
			DedupeWindow:        time.Minute,
			LeaseExtendDuration: 30 * time.Second,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Start(ctx)
	waitForConsumerSubscriptions(t, manager, bus, 2*time.Second)

	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "msg-review-rerun-1",
		TaskID:    "default/message-review-task",
		System:    "message-review-system",
		Namespace: "default",
		FromAgent: "system",
		ToAgent:   "writer-agent",
		Type:      "review_request_changes",
		Payload: EncodeReviewRequestPayload(ReviewRequestPayload{
			Content:        "draft v1",
			Feedback:       "Please tighten the disclaimer.",
			PreviousOutput: "draft v0",
			CheckpointID:   "writer-review",
			Cycle:          2,
			RequestedBy:    "reviewer@example.com",
			Supersedes:     "tra-old",
		}),
		Attempt: 1,
		TraceID: "default/message-review-task/a001",
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	waitForConsumer(t, 2*time.Second, func() bool {
		task, ok, _ := taskStore.Get(context.Background(), "message-review-task")
		return ok && strings.EqualFold(task.Status.Phase, "waitingapproval") && task.Status.BlockedOn != nil && task.Status.BlockedOn.Name != ""
	})

	task, ok, err := taskStore.Get(context.Background(), "message-review-task")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "WaitingApproval" {
		t.Fatalf("expected WaitingApproval after rerun review output, got %q", task.Status.Phase)
	}
	if task.Status.BlockedOn == nil || task.Status.BlockedOn.Name == "" {
		t.Fatalf("expected blocked_on task approval, got %+v", task.Status.BlockedOn)
	}

	approval, ok, err := taskApprovalStore.Get(context.Background(), store.ScopedName("default", task.Status.BlockedOn.Name))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("task approval %q not found", task.Status.BlockedOn.Name)
	}
	if approval.Spec.ReviewCycle != 2 {
		t.Fatalf("expected review cycle 2, got %d", approval.Spec.ReviewCycle)
	}
	if approval.Spec.Supersedes != "tra-old" {
		t.Fatalf("expected supersedes tra-old, got %q", approval.Spec.Supersedes)
	}
	if approval.Spec.Agent != "writer-agent" {
		t.Fatalf("expected approval agent writer-agent, got %q", approval.Spec.Agent)
	}

	resume, err := resources.DecodeTaskApprovalResumeContext(approval.Spec.ResumeContext)
	if err != nil {
		t.Fatalf("decode resume context failed: %v", err)
	}
	if resume.CurrentMessage == nil {
		t.Fatal("expected current message in resume context")
	}
	if resume.CurrentMessage.Type != "review_request_changes" {
		t.Fatalf("expected current message type review_request_changes, got %q", resume.CurrentMessage.Type)
	}
	payload, ok := DecodeReviewRequestPayload(resume.CurrentMessage.Payload)
	if !ok {
		t.Fatalf("expected decodable current message payload, got %q", resume.CurrentMessage.Payload)
	}
	if payload.Feedback != "Please tighten the disclaimer." {
		t.Fatalf("expected feedback to round-trip, got %+v", payload)
	}
	if payload.PreviousOutput != "draft v0" {
		t.Fatalf("expected previous output to round-trip, got %+v", payload)
	}
	if payload.Cycle != 2 {
		t.Fatalf("expected cycle 2 to round-trip, got %+v", payload)
	}
	if payload.Supersedes != "tra-old" {
		t.Fatalf("expected supersedes tra-old to round-trip, got %+v", payload)
	}

	message, ok := taskMessageByID(task.Status.Messages, "msg-review-rerun-1")
	if !ok {
		t.Fatal("expected rerun message to be recorded on task")
	}
	if message.Phase != "WaitingApproval" {
		t.Fatalf("expected rerun message phase WaitingApproval, got %q", message.Phase)
	}
}
