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
		if _, err := agentStore.Upsert(context.Background(), agent); err != nil {
			t.Fatalf("upsert agent failed: %v", err)
		}
	}

	if _, err := systemStore.Upsert(context.Background(), resources.AgentSystem{
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

	if _, err := taskStore.Upsert(context.Background(), resources.Task{
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
	waitForConsumerSubscriptions(t, manager, bus, 2*time.Second)

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
		task, ok, _ := taskStore.Get(context.Background(), "task-1")
		return ok && strings.EqualFold(task.Status.Phase, "succeeded")
	})

	task, _, _ := taskStore.Get(context.Background(), "task-1")
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

func TestAgentMessageConsumerExecutesMcpToolRuntime(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()
	toolStore := store.NewToolStore()
	mcpServerStore := store.NewMcpServerStore()

	if _, err := agentStore.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "mcp-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "use the MCP tool",
			Tools:    []string{"test-mcp--search"},
			Limits:   resources.AgentLimits{MaxSteps: 2, Timeout: "2s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := systemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "mcp-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"mcp-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := toolStore.Upsert(context.Background(), resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "test-mcp--search", Namespace: "default"},
		Spec: resources.ToolSpec{
			Type:         "mcp",
			McpServerRef: "test-mcp",
			McpToolName:  "search",
		},
	}); err != nil {
		t.Fatalf("upsert tool failed: %v", err)
	}
	server, err := mcpServerStore.Upsert(context.Background(), resources.McpServer{
		APIVersion: "orloj.dev/v1",
		Kind:       "McpServer",
		Metadata:   resources.ObjectMeta{Name: "test-mcp", Namespace: "default"},
		Spec:       resources.McpServerSpec{Transport: "stdio", Command: "echo"},
	})
	if err != nil {
		t.Fatalf("upsert mcp server failed: %v", err)
	}

	mcpTransport := &mockMcpTransport{
		tools: []McpToolDefinition{{Name: "search"}},
		callResults: map[string]*McpToolResult{
			"search": {Content: []McpContent{{Type: "text", Text: "mcp search result"}}},
		},
	}
	sessionMgr := NewMcpSessionManager(nil)
	defer sessionMgr.Close()
	sessionMgr.sessions["default/test-mcp"] = &McpSession{
		Transport:  mcpTransport,
		InitResult: &McpInitResult{},
		ServerName: "test-mcp",
		generation: server.Metadata.Generation,
		lastUsedAt: time.Now(),
	}

	if _, err := taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "mcp-task"},
		Spec: resources.TaskSpec{
			System: "mcp-system",
			Input:  map[string]string{"topic": "search"},
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
		AgentMessageConsumerOptions{
			ModelEndpoints:      newTestModelEndpointStore(t),
			Tools:               toolStore,
			McpSessionManager:   sessionMgr,
			McpServerStore:      mcpServerStore,
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
		MessageID: "msg-mcp",
		TaskID:    "default/mcp-task",
		Namespace: "default",
		FromAgent: "system",
		ToAgent:   "mcp-agent",
		Type:      "task_start",
		Payload:   "search",
		Attempt:   1,
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	waitForConsumer(t, 3*time.Second, func() bool {
		task, ok, _ := taskStore.Get(context.Background(), "mcp-task")
		return ok && strings.EqualFold(task.Status.Phase, "succeeded")
	})

	task, _, _ := taskStore.Get(context.Background(), "mcp-task")
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected task succeeded, got %q error=%s", task.Status.Phase, task.Status.LastError)
	}
	if task.Status.Output["last_tool_calls"] != "1" {
		t.Fatalf("expected one MCP tool call, got output=%+v", task.Status.Output)
	}
}

func TestAgentMessageConsumerWaitsForLeaseThenTakesOver(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()

	if _, err := agentStore.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "research-agent"},
		Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "research"},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := systemStore.Upsert(context.Background(), resources.AgentSystem{
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
	waitForConsumerSubscriptions(t, manager, bus, 2*time.Second)

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

	task, _, _ := taskStore.Get(context.Background(), "task-2")
	if countMessages(task.Status.Messages, "msg-skip") != 0 {
		t.Fatalf("expected no message persisted for non-owner worker, got %d", countMessages(task.Status.Messages, "msg-skip"))
	}
	if countTraceByTypeAndMessage(task.Status.Trace, "agent_message_processed", "msg-skip") != 0 {
		t.Fatalf("expected no processed trace for non-owner worker, got %d", countTraceByTypeAndMessage(task.Status.Trace, "agent_message_processed", "msg-skip"))
	}

	waitForConsumer(t, 2*time.Second, func() bool {
		current, ok, err := taskStore.Get(context.Background(), "task-2")
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			return false
		}
		return strings.EqualFold(current.Status.Phase, "succeeded") && countTraceByTypeAndMessage(current.Status.Trace, "agent_message_processed", "msg-skip") == 1
	})

	task, _, _ = taskStore.Get(context.Background(), "task-2")
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

	if _, err := agentStore.Upsert(context.Background(), resources.Agent{
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
	if _, err := systemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "retry-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"planner-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := taskStore.Upsert(context.Background(), resources.Task{
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
	waitForConsumerSubscriptions(t, manager, bus, 2*time.Second)

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
		task, ok, _ := taskStore.Get(context.Background(), "retry-task")
		if ok && strings.EqualFold(task.Status.Phase, "deadletter") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	task, _, _ := taskStore.Get(context.Background(), "retry-task")
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

	if _, err := agentStore.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "runner-agent"},
		Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "run"},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := taskStore.Upsert(context.Background(), resources.Task{
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
	waitForConsumerSubscriptions(t, manager, bus, 2*time.Second)

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
		task, ok, _ := taskStore.Get(context.Background(), "invalid-ref-task")
		return ok && strings.EqualFold(task.Status.Phase, "deadletter")
	})

	task, _, _ := taskStore.Get(context.Background(), "invalid-ref-task")
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

	if _, err := agentStore.Upsert(context.Background(), resources.Agent{
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
	if _, err := systemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "contract-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"contract-agent"},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := taskStore.Upsert(context.Background(), resources.Task{
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
	waitForConsumerSubscriptions(t, manager, bus, 2*time.Second)

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
		task, ok, _ := taskStore.Get(context.Background(), "contract-task")
		return ok && strings.EqualFold(task.Status.Phase, "deadletter")
	})

	task, _, _ := taskStore.Get(context.Background(), "contract-task")
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
		if _, err := agentStore.Upsert(context.Background(), agent); err != nil {
			t.Fatalf("upsert agent failed: %v", err)
		}
	}

	if _, err := systemStore.Upsert(context.Background(), resources.AgentSystem{
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

	if _, err := taskStore.Upsert(context.Background(), resources.Task{
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
	waitForConsumerSubscriptions(t, manager, bus, 2*time.Second)

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

	waitForConsumer(t, 8*time.Second, func() bool {
		task, ok, _ := taskStore.Get(context.Background(), "fanout-task")
		return ok && strings.EqualFold(task.Status.Phase, "succeeded")
	})

	task, _, _ := taskStore.Get(context.Background(), "fanout-task")
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

	if _, err := agentStore.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "writer-agent"},
		Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "write", Limits: resources.AgentLimits{MaxSteps: 1, Timeout: "1s"}},
	}); err != nil {
		t.Fatalf("upsert writer failed: %v", err)
	}
	if _, err := systemStore.Upsert(context.Background(), resources.AgentSystem{
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
	if _, err := taskStore.Upsert(context.Background(), resources.Task{
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
	waitForConsumerSubscriptions(t, manager, bus, 2*time.Second)

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
		task, ok, err := taskStore.Get(context.Background(), "join-task")
		if err != nil {
			t.Fatal(err)
		}
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

	task, _, _ := taskStore.Get(context.Background(), "join-task")
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
		if _, err := agentStore.Upsert(context.Background(), agent); err != nil {
			t.Fatalf("upsert agent failed: %v", err)
		}
	}

	if _, err := systemStore.Upsert(context.Background(), resources.AgentSystem{
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

	if _, err := taskStore.Upsert(context.Background(), resources.Task{
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
	waitForConsumerSubscriptions(t, manager, bus, 2*time.Second)

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
		task, ok, _ := taskStore.Get(context.Background(), "cycle-task")
		return ok && strings.EqualFold(task.Status.Phase, "succeeded")
	})

	task, _, _ := taskStore.Get(context.Background(), "cycle-task")
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

func TestAgentMessageConsumerEnforcesPolicyBlockedModel(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()
	policyStore := store.NewAgentPolicyStore()

	if _, err := agentStore.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "policy-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "do work",
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := systemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "policy-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"policy-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := policyStore.Upsert(context.Background(), resources.AgentPolicy{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentPolicy",
		Metadata:   resources.ObjectMeta{Name: "restrict-models"},
		Spec: resources.AgentPolicySpec{
			ApplyMode:     "scoped",
			TargetSystems: []string{"policy-system"},
			AllowedModels: []string{"claude-3"},
		},
	}); err != nil {
		t.Fatalf("upsert policy failed: %v", err)
	}
	if _, err := taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "policy-task"},
		Spec: resources.TaskSpec{
			System: "policy-system",
			Input:  map[string]string{"topic": "testing"},
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
		AgentMessageConsumerOptions{
			ModelEndpoints: newTestModelEndpointStore(t),
			WorkerID:       "worker-a",
			RefreshEvery:   20 * time.Millisecond,
			DedupeWindow:   time.Minute,
			Policies:       policyStore,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Start(ctx)
	waitForConsumerSubscriptions(t, manager, bus, 2*time.Second)

	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "msg-policy-block",
		TaskID:    "default/policy-task",
		Namespace: "default",
		FromAgent: "system",
		ToAgent:   "policy-agent",
		Type:      "task_start",
		Payload:   "start",
		Attempt:   1,
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	waitForConsumer(t, 2*time.Second, func() bool {
		task, ok, _ := taskStore.Get(context.Background(), "policy-task")
		return ok && strings.EqualFold(task.Status.Phase, "deadletter")
	})

	task, _, _ := taskStore.Get(context.Background(), "policy-task")
	if task.Status.Phase != "DeadLetter" {
		t.Fatalf("expected DeadLetter for policy violation, got %q", task.Status.Phase)
	}
	if !strings.Contains(strings.ToLower(task.Status.LastError), "disallows model") {
		t.Fatalf("expected policy violation in lastError, got %q", task.Status.LastError)
	}
}

func TestAgentMessageConsumerEnforcesPolicyBlockedTool(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()
	policyStore := store.NewAgentPolicyStore()

	if _, err := agentStore.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "tool-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "do work",
			Tools:    []string{"web-search", "dangerous-tool"},
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := systemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "tool-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"tool-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := policyStore.Upsert(context.Background(), resources.AgentPolicy{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentPolicy",
		Metadata:   resources.ObjectMeta{Name: "block-tools"},
		Spec: resources.AgentPolicySpec{
			ApplyMode:    "global",
			BlockedTools: []string{"dangerous-tool"},
		},
	}); err != nil {
		t.Fatalf("upsert policy failed: %v", err)
	}
	if _, err := taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "tool-task"},
		Spec: resources.TaskSpec{
			System: "tool-system",
			Input:  map[string]string{"topic": "testing"},
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
		AgentMessageConsumerOptions{
			ModelEndpoints: newTestModelEndpointStore(t),
			WorkerID:       "worker-a",
			RefreshEvery:   20 * time.Millisecond,
			DedupeWindow:   time.Minute,
			Policies:       policyStore,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Start(ctx)
	waitForConsumerSubscriptions(t, manager, bus, 2*time.Second)

	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "msg-tool-block",
		TaskID:    "default/tool-task",
		Namespace: "default",
		FromAgent: "system",
		ToAgent:   "tool-agent",
		Type:      "task_start",
		Payload:   "start",
		Attempt:   1,
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	waitForConsumer(t, 2*time.Second, func() bool {
		task, ok, _ := taskStore.Get(context.Background(), "tool-task")
		return ok && strings.EqualFold(task.Status.Phase, "deadletter")
	})

	task, _, _ := taskStore.Get(context.Background(), "tool-task")
	if task.Status.Phase != "DeadLetter" {
		t.Fatalf("expected DeadLetter for blocked tool, got %q", task.Status.Phase)
	}
	if !strings.Contains(strings.ToLower(task.Status.LastError), "blocks tool") {
		t.Fatalf("expected blocked tool in lastError, got %q", task.Status.LastError)
	}
}

func TestAgentMessageConsumerPassesWithNoPolicies(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()
	policyStore := store.NewAgentPolicyStore()

	if _, err := agentStore.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "free-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "do work",
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := systemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "free-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"free-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "free-task"},
		Spec: resources.TaskSpec{
			System: "free-system",
			Input:  map[string]string{"topic": "testing"},
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
		AgentMessageConsumerOptions{
			ModelEndpoints: newTestModelEndpointStore(t),
			WorkerID:       "worker-a",
			RefreshEvery:   20 * time.Millisecond,
			DedupeWindow:   time.Minute,
			Policies:       policyStore,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Start(ctx)
	waitForConsumerSubscriptions(t, manager, bus, 2*time.Second)

	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "msg-free",
		TaskID:    "default/free-task",
		Namespace: "default",
		FromAgent: "system",
		ToAgent:   "free-agent",
		Type:      "task_start",
		Payload:   "start",
		Attempt:   1,
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	waitForConsumer(t, 2*time.Second, func() bool {
		task, ok, _ := taskStore.Get(context.Background(), "free-task")
		return ok && strings.EqualFold(task.Status.Phase, "succeeded")
	})

	task, _, _ := taskStore.Get(context.Background(), "free-task")
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected Succeeded with no policies, got %q (error: %s)", task.Status.Phase, task.Status.LastError)
	}
}

func TestConditionalEdgeRoutingSkipsUnmatchedAgents(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 256, time.Minute)
	defer func() { _ = bus.Close() }()

	agentStore := store.NewAgentStore()
	systemStore := store.NewAgentSystemStore()
	taskStore := store.NewTaskStore()

	for _, agent := range []resources.Agent{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "classifier"},
			Spec: resources.AgentSpec{
				ModelRef: "openai-default",
				Prompt:   "classify",
				Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "billing-agent"},
			Spec: resources.AgentSpec{
				ModelRef: "openai-default",
				Prompt:   "handle billing",
				Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Agent",
			Metadata:   resources.ObjectMeta{Name: "tech-agent"},
			Spec: resources.AgentSpec{
				ModelRef: "openai-default",
				Prompt:   "handle tech",
				Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
			},
		},
	} {
		if _, err := agentStore.Upsert(context.Background(), agent); err != nil {
			t.Fatalf("upsert agent failed: %v", err)
		}
	}

	if _, err := systemStore.Upsert(context.Background(), resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "triage-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"classifier", "billing-agent", "tech-agent"},
			Graph: map[string]resources.GraphEdge{
				"classifier": {Edges: []resources.GraphRoute{
					{To: "billing-agent", Condition: &resources.EdgeCondition{OutputContains: "BILLING"}},
					{To: "tech-agent", Condition: &resources.EdgeCondition{OutputContains: "TECH"}},
				}},
			},
		},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}

	if _, err := taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "triage-task-1"},
		Spec: resources.TaskSpec{
			System: "triage-system",
			Input:  map[string]string{"topic": "my bill is wrong"},
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
		AgentMessageConsumerOptions{
			ModelEndpoints:      newTestModelEndpointStore(t),
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
		MessageID: "cond-msg-1",
		TaskID:    "default/triage-task-1",
		Namespace: "default",
		FromAgent: "system",
		ToAgent:   "classifier",
		Type:      "task_start",
		Payload:   "my bill is wrong",
		Attempt:   1,
		TraceID:   "default/triage-task-1/a001",
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	waitForConsumer(t, 3*time.Second, func() bool {
		task, ok, _ := taskStore.Get(context.Background(), "triage-task-1")
		return ok && strings.EqualFold(task.Status.Phase, "succeeded")
	})

	task, _, _ := taskStore.Get(context.Background(), "triage-task-1")
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected Succeeded, got %q (error: %s)", task.Status.Phase, task.Status.LastError)
	}

	hasBilling := false
	hasTech := false
	for _, msg := range task.Status.Messages {
		if strings.EqualFold(msg.ToAgent, "billing-agent") {
			hasBilling = true
		}
		if strings.EqualFold(msg.ToAgent, "tech-agent") {
			hasTech = true
		}
	}

	// The mock model output won't contain "BILLING" or "TECH", so the
	// classifier's output should not match either condition. Both agents
	// should be skipped and the task should complete at the classifier.
	if hasBilling || hasTech {
		t.Fatalf("expected conditional routing to skip both agents (mock output has no BILLING/TECH), billing=%v tech=%v", hasBilling, hasTech)
	}
}

func TestNextAgentsFromSystemForOutputFiltersEdges(t *testing.T) {
	system := resources.AgentSystem{
		Spec: resources.AgentSystemSpec{
			Agents: []string{"router", "a", "b", "c"},
			Graph: map[string]resources.GraphEdge{
				"router": {Edges: []resources.GraphRoute{
					{To: "a", Condition: &resources.EdgeCondition{OutputContains: "ALPHA"}},
					{To: "b", Condition: &resources.EdgeCondition{OutputContains: "BETA"}},
					{To: "c", Condition: &resources.EdgeCondition{Default: true}},
				}},
			},
		},
	}

	got := nextAgentsFromSystemForOutput(system, "router", "This has ALPHA tag", "")
	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("expected [a], got %v", got)
	}

	got = nextAgentsFromSystemForOutput(system, "router", "Both ALPHA and BETA", "")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("expected [a, b], got %v", got)
	}

	got = nextAgentsFromSystemForOutput(system, "router", "nothing matches", "")
	if len(got) != 1 || got[0] != "c" {
		t.Fatalf("expected [c] (default), got %v", got)
	}
}

func TestNextAgentsFromSystemBackwardCompatNoConditions(t *testing.T) {
	system := resources.AgentSystem{
		Spec: resources.AgentSystemSpec{
			Agents: []string{"a", "b", "c"},
			Graph: map[string]resources.GraphEdge{
				"a": {Edges: []resources.GraphRoute{
					{To: "b"},
					{To: "c"},
				}},
			},
		},
	}

	got := nextAgentsFromSystemForOutput(system, "a", "any output", "")
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Fatalf("expected [b, c] (no conditions = all fire), got %v", got)
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

func waitForConsumerSubscriptions(t *testing.T, manager *AgentMessageConsumerManager, bus *MemoryAgentMessageBus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		manager.mu.Lock()
		consumerCount := len(manager.consumers)
		manager.mu.Unlock()

		bus.mu.Lock()
		subscriberCount := len(bus.subs)
		bus.mu.Unlock()

		if consumerCount > 0 && subscriberCount == consumerCount {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	manager.mu.Lock()
	consumerCount := len(manager.consumers)
	manager.mu.Unlock()
	bus.mu.Lock()
	subscriberCount := len(bus.subs)
	bus.mu.Unlock()
	t.Fatalf("consumer subscriptions not ready before timeout: consumers=%d subscribers=%d", consumerCount, subscriberCount)
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

func TestNextAgentsFromSystemDelegateOfReturn(t *testing.T) {
	system := resources.AgentSystem{
		Spec: resources.AgentSystemSpec{
			Graph: map[string]resources.GraphEdge{
				"worker": {},
			},
		},
	}

	got := nextAgentsFromSystemForOutput(system, "worker", "some output", "manager")
	if len(got) != 1 || got[0] != "manager" {
		t.Fatalf("expected [manager] from delegate_of return, got %v", got)
	}

	got = nextAgentsFromSystemForOutput(system, "worker", "some output", "")
	if len(got) != 0 {
		t.Fatalf("expected empty when no delegate_of, got %v", got)
	}
}

func TestNextAgentsFromSystemEdgesTakePriorityOverDelegateOf(t *testing.T) {
	system := resources.AgentSystem{
		Spec: resources.AgentSystemSpec{
			Graph: map[string]resources.GraphEdge{
				"worker": {
					Edges: []resources.GraphRoute{
						{To: "next-agent"},
					},
				},
			},
		},
	}

	got := nextAgentsFromSystemForOutput(system, "worker", "output", "manager")
	if len(got) != 1 || got[0] != "next-agent" {
		t.Fatalf("edges should take priority over delegate_of, got %v", got)
	}
}

func TestBuildDelegateMessages(t *testing.T) {
	task := resources.Task{
		Metadata: resources.ObjectMeta{
			Name:      "test-task",
			Namespace: "default",
		},
		Status: resources.TaskStatus{
			Attempts: 1,
		},
	}
	current := AgentMessage{
		MessageID: "default/test-task/a001/h001/init/manager",
		TaskID:    "default/test-task",
		Attempt:   1,
		System:    "my-system",
		Namespace: "default",
		FromAgent: "init",
		ToAgent:   "manager",
		BranchID:  "b001",
		TraceID:   "trace-1",
	}
	result := AgentExecutionResult{
		Agent:  "manager",
		Output: "delegate to workers",
	}

	msgs := buildDelegateMessages(task, current, result, []string{"worker-a", "worker-b"}, "manager")
	if len(msgs) != 2 {
		t.Fatalf("expected 2 delegate messages, got %d", len(msgs))
	}

	if msgs[0].ToAgent != "worker-a" {
		t.Fatalf("expected first message to worker-a, got %q", msgs[0].ToAgent)
	}
	if msgs[0].DelegateOf != "manager" {
		t.Fatalf("expected delegate_of=manager, got %q", msgs[0].DelegateOf)
	}
	if msgs[0].Type != "delegation" {
		t.Fatalf("expected type=delegation, got %q", msgs[0].Type)
	}
	if !strings.Contains(msgs[0].BranchID, ".d001") {
		t.Fatalf("expected delegation branch suffix .d001, got %q", msgs[0].BranchID)
	}

	if msgs[1].ToAgent != "worker-b" {
		t.Fatalf("expected second message to worker-b, got %q", msgs[1].ToAgent)
	}
	if msgs[1].DelegateOf != "manager" {
		t.Fatalf("expected delegate_of=manager on second message, got %q", msgs[1].DelegateOf)
	}
	if !strings.Contains(msgs[1].BranchID, ".d002") {
		t.Fatalf("expected delegation branch suffix .d002, got %q", msgs[1].BranchID)
	}
}

func TestBuildNextAgentMessagesPropagatesDelegateOf(t *testing.T) {
	task := resources.Task{
		Metadata: resources.ObjectMeta{
			Name:      "test-task",
			Namespace: "default",
		},
		Status: resources.TaskStatus{
			Attempts: 1,
		},
	}
	current := AgentMessage{
		MessageID:  "default/test-task/a001/h002/manager/worker-a",
		TaskID:     "default/test-task",
		Attempt:    1,
		System:     "my-system",
		Namespace:  "default",
		FromAgent:  "manager",
		ToAgent:    "worker-a",
		BranchID:   "b001.d001",
		DelegateOf: "manager",
		TraceID:    "trace-1",
	}
	result := AgentExecutionResult{
		Agent:  "worker-a",
		Output: "sub-step output",
	}

	msgs := buildNextAgentMessages(task, current, result, []string{"sub-worker"})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].DelegateOf != "manager" {
		t.Fatalf("expected delegate_of=manager to propagate, got %q", msgs[0].DelegateOf)
	}
}

func TestEnsureTaskDelegationState(t *testing.T) {
	task := resources.Task{}
	_ = task.Normalize()

	idx := ensureTaskDelegationState(&task, 1, "manager", "wait_for_all", 2, 2)
	if idx != 0 {
		t.Fatalf("expected index 0, got %d", idx)
	}
	if len(task.Status.DelegationStates) != 1 {
		t.Fatalf("expected 1 delegation state, got %d", len(task.Status.DelegationStates))
	}
	state := task.Status.DelegationStates[0]
	if state.Node != "manager" || state.Mode != "wait_for_all" || state.Expected != 2 || state.QuorumRequired != 2 {
		t.Fatalf("unexpected state: %+v", state)
	}

	idx2 := ensureTaskDelegationState(&task, 1, "manager", "wait_for_all", 2, 2)
	if idx2 != 0 {
		t.Fatalf("expected same index 0 for existing state, got %d", idx2)
	}
	if len(task.Status.DelegationStates) != 1 {
		t.Fatalf("expected still 1 delegation state, got %d", len(task.Status.DelegationStates))
	}

	idx3 := ensureTaskDelegationState(&task, 1, "other-node", "quorum", 3, 2)
	if idx3 != 1 {
		t.Fatalf("expected index 1 for different node, got %d", idx3)
	}
	if len(task.Status.DelegationStates) != 2 {
		t.Fatalf("expected 2 delegation states, got %d", len(task.Status.DelegationStates))
	}
}

func TestAppendDelegationSource(t *testing.T) {
	state := resources.TaskDelegationState{
		Node:     "manager",
		Expected: 2,
		Sources:  make([]resources.TaskJoinSource, 0),
	}

	source1 := resources.TaskJoinSource{MessageID: "msg-1", FromAgent: "worker-a", Payload: "result-a"}
	state = appendDelegationSource(state, source1)
	if len(state.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(state.Sources))
	}

	source2 := resources.TaskJoinSource{MessageID: "msg-2", FromAgent: "worker-b", Payload: "result-b"}
	state = appendDelegationSource(state, source2)
	if len(state.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(state.Sources))
	}

	source1Updated := resources.TaskJoinSource{MessageID: "msg-1", FromAgent: "worker-a", Payload: "updated-result-a"}
	state = appendDelegationSource(state, source1Updated)
	if len(state.Sources) != 2 {
		t.Fatalf("expected still 2 sources after update, got %d", len(state.Sources))
	}
	if state.Sources[0].Payload != "updated-result-a" {
		t.Fatalf("expected updated payload, got %q", state.Sources[0].Payload)
	}
}

func TestCountDispatchedDelegates(t *testing.T) {
	delegates := []resources.GraphRoute{
		{To: "worker-a"},
		{To: "worker-b"},
		{To: "worker-c"},
	}
	messages := []resources.TaskMessage{
		{ToAgent: "worker-a", DelegateOf: "manager"},
		{ToAgent: "worker-b", DelegateOf: "manager"},
		{ToAgent: "worker-c", DelegateOf: "other-node"},
	}

	count := countDispatchedDelegates(messages, delegates, "manager")
	if count != 2 {
		t.Fatalf("expected 2 dispatched delegates for manager, got %d", count)
	}
}

func TestDelegationGateDecisionHelpers(t *testing.T) {
	d := delegationGateDecision{
		DelegationMode: "wait_for_all",
		Required:       2,
		Sources: []resources.TaskJoinSource{
			{FromAgent: "worker-a", Payload: "result-a"},
			{FromAgent: "worker-b", Payload: "result-b"},
		},
	}

	agents := d.SourceAgents()
	if agents != "worker-a,worker-b" {
		t.Fatalf("expected 'worker-a,worker-b', got %q", agents)
	}

	payloads := d.SourcePayloads()
	if !strings.Contains(payloads, "worker-a:result-a") || !strings.Contains(payloads, "worker-b:result-b") {
		t.Fatalf("unexpected payloads: %q", payloads)
	}
}
