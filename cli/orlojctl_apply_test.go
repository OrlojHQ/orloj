package cli

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const minimalTaskRunYAML = `apiVersion: orloj.dev/v1
kind: Task
metadata:
  name: run-task
spec:
  system: demo-system
  input:
    topic: test
`

const minimalTaskTemplateYAML = `apiVersion: orloj.dev/v1
kind: Task
metadata:
  name: template-task
spec:
  mode: template
  system: demo-system
  input:
    topic: test
`

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	_ = r.Close()
	return string(out), runErr
}

func writeManifest(t *testing.T, path string, raw string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type applyRecorder struct {
	mu          sync.Mutex
	agentPosts  int
	taskPosts   int
	taskPayload []string
}

type mockApplyTransport struct {
	rec *applyRecorder
}

func (m *mockApplyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()

	m.rec.mu.Lock()
	defer m.rec.mu.Unlock()

	statusCode := http.StatusNotFound
	switch r.URL.Path {
	case "/v1/agents":
		m.rec.agentPosts++
		statusCode = http.StatusOK
	case "/v1/tasks":
		m.rec.taskPosts++
		m.rec.taskPayload = append(m.rec.taskPayload, string(body))
		statusCode = http.StatusOK
	}

	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader([]byte("{}"))),
		Request:    r,
	}, nil
}

func withMockTransport(t *testing.T, rec *applyRecorder, fn func()) {
	t.Helper()
	oldTransport := http.DefaultTransport
	oldClient := http.DefaultClient
	http.DefaultTransport = &mockApplyTransport{rec: rec}
	defer func() {
		http.DefaultTransport = oldTransport
		http.DefaultClient = oldClient
	}()
	fn()
}

func testServerURL() string {
	return "http://orloj.test"
}

func recordPostPaths(rec *applyRecorder) (int, int, []string) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	payloadCopy := make([]string, len(rec.taskPayload))
	copy(payloadCopy, rec.taskPayload)
	return rec.agentPosts, rec.taskPosts, payloadCopy
}

func TestRunApply_DirectorySkipsRunnableTaskWithoutRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	writeManifest(t, filepath.Join(dir, "agent.yaml"), minimalAgentYAML)
	writeManifest(t, filepath.Join(dir, "task-run.yaml"), minimalTaskRunYAML)
	writeManifest(t, filepath.Join(dir, "task-template.yaml"), minimalTaskTemplateYAML)

	rec := &applyRecorder{}

	var (
		out string
		err error
	)
	withMockTransport(t, rec, func() {
		out, err = captureStdout(t, func() error {
			return Run([]string{"apply", "-f", dir, "--server", testServerURL()})
		})
	})
	if err != nil {
		t.Fatalf("apply directory failed: %v", err)
	}
	if !strings.Contains(out, "use --run to include") {
		t.Fatalf("expected skip hint in output, got:\n%s", out)
	}

	agentPosts, taskPosts, taskPayload := recordPostPaths(rec)
	if agentPosts != 1 {
		t.Fatalf("expected 1 agent apply, got %d", agentPosts)
	}
	if taskPosts != 1 {
		t.Fatalf("expected 1 task apply (template only), got %d", taskPosts)
	}
	if len(taskPayload) != 1 || !strings.Contains(taskPayload[0], "\"mode\":\"template\"") {
		t.Fatalf("expected template task payload, got %v", taskPayload)
	}
}

func TestRunApply_DirectoryIncludesRunnableWithRunFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	writeManifest(t, filepath.Join(dir, "agent.yaml"), minimalAgentYAML)
	writeManifest(t, filepath.Join(dir, "task-run.yaml"), minimalTaskRunYAML)
	writeManifest(t, filepath.Join(dir, "task-template.yaml"), minimalTaskTemplateYAML)

	rec := &applyRecorder{}

	var (
		out string
		err error
	)
	withMockTransport(t, rec, func() {
		out, err = captureStdout(t, func() error {
			return Run([]string{"apply", "-f", dir, "--run", "--server", testServerURL()})
		})
	})
	if err != nil {
		t.Fatalf("apply directory with --run failed: %v", err)
	}
	if strings.Contains(out, "skipped task/") {
		t.Fatalf("did not expect skip output with --run, got:\n%s", out)
	}

	agentPosts, taskPosts, _ := recordPostPaths(rec)
	if agentPosts != 1 {
		t.Fatalf("expected 1 agent apply, got %d", agentPosts)
	}
	if taskPosts != 2 {
		t.Fatalf("expected 2 task applies, got %d", taskPosts)
	}
}

