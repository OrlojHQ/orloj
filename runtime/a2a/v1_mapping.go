package a2a

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	lf "github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/OrlojHQ/orloj/resources"
)

const (
	LabelA2AMessageID       = "orloj.dev/a2a-message-id"
	LabelA2AProtocolVersion = "orloj.dev/a2a-protocol-version"
)

// V1MessageText converts a v1 message into Orloj's text task input.
//
// Orloj currently advertises text/plain input. Rejecting other part types is
// preferable to silently dropping content that could affect the requested work.
func V1MessageText(message *lf.Message) (string, error) {
	if message == nil || len(message.Parts) == 0 {
		return "", fmt.Errorf("message parts are required: %w", lf.ErrInvalidParams)
	}

	parts := make([]string, 0, len(message.Parts))
	for _, part := range message.Parts {
		if part == nil {
			return "", fmt.Errorf("message part is null: %w", lf.ErrInvalidParams)
		}
		text, ok := part.Content.(lf.Text)
		if !ok {
			return "", fmt.Errorf("Orloj currently accepts only text parts: %w", lf.ErrUnsupportedContentType)
		}
		if value := strings.TrimSpace(string(text)); value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("message must contain non-empty text: %w", lf.ErrInvalidParams)
	}
	return strings.Join(parts, "\n"), nil
}

// CreateOrlojTaskFromV1 builds an Orloj task from a normative A2A v1 request.
// The A2A task and context IDs are generated server-side when omitted.
func CreateOrlojTaskFromV1(req *lf.SendMessageRequest, system, namespace string) (resources.Task, error) {
	if req == nil || req.Message == nil {
		return resources.Task{}, fmt.Errorf("message is required: %w", lf.ErrInvalidParams)
	}
	if strings.TrimSpace(req.Message.ID) == "" {
		return resources.Task{}, fmt.Errorf("messageId is required: %w", lf.ErrInvalidParams)
	}
	if req.Message.Role != lf.MessageRoleUser {
		return resources.Task{}, fmt.Errorf("initial message role must be ROLE_USER: %w", lf.ErrInvalidParams)
	}

	input, err := V1MessageText(req.Message)
	if err != nil {
		return resources.Task{}, err
	}

	taskID := req.Message.TaskID
	if taskID == "" {
		taskID = lf.NewTaskID()
	}
	contextID := strings.TrimSpace(req.Message.ContextID)
	if contextID == "" {
		contextID = lf.NewContextID()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	return resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Name:      "a2a-" + string(taskID),
			Namespace: resources.NormalizeNamespace(namespace),
			Labels: map[string]string{
				LabelA2ATaskID:          string(taskID),
				LabelA2AContextID:       contextID,
				LabelA2AMessageID:       req.Message.ID,
				LabelA2AProtocolVersion: string(lf.Version),
			},
			Annotations: map[string]string{
				"orloj.dev/created-by": "a2a-protocol",
			},
		},
		Spec: resources.TaskSpec{
			System: strings.TrimSpace(system),
			Input:  map[string]string{"prompt": input},
		},
		Status: resources.TaskStatus{
			Phase:     "Pending",
			StartedAt: now,
			Messages: []resources.TaskMessage{{
				Timestamp: now,
				MessageID: req.Message.ID,
				TaskID:    string(taskID),
				System:    strings.TrimSpace(system),
				Type:      "a2a.user",
				Content:   input,
			}},
		},
	}, nil
}

