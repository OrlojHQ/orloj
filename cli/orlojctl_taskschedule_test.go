package cli

import "testing"

func TestNormalizeResourceTaskSchedule(t *testing.T) {
	if got := normalizeResource("taskschedule"); got != "task-schedules" {
		t.Fatalf("expected task-schedules, got %q", got)
	}
	if got := normalizeResource("task-schedules"); got != "task-schedules" {
		t.Fatalf("expected task-schedules, got %q", got)
	}
}

func TestListEndpointForTaskSchedule(t *testing.T) {
	endpoint, err := listEndpointForResource("task-schedules")
	if err != nil {
		t.Fatalf("listEndpointForResource returned error: %v", err)
	}
	if endpoint != "/v1/task-schedules" {
		t.Fatalf("unexpected endpoint %q", endpoint)
	}
}

func TestParseApplyPayloadTaskSchedule(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: TaskSchedule
metadata:
  name: hourly
spec:
  task_ref: template-task
  schedule: "0 * * * *"
  time_zone: UTC
`)
	endpoint, payload, name, err := buildApplyRequest("TaskSchedule", raw)
	if err != nil {
		t.Fatalf("parseApplyPayload failed: %v", err)
	}
	if endpoint != "/v1/task-schedules" {
		t.Fatalf("unexpected endpoint %q", endpoint)
	}
	if name != "hourly" {
		t.Fatalf("unexpected resource name %q", name)
	}
	if len(payload) == 0 {
		t.Fatal("expected non-empty payload")
	}
}
