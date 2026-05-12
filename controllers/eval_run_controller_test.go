package controllers

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

func newEvalRunControllerHarness(t *testing.T) (*EvalRunController, *store.EvalRunStore, *store.EvalDatasetStore, *store.TaskStore) {
	t.Helper()
	evalRuns := store.NewEvalRunStore()
	evalDatasets := store.NewEvalDatasetStore()
	tasks := store.NewTaskStore()
	scorer := &agentruntime.EvalScorer{}
	logger := log.New(io.Discard, "", 0)
	c := NewEvalRunController(evalRuns, evalDatasets, tasks, scorer, logger)
	c.reconcileEvery = 10 * time.Millisecond
	return c, evalRuns, evalDatasets, tasks
}

func seedDatasetForController(t *testing.T, datasets *store.EvalDatasetStore) {
	t.Helper()
	ctx := context.Background()
	ds := resources.EvalDataset{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalDataset",
		Metadata:   resources.ObjectMeta{Name: "golden", Namespace: "default"},
		Spec: resources.EvalDatasetSpec{
			Samples: []resources.EvalSample{
				{Name: "s1", Input: map[string]string{"prompt": "hello"}, Expected: resources.EvalExpected{OutputContains: "hi"}},
				{Name: "s2", Input: map[string]string{"prompt": "bye"}, Expected: resources.EvalExpected{OutputContains: "goodbye"}},
			},
		},
	}
	if _, err := datasets.Upsert(ctx, ds); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}
}

func seedPendingEvalRun(t *testing.T, runs *store.EvalRunStore) {
	t.Helper()
	ctx := context.Background()
	run := resources.EvalRun{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalRun",
		Metadata:   resources.ObjectMeta{Name: "eval-1", Namespace: "default"},
		Spec: resources.EvalRunSpec{
			DatasetRef:  "golden",
			System:      "test-system",
			Scoring:     resources.EvalScoringConfig{Strategy: "exact_match"},
			Concurrency: 2,
		},
	}
	if _, err := runs.Upsert(ctx, run); err != nil {
		t.Fatalf("seed eval run: %v", err)
	}
}

func TestEvalRunController_ReconcilePending_CreatesTasksAndTransitions(t *testing.T) {
	ctx := context.Background()
	c, runs, datasets, tasks := newEvalRunControllerHarness(t)

	seedDatasetForController(t, datasets)
	seedPendingEvalRun(t, runs)

	if err := c.ReconcileOnce(ctx); err != nil {
		t.Fatal(err)
	}

	run, ok, _ := runs.Get(ctx, "default/eval-1")
	if !ok {
		t.Fatal("eval run not found after reconcile")
	}
	if run.Status.Phase != resources.EvalRunPhaseRunning {
		t.Fatalf("expected Running, got %q", run.Status.Phase)
	}
	if run.Status.TotalSamples != 2 {
		t.Fatalf("expected 2 total samples, got %d", run.Status.TotalSamples)
	}
	if len(run.Status.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(run.Status.Results))
	}

	allTasks, _ := tasks.List(ctx)
	if len(allTasks) != 2 {
		t.Fatalf("expected 2 tasks created, got %d", len(allTasks))
	}

	taskMap := make(map[string]resources.Task, len(allTasks))
	for _, task := range allTasks {
		taskMap[task.Metadata.Name] = task
	}
	for _, result := range run.Status.Results {
		task, ok := taskMap[result.TaskName]
		if !ok {
			t.Fatalf("expected task %q to exist", result.TaskName)
		}
		if task.Spec.System != "test-system" {
			t.Fatalf("expected system test-system, got %q", task.Spec.System)
		}
		if task.Metadata.Labels["orloj.dev/eval-run"] != "eval-1" {
			t.Fatalf("expected eval-run label, got %v", task.Metadata.Labels)
		}
	}
}

