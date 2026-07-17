package api

import (
	"context"
	"io"
	"log"
	"testing"

	lf "github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	orloja2a "github.com/OrlojHQ/orloj/runtime/a2a"
	"github.com/OrlojHQ/orloj/store"
)

type recordingPushSender struct {
	config *lf.PushConfig
	event  lf.Event
}

func (s *recordingPushSender) SendPush(_ context.Context, config *lf.PushConfig, event lf.Event) error {
	s.config = config
	s.event = event
	return nil
}

func TestDispatchA2APushEventDeliversLatestTask(t *testing.T) {
	ctx := context.Background()
	stores := Stores{
		Tasks:          store.NewTaskStore(),
		A2APushConfigs: store.NewA2APushConfigStore(),
	}
	server := NewServerWithOptions(stores, agentruntime.NewManager(log.New(io.Discard, "", 0)), log.New(io.Discard, "", 0), ServerOptions{})
	task, err := server.stores.Tasks.Upsert(ctx, resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Name:      "internal-task",
			Namespace: "team",
			Labels: map[string]string{
				orloja2a.LabelA2ATaskID:    "task-1",
				orloja2a.LabelA2AContextID: "context-1",
			},
		},
		Spec:   resources.TaskSpec{System: "system", Input: map[string]string{"prompt": "test"}},
		Status: resources.TaskStatus{Phase: "Succeeded"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	config, err := server.stores.A2APushConfigs.Save(ctx, "task-1", &lf.PushConfig{
		URL: "https://callbacks.example.com/a2a",
	})
	if err != nil {
		t.Fatalf("save push config: %v", err)
	}
	sender := &recordingPushSender{}

	server.dispatchA2APushEvent(ctx, sender, eventbus.Event{
		Kind:      "Task",
		Name:      task.Metadata.Name,
		Namespace: task.Metadata.Namespace,
	})

	if sender.config == nil || sender.config.ID != config.ID {
		t.Fatalf("delivered config = %#v, want %q", sender.config, config.ID)
	}
	delivered, ok := sender.event.(*lf.Task)
	if !ok {
		t.Fatalf("delivered event type = %T, want *a2a.Task", sender.event)
	}
	if delivered.ID != "task-1" || delivered.Status.State != lf.TaskStateCompleted {
		t.Fatalf("delivered task = %#v", delivered)
	}
}
