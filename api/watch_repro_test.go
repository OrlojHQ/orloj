package api

import (
	"context"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/eventbus"
)

func TestWatchResourceStreamDoesNotPollWhenEventBusIsAvailable(t *testing.T) {
	var snapshots atomic.Int32
	server := &Server{bus: eventbus.NewMemoryBus(32)}
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/v1/tasks/watch", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})

	go func() {
		defer close(done)
		server.watchResourceStream(recorder, req, "Task", func() []watchRecord {
			snapshots.Add(1)
			return []watchRecord{{
				Name:            "task-a",
				Namespace:       "default",
				ResourceVersion: 1,
				Resource:        map[string]any{"metadata": map[string]any{"name": "task-a"}},
			}}
		})
	}()

	time.Sleep(2200 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watch stream shutdown")
	}

	if got := snapshots.Load(); got != 1 {
		t.Fatalf("expected only the initial snapshot when idle, got %d snapshots", got)
	}
}
