package controllers

import (
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

func TestTaskScheduleControllerCreatesRunAndIsIdempotent(t *testing.T) {
	taskStore := store.NewTaskStore()
	taskScheduleStore := store.NewTaskScheduleStore()
	logger := log.New(io.Discard, "", 0)

	template := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "report-template", Namespace: "team-a"},
		Spec: resources.TaskSpec{
			Mode:     "template",
			System:   "report-system",
			Priority: "normal",
			Input:    map[string]string{"topic": "weekly"},
		},
	}
	if _, err := taskStore.Upsert(template); err != nil {
		t.Fatalf("upsert template failed: %v", err)
	}

	schedule := resources.TaskSchedule{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskSchedule",
		Metadata:   resources.ObjectMeta{Name: "weekly-report", Namespace: "team-a"},
		Spec: resources.TaskScheduleSpec{
			TaskRef:           "report-template",
			Schedule:          "* * * * *",
			TimeZone:          "UTC",
			Suspend:           false,
			ConcurrencyPolicy: "forbid",
		},
		Status: resources.TaskScheduleStatus{
			LastScheduleTime: time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano),
		},
	}
	if _, err := taskScheduleStore.Upsert(schedule); err != nil {
		t.Fatalf("upsert schedule failed: %v", err)
	}

	controller := NewTaskScheduleController(taskScheduleStore, taskStore, logger, 5*time.Millisecond)
	if err := controller.ReconcileOnce(); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if err := controller.ReconcileOnce(); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	tasks := taskStore.List()
	generated := 0
	for _, task := range tasks {
		if task.Metadata.Labels[taskScheduleNameLabel] != "weekly-report" {
			continue
		}
		generated++
		if task.Spec.Mode != "run" {
			t.Fatalf("expected generated task mode=run, got %q", task.Spec.Mode)
		}
		if task.Spec.System != "report-system" {
			t.Fatalf("expected generated task system=report-system, got %q", task.Spec.System)
		}
	}
	if generated != 1 {
		t.Fatalf("expected exactly one generated task, got %d", generated)
	}
}

