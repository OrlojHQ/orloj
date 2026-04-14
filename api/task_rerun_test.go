package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func postTask(t *testing.T, baseURL string, task resources.Task) resources.Task {
	t.Helper()
	body, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}
	resp, err := http.Post(baseURL+"/v1/tasks", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post task: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("post task status=%d body=%s", resp.StatusCode, string(respBody))
	}
	var out resources.Task
	if err := json.Unmarshal(respBody, &out); err != nil {
		t.Fatalf("decode task response: %v", err)
	}
	return out
}

func setTaskPhase(t *testing.T, baseURL string, task resources.Task, phase string) {
	t.Helper()
	patch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": task.Metadata.ResourceVersion,
		},
		"status": map[string]any{
			"phase": phase,
		},
	}
	putStatusPatch(t, baseURL+"/v1/tasks/"+task.Metadata.Name+"/status", patch)
}

func TestTaskRerun_TerminalCreatesNewInstance(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	for _, phase := range []string{"DeadLetter", "Failed", "Succeeded"} {
		t.Run(phase, func(t *testing.T) {
			name := "rerun-" + strings.ToLower(phase)
			original := postTask(t, server.URL, resources.Task{
				APIVersion: "orloj.dev/v1",
				Kind:       "Task",
				Metadata:   resources.ObjectMeta{Name: name},
				Spec:       resources.TaskSpec{System: "sys"},
			})
			setTaskPhase(t, server.URL, original, phase)

			body, _ := json.Marshal(resources.Task{
				APIVersion: "orloj.dev/v1",
				Kind:       "Task",
				Metadata:   resources.ObjectMeta{Name: name, Labels: map[string]string{"env": "test"}},
				Spec:       resources.TaskSpec{System: "sys"},
			})
			resp, err := http.Post(server.URL+"/v1/tasks?rerun=true", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("rerun request failed: %v", err)
			}
			defer resp.Body.Close()
			respBody, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(respBody))
			}

			var rerunTask resources.Task
			if err := json.Unmarshal(respBody, &rerunTask); err != nil {
				t.Fatalf("decode rerun response: %v", err)
			}

			if rerunTask.Metadata.Name == name {
				t.Fatal("expected new instance name, got original name")
			}
			if !strings.HasPrefix(rerunTask.Metadata.Name, name+"-run-") {
				t.Fatalf("expected name prefix %q-run-, got %q", name, rerunTask.Metadata.Name)
			}
			if rerunTask.Metadata.Labels["orloj.dev/source-task"] != name {
				t.Fatalf("expected source-task label %q, got %q", name, rerunTask.Metadata.Labels["orloj.dev/source-task"])
			}
			if rerunTask.Metadata.Labels["env"] != "test" {
				t.Fatal("expected manifest labels to be preserved")
			}
			if rerunTask.Status.Phase != "" && !strings.EqualFold(rerunTask.Status.Phase, "Pending") {
				t.Fatalf("expected fresh phase for new instance, got %q", rerunTask.Status.Phase)
			}

			// Original task should still exist with its terminal phase.
			origAfter := getTaskForContract(t, server.URL, name)
			if !strings.EqualFold(origAfter.Status.Phase, phase) {
				t.Fatalf("expected original task to retain phase %q, got %q", phase, origAfter.Status.Phase)
			}
		})
	}
}

func TestTaskRerun_ActiveReturns409(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	for _, phase := range []string{"Pending", "Running", ""} {
		label := phase
		if label == "" {
			label = "empty"
		}
		t.Run(label, func(t *testing.T) {
			name := "active-" + strings.ToLower(label)
			created := postTask(t, server.URL, resources.Task{
				APIVersion: "orloj.dev/v1",
				Kind:       "Task",
				Metadata:   resources.ObjectMeta{Name: name},
				Spec:       resources.TaskSpec{System: "sys"},
			})
			if phase != "" {
				setTaskPhase(t, server.URL, created, phase)
			}

			body, _ := json.Marshal(resources.Task{
				APIVersion: "orloj.dev/v1",
				Kind:       "Task",
				Metadata:   resources.ObjectMeta{Name: name},
				Spec:       resources.TaskSpec{System: "sys"},
			})
			resp, err := http.Post(server.URL+"/v1/tasks?rerun=true", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("rerun request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusConflict {
				respBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 409, got %d: %s", resp.StatusCode, string(respBody))
			}
		})
	}
}

func TestTaskRerun_NonExistentCreatesNormally(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	body, _ := json.Marshal(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "brand-new"},
		Spec:       resources.TaskSpec{System: "sys"},
	})
	resp, err := http.Post(server.URL+"/v1/tasks?rerun=true", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(respBody))
	}

	var task resources.Task
	if err := json.Unmarshal(respBody, &task); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if task.Metadata.Name != "brand-new" {
		t.Fatalf("expected original name for new task, got %q", task.Metadata.Name)
	}
}

func TestTaskRerun_WithoutParamPreservesStatus(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	original := postTask(t, server.URL, resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "keep-status"},
		Spec:       resources.TaskSpec{System: "sys"},
	})
	setTaskPhase(t, server.URL, original, "Failed")

	body, _ := json.Marshal(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "keep-status"},
		Spec:       resources.TaskSpec{System: "sys"},
	})
	resp, err := http.Post(server.URL+"/v1/tasks", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(respBody))
	}

	var task resources.Task
	if err := json.Unmarshal(respBody, &task); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if task.Metadata.Name != "keep-status" {
		t.Fatalf("expected same name, got %q", task.Metadata.Name)
	}
	if !strings.EqualFold(task.Status.Phase, "Failed") {
		t.Fatalf("expected status to be preserved as Failed, got %q", task.Status.Phase)
	}
}
