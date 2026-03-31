package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRunCreateTokenCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var (
		gotMethod string
		gotPath   string
		gotBody   map[string]string
	)
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		_ = r.Body.Close()
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}
		return mockResponse(r, http.StatusCreated, `{"name":"ci","role":"writer","token":"secret-token"}`), nil
	})

	var (
		out string
		err error
	)
	withRoundTripper(t, rt, func() {
		out, err = captureStdout(t, func() error {
			return Run([]string{"create", "token", "ci", "--role", "writer", "--server", "http://orloj.test"})
		})
	})
	if err != nil {
		t.Fatalf("create token command failed: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/v1/tokens" {
		t.Fatalf("expected /v1/tokens path, got %q", gotPath)
	}
	if gotBody["name"] != "ci" || gotBody["role"] != "writer" {
		t.Fatalf("unexpected create token payload: %+v", gotBody)
	}
	if !strings.Contains(out, "created token/ci role=writer") || !strings.Contains(out, "token: secret-token") {
		t.Fatalf("unexpected stdout: %q", out)
	}
}

func TestRunGetTokensCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/tokens" {
			t.Fatalf("expected /v1/tokens path, got %q", r.URL.Path)
		}
		return mockResponse(r, http.StatusOK, `{"items":[{"name":"ci","role":"writer","created_at":"2026-03-30T00:00:00Z"}]}`), nil
	})

	var (
		out string
		err error
	)
	withRoundTripper(t, rt, func() {
		out, err = captureStdout(t, func() error {
			return Run([]string{"get", "tokens", "--server", "http://orloj.test"})
		})
	})
	if err != nil {
		t.Fatalf("get tokens command failed: %v", err)
	}
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "ROLE") || !strings.Contains(out, "ci") {
		t.Fatalf("unexpected get tokens output: %q", out)
	}
}

func TestRunDeleteTokenCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var (
		gotMethod string
		gotPath   string
	)
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		return mockResponse(r, http.StatusOK, `{"status":"token deleted"}`), nil
	})

	var (
		out string
		err error
	)
	withRoundTripper(t, rt, func() {
		out, err = captureStdout(t, func() error {
			return Run([]string{"delete", "token", "ci", "--server", "http://orloj.test"})
		})
	})
	if err != nil {
		t.Fatalf("delete token command failed: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("expected DELETE, got %s", gotMethod)
	}
	if gotPath != "/v1/tokens/ci" {
		t.Fatalf("unexpected delete path %q", gotPath)
	}
	if !strings.Contains(out, "deleted tokens/ci") {
		t.Fatalf("unexpected stdout: %q", out)
	}
}

func TestRunAuthWhoamiCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/auth/me" {
			t.Fatalf("expected /v1/auth/me path, got %q", r.URL.Path)
		}
		return mockResponse(r, http.StatusOK, `{"authenticated":true,"method":"bearer","name":"ci-bot","role":"writer"}`), nil
	})

	var (
		out string
		err error
	)
	withRoundTripper(t, rt, func() {
		out, err = captureStdout(t, func() error {
			return Run([]string{"auth", "whoami", "--server", "http://orloj.test"})
		})
	})
	if err != nil {
		t.Fatalf("auth whoami command failed: %v", err)
	}
	if !strings.Contains(out, "method=bearer name=ci-bot role=writer") {
		t.Fatalf("unexpected whoami output: %q", out)
	}
}

func TestRunAdminCreateUserCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var (
		gotMethod string
		gotPath   string
		gotBody   map[string]string
	)
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		_ = r.Body.Close()
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}
		return mockResponse(r, http.StatusCreated, `{"username":"alice","role":"writer","password":"one-time-pass"}`), nil
	})

	var (
		out string
		err error
	)
	withRoundTripper(t, rt, func() {
		out, err = captureStdout(t, func() error {
			return Run([]string{"admin", "create-user", "alice", "--role", "writer", "--server", "http://orloj.test"})
		})
	})
	if err != nil {
		t.Fatalf("admin create-user command failed: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/auth/users" {
		t.Fatalf("unexpected request method/path: %s %s", gotMethod, gotPath)
	}
	if gotBody["username"] != "alice" || gotBody["role"] != "writer" {
		t.Fatalf("unexpected payload: %+v", gotBody)
	}
	if !strings.Contains(out, "created user/alice role=writer") || !strings.Contains(out, "password: one-time-pass") {
		t.Fatalf("unexpected stdout: %q", out)
	}
}