func TestEvalRunController_ReconcilePending_SkipsSuspendedRuns(t *testing.T) {
	ctx := context.Background()
	c, runs, datasets, tasks := newEvalRunControllerHarness(t)

	seedDatasetForController(t, datasets)

	run := resources.EvalRun{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalRun",
		Metadata:   resources.ObjectMeta{Name: "suspended-run", Namespace: "default"},
		Spec: resources.EvalRunSpec{
			DatasetRef:  "golden",
			System:      "test-system",
			Scoring:     resources.EvalScoringConfig{Strategy: "exact_match"},
			Concurrency: 2,
			Suspended:   true,
		},
	}
	if _, err := runs.Upsert(ctx, run); err != nil {
		t.Fatalf("seed suspended eval run: %v", err)
	}

	if err := c.ReconcileOnce(ctx); err != nil {
		t.Fatal(err)
	}

	got, ok, _ := runs.Get(ctx, "default/suspended-run")
	if !ok {
		t.Fatal("eval run not found after reconcile")
	}
	if got.Status.Phase == resources.EvalRunPhaseRunning {
		t.Fatal("suspended eval run should not have transitioned to Running")
	}

	allTasks, _ := tasks.List(ctx)
	if len(allTasks) != 0 {
		t.Fatalf("expected 0 tasks for suspended run, got %d", len(allTasks))
	}
}

func TestEvalRunController_ReconcilePending_MissingDataset(t *testing.T) {
	ctx := context.Background()
	c, runs, _, _ := newEvalRunControllerHarness(t)

	seedPendingEvalRun(t, runs)

	_ = c.ReconcileOnce(ctx)

	run, ok, _ := runs.Get(ctx, "default/eval-1")
	if !ok {
		t.Fatal("eval run not found")
	}
	if run.Status.Phase != resources.EvalRunPhaseFailed {
		t.Fatalf("expected Failed for missing dataset, got %q", run.Status.Phase)
	}
}

func TestEvalRunController_ReconcileRunning_CollectsOutputAndTransitions(t *testing.T) {
	ctx := context.Background()
	c, runs, datasets, tasks := newEvalRunControllerHarness(t)

	seedDatasetForController(t, datasets)
	seedPendingEvalRun(t, runs)
	_ = c.ReconcileOnce(ctx)

	allTasks, _ := tasks.List(ctx)
	for i := range allTasks {
		allTasks[i].Status.Phase = "Succeeded"
		allTasks[i].Status.Output = map[string]string{"result": "hi there, goodbye"}
		allTasks[i].Status.StartedAt = time.Now().Add(-2 * time.Second).UTC().Format(time.RFC3339)
		allTasks[i].Status.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		tasks.Upsert(ctx, allTasks[i])
	}

	_ = c.ReconcileOnce(ctx)

	run, _, _ := runs.Get(ctx, "default/eval-1")
	if run.Status.Phase != resources.EvalRunPhaseScoring {
		t.Fatalf("expected Scoring, got %q", run.Status.Phase)
	}
	for _, result := range run.Status.Results {
		if result.Output == "" {
			t.Fatalf("expected output for %s, got empty", result.SampleName)
		}
	}
}

func TestEvalRunController_ReconcileRunning_StaysRunningWhileTasksPending(t *testing.T) {
	ctx := context.Background()
	c, runs, datasets, tasks := newEvalRunControllerHarness(t)

	seedDatasetForController(t, datasets)
	seedPendingEvalRun(t, runs)
	_ = c.ReconcileOnce(ctx)

	allTasks, _ := tasks.List(ctx)
	allTasks[0].Status.Phase = "Succeeded"
	allTasks[0].Status.Output = map[string]string{"result": "done"}
	tasks.Upsert(ctx, allTasks[0])

	_ = c.ReconcileOnce(ctx)

	run, _, _ := runs.Get(ctx, "default/eval-1")
	if run.Status.Phase != resources.EvalRunPhaseRunning {
		t.Fatalf("expected Running while tasks still pending, got %q", run.Status.Phase)
	}
	if run.Status.CompletedSamples != 1 {
		t.Fatalf("expected 1 completed sample during running phase, got %d", run.Status.CompletedSamples)
	}
}

