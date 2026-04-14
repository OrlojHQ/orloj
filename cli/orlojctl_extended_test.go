package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestNamespaceFlagOrdering(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return mockResponse(r, http.StatusOK, `{"items":[]}`), nil
	})

	t.Run("namespace before subcommand", func(t *testing.T) {
		withRoundTripper(t, rt, func() {
			_, err := captureStdout(t, func() error {
				return Run([]string{"--namespace", "prod", "get", "agents"})
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	})

	t.Run("namespace after positional args with -n", func(t *testing.T) {
		withRoundTripper(t, rt, func() {
			_, err := captureStdout(t, func() error {
				return Run([]string{"get", "agents", "-n", "prod"})
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	})

	t.Run("namespace after positional args with --namespace", func(t *testing.T) {
		withRoundTripper(t, rt, func() {
			_, err := captureStdout(t, func() error {
				return Run([]string{"get", "agents", "--namespace", "staging"})
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	})

	t.Run("delete with trailing -n", func(t *testing.T) {
		rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			ns := r.URL.Query().Get("namespace")
			if ns != "prod" {
				t.Errorf("expected namespace=prod in URL, got %q (url=%s)", ns, r.URL.String())
			}
			return mockResponse(r, http.StatusOK, `{"ok":true}`), nil
		})
		withRoundTripper(t, rt, func() {
			err := Run([]string{"delete", "agent", "my-agent", "-n", "prod"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	})
}

func TestNormalizeOutputFormat(t *testing.T) {
	cases := map[string]string{
		"":      "table",
		"json":  "json",
		"yaml":  "yaml",
		"yml":   "yaml",
		"table": "table",
	}
	for input, want := range cases {
		got, err := normalizeOutputFormat(input)
		if err != nil {
			t.Fatalf("normalizeOutputFormat(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeOutputFormat(%q) got %q want %q", input, got, want)
		}
	}
	if _, err := normalizeOutputFormat("xml"); err == nil {
		t.Fatal("expected unsupported format error for xml")
	}
}

func TestWaitConditionMet(t *testing.T) {
	if !waitConditionMet("Complete", "Succeeded") {
		t.Fatal("expected Complete to match Succeeded")
	}
	if !waitConditionMet("failed", "DeadLetter") {
		t.Fatal("expected failed to match DeadLetter")
	}
	if waitConditionMet("ready", "Running") {
		t.Fatal("did not expect ready to match Running")
	}
}

func TestParseTaskTarget(t *testing.T) {
	got, err := parseTaskTarget([]string{"task/demo"})
	if err != nil {
		t.Fatalf("parseTaskTarget task/demo failed: %v", err)
	}
	if got != "demo" {
		t.Fatalf("expected demo, got %q", got)
	}
	got, err = parseTaskTarget([]string{"task", "demo"})
	if err != nil {
		t.Fatalf("parseTaskTarget task demo failed: %v", err)
	}
	if got != "demo" {
		t.Fatalf("expected demo, got %q", got)
	}
	if _, err := parseTaskTarget([]string{"demo"}); err == nil {
		t.Fatal("expected parseTaskTarget error for invalid target")
	}
}

func TestPreviewApplyChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	desired := map[string]any{
		"apiVersion": "orloj.dev/v1",
		"kind":       "Agent",
		"metadata": map[string]any{
			"name":      "demo",
			"namespace": "default",
		},
		"spec": map[string]any{
			"prompt": "hello",
		},
	}
	payload, _ := json.Marshal(desired)

	t.Run("create when not found", func(t *testing.T) {
		rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return mockResponse(r, http.StatusNotFound, "not found"), nil
		})
		withRoundTripper(t, rt, func() {
			action, err := previewApplyChange("http://orloj.test", "/v1/agents", "demo", payload)
			if err != nil {
				t.Fatalf("previewApplyChange returned error: %v", err)
			}
			if action != "create" {
				t.Fatalf("expected create, got %q", action)
			}
		})
	})

	t.Run("no-op when desired equals normalized current", func(t *testing.T) {
		current := map[string]any{
			"apiVersion": "orloj.dev/v1",
			"kind":       "Agent",
			"metadata": map[string]any{
				"name":            "demo",
				"namespace":       "default",
				"resourceVersion": "42",
				"generation":      float64(7),
				"createdAt":       "2026-03-30T00:00:00Z",
			},
			"spec": map[string]any{
				"prompt": "hello",
			},
			"status": map[string]any{
				"phase": "Ready",
			},
		}
		raw, _ := json.Marshal(current)
		rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if !strings.Contains(r.URL.String(), "namespace=default") {
				t.Fatalf("expected namespace query, got url=%s", r.URL.String())
			}
			return mockResponse(r, http.StatusOK, string(raw)), nil
		})
		withRoundTripper(t, rt, func() {
			action, err := previewApplyChange("http://orloj.test", "/v1/agents", "demo", payload)
			if err != nil {
				t.Fatalf("previewApplyChange returned error: %v", err)
			}
			if action != "no-op" {
				t.Fatalf("expected no-op, got %q", action)
			}
		})
	})

	t.Run("update when current differs", func(t *testing.T) {
		current := map[string]any{
			"apiVersion": "orloj.dev/v1",
			"kind":       "Agent",
			"metadata": map[string]any{
				"name":      "demo",
				"namespace": "default",
			},
			"spec": map[string]any{
				"prompt": "different",
			},
		}
		raw, _ := json.Marshal(current)
		rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return mockResponse(r, http.StatusOK, string(raw)), nil
		})
		withRoundTripper(t, rt, func() {
			action, err := previewApplyChange("http://orloj.test", "/v1/agents", "demo", payload)
			if err != nil {
				t.Fatalf("previewApplyChange returned error: %v", err)
			}
			if action != "update" {
				t.Fatalf("expected update, got %q", action)
			}
		})
	})
}

func TestCanonicalComparableDocument(t *testing.T) {
	raw := []byte(`{
		"apiVersion":"orloj.dev/v1",
		"kind":"Task",
		"metadata":{"name":"demo","namespace":"default","resourceVersion":"7","generation":3,"createdAt":"2026-03-01T00:00:00Z"},
		"spec":{"system":"demo"},
		"status":{"phase":"Running"}
	}`)
	got, err := canonicalComparableDocument(raw)
	if err != nil {
		t.Fatalf("canonicalComparableDocument returned error: %v", err)
	}
	if strings.Contains(got, `"status"`) {
		t.Fatalf("expected status field to be removed, got:\n%s", got)
	}
	if strings.Contains(got, "resourceVersion") || strings.Contains(got, "generation") || strings.Contains(got, "createdAt") {
		t.Fatalf("expected mutable metadata fields to be removed, got:\n%s", got)
	}
	if !strings.Contains(got, `"name": "demo"`) || !strings.Contains(got, `"namespace": "default"`) {
		t.Fatalf("expected identity metadata preserved, got:\n%s", got)
	}
}

func TestRetryTaskSetsSourceLabels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	sourceTask := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Name:            "my-task",
			Namespace:       "default",
			ResourceVersion: "1",
			Labels:          map[string]string{"env": "prod"},
		},
		Spec:   resources.TaskSpec{System: "sys"},
		Status: resources.TaskStatus{Phase: "Failed"},
	}
	sourceJSON, _ := json.Marshal(sourceTask)

	var capturedPayload []byte
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			return mockResponse(r, http.StatusOK, string(sourceJSON)), nil
		}
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		capturedPayload = body
		return mockResponse(r, http.StatusCreated, "{}"), nil
	})

	withRoundTripper(t, rt, func() {
		_, _ = captureStdout(t, func() error {
			return Run([]string{"retry", "task", "my-task", "--server", "http://test"})
		})
	})

	if capturedPayload == nil {
		t.Fatal("expected retry to POST a new task")
	}
	var retried resources.Task
	if err := json.Unmarshal(capturedPayload, &retried); err != nil {
		t.Fatalf("decode retry payload: %v", err)
	}
	if retried.Metadata.Labels["orloj.dev/source-task"] != "my-task" {
		t.Fatalf("expected source-task label, got labels=%v", retried.Metadata.Labels)
	}
	if retried.Metadata.Labels["orloj.dev/source-task-namespace"] != "default" {
		t.Fatalf("expected source-task-namespace label, got labels=%v", retried.Metadata.Labels)
	}
	if retried.Metadata.Labels["env"] != "prod" {
		t.Fatalf("expected original labels preserved, got labels=%v", retried.Metadata.Labels)
	}
	if !strings.Contains(retried.Metadata.Name, "my-task-retry-") {
		t.Fatalf("expected retry name pattern, got %q", retried.Metadata.Name)
	}
}

func TestRenderUnifiedDiff(t *testing.T) {
	oldText := "apiVersion: orloj.dev/v1\nkind: Agent\nspec:\n  prompt: old\n"
	newText := "apiVersion: orloj.dev/v1\nkind: Agent\nspec:\n  prompt: new\n"
	got := renderUnifiedDiff(oldText, newText, "live/agent/demo", "desired/agent/demo")
	if !strings.Contains(got, "--- live/agent/demo") {
		t.Fatalf("expected old header, got:\n%s", got)
	}
	if !strings.Contains(got, "+++ desired/agent/demo") {
		t.Fatalf("expected new header, got:\n%s", got)
	}
	if !strings.Contains(got, "-  prompt: old") {
		t.Fatalf("expected removal line, got:\n%s", got)
	}
	if !strings.Contains(got, "+  prompt: new") {
		t.Fatalf("expected addition line, got:\n%s", got)
	}
}
