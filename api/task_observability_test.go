package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/OrlojHQ/orloj/crds"
)

func TestTaskMessagesEndpointFiltersByLifecycleAndAgent(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedTaskObservabilityFixture(t, server.URL)

	resp, err := http.Get(server.URL + "/v1/tasks/obs-task/messages?to_agent=writer&phase=running,retrypending")
	if err != nil {
		t.Fatalf("get filtered messages failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status=%d body=%s", resp.StatusCode, string(body))
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if got := intField(t, payload, "total"); got != 2 {
		t.Fatalf("expected total=2, got %d payload=%+v", got, payload)
	}
	if got := intField(t, payload, "filtered_from"); got != 3 {
		t.Fatalf("expected filtered_from=3, got %d payload=%+v", got, payload)
	}

	counts := mapField(t, payload, "lifecycle_counts")
	if intField(t, counts, "running") != 1 || intField(t, counts, "retrypending") != 1 || intField(t, counts, "succeeded") != 1 {
		t.Fatalf("unexpected lifecycle counts: %+v", counts)
	}
	messages := sliceField(t, payload, "messages")
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
}

func TestTaskMessagesEndpointRejectsUnknownLifecycle(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedTaskObservabilityFixture(t, server.URL)

	resp, err := http.Get(server.URL + "/v1/tasks/obs-task/messages?phase=unknown")
	if err != nil {
		t.Fatalf("get messages failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 for invalid lifecycle, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestTaskMetricsEndpointAggregatesPerAgentAndPerEdge(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	seedTaskObservabilityFixture(t, server.URL)

	resp, err := http.Get(server.URL + "/v1/tasks/obs-task/metrics")
	if err != nil {
		t.Fatalf("get metrics failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status=%d body=%s", resp.StatusCode, string(body))
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode metrics payload failed: %v", err)
	}

	totals := mapField(t, payload, "totals")
	if intField(t, totals, "messages") != 5 {
		t.Fatalf("expected totals.messages=5, got %+v", totals)
	}
	if intField(t, totals, "in_flight") != 3 {
		t.Fatalf("expected totals.in_flight=3, got %+v", totals)
	}
	if intField(t, totals, "retry_count") != 3 {
		t.Fatalf("expected totals.retry_count=3, got %+v", totals)
	}
	if intField(t, totals, "deadletters") != 1 {
		t.Fatalf("expected totals.deadletters=1, got %+v", totals)
	}
	if intField(t, totals, "latency_sample_size") != 2 || intField(t, totals, "latency_ms_avg") != 50 || intField(t, totals, "latency_ms_p95") != 80 {
		t.Fatalf("unexpected totals latency stats: %+v", totals)
	}

	perAgent := sliceField(t, payload, "per_agent")
	writer := findMapByKeyValue(t, perAgent, "agent", "writer")
	if intField(t, writer, "inbound") != 3 || intField(t, writer, "outbound") != 1 || intField(t, writer, "retry_count") != 1 {
		t.Fatalf("unexpected writer metrics: %+v", writer)
	}

	perEdge := sliceField(t, payload, "per_edge")
	plannerWriter := findEdgeMetric(t, perEdge, "planner", "writer")
	if intField(t, plannerWriter, "messages") != 3 || intField(t, plannerWriter, "retry_count") != 1 {
		t.Fatalf("unexpected planner->writer metrics: %+v", plannerWriter)
	}

	respFiltered, err := http.Get(server.URL + "/v1/tasks/obs-task/metrics?phase=deadletter")
	if err != nil {
		t.Fatalf("get filtered metrics failed: %v", err)
	}
	defer respFiltered.Body.Close()
	if respFiltered.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respFiltered.Body)
		t.Fatalf("unexpected filtered status=%d body=%s", respFiltered.StatusCode, string(body))
	}
	var filteredPayload map[string]any
	if err := json.NewDecoder(respFiltered.Body).Decode(&filteredPayload); err != nil {
		t.Fatalf("decode filtered metrics failed: %v", err)
	}
	filteredTotals := mapField(t, filteredPayload, "totals")
	if intField(t, filteredTotals, "messages") != 1 || intField(t, filteredTotals, "deadletter") != 1 {
		t.Fatalf("unexpected filtered totals: %+v", filteredTotals)
	}
}

func seedTaskObservabilityFixture(t *testing.T, baseURL string) {
	t.Helper()

	postJSON(t, baseURL+"/v1/tasks", crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "obs-task"},
		Spec:       crds.TaskSpec{System: "report-system"},
	})
	current := getTaskForContract(t, baseURL, "obs-task")

	patch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": current.Metadata.ResourceVersion,
		},
		"status": map[string]any{
			"phase": "Running",
			"messages": []map[string]any{
				{
					"message_id": "m1",
					"task_id":    "default/obs-task",
					"from_agent": "system",
					"to_agent":   "planner",
					"phase":      "Queued",
					"attempts":   1,
					"timestamp":  "2026-03-10T10:00:00Z",
					"trace_id":   "default/obs-task/a001",
					"branch_id":  "b001",
				},
				{
					"message_id": "m2",
					"task_id":    "default/obs-task",
					"from_agent": "planner",
					"to_agent":   "writer",
					"phase":      "Running",
					"attempts":   1,
					"timestamp":  "2026-03-10T10:00:10Z",
					"trace_id":   "default/obs-task/a001",
					"branch_id":  "b001",
				},
				{
					"message_id": "m3",
					"task_id":    "default/obs-task",
					"from_agent": "planner",
					"to_agent":   "writer",
					"phase":      "RetryPending",
					"attempts":   2,
					"timestamp":  "2026-03-10T10:00:20Z",
					"trace_id":   "default/obs-task/a001",
					"branch_id":  "b001",
				},
				{
					"message_id":   "m4",
					"task_id":      "default/obs-task",
					"from_agent":   "planner",
					"to_agent":     "writer",
					"phase":        "Succeeded",
					"attempts":     1,
					"timestamp":    "2026-03-10T10:00:30Z",
					"processed_at": "2026-03-10T10:00:30.020Z",
					"trace_id":     "default/obs-task/a001",
					"branch_id":    "b001",
				},
				{
					"message_id":   "m5",
					"task_id":      "default/obs-task",
					"from_agent":   "writer",
					"to_agent":     "reviewer",
					"phase":        "DeadLetter",
					"attempts":     3,
					"timestamp":    "2026-03-10T10:01:00Z",
					"processed_at": "2026-03-10T10:01:00.080Z",
					"trace_id":     "default/obs-task/a001",
					"branch_id":    "b001",
				},
			},
		},
	}
	putStatusPatch(t, baseURL+"/v1/tasks/obs-task/status", patch)
}

