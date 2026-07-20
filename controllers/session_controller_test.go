package controllers

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

func TestSessionControllerRunsTurnThroughTask(t *testing.T) {
	sessions := store.NewSessionStore()
	tasks := store.NewTaskStore()
	systems := store.NewAgentSystemStore()
	if _, err := systems.Upsert(context.Background(), resources.AgentSystem{
		Metadata: resources.ObjectMeta{Name: "support"},
		Spec:     resources.AgentSystemSpec{Agents: []string{"assistant"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Upsert(context.Background(), resources.Session{
		Metadata: resources.ObjectMeta{Name: "chat"},
		Spec:     resources.SessionSpec{System: "support"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := sessions.EnqueueTurn(context.Background(), "chat", resources.SessionTurn{
		Content:        "hello",
		IdempotencyKey: "one",
	}); err != nil {
		t.Fatal(err)
	}

	controller := NewSessionController(sessions, tasks, systems, log.New(io.Discard, "", 0), time.Millisecond)
	controller.ConfigureWorker("test-worker", time.Second, 100*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- controller.ReconcileOnce(ctx)
	}()

	var task resources.Task
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		listed, err := tasks.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) > 0 {
			task = listed[0]
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if task.Metadata.Name == "" {
		t.Fatal("session controller did not create a child task")
	}
	if task.Spec.System != "support" || task.Spec.Input["topic"] != "hello" {
		t.Fatalf("child task = %#v", task.Spec)
	}

	task.Status.Phase = "Succeeded"
	task.Status.Output = map[string]string{"last_output": "Hello from the system"}
	if _, err := tasks.Upsert(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("session reconcile did not finish")
	}

	session, ok, err := sessions.Get(context.Background(), "chat")
	if err != nil || !ok {
		t.Fatalf("get session ok=%v err=%v", ok, err)
	}
	if session.Status.Phase != resources.SessionPhaseWaitingInput || session.Status.CompletedTurns != 1 {
		t.Fatalf("session status = %#v", session.Status)
	}
	events, err := sessions.ListEvents(context.Background(), "chat", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, evt := range events {
		if evt.Type == resources.SessionEventMessageCompleted && evt.Payload["content"] == "Hello from the system" {
			found = true
		}
	}
	if !found {
		t.Fatalf("message.completed not found in %#v", events)
	}
}

func TestTaskControllerStreamsModelDeltasIntoSessionEvents(t *testing.T) {
	sessions := store.NewSessionStore()
	if _, err := sessions.Upsert(context.Background(), resources.Session{
		Metadata: resources.ObjectMeta{Name: "stream-chat"},
		Spec:     resources.SessionSpec{System: "support"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := sessions.EnqueueTurn(context.Background(), "stream-chat", resources.SessionTurn{
		Content:        "hello",
		IdempotencyKey: "one",
	}); err != nil {
		t.Fatal(err)
	}
	claim, ok, _, err := sessions.ClaimNextTurn(context.Background(), "session-worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}

	controller := NewTaskController(nil, nil, nil, nil, nil, nil, nil, log.New(io.Discard, "", 0), time.Second)
	controller.SetSessionStore(sessions)
	task := resources.Task{
		Metadata: resources.ObjectMeta{
			Name:      "child-task",
			Namespace: resources.DefaultNamespace,
			Labels: map[string]string{
				"orloj.dev/session": "stream-chat",
				"orloj.dev/turn":    claim.Turn.ID,
			},
		},
		Spec: resources.TaskSpec{Input: map[string]string{"session.stream_agent": "writer"}},
	}
	sink := controller.executionEventSink(context.Background(), &task, "writer", "", "")
	sink.ModelStream(agentruntime.ModelStreamEvent{
		Type:  agentruntime.ModelStreamEventTextDelta,
		Delta: "Hel",
	})
	events, err := sessions.ListEvents(context.Background(), "stream-chat", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got := events[len(events)-1]; got.Type != resources.SessionEventMessageDelta || got.Payload["delta"] != "Hel" {
		t.Fatalf("last event = %#v", got)
	}

	before := len(events)
	filtered := controller.executionEventSink(context.Background(), &task, "researcher", "", "")
	filtered.ModelStream(agentruntime.ModelStreamEvent{
		Type:  agentruntime.ModelStreamEventTextDelta,
		Delta: "internal handoff",
	})
	events, err = sessions.ListEvents(context.Background(), "stream-chat", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != before {
		t.Fatalf("non-final agent delta was persisted: before=%d after=%d", before, len(events))
	}
}
