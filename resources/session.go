package resources

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	yaml "go.yaml.in/yaml/v2"
)

const (
	SessionPhaseWaitingInput    = "WaitingInput"
	SessionPhaseRunning         = "Running"
	SessionPhasePaused          = "Paused"
	SessionPhaseWaitingApproval = "WaitingApproval"
	SessionPhaseFailed          = "Failed"
	SessionPhaseCancelled       = "Cancelled"
	SessionPhaseCompleted       = "Completed"
	SessionPhaseExpired         = "Expired"

	SessionTurnPhaseQueued    = "Queued"
	SessionTurnPhaseRunning   = "Running"
	SessionTurnPhaseSucceeded = "Succeeded"
	SessionTurnPhaseFailed    = "Failed"
	SessionTurnPhaseCancelled = "Cancelled"

	SessionEventSessionCreated    = "session.created"
	SessionEventSessionPaused     = "session.paused"
	SessionEventSessionResumed    = "session.resumed"
	SessionEventSessionCancelled  = "session.cancelled"
	SessionEventSessionCompleted  = "session.completed"
	SessionEventSessionExpired    = "session.expired"
	SessionEventTurnQueued        = "turn.queued"
	SessionEventTurnStarted       = "turn.started"
	SessionEventTurnRetrying      = "turn.retrying"
	SessionEventTurnCompleted     = "turn.completed"
	SessionEventTurnFailed        = "turn.failed"
	SessionEventTurnCancelled     = "turn.cancelled"
	SessionEventMessageCreated    = "message.created"
	SessionEventMessageDelta      = "message.delta"
	SessionEventMessageReset      = "message.reset"
	SessionEventMessageCompleted  = "message.completed"
	SessionEventApprovalRequested = "approval.requested"
	SessionEventApprovalResolved  = "approval.resolved"
	SessionEventToolStarted       = "tool.started"
	SessionEventToolCompleted     = "tool.completed"
	SessionEventCheckpointCreated = "checkpoint.created"
	SessionEventCheckpointPruned  = "checkpoint.pruned"
	SessionEventSessionRecovered  = "session.recovered"
	SessionEventSessionRewound    = "session.rewound"
	SessionEventSessionForked     = "session.forked"
	SessionEventError             = "error"

	SessionCheckpointStateVersion   = 1
	SessionCheckpointSafePointStep  = "step.completed"
	SessionCheckpointSafePointAgent = "agent.completed"
	SessionCheckpointSafePointTurn  = "turn.completed"
)

