package controllers

import (
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestAppendTaskTraceDeterministicStepIDsByAttempt(t *testing.T) {
	controller := &TaskController{}
	task := resources.Task{
		Status: resources.TaskStatus{
			Attempts: 2,
			Trace: []resources.TaskTraceEvent{
				{StepID: "a001.s0001", Attempt: 1},
				{StepID: "a002.s0001", Attempt: 2},
				{StepID: "a002.s0002", Attempt: 2},
			},
		},
	}

	controller.appendTaskTrace(&task, resources.TaskTraceEvent{Type: "agent_start"})
	first := task.Status.Trace[len(task.Status.Trace)-1]
	if first.StepID != "a002.s0003" {
		t.Fatalf("expected first generated stepID a002.s0003, got %q", first.StepID)
	}
	if first.Attempt != 2 {
		t.Fatalf("expected attempt=2, got %d", first.Attempt)
	}

	controller.appendTaskTrace(&task, resources.TaskTraceEvent{Type: "agent_end"})
	second := task.Status.Trace[len(task.Status.Trace)-1]
	if second.StepID != "a002.s0004" {
		t.Fatalf("expected second generated stepID a002.s0004, got %q", second.StepID)
	}

	task.Status.Attempts = 3
	controller.appendTaskTrace(&task, resources.TaskTraceEvent{Type: "agent_start"})
	third := task.Status.Trace[len(task.Status.Trace)-1]
	if third.StepID != "a003.s0001" {
		t.Fatalf("expected new attempt stepID a003.s0001, got %q", third.StepID)
	}
	if third.Attempt != 3 {
		t.Fatalf("expected attempt=3, got %d", third.Attempt)
	}
}

func TestAppendTaskTraceUsesExplicitStepIDAndStepFallback(t *testing.T) {
	controller := &TaskController{}
	task := resources.Task{
		Status: resources.TaskStatus{
			Attempts: 4,
			Trace: []resources.TaskTraceEvent{
				{Attempt: 4, Step: 5},
			},
		},
	}

	controller.appendTaskTrace(&task, resources.TaskTraceEvent{
		StepID:  "custom-step",
		Attempt: 4,
		Type:    "agent_event",
	})
	custom := task.Status.Trace[len(task.Status.Trace)-1]
	if custom.StepID != "custom-step" {
		t.Fatalf("expected explicit stepID to be preserved, got %q", custom.StepID)
	}

	controller.appendTaskTrace(&task, resources.TaskTraceEvent{Type: "agent_event"})
	generated := task.Status.Trace[len(task.Status.Trace)-1]
	if generated.StepID != "a004.s0006" {
		t.Fatalf("expected generated stepID a004.s0006 from step fallback, got %q", generated.StepID)
	}
}
