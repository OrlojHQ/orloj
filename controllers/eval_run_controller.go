package controllers

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

var modelOutputPrefixRegex = regexp.MustCompile(`^step=\d+\s+model_output=`) //nolint:gochecknoglobals

// EvalRunController watches for Pending EvalRuns, creates tasks for each
// dataset sample, watches for task completion, scores results, and updates
// the EvalRun status.
type EvalRunController struct {
	evalRuns     *store.EvalRunStore
	evalDatasets *store.EvalDatasetStore
	tasks        *store.TaskStore
	scorer       *agentruntime.EvalScorer
	logger       *log.Logger

	reconcileEvery time.Duration
}

func NewEvalRunController(
	evalRuns *store.EvalRunStore,
	evalDatasets *store.EvalDatasetStore,
	tasks *store.TaskStore,
	scorer *agentruntime.EvalScorer,
	logger *log.Logger,
) *EvalRunController {
	return &EvalRunController{
		evalRuns:       evalRuns,
		evalDatasets:   evalDatasets,
		tasks:          tasks,
		scorer:         scorer,
		logger:         logger,
		reconcileEvery: 5 * time.Second,
	}
}

// Start runs the reconcile loop until ctx is cancelled.
func (c *EvalRunController) Start(ctx context.Context) {
	ticker := time.NewTicker(c.reconcileEvery)
	defer ticker.Stop()

	for {
		if err := c.ReconcileOnce(ctx); err != nil && c.logger != nil {
			c.logger.Printf("eval run controller reconcile error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ReconcileOnce processes all active eval runs.
func (c *EvalRunController) ReconcileOnce(ctx context.Context) error {
	runs, err := c.evalRuns.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list eval runs: %w", err)
	}

	for i := range runs {
		run := &runs[i]
		switch run.Status.Phase {
		case resources.EvalRunPhasePending:
			if run.Spec.Suspended {
				continue
			}
			if err := c.reconcilePending(ctx, run); err != nil {
				c.logf("error reconciling pending eval run %s: %v", run.Metadata.Name, err)
			}
		case resources.EvalRunPhaseRunning:
			if err := c.reconcileRunning(ctx, run); err != nil {
				c.logf("error reconciling running eval run %s: %v", run.Metadata.Name, err)
			}
		case resources.EvalRunPhaseScoring:
			if err := c.reconcileScoring(ctx, run); err != nil {
				c.logf("error reconciling scoring eval run %s: %v", run.Metadata.Name, err)
			}
		case resources.EvalRunPhaseCancelled:
			c.reconcileCancelled(ctx, run)
		}
	}
	return nil
}

// reconcilePending creates tasks for each dataset sample and transitions to Running.
func (c *EvalRunController) reconcilePending(ctx context.Context, run *resources.EvalRun) error {
	dataset, ok, err := c.evalDatasets.Get(ctx, store.ScopedName(run.Metadata.Namespace, run.Spec.DatasetRef))
	if err != nil {
		return fmt.Errorf("failed to get dataset %q: %w", run.Spec.DatasetRef, err)
	}
	if !ok {
		run.Status.Phase = resources.EvalRunPhaseFailed
		run.Status.Results = nil
		c.updateStatus(ctx, run)
		return fmt.Errorf("dataset %q not found", run.Spec.DatasetRef)
	}

	run.Status.DatasetGeneration = dataset.Metadata.Generation
	run.Status.TotalSamples = len(dataset.Spec.Samples)
	run.Status.Results = make([]resources.EvalSampleResult, 0, len(dataset.Spec.Samples))
	run.Status.StartedAt = time.Now().UTC().Format(time.RFC3339)
	run.Status.Phase = resources.EvalRunPhaseRunning

	concurrency := run.Spec.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}

	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, sample := range dataset.Spec.Samples {
		sem <- struct{}{}
		wg.Add(1)
		go func(s resources.EvalSample) {
			defer wg.Done()
			defer func() { <-sem }()

			taskName := fmt.Sprintf("eval-%s-%s", run.Metadata.Name, s.Name)
			task := resources.Task{
				APIVersion: "orloj.dev/v1",
				Kind:       "Task",
				Metadata: resources.ObjectMeta{
					Name:      taskName,
					Namespace: run.Metadata.Namespace,
					Labels: map[string]string{
						"orloj.dev/eval-run":    run.Metadata.Name,
						"orloj.dev/eval-sample": s.Name,
					},
				},
				Spec: resources.TaskSpec{
					System: run.Spec.System,
					Input:  s.Input,
				},
			}

			if _, err := c.tasks.Upsert(ctx, task); err != nil {
				mu.Lock()
				run.Status.Results = append(run.Status.Results, resources.EvalSampleResult{
					SampleName: s.Name,
					TaskName:   taskName,
					Error:      fmt.Sprintf("failed to create task: %v", err),
				})
				mu.Unlock()
				return
			}

			mu.Lock()
			run.Status.Results = append(run.Status.Results, resources.EvalSampleResult{
				SampleName: s.Name,
				TaskName:   taskName,
			})
			mu.Unlock()
		}(sample)
	}
	wg.Wait()

	c.updateStatus(ctx, run)
	return nil
}

// reconcileRunning checks task completion and transitions to Scoring when done.
// Progress is persisted on every tick so the frontend can show live sample counts.
func (c *EvalRunController) reconcileRunning(ctx context.Context, run *resources.EvalRun) error {
	allDone := true
	completed := 0
	for i, result := range run.Status.Results {
		if result.Error != "" {
			completed++
			continue
		}
		if result.TaskName == "" {
			continue
		}

		task, ok, err := c.tasks.Get(ctx, store.ScopedName(run.Metadata.Namespace, result.TaskName))
		if err != nil {
			c.logf("failed to get task %s: %v", result.TaskName, err)
			allDone = false
			continue
		}
		if !ok {
			run.Status.Results[i].Error = "task not found"
			completed++
			continue
		}

		switch task.Status.Phase {
		case "Succeeded":
			output := flattenOutput(task.Status.Output)
			run.Status.Results[i].Output = output
			run.Status.Results[i].Latency = computeLatency(task.Status.StartedAt, task.Status.CompletedAt)
			completed++
		case "Failed", "DeadLetter":
			run.Status.Results[i].Error = task.Status.LastError
			if run.Status.Results[i].Error == "" {
				run.Status.Results[i].Error = "task failed"
			}
			completed++
		default:
			allDone = false
		}
	}

	run.Status.CompletedSamples = completed
	if allDone {
		run.Status.Phase = resources.EvalRunPhaseScoring
	}
	c.updateStatus(ctx, run)
	return nil
}

// reconcileScoring runs the scoring pipeline and transitions to terminal phase.
func (c *EvalRunController) reconcileScoring(ctx context.Context, run *resources.EvalRun) error {
	dataset, ok, err := c.evalDatasets.Get(ctx, store.ScopedName(run.Metadata.Namespace, run.Spec.DatasetRef))
	if err != nil || !ok {
		c.logf("failed to get dataset %q for scoring: %v", run.Spec.DatasetRef, err)
		dataset = resources.EvalDataset{}
	}

	sampleMap := make(map[string]resources.EvalSample, len(dataset.Spec.Samples))
	for _, s := range dataset.Spec.Samples {
		sampleMap[s.Name] = s
	}

	run.Status.CompletedSamples = 0
	run.Status.ErroredSamples = 0
	run.Status.PassedSamples = 0
	run.Status.FailedSamples = 0

	hasManual := false
	for i := range run.Status.Results {
		result := &run.Status.Results[i]
		if result.Error != "" {
			run.Status.ErroredSamples++
			run.Status.CompletedSamples++
			continue
		}

		sample := sampleMap[result.SampleName]
		scoring := run.Spec.Scoring
		if sample.Scoring != nil {
			scoring = *sample.Scoring
		}

		if strings.ToLower(strings.TrimSpace(scoring.Strategy)) == resources.EvalScoringManual {
			hasManual = true
			run.Status.CompletedSamples++
			continue
		}

		if scoring.Strategy == "" && !sample.Expected.IsEmpty() {
			scoring.Strategy = resources.EvalScoringExactMatch
		}

		if scoring.Strategy == "" {
			run.Status.CompletedSamples++
			continue
		}

		scoreResult := c.scorer.Score(ctx, scoring, sample.Expected, sample.Input, result.Output)
		result.Score = scoreResult.Score
		result.Pass = scoreResult.Pass
		result.Reasoning = scoreResult.Reasoning
		if scoreResult.Error != "" {
			result.Error = scoreResult.Error
			run.Status.ErroredSamples++
		} else if result.Pass != nil {
			if *result.Pass {
				run.Status.PassedSamples++
			} else {
				run.Status.FailedSamples++
			}
		}
		run.Status.CompletedSamples++
	}

	run.Status.Summary = resources.ComputeEvalSummary(run.Status.Results)
	run.Status.CompletedAt = time.Now().UTC().Format(time.RFC3339)

	if hasManual {
		run.Status.Phase = resources.EvalRunPhasePendingReview
	} else {
		run.Status.Phase = resources.EvalRunPhaseSucceeded
	}

	c.updateStatus(ctx, run)
	return nil
}

// reconcileCancelled attempts to cancel any in-flight tasks.
func (c *EvalRunController) reconcileCancelled(ctx context.Context, run *resources.EvalRun) {
	for _, result := range run.Status.Results {
		if result.TaskName == "" || result.Error != "" {
			continue
		}
		task, ok, err := c.tasks.Get(ctx, store.ScopedName(run.Metadata.Namespace, result.TaskName))
		if err != nil || !ok {
			continue
		}
		if task.Status.Phase == "Succeeded" || task.Status.Phase == "Failed" || task.Status.Phase == "DeadLetter" {
			continue
		}
		task.Status.Phase = "Failed"
		task.Status.LastError = "cancelled by eval run"
		if _, err := c.tasks.Upsert(ctx, task); err != nil {
			c.logf("failed to cancel task %s: %v", result.TaskName, err)
		}
	}
}

func (c *EvalRunController) updateStatus(ctx context.Context, run *resources.EvalRun) {
	key := store.ScopedName(run.Metadata.Namespace, run.Metadata.Name)
	if _, err := c.evalRuns.UpdateStatus(ctx, key, run.Status); err != nil && c.logger != nil {
		c.logger.Printf("failed to update eval run %s status: %v", run.Metadata.Name, err)
	}
}

func (c *EvalRunController) logf(format string, args ...any) {
	if c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

func flattenOutput(output map[string]string) string {
	if len(output) == 0 {
		return ""
	}
	if v, ok := output["last_output"]; ok && strings.TrimSpace(v) != "" {
		v = modelOutputPrefixRegex.ReplaceAllString(v, "")
		return resources.UnwrapFencedCodeBlock(strings.TrimSpace(v))
	}
	if v, ok := output["response"]; ok {
		return v
	}
	if v, ok := output["result"]; ok && v != "executed" {
		return v
	}
	var parts []string
	for _, v := range output {
		parts = append(parts, v)
	}
	return strings.Join(parts, "\n")
}

func computeLatency(startedAt, completedAt string) string {
	if startedAt == "" || completedAt == "" {
		return ""
	}
	start, err1 := time.Parse(time.RFC3339, startedAt)
	end, err2 := time.Parse(time.RFC3339, completedAt)
	if err1 != nil || err2 != nil {
		start2, err3 := time.Parse(time.RFC3339Nano, startedAt)
		end2, err4 := time.Parse(time.RFC3339Nano, completedAt)
		if err3 != nil || err4 != nil {
			return ""
		}
		start, end = start2, end2
	}
	return end.Sub(start).String()
}