func TestEvalRunController_ReconcileRunning_FailedTaskRecordsError(t *testing.T) {
	ctx := context.Background()
	c, runs, datasets, tasks := newEvalRunControllerHarness(t)

	seedDatasetForController(t, datasets)
	seedPendingEvalRun(t, runs)
	_ = c.ReconcileOnce(ctx)

	allTasks, _ := tasks.List(ctx)
	for i := range allTasks {
		allTasks[i].Status.Phase = "Failed"
		allTasks[i].Status.LastError = "model timeout"
		tasks.Upsert(ctx, allTasks[i])
	}

	_ = c.ReconcileOnce(ctx)

	run, _, _ := runs.Get(ctx, "default/eval-1")
	if run.Status.Phase != resources.EvalRunPhaseScoring {
		t.Fatalf("expected Scoring, got %q", run.Status.Phase)
	}
	for _, r := range run.Status.Results {
		if r.Error == "" {
			t.Fatalf("expected error for failed task %s", r.SampleName)
		}
	}
}

func TestEvalRunController_ReconcileRunning_DeadLetterTaskRecordsError(t *testing.T) {
	ctx := context.Background()
	c, runs, datasets, tasks := newEvalRunControllerHarness(t)

	seedDatasetForController(t, datasets)
	seedPendingEvalRun(t, runs)
	_ = c.ReconcileOnce(ctx)

	allTasks, _ := tasks.List(ctx)
	for i := range allTasks {
		allTasks[i].Status.Phase = "DeadLetter"
		allTasks[i].Status.LastError = "message dead-lettered after 1 attempts: model request failed status=401"
		allTasks[i].Status.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		tasks.Upsert(ctx, allTasks[i])
	}

	_ = c.ReconcileOnce(ctx)

	run, _, _ := runs.Get(ctx, "default/eval-1")
	if run.Status.Phase != resources.EvalRunPhaseScoring {
		t.Fatalf("expected Scoring after all tasks dead-lettered, got %q", run.Status.Phase)
	}
	for _, r := range run.Status.Results {
		if r.Error == "" {
			t.Fatalf("expected error for dead-lettered task %s", r.SampleName)
		}
	}
}

func TestEvalRunController_ReconcileRunning_MixedSucceededAndDeadLetter(t *testing.T) {
	ctx := context.Background()
	c, runs, datasets, tasks := newEvalRunControllerHarness(t)

	seedDatasetForController(t, datasets)
	seedPendingEvalRun(t, runs)
	_ = c.ReconcileOnce(ctx)

	allTasks, _ := tasks.List(ctx)
	allTasks[0].Status.Phase = "Succeeded"
	allTasks[0].Status.Output = map[string]string{"result": "done"}
	allTasks[0].Status.StartedAt = time.Now().Add(-2 * time.Second).UTC().Format(time.RFC3339)
	allTasks[0].Status.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	tasks.Upsert(ctx, allTasks[0])

	allTasks[1].Status.Phase = "DeadLetter"
	allTasks[1].Status.LastError = "model request failed status=401"
	allTasks[1].Status.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	tasks.Upsert(ctx, allTasks[1])

	_ = c.ReconcileOnce(ctx)

	run, _, _ := runs.Get(ctx, "default/eval-1")
	if run.Status.Phase != resources.EvalRunPhaseScoring {
		t.Fatalf("expected Scoring after mixed succeeded/dead-lettered, got %q", run.Status.Phase)
	}

	hasOutput := false
	hasError := false
	for _, r := range run.Status.Results {
		if r.Output != "" {
			hasOutput = true
		}
		if r.Error != "" {
			hasError = true
		}
	}
	if !hasOutput {
		t.Fatal("expected at least one result with output from succeeded task")
	}
	if !hasError {
		t.Fatal("expected at least one result with error from dead-lettered task")
	}
}

