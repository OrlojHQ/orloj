package eventbus

import (
	"context"
	"testing"
	"time"
)

func TestMemoryBusPublishSubscribeWithFilter(t *testing.T) {
	bus := NewMemoryBus(64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := bus.Subscribe(ctx, Filter{Source: "apiserver", Kind: "Task", Name: "task-a"})

	bus.Publish(Event{Source: "apiserver", Type: "resource.created", Kind: "Task", Name: "task-b"})
	bus.Publish(Event{Source: "task-controller", Type: "task.succeeded", Kind: "Task", Name: "task-a"})
	want := bus.Publish(Event{Source: "apiserver", Type: "resource.created", Kind: "Task", Name: "task-a"})

	select {
	case got := <-ch:
		if got.ID != want.ID {
			t.Fatalf("expected id=%d, got id=%d", want.ID, got.ID)
		}
		if got.Name != "task-a" {
			t.Fatalf("expected name task-a, got %q", got.Name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for matching event")
	}
}

func TestMemoryBusSubscribeWithSinceID(t *testing.T) {
	bus := NewMemoryBus(64)
	first := bus.Publish(Event{Source: "apiserver", Type: "resource.created", Kind: "Task", Name: "t1"})
	second := bus.Publish(Event{Source: "apiserver", Type: "resource.updated", Kind: "Task", Name: "t1"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := bus.Subscribe(ctx, Filter{SinceID: first.ID})

	select {
	case got := <-ch:
		if got.ID != second.ID {
			t.Fatalf("expected second event id=%d, got %d", second.ID, got.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for replayed event")
	}
}