// Session is a durable conversation routed through an AgentSystem.
// Conversation events are stored separately and are intentionally not embedded
// in status so long-running sessions do not grow the resource row without bound.
type Session struct {
	APIVersion string        `json:"apiVersion" yaml:"apiVersion"`
	Kind       string        `json:"kind" yaml:"kind"`
	Metadata   ObjectMeta    `json:"metadata" yaml:"metadata"`
	Spec       SessionSpec   `json:"spec" yaml:"spec"`
	Status     SessionStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type SessionSpec struct {
	System              string                     `json:"system" yaml:"system"`
	IdleTTL             string                     `json:"idle_ttl,omitempty" yaml:"idle_ttl,omitempty"`
	MaxTurns            int                        `json:"max_turns,omitempty" yaml:"max_turns,omitempty"`
	Input               map[string]string          `json:"input,omitempty" yaml:"input,omitempty"`
	CheckpointRetention SessionCheckpointRetention `json:"checkpoint_retention,omitempty" yaml:"checkpoint_retention,omitempty"`
}

type SessionCheckpointRetention struct {
	MaxCount int    `json:"max_count,omitempty" yaml:"max_count,omitempty"`
	MaxAge   string `json:"max_age,omitempty" yaml:"max_age,omitempty"`
}

type SessionStatus struct {
	Phase              string         `json:"phase,omitempty" yaml:"phase,omitempty"`
	LastError          string         `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	StartedAt          string         `json:"startedAt,omitempty" yaml:"startedAt,omitempty"`
	CompletedAt        string         `json:"completedAt,omitempty" yaml:"completedAt,omitempty"`
	LastActivityAt     string         `json:"lastActivityAt,omitempty" yaml:"lastActivityAt,omitempty"`
	ExpiresAt          string         `json:"expiresAt,omitempty" yaml:"expiresAt,omitempty"`
	ActiveTurnID       string         `json:"activeTurnID,omitempty" yaml:"activeTurnID,omitempty"`
	QueuedTurns        int            `json:"queuedTurns,omitempty" yaml:"queuedTurns,omitempty"`
	CompletedTurns     int            `json:"completedTurns,omitempty" yaml:"completedTurns,omitempty"`
	LastEventSequence  uint64         `json:"lastEventSequence,omitempty" yaml:"lastEventSequence,omitempty"`
	LastCheckpointID   string         `json:"lastCheckpointID,omitempty" yaml:"lastCheckpointID,omitempty"`
	RestoredCheckpoint string         `json:"restoredCheckpoint,omitempty" yaml:"restoredCheckpoint,omitempty"`
	ClaimedBy          string         `json:"claimedBy,omitempty" yaml:"claimedBy,omitempty"`
	LeaseUntil         string         `json:"leaseUntil,omitempty" yaml:"leaseUntil,omitempty"`
	LastHeartbeat      string         `json:"lastHeartbeat,omitempty" yaml:"lastHeartbeat,omitempty"`
	Fence              int64          `json:"fence,omitempty" yaml:"fence,omitempty"`
	SystemGeneration   int64          `json:"systemGeneration,omitempty" yaml:"systemGeneration,omitempty"`
	BlockedOn          *TaskBlockedOn `json:"blockedOn,omitempty" yaml:"blockedOn,omitempty"`
	ObservedGeneration int64          `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
}

type SessionList struct {
	ListMeta `json:",inline" yaml:",inline"`
	Items    []Session `json:"items" yaml:"items"`
}

// SessionTurn is one user message and the bounded AgentSystem execution it
// triggers. Only one turn is executed at a time for a Session.
type SessionTurn struct {
	ID                 string `json:"id" yaml:"id"`
	SessionName        string `json:"session_name,omitempty" yaml:"session_name,omitempty"`
	Namespace          string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	QueueSequence      uint64 `json:"queue_sequence,omitempty" yaml:"queue_sequence,omitempty"`
	MessageID          string `json:"message_id,omitempty" yaml:"message_id,omitempty"`
	AssistantMessageID string `json:"assistant_message_id,omitempty" yaml:"assistant_message_id,omitempty"`
	Content            string `json:"content" yaml:"content"`
	Interrupt          bool   `json:"interrupt,omitempty" yaml:"interrupt,omitempty"`
	IdempotencyKey     string `json:"idempotency_key,omitempty" yaml:"idempotency_key,omitempty"`
	Phase              string `json:"phase,omitempty" yaml:"phase,omitempty"`
	Attempt            int    `json:"attempt,omitempty" yaml:"attempt,omitempty"`
	ClaimedBy          string `json:"claimed_by,omitempty" yaml:"claimed_by,omitempty"`
	LeaseUntil         string `json:"lease_until,omitempty" yaml:"lease_until,omitempty"`
	Fence              int64  `json:"fence,omitempty" yaml:"fence,omitempty"`
	CreatedAt          string `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	StartedAt          string `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	CompletedAt        string `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	LastError          string `json:"last_error,omitempty" yaml:"last_error,omitempty"`
}

// SessionEvent is the ordered, replayable source of conversation truth.
type SessionEvent struct {
	Sequence       uint64         `json:"seq" yaml:"seq"`
	ID             string         `json:"event_id" yaml:"event_id"`
	SessionName    string         `json:"session_name" yaml:"session_name"`
	Namespace      string         `json:"namespace" yaml:"namespace"`
	TurnID         string         `json:"turn_id,omitempty" yaml:"turn_id,omitempty"`
	MessageID      string         `json:"message_id,omitempty" yaml:"message_id,omitempty"`
	Attempt        int            `json:"attempt,omitempty" yaml:"attempt,omitempty"`
	CausationID    string         `json:"causation_id,omitempty" yaml:"causation_id,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty" yaml:"idempotency_key,omitempty"`
	Type           string         `json:"type" yaml:"type"`
	Timestamp      string         `json:"timestamp" yaml:"timestamp"`
	Payload        map[string]any `json:"payload,omitempty" yaml:"payload,omitempty"`
}

// SessionCheckpoint is a durable, versioned snapshot captured at a safe
// execution boundary. State is runtime-owned JSON so checkpoint schema
// versions can evolve independently from the Session API resource.
type SessionCheckpoint struct {
	ID                 string          `json:"id" yaml:"id"`
	SessionName        string          `json:"session_name" yaml:"session_name"`
	Namespace          string          `json:"namespace" yaml:"namespace"`
	TurnID             string          `json:"turn_id,omitempty" yaml:"turn_id,omitempty"`
	TaskName           string          `json:"task_name,omitempty" yaml:"task_name,omitempty"`
	Agent              string          `json:"agent,omitempty" yaml:"agent,omitempty"`
	AgentIndex         int             `json:"agent_index,omitempty" yaml:"agent_index,omitempty"`
	MessageID          string          `json:"message_id,omitempty" yaml:"message_id,omitempty"`
	BranchID           string          `json:"branch_id,omitempty" yaml:"branch_id,omitempty"`
	Attempt            int             `json:"attempt,omitempty" yaml:"attempt,omitempty"`
	Fence              int64           `json:"fence,omitempty" yaml:"fence,omitempty"`
	SystemGeneration   int64           `json:"system_generation,omitempty" yaml:"system_generation,omitempty"`
	EventSequence      uint64          `json:"event_sequence" yaml:"event_sequence"`
	ParentCheckpointID string          `json:"parent_checkpoint_id,omitempty" yaml:"parent_checkpoint_id,omitempty"`
	SafePoint          string          `json:"safe_point" yaml:"safe_point"`
	StateVersion       int             `json:"state_version" yaml:"state_version"`
	StateHash          string          `json:"state_hash" yaml:"state_hash"`
	State              json.RawMessage `json:"state" yaml:"state"`
	CreatedAt          string          `json:"created_at" yaml:"created_at"`
	ExpiresAt          string          `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

type SessionCheckpointList struct {
	Items []SessionCheckpoint `json:"items" yaml:"items"`
}

type SessionReplayResult struct {
	SessionName     string             `json:"session_name" yaml:"session_name"`
	CheckpointID    string             `json:"checkpoint_id" yaml:"checkpoint_id"`
	StateVersion    int                `json:"state_version" yaml:"state_version"`
	StateHash       string             `json:"state_hash" yaml:"state_hash"`
	Verified        bool               `json:"verified" yaml:"verified"`
	CheckpointCount int                `json:"checkpoint_count" yaml:"checkpoint_count"`
	Events          []SessionEvent     `json:"events,omitempty" yaml:"events,omitempty"`
	FinalCheckpoint *SessionCheckpoint `json:"final_checkpoint,omitempty" yaml:"final_checkpoint,omitempty"`
}

func (s *Session) Normalize() error {
	if s.APIVersion == "" {
		s.APIVersion = "orloj.dev/v1"
	}
	if s.Kind == "" {
		s.Kind = "Session"
	}
	if !strings.EqualFold(s.Kind, "Session") {
		return fmt.Errorf("unsupported kind %q for Session", s.Kind)
	}
	NormalizeObjectMetaNamespace(&s.Metadata)
	if err := ValidateMetadataName(s.Metadata.Name); err != nil {
		return err
	}
	s.Spec.System = strings.TrimSpace(s.Spec.System)
	if s.Spec.System == "" {
		return fmt.Errorf("spec.system is required")
	}
	s.Spec.IdleTTL = strings.TrimSpace(s.Spec.IdleTTL)
	if s.Spec.IdleTTL == "" {
		s.Spec.IdleTTL = "24h"
	}
	idleTTL, err := time.ParseDuration(s.Spec.IdleTTL)
	if err != nil || idleTTL <= 0 {
		return fmt.Errorf("invalid spec.idle_ttl %q: expected a positive duration", s.Spec.IdleTTL)
	}
	if s.Spec.MaxTurns < 0 {
		return fmt.Errorf("spec.max_turns cannot be negative")
	}
	if s.Spec.CheckpointRetention.MaxCount < 0 {
		return fmt.Errorf("spec.checkpoint_retention.max_count cannot be negative")
	}
	if s.Spec.CheckpointRetention.MaxCount == 0 {
		s.Spec.CheckpointRetention.MaxCount = 100
	}
	s.Spec.CheckpointRetention.MaxAge = strings.TrimSpace(s.Spec.CheckpointRetention.MaxAge)
	if s.Spec.CheckpointRetention.MaxAge == "" {
		s.Spec.CheckpointRetention.MaxAge = "168h"
	}
	checkpointMaxAge, err := time.ParseDuration(s.Spec.CheckpointRetention.MaxAge)
	if err != nil || checkpointMaxAge <= 0 {
		return fmt.Errorf(
			"invalid spec.checkpoint_retention.max_age %q: expected a positive duration",
			s.Spec.CheckpointRetention.MaxAge,
		)
	}
	if s.Spec.Input == nil {
		s.Spec.Input = map[string]string{}
	}
	if strings.TrimSpace(s.Status.Phase) == "" {
		s.Status.Phase = SessionPhaseWaitingInput
	}
	if !IsValidSessionPhase(s.Status.Phase) {
		return fmt.Errorf("invalid status.phase %q for Session", s.Status.Phase)
	}
	return nil
}

func IsValidSessionPhase(phase string) bool {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case strings.ToLower(SessionPhaseWaitingInput),
		strings.ToLower(SessionPhaseRunning),
		strings.ToLower(SessionPhasePaused),
		strings.ToLower(SessionPhaseWaitingApproval),
		strings.ToLower(SessionPhaseFailed),
		strings.ToLower(SessionPhaseCancelled),
		strings.ToLower(SessionPhaseCompleted),
		strings.ToLower(SessionPhaseExpired):
		return true
	default:
		return false
	}
}

func IsTerminalSessionPhase(phase string) bool {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case strings.ToLower(SessionPhaseFailed),
		strings.ToLower(SessionPhaseCancelled),
		strings.ToLower(SessionPhaseCompleted),
		strings.ToLower(SessionPhaseExpired):
		return true
	default:
		return false
	}
}

func ParseSessionManifest(data []byte) (Session, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return Session{}, err
	}
	var out Session
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return Session{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, &out); err != nil {
			return Session{}, fmt.Errorf("failed to decode YAML manifest: %w", err)
		}
	}
	if err := out.Normalize(); err != nil {
		return Session{}, err
	}
	return out, nil
}

func (s Session) DeepCopy() Session {
	out := s
	out.Metadata = copyObjectMeta(s.Metadata)
	out.Spec.Input = copyStringMap(s.Spec.Input)
	if s.Status.BlockedOn != nil {
		blocked := *s.Status.BlockedOn
		out.Status.BlockedOn = &blocked
	}
	return out
}

func (e SessionEvent) DeepCopy() SessionEvent {
	out := e
	if e.Payload != nil {
		raw, _ := json.Marshal(e.Payload)
		_ = json.Unmarshal(raw, &out.Payload)
	}
	return out
}

func (c SessionCheckpoint) DeepCopy() SessionCheckpoint {
	out := c
	out.State = append(json.RawMessage(nil), c.State...)
	return out
}
