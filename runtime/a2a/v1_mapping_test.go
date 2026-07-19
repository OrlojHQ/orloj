package a2a

import (
	"errors"
	"testing"

	lf "github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/OrlojHQ/orloj/resources"
)

func TestCreateOrlojTaskFromV1GeneratesTaskAndContextIDs(t *testing.T) {
	req := &lf.SendMessageRequest{
		Message: &lf.Message{
			ID:    "message-1",
			Role:  lf.MessageRoleUser,
			Parts: lf.ContentParts{lf.NewTextPart("research this")},
		},
	}

	task, err := CreateOrlojTaskFromV1(req, "research-system", "team")
	if err != nil {
		t.Fatalf("CreateOrlojTaskFromV1() error = %v", err)
	}
	if task.Metadata.Labels[LabelA2ATaskID] == "" {
		t.Fatal("expected generated A2A task ID")
	}
	if task.Metadata.Labels[LabelA2AContextID] == "" {
		t.Fatal("expected generated A2A context ID")
	}
	if got := task.Metadata.Labels[LabelA2AProtocolVersion]; got != "1.0" {
		t.Fatalf("protocol version = %q, want 1.0", got)
	}
	if got := task.Spec.Input["prompt"]; got != "research this" {
		t.Fatalf("prompt = %q, want research this", got)
	}
	if len(task.Status.Messages) != 1 || task.Status.Messages[0].MessageID != "message-1" {
		t.Fatalf("initial message was not preserved: %#v", task.Status.Messages)
	}
}

func TestCreateOrlojTaskFromV1PreservesProvidedIDs(t *testing.T) {
	req := &lf.SendMessageRequest{
		Message: &lf.Message{
			ID:        "message-1",
			TaskID:    "task-1",
			ContextID: "context-1",
			Role:      lf.MessageRoleUser,
			Parts:     lf.ContentParts{lf.NewTextPart("continue")},
		},
	}

	task, err := CreateOrlojTaskFromV1(req, "system", "default")
	if err != nil {
		t.Fatalf("CreateOrlojTaskFromV1() error = %v", err)
	}
	if got := task.Metadata.Labels[LabelA2ATaskID]; got != "task-1" {
		t.Fatalf("task ID = %q, want task-1", got)
	}
	if got := task.Metadata.Labels[LabelA2AContextID]; got != "context-1" {
		t.Fatalf("context ID = %q, want context-1", got)
	}
}

func TestV1MessageTextRejectsUnsupportedPart(t *testing.T) {
	_, err := V1MessageText(&lf.Message{
		Parts: lf.ContentParts{lf.NewDataPart(map[string]any{"query": "test"})},
	})
	if !errors.Is(err, lf.ErrUnsupportedContentType) {
		t.Fatalf("error = %v, want ErrUnsupportedContentType", err)
	}
}

func TestOrlojTaskToV1UsesNormativeShapes(t *testing.T) {
	task := resources.Task{
		Metadata: resources.ObjectMeta{
			Name: "internal-task",
			Labels: map[string]string{
				LabelA2ATaskID:    "task-1",
				LabelA2AContextID: "context-1",
				LabelA2AMessageID: "message-1",
			},
		},
		Spec: resources.TaskSpec{Input: map[string]string{"prompt": "hello"}},
		Status: resources.TaskStatus{
			Phase:  "Succeeded",
			Output: map[string]string{"answer": "world"},
		},
	}

	got := OrlojTaskToV1(task)
	if got.ID != "task-1" || got.ContextID != "context-1" {
		t.Fatalf("task identity = (%q, %q)", got.ID, got.ContextID)
	}
	if got.Status.State != lf.TaskStateCompleted {
		t.Fatalf("state = %q, want %q", got.Status.State, lf.TaskStateCompleted)
	}
	if len(got.History) != 1 || got.History[0].Role != lf.MessageRoleUser {
		t.Fatalf("history = %#v", got.History)
	}
	if len(got.Artifacts) != 1 || got.Artifacts[0].ID != "output" {
		t.Fatalf("artifacts = %#v", got.Artifacts)
	}
	if data := got.Artifacts[0].Parts[0].Data(); data == nil {
		t.Fatal("output artifact should use a v1 data part")
	}
}

func TestOrlojTaskToV1UsesStableLegacyContextFallback(t *testing.T) {
	task := resources.Task{Metadata: resources.ObjectMeta{Name: "legacy"}, Status: resources.TaskStatus{Phase: "Pending"}}
	first := OrlojTaskToV1(task)
	second := OrlojTaskToV1(task)
	if first.ContextID == "" || first.ContextID != second.ContextID {
		t.Fatalf("context fallback is not stable: %q != %q", first.ContextID, second.ContextID)
	}
}
