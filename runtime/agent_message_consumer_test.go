package agentruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/AnonJon/orloj/crds"
	"github.com/AnonJon/orloj/store"
)

func TestAgentMessageConsumerExecutesGraphAndCompletesTask(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()

	for _, agent := range []crds.Agent{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   crds.ObjectMeta{Name: "planner-agent"},
			Spec: crds.AgentSpec{
				Model:  "gpt-4o",
				Prompt: "plan",
				Limits: crds.AgentLimits{MaxSteps: 1, Timeout: "1s"},
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   crds.ObjectMeta{Name: "writer-agent"},
			Spec: crds.AgentSpec{
				Model:  "gpt-4o",
				Prompt: "write",
				Limits: crds.AgentLimits{MaxSteps: 1, Timeout: "1s"},
			},
		},
	} {
		if _, err := agentStore.Upsert(agent); err != nil {
			t.Fatalf("upsert agent failed: %v", err)
		}
	}

	if _, err := systemStore.Upsert(crds.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   crds.ObjectMeta{Name: "report-system"},
		Spec: crds.AgentSystemSpec{
			Agents: []string{"planner-agent", "writer-agent"},
			Graph: map[string]crds.GraphEdge{
				"planner-agent": {Next: "writer-agent"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}

	if _, err := taskStore.Upsert(crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "task-1"},
		Spec: crds.TaskSpec{
			System: "report-system",
			Input:  map[string]string{"topic": "agent systems"},
		},
		Status: crds.TaskStatus{
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
			WorkerID:            "worker-a",
			RefreshEvery:        20 * time.Millisecond,
			DedupeWindow:        time.Minute,
			LeaseExtendDuration: 30 * time.Second,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Start(ctx)
	time.Sleep(40 * time.Millisecond)

	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "msg-1",
		TaskID:    "default/task-1",
		Namespace: "default",
		FromAgent: "system",
		ToAgent:   "planner-agent",
		Type:      "task_start",
		Payload:   "start",
		Attempt:   1,
		TraceID:   "default/task-1/a001",
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	waitForConsumer(t, 2*time.Second, func() bool {
		task, ok := taskStore.Get("task-1")
		return ok && strings.EqualFold(task.Status.Phase, "succeeded")
	})

	task, _ := taskStore.Get("task-1")
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected task succeeded, got %q", task.Status.Phase)
	}
	if task.Status.Output["runtime.mode"] != "message-driven" {
		t.Fatalf("expected runtime.mode=message-driven, got %q", task.Status.Output["runtime.mode"])
	}
	if task.Status.Output["last_agent"] != "writer-agent" {
		t.Fatalf("expected last_agent writer-agent, got %q", task.Status.Output["last_agent"])
	}
	if countMessages(task.Status.Messages, "msg-1") != 1 {
		t.Fatalf("expected one kickoff message record, got %d", countMessages(task.Status.Messages, "msg-1"))
	}
	nextID := "default/task-1/a001/h002/planner-agent/writer-agent"
	if countMessages(task.Status.Messages, nextID) != 1 {
		t.Fatalf("expected one forwarded message record %q, got %d", nextID, countMessages(task.Status.Messages, nextID))
	}
	if countTraceByTypeAndMessage(task.Status.Trace, "agent_message_processed", "msg-1") == 0 {
		t.Fatal("expected processed trace for kickoff message")
	}
	if countTraceByTypeAndMessage(task.Status.Trace, "agent_message_processed", nextID) == 0 {
		t.Fatal("expected processed trace for forwarded message")
	}
}

func TestAgentMessageConsumerWaitsForLeaseThenTakesOver(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()

	if _, err := agentStore.Upsert(crds.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   crds.ObjectMeta{Name: "research-agent"},
		Spec:       crds.AgentSpec{Model: "gpt-4o", Prompt: "research"},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := systemStore.Upsert(crds.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   crds.ObjectMeta{Name: "report-system"},
		Spec:       crds.AgentSystemSpec{Agents: []string{"research-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}

	if _, err := taskStore.Upsert(crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "task-2"},
		Spec:       crds.TaskSpec{System: "report-system"},
		Status: crds.TaskStatus{
			Phase:      "Running",
			ClaimedBy:  "worker-owner",
			LeaseUntil: time.Now().UTC().Add(180 * time.Millisecond).Format(time.RFC3339Nano),
			Attempts:   1,
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
			WorkerID:     "worker-other",
			RefreshEvery: 20 * time.Millisecond,
			DedupeWindow: time.Minute,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Start(ctx)
	time.Sleep(40 * time.Millisecond)

	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "msg-skip",
		TaskID:    "default/task-2",
		FromAgent: "planner-agent",
		ToAgent:   "research-agent",
		Type:      "task_handoff",
		Payload:   "work item",
		Attempt:   1,
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	time.Sleep(90 * time.Millisecond)

	task, _ := taskStore.Get("task-2")
	if countMessages(task.Status.Messages, "msg-skip") != 0 {
		t.Fatalf("expected no message persisted for non-owner worker, got %d", countMessages(task.Status.Messages, "msg-skip"))
	}
	if countTraceByTypeAndMessage(task.Status.Trace, "agent_message_processed", "msg-skip") != 0 {
		t.Fatalf("expected no processed trace for non-owner worker, got %d", countTraceByTypeAndMessage(task.Status.Trace, "agent_message_processed", "msg-skip"))
	}

	waitForConsumer(t, 2*time.Second, func() bool {
		current, ok := taskStore.Get("task-2")
		if !ok {
			return false
		}
		return strings.EqualFold(current.Status.Phase, "succeeded") && countTraceByTypeAndMessage(current.Status.Trace, "agent_message_processed", "msg-skip") == 1
	})

	task, _ = taskStore.Get("task-2")
	seenTakeover := false
	for _, entry := range task.Status.History {
		if strings.EqualFold(strings.TrimSpace(entry.Type), "takeover") && strings.EqualFold(strings.TrimSpace(entry.Worker), "worker-other") {
			seenTakeover = true
			break
		}
	}
	if !seenTakeover {
		t.Fatalf("expected takeover history event for worker-other, history=%+v", task.Status.History)
	}
}

func TestAgentMessageConsumerRetriesThenDeadLettersMessage(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()

	if _, err := agentStore.Upsert(crds.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   crds.ObjectMeta{Name: "planner-agent"},
		Spec: crds.AgentSpec{
			Model:  "gpt-4o",
			Prompt: "plan",
			Limits: crds.AgentLimits{MaxSteps: 50, Timeout: "1ms"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := systemStore.Upsert(crds.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   crds.ObjectMeta{Name: "retry-system"},
		Spec:       crds.AgentSystemSpec{Agents: []string{"planner-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := taskStore.Upsert(crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "retry-task"},
		Spec: crds.TaskSpec{
			System: "retry-system",
			Retry: crds.TaskRetryPolicy{
				MaxAttempts: 4,
				Backoff:     "800ms",
			},
			MessageRetry: crds.TaskMessageRetryPolicy{
				MaxAttempts: 2,
				Backoff:     "120ms",
				MaxBackoff:  "250ms",
				Jitter:      "none",
			},
		},
		Status: crds.TaskStatus{
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
			WorkerID:            "worker-a",
			RefreshEvery:        20 * time.Millisecond,
			DedupeWindow:        time.Minute,
			LeaseExtendDuration: 30 * time.Second,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Start(ctx)
	time.Sleep(40 * time.Millisecond)

	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "msg-retry",
		TaskID:    "default/retry-task",
		Namespace: "default",
		FromAgent: "system",
		ToAgent:   "planner-agent",
		Type:      "task_start",
		Payload:   "start",
		Attempt:   1,
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	start := time.Now()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		task, ok := taskStore.Get("retry-task")
		if ok && strings.EqualFold(task.Status.Phase, "deadletter") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	task, _ := taskStore.Get("retry-task")
	if task.Status.Phase != "DeadLetter" {
		payload, _ := json.Marshal(task.Status)
		t.Fatalf("expected task DeadLetter, got %q status=%s", task.Status.Phase, string(payload))
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond || elapsed > 700*time.Millisecond {
		t.Fatalf("expected message_retry backoff window (~120ms, not task retry 800ms), got elapsed=%s", elapsed)
	}
	if countTraceByTypeAndMessage(task.Status.Trace, "agent_message_retry_scheduled", "msg-retry") == 0 {
		t.Fatal("expected retry_scheduled trace for msg-retry")
	}
	if countTraceByTypeAndMessage(task.Status.Trace, "agent_message_deadletter", "msg-retry") == 0 {
		t.Fatal("expected deadletter trace for msg-retry")
	}
	message, ok := taskMessageByID(task.Status.Messages, "msg-retry")
	if !ok {
		t.Fatal("expected retry message record in task status")
	}
	if message.Phase != "DeadLetter" {
		t.Fatalf("expected message phase DeadLetter, got %q", message.Phase)
	}
	if message.Attempts != 2 {
		t.Fatalf("expected message attempts=2, got %d", message.Attempts)
	}
	if message.MaxAttempts != 2 {
		t.Fatalf("expected message maxAttempts=2, got %d", message.MaxAttempts)
	}
	if strings.TrimSpace(message.LastError) == "" {
		t.Fatal("expected message last_error to be set")
	}
}

func TestAgentMessageConsumerNonRetryableInvalidSystemDeadLettersImmediately(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()

	if _, err := agentStore.Upsert(crds.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   crds.ObjectMeta{Name: "runner-agent"},
		Spec:       crds.AgentSpec{Model: "gpt-4o", Prompt: "run"},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := taskStore.Upsert(crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "invalid-ref-task"},
		Spec: crds.TaskSpec{
			System: "missing-system",
			MessageRetry: crds.TaskMessageRetryPolicy{
				MaxAttempts: 5,
				Backoff:     "150ms",
				MaxBackoff:  "2s",
				Jitter:      "none",
			},
		},
		Status: crds.TaskStatus{
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
			WorkerID:            "worker-a",
			RefreshEvery:        20 * time.Millisecond,
			DedupeWindow:        time.Minute,
			LeaseExtendDuration: 30 * time.Second,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Start(ctx)
	time.Sleep(40 * time.Millisecond)

	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "msg-invalid-agent",
		TaskID:    "default/invalid-ref-task",
		Namespace: "default",
		FromAgent: "system",
		ToAgent:   "runner-agent",
		Type:      "task_start",
		Payload:   "start",
		Attempt:   1,
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	waitForConsumer(t, 2*time.Second, func() bool {
		task, ok := taskStore.Get("invalid-ref-task")
		return ok && strings.EqualFold(task.Status.Phase, "deadletter")
	})

	task, _ := taskStore.Get("invalid-ref-task")
	if countTraceByTypeAndMessage(task.Status.Trace, "agent_message_retry_scheduled", "msg-invalid-agent") != 0 {
		t.Fatalf("expected no retry for non-retryable invalid system ref, trace=%+v", task.Status.Trace)
	}
	if countTraceByTypeAndMessage(task.Status.Trace, "agent_message_non_retryable", "msg-invalid-agent") == 0 {
		t.Fatal("expected non_retryable trace marker")
	}
	message, ok := taskMessageByID(task.Status.Messages, "msg-invalid-agent")
	if !ok {
		t.Fatal("expected deadletter message record")
	}
	if message.Attempts != 1 {
		t.Fatalf("expected attempts=1 for non-retryable failure, got %d", message.Attempts)
	}
}

func TestComputeMessageRetryDelayCappedAndJitterModes(t *testing.T) {
	msg := AgentMessage{MessageID: "msg-delay", TaskID: "default/delay-task", ToAgent: "writer-agent"}

	none := computeMessageRetryDelay(crds.TaskMessageRetryPolicy{
		Backoff:    "50ms",
		MaxBackoff: "120ms",
		Jitter:     "none",
	}, msg, 3)
	if none != 120*time.Millisecond {
		t.Fatalf("expected capped delay=120ms for attempt=3, got %s", none)
	}

	full := computeMessageRetryDelay(crds.TaskMessageRetryPolicy{
		Backoff:    "100ms",
		MaxBackoff: "5s",
		Jitter:     "full",
	}, msg, 1)
	if full <= 0 || full > 100*time.Millisecond {
		t.Fatalf("expected full jitter delay in (0,100ms], got %s", full)
	}

	equal := computeMessageRetryDelay(crds.TaskMessageRetryPolicy{
		Backoff:    "100ms",
		MaxBackoff: "5s",
		Jitter:     "equal",
	}, msg, 1)
	if equal < 50*time.Millisecond || equal > 100*time.Millisecond {
		t.Fatalf("expected equal jitter delay in [50ms,100ms], got %s", equal)
	}
}

func TestAgentMessageConsumerFanOutJoinWaitForAll(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 512, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()

	for _, agent := range []crds.Agent{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   crds.ObjectMeta{Name: "planner-agent"},
			Spec:       crds.AgentSpec{Model: "gpt-4o", Prompt: "plan", Limits: crds.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   crds.ObjectMeta{Name: "researcher-agent"},
			Spec:       crds.AgentSpec{Model: "gpt-4o", Prompt: "research", Limits: crds.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   crds.ObjectMeta{Name: "reviewer-agent"},
			Spec:       crds.AgentSpec{Model: "gpt-4o", Prompt: "review", Limits: crds.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   crds.ObjectMeta{Name: "writer-agent"},
			Spec:       crds.AgentSpec{Model: "gpt-4o", Prompt: "write", Limits: crds.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
		},
	} {
		if _, err := agentStore.Upsert(agent); err != nil {
			t.Fatalf("upsert agent failed: %v", err)
		}
	}

	if _, err := systemStore.Upsert(crds.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   crds.ObjectMeta{Name: "fanout-system"},
		Spec: crds.AgentSystemSpec{
			Agents: []string{"planner-agent", "researcher-agent", "reviewer-agent", "writer-agent"},
			Graph: map[string]crds.GraphEdge{
				"planner-agent": {
					Edges: []crds.GraphRoute{
						{To: "researcher-agent"},
						{To: "reviewer-agent"},
					},
				},
				"researcher-agent": {Edges: []crds.GraphRoute{{To: "writer-agent"}}},
				"reviewer-agent":   {Edges: []crds.GraphRoute{{To: "writer-agent"}}},
				"writer-agent": {
					Join: crds.GraphJoin{Mode: "wait_for_all"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}

	if _, err := taskStore.Upsert(crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "fanout-task"},
		Spec:       crds.TaskSpec{System: "fanout-system", Input: map[string]string{"topic": "fanout"}},
		Status: crds.TaskStatus{
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
			WorkerID:            "worker-a",
			RefreshEvery:        20 * time.Millisecond,
			DedupeWindow:        time.Minute,
			LeaseExtendDuration: 30 * time.Second,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Start(ctx)
	time.Sleep(40 * time.Millisecond)

	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID:      "msg-fanout-root",
		IdempotencyKey: "msg-fanout-root",
		TaskID:         "default/fanout-task",
		Namespace:      "default",
		FromAgent:      "system",
		ToAgent:        "planner-agent",
		BranchID:       "b001",
		Type:           "task_start",
		Payload:        "start",
		Attempt:        1,
		TraceID:        "default/fanout-task/a001",
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	waitForConsumer(t, 4*time.Second, func() bool {
		task, ok := taskStore.Get("fanout-task")
		return ok && strings.EqualFold(task.Status.Phase, "succeeded")
	})

	task, _ := taskStore.Get("fanout-task")
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected fanout task succeeded, got %q", task.Status.Phase)
	}
	if countTraceByAgentAndType(task.Status.Trace, "writer-agent", "agent_worker_start") != 1 {
		payload, _ := json.Marshal(task.Status.Trace)
		t.Fatalf("expected writer to execute exactly once, trace=%s", string(payload))
	}

	if len(task.Status.JoinStates) == 0 {
		t.Fatal("expected join state to be recorded")
	}
	var writerJoin *crds.TaskJoinState
	for i := range task.Status.JoinStates {
		if strings.EqualFold(task.Status.JoinStates[i].Node, "writer-agent") {
			writerJoin = &task.Status.JoinStates[i]
			break
		}
	}
	if writerJoin == nil {
		t.Fatalf("expected join state for writer-agent, got %+v", task.Status.JoinStates)
	}
	if writerJoin.Expected != 2 || writerJoin.QuorumRequired != 2 {
		t.Fatalf("expected writer join expected=2 required=2, got %+v", *writerJoin)
	}
	if !writerJoin.Activated {
		t.Fatalf("expected writer join activated, got %+v", *writerJoin)
	}
	if len(writerJoin.Sources) != 2 {
		t.Fatalf("expected writer join to record 2 sources, got %+v", *writerJoin)
	}

	if len(task.Status.MessageIdempotency) < 3 {
		t.Fatalf("expected idempotency records to be persisted, got %d", len(task.Status.MessageIdempotency))
	}
}

func TestAgentMessageConsumerJoinWaitPersistsIdempotencyAndSkipsDuplicate(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()

	if _, err := agentStore.Upsert(crds.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   crds.ObjectMeta{Name: "writer-agent"},
		Spec:       crds.AgentSpec{Model: "gpt-4o", Prompt: "write", Limits: crds.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
	}); err != nil {
		t.Fatalf("upsert writer failed: %v", err)
	}
	if _, err := systemStore.Upsert(crds.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   crds.ObjectMeta{Name: "join-system"},
		Spec: crds.AgentSystemSpec{
			Agents: []string{"researcher-agent", "reviewer-agent", "writer-agent"},
			Graph: map[string]crds.GraphEdge{
				"researcher-agent": {Edges: []crds.GraphRoute{{To: "writer-agent"}}},
				"reviewer-agent":   {Edges: []crds.GraphRoute{{To: "writer-agent"}}},
				"writer-agent":     {Join: crds.GraphJoin{Mode: "wait_for_all"}},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := taskStore.Upsert(crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "join-task"},
		Spec:       crds.TaskSpec{System: "join-system"},
		Status: crds.TaskStatus{
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
		AgentMessageConsumerOptions{WorkerID: "worker-a", RefreshEvery: 20 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Start(ctx)
	time.Sleep(40 * time.Millisecond)

	first := AgentMessage{
		MessageID:      "msg-join-1",
		IdempotencyKey: "msg-join-1",
		TaskID:         "default/join-task",
		Namespace:      "default",
		FromAgent:      "researcher-agent",
		ToAgent:        "writer-agent",
		BranchID:       "b001.001",
		Type:           "task_handoff",
		Payload:        "research payload",
		Attempt:        1,
	}
	if _, err := bus.Publish(context.Background(), first); err != nil {
		t.Fatalf("publish first join msg failed: %v", err)
	}
	waitForConsumer(t, 2*time.Second, func() bool {
		task, ok := taskStore.Get("join-task")
		if !ok {
			return false
		}
		msg, ok := taskMessageByID(task.Status.Messages, "msg-join-1")
		return ok && strings.EqualFold(msg.Phase, "succeeded")
	})

	// Replay the same message; persistent idempotency should skip duplicate execution/attempt.
	if _, err := bus.Publish(context.Background(), first); err != nil {
		t.Fatalf("publish duplicate join msg failed: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	task, _ := taskStore.Get("join-task")
	msg, ok := taskMessageByID(task.Status.Messages, "msg-join-1")
	if !ok {
		t.Fatal("expected join message record")
	}
	if msg.Attempts != 1 {
		t.Fatalf("expected duplicate replay to keep attempts=1, got %d", msg.Attempts)
	}
	if countTraceByTypeAndMessage(task.Status.Trace, "agent_message_processed", "msg-join-1") != 1 {
		t.Fatalf("expected one processed trace for msg-join-1, got %d", countTraceByTypeAndMessage(task.Status.Trace, "agent_message_processed", "msg-join-1"))
	}
	foundIdempotency := false
	for _, record := range task.Status.MessageIdempotency {
		if strings.EqualFold(strings.TrimSpace(record.Key), "msg-join-1") && strings.EqualFold(record.State, "completed") {
			foundIdempotency = true
			break
		}
	}
	if !foundIdempotency {
		t.Fatalf("expected completed idempotency record for msg-join-1, got %+v", task.Status.MessageIdempotency)
	}
}

func TestAgentMessageConsumerStopsCyclicBranchAtTaskMaxTurns(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()

	for _, agent := range []crds.Agent{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   crds.ObjectMeta{Name: "manager-agent"},
			Spec:       crds.AgentSpec{Model: "gpt-4o", Prompt: "manage", Limits: crds.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   crds.ObjectMeta{Name: "research-agent"},
			Spec:       crds.AgentSpec{Model: "gpt-4o", Prompt: "research", Limits: crds.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
		},
	} {
		if _, err := agentStore.Upsert(agent); err != nil {
			t.Fatalf("upsert agent failed: %v", err)
		}
	}

	if _, err := systemStore.Upsert(crds.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   crds.ObjectMeta{Name: "cycle-system"},
		Spec: crds.AgentSystemSpec{
			Agents: []string{"manager-agent", "research-agent"},
			Graph: map[string]crds.GraphEdge{
				"manager-agent":  {Next: "research-agent"},
				"research-agent": {Next: "manager-agent"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}

	if _, err := taskStore.Upsert(crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "cycle-task"},
		Spec: crds.TaskSpec{
			System:   "cycle-system",
			MaxTurns: 3,
		},
		Status: crds.TaskStatus{
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
			WorkerID:            "worker-a",
			RefreshEvery:        20 * time.Millisecond,
			DedupeWindow:        time.Minute,
			LeaseExtendDuration: 30 * time.Second,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Start(ctx)
	time.Sleep(40 * time.Millisecond)

	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "cycle-msg-1",
		TaskID:    "default/cycle-task",
		Namespace: "default",
		FromAgent: "system",
		ToAgent:   "manager-agent",
		Type:      "task_start",
		Payload:   "start",
		Attempt:   1,
		BranchID:  "b001",
		TraceID:   "default/cycle-task/a001",
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	waitForConsumer(t, 3*time.Second, func() bool {
		task, ok := taskStore.Get("cycle-task")
		return ok && strings.EqualFold(task.Status.Phase, "succeeded")
	})

	task, _ := taskStore.Get("cycle-task")
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected task succeeded, got %q", task.Status.Phase)
	}

	branchCount := 0
	for _, message := range task.Status.Messages {
		if strings.EqualFold(strings.TrimSpace(message.BranchID), "b001") {
			branchCount++
		}
	}
	if branchCount != 3 {
		t.Fatalf("expected exactly 3 branch messages due max_turns=3, got %d messages=%+v", branchCount, task.Status.Messages)
	}
}

func waitForConsumer(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func countMessages(messages []crds.TaskMessage, messageID string) int {
	count := 0
	for _, msg := range messages {
		if strings.EqualFold(strings.TrimSpace(msg.MessageID), strings.TrimSpace(messageID)) {
			count++
		}
	}
	return count
}

func countTraceByTypeAndMessage(trace []crds.TaskTraceEvent, eventType, messageID string) int {
	count := 0
	needle := "message_id=" + strings.TrimSpace(messageID)
	for _, event := range trace {
		if !strings.EqualFold(strings.TrimSpace(event.Type), strings.TrimSpace(eventType)) {
			continue
		}
		if strings.Contains(event.Message, needle) {
			count++
		}
	}
	return count
}

func countTraceByAgentAndType(trace []crds.TaskTraceEvent, agent, eventType string) int {
	count := 0
	for _, event := range trace {
		if !strings.EqualFold(strings.TrimSpace(event.Agent), strings.TrimSpace(agent)) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(event.Type), strings.TrimSpace(eventType)) {
			continue
		}
		count++
	}
	return count
}

func taskMessageByID(messages []crds.TaskMessage, messageID string) (crds.TaskMessage, bool) {
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.MessageID), strings.TrimSpace(messageID)) {
			return message, true
		}
	}
	return crds.TaskMessage{}, false
}
