package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func waitFor(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(message)
}

func TestSelectRunnableTask(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, filepath.Join(dir, "one.yaml"), strings.Replace(minimalTaskRunYAML, "run-task", "task-one", 1))
	writeManifest(t, filepath.Join(dir, "two.yaml"), strings.Replace(minimalTaskRunYAML, "run-task", "task-two", 1))
	writeManifest(t, filepath.Join(dir, "template.yaml"), minimalTaskTemplateYAML)

	if _, err := selectRunnableTask(dir, ""); err == nil || !strings.Contains(err.Error(), "--task") {
		t.Fatalf("expected multiple tasks to require --task, got %v", err)
	}
	selected, err := selectRunnableTask(dir, "task-two")
	if err != nil {
		t.Fatalf("select task-two: %v", err)
	}
	if selected.Name != "task-two" {
		t.Fatalf("expected task-two, got %q", selected.Name)
	}
	if _, err := selectRunnableTask(dir, "missing"); err == nil {
		t.Fatal("expected missing task selection to fail")
	}
}

func TestSelectRunnableTaskTracksInferredRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.yaml")
	writeManifest(t, path, strings.Replace(minimalTaskRunYAML, "run-task", "task-before", 1))
	selected, err := selectRunnableTask(path, "")
	if err != nil || selected.Name != "task-before" {
		t.Fatalf("initial inferred task: selected=%+v err=%v", selected, err)
	}

	writeManifest(t, path, strings.Replace(minimalTaskRunYAML, "run-task", "task-after", 1))
	selected, err = selectRunnableTask(path, "")
	if err != nil || selected.Name != "task-after" {
		t.Fatalf("renamed inferred task: selected=%+v err=%v", selected, err)
	}
}

func TestManifestContentHashIgnoresUnchangedAndTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "agent.yaml")
	writeManifest(t, manifest, minimalAgentYAML)

	first, err := manifestContentHash(dir)
	if err != nil {
		t.Fatalf("initial hash: %v", err)
	}
	writeManifest(t, manifest, minimalAgentYAML)
	writeManifest(t, filepath.Join(dir, "agent.yaml.tmp"), "editor noise")
	second, err := manifestContentHash(dir)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if first != second {
		t.Fatal("expected unchanged manifest content and temporary files to keep the same hash")
	}

	writeManifest(t, manifest, strings.Replace(minimalAgentYAML, "prompt: hello", "prompt: changed", 1))
	third, err := manifestContentHash(dir)
	if err != nil {
		t.Fatalf("changed hash: %v", err)
	}
	if third == second {
		t.Fatal("expected changed manifest content to update the hash")
	}
}

func TestIsRelevantManifestEvent(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "agent.yaml")
	writeManifest(t, manifest, minimalAgentYAML)

	if !isRelevantManifestEvent(dir, fsnotify.Event{Name: manifest, Op: fsnotify.Write}) {
		t.Fatal("expected manifest write to be relevant")
	}
	if isRelevantManifestEvent(dir, fsnotify.Event{Name: filepath.Join(dir, "agent.yaml.tmp"), Op: fsnotify.Create}) {
		t.Fatal("expected temporary file to be ignored")
	}
	if isRelevantManifestEvent(dir, fsnotify.Event{Name: manifest, Op: fsnotify.Chmod}) {
		t.Fatal("expected chmod-only event to be ignored")
	}
	if isRelevantManifestEvent(dir, fsnotify.Event{Name: filepath.Join(t.TempDir(), "other.yaml"), Op: fsnotify.Write}) {
		t.Fatal("expected manifest outside root to be ignored")
	}
	if !isRelevantManifestEvent(dir, fsnotify.Event{Name: dir, Op: fsnotify.Rename}) {
		t.Fatal("expected replacement of the watched root to trigger a rescan")
	}
	if !pathWithinRoot(dir, filepath.Join(dir, "nested")) {
		t.Fatal("expected nested directory to be within root")
	}
	if pathWithinRoot(dir, filepath.Join(filepath.Dir(dir), "sibling")) {
		t.Fatal("expected sibling directory to remain outside root")
	}
}

func TestRunDevLoopRecoversAfterInvalidSave(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "agent.yaml")
	writeManifest(t, manifest, minimalAgentYAML)

	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/agents" {
			http.NotFound(w, r)
			return
		}
		posts.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]string{"name": "demo-agent"}})
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, errOut lockedBuffer
	done := make(chan error, 1)
	go func() {
		done <- runDevLoop(ctx, devOptions{
			Server:         server.URL,
			ManifestPath:   dir,
			Debounce:       30 * time.Millisecond,
			FollowInterval: time.Millisecond,
			Out:            &out,
			ErrOut:         &errOut,
		})
	}()

	waitFor(t, func() bool { return posts.Load() == 1 }, "initial manifest was not applied")
	writeManifest(t, manifest, "kind: Agent\nmetadata:\n")
	waitFor(t, func() bool {
		return strings.Contains(errOut.String(), "manifest validation failed")
	}, "invalid save was not reported")
	if posts.Load() != 1 {
		t.Fatalf("invalid manifest should not be applied, got %d posts", posts.Load())
	}

	changed := strings.Replace(minimalAgentYAML, "prompt: hello", "prompt: changed", 1)
	writeManifest(t, manifest, changed)
	waitFor(t, func() bool { return posts.Load() == 2 }, "valid follow-up save was not reapplied")

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("dev loop returned error after cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("dev loop did not stop after cancellation")
	}
}

