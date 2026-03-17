package resources

import "testing"

func TestTaskNormalizeModeDefaultsAndValidation(t *testing.T) {
	task := Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   ObjectMeta{Name: "t1"},
		Spec:       TaskSpec{System: "sys"},
	}
	if err := task.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if task.Spec.Mode != "run" {
		t.Fatalf("expected default mode run, got %q", task.Spec.Mode)
	}

	task.Spec.Mode = "template"
	if err := task.Normalize(); err != nil {
		t.Fatalf("normalize template mode failed: %v", err)
	}
	if task.Spec.Mode != "template" {
		t.Fatalf("expected template mode, got %q", task.Spec.Mode)
	}

	task.Spec.Mode = "bad"
	if err := task.Normalize(); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestTaskScheduleNormalizeDefaultsAndValidation(t *testing.T) {
	s := TaskSchedule{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskSchedule",
		Metadata:   ObjectMeta{Name: "sched"},
		Spec: TaskScheduleSpec{
			TaskRef:  "template-task",
			Schedule: "*/5 * * * *",
			TimeZone: "America/Chicago",
			Suspend:  false,
		},
	}
	if err := s.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if s.Spec.ConcurrencyPolicy != "forbid" {
		t.Fatalf("expected default policy forbid, got %q", s.Spec.ConcurrencyPolicy)
	}
	if s.Spec.StartingDeadlineSeconds != 300 {
		t.Fatalf("expected default deadline 300, got %d", s.Spec.StartingDeadlineSeconds)
	}
	if s.Spec.SuccessfulHistoryLimit != 10 {
		t.Fatalf("expected default success limit 10, got %d", s.Spec.SuccessfulHistoryLimit)
	}
	if s.Spec.FailedHistoryLimit != 3 {
		t.Fatalf("expected default fail limit 3, got %d", s.Spec.FailedHistoryLimit)
	}
}

func TestTaskScheduleNormalizeRejectsInvalidFields(t *testing.T) {
	cases := []TaskSchedule{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "TaskSchedule",
			Metadata:   ObjectMeta{Name: "bad-ref"},
			Spec:       TaskScheduleSpec{TaskRef: "default/", Schedule: "* * * * *", TimeZone: "UTC"},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "TaskSchedule",
			Metadata:   ObjectMeta{Name: "bad-cron"},
			Spec:       TaskScheduleSpec{TaskRef: "tmpl", Schedule: "* * * *", TimeZone: "UTC"},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "TaskSchedule",
			Metadata:   ObjectMeta{Name: "bad-tz"},
			Spec:       TaskScheduleSpec{TaskRef: "tmpl", Schedule: "* * * * *", TimeZone: "Mars/Colony"},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "TaskSchedule",
			Metadata:   ObjectMeta{Name: "bad-policy"},
			Spec:       TaskScheduleSpec{TaskRef: "tmpl", Schedule: "* * * * *", TimeZone: "UTC", ConcurrencyPolicy: "allow"},
		},
	}
	for _, tc := range cases {
		item := tc
		if err := item.Normalize(); err == nil {
			t.Fatalf("expected normalize error for %s", item.Metadata.Name)
		}
	}
}

