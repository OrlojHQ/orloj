package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestValidateConfig(t *testing.T) {
	if err := validateConfig(alertConfig{baseURL: "http://x", namespace: "n", pollConcurrency: 1, timeout: 1, profilePath: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := validateConfig(alertConfig{baseURL: "", namespace: "n", pollConcurrency: 1, timeout: 1, profilePath: "p"}); err == nil {
		t.Fatal("expected error for empty base-url")
	}
	if err := validateConfig(alertConfig{baseURL: "http://x", namespace: "", pollConcurrency: 1, timeout: 1, profilePath: "p"}); err == nil {
		t.Fatal("expected error for empty namespace")
	}
	if err := validateConfig(alertConfig{baseURL: "http://x", namespace: "n", pollConcurrency: 0, timeout: 1, profilePath: "p"}); err == nil {
		t.Fatal("expected error for poll-concurrency")
	}
	if err := validateConfig(alertConfig{baseURL: "http://x", namespace: "n", pollConcurrency: 1, timeout: 0, profilePath: "p"}); err == nil {
		t.Fatal("expected error for timeout")
	}
	if err := validateConfig(alertConfig{baseURL: "http://x", namespace: "n", pollConcurrency: 1, timeout: 1, profilePath: "  "}); err == nil {
		t.Fatal("expected error for empty profile")
	}
}

func TestFilterTasks(t *testing.T) {
	tasks := []resources.Task{
		{Metadata: resources.ObjectMeta{Name: "alpha-one"}, Spec: resources.TaskSpec{System: "sys-a"}},
		{Metadata: resources.ObjectMeta{Name: "beta-two"}, Spec: resources.TaskSpec{System: "sys-b"}},
	}
	cfg := alertConfig{taskNamePrefix: "alpha", taskSystem: "sys-a"}
	out := filterTasks(tasks, cfg)
	if len(out) != 1 || out[0].Metadata.Name != "alpha-one" {
		t.Fatalf("got %+v", out)
	}
}

func TestEvaluateViolations_MinTasks(t *testing.T) {
	v := evaluateViolations(thresholdProfile{MinTasks: 5, Name: "p"}, taskAggregate{Tasks: 1})
	if len(v) != 1 || v[0].Code != "insufficient_sample" {
		t.Fatalf("got %+v", v)
	}
}

func TestEvaluateViolations_NoSuccess(t *testing.T) {
	v := evaluateViolations(thresholdProfile{
		MinTasks:                  1,
		RequireAnyTaskSucceeded:   true,
		RetryStormTotal:           0,
		RetryStormRate:            0,
		DeadletterGrowthTotal:     0,
		DeadletterGrowthRate:      0,
		InFlightMax:               0,
		LatencyP95MsMax:           0,
	}, taskAggregate{Tasks: 2, SucceededTasks: 0})
	found := false
	for _, x := range v {
		if x.Code == "no_success" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected no_success, got %+v", v)
	}
}

func TestEvaluateViolations_RetryStormTotal(t *testing.T) {
	v := evaluateViolations(thresholdProfile{MinTasks: 1, RetryStormTotal: 10}, taskAggregate{Tasks: 1, RetryCountTotal: 10})
	if len(v) != 1 || v[0].Code != "retry_storm_total" {
		t.Fatalf("got %+v", v)
	}
}

func TestEvaluateViolations_OK(t *testing.T) {
	v := evaluateViolations(thresholdProfile{Name: "p"}, taskAggregate{Tasks: 3})
	if len(v) != 0 {
		t.Fatalf("expected no violations, got %+v", v)
	}
}

func TestLoadProfile_DefaultName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	if err := os.WriteFile(path, []byte(`{"min_tasks":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := loadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "default" {
		t.Fatalf("name: %q", p.Name)
	}
}