func intField(t *testing.T, payload map[string]any, key string) int {
	t.Helper()
	value, ok := payload[key]
	if !ok {
		t.Fatalf("missing key %q in payload %+v", key, payload)
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		t.Fatalf("expected numeric field %q, got %T", key, value)
		return 0
	}
}

func mapField(t *testing.T, payload map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := payload[key]
	if !ok {
		t.Fatalf("missing key %q in payload %+v", key, payload)
	}
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map field %q, got %T", key, value)
	}
	return out
}

func sliceField(t *testing.T, payload map[string]any, key string) []any {
	t.Helper()
	value, ok := payload[key]
	if !ok {
		t.Fatalf("missing key %q in payload %+v", key, payload)
	}
	out, ok := value.([]any)
	if !ok {
		t.Fatalf("expected slice field %q, got %T", key, value)
	}
	return out
}

func findMapByKeyValue(t *testing.T, items []any, key string, expected string) map[string]any {
	t.Helper()
	for _, item := range items {
		asMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		value, _ := asMap[key].(string)
		if value == expected {
			return asMap
		}
	}
	t.Fatalf("item with %s=%s not found in %+v", key, expected, items)
	return nil
}

func findEdgeMetric(t *testing.T, items []any, fromAgent string, toAgent string) map[string]any {
	t.Helper()
	for _, item := range items {
		asMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		from, _ := asMap["from_agent"].(string)
		to, _ := asMap["to_agent"].(string)
		if from == fromAgent && to == toAgent {
			return asMap
		}
	}
	t.Fatalf("edge metric %s->%s not found in %+v", fromAgent, toAgent, items)
	return nil
}
