package a2a

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

// A2A label keys used on Orloj Tasks for A2A correlation.
const (
	LabelA2ATaskID    = "orloj.dev/a2a-task-id"
	LabelA2AContextID = "orloj.dev/a2a-context-id"
	LabelA2AClient    = "orloj.dev/a2a-client"
	LabelA2ACancelled = "orloj.dev/a2a-cancelled"
	LabelBlockedReason = "orloj.dev/blocked-reason"

	BlockedReasonA2AInput = "a2a-input-required"
	BlockedKindA2AInput   = "A2AInputRequest"
)

// OrlojPhaseToA2AState converts an Orloj Task phase to an A2A task state.
func OrlojPhaseToA2AState(task resources.Task) string {
	phase := strings.TrimSpace(task.Status.Phase)
	switch phase {
	case "Pending":
		return TaskStateSubmitted
	case "Running":
		return TaskStateWorking
	case "WaitingApproval":
		if isA2AInputRequired(task) {
			return TaskStateInputRequired
		}
		return TaskStateWorking
	case "Succeeded":
		return TaskStateCompleted
	case "Failed":
		if isA2ACancelled(task) {
			return TaskStateCanceled
		}
		return TaskStateFailed
	default:
		return TaskStateWorking
	}
}

// IsTerminal returns true if the A2A state represents a terminal condition.
func IsTerminal(state string) bool {
	switch state {
	case TaskStateCompleted, TaskStateFailed, TaskStateCanceled, TaskStateRejected:
		return true
	}
	return false
}

func isA2AInputRequired(task resources.Task) bool {
	if task.Metadata.Labels != nil && task.Metadata.Labels[LabelBlockedReason] == BlockedReasonA2AInput {
		return true
	}
	if task.Status.BlockedOn != nil && task.Status.BlockedOn.Kind == BlockedKindA2AInput {
		return true
	}
	return false
}

func isA2ACancelled(task resources.Task) bool {
	return task.Metadata.Labels != nil && task.Metadata.Labels[LabelA2ACancelled] == "true"
}

// OrlojTaskToA2AResult converts an Orloj Task to an A2A TaskResult.
func OrlojTaskToA2AResult(task resources.Task) TaskResult {
	a2aTaskID := ""
	if task.Metadata.Labels != nil {
		a2aTaskID = task.Metadata.Labels[LabelA2ATaskID]
	}
	if a2aTaskID == "" {
		a2aTaskID = task.Metadata.Name
	}

	result := TaskResult{
		ID: a2aTaskID,
		Status: TaskStatus{
			State: OrlojPhaseToA2AState(task),
		},
		Metadata: map[string]string{
			"orloj.task": task.Metadata.Name,
		},
	}

	if task.Status.LastError != "" {
		result.Status.Message = &TaskMessage{
			Role: "agent",
			Parts: []TaskPart{{
				Type: "text",
				Text: task.Status.LastError,
			}},
		}
	}

	if task.Status.Output != nil {
		artifact := TaskArtifact{
			Name:  "output",
			Index: 0,
		}
		outputJSON, err := json.Marshal(task.Status.Output)
		if err == nil {
			artifact.Parts = []TaskPart{{
				Type: "text",
				Text: string(outputJSON),
			}}
		}
		result.Artifacts = append(result.Artifacts, artifact)
	}

	return result
}

// CreateOrlojTaskFromA2A builds an Orloj Task from an A2A send request.
func CreateOrlojTaskFromA2A(params TaskSendParams, system string, namespace string) resources.Task {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	inputText := extractTextFromMessage(params.Message)

	labels := map[string]string{
		LabelA2ATaskID: params.ID,
	}
	if params.Metadata != nil {
		if contextID, ok := params.Metadata["contextId"]; ok {
			labels[LabelA2AContextID] = contextID
		}
		if client, ok := params.Metadata["client"]; ok {
			labels[LabelA2AClient] = client
		}
	}

	task := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Name:      fmt.Sprintf("a2a-%s", params.ID),
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"orloj.dev/created-by": "a2a-protocol",
			},
		},
		Spec: resources.TaskSpec{
			System: system,
			Input: map[string]string{
				"prompt": inputText,
			},
		},
		Status: resources.TaskStatus{
			Phase:     "Pending",
			StartedAt: now,
		},
	}

	return task
}

func extractTextFromMessage(msg TaskMessage) string {
	var parts []string
	for _, part := range msg.Parts {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			parts = append(parts, strings.TrimSpace(part.Text))
		}
	}
	return strings.Join(parts, "\n")
}
