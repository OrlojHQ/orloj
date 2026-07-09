package agentruntime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

type countingLeaseDelivery struct {
	message     AgentMessage
	extendCalls atomic.Int64
}

func (d *countingLeaseDelivery) Message() AgentMessage                       { return d.message }
func (d *countingLeaseDelivery) Ack(context.Context) error                   { return nil }
func (d *countingLeaseDelivery) Nack(context.Context, bool) error            { return nil }
func (d *countingLeaseDelivery) NackWithDelay(context.Context, time.Duration) error {
	return nil
}
func (d *countingLeaseDelivery) ExtendLease(context.Context, time.Duration) error {
	d.extendCalls.Add(1)
	return nil
}

func TestRenewTaskMessageLeaseExtendsOwnedLease(t *testing.T) {
	taskStore := store.NewTaskStore()
	if _, err := taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "lease-task", Namespace: "default"},
		Spec:       resources.TaskSpec{System: "sys", Mode: "run"},
		Status: resources.TaskStatus{
			Phase:     "Running",
			ClaimedBy: "worker-a",
		},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	manager := &AgentMessageConsumerManager{
		tasks:       taskStore,
		workerID:    "worker-a",
		leaseExtend: 30 * time.Second,
	}
	if err := manager.renewTaskMessageLease(context.Background(), "default/lease-task"); err != nil {
		t.Fatalf("renew: %v", err)
	}
	got, ok, err := taskStore.Get(context.Background(), "default/lease-task")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got.Status.LastHeartbeat == "" || got.Status.LeaseUntil == "" {
		t.Fatalf("expected heartbeat/leaseUntil to be set, got %+v", got.Status)
	}
	leaseUntil, ok := parseTaskLeaseUntil(got.Status.LeaseUntil)
	if !ok || time.Until(leaseUntil) < 20*time.Second {
		t.Fatalf("expected lease ~30s ahead, got %q", got.Status.LeaseUntil)
	}
}

func TestRenewTaskMessageLeaseLostWhenClaimedByOther(t *testing.T) {
	taskStore := store.NewTaskStore()
	if _, err := taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "lease-task", Namespace: "default"},
		Spec:       resources.TaskSpec{System: "sys", Mode: "run"},
		Status: resources.TaskStatus{
			Phase:     "Running",
			ClaimedBy: "worker-b",
		},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	manager := &AgentMessageConsumerManager{
		tasks:       taskStore,
		workerID:    "worker-a",
		leaseExtend: 30 * time.Second,
	}
	err := manager.renewTaskMessageLease(context.Background(), "default/lease-task")
	if !errors.Is(err, errMessageLeaseLost) {
		t.Fatalf("expected errMessageLeaseLost, got %v", err)
	}
}

func TestRenewTaskMessageLeaseLostWhenTaskMissing(t *testing.T) {
	manager := &AgentMessageConsumerManager{
		tasks:       store.NewTaskStore(),
		workerID:    "worker-a",
		leaseExtend: 30 * time.Second,
	}
	err := manager.renewTaskMessageLease(context.Background(), "default/missing")
	if !errors.Is(err, errMessageLeaseLost) {
		t.Fatalf("expected errMessageLeaseLost, got %v", err)
	}
}

func TestStartMessageLeaseHeartbeatCallsExtendLease(t *testing.T) {
	taskStore := store.NewTaskStore()
	if _, err := taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "hb-task", Namespace: "default"},
		Spec:       resources.TaskSpec{System: "sys", Mode: "run"},
		Status: resources.TaskStatus{
			Phase:     "Running",
			ClaimedBy: "worker-a",
		},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	delivery := &countingLeaseDelivery{message: AgentMessage{MessageID: "m1"}}
	manager := &AgentMessageConsumerManager{
		tasks:       taskStore,
		workerID:    "worker-a",
		leaseExtend: 30 * time.Millisecond,
	}
	ctx, stop := manager.startMessageLeaseHeartbeat(context.Background(), delivery, "default/hb-task")
	defer stop()

	deadline := time.Now().Add(500 * time.Millisecond)
	for delivery.extendCalls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if delivery.extendCalls.Load() < 2 {
		t.Fatalf("expected >=2 ExtendLease calls, got %d", delivery.extendCalls.Load())
	}
	got, ok, err := taskStore.Get(ctx, "default/hb-task")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got.Status.LeaseUntil == "" {
		t.Fatal("expected leaseUntil refreshed by heartbeat")
	}
}

func TestStartMessageLeaseHeartbeatCancelsOnOwnershipLoss(t *testing.T) {
	taskStore := store.NewTaskStore()
	if _, err := taskStore.Upsert(context.Background(), resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "lost-task", Namespace: "default"},
		Spec:       resources.TaskSpec{System: "sys", Mode: "run"},
		Status: resources.TaskStatus{
			Phase:     "Running",
			ClaimedBy: "worker-a",
		},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	delivery := &countingLeaseDelivery{}
	manager := &AgentMessageConsumerManager{
		tasks:       taskStore,
		workerID:    "worker-a",
		leaseExtend: 30 * time.Millisecond,
	}
	ctx, stop := manager.startMessageLeaseHeartbeat(context.Background(), delivery, "default/lost-task")
	defer stop()

	// Steal ownership so the next renew returns errMessageLeaseLost and cancels ctx.
	task, ok, err := taskStore.Get(context.Background(), "default/lost-task")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	task.Status.ClaimedBy = "worker-b"
	if _, err := taskStore.Upsert(context.Background(), task); err != nil {
		t.Fatalf("steal ownership: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected heartbeat to cancel context after ownership loss")
	}
}
