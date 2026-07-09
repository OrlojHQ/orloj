package agentruntime

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

type countingErrorGateway struct {
	calls atomic.Int32
	err   error
}

func (g *countingErrorGateway) Complete(_ context.Context, _ ModelRequest) (ModelResponse, error) {
	g.calls.Add(1)
	return ModelResponse{}, g.err
}

func TestAgentWorkerStopsAfterConsecutiveModelErrors(t *testing.T) {
	gateway := &countingErrorGateway{err: errors.New("context deadline exceeded")}
	var events []string
	worker := NewAgentWorkerWithIntervalAndGateway(
		resources.Agent{
			Metadata: resources.ObjectMeta{Name: "planner", Namespace: "ns"},
			Spec: resources.AgentSpec{
				Model:  "claude-sonnet-4-5",
				Prompt: "test",
				Limits: resources.AgentLimits{MaxSteps: 20},
			},
		},
		&staticToolRuntime{},
		nil,
		gateway,
		func(event string) { events = append(events, event) },
		time.Millisecond,
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(context.Background())
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after consecutive model errors")
	}

	if got := gateway.calls.Load(); got != 3 {
		t.Fatalf("expected 3 model attempts before circuit open, got %d", got)
	}
	joined := strings.Join(events, "\n")
	if !strings.Contains(joined, "model_error circuit open after 3 consecutive failures") {
		t.Fatalf("expected circuit-open event, got events:\n%s", joined)
	}
	if !strings.Contains(joined, "worker stopped model error") {
		t.Fatalf("expected worker stopped model error, got events:\n%s", joined)
	}
}

func TestAgentWorkerStopsOnNonRetryableModelGatewayError(t *testing.T) {
	gateway := &countingErrorGateway{err: &ModelGatewayError{
		StatusCode: http.StatusUnauthorized,
		Provider:   "anthropic",
		Message:    "invalid api key",
	}}
	var events []string
	worker := NewAgentWorkerWithIntervalAndGateway(
		resources.Agent{
			Metadata: resources.ObjectMeta{Name: "planner", Namespace: "ns"},
			Spec: resources.AgentSpec{
				Model:  "claude-sonnet-4-5",
				Prompt: "test",
				Limits: resources.AgentLimits{MaxSteps: 20},
			},
		},
		&staticToolRuntime{},
		nil,
		gateway,
		func(event string) { events = append(events, event) },
		time.Millisecond,
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(context.Background())
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop on non-retryable model error")
	}

	if got := gateway.calls.Load(); got != 1 {
		t.Fatalf("expected single non-retryable attempt, got %d", got)
	}
	joined := strings.Join(events, "\n")
	if !strings.Contains(joined, "model_error non-retryable status=401") {
		t.Fatalf("expected non-retryable stop event, got events:\n%s", joined)
	}
}