// OrlojTaskToV1 converts an Orloj task to the normative A2A v1 task shape.
func OrlojTaskToV1(task resources.Task) *lf.Task {
	taskID := task.Metadata.Name
	contextID := ""
	messageID := ""
	if task.Metadata.Labels != nil {
		if value := strings.TrimSpace(task.Metadata.Labels[LabelA2ATaskID]); value != "" {
			taskID = value
		}
		contextID = strings.TrimSpace(task.Metadata.Labels[LabelA2AContextID])
		messageID = strings.TrimSpace(task.Metadata.Labels[LabelA2AMessageID])
	}
	if contextID == "" {
		// Older Orloj A2A tasks predate context IDs. A stable fallback prevents
		// the required v1 field from changing between reads.
		contextID = "ctx-" + taskID
	}

	status := lf.TaskStatus{State: OrlojPhaseToV1State(task)}
	status.Timestamp = taskStatusTimestamp(task)
	if task.Status.LastError != "" {
		status.Message = lf.NewMessageForTask(
			lf.MessageRoleAgent,
			lf.TaskInfo{TaskID: lf.TaskID(taskID), ContextID: contextID},
			lf.NewTextPart(task.Status.LastError),
		)
	}

	result := &lf.Task{
		ID:        lf.TaskID(taskID),
		ContextID: contextID,
		Status:    status,
		Metadata: map[string]any{
			"orloj.task": task.Metadata.Name,
		},
	}

	if prompt := strings.TrimSpace(task.Spec.Input["prompt"]); prompt != "" {
		if messageID == "" {
			messageID = "initial-" + taskID
		}
		result.History = []*lf.Message{{
			ID:        messageID,
			ContextID: contextID,
			TaskID:    lf.TaskID(taskID),
			Role:      lf.MessageRoleUser,
			Parts:     lf.ContentParts{lf.NewTextPart(prompt)},
		}}
	}

	if len(task.Status.Output) > 0 {
		output := make(map[string]any, len(task.Status.Output))
		for key, value := range task.Status.Output {
			output[key] = value
		}
		result.Artifacts = []*lf.Artifact{{
			ID:    lf.ArtifactID("output"),
			Name:  "output",
			Parts: lf.ContentParts{lf.NewDataPart(output)},
		}}
	}

	return result
}

// OrlojPhaseToV1State converts Orloj phases to A2A v1 enum values.
func OrlojPhaseToV1State(task resources.Task) lf.TaskState {
	switch strings.TrimSpace(task.Status.Phase) {
	case "Pending":
		return lf.TaskStateSubmitted
	case "Running":
		return lf.TaskStateWorking
	case "WaitingApproval":
		if isA2AInputRequired(task) {
			return lf.TaskStateInputRequired
		}
		return lf.TaskStateWorking
	case "Succeeded":
		return lf.TaskStateCompleted
	case "Failed", "DeadLetter":
		if isA2ACancelled(task) {
			return lf.TaskStateCanceled
		}
		return lf.TaskStateFailed
	default:
		return lf.TaskStateWorking
	}
}

func taskStatusTimestamp(task resources.Task) *time.Time {
	for _, value := range []string{task.Status.CompletedAt, task.Status.LastHeartbeat, task.Status.StartedAt} {
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err == nil {
			utc := parsed.UTC()
			return &utc
		}
	}
	return nil
}

// MarshalV1Metadata preserves arbitrary A2A metadata in string-only Orloj
// fields when an adapter needs to persist it.
func MarshalV1Metadata(value map[string]any) string {
	if len(value) == 0 {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// IsV1Error reports whether err belongs to the normative A2A error taxonomy.
func IsV1Error(err error) bool {
	for _, target := range []error{
		lf.ErrParseError,
		lf.ErrInvalidRequest,
		lf.ErrMethodNotFound,
		lf.ErrInvalidParams,
		lf.ErrInternalError,
		lf.ErrTaskNotFound,
		lf.ErrTaskNotCancelable,
		lf.ErrPushNotificationNotSupported,
		lf.ErrUnsupportedOperation,
		lf.ErrUnsupportedContentType,
		lf.ErrInvalidAgentResponse,
		lf.ErrExtendedCardNotConfigured,
		lf.ErrExtensionSupportRequired,
		lf.ErrVersionNotSupported,
		lf.ErrUnauthenticated,
		lf.ErrUnauthorized,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
