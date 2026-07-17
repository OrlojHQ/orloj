package a2a

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	lf "github.com/a2aproject/a2a-go/v2/a2a"
)

func TestSafePushSenderDeliversTaskAndAuthentication(t *testing.T) {
	var received lf.StreamResponse
	sender := NewSafePushSender(true, 0)
	sender.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get(notificationTokenHeader); got != "notify-token" {
			t.Errorf("notification token = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer credential" {
			t.Errorf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode callback body: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Status:     "204 No Content",
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})}

	task := &lf.Task{
		ID:        "task-1",
		ContextID: "context-1",
		Status:    lf.TaskStatus{State: lf.TaskStateCompleted},
	}
	err := sender.SendPush(context.Background(), &lf.PushConfig{
		URL:   "http://10.0.0.1/a2a",
		Token: "notify-token",
		Auth:  &lf.PushAuthInfo{Scheme: "Bearer", Credentials: "credential"},
	}, task)
	if err != nil {
		t.Fatalf("SendPush() error = %v", err)
	}
	got, ok := received.Event.(*lf.Task)
	if !ok || got.ID != "task-1" {
		t.Fatalf("received event = %#v", received.Event)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestSafePushSenderRejectsPrivateEndpointByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("private callback should not be reached")
	}))
	defer server.Close()

	err := NewSafePushSender(false, 0).SendPush(
		context.Background(),
		&lf.PushConfig{URL: server.URL},
		&lf.Task{ID: "task-1", ContextID: "context-1"},
	)
	if err == nil {
		t.Fatal("expected private endpoint validation error")
	}
}
