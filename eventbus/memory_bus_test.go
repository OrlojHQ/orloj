package eventbus

import (
	"context"
	"sync"
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

func TestMemoryBusPublishWhileSubscribersCancel(t *testing.T) {
	bus := NewMemoryBus(64)
	panicCh := make(chan any, 1)
	stop := make(chan struct{})

	var publishers sync.WaitGroup
	for i := 0; i < 4; i++ {
		publishers.Add(1)
		go func(idx int) {
			defer publishers.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					select {
					case panicCh <- recovered:
					default:
					}
				}
			}()
			for {
				select {
				case <-stop:
					return
				default:
					bus.Publish(Event{
						Source: "apiserver",
						Type:   "resource.updated",
						Kind:   "Task",
						Name:   "task-a",
						Action: "publisher",
					})
				}
			}
		}(i)
	}

	var subscribers sync.WaitGroup
	for i := 0; i < 200; i++ {
		subscribers.Add(1)
		go func(idx int) {
			defer subscribers.Done()
			ctx, cancel := context.WithCancel(context.Background())
			ch := bus.Subscribe(ctx, Filter{Kind: "Task"})
			time.Sleep(time.Duration((idx%5)+1) * time.Millisecond)
			select {
			case <-ch:
			default:
			}
			cancel()
		}(i)
	}

	subscribers.Wait()
	time.Sleep(25 * time.Millisecond)
	close(stop)
	publishers.Wait()

	select {
	case recovered := <-panicCh:
		t.Fatalf("publish panicked during subscriber cancellation: %v", recovered)
	default:
	}
}
