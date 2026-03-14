package agentruntime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryAgentMessageBusPublishConsume(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 64, time.Minute)
	defer func() { _ = bus.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	deliveries := make(chan AgentMessage, 1)
	errCh := make(chan error, 1)

	go func() {
		errCh <- bus.Consume(ctx, AgentMessageSubscription{
			Namespace: "default",
			Agent:     "writer-agent",
		}, func(ctx context.Context, delivery AgentMessageDelivery) error {
			deliveries <- delivery.Message()
			return delivery.Ack(ctx)
		})
	}()

	time.Sleep(20 * time.Millisecond)
	published, err := bus.Publish(context.Background(), AgentMessage{
		TaskID:    "default/weekly-report",
		FromAgent: "planner-agent",
		ToAgent:   "writer-agent",
		Payload:   "draft intro",
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if published.Type != "task_handoff" {
		t.Fatalf("expected default type task_handoff, got %q", published.Type)
	}
	if published.MessageID == "" {
		t.Fatal("expected publish to assign message id")
	}

	select {
	case got := <-deliveries:
		if got.ToAgent != "writer-agent" {
			t.Fatalf("expected to_agent writer-agent, got %q", got.ToAgent)
		}
		if got.Payload != "draft intro" {
			t.Fatalf("expected payload draft intro, got %q", got.Payload)
		}
		if got.MessageID != published.MessageID {
			t.Fatalf("expected delivery message id %q, got %q", published.MessageID, got.MessageID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for delivery")
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("consume returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for consume shutdown")
	}
}

func TestMemoryAgentMessageBusDedupeByMessageID(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 64, time.Minute)
	defer func() { _ = bus.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var deliveries atomic.Int32
	done := make(chan struct{}, 2)

	go func() {
		_ = bus.Consume(ctx, AgentMessageSubscription{
			Namespace: "default",
			Agent:     "research-agent",
		}, func(ctx context.Context, delivery AgentMessageDelivery) error {
			deliveries.Add(1)
			done <- struct{}{}
			return delivery.Ack(ctx)
		})
	}()

	time.Sleep(20 * time.Millisecond)
	message := AgentMessage{
		MessageID: "msg-dedupe-1",
		TaskID:    "default/task-1",
		FromAgent: "planner-agent",
		ToAgent:   "research-agent",
		Type:      "task_handoff",
		Payload:   "payload",
	}
	if _, err := bus.Publish(context.Background(), message); err != nil {
		t.Fatalf("first publish failed: %v", err)
	}
	if _, err := bus.Publish(context.Background(), message); err != nil {
		t.Fatalf("second publish failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first delivery")
	}
	time.Sleep(150 * time.Millisecond)

	if got := deliveries.Load(); got != 1 {
		t.Fatalf("expected 1 delivery after dedupe, got %d", got)
	}
}

func TestMemoryAgentMessageBusConsumeReplaysHistory(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 64, time.Minute)
	defer func() { _ = bus.Close() }()

	published, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "msg-history-1",
		TaskID:    "default/task-history",
		FromAgent: "planner-agent",
		ToAgent:   "research-agent",
		Type:      "task_handoff",
		Payload:   "from history",
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	deliveries := make(chan AgentMessage, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- bus.Consume(ctx, AgentMessageSubscription{
			Namespace: "default",
			Agent:     "research-agent",
		}, func(ctx context.Context, delivery AgentMessageDelivery) error {
			deliveries <- delivery.Message()
			return delivery.Ack(ctx)
		})
	}()

	select {
	case got := <-deliveries:
		if got.MessageID != published.MessageID {
			t.Fatalf("expected replayed message id %q, got %q", published.MessageID, got.MessageID)
		}
		if got.Payload != "from history" {
			t.Fatalf("expected replayed payload from history, got %q", got.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for replayed history delivery")
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("consume returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for consume shutdown")
	}
}

func TestMemoryAgentMessageBusNackRequeues(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 64, time.Minute)
	defer func() { _ = bus.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var attempts atomic.Int32
	delivered := make(chan struct{}, 1)

	go func() {
		_ = bus.Consume(ctx, AgentMessageSubscription{
			Namespace: "default",
			Agent:     "research-agent",
		}, func(ctx context.Context, delivery AgentMessageDelivery) error {
			attempt := attempts.Add(1)
			if attempt == 1 {
				return errors.New("transient failure")
			}
			delivered <- struct{}{}
			return delivery.Ack(ctx)
		})
	}()

	time.Sleep(20 * time.Millisecond)
	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "msg-requeue-1",
		TaskID:    "default/task-1",
		FromAgent: "planner-agent",
		ToAgent:   "research-agent",
		Type:      "task_handoff",
		Payload:   "retry me",
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case <-delivered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for requeued delivery")
	}

	if got := attempts.Load(); got < 2 {
		t.Fatalf("expected at least 2 attempts after nack requeue, got %d", got)
	}
}

func TestMemoryAgentMessageBusRetryAfterDelay(t *testing.T) {
	bus := NewMemoryAgentMessageBus("orloj.agentmsg", 64, time.Minute)
	defer func() { _ = bus.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var firstAttemptAt time.Time
	var secondAttemptAt time.Time
	var attempts atomic.Int32
	done := make(chan struct{}, 1)

	go func() {
		_ = bus.Consume(ctx, AgentMessageSubscription{
			Namespace: "default",
			Agent:     "research-agent",
		}, func(ctx context.Context, delivery AgentMessageDelivery) error {
			cur := attempts.Add(1)
			if cur == 1 {
				firstAttemptAt = time.Now()
				return RetryAfter(120*time.Millisecond, errors.New("retry later"))
			}
			secondAttemptAt = time.Now()
			done <- struct{}{}
			return delivery.Ack(ctx)
		})
	}()

	time.Sleep(20 * time.Millisecond)
	if _, err := bus.Publish(context.Background(), AgentMessage{
		MessageID: "msg-delay-1",
		TaskID:    "default/task-delay",
		FromAgent: "planner-agent",
		ToAgent:   "research-agent",
		Type:      "task_handoff",
		Payload:   "delayed",
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for delayed retry delivery")
	}

	if attempts.Load() < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", attempts.Load())
	}
	elapsed := secondAttemptAt.Sub(firstAttemptAt)
	if elapsed < 100*time.Millisecond {
		t.Fatalf("expected delayed retry >=100ms, got %s", elapsed)
	}
}