func TestRunApply_SingleRunnableTaskStillAppliesWithoutRunFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	taskPath := filepath.Join(dir, "task-run.yaml")
	writeManifest(t, taskPath, minimalTaskRunYAML)

	rec := &applyRecorder{}
	var err error
	withMockTransport(t, rec, func() {
		err = Run([]string{"apply", "-f", taskPath, "--server", testServerURL()})
	})
	if err != nil {
		t.Fatalf("single-file runnable task apply failed: %v", err)
	}

	_, taskPosts, _ := recordPostPaths(rec)
	if taskPosts != 1 {
		t.Fatalf("expected runnable task to apply for single file, got %d", taskPosts)
	}
}

func TestRunApply_SingleTemplateTaskAppliesWithoutRunFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	taskPath := filepath.Join(dir, "task-template.yaml")
	writeManifest(t, taskPath, minimalTaskTemplateYAML)

	rec := &applyRecorder{}
	var err error
	withMockTransport(t, rec, func() {
		err = Run([]string{"apply", "-f", taskPath, "--server", testServerURL()})
	})
	if err != nil {
		t.Fatalf("single-file template task apply failed: %v", err)
	}

	_, taskPosts, taskPayload := recordPostPaths(rec)
	if taskPosts != 1 {
		t.Fatalf("expected template task to apply for single file, got %d", taskPosts)
	}
	if len(taskPayload) != 1 || !strings.Contains(taskPayload[0], "\"mode\":\"template\"") {
		t.Fatalf("expected template mode payload, got %v", taskPayload)
	}
}

func TestRunApply_DirectoryOnlyRunnableTasksReturnsSuccess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	writeManifest(t, filepath.Join(dir, "task-run.yaml"), minimalTaskRunYAML)

	rec := &applyRecorder{}
	var (
		out string
		err error
	)
	withMockTransport(t, rec, func() {
		out, err = captureStdout(t, func() error {
			return Run([]string{"apply", "-f", dir, "--server", testServerURL()})
		})
	})
	if err != nil {
		t.Fatalf("expected success when directory has only runnable tasks, got %v", err)
	}
	if !strings.Contains(out, "0 file(s) applied, 1 runnable task(s) skipped") {
		t.Fatalf("expected zero-applied summary, got:\n%s", out)
	}

	_, taskPosts, _ := recordPostPaths(rec)
	if taskPosts != 0 {
		t.Fatalf("expected no task apply calls, got %d", taskPosts)
	}
}

func TestRunApply_DirectoryInvalidTaskStillFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	writeManifest(t, filepath.Join(dir, "a-agent.yaml"), minimalAgentYAML)
	writeManifest(t, filepath.Join(dir, "b-bad-task.yaml"), `apiVersion: orloj.dev/v1
kind: Task
metadata:
  namespace: default
spec:
  system: demo-system
`)

	rec := &applyRecorder{}
	var err error
	withMockTransport(t, rec, func() {
		err = Run([]string{"apply", "-f", dir, "--server", testServerURL()})
	})
	if err == nil {
		t.Fatal("expected apply error for invalid task manifest")
	}
	if !strings.Contains(err.Error(), "apply failed") {
		t.Fatalf("unexpected error: %v", err)
	}

	agentPosts, _, _ := recordPostPaths(rec)
	if agentPosts != 1 {
		t.Fatalf("expected valid resources to be applied before failure, got %d agent applies", agentPosts)
	}
}

func TestRunApply_ReapplyDirectoryKeepsSkippingRunnableTasksWithoutRunFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	writeManifest(t, filepath.Join(dir, "agent.yaml"), minimalAgentYAML)
	writeManifest(t, filepath.Join(dir, "task-run.yaml"), minimalTaskRunYAML)

	rec := &applyRecorder{}
	var (
		firstOut  string
		secondOut string
		err       error
	)
	withMockTransport(t, rec, func() {
		firstOut, err = captureStdout(t, func() error {
			return Run([]string{"apply", "-f", dir, "--server", testServerURL()})
		})
		if err != nil {
			return
		}
		secondOut, err = captureStdout(t, func() error {
			return Run([]string{"apply", "-f", dir, "--server", testServerURL()})
		})
	})
	if err != nil {
		t.Fatalf("re-apply failed: %v", err)
	}
	if !strings.Contains(firstOut, "runnable task(s) skipped") {
		t.Fatalf("expected first apply to report skipped runnable task, got:\n%s", firstOut)
	}
	if !strings.Contains(secondOut, "runnable task(s) skipped") {
		t.Fatalf("expected second apply to report skipped runnable task, got:\n%s", secondOut)
	}

	agentPosts, taskPosts, _ := recordPostPaths(rec)
	if agentPosts != 2 {
		t.Fatalf("expected agent manifest to apply twice, got %d applies", agentPosts)
	}
	if taskPosts != 0 {
		t.Fatalf("expected runnable task to remain skipped across re-apply, got %d task applies", taskPosts)
	}
}