func TestParseTaskScheduleManifestYAML(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: TaskSchedule
metadata:
  name: hourly-report
spec:
  task_ref: report-template
  schedule: "0 * * * *"
  time_zone: UTC
  suspend: false
  starting_deadline_seconds: 120
  concurrency_policy: forbid
  successful_history_limit: 7
  failed_history_limit: 2
`)
	item, err := ParseTaskScheduleManifest(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if item.Spec.TaskRef != "report-template" {
		t.Fatalf("expected task_ref=report-template, got %q", item.Spec.TaskRef)
	}
	if item.Spec.Schedule != "0 * * * *" {
		t.Fatalf("expected schedule 0 * * * *, got %q", item.Spec.Schedule)
	}
	if item.Spec.TimeZone != "UTC" {
		t.Fatalf("expected timezone UTC, got %q", item.Spec.TimeZone)
	}
	if item.Spec.StartingDeadlineSeconds != 120 {
		t.Fatalf("expected deadline 120, got %d", item.Spec.StartingDeadlineSeconds)
	}
}

func TestTaskWebhookNormalizeDefaultsAndValidation(t *testing.T) {
	hook := TaskWebhook{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskWebhook",
		Metadata:   ObjectMeta{Name: "github-hook"},
		Spec: TaskWebhookSpec{
			TaskRef: "report-template",
			Auth: TaskWebhookAuthSpec{
				Profile:   "github",
				SecretRef: "github-secret",
			},
		},
	}
	if err := hook.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if hook.Spec.Auth.SignatureHeader != "X-Hub-Signature-256" {
		t.Fatalf("expected github signature header default, got %q", hook.Spec.Auth.SignatureHeader)
	}
	if hook.Spec.Idempotency.EventIDHeader != "X-GitHub-Delivery" {
		t.Fatalf("expected github event id header default, got %q", hook.Spec.Idempotency.EventIDHeader)
	}
	if hook.Spec.Payload.Mode != "raw" {
		t.Fatalf("expected payload mode raw, got %q", hook.Spec.Payload.Mode)
	}
	if hook.Spec.Payload.InputKey != "webhook_payload" {
		t.Fatalf("expected payload input key webhook_payload, got %q", hook.Spec.Payload.InputKey)
	}
	if hook.Spec.Idempotency.DedupeWindowSeconds != 86400 {
		t.Fatalf("expected default dedupe window 86400, got %d", hook.Spec.Idempotency.DedupeWindowSeconds)
	}
	if hook.Status.EndpointID == "" {
		t.Fatal("expected endpointID to be populated")
	}
	if hook.Status.EndpointPath == "" {
		t.Fatal("expected endpointPath to be populated")
	}
}

func TestTaskWebhookNormalizeRejectsInvalidFields(t *testing.T) {
	cases := []TaskWebhook{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "TaskWebhook",
			Metadata:   ObjectMeta{Name: "bad-ref"},
			Spec: TaskWebhookSpec{
				TaskRef: "default/",
				Auth: TaskWebhookAuthSpec{
					SecretRef: "hook-secret",
				},
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "TaskWebhook",
			Metadata:   ObjectMeta{Name: "bad-profile"},
			Spec: TaskWebhookSpec{
				TaskRef: "tmpl",
				Auth: TaskWebhookAuthSpec{
					Profile:   "stripe",
					SecretRef: "hook-secret",
				},
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "TaskWebhook",
			Metadata:   ObjectMeta{Name: "missing-secret"},
			Spec: TaskWebhookSpec{
				TaskRef: "tmpl",
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "TaskWebhook",
			Metadata:   ObjectMeta{Name: "bad-window"},
			Spec: TaskWebhookSpec{
				TaskRef: "tmpl",
				Auth: TaskWebhookAuthSpec{
					SecretRef: "hook-secret",
				},
				Idempotency: TaskWebhookIdempotency{
					DedupeWindowSeconds: -1,
				},
			},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "TaskWebhook",
			Metadata:   ObjectMeta{Name: "bad-payload"},
			Spec: TaskWebhookSpec{
				TaskRef: "tmpl",
				Auth: TaskWebhookAuthSpec{
					SecretRef: "hook-secret",
				},
				Payload: TaskWebhookPayloadSpec{
					Mode: "mapped",
				},
			},
		},
	}
	for _, tc := range cases {
		item := tc
		if err := item.Normalize(); err == nil {
			t.Fatalf("expected normalize error for %s", item.Metadata.Name)
		}
	}
}

func TestParseTaskWebhookManifestYAML(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: TaskWebhook
metadata:
  name: github-report
spec:
  task_ref: report-template
  suspend: false
  auth:
    profile: github
    secret_ref: github-secret
  idempotency:
    dedupe_window_seconds: 300
  payload:
    mode: raw
    input_key: webhook_payload
`)
	item, err := ParseTaskWebhookManifest(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if item.Spec.TaskRef != "report-template" {
		t.Fatalf("expected task_ref=report-template, got %q", item.Spec.TaskRef)
	}
	if item.Spec.Auth.Profile != "github" {
		t.Fatalf("expected profile github, got %q", item.Spec.Auth.Profile)
	}
	if item.Spec.Auth.SecretRef != "github-secret" {
		t.Fatalf("expected secret_ref github-secret, got %q", item.Spec.Auth.SecretRef)
	}
	if item.Spec.Idempotency.DedupeWindowSeconds != 300 {
		t.Fatalf("expected dedupe window 300, got %d", item.Spec.Idempotency.DedupeWindowSeconds)
	}
}
