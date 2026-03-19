package agentruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

func newTestModelEndpointStore(t *testing.T) *store.ModelEndpointStore {
	t.Helper()
	modelEPStore := store.NewModelEndpointStore()
	if _, err := modelEPStore.Upsert(resources.ModelEndpoint{
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
	return modelEPStore
}

func TestAgentMessageConsumerExecutesGraphAndCompletesTask(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()

	for _, agent := range []resources.Agent{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "planner-agent"},
			Spec: resources.AgentSpec{
				ModelRef: "openai-default",
				Prompt:   "plan",
				Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "writer-agent"},
			Spec: resources.AgentSpec{
				ModelRef: "openai-default",
				Prompt:   "write",
				Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
			},
		},
	} {
		if _, err := agentStore.Upsert(agent); err != nil {
			t.Fatalf("upsert agent failed: %v", err)
		}
	}

	if _, err := systemStore.Upsert(resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "report-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"planner-agent", "writer-agent"},
			Graph: map[string]resources.GraphEdge{
				"planner-agent": {Next: "writer-agent"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}

	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "task-1"},
		Spec: resources.TaskSpec{
			System: "report-system",
			Input:  map[string]string{"topic": "agent systems"},
		},
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
		AgentMessageConsumerOptions{ModelEndpoints: newTestModelEndpointStore(t),
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

	if _, err := agentStore.Upsert(resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "research-agent"},
		Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "research"},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := systemStore.Upsert(resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "report-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"research-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}

	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "task-2"},
		Spec:       resources.TaskSpec{System: "report-system"},
		Status: resources.TaskStatus{
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
		AgentMessageConsumerOptions{ModelEndpoints: newTestModelEndpointStore(t),
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

	if _, err := agentStore.Upsert(resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "planner-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "plan",
			Limits:   resources.AgentLimits{MaxSteps: 50, Timeout: "1ms"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := systemStore.Upsert(resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "retry-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"planner-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "retry-task"},
		Spec: resources.TaskSpec{
			System: "retry-system",
			Retry: resources.TaskRetryPolicy{
				MaxAttempts: 4,
				Backoff:     "800ms",
			},
			MessageRetry: resources.TaskMessageRetryPolicy{
				MaxAttempts: 2,
				Backoff:     "120ms",
				MaxBackoff:  "250ms",
				Jitter:      "none",
			},
		},
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
		AgentMessageConsumerOptions{ModelEndpoints: newTestModelEndpointStore(t),
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

	if _, err := agentStore.Upsert(resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "runner-agent"},
		Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "run"},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "invalid-ref-task"},
		Spec: resources.TaskSpec{
			System: "missing-system",
			MessageRetry: resources.TaskMessageRetryPolicy{
				MaxAttempts: 5,
				Backoff:     "150ms",
				MaxBackoff:  "2s",
				Jitter:      "none",
			},
		},
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
		AgentMessageConsumerOptions{ModelEndpoints: newTestModelEndpointStore(t),
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

func TestAgentMessageConsumerContractViolationDeadLettersWithoutRetry(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()

	if _, err := agentStore.Upsert(resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "contract-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "contract run",
			Tools:    []string{"tool.alpha", "tool.beta"},
			Execution: resources.AgentExecutionSpec{
				Profile:               resources.AgentExecutionProfileContract,
				ToolSequence:          []string{"tool.alpha", "tool.beta"},
				RequiredOutputMarkers: []string{"DONE"},
			},
			Limits: resources.AgentLimits{MaxSteps: 3},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := systemStore.Upsert(resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "contract-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"contract-agent"},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "contract-task"},
		Spec: resources.TaskSpec{
			System: "contract-system",
			MessageRetry: resources.TaskMessageRetryPolicy{
				MaxAttempts: 4,
				Backoff:     "150ms",
				MaxBackoff:  "2s",
				Jitter:      "none",
			},
		},
		Status: resources.TaskStatus{
			Phase:     "Running",
			ClaimedBy: "worker-a",
			Attempts:  1,
		},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	executor := NewTaskExecutorWithRuntime(nil, &staticToolRuntime{}, &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "out of order",
				ToolCalls: []ModelToolCall{
					{Name: "tool.beta", Input: `{"q":"wrong-order"}`},
				},
			},
		},
	}, nil)

	manager := NewAgentMessageConsumerManager(
		bus,
		agentStore,
		systemStore,
		taskStore,
		nil,
		AgentMessageConsumerOptions{ModelEndpoints: newTestModelEndpointStore(t),
			WorkerID:            "worker-a",
			RefreshEvery:        20 * time.Millisecond,
			DedupeWindow:        time.Minute,
			LeaseExtendDuration: 30 * time.Second,
			Executor:            executor,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Start(ctx)
	time.Sleep(40 * time.Millisecond)

	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "msg-contract-violation",
		TaskID:    "default/contract-task",
		Namespace: "default",
		FromAgent: "system",
		ToAgent:   "contract-agent",
		Type:      "task_start",
		Payload:   "start",
		Attempt:   1,
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	waitForConsumer(t, 2*time.Second, func() bool {
		task, ok := taskStore.Get("contract-task")
		return ok && strings.EqualFold(task.Status.Phase, "deadletter")
	})

	task, _ := taskStore.Get("contract-task")
	if countTraceByTypeAndMessage(task.Status.Trace, "agent_message_retry_scheduled", "msg-contract-violation") != 0 {
		t.Fatalf("expected no retry for contract violation, trace=%+v", task.Status.Trace)
	}
	if countTraceByTypeAndMessage(task.Status.Trace, "agent_message_non_retryable", "msg-contract-violation") == 0 {
		t.Fatal("expected non_retryable trace marker for contract violation")
	}
	sawContractReason := false
	for _, event := range task.Status.Trace {
		if !strings.EqualFold(strings.TrimSpace(event.Type), "agent_message_non_retryable") {
			continue
		}
		if strings.Contains(event.Message, "reason="+ToolReasonAgentContractViolation) {
			sawContractReason = true
			break
		}
	}
	if !sawContractReason {
		payload, _ := json.Marshal(task.Status.Trace)
		t.Fatalf("expected non_retryable reason %q in trace: %s", ToolReasonAgentContractViolation, string(payload))
	}
	message, ok := taskMessageByID(task.Status.Messages, "msg-contract-violation")
	if !ok {
		t.Fatal("expected deadletter message record")
	}
	if message.Attempts != 1 {
		t.Fatalf("expected attempts=1 for non-retryable contract violation, got %d", message.Attempts)
	}
}

func TestComputeMessageRetryDelayCappedAndJitterModes(t *testing.T) {
	msg := AgentMessage{MessageID: "msg-delay", TaskID: "default/delay-task", ToAgent: "writer-agent"}

	none := computeMessageRetryDelay(resources.TaskMessageRetryPolicy{
		Backoff:    "50ms",
		MaxBackoff: "120ms",
		Jitter:     "none",
	}, msg, 3)
	if none != 120*time.Millisecond {
		t.Fatalf("expected capped delay=120ms for attempt=3, got %s", none)
	}

	full := computeMessageRetryDelay(resources.TaskMessageRetryPolicy{
		Backoff:    "100ms",
		MaxBackoff: "5s",
		Jitter:     "full",
	}, msg, 1)
	if full <= 0 || full > 100*time.Millisecond {
		t.Fatalf("expected full jitter delay in (0,100ms], got %s", full)
	}

	equal := computeMessageRetryDelay(resources.TaskMessageRetryPolicy{
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

	for _, agent := range []resources.Agent{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "planner-agent"},
			Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "plan", Limits: resources.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "researcher-agent"},
			Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "research", Limits: resources.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "reviewer-agent"},
			Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "review", Limits: resources.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "writer-agent"},
			Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "write", Limits: resources.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
		},
	} {
		if _, err := agentStore.Upsert(agent); err != nil {
			t.Fatalf("upsert agent failed: %v", err)
		}
	}

	if _, err := systemStore.Upsert(resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "fanout-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"planner-agent", "researcher-agent", "reviewer-agent", "writer-agent"},
			Graph: map[string]resources.GraphEdge{
				"planner-agent": {
					Edges: []resources.GraphRoute{
						{To: "researcher-agent"},
						{To: "reviewer-agent"},
					},
				},
				"researcher-agent": {Edges: []resources.GraphRoute{{To: "writer-agent"}}},
				"reviewer-agent":   {Edges: []resources.GraphRoute{{To: "writer-agent"}}},
				"writer-agent": {
					Join: resources.GraphJoin{Mode: "wait_for_all"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}

	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "fanout-task"},
		Spec:       resources.TaskSpec{System: "fanout-system", Input: map[string]string{"topic": "fanout"}},
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
		AgentMessageConsumerOptions{ModelEndpoints: newTestModelEndpointStore(t),
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
	var writerJoin *resources.TaskJoinState
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

	if _, err := agentStore.Upsert(resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "writer-agent"},
		Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "write", Limits: resources.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
	}); err != nil {
		t.Fatalf("upsert writer failed: %v", err)
	}
	if _, err := systemStore.Upsert(resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "join-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"researcher-agent", "reviewer-agent", "writer-agent"},
			Graph: map[string]resources.GraphEdge{
				"researcher-agent": {Edges: []resources.GraphRoute{{To: "writer-agent"}}},
				"reviewer-agent":   {Edges: []resources.GraphRoute{{To: "writer-agent"}}},
				"writer-agent":     {Join: resources.GraphJoin{Mode: "wait_for_all"}},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "join-task"},
		Spec:       resources.TaskSpec{System: "join-system"},
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
		AgentMessageConsumerOptions{ModelEndpoints: newTestModelEndpointStore(t), WorkerID: "worker-a", RefreshEvery: 20 * time.Millisecond},
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

	for _, agent := range []resources.Agent{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "manager-agent"},
			Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "manage", Limits: resources.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "research-agent"},
			Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "research", Limits: resources.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
		},
	} {
		if _, err := agentStore.Upsert(agent); err != nil {
			t.Fatalf("upsert agent failed: %v", err)
		}
	}

	if _, err := systemStore.Upsert(resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "cycle-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"manager-agent", "research-agent"},
			Graph: map[string]resources.GraphEdge{
				"manager-agent":  {Next: "research-agent"},
				"research-agent": {Next: "manager-agent"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}

	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "cycle-task"},
		Spec: resources.TaskSpec{
			System:   "cycle-system",
			MaxTurns: 3,
		},
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
		AgentMessageConsumerOptions{ModelEndpoints: newTestModelEndpointStore(t),
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

func TestAppendRuntimeStepTraceCarriesModelOutputTokenBreakdown(t *testing.T) {
	task := &resources.Task{}
	events := []AgentStepEvent{
		{
			Timestamp:    "2026-03-18T00:00:00Z",
			Type:         "model_call",
			Step:         1,
			Message:      "step=1 model success",
			Tokens:       120,
			InputTokens:  90,
			OutputTokens: 30,
			UsageSource:  "provider",
		},
		{
			Timestamp: "2026-03-18T00:00:01Z",
			Type:      "model_output",
			Step:      1,
			Message:   "step=1 model_output=hello",
		},
	}

	appendRuntimeStepTrace(task, "writer-agent", events)
	if len(task.Status.Trace) != 2 {
		t.Fatalf("expected 2 trace events, got %d", len(task.Status.Trace))
	}
	modelOutput := task.Status.Trace[1]
	if modelOutput.Type != "model_output" {
		t.Fatalf("expected second trace type model_output, got %q", modelOutput.Type)
	}
	if modelOutput.InputTokens != 90 {
		t.Fatalf("expected model_output input_tokens=90, got %d", modelOutput.InputTokens)
	}
	if modelOutput.OutputTokens != 30 {
		t.Fatalf("expected model_output output_tokens=30, got %d", modelOutput.OutputTokens)
	}
	if modelOutput.Tokens != 30 {
		t.Fatalf("expected model_output tokens to show output token cost=30, got %d", modelOutput.Tokens)
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

func countMessages(messages []resources.TaskMessage, messageID string) int {
	count := 0
	for _, msg := range messages {
		if strings.EqualFold(strings.TrimSpace(msg.MessageID), strings.TrimSpace(messageID)) {
			count++
		}
	}
	return count
}

func countTraceByTypeAndMessage(trace []resources.TaskTraceEvent, eventType, messageID string) int {
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

func countTraceByAgentAndType(trace []resources.TaskTraceEvent, agent, eventType string) int {
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

func taskMessageByID(messages []resources.TaskMessage, messageID string) (resources.TaskMessage, bool) {
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.MessageID), strings.TrimSpace(messageID)) {
			return message, true
		}
	}
	return resources.TaskMessage{}, false
}
