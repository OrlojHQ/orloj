package api

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	grpcpeer "google.golang.org/grpc/peer"
)

func TestA2AWaiterLimitAndTimeout(t *testing.T) {
	server := &Server{
		a2aConfig: &A2AConfig{
			MaxConcurrentSubscribe: 1,
			MaxWaitDuration:        20 * time.Millisecond,
		},
	}
	firstCtx, release, ok := server.acquireA2AWaiter(context.Background())
	if !ok {
		t.Fatal("first waiter was rejected")
	}
	if _, _, ok := server.acquireA2AWaiter(context.Background()); ok {
		t.Fatal("second waiter exceeded configured limit")
	}
	select {
	case <-firstCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("waiter did not reach configured deadline")
	}
	release()
	_, releaseAgain, ok := server.acquireA2AWaiter(context.Background())
	if !ok {
		t.Fatal("waiter slot was not released")
	}
	releaseAgain()
}

func TestSessionErrorDoesNotExposeInternalDetails(t *testing.T) {
	var logs bytes.Buffer
	server := &Server{logger: log.New(&logs, "", 0)}
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/chat/checkpoints", nil)
	response := httptest.NewRecorder()
	internal := errors.New(`pq: relation "session_checkpoints" does not exist`)
	server.writeSessionError(response, request, internal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	if strings.Contains(response.Body.String(), "pq:") ||
		strings.Contains(response.Body.String(), "session_checkpoints") {
		t.Fatalf("internal detail leaked in response: %q", response.Body.String())
	}
	if !strings.Contains(logs.String(), internal.Error()) {
		t.Fatalf("internal detail missing from server log: %q", logs.String())
	}
}

func TestA2AGRPCCallContextUsesPeerForRateLimiting(t *testing.T) {
	server := &Server{}
	server.SetA2AConfig(&A2AConfig{RateLimitRPM: 1})
	handler := &a2aV1Handler{server: server}
	ctx := grpcpeer.NewContext(context.Background(), &grpcpeer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.44"), Port: 4242},
	})
	if _, err := handler.callFromContext(ctx); err != nil {
		t.Fatalf("first callFromContext() error = %v", err)
	}
	if _, err := handler.callFromContext(ctx); err == nil {
		t.Fatal("second gRPC call bypassed per-IP rate limit")
	}
}
