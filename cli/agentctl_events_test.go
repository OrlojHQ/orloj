package cli

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/AnonJon/orloj/eventbus"
)

func TestEventsWatchURL(t *testing.T) {
	got, err := eventsWatchURL("http://127.0.0.1:8080", eventFilters{
		Since:     42,
		Source:    "apiserver",
		Type:      "resource.created",
		Kind:      "Task",
		Name:      "weekly-report",
		Namespace: "team-a",
	})
	if err != nil {
		t.Fatalf("eventsWatchURL returned error: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("failed to parse output URL: %v", err)
	}
	if u.Path != "/v1/events/watch" {
		t.Fatalf("unexpected path: %q", u.Path)
	}
	q := u.Query()
	if q.Get("since") != "42" || q.Get("source") != "apiserver" || q.Get("type") != "resource.created" || q.Get("kind") != "Task" || q.Get("name") != "weekly-report" || q.Get("namespace") != "team-a" {
		t.Fatalf("unexpected query params: %v", q)
	}
}

func TestFormatEventLine(t *testing.T) {
	line := formatEventLine(eventbus.Event{
		ID:        7,
		Timestamp: "2026-03-10T01:02:03Z",
		Source:    "controller",
		Type:      "task.succeeded",
		Kind:      "Task",
		Name:      "weekly-report",
		Namespace: "team-a",
		Action:    "succeeded",
		Message:   "done",
	})
	required := []string{
		"2026-03-10T01:02:03Z",
		"id=7",
		"source=controller",
		"type=task.succeeded",
		"kind=Task",
		"name=weekly-report",
		"namespace=team-a",
		"action=succeeded",
		"message=done",
	}
	for _, part := range required {
		if !strings.Contains(line, part) {
			t.Fatalf("expected line %q to contain %q", line, part)
		}
	}
}

func TestEventsTimeoutError(t *testing.T) {
	timeout := 2 * time.Second

	if err := eventsTimeoutError(timeout, true, 0); err == nil {
		t.Fatal("expected timeout error for --once when no events are received")
	}
	if err := eventsTimeoutError(timeout, false, 0); err == nil {
		t.Fatal("expected timeout error when no events are received")
	}
	if err := eventsTimeoutError(timeout, false, 3); err != nil {
		t.Fatalf("expected nil when timeout elapsed after receiving events, got %v", err)
	}
}