func TestRunDevLoopFollowsRerunNameAndNamespace(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, filepath.Join(dir, "task.yaml"), minimalTaskRunYAML)

	var queryMu sync.Mutex
	var applyQuery, watchQuery, logsQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tasks":
			queryMu.Lock()
			applyQuery = r.URL.RawQuery
			queryMu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"metadata": map[string]string{"name": "run-task-run-123"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tasks/run-task-run-123/logs":
			queryMu.Lock()
			logsQuery = r.URL.RawQuery
			queryMu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "run-task-run-123",
				"logs": []string{"started"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tasks/watch":
			queryMu.Lock()
			watchQuery = r.URL.RawQuery
			queryMu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"type\":\"updated\",\"resource\":{\"metadata\":{\"name\":\"run-task-run-123\"},\"status\":{\"phase\":\"Succeeded\"}}}\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var out, errOut lockedBuffer
	done := make(chan error, 1)
	go func() {
		done <- runDevLoop(ctx, devOptions{
			Server:         server.URL,
			Namespace:      "team-a",
			ManifestPath:   dir,
			Run:            true,
			TaskName:       "run-task",
			Debounce:       30 * time.Millisecond,
			FollowInterval: time.Millisecond,
			Out:            &out,
			ErrOut:         &errOut,
		})
	}()

	waitFor(t, func() bool {
		text := out.String()
		return strings.Contains(text, "task: following run-task-run-123") &&
			strings.Contains(text, "logs: started") &&
			strings.Contains(text, "phase=Succeeded")
	}, "dev loop did not follow the server-generated task name")

	queryMu.Lock()
	queries := map[string]string{
		"apply": applyQuery,
		"watch": watchQuery,
		"logs":  logsQuery,
	}
	queryMu.Unlock()
	for label, rawQuery := range map[string]string{
		"apply": queries["apply"],
		"watch": queries["watch"],
		"logs":  queries["logs"],
	} {
		values, err := url.ParseQuery(rawQuery)
		if err != nil {
			t.Fatalf("parse %s query: %v", label, err)
		}
		if values.Get("namespace") != "team-a" {
			t.Fatalf("%s request missing namespace: %q", label, rawQuery)
		}
	}
	applyValues, _ := url.ParseQuery(queries["apply"])
	if applyValues.Get("rerun") != "true" {
		t.Fatalf("apply request missing rerun=true: %q", queries["apply"])
	}
	watchValues, _ := url.ParseQuery(queries["watch"])
	if watchValues.Get("name") != "run-task-run-123" {
		t.Fatalf("watch request used wrong task name: %q", queries["watch"])
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("dev loop returned error after cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("dev loop did not stop after cancellation")
	}
}

func TestApplyManifestsCanIgnoreActiveTaskConflict(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, filepath.Join(dir, "task.yaml"), minimalTaskRunYAML)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "task is still active", http.StatusConflict)
	}))
	defer server.Close()

	var out bytes.Buffer
	result, err := applyManifests(applyOptions{
		Server:              server.URL,
		ManifestPath:        dir,
		IncludeRunnable:     true,
		SelectedRunnable:    "run-task",
		IgnoreTaskConflicts: true,
		Out:                 &out,
	})
	if err != nil {
		t.Fatalf("expected active task conflict to be non-fatal in dev mode: %v", err)
	}
	if result.TaskConflicts != 1 {
		t.Fatalf("expected one task conflict, got %d", result.TaskConflicts)
	}
	if !strings.Contains(out.String(), "still active") {
		t.Fatalf("expected visible conflict message, got %q", out.String())
	}
}

func TestDevApplySkipsSingleRunnableTaskWithoutRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.yaml")
	writeManifest(t, path, minimalTaskRunYAML)

	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var out bytes.Buffer
	result, err := applyManifests(applyOptions{
		Server:             server.URL,
		ManifestPath:       path,
		SkipRunnableAlways: true,
		Out:                &out,
	})
	if err != nil {
		t.Fatalf("skip single runnable task: %v", err)
	}
	if posts.Load() != 0 || result.Applied != 0 || result.SkippedRunnableTasks != 1 {
		t.Fatalf("expected task to be skipped without --run, result=%+v posts=%d", result, posts.Load())
	}
}

func TestValidateDevManifestsRejectsMultiDocumentYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.yaml")
	raw := minimalAgentYAML + "\n---\n" + minimalTaskRunYAML
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write multi-document manifest: %v", err)
	}
	if err := validateDevManifests(path); err == nil || !strings.Contains(err.Error(), "multi-document") {
		t.Fatalf("expected multi-document validation error, got %v", err)
	}
}

func TestPrefixWriter(t *testing.T) {
	var out bytes.Buffer
	writer := prefixWriter{out: &out, prefix: "logs: "}
	n, err := io.WriteString(writer, "hello\n")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len("hello\n") || out.String() != "logs: hello\n" {
		t.Fatalf("unexpected prefixed output %q", out.String())
	}
}
