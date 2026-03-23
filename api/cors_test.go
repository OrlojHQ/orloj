package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORS_AllowedOriginSetsHeaders(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	h := withCORS([]string{"https://app.example"}, next)

	req := httptest.NewRequest(http.MethodGet, "/v1/tools", nil)
	req.Header.Set("Origin", "https://app.example")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !nextCalled {
		t.Fatal("expected next handler to run for non-OPTIONS request")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("Access-Control-Allow-Origin: got %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("expected Access-Control-Allow-Methods")
	}
}

func TestWithCORS_DisallowedOriginNoCORSHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := withCORS([]string{"https://trusted.example"}, next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("expected no ACAO for disallowed origin, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestWithCORS_WildcardOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := withCORS([]string{"*"}, next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://any.example")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://any.example" {
		t.Fatalf("wildcard list: expected reflected origin, got %q", got)
	}
}

func TestWithCORS_OptionsShortCircuits(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	})
	h := withCORS([]string{"https://app.example"}, next)

	req := httptest.NewRequest(http.MethodOptions, "/v1/tools", nil)
	req.Header.Set("Origin", "https://app.example")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if nextCalled {
		t.Fatal("OPTIONS must not invoke next handler")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for OPTIONS, got %d", rr.Code)
	}
}

func TestWithCORS_TrimsWhitespaceInAllowList(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := withCORS([]string{"  https://trimmed.example  "}, next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://trimmed.example")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "https://trimmed.example" {
		t.Fatalf("expected allowed origin after trim, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}