func TestEvalRunController_ReconcileScoring_ExactMatchScores(t *testing.T) {
	ctx := context.Background()
	c, runs, datasets, tasks := newEvalRunControllerHarness(t)

	seedDatasetForController(t, datasets)
	seedPendingEvalRun(t, runs)
	_ = c.ReconcileOnce(ctx)

	allTasks, _ := tasks.List(ctx)
	for i := range allTasks {
		allTasks[i].Status.Phase = "Succeeded"
		allTasks[i].Status.Output = map[string]string{"result": "hi there, goodbye"}
		tasks.Upsert(ctx, allTasks[i])
	}
	_ = c.ReconcileOnce(ctx) // Running -> Scoring

	run, _, _ := runs.Get(ctx, "default/eval-1")
	if run.Status.Phase != resources.EvalRunPhaseScoring {
		t.Fatalf("expected Scoring, got %q", run.Status.Phase)
	}

	_ = c.ReconcileOnce(ctx) // Scoring -> Succeeded

	run, _, _ = runs.Get(ctx, "default/eval-1")
	if run.Status.Phase != resources.EvalRunPhaseSucceeded {
		t.Fatalf("expected Succeeded, got %q", run.Status.Phase)
	}

	for _, r := range run.Status.Results {
		if r.Score == nil {
			t.Fatalf("expected score for %s", r.SampleName)
		}
		if *r.Score != 1.0 {
			t.Fatalf("expected score 1.0 for %s (output contains both hi and goodbye), got %f", r.SampleName, *r.Score)
		}
		if r.Pass == nil || !*r.Pass {
			t.Fatalf("expected pass for %s", r.SampleName)
		}
	}

	if run.Status.Summary.PassRate != 1.0 {
		t.Fatalf("expected pass rate 1.0, got %f", run.Status.Summary.PassRate)
	}
	if run.Status.Summary.MeanScore != 1.0 {
		t.Fatalf("expected mean score 1.0, got %f", run.Status.Summary.MeanScore)
	}
}

func TestEvalRunController_ReconcileScoring_ManualTransitionsToPendingReview(t *testing.T) {
	ctx := context.Background()
	evalRuns := store.NewEvalRunStore()
	evalDatasets := store.NewEvalDatasetStore()
	taskStore := store.NewTaskStore()
	scorer := &agentruntime.EvalScorer{}
	logger := log.New(io.Discard, "", 0)
	ctrl := NewEvalRunController(evalRuns, evalDatasets, taskStore, scorer, logger)

	ds := resources.EvalDataset{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalDataset",
		Metadata:   resources.ObjectMeta{Name: "manual-ds", Namespace: "default"},
		Spec: resources.EvalDatasetSpec{
			Samples: []resources.EvalSample{
				{Name: "s1", Input: map[string]string{"prompt": "hello"}, Scoring: &resources.EvalScoringConfig{Strategy: "manual"}},
			},
		},
	}
	evalDatasets.Upsert(ctx, ds)

	run := resources.EvalRun{
		APIVersion: "orloj.dev/v1",
		Kind:       "EvalRun",
		Metadata:   resources.ObjectMeta{Name: "manual-run", Namespace: "default"},
		Spec: resources.EvalRunSpec{
			DatasetRef: "manual-ds",
			System:     "sys",
			Scoring:    resources.EvalScoringConfig{Strategy: "manual"},
		},
	}
	evalRuns.Upsert(ctx, run)

	ctrl.ReconcileOnce(ctx) // Pending -> Running

	allTasks, _ := taskStore.List(ctx)
	for i := range allTasks {
		allTasks[i].Status.Phase = "Succeeded"
		allTasks[i].Status.Output = map[string]string{"result": "output"}
		taskStore.Upsert(ctx, allTasks[i])
	}

	ctrl.ReconcileOnce(ctx) // Running -> Scoring
	ctrl.ReconcileOnce(ctx) // Scoring -> PendingReview

	got, _, _ := evalRuns.Get(ctx, "default/manual-run")
	if got.Status.Phase != resources.EvalRunPhasePendingReview {
		t.Fatalf("expected PendingReview, got %q", got.Status.Phase)
	}
}

