package controllers

import (
	"context"
	"testing"
	"time"
)

func TestKeyQueueEnqueuePopDone(t *testing.T) {
	q := newKeyQueue(4)
	q.Enqueue("task-a")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	key, ok := q.Pop(ctx)
	if !ok || key != "task-a" {
		t.Fatalf("Pop: ok=%v key=%q", ok, key)
	}

	q.Done("task-a")
	q.Enqueue("task-a")
	key2, ok := q.Pop(ctx)
	if !ok || key2 != "task-a" {
		t.Fatalf("second Pop after Done: ok=%v key=%q", ok, key2)
	}
}

func TestKeyQueueEnqueueDedupesWhilePending(t *testing.T) {
	q := newKeyQueue(4)
	q.Enqueue("same")
	q.Enqueue("same")
	q.Enqueue("same")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	key, ok := q.Pop(ctx)
	if !ok || key != "same" {
		t.Fatalf("expected single queued key, got ok=%v key=%q", ok, key)
	}
}

func TestKeyQueueEnqueueIgnoresEmptyKey(t *testing.T) {
	q := newKeyQueue(4)
	q.Enqueue("  ")
	q.Enqueue("")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, ok := q.Pop(ctx)
	if ok {
		t.Fatal("expected no key for empty enqueues")
	}
}

func TestKeyQueuePopRespectsCancelledContext(t *testing.T) {
	q := newKeyQueue(4)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, ok := q.Pop(ctx)
	if ok {
		t.Fatal("expected Pop to fail when context already cancelled")
	}
}

func TestKeyQueueNewKeyQueueDefaultSize(t *testing.T) {
	q := newKeyQueue(0)
	if q == nil || cap(q.ch) != 1024 {
		t.Fatalf("expected default buffer 1024, got cap=%d", cap(q.ch))
	}
}
