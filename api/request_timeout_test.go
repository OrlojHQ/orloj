package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithRequestTimeoutSkipsStreamingWatchRequests(t *testing.T) {
	var sawDeadline bool
	handler := withRequestTimeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawDeadline = r.Context().Deadline()
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/events/watch", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if sawDeadline {
		t.Fatal("expected streaming watch request to bypass the global timeout")
	}
}

func TestWithRequestTimeoutStillAppliesDeadlineToRegularReads(t *testing.T) {
	var sawDeadline bool
	handler := withRequestTimeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawDeadline = r.Context().Deadline()
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !sawDeadline {
		t.Fatal("expected regular GET requests to retain the read timeout")
	}
}
