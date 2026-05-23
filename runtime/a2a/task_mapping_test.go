package a2a

import (
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestOrlojPhaseToA2AState(t *testing.T) {
	tests := []struct {
		phase    string
		labels   map[string]string
		blocked  *resources.TaskBlockedOn
		expected string
	}{
		{"Pending", nil, nil, TaskStateSubmitted},
		{"Running", nil, nil, TaskStateWorking},
		{"Succeeded", nil, nil, TaskStateCompleted},
		{"Failed", nil, nil, TaskStateFailed},
		{"Failed", map[string]string{LabelA2ACancelled: "true"}, nil, TaskStateCanceled},
		{"WaitingApproval", nil, nil, TaskStateWorking},
		{"WaitingApproval", map[string]string{LabelBlockedReason: BlockedReasonA2AInput}, nil, TaskStateInputRequired},
		{"WaitingApproval", nil, &resources.TaskBlockedOn{Kind: BlockedKindA2AInput}, TaskStateInputRequired},
		{"", nil, nil, TaskStateWorking},
	}

	for _, tt := range tests {
		task := resources.Task{
			Metadata: resources.ObjectMeta{Labels: tt.labels},
			Status:   resources.TaskStatus{Phase: tt.phase, BlockedOn: tt.blocked},
		}
		got := OrlojPhaseToA2AState(task)
		if got != tt.expected {
			t.Errorf("phase=%q labels=%v blocked=%v: expected %q, got %q", tt.phase, tt.labels, tt.blocked, tt.expected, got)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	terminals := []string{TaskStateCompleted, TaskStateFailed, TaskStateCanceled, TaskStateRejected}
	for _, s := range terminals {
		if !IsTerminal(s) {
			t.Errorf("expected %q to be terminal", s)
		}
	}

	nonTerminals := []string{TaskStateSubmitted, TaskStateWorking, TaskStateInputRequired}
	for _, s := range nonTerminals {
		if IsTerminal(s) {
			t.Errorf("expected %q to not be terminal", s)
		}
	}
}

func TestOrlojTaskToA2AResult(t *testing.T) {
	task := resources.Task{
		Metadata: resources.ObjectMeta{
			Name: "test-task",
			Labels: map[string]string{
				LabelA2ATaskID: "ext-123",
			},
		},
		Status: resources.TaskStatus{
			Phase: "Succeeded",
			Output: map[string]string{
				"result": "hello world",
			},
		},
	}

	result := OrlojTaskToA2AResult(task)

	if result.ID != "ext-123" {
		t.Errorf("expected ID ext-123, got %s", result.ID)
	}
	if result.Status.State != TaskStateCompleted {
		t.Errorf("expected completed, got %s", result.Status.State)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(result.Artifacts))
	}
	if result.Artifacts[0].Name != "output" {
		t.Errorf("expected artifact name 'output', got %q", result.Artifacts[0].Name)
	}
}

func TestOrlojTaskToA2AResult_FallbackID(t *testing.T) {
	task := resources.Task{
		Metadata: resources.ObjectMeta{Name: "fallback-name"},
		Status:   resources.TaskStatus{Phase: "Running"},
	}

	result := OrlojTaskToA2AResult(task)

	if result.ID != "fallback-name" {
		t.Errorf("expected fallback ID fallback-name, got %s", result.ID)
	}
}

func TestOrlojTaskToA2AResult_WithError(t *testing.T) {
	task := resources.Task{
		Metadata: resources.ObjectMeta{Name: "err-task"},
		Status: resources.TaskStatus{
			Phase:     "Failed",
			LastError: "model timeout",
		},
	}

	result := OrlojTaskToA2AResult(task)

	if result.Status.State != TaskStateFailed {
		t.Errorf("expected failed, got %s", result.Status.State)
	}
	if result.Status.Message == nil {
		t.Fatal("expected status message for error")
	}
	if len(result.Status.Message.Parts) == 0 || result.Status.Message.Parts[0].Text != "model timeout" {
		t.Errorf("expected error text 'model timeout', got %+v", result.Status.Message)
	}
}

func TestCreateOrlojTaskFromA2A(t *testing.T) {
	params := TaskSendParams{
		ID: "a2a-task-1",
		Message: TaskMessage{
			Role: "user",
			Parts: []TaskPart{
				{Type: "text", Text: "Summarize this document"},
			},
		},
		Metadata: map[string]string{
			"contextId": "session-1",
			"client":    "external-app",
		},
	}

	task := CreateOrlojTaskFromA2A(params, "summarizer-system", "default")

	if task.Metadata.Name != "a2a-a2a-task-1" {
		t.Errorf("expected name a2a-a2a-task-1, got %s", task.Metadata.Name)
	}
	if task.Metadata.Namespace != "default" {
		t.Errorf("expected namespace default, got %s", task.Metadata.Namespace)
	}
	if task.Spec.System != "summarizer-system" {
		t.Errorf("expected system summarizer-system, got %s", task.Spec.System)
	}
	if task.Spec.Input["prompt"] != "Summarize this document" {
		t.Errorf("unexpected input: %v", task.Spec.Input)
	}
	if task.Metadata.Labels[LabelA2ATaskID] != "a2a-task-1" {
		t.Errorf("expected a2a task ID label, got %v", task.Metadata.Labels)
	}
	if task.Metadata.Labels[LabelA2AContextID] != "session-1" {
		t.Errorf("expected context ID label, got %v", task.Metadata.Labels)
	}
	if task.Metadata.Labels[LabelA2AClient] != "external-app" {
		t.Errorf("expected client label, got %v", task.Metadata.Labels)
	}
}

func TestOrlojTaskToA2AResult_NoOutput(t *testing.T) {
	task := resources.Task{
		Metadata: resources.ObjectMeta{Name: "empty-output"},
		Status:   resources.TaskStatus{Phase: "Succeeded"},
	}

	result := OrlojTaskToA2AResult(task)

	if result.Status.State != TaskStateCompleted {
		t.Errorf("expected completed, got %s", result.Status.State)
	}
	if len(result.Artifacts) != 0 {
		t.Errorf("expected 0 artifacts for nil output, got %d", len(result.Artifacts))
	}
}

func TestOrlojTaskToA2AResult_MultipleOutputKeys(t *testing.T) {
	task := resources.Task{
		Metadata: resources.ObjectMeta{Name: "multi-output"},
		Status: resources.TaskStatus{
			Phase: "Succeeded",
			Output: map[string]string{
				"summary": "This is a summary",
				"details": "These are the details",
			},
		},
	}

	result := OrlojTaskToA2AResult(task)

	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 combined artifact, got %d", len(result.Artifacts))
	}
}

func TestOrlojTaskToA2AResult_CancelledTask(t *testing.T) {
	task := resources.Task{
		Metadata: resources.ObjectMeta{
			Name:   "cancelled-task",
			Labels: map[string]string{LabelA2ACancelled: "true"},
		},
		Status: resources.TaskStatus{Phase: "Failed"},
	}

	result := OrlojTaskToA2AResult(task)

	if result.Status.State != TaskStateCanceled {
		t.Errorf("expected canceled, got %s", result.Status.State)
	}
}

func TestCreateOrlojTaskFromA2A_NoMetadata(t *testing.T) {
	params := TaskSendParams{
		ID: "simple-task",
		Message: TaskMessage{
			Role:  "user",
			Parts: []TaskPart{{Type: "text", Text: "Hello"}},
		},
	}

	task := CreateOrlojTaskFromA2A(params, "my-system", "default")

	if task.Metadata.Labels[LabelA2AContextID] != "" {
		t.Errorf("expected empty context ID, got %q", task.Metadata.Labels[LabelA2AContextID])
	}
	if task.Metadata.Labels[LabelA2AClient] != "" {
		t.Errorf("expected empty client, got %q", task.Metadata.Labels[LabelA2AClient])
	}
}

func TestCreateOrlojTaskFromA2A_MultiPartMessage(t *testing.T) {
	params := TaskSendParams{
		ID: "multi-part",
		Message: TaskMessage{
			Role: "user",
			Parts: []TaskPart{
				{Type: "text", Text: "Part one."},
				{Type: "data", Text: "ignored"},
				{Type: "text", Text: "Part two."},
			},
		},
	}

	task := CreateOrlojTaskFromA2A(params, "system", "ns")

	if task.Spec.Input["prompt"] != "Part one.\nPart two." {
		t.Errorf("expected concatenated text parts, got %q", task.Spec.Input["prompt"])
	}
}

func TestExtractTextFromMessage(t *testing.T) {
	msg := TaskMessage{
		Role: "user",
		Parts: []TaskPart{
			{Type: "text", Text: "Part 1"},
			{Type: "data", Text: "ignored"},
			{Type: "text", Text: "Part 2"},
		},
	}

	result := extractTextFromMessage(msg)

	if result != "Part 1\nPart 2" {
		t.Errorf("expected 'Part 1\\nPart 2', got %q", result)
	}
}
