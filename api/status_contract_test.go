package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/AnonJon/orloj/crds"
)

func TestTaskStatusContractFields(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/tasks", crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "contract-task"},
		Spec:       crds.TaskSpec{System: "sys"},
	})
	task := getTaskForContract(t, server.URL, "contract-task")

	patch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": task.Metadata.ResourceVersion,
		},
		"status": map[string]any{
			"phase":              "Running",
			"lastError":          "none",
			"startedAt":          "2026-03-09T10:00:00Z",
			"completedAt":        "",
			"nextAttemptAt":      "2026-03-09T10:05:00Z",
			"attempts":           2,
			"output":             map[string]string{"result": "ok"},
			"assignedWorker":     "worker-a",
			"claimedBy":          "worker-a",
			"leaseUntil":         "2026-03-09T10:01:00Z",
			"lastHeartbeat":      "2026-03-09T10:00:30Z",
			"trace":              []map[string]any{{"timestamp": "2026-03-09T10:00:01Z", "type": "agent_end", "agent": "a", "message": "done", "tool_contract_version": "v1", "tool_request_id": "default/contract-task/s001/web_search", "tool_attempt": 1, "error_code": "permission_denied", "error_reason": "tool_permission_denied", "retryable": false, "latency_ms": 10, "tokens": 20, "tool_calls": 1, "memory_writes": 1}},
			"history":            []map[string]any{{"timestamp": "2026-03-09T10:00:00Z", "type": "claim", "worker": "worker-a", "message": "claimed"}},
			"observedGeneration": 3,
		},
	}
	putStatusPatch(t, server.URL+"/v1/tasks/contract-task/status", patch)

	payload := getStatusPayload(t, server.URL+"/v1/tasks/contract-task/status")
	status := mustMapField(t, payload, "status")

	expectedTaskStatusKeys := []string{
		"phase",
		"lastError",
		"startedAt",
		"nextAttemptAt",
		"attempts",
		"output",
		"assignedWorker",
		"claimedBy",
		"leaseUntil",
		"lastHeartbeat",
		"trace",
		"history",
		"observedGeneration",
	}
	assertMapHasKeys(t, status, expectedTaskStatusKeys)

	trace := mustSliceField(t, status, "trace")
	if len(trace) == 0 {
		t.Fatal("expected trace entries")
	}
	trace0 := mustMapAny(t, trace[0])
	assertMapHasKeys(t, trace0, []string{"timestamp", "type", "agent", "message", "tool_contract_version", "tool_request_id", "tool_attempt", "error_code", "error_reason", "retryable", "latency_ms", "tokens", "tool_calls", "memory_writes"})

	history := mustSliceField(t, status, "history")
	if len(history) == 0 {
		t.Fatal("expected history entries")
	}
	history0 := mustMapAny(t, history[0])
	assertMapHasKeys(t, history0, []string{"timestamp", "type", "worker", "message"})
}

func TestWorkerStatusContractFields(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/workers", crds.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   crds.ObjectMeta{Name: "contract-worker"},
		Spec:       crds.WorkerSpec{Region: "default", MaxConcurrentTasks: 2},
	})
	worker := getWorkerForContract(t, server.URL, "contract-worker")

	patch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": worker.Metadata.ResourceVersion,
		},
		"status": map[string]any{
			"phase":              "Ready",
			"lastError":          "none",
			"lastHeartbeat":      "2026-03-09T11:00:00Z",
			"observedGeneration": 2,
			"currentTasks":       1,
		},
	}
	putStatusPatch(t, server.URL+"/v1/workers/contract-worker/status", patch)

	payload := getStatusPayload(t, server.URL+"/v1/workers/contract-worker/status")
	status := mustMapField(t, payload, "status")
	assertMapHasKeys(t, status, []string{"phase", "lastError", "lastHeartbeat", "observedGeneration", "currentTasks"})
}

func TestTaskScheduleStatusContractFields(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/task-schedules", crds.TaskSchedule{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskSchedule",
		Metadata:   crds.ObjectMeta{Name: "contract-schedule"},
		Spec: crds.TaskScheduleSpec{
			TaskRef:  "template-task",
			Schedule: "0 * * * *",
			TimeZone: "UTC",
		},
	})

	resp, err := http.Get(server.URL + "/v1/task-schedules/contract-schedule")
	if err != nil {
		t.Fatalf("get task schedule failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("get task schedule failed: %d body=%s", resp.StatusCode, string(b))
	}
	var schedule crds.TaskSchedule
	if err := json.NewDecoder(resp.Body).Decode(&schedule); err != nil {
		t.Fatalf("decode task schedule failed: %v", err)
	}

	patch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": schedule.Metadata.ResourceVersion,
		},
		"status": map[string]any{
			"phase":              "Ready",
			"lastError":          "none",
			"lastScheduleTime":   "2026-03-09T12:00:00Z",
			"lastSuccessfulTime": "2026-03-09T11:00:00Z",
			"nextScheduleTime":   "2026-03-09T13:00:00Z",
			"lastTriggeredTask":  "default/contract-schedule-20260309-1200",
			"activeRuns":         []string{"default/contract-schedule-20260309-1200"},
			"observedGeneration": 2,
		},
	}
	putStatusPatch(t, server.URL+"/v1/task-schedules/contract-schedule/status", patch)

	payload := getStatusPayload(t, server.URL+"/v1/task-schedules/contract-schedule/status")
	status := mustMapField(t, payload, "status")
	assertMapHasKeys(t, status, []string{
		"phase",
		"lastError",
		"lastScheduleTime",
		"lastSuccessfulTime",
		"nextScheduleTime",
		"lastTriggeredTask",
		"activeRuns",
		"observedGeneration",
	})
}

