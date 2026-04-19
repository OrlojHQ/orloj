package agentruntime

import (
	"encoding/json"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

type ReviewRequestPayload struct {
	Content        string `json:"content,omitempty"`
	Feedback       string `json:"feedback,omitempty"`
	PreviousOutput string `json:"previous_output,omitempty"`
	CheckpointID   string `json:"checkpoint_id,omitempty"`
	Cycle          int    `json:"cycle,omitempty"`
	RequestedBy    string `json:"requested_by,omitempty"`
	Supersedes     string `json:"supersedes,omitempty"`
}

func EncodeReviewRequestPayload(payload ReviewRequestPayload) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		return strings.TrimSpace(payload.Content)
	}
	return string(raw)
}

func DecodeReviewRequestPayload(raw string) (ReviewRequestPayload, bool) {
	var payload ReviewRequestPayload
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return ReviewRequestPayload{}, false
	}
	return payload, true
}

func ResumeMessageFromAgentMessage(msg AgentMessage) resources.TaskApprovalResumeMessage {
	return resources.TaskApprovalResumeMessage{
		MessageID:      strings.TrimSpace(msg.MessageID),
		IdempotencyKey: strings.TrimSpace(msg.IdempotencyKey),
		TaskID:         strings.TrimSpace(msg.TaskID),
		Attempt:        msg.Attempt,
		System:         strings.TrimSpace(msg.System),
		Namespace:      strings.TrimSpace(msg.Namespace),
		FromAgent:      strings.TrimSpace(msg.FromAgent),
		ToAgent:        strings.TrimSpace(msg.ToAgent),
		BranchID:       strings.TrimSpace(msg.BranchID),
		ParentBranchID: strings.TrimSpace(msg.ParentBranchID),
		Type:           strings.TrimSpace(msg.Type),
		Payload:        strings.TrimSpace(msg.Payload),
		Timestamp:      strings.TrimSpace(msg.Timestamp),
		TraceID:        strings.TrimSpace(msg.TraceID),
		ParentID:       strings.TrimSpace(msg.ParentID),
		DelegateOf:     strings.TrimSpace(msg.DelegateOf),
	}
}

func AgentMessageFromResumeMessage(msg resources.TaskApprovalResumeMessage) AgentMessage {
	return AgentMessage{
		MessageID:      strings.TrimSpace(msg.MessageID),
		IdempotencyKey: strings.TrimSpace(msg.IdempotencyKey),
		TaskID:         strings.TrimSpace(msg.TaskID),
		Attempt:        msg.Attempt,
		System:         strings.TrimSpace(msg.System),
		Namespace:      strings.TrimSpace(msg.Namespace),
		FromAgent:      strings.TrimSpace(msg.FromAgent),
		ToAgent:        strings.TrimSpace(msg.ToAgent),
		BranchID:       strings.TrimSpace(msg.BranchID),
		ParentBranchID: strings.TrimSpace(msg.ParentBranchID),
		Type:           strings.TrimSpace(msg.Type),
		Payload:        strings.TrimSpace(msg.Payload),
		Timestamp:      strings.TrimSpace(msg.Timestamp),
		TraceID:        strings.TrimSpace(msg.TraceID),
		ParentID:       strings.TrimSpace(msg.ParentID),
		DelegateOf:     strings.TrimSpace(msg.DelegateOf),
	}
}
