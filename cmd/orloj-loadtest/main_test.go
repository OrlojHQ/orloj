package main

import "testing"

func TestMergeQualityProfileUsesProfileWhenUnset(t *testing.T) {
	cfg := loadConfig{
		maxDeadletterRate: -1,
		maxFailedRate:     -1,
		maxTimedOut:       -1,
		minRetryTotal:     -1,
		minTakeoverEvents: -1,
	}
	profile := qualityProfile{
		Name:              "p",
		MinSuccessRate:    0.95,
		MaxDeadletterRate: 0.05,
		MaxFailedRate:     0.02,
		MaxTimedOut:       0,
		MinRetryTotal:     10,
		MinTakeoverEvents: 2,
	}
	merged := mergeQualityProfile(cfg, profile)
	if merged.qualityProfileName != "p" {
		t.Fatalf("expected qualityProfileName p, got %q", merged.qualityProfileName)
	}
	if merged.minSuccessRate != 0.95 {
		t.Fatalf("expected minSuccessRate 0.95, got %f", merged.minSuccessRate)
	}
	if merged.maxDeadletterRate != 0.05 {
		t.Fatalf("expected maxDeadletterRate 0.05, got %f", merged.maxDeadletterRate)
	}
	if merged.maxFailedRate != 0.02 {
		t.Fatalf("expected maxFailedRate 0.02, got %f", merged.maxFailedRate)
	}
	if merged.maxTimedOut != 0 {
		t.Fatalf("expected maxTimedOut 0, got %d", merged.maxTimedOut)
	}
	if merged.minRetryTotal != 10 {
		t.Fatalf("expected minRetryTotal 10, got %d", merged.minRetryTotal)
	}
	if merged.minTakeoverEvents != 2 {
		t.Fatalf("expected minTakeoverEvents 2, got %d", merged.minTakeoverEvents)
	}
}

func TestEvaluateRunViolations(t *testing.T) {
	summary := runSummary{
		createErrors:    1,
		timedOut:        2,
		injectionErrors: 1,
		succeeded:       7,
		failed:          2,
		deadletter:      1,
		totalTaskRetry:  3,
		totalTakeovers:  1,
	}
	metrics := computeRunMetrics(summary)
	cfg := loadConfig{
		maxTimedOut:       0,
		minSuccessRate:    0.9,
		maxDeadletterRate: 0.05,
		maxFailedRate:     0.05,
		minRetryTotal:     10,
		minTakeoverEvents: 2,
	}
	gates := evaluateRun(summary, metrics, cfg)
	if gates.Passed {
		t.Fatal("expected quality gates to fail")
	}
	if len(gates.Violations) < 5 {
		t.Fatalf("expected multiple violations, got %d", len(gates.Violations))
	}
}
