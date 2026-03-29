package cli

import "testing"

func TestNormalizeResourceTaskWebhook(t *testing.T) {
	if got := normalizeResource("taskwebhook"); got != "task-webhooks" {
		t.Fatalf("expected task-webhooks, got %q", got)
	}
	if got := normalizeResource("task-webhooks"); got != "task-webhooks" {
		t.Fatalf("expected task-webhooks, got %q", got)
	}
}

func TestListEndpointForTaskWebhook(t *testing.T) {
	endpoint, err := listEndpointForResource("task-webhooks")
	if err != nil {
		t.Fatalf("listEndpointForResource returned error: %v", err)
	}
	if endpoint != "/v1/task-webhooks" {
		t.Fatalf("unexpected endpoint %q", endpoint)
	}
}

func TestParseApplyPayloadTaskWebhook(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: TaskWebhook
metadata:
  name: github-report
spec:
  task_ref: report-template
  auth:
    profile: github
    secret_ref: github-secret
`)
	endpoint, payload, name, err := buildApplyRequest("TaskWebhook", raw)
	if err != nil {
		t.Fatalf("parseApplyPayload failed: %v", err)
	}
	if endpoint != "/v1/task-webhooks" {
		t.Fatalf("unexpected endpoint %q", endpoint)
	}
	if name != "github-report" {
		t.Fatalf("unexpected resource name %q", name)
	}
	if len(payload) == 0 {
		t.Fatal("expected non-empty payload")
	}
}
