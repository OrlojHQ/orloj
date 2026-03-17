package resources

import "testing"

func TestParseTaskManifestMessageRetryYAML(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: Task
metadata:
  name: retry-task
spec:
  system: report-system
  retry:
    max_attempts: 7
    backoff: 5s
  message_retry:
    max_attempts: 3
    backoff: 120ms
    max_backoff: 2s
    jitter: equal
    non_retryable:
      - policy
      - agentsystem
`)

	task, err := ParseTaskManifest(raw)
	if err != nil {
		t.Fatalf("parse task manifest failed: %v", err)
	}
	if task.Spec.MessageRetry.MaxAttempts != 3 {
		t.Fatalf("expected message_retry.max_attempts=3, got %d", task.Spec.MessageRetry.MaxAttempts)
	}
	if task.Spec.MessageRetry.Backoff != "120ms" {
		t.Fatalf("expected message_retry.backoff=120ms, got %q", task.Spec.MessageRetry.Backoff)
	}
	if task.Spec.MessageRetry.MaxBackoff != "2s" {
		t.Fatalf("expected message_retry.max_backoff=2s, got %q", task.Spec.MessageRetry.MaxBackoff)
	}
	if task.Spec.MessageRetry.Jitter != "equal" {
		t.Fatalf("expected message_retry.jitter=equal, got %q", task.Spec.MessageRetry.Jitter)
	}
	if len(task.Spec.MessageRetry.NonRetryable) != 2 {
		t.Fatalf("expected message_retry.non_retryable length=2, got %d", len(task.Spec.MessageRetry.NonRetryable))
	}
}

func TestTaskNormalizeMessageRetryDefaultsFromTaskRetry(t *testing.T) {
	task := Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   ObjectMeta{Name: "defaults-task"},
		Spec: TaskSpec{
			System: "report-system",
			Retry: TaskRetryPolicy{
				MaxAttempts: 4,
				Backoff:     "2s",
			},
		},
	}

	if err := task.Normalize(); err != nil {
		t.Fatalf("normalize task failed: %v", err)
	}
	if task.Spec.MessageRetry.MaxAttempts != 4 {
		t.Fatalf("expected message_retry.max_attempts default=4, got %d", task.Spec.MessageRetry.MaxAttempts)
	}
	if task.Spec.MessageRetry.Backoff != "2s" {
		t.Fatalf("expected message_retry.backoff default=2s, got %q", task.Spec.MessageRetry.Backoff)
	}
	if task.Spec.MessageRetry.MaxBackoff != "24h" {
		t.Fatalf("expected message_retry.max_backoff default=24h, got %q", task.Spec.MessageRetry.MaxBackoff)
	}
	if task.Spec.MessageRetry.Jitter != "full" {
		t.Fatalf("expected message_retry.jitter default=full, got %q", task.Spec.MessageRetry.Jitter)
	}
}

func TestParseTaskManifestMaxTurnsYAML(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: Task
metadata:
  name: cycle-task
spec:
  system: cycle-system
  max_turns: 6
`)

	task, err := ParseTaskManifest(raw)
	if err != nil {
		t.Fatalf("parse task manifest failed: %v", err)
	}
	if task.Spec.MaxTurns != 6 {
		t.Fatalf("expected max_turns=6, got %d", task.Spec.MaxTurns)
	}
}

func TestTaskNormalizeRejectsNegativeMaxTurns(t *testing.T) {
	task := Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   ObjectMeta{Name: "cycle-task"},
		Spec: TaskSpec{
			System:   "cycle-system",
			MaxTurns: -1,
		},
	}

	if err := task.Normalize(); err == nil {
		t.Fatal("expected normalize to reject negative max_turns")
	}
}