func TestTaskWebhookStatusContractFields(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/task-webhooks", crds.TaskWebhook{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskWebhook",
		Metadata:   crds.ObjectMeta{Name: "contract-webhook"},
		Spec: crds.TaskWebhookSpec{
			TaskRef: "template-task",
			Auth: crds.TaskWebhookAuthSpec{
				Profile:   "generic",
				SecretRef: "webhook-secret",
			},
		},
	})

	resp, err := http.Get(server.URL + "/v1/task-webhooks/contract-webhook")
	if err != nil {
		t.Fatalf("get task webhook failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("get task webhook failed: %d body=%s", resp.StatusCode, string(b))
	}
	var hook crds.TaskWebhook
	if err := json.NewDecoder(resp.Body).Decode(&hook); err != nil {
		t.Fatalf("decode task webhook failed: %v", err)
	}

	patch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": hook.Metadata.ResourceVersion,
		},
		"status": map[string]any{
			"phase":              "Ready",
			"lastError":          "none",
			"endpointID":         hook.Status.EndpointID,
			"endpointPath":       hook.Status.EndpointPath,
			"lastDeliveryTime":   "2026-03-14T12:00:00Z",
			"lastEventID":        "evt_123",
			"lastTriggeredTask":  "default/run-123",
			"acceptedCount":      1,
			"duplicateCount":     2,
			"rejectedCount":      3,
			"observedGeneration": 2,
		},
	}
	putStatusPatch(t, server.URL+"/v1/task-webhooks/contract-webhook/status", patch)

	payload := getStatusPayload(t, server.URL+"/v1/task-webhooks/contract-webhook/status")
	status := mustMapField(t, payload, "status")
	assertMapHasKeys(t, status, []string{
		"phase",
		"lastError",
		"endpointID",
		"endpointPath",
		"lastDeliveryTime",
		"lastEventID",
		"lastTriggeredTask",
		"acceptedCount",
		"duplicateCount",
		"rejectedCount",
		"observedGeneration",
	})
}

func putStatusPatch(t *testing.T, url string, patch map[string]any) {
	t.Helper()
	body, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("marshal status patch failed: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new status request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status update failed: %d body=%s", resp.StatusCode, string(b))
	}
}

func getStatusPayload(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("get status failed: %d body=%s", resp.StatusCode, string(b))
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode status payload failed: %v", err)
	}
	return payload
}

func getTaskForContract(t *testing.T, baseURL, name string) crds.Task {
	t.Helper()
	resp, err := http.Get(baseURL + "/v1/tasks/" + name)
	if err != nil {
		t.Fatalf("get task failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("get task failed: %d body=%s", resp.StatusCode, string(b))
	}
	var out crds.Task
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode task failed: %v", err)
	}
	return out
}

func getWorkerForContract(t *testing.T, baseURL, name string) crds.Worker {
	t.Helper()
	resp, err := http.Get(baseURL + "/v1/workers/" + name)
	if err != nil {
		t.Fatalf("get worker failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("get worker failed: %d body=%s", resp.StatusCode, string(b))
	}
	var out crds.Worker
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode worker failed: %v", err)
	}
	return out
}

func mustMapField(t *testing.T, payload map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := payload[key]
	if !ok {
		t.Fatalf("expected key %q in payload: %+v", key, payload)
	}
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map field for %q, got %T", key, value)
	}
	return out
}

func mustSliceField(t *testing.T, payload map[string]any, key string) []any {
	t.Helper()
	value, ok := payload[key]
	if !ok {
		t.Fatalf("expected key %q in payload: %+v", key, payload)
	}
	out, ok := value.([]any)
	if !ok {
		t.Fatalf("expected slice field for %q, got %T", key, value)
	}
	return out
}

func mustMapAny(t *testing.T, value any) map[string]any {
	t.Helper()
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map value, got %T", value)
	}
	return out
}

func assertMapHasKeys(t *testing.T, m map[string]any, keys []string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := m[key]; !ok {
			t.Fatalf("expected key %q in map %+v", key, m)
		}
	}
}