func TestTaskScheduleControllerForbidOverlapSkipsNewRun(t *testing.T) {
	taskStore := store.NewTaskStore()
	taskScheduleStore := store.NewTaskScheduleStore()
	logger := log.New(io.Discard, "", 0)

	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "tmpl", Namespace: "default"},
		Spec:       resources.TaskSpec{Mode: "template", System: "sys"},
	}); err != nil {
		t.Fatalf("upsert template failed: %v", err)
	}

	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Name:      "existing-run",
			Namespace: "default",
			Labels: map[string]string{
				taskScheduleNameLabel:      "overlap",
				taskScheduleNamespaceLabel: "default",
				taskScheduleSlotLabel:      time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano),
			},
		},
		Spec:   resources.TaskSpec{Mode: "run", System: "sys"},
		Status: resources.TaskStatus{Phase: "Running"},
	}); err != nil {
		t.Fatalf("upsert existing run failed: %v", err)
	}

	if _, err := taskScheduleStore.Upsert(resources.TaskSchedule{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskSchedule",
		Metadata:   resources.ObjectMeta{Name: "overlap", Namespace: "default"},
		Spec: resources.TaskScheduleSpec{
			TaskRef:                 "tmpl",
			Schedule:                "* * * * *",
			TimeZone:                "UTC",
			ConcurrencyPolicy:       "forbid",
			StartingDeadlineSeconds: 300,
		},
		Status: resources.TaskScheduleStatus{
			LastScheduleTime: time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("upsert schedule failed: %v", err)
	}

	controller := NewTaskScheduleController(taskScheduleStore, taskStore, logger, 5*time.Millisecond)
	if err := controller.ReconcileOnce(); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	generated := 0
	for _, task := range taskStore.List() {
		if task.Metadata.Labels[taskScheduleNameLabel] == "overlap" {
			generated++
		}
	}
	if generated != 1 {
		t.Fatalf("expected no new run due to overlap policy, got %d generated tasks", generated)
	}
}

func TestTaskScheduleControllerMissedDeadlineSkipsRun(t *testing.T) {
	taskStore := store.NewTaskStore()
	taskScheduleStore := store.NewTaskScheduleStore()
	logger := log.New(io.Discard, "", 0)

	if _, err := taskStore.Upsert(resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "tmpl", Namespace: "default"},
		Spec:       resources.TaskSpec{Mode: "template", System: "sys"},
	}); err != nil {
		t.Fatalf("upsert template failed: %v", err)
	}
	if _, err := taskScheduleStore.Upsert(resources.TaskSchedule{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskSchedule",
		Metadata:   resources.ObjectMeta{Name: "deadline", Namespace: "default"},
		Spec: resources.TaskScheduleSpec{
			TaskRef:                 "tmpl",
			Schedule:                "* * * * *",
			TimeZone:                "UTC",
			StartingDeadlineSeconds: 1,
		},
		Status: resources.TaskScheduleStatus{
			LastScheduleTime: time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("upsert schedule failed: %v", err)
	}

	controller := NewTaskScheduleController(taskScheduleStore, taskStore, logger, 5*time.Millisecond)
	if err := controller.ReconcileOnce(); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	for _, task := range taskStore.List() {
		if task.Metadata.Labels[taskScheduleNameLabel] == "deadline" {
			t.Fatalf("expected no generated run on missed deadline, found %s", task.Metadata.Name)
		}
	}
}

func TestTaskScheduleControllerRetentionPrunesHistory(t *testing.T) {
	taskStore := store.NewTaskStore()
	taskScheduleStore := store.NewTaskScheduleStore()
	logger := log.New(io.Discard, "", 0)

	schedule := resources.TaskSchedule{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskSchedule",
		Metadata:   resources.ObjectMeta{Name: "retention", Namespace: "default"},
		Spec: resources.TaskScheduleSpec{
			TaskRef:                "tmpl",
			Schedule:               "* * * * *",
			TimeZone:               "UTC",
			SuccessfulHistoryLimit: 1,
			FailedHistoryLimit:     1,
		},
		Status: resources.TaskScheduleStatus{
			LastScheduleTime: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}
	if _, err := taskScheduleStore.Upsert(schedule); err != nil {
		t.Fatalf("upsert schedule failed: %v", err)
	}

	now := time.Now().UTC()
	cases := []struct {
		name      string
		phase     string
		completed time.Time
	}{
		{name: "s-old", phase: "Succeeded", completed: now.Add(-4 * time.Minute)},
		{name: "s-new", phase: "Succeeded", completed: now.Add(-3 * time.Minute)},
		{name: "f-old", phase: "Failed", completed: now.Add(-2 * time.Minute)},
		{name: "f-new", phase: "DeadLetter", completed: now.Add(-1 * time.Minute)},
	}
	for _, tc := range cases {
		if _, err := taskStore.Upsert(resources.Task{
			APIVersion: "orloj.dev/v1",
			Kind:       "Task",
			Metadata: resources.ObjectMeta{
				Name:      tc.name,
				Namespace: "default",
				Labels: map[string]string{
					taskScheduleNameLabel:      "retention",
					taskScheduleNamespaceLabel: "default",
					taskScheduleSlotLabel:      tc.completed.Format(time.RFC3339Nano),
				},
			},
			Spec: resources.TaskSpec{Mode: "run", System: "sys"},
			Status: resources.TaskStatus{
				Phase:       tc.phase,
				CompletedAt: tc.completed.Format(time.RFC3339Nano),
			},
		}); err != nil {
			t.Fatalf("upsert %s failed: %v", tc.name, err)
		}
	}

	controller := NewTaskScheduleController(taskScheduleStore, taskStore, logger, 5*time.Millisecond)
	if err := controller.ReconcileOnce(); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	remaining := map[string]struct{}{}
	for _, task := range taskStore.List() {
		if task.Metadata.Labels[taskScheduleNameLabel] == "retention" {
			remaining[task.Metadata.Name] = struct{}{}
		}
	}
	if _, ok := remaining["s-new"]; !ok {
		t.Fatalf("expected s-new retained, got %#v", remaining)
	}
	if _, ok := remaining["f-new"]; !ok {
		t.Fatalf("expected f-new retained, got %#v", remaining)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 retained runs, got %d", len(remaining))
	}
}

func TestTaskScheduleEnsureRunUsesLatestTemplateSpec(t *testing.T) {
	taskStore := store.NewTaskStore()
	taskScheduleStore := store.NewTaskScheduleStore()
	logger := log.New(io.Discard, "", 0)

	template := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "tmpl", Namespace: "default"},
		Spec:       resources.TaskSpec{Mode: "template", System: "sys-v1"},
	}
	if _, err := taskStore.Upsert(template); err != nil {
		t.Fatalf("upsert template failed: %v", err)
	}

	controller := NewTaskScheduleController(taskScheduleStore, taskStore, logger, 5*time.Millisecond)
	schedule := resources.TaskSchedule{
		Metadata: resources.ObjectMeta{Name: "latest", Namespace: "default"},
		Spec: resources.TaskScheduleSpec{
			TaskRef:  "tmpl",
			Schedule: "* * * * *",
			TimeZone: "UTC",
		},
	}

	slot1 := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Minute)
	run1, err := controller.ensureRunTask(schedule, slot1)
	if err != nil {
		t.Fatalf("ensure run v1 failed: %v", err)
	}

	template.Spec.System = "sys-v2"
	if _, err := taskStore.Upsert(template); err != nil {
		t.Fatalf("update template failed: %v", err)
	}
	slot2 := slot1.Add(time.Minute)
	run2, err := controller.ensureRunTask(schedule, slot2)
	if err != nil {
		t.Fatalf("ensure run v2 failed: %v", err)
	}
	if run1 == run2 {
		t.Fatalf("expected distinct run keys, got %s", run1)
	}

	task1, ok := taskStore.Get(run1)
	if !ok {
		t.Fatalf("run1 not found")
	}
	task2, ok := taskStore.Get(run2)
	if !ok {
		t.Fatalf("run2 not found")
	}
	if task1.Spec.System != "sys-v1" {
		t.Fatalf("expected run1 system sys-v1, got %q", task1.Spec.System)
	}
	if task2.Spec.System != "sys-v2" {
		t.Fatalf("expected run2 system sys-v2, got %q", task2.Spec.System)
	}
}

func TestResolveTaskRef(t *testing.T) {
	ns, name, err := resolveTaskRef("team-a", "task-x")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if ns != "team-a" || name != "task-x" {
		t.Fatalf("unexpected ref resolution %s/%s", ns, name)
	}

	ns, name, err = resolveTaskRef("team-a", "team-b/task-y")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if ns != "team-b" || name != "task-y" {
		t.Fatalf("unexpected ref resolution %s/%s", ns, name)
	}

	if _, _, err := resolveTaskRef("team-a", "team-b/"); err == nil {
		t.Fatal("expected invalid ref error")
	}
}

func TestScheduledTaskNameSanitizes(t *testing.T) {
	name := scheduledTaskName("Weekly Report!", time.Date(2026, 3, 13, 10, 15, 0, 0, time.UTC))
	if !strings.HasPrefix(name, "weekly-report-") {
		t.Fatalf("unexpected task name %q", name)
	}
}
