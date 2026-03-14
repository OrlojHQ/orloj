package eventbus

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNATSBusCrossInstanceDelivery(t *testing.T) {
	url := os.Getenv("ORLOJ_NATS_URL")
	if url == "" {
		t.Skip("ORLOJ_NATS_URL not set; skipping NATS integration test")
	}
	subjectPrefix := "orloj.test." + time.Now().UTC().Format("20060102150405")

	busA, err := NewNATSBus(url, subjectPrefix, 64, nil)
	if err != nil {
		t.Skipf("nats unavailable at ORLOJ_NATS_URL: %v", err)
	}
	defer busA.Close()

	busB, err := NewNATSBus(url, subjectPrefix, 64, nil)
	if err != nil {
		t.Fatalf("create second nats bus failed: %v", err)
	}
	defer busB.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := busB.Subscribe(ctx, Filter{Source: "apiserver", Kind: "Task", Name: "task-nats"})

	busA.Publish(Event{Source: "apiserver", Type: "resource.created", Kind: "Task", Name: "task-nats"})

	select {
	case evt := <-ch:
		if evt.Kind != "Task" || evt.Name != "task-nats" {
			t.Fatalf("unexpected event kind=%q name=%q", evt.Kind, evt.Name)
		}
		if evt.Type != "resource.created" {
			t.Fatalf("expected resource.created, got %q", evt.Type)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("timed out waiting for cross-instance nats event")
	}
}