func TestEvalRunController_ReconcileCancelled_CancelsInFlightTasks(t *testing.T) {
	ctx := context.Background()
	c, runs, datasets, tasks := newEvalRunControllerHarness(t)

	seedDatasetForController(t, datasets)
	seedPendingEvalRun(t, runs)
	_ = c.ReconcileOnce(ctx) // Pending -> Running

	run, _, _ := runs.Get(ctx, "default/eval-1")
	run.Status.Phase = resources.EvalRunPhaseCancelled
	runs.UpdateStatus(ctx, "default/eval-1", run.Status)

	_ = c.ReconcileOnce(ctx) // Cancelled reconcile

	allTasks, _ := tasks.List(ctx)
	for _, task := range allTasks {
		if task.Status.Phase != "Failed" {
			t.Fatalf("expected task %s to be Failed after cancel, got %q", task.Metadata.Name, task.Status.Phase)
		}
		if task.Status.LastError != "cancelled by eval run" {
			t.Fatalf("expected cancel error, got %q", task.Status.LastError)
		}
	}
}

func TestFlattenOutput(t *testing.T) {
	t.Parallel()

	if got := flattenOutput(nil); got != "" {
		t.Fatalf("nil output should return empty, got %q", got)
	}
	if got := flattenOutput(map[string]string{"result": "hello"}); got != "hello" {
		t.Fatalf("non-sentinel result should be returned, got %q", got)
	}
	if got := flattenOutput(map[string]string{"response": "world"}); got != "world" {
		t.Fatalf("expected 'world', got %q", got)
	}
	if got := flattenOutput(map[string]string{"foo": "bar"}); got == "" {
		t.Fatal("non-standard keys should still produce output")
	}

	// last_output takes precedence over result="executed"
	got := flattenOutput(map[string]string{
		"result":      "executed",
		"last_output": `{"decision":"approve"}`,
	})
	if got != `{"decision":"approve"}` {
		t.Fatalf("last_output should take precedence over result=executed, got %q", got)
	}

	// last_output with markdown fences should be unwrapped
	got = flattenOutput(map[string]string{
		"result":      "executed",
		"last_output": "```json\n{\"decision\":\"approve\"}\n```",
	})
	if got != `{"decision":"approve"}` {
		t.Fatalf("fenced last_output should be unwrapped, got %q", got)
	}

	// last_output with step=N model_output= prefix and fences should be cleaned
	got = flattenOutput(map[string]string{
		"result":      "executed",
		"last_output": "step=1 model_output=```json\n{\"decision\":\"approve\"}\n```",
	})
	if got != `{"decision":"approve"}` {
		t.Fatalf("prefixed fenced last_output should be cleaned, got %q", got)
	}

	// last_output with step=N model_output= prefix but no fences
	got = flattenOutput(map[string]string{
		"result":      "executed",
		"last_output": `step=3 model_output={"decision":"decline"}`,
	})
	if got != `{"decision":"decline"}` {
		t.Fatalf("prefixed last_output without fences should strip prefix, got %q", got)
	}

	// last_output takes precedence even when result is non-sentinel
	got = flattenOutput(map[string]string{
		"result":      "some-value",
		"last_output": `{"decision":"decline"}`,
	})
	if got != `{"decision":"decline"}` {
		t.Fatalf("last_output should take precedence over any result, got %q", got)
	}

	// result="executed" without last_output falls through to other keys
	got = flattenOutput(map[string]string{
		"result":   "executed",
		"response": "fallback",
	})
	if got != "fallback" {
		t.Fatalf("result=executed without last_output should fall through to response, got %q", got)
	}

	// result="executed" alone falls through to join fallback
	got = flattenOutput(map[string]string{
		"result": "executed",
		"foo":    "bar",
	})
	if got == "" || got == "executed" {
		t.Fatalf("result=executed should be skipped in fallback, got %q", got)
	}
}

func TestComputeLatency(t *testing.T) {
	t.Parallel()

	if got := computeLatency("", ""); got != "" {
		t.Fatalf("empty times should return empty, got %q", got)
	}
	if got := computeLatency("not-a-time", "not-a-time"); got != "" {
		t.Fatalf("invalid times should return empty, got %q", got)
	}

	start := time.Now().Add(-5 * time.Second).UTC().Format(time.RFC3339)
	end := time.Now().UTC().Format(time.RFC3339)
	result := computeLatency(start, end)
	if result == "" {
		t.Fatal("expected non-empty latency")
	}
}
