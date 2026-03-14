package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnonJon/orloj/crds"
)

// AgentMessage is the runtime envelope exchanged between agents.
type AgentMessage struct {
	MessageID      string `json:"message_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	Attempt        int    `json:"attempt,omitempty"`
	System         string `json:"system,omitempty"`
	Namespace      string `json:"namespace,omitempty"`
	FromAgent      string `json:"from_agent,omitempty"`
	ToAgent        string `json:"to_agent,omitempty"`
	BranchID       string `json:"branch_id,omitempty"`
	ParentBranchID string `json:"parent_branch_id,omitempty"`
	Type           string `json:"type,omitempty"`
	Payload        string `json:"payload,omitempty"`
	Timestamp      string `json:"timestamp,omitempty"`
	TraceID        string `json:"trace_id,omitempty"`
	ParentID       string `json:"parent_id,omitempty"`
}

// AgentMessageDelivery represents one delivery instance with ack semantics.
type AgentMessageDelivery interface {
	Message() AgentMessage
	Ack(context.Context) error
	Nack(context.Context, bool) error
	NackWithDelay(context.Context, time.Duration) error
	ExtendLease(context.Context, time.Duration) error
}

// AgentMessageHandler processes one message delivery.
type AgentMessageHandler func(context.Context, AgentMessageDelivery) error

// AgentMessageSubscription identifies a consumer input stream.
type AgentMessageSubscription struct {
	Namespace string
	Agent     string
	Durable   string
}

// AgentMessageBus is the runtime data-plane contract for agent messaging.
type AgentMessageBus interface {
	Publish(context.Context, AgentMessage) (AgentMessage, error)
	Consume(context.Context, AgentMessageSubscription, AgentMessageHandler) error
	Close() error
}

// RetryRequestError instructs the message bus consumer to requeue after a delay.
type RetryRequestError struct {
	Delay time.Duration
	Err   error
}

func (e *RetryRequestError) Error() string {
	if e == nil {
		return "retry requested"
	}
	if e.Err == nil {
		return fmt.Sprintf("retry requested after %s", e.Delay)
	}
	return fmt.Sprintf("retry requested after %s: %v", e.Delay, e.Err)
}

func (e *RetryRequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// RetryAfter builds a handler error that asks the bus to requeue this delivery after delay.
func RetryAfter(delay time.Duration, cause error) error {
	if delay < 0 {
		delay = 0
	}
	return &RetryRequestError{
		Delay: delay,
		Err:   cause,
	}
}

func retryDelayFromError(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	var retryErr *RetryRequestError
	if !errors.As(err, &retryErr) || retryErr == nil {
		return 0, false
	}
	if retryErr.Delay < 0 {
		return 0, true
	}
	return retryErr.Delay, true
}

func normalizeAgentMessage(message AgentMessage) (AgentMessage, error) {
	message.Namespace = crds.NormalizeNamespace(message.Namespace)
	message.FromAgent = strings.TrimSpace(message.FromAgent)
	message.ToAgent = strings.TrimSpace(message.ToAgent)
	message.TaskID = strings.TrimSpace(message.TaskID)
	message.System = strings.TrimSpace(message.System)
	message.Type = strings.TrimSpace(message.Type)
	message.Payload = strings.TrimSpace(message.Payload)
	message.TraceID = strings.TrimSpace(message.TraceID)
	message.ParentID = strings.TrimSpace(message.ParentID)
	message.BranchID = strings.TrimSpace(message.BranchID)
	message.ParentBranchID = strings.TrimSpace(message.ParentBranchID)
	message.IdempotencyKey = strings.TrimSpace(message.IdempotencyKey)

	if message.ToAgent == "" {
		return AgentMessage{}, fmt.Errorf("to_agent is required")
	}
	if message.Type == "" {
		message.Type = "task_handoff"
	}
	if message.Attempt <= 0 {
		message.Attempt = 1
	}
	if strings.TrimSpace(message.Timestamp) == "" {
		message.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(message.MessageID) == "" {
		message.MessageID = fmt.Sprintf("%s:%s:%s:%d", message.TaskID, message.FromAgent, message.ToAgent, time.Now().UnixNano())
	}
	if message.IdempotencyKey == "" {
		message.IdempotencyKey = strings.TrimSpace(message.MessageID)
	}
	return message, nil
}

func messageSubject(prefix string, namespace string, agent string) string {
	p := strings.Trim(strings.TrimSpace(prefix), ".")
	if p == "" {
		p = "orloj.agentmsg"
	}
	ns := crds.NormalizeNamespace(namespace)
	ag := sanitizeSubjectToken(agent)
	return p + "." + sanitizeSubjectToken(ns) + "." + ag + ".inbox"
}

func sanitizeSubjectToken(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "default"
	}
	replacer := strings.NewReplacer(" ", "_", ".", "_", ">", "_", "*", "_", "/", "_", ":", "_")
	return replacer.Replace(raw)
}
