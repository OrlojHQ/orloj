package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/google/uuid"
)

const defaultSessionEventLimit = 1000

type SessionStore struct {
	mu          sync.RWMutex
	items       map[string]resources.Session
	turns       map[string][]resources.SessionTurn
	events      map[string][]resources.SessionEvent
	checkpoints map[string][]resources.SessionCheckpoint
	db          *sql.DB
}

type SessionClaim struct {
	Session    resources.Session
	Turn       resources.SessionTurn
	Checkpoint *resources.SessionCheckpoint
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		items:       make(map[string]resources.Session),
		turns:       make(map[string][]resources.SessionTurn),
		events:      make(map[string][]resources.SessionEvent),
		checkpoints: make(map[string][]resources.SessionCheckpoint),
	}
}

func NewSessionStoreWithDB(db *sql.DB) *SessionStore {
	s := NewSessionStore()
	s.db = db
	return s
}

func (s *SessionStore) Upsert(ctx context.Context, item resources.Session) (resources.Session, error) {
	if err := item.Normalize(); err != nil {
		return resources.Session{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	now := time.Now().UTC()

	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return resources.Session{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.Session](ctx, tx, tableSessions, key)
		if err != nil {
			return resources.Session{}, err
		}
		if !found {
			if err := initializeCreateMetadata("Session", &item.Metadata); err != nil {
				return resources.Session{}, err
			}
			initializeSessionStatus(&item, now)
			item.Status.LastEventSequence = 1
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("Session", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.Session{}, err
			}
		}
		item.Status.ObservedGeneration = item.Metadata.Generation
		if err := upsertSessionSQL(ctx, tx, key, item); err != nil {
			return resources.Session{}, err
		}
		if !found {
			evt := newSessionEvent(item, resources.SessionEventSessionCreated, "", "", 0, map[string]any{
				"system": item.Spec.System,
			})
			evt.Sequence = 1
			if err := insertSessionEventSQL(ctx, tx, key, evt); err != nil {
				return resources.Session{}, err
			}
		}
		if err := tx.Commit(); err != nil {
			return resources.Session{}, err
		}
		return item, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("Session", &item.Metadata); err != nil {
			return resources.Session{}, err
		}
		initializeSessionStatus(&item, now)
		item.Status.LastEventSequence = 1
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("Session", &item.Metadata, existing.Metadata, specChanged); err != nil {
			return resources.Session{}, err
		}
	}
	item.Status.ObservedGeneration = item.Metadata.Generation
	stored := item.DeepCopy()
	s.items[key] = stored
	if !found {
		evt := newSessionEvent(stored, resources.SessionEventSessionCreated, "", "", 0, map[string]any{
			"system": stored.Spec.System,
		})
		evt.Sequence = 1
		s.events[key] = append(s.events[key], evt)
	}
	return stored.DeepCopy(), nil
}

func initializeSessionStatus(item *resources.Session, now time.Time) {
	if strings.TrimSpace(item.Status.Phase) == "" {
		item.Status.Phase = resources.SessionPhaseWaitingInput
	}
	if strings.TrimSpace(item.Status.StartedAt) == "" {
		item.Status.StartedAt = now.Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(item.Status.LastActivityAt) == "" {
		item.Status.LastActivityAt = now.Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(item.Status.ExpiresAt) == "" {
		if ttl, err := time.ParseDuration(item.Spec.IdleTTL); err == nil {
			item.Status.ExpiresAt = now.Add(ttl).Format(time.RFC3339Nano)
		}
	}
}

func (s *SessionStore) Get(ctx context.Context, name string) (resources.Session, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		item, ok, err := getFromTable[resources.Session](ctx, s.db, tableSessions, key)
		return item, ok, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	if !ok {
		return resources.Session{}, false, nil
	}
	return item.DeepCopy(), true, nil
}

func (s *SessionStore) List(ctx context.Context) ([]resources.Session, error) {
	if s.db != nil {
		return listFromTable[resources.Session](ctx, s.db, tableSessions)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.Session, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item.DeepCopy())
	}
	sort.Slice(out, func(i, j int) bool {
		return scopedNameFromMeta(out[i].Metadata) < scopedNameFromMeta(out[j].Metadata)
	})
	return out, nil
}

func (s *SessionStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.Session, error) {
	if s.db != nil {
		return listFromTableCursor[resources.Session](ctx, s.db, tableSessions, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.Session, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item.DeepCopy())
	}
	sort.Slice(out, func(i, j int) bool {
		return scopedNameFromMeta(out[i].Metadata) < scopedNameFromMeta(out[j].Metadata)
	})
	return cursorFilter(out,
		func(item resources.Session) string { return item.Metadata.Name },
		func(item resources.Session) string { return resources.NormalizeNamespace(item.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *SessionStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableSessions, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("session %q not found", name)
		}
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("session %q not found", name)
	}
	delete(s.items, key)
	delete(s.turns, key)
	delete(s.events, key)
	delete(s.checkpoints, key)
	return nil
}

func (s *SessionStore) EnqueueTurn(ctx context.Context, name string, turn resources.SessionTurn) (resources.SessionTurn, []resources.SessionEvent, bool, error) {
	key := normalizeLookupName(name)
	turn.Content = strings.TrimSpace(turn.Content)
	turn.IdempotencyKey = strings.TrimSpace(turn.IdempotencyKey)
	if turn.Content == "" {
		return resources.SessionTurn{}, nil, false, fmt.Errorf("turn content is required")
	}
	if turn.IdempotencyKey == "" {
		return resources.SessionTurn{}, nil, false, fmt.Errorf("idempotency key is required")
	}
	if s.db != nil {
		return s.enqueueTurnSQL(ctx, key, turn)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.items[key]
	if !ok {
		return resources.SessionTurn{}, nil, false, fmt.Errorf("session %q not found", name)
	}
	for _, existing := range s.turns[key] {
		if existing.IdempotencyKey == turn.IdempotencyKey {
			return existing, nil, false, nil
		}
	}
	if err := validateTurnEnqueue(session, turn.Interrupt); err != nil {
		return resources.SessionTurn{}, nil, false, err
	}
	now := time.Now().UTC()
	events := make([]resources.SessionEvent, 0, 4)
	if turn.Interrupt && session.Status.ActiveTurnID != "" {
		for i := range s.turns[key] {
			if s.turns[key][i].ID == session.Status.ActiveTurnID {
				events = append(events, interruptSessionTurn(&session, &s.turns[key][i], now)...)
				break
			}
		}
	}
	prepareTurn(&turn, session, now)
	queuedEvents := enqueueEvents(session, turn, now)
	turn.QueueSequence = queuedEvents[0].Sequence
	events = append(events, queuedEvents...)
	session.Status.LastEventSequence = events[len(events)-1].Sequence
	session.Status.QueuedTurns++
	touchSession(&session, now)
	if err := bumpSessionStatusMetadata(&session, s.items[key].Metadata); err != nil {
		return resources.SessionTurn{}, nil, false, err
	}
	s.items[key] = session.DeepCopy()
	s.turns[key] = append(s.turns[key], turn)
	s.events[key] = append(s.events[key], events...)
	return turn, copySessionEvents(events), true, nil
}

func validateTurnEnqueue(session resources.Session, interrupt bool) error {
	if resources.IsTerminalSessionPhase(session.Status.Phase) {
		return fmt.Errorf("session %q is %s", session.Metadata.Name, session.Status.Phase)
	}
	if expiresAt, err := time.Parse(time.RFC3339Nano, session.Status.ExpiresAt); err == nil && !expiresAt.After(time.Now().UTC()) {
		return fmt.Errorf("session %q has expired", session.Metadata.Name)
	}
	activeTurns := 0
	if session.Status.ActiveTurnID != "" && !interrupt {
		activeTurns = 1
	}
	if session.Spec.MaxTurns > 0 && session.Status.CompletedTurns+session.Status.QueuedTurns+activeTurns >= session.Spec.MaxTurns {
		return fmt.Errorf("session %q reached max_turns=%d", session.Metadata.Name, session.Spec.MaxTurns)
	}
	return nil
}

func prepareTurn(turn *resources.SessionTurn, session resources.Session, now time.Time) {
	if strings.TrimSpace(turn.ID) == "" {
		turn.ID = newUUID()
	}
	if strings.TrimSpace(turn.MessageID) == "" {
		turn.MessageID = newUUID()
	}
	if strings.TrimSpace(turn.AssistantMessageID) == "" {
		turn.AssistantMessageID = newUUID()
	}
	turn.SessionName = session.Metadata.Name
	turn.Namespace = resources.NormalizeNamespace(session.Metadata.Namespace)
	turn.Phase = resources.SessionTurnPhaseQueued
	turn.CreatedAt = now.Format(time.RFC3339Nano)
}

func enqueueEvents(session resources.Session, turn resources.SessionTurn, now time.Time) []resources.SessionEvent {
	queued := newSessionEventAt(session, resources.SessionEventTurnQueued, turn.ID, turn.MessageID, 0, map[string]any{
		"interrupt": turn.Interrupt,
	}, now)
	queued.IdempotencyKey = turn.IdempotencyKey
	queued.Sequence = session.Status.LastEventSequence + 1
	message := newSessionEventAt(session, resources.SessionEventMessageCreated, turn.ID, turn.MessageID, 0, map[string]any{
		"role":    "user",
		"content": turn.Content,
	}, now)
	message.CausationID = queued.ID
	message.Sequence = queued.Sequence + 1
	return []resources.SessionEvent{queued, message}
}

func interruptSessionTurn(session *resources.Session, active *resources.SessionTurn, now time.Time) []resources.SessionEvent {
	reset := newSessionEventAt(*session, resources.SessionEventMessageReset, active.ID, active.AssistantMessageID, active.Attempt, map[string]any{
		"reason": "interrupted by a new turn",
	}, now)
	reset.Sequence = session.Status.LastEventSequence + 1
	cancelled := newSessionEventAt(*session, resources.SessionEventTurnCancelled, active.ID, active.AssistantMessageID, active.Attempt, map[string]any{
		"reason": "interrupted by a new turn",
	}, now)
	cancelled.Sequence = reset.Sequence + 1
	cancelled.CausationID = reset.ID
	active.Phase = resources.SessionTurnPhaseCancelled
	active.ClaimedBy = ""
	active.LeaseUntil = ""
	active.CompletedAt = now.Format(time.RFC3339Nano)
	active.LastError = "interrupted by a new turn"
	session.Status.Phase = resources.SessionPhaseWaitingInput
	session.Status.ActiveTurnID = ""
	session.Status.ClaimedBy = ""
	session.Status.LeaseUntil = ""
	session.Status.LastHeartbeat = ""
	session.Status.BlockedOn = nil
	session.Status.Fence++
	session.Status.LastEventSequence = cancelled.Sequence
	return []resources.SessionEvent{reset, cancelled}
}

func (s *SessionStore) enqueueTurnSQL(ctx context.Context, key string, turn resources.SessionTurn) (resources.SessionTurn, []resources.SessionEvent, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return resources.SessionTurn{}, nil, false, err
	}
	defer tx.Rollback()
	session, found, err := getFromTableForUpdate[resources.Session](ctx, tx, tableSessions, key)
	if err != nil {
		return resources.SessionTurn{}, nil, false, err
	}
	if !found {
		return resources.SessionTurn{}, nil, false, fmt.Errorf("session %q not found", key)
	}
	var existingPayload []byte
	err = tx.QueryRowContext(ctx,
		`SELECT payload FROM session_turns WHERE session_name = $1 AND idempotency_key = $2`,
		key, turn.IdempotencyKey,
	).Scan(&existingPayload)
	if err == nil {
		var existing resources.SessionTurn
		if err := json.Unmarshal(existingPayload, &existing); err != nil {
			return resources.SessionTurn{}, nil, false, err
		}
		if err := tx.Commit(); err != nil {
			return resources.SessionTurn{}, nil, false, err
		}
		return existing, nil, false, nil
	}
	if err != sql.ErrNoRows {
		return resources.SessionTurn{}, nil, false, err
	}
	if err := validateTurnEnqueue(session, turn.Interrupt); err != nil {
		return resources.SessionTurn{}, nil, false, err
	}
	now := time.Now().UTC()
	events := make([]resources.SessionEvent, 0, 4)
	if turn.Interrupt && session.Status.ActiveTurnID != "" {
		var raw []byte
		if err := tx.QueryRowContext(ctx,
			`SELECT payload FROM session_turns WHERE session_name = $1 AND turn_id = $2 FOR UPDATE`,
			key, session.Status.ActiveTurnID,
		).Scan(&raw); err != nil {
			return resources.SessionTurn{}, nil, false, err
		}
		var active resources.SessionTurn
		if err := json.Unmarshal(raw, &active); err != nil {
			return resources.SessionTurn{}, nil, false, err
		}
		events = append(events, interruptSessionTurn(&session, &active, now)...)
		if err := updateSessionTurnSQL(ctx, tx, key, active); err != nil {
			return resources.SessionTurn{}, nil, false, err
		}
	}
	prepareTurn(&turn, session, now)
	queuedEvents := enqueueEvents(session, turn, now)
	turn.QueueSequence = queuedEvents[0].Sequence
	events = append(events, queuedEvents...)
	session.Status.LastEventSequence = events[len(events)-1].Sequence
	session.Status.QueuedTurns++
	touchSession(&session, now)
	if err := bumpSessionStatusMetadata(&session, session.Metadata); err != nil {
		return resources.SessionTurn{}, nil, false, err
	}
	if err := insertSessionTurnSQL(ctx, tx, key, turn); err != nil {
		return resources.SessionTurn{}, nil, false, err
	}
	for _, evt := range events {
		if err := insertSessionEventSQL(ctx, tx, key, evt); err != nil {
			return resources.SessionTurn{}, nil, false, err
		}
	}
	if err := upsertSessionSQL(ctx, tx, key, session); err != nil {
		return resources.SessionTurn{}, nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return resources.SessionTurn{}, nil, false, err
	}
	return turn, events, true, nil
}

func (s *SessionStore) ExpireIdleSessions(ctx context.Context) ([]resources.SessionEvent, error) {
	now := time.Now().UTC()
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		rows, err := tx.QueryContext(ctx, `
			SELECT name
			FROM sessions
			WHERE expires_at IS NOT NULL
			  AND expires_at <= NOW()
			  AND status_phase NOT IN ('running', 'waitingapproval', 'failed', 'cancelled', 'completed', 'expired')
			ORDER BY expires_at
			FOR UPDATE SKIP LOCKED`)
		if err != nil {
			return nil, err
		}
		var keys []string
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				rows.Close()
				return nil, err
			}
			keys = append(keys, key)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		events := make([]resources.SessionEvent, 0, len(keys))
		for _, key := range keys {
			session, found, err := getFromTableForUpdate[resources.Session](ctx, tx, tableSessions, key)
			if err != nil || !found {
				if err != nil {
					return nil, err
				}
				continue
			}
			evt := expireSession(&session, now)
			if err := cancelOpenSessionTurnsSQL(ctx, tx, key, now, "session expired", false); err != nil {
				return nil, err
			}
			if err := bumpSessionStatusMetadata(&session, session.Metadata); err != nil {
				return nil, err
			}
			if err := insertSessionEventSQL(ctx, tx, key, evt); err != nil {
				return nil, err
			}
			if err := upsertSessionSQL(ctx, tx, key, session); err != nil {
				return nil, err
			}
			events = append(events, evt)
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return events, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	events := make([]resources.SessionEvent, 0)
	for key, current := range s.items {
		if !sessionIdleExpired(current, now) {
			continue
		}
		session := current.DeepCopy()
		evt := expireSession(&session, now)
		for i := range s.turns[key] {
			if strings.EqualFold(s.turns[key][i].Phase, resources.SessionTurnPhaseQueued) {
				s.turns[key][i].Phase = resources.SessionTurnPhaseCancelled
				s.turns[key][i].ClaimedBy = ""
				s.turns[key][i].LeaseUntil = ""
				s.turns[key][i].Fence = 0
				s.turns[key][i].CompletedAt = now.Format(time.RFC3339Nano)
				s.turns[key][i].LastError = "session expired"
			}
		}
		if err := bumpSessionStatusMetadata(&session, current.Metadata); err != nil {
			return nil, err
		}
		s.items[key] = session
		s.events[key] = append(s.events[key], evt)
		events = append(events, evt)
	}
	return copySessionEvents(events), nil
}

func sessionIdleExpired(session resources.Session, now time.Time) bool {
	if resources.IsTerminalSessionPhase(session.Status.Phase) ||
		strings.EqualFold(session.Status.Phase, resources.SessionPhaseRunning) ||
		strings.EqualFold(session.Status.Phase, resources.SessionPhaseWaitingApproval) {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, session.Status.ExpiresAt)
	return err == nil && !expiresAt.After(now)
}

func expireSession(session *resources.Session, now time.Time) resources.SessionEvent {
	session.Status.Phase = resources.SessionPhaseExpired
	session.Status.CompletedAt = now.Format(time.RFC3339Nano)
	session.Status.ActiveTurnID = ""
	session.Status.QueuedTurns = 0
	session.Status.ClaimedBy = ""
	session.Status.LeaseUntil = ""
	session.Status.LastHeartbeat = ""
	session.Status.BlockedOn = nil
	session.Status.Fence++
	evt := newSessionEventAt(*session, resources.SessionEventSessionExpired, "", "", 0, map[string]any{
		"reason": "idle TTL elapsed",
	}, now)
	evt.Sequence = session.Status.LastEventSequence + 1
	session.Status.LastEventSequence = evt.Sequence
	return evt
}

func (s *SessionStore) ClaimNextTurn(ctx context.Context, workerID string, lease time.Duration) (SessionClaim, bool, []resources.SessionEvent, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return SessionClaim{}, false, nil, fmt.Errorf("worker ID is required")
	}
	if lease <= 0 {
		lease = 30 * time.Second
	}
	if s.db != nil {
		return s.claimNextTurnSQL(ctx, workerID, lease)
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	keys := make([]string, 0, len(s.items))
	for key := range s.items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var chosenKey string
	var chosenIndex = -1
	var chosenSeq uint64 = ^uint64(0)
	for _, key := range keys {
		session := s.items[key]
		if !sessionCanClaim(session, now) {
			continue
		}
		for i, turn := range s.turns[key] {
			if !turnCanClaim(turn, now) || turn.QueueSequence >= chosenSeq {
				continue
			}
			chosenKey, chosenIndex, chosenSeq = key, i, turn.QueueSequence
		}
	}
	if chosenIndex < 0 {
		return SessionClaim{}, false, nil, nil
	}
	session := s.items[chosenKey]
	turn := s.turns[chosenKey][chosenIndex]
	var latestCheckpoint *resources.SessionCheckpoint
	if candidate, found, err := selectExecutionCheckpointFromLineage(
		session,
		s.checkpoints[chosenKey],
		turn.ID,
		"",
		-1,
		"",
		"",
	); err != nil {
		return SessionClaim{}, false, nil, err
	} else if found {
		if checkpointStateHash(candidate.State) != candidate.StateHash {
			return SessionClaim{}, false, nil, fmt.Errorf("checkpoint %q state hash mismatch", candidate.ID)
		}
		copied := candidate.DeepCopy()
		latestCheckpoint = &copied
	}
	checkpointID := ""
	if latestCheckpoint != nil {
		checkpointID = latestCheckpoint.ID
	}
	events := applyTurnClaim(&session, &turn, workerID, lease, now, checkpointID)
	if err := bumpSessionStatusMetadata(&session, s.items[chosenKey].Metadata); err != nil {
		return SessionClaim{}, false, nil, err
	}
	s.items[chosenKey] = session.DeepCopy()
	s.turns[chosenKey][chosenIndex] = turn
	s.events[chosenKey] = append(s.events[chosenKey], events...)
	return SessionClaim{Session: session.DeepCopy(), Turn: turn, Checkpoint: latestCheckpoint}, true, copySessionEvents(events), nil
}

func sessionCanClaim(session resources.Session, now time.Time) bool {
	if resources.IsTerminalSessionPhase(session.Status.Phase) ||
		strings.EqualFold(session.Status.Phase, resources.SessionPhasePaused) {
		return false
	}
	if expiresAt, err := time.Parse(time.RFC3339Nano, session.Status.ExpiresAt); err == nil && !expiresAt.After(now) {
		return false
	}
	if !strings.EqualFold(session.Status.Phase, resources.SessionPhaseRunning) &&
		!strings.EqualFold(session.Status.Phase, resources.SessionPhaseWaitingApproval) {
		return true
	}
	until, err := time.Parse(time.RFC3339Nano, session.Status.LeaseUntil)
	return err != nil || !until.After(now)
}

func turnCanClaim(turn resources.SessionTurn, now time.Time) bool {
	if strings.EqualFold(turn.Phase, resources.SessionTurnPhaseQueued) {
		return true
	}
	if !strings.EqualFold(turn.Phase, resources.SessionTurnPhaseRunning) {
		return false
	}
	until, err := time.Parse(time.RFC3339Nano, turn.LeaseUntil)
	return err != nil || !until.After(now)
}

func applyTurnClaim(
	session *resources.Session,
	turn *resources.SessionTurn,
	workerID string,
	lease time.Duration,
	now time.Time,
	checkpointID string,
) []resources.SessionEvent {
	retry := strings.EqualFold(turn.Phase, resources.SessionTurnPhaseRunning)
	turn.Phase = resources.SessionTurnPhaseRunning
	turn.Attempt++
	turn.ClaimedBy = workerID
	turn.LeaseUntil = now.Add(lease).Format(time.RFC3339Nano)
	turn.StartedAt = now.Format(time.RFC3339Nano)
	turn.LastError = ""
	session.Status.Fence++
	turn.Fence = session.Status.Fence
	session.Status.Phase = resources.SessionPhaseRunning
	session.Status.ActiveTurnID = turn.ID
	session.Status.ClaimedBy = workerID
	session.Status.LeaseUntil = turn.LeaseUntil
	session.Status.LastHeartbeat = now.Format(time.RFC3339Nano)
	if !retry && session.Status.QueuedTurns > 0 {
		session.Status.QueuedTurns--
	}
	touchSession(session, now)

	events := make([]resources.SessionEvent, 0, 3)
	if retry {
		retrying := newSessionEventAt(*session, resources.SessionEventTurnRetrying, turn.ID, turn.AssistantMessageID, turn.Attempt, map[string]any{
			"reason": "worker lease expired",
		}, now)
		retrying.Sequence = session.Status.LastEventSequence + 1
		events = append(events, retrying)
		if checkpointID != "" {
			reset := newSessionEventAt(*session, resources.SessionEventMessageReset, turn.ID, turn.AssistantMessageID, turn.Attempt, map[string]any{
				"role":          "assistant",
				"checkpoint_id": checkpointID,
				"reason":        "discard output emitted after the recovered checkpoint",
			}, now)
			reset.Sequence = retrying.Sequence + 1
			reset.CausationID = retrying.ID
			events = append(events, reset)
			recovered := newSessionEventAt(*session, resources.SessionEventSessionRecovered, turn.ID, turn.AssistantMessageID, turn.Attempt, map[string]any{
				"checkpoint_id": checkpointID,
			}, now)
			recovered.Sequence = reset.Sequence + 1
			recovered.CausationID = reset.ID
			events = append(events, recovered)
			session.Status.RestoredCheckpoint = checkpointID
		} else {
			reset := newSessionEventAt(*session, resources.SessionEventMessageReset, turn.ID, turn.AssistantMessageID, turn.Attempt, map[string]any{
				"role": "assistant",
			}, now)
			reset.Sequence = retrying.Sequence + 1
			reset.CausationID = retrying.ID
			events = append(events, reset)
		}
	}
	started := newSessionEventAt(*session, resources.SessionEventTurnStarted, turn.ID, turn.AssistantMessageID, turn.Attempt, map[string]any{
		"worker": workerID,
		"fence":  turn.Fence,
	}, now)
	started.Sequence = session.Status.LastEventSequence + uint64(len(events)) + 1
	events = append(events, started)
	session.Status.LastEventSequence = started.Sequence
	return events
}

func (s *SessionStore) claimNextTurnSQL(ctx context.Context, workerID string, lease time.Duration) (SessionClaim, bool, []resources.SessionEvent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionClaim{}, false, nil, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	var key, turnID string
	err = tx.QueryRowContext(ctx, `
		SELECT st.session_name, st.turn_id
		FROM session_turns st
		JOIN sessions s ON s.name = st.session_name
		WHERE (st.status_phase = 'queued' OR (st.status_phase = 'running' AND (st.lease_until IS NULL OR st.lease_until <= NOW())))
		  AND s.status_phase NOT IN ('paused', 'failed', 'cancelled', 'completed', 'expired')
		  AND (s.expires_at IS NULL OR s.expires_at > NOW())
		  AND (s.status_phase NOT IN ('running', 'waitingapproval') OR s.lease_until IS NULL OR s.lease_until <= NOW())
		ORDER BY st.queue_sequence ASC
		FOR UPDATE OF s SKIP LOCKED
		LIMIT 1`).Scan(&key, &turnID)
	if err == sql.ErrNoRows {
		if err := tx.Commit(); err != nil {
			return SessionClaim{}, false, nil, err
		}
		return SessionClaim{}, false, nil, nil
	}
	if err != nil {
		return SessionClaim{}, false, nil, err
	}
	session, found, err := getFromTableForUpdate[resources.Session](ctx, tx, tableSessions, key)
	if err != nil || !found {
		if err == nil {
			err = fmt.Errorf("session %q not found", key)
		}
		return SessionClaim{}, false, nil, err
	}
	var raw []byte
	if err := tx.QueryRowContext(ctx,
		`SELECT payload FROM session_turns WHERE session_name = $1 AND turn_id = $2 FOR UPDATE`,
		key, turnID,
	).Scan(&raw); err != nil {
		return SessionClaim{}, false, nil, err
	}
	var turn resources.SessionTurn
	if err := json.Unmarshal(raw, &turn); err != nil {
		return SessionClaim{}, false, nil, err
	}
	var latestCheckpoint *resources.SessionCheckpoint
	checkpointRows, err := tx.QueryContext(ctx, `
		SELECT payload
		FROM session_checkpoints
		WHERE session_name = $1
		ORDER BY event_seq DESC, created_at DESC`,
		key,
	)
	if err != nil {
		return SessionClaim{}, false, nil, err
	}
	checkpoints := make([]resources.SessionCheckpoint, 0)
	for checkpointRows.Next() {
		var checkpointRaw []byte
		if err := checkpointRows.Scan(&checkpointRaw); err != nil {
			checkpointRows.Close()
			return SessionClaim{}, false, nil, err
		}
		var checkpoint resources.SessionCheckpoint
		if err := json.Unmarshal(checkpointRaw, &checkpoint); err != nil {
			checkpointRows.Close()
			return SessionClaim{}, false, nil, err
		}
		checkpoints = append(checkpoints, checkpoint)
	}
	if err := checkpointRows.Close(); err != nil {
		return SessionClaim{}, false, nil, err
	}
	if checkpoint, found, err := selectExecutionCheckpointFromLineage(
		session,
		checkpoints,
		turn.ID,
		"",
		-1,
		"",
		"",
	); err != nil {
		return SessionClaim{}, false, nil, err
	} else if found {
		if checkpointStateHash(checkpoint.State) != checkpoint.StateHash {
			return SessionClaim{}, false, nil, fmt.Errorf("checkpoint %q state hash mismatch", checkpoint.ID)
		}
		latestCheckpoint = &checkpoint
	}
	checkpointID := ""
	if latestCheckpoint != nil {
		checkpointID = latestCheckpoint.ID
	}
	events := applyTurnClaim(&session, &turn, workerID, lease, now, checkpointID)
	if err := bumpSessionStatusMetadata(&session, session.Metadata); err != nil {
		return SessionClaim{}, false, nil, err
	}
	if err := updateSessionTurnSQL(ctx, tx, key, turn); err != nil {
		return SessionClaim{}, false, nil, err
	}
	for _, evt := range events {
		if err := insertSessionEventSQL(ctx, tx, key, evt); err != nil {
			return SessionClaim{}, false, nil, err
		}
	}
	if err := upsertSessionSQL(ctx, tx, key, session); err != nil {
		return SessionClaim{}, false, nil, err
	}
	if err := tx.Commit(); err != nil {
		return SessionClaim{}, false, nil, err
	}
	return SessionClaim{Session: session, Turn: turn, Checkpoint: latestCheckpoint}, true, events, nil
}

func (s *SessionStore) AppendEvent(ctx context.Context, name, turnID, workerID string, fence int64, evt resources.SessionEvent) (resources.SessionEvent, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return resources.SessionEvent{}, err
		}
		defer tx.Rollback()
		session, found, err := getFromTableForUpdate[resources.Session](ctx, tx, tableSessions, key)
		if err != nil || !found {
			if err == nil {
				err = fmt.Errorf("session %q not found", name)
			}
			return resources.SessionEvent{}, err
		}
		if err := validateSessionFence(session, turnID, workerID, fence); err != nil {
			return resources.SessionEvent{}, err
		}
		prepareAppendedEvent(&evt, session, turnID)
		session.Status.LastEventSequence = evt.Sequence
		touchSession(&session, time.Now().UTC())
		if err := bumpSessionStatusMetadata(&session, session.Metadata); err != nil {
			return resources.SessionEvent{}, err
		}
		if err := insertSessionEventSQL(ctx, tx, key, evt); err != nil {
			return resources.SessionEvent{}, err
		}
		if err := upsertSessionSQL(ctx, tx, key, session); err != nil {
			return resources.SessionEvent{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.SessionEvent{}, err
		}
		return evt, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.items[key]
	if !ok {
		return resources.SessionEvent{}, fmt.Errorf("session %q not found", name)
	}
	if err := validateSessionFence(session, turnID, workerID, fence); err != nil {
		return resources.SessionEvent{}, err
	}
	prepareAppendedEvent(&evt, session, turnID)
	session.Status.LastEventSequence = evt.Sequence
	touchSession(&session, time.Now().UTC())
	if err := bumpSessionStatusMetadata(&session, s.items[key].Metadata); err != nil {
		return resources.SessionEvent{}, err
	}
	s.items[key] = session.DeepCopy()
	s.events[key] = append(s.events[key], evt.DeepCopy())
	return evt, nil
}

func prepareAppendedEvent(evt *resources.SessionEvent, session resources.Session, turnID string) {
	if strings.TrimSpace(evt.ID) == "" {
		evt.ID = newUUID()
	}
	evt.Sequence = session.Status.LastEventSequence + 1
	evt.SessionName = session.Metadata.Name
	evt.Namespace = resources.NormalizeNamespace(session.Metadata.Namespace)
	if strings.TrimSpace(evt.TurnID) == "" {
		evt.TurnID = turnID
	}
	if strings.TrimSpace(evt.Timestamp) == "" {
		evt.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
}

func validateSessionFence(session resources.Session, turnID, workerID string, fence int64) error {
	if !strings.EqualFold(session.Status.Phase, resources.SessionPhaseRunning) &&
		!strings.EqualFold(session.Status.Phase, resources.SessionPhaseWaitingApproval) {
		return fmt.Errorf("session %q is not running or waiting for approval", session.Metadata.Name)
	}
	if strings.TrimSpace(session.Status.ActiveTurnID) != strings.TrimSpace(turnID) {
		return fmt.Errorf("session %q active turn changed", session.Metadata.Name)
	}
	if strings.TrimSpace(session.Status.ClaimedBy) != strings.TrimSpace(workerID) {
		return fmt.Errorf("session %q is claimed by %q", session.Metadata.Name, session.Status.ClaimedBy)
	}
	if session.Status.Fence != fence {
		return fmt.Errorf("session %q fence changed: expected=%d current=%d", session.Metadata.Name, fence, session.Status.Fence)
	}
	return nil
}

func (s *SessionStore) SetApprovalState(
	ctx context.Context,
	claim SessionClaim,
	waiting bool,
	blockedOn *resources.TaskBlockedOn,
) ([]resources.SessionEvent, resources.Session, error) {
	key := scopedNameFromMeta(claim.Session.Metadata)
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, resources.Session{}, err
		}
		defer tx.Rollback()
		session, found, err := getFromTableForUpdate[resources.Session](ctx, tx, tableSessions, key)
		if err != nil || !found {
			if err == nil {
				err = fmt.Errorf("session %q not found", key)
			}
			return nil, resources.Session{}, err
		}
		if err := validateSessionFence(session, claim.Turn.ID, claim.Turn.ClaimedBy, claim.Turn.Fence); err != nil {
			return nil, resources.Session{}, err
		}
		events := applySessionApprovalState(&session, claim.Turn, waiting, blockedOn, time.Now().UTC())
		if len(events) == 0 {
			if err := tx.Commit(); err != nil {
				return nil, resources.Session{}, err
			}
			return nil, session, nil
		}
		if err := bumpSessionStatusMetadata(&session, session.Metadata); err != nil {
			return nil, resources.Session{}, err
		}
		for _, evt := range events {
			if err := insertSessionEventSQL(ctx, tx, key, evt); err != nil {
				return nil, resources.Session{}, err
			}
		}
		if err := upsertSessionSQL(ctx, tx, key, session); err != nil {
			return nil, resources.Session{}, err
		}
		if err := tx.Commit(); err != nil {
			return nil, resources.Session{}, err
		}
		return events, session, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.items[key]
	if !ok {
		return nil, resources.Session{}, fmt.Errorf("session %q not found", key)
	}
	if err := validateSessionFence(session, claim.Turn.ID, claim.Turn.ClaimedBy, claim.Turn.Fence); err != nil {
		return nil, resources.Session{}, err
	}
	events := applySessionApprovalState(&session, claim.Turn, waiting, blockedOn, time.Now().UTC())
	if len(events) == 0 {
		return nil, session.DeepCopy(), nil
	}
	if err := bumpSessionStatusMetadata(&session, s.items[key].Metadata); err != nil {
		return nil, resources.Session{}, err
	}
	s.items[key] = session.DeepCopy()
	s.events[key] = append(s.events[key], events...)
	return copySessionEvents(events), session.DeepCopy(), nil
}

func applySessionApprovalState(
	session *resources.Session,
	turn resources.SessionTurn,
	waiting bool,
	blockedOn *resources.TaskBlockedOn,
	now time.Time,
) []resources.SessionEvent {
	if waiting == strings.EqualFold(session.Status.Phase, resources.SessionPhaseWaitingApproval) {
		return nil
	}
	eventType := resources.SessionEventApprovalResolved
	payload := map[string]any{}
	if waiting {
		session.Status.Phase = resources.SessionPhaseWaitingApproval
		eventType = resources.SessionEventApprovalRequested
		if blockedOn != nil {
			copy := *blockedOn
			session.Status.BlockedOn = &copy
			payload["kind"] = copy.Kind
			payload["name"] = copy.Name
			payload["reason"] = copy.Reason
		}
	} else {
		session.Status.Phase = resources.SessionPhaseRunning
		if session.Status.BlockedOn != nil {
			payload["kind"] = session.Status.BlockedOn.Kind
			payload["name"] = session.Status.BlockedOn.Name
		}
		session.Status.BlockedOn = nil
	}
	evt := newSessionEventAt(*session, eventType, turn.ID, turn.AssistantMessageID, turn.Attempt, payload, now)
	evt.Sequence = session.Status.LastEventSequence + 1
	session.Status.LastEventSequence = evt.Sequence
	touchSession(session, now)
	return []resources.SessionEvent{evt}
}

func (s *SessionStore) RenewLease(ctx context.Context, name, turnID, workerID string, fence int64, lease time.Duration) error {
	key := normalizeLookupName(name)
	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		session, found, err := getFromTableForUpdate[resources.Session](ctx, tx, tableSessions, key)
		if err != nil || !found {
			if err == nil {
				err = fmt.Errorf("session %q not found", name)
			}
			return err
		}
		if err := validateSessionFence(session, turnID, workerID, fence); err != nil {
			return err
		}
		until := now.Add(lease).Format(time.RFC3339Nano)
		session.Status.LeaseUntil = until
		session.Status.LastHeartbeat = now.Format(time.RFC3339Nano)
		if err := bumpSessionStatusMetadata(&session, session.Metadata); err != nil {
			return err
		}
		var raw []byte
		if err := tx.QueryRowContext(ctx,
			`SELECT payload FROM session_turns WHERE session_name = $1 AND turn_id = $2 FOR UPDATE`,
			key, turnID,
		).Scan(&raw); err != nil {
			return err
		}
		var turn resources.SessionTurn
		if err := json.Unmarshal(raw, &turn); err != nil {
			return err
		}
		turn.LeaseUntil = until
		if err := updateSessionTurnSQL(ctx, tx, key, turn); err != nil {
			return err
		}
		if err := upsertSessionSQL(ctx, tx, key, session); err != nil {
			return err
		}
		return tx.Commit()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.items[key]
	if !ok {
		return fmt.Errorf("session %q not found", name)
	}
	if err := validateSessionFence(session, turnID, workerID, fence); err != nil {
		return err
	}
	until := now.Add(lease).Format(time.RFC3339Nano)
	session.Status.LeaseUntil = until
	session.Status.LastHeartbeat = now.Format(time.RFC3339Nano)
	if err := bumpSessionStatusMetadata(&session, s.items[key].Metadata); err != nil {
		return err
	}
	for i := range s.turns[key] {
		if s.turns[key][i].ID == turnID {
			s.turns[key][i].LeaseUntil = until
			break
		}
	}
	s.items[key] = session.DeepCopy()
	return nil
}

func (s *SessionStore) CompleteTurn(ctx context.Context, claim SessionClaim, content string, usage map[string]any) ([]resources.SessionEvent, resources.Session, error) {
	return s.finishTurn(ctx, claim, strings.TrimSpace(content), usage, nil)
}

func (s *SessionStore) FailTurn(ctx context.Context, claim SessionClaim, cause error) ([]resources.SessionEvent, resources.Session, error) {
	if cause == nil {
		cause = fmt.Errorf("session turn failed")
	}
	return s.finishTurn(ctx, claim, "", nil, cause)
}

func (s *SessionStore) finishTurn(ctx context.Context, claim SessionClaim, content string, usage map[string]any, failure error) ([]resources.SessionEvent, resources.Session, error) {
	key := scopedNameFromMeta(claim.Session.Metadata)
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, resources.Session{}, err
		}
		defer tx.Rollback()
		session, found, err := getFromTableForUpdate[resources.Session](ctx, tx, tableSessions, key)
		if err != nil || !found {
			if err == nil {
				err = fmt.Errorf("session %q not found", key)
			}
			return nil, resources.Session{}, err
		}
		if err := validateSessionFence(session, claim.Turn.ID, claim.Turn.ClaimedBy, claim.Turn.Fence); err != nil {
			return nil, resources.Session{}, err
		}
		var raw []byte
		if err := tx.QueryRowContext(ctx,
			`SELECT payload FROM session_turns WHERE session_name = $1 AND turn_id = $2 FOR UPDATE`,
			key, claim.Turn.ID,
		).Scan(&raw); err != nil {
			return nil, resources.Session{}, err
		}
		var turn resources.SessionTurn
		if err := json.Unmarshal(raw, &turn); err != nil {
			return nil, resources.Session{}, err
		}
		events := applyTurnFinish(&session, &turn, content, usage, failure, time.Now().UTC())
		if err := bumpSessionStatusMetadata(&session, session.Metadata); err != nil {
			return nil, resources.Session{}, err
		}
		if err := updateSessionTurnSQL(ctx, tx, key, turn); err != nil {
			return nil, resources.Session{}, err
		}
		for _, evt := range events {
			if err := insertSessionEventSQL(ctx, tx, key, evt); err != nil {
				return nil, resources.Session{}, err
			}
		}
		if err := upsertSessionSQL(ctx, tx, key, session); err != nil {
			return nil, resources.Session{}, err
		}
		if err := tx.Commit(); err != nil {
			return nil, resources.Session{}, err
		}
		return events, session, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.items[key]
	if !ok {
		return nil, resources.Session{}, fmt.Errorf("session %q not found", key)
	}
	if err := validateSessionFence(session, claim.Turn.ID, claim.Turn.ClaimedBy, claim.Turn.Fence); err != nil {
		return nil, resources.Session{}, err
	}
	index := -1
	for i := range s.turns[key] {
		if s.turns[key][i].ID == claim.Turn.ID {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, resources.Session{}, fmt.Errorf("session turn %q not found", claim.Turn.ID)
	}
	turn := s.turns[key][index]
	events := applyTurnFinish(&session, &turn, content, usage, failure, time.Now().UTC())
	if err := bumpSessionStatusMetadata(&session, s.items[key].Metadata); err != nil {
		return nil, resources.Session{}, err
	}
	s.items[key] = session.DeepCopy()
	s.turns[key][index] = turn
	s.events[key] = append(s.events[key], events...)
	return copySessionEvents(events), session.DeepCopy(), nil
}

func applyTurnFinish(session *resources.Session, turn *resources.SessionTurn, content string, usage map[string]any, failure error, now time.Time) []resources.SessionEvent {
	events := make([]resources.SessionEvent, 0, 3)
	if failure == nil {
		message := newSessionEventAt(*session, resources.SessionEventMessageCompleted, turn.ID, turn.AssistantMessageID, turn.Attempt, map[string]any{
			"role":    "assistant",
			"content": content,
			"usage":   usage,
		}, now)
		message.Sequence = session.Status.LastEventSequence + 1
		events = append(events, message)
		completed := newSessionEventAt(*session, resources.SessionEventTurnCompleted, turn.ID, turn.AssistantMessageID, turn.Attempt, map[string]any{
			"usage": usage,
		}, now)
		completed.Sequence = message.Sequence + 1
		completed.CausationID = message.ID
		events = append(events, completed)
		turn.Phase = resources.SessionTurnPhaseSucceeded
		turn.CompletedAt = now.Format(time.RFC3339Nano)
		session.Status.CompletedTurns++
		session.Status.Phase = resources.SessionPhaseWaitingInput
		session.Status.LastError = ""
		if session.Spec.MaxTurns > 0 && session.Status.CompletedTurns >= session.Spec.MaxTurns && session.Status.QueuedTurns == 0 {
			session.Status.Phase = resources.SessionPhaseCompleted
			session.Status.CompletedAt = now.Format(time.RFC3339Nano)
			closed := newSessionEventAt(*session, resources.SessionEventSessionCompleted, turn.ID, "", turn.Attempt, map[string]any{
				"reason": "max_turns reached",
			}, now)
			closed.Sequence = completed.Sequence + 1
			events = append(events, closed)
		}
	} else {
		message := strings.TrimSpace(failure.Error())
		failed := newSessionEventAt(*session, resources.SessionEventTurnFailed, turn.ID, turn.AssistantMessageID, turn.Attempt, map[string]any{
			"error": message,
		}, now)
		failed.Sequence = session.Status.LastEventSequence + 1
		events = append(events, failed)
		errEvent := newSessionEventAt(*session, resources.SessionEventError, turn.ID, turn.AssistantMessageID, turn.Attempt, map[string]any{
			"message": message,
		}, now)
		errEvent.Sequence = failed.Sequence + 1
		errEvent.CausationID = failed.ID
		events = append(events, errEvent)
		turn.Phase = resources.SessionTurnPhaseFailed
		turn.LastError = message
		turn.CompletedAt = now.Format(time.RFC3339Nano)
		session.Status.Phase = resources.SessionPhaseFailed
		session.Status.LastError = message
		session.Status.CompletedAt = now.Format(time.RFC3339Nano)
	}
	session.Status.ActiveTurnID = ""
	session.Status.ClaimedBy = ""
	session.Status.LeaseUntil = ""
	session.Status.LastHeartbeat = ""
	session.Status.LastEventSequence = events[len(events)-1].Sequence
	touchSession(session, now)
	return events
}

func (s *SessionStore) ApplyControl(ctx context.Context, name, action, reason string) ([]resources.SessionEvent, resources.Session, error) {
	key := normalizeLookupName(name)
	action = strings.ToLower(strings.TrimSpace(action))
	reason = strings.TrimSpace(reason)
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, resources.Session{}, err
		}
		defer tx.Rollback()
		session, found, err := getFromTableForUpdate[resources.Session](ctx, tx, tableSessions, key)
		if err != nil || !found {
			if err == nil {
				err = fmt.Errorf("session %q not found", name)
			}
			return nil, resources.Session{}, err
		}
		if err := validateSessionControl(session, action); err != nil {
			return nil, resources.Session{}, err
		}
		activeTurn := session.Status.ActiveTurnID
		resetMessageID := ""
		if action == "pause" && activeTurn != "" {
			requeued, changed, err := requeueActiveSessionTurnSQL(ctx, tx, key, activeTurn)
			if err != nil {
				return nil, resources.Session{}, err
			}
			if changed {
				resetMessageID = requeued.AssistantMessageID
				session.Status.QueuedTurns++
			}
		}
		now := time.Now().UTC()
		events, err := applySessionControl(&session, action, reason, resetMessageID, now)
		if err != nil {
			return nil, resources.Session{}, err
		}
		if action == "cancel" || action == "complete" {
			turnReason := "session cancelled"
			if action == "complete" {
				turnReason = "session completed"
			}
			if err := cancelOpenSessionTurnsSQL(ctx, tx, key, now, turnReason, true); err != nil {
				return nil, resources.Session{}, err
			}
		}
		if err := bumpSessionStatusMetadata(&session, session.Metadata); err != nil {
			return nil, resources.Session{}, err
		}
		for _, evt := range events {
			if err := insertSessionEventSQL(ctx, tx, key, evt); err != nil {
				return nil, resources.Session{}, err
			}
		}
		if err := upsertSessionSQL(ctx, tx, key, session); err != nil {
			return nil, resources.Session{}, err
		}
		if err := tx.Commit(); err != nil {
			return nil, resources.Session{}, err
		}
		return events, session, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.items[key]
	if !ok {
		return nil, resources.Session{}, fmt.Errorf("session %q not found", name)
	}
	if err := validateSessionControl(session, action); err != nil {
		return nil, resources.Session{}, err
	}
	activeTurn := session.Status.ActiveTurnID
	resetMessageID := ""
	now := time.Now().UTC()
	turnReason := "session cancelled"
	if action == "complete" {
		turnReason = "session completed"
	}
	for i := range s.turns[key] {
		turn := &s.turns[key][i]
		if action == "pause" && turn.ID == activeTurn && strings.EqualFold(turn.Phase, resources.SessionTurnPhaseRunning) {
			resetMessageID = turn.AssistantMessageID
			turn.Phase = resources.SessionTurnPhaseQueued
			turn.ClaimedBy = ""
			turn.LeaseUntil = ""
			turn.Fence = 0
			session.Status.QueuedTurns++
		}
		if (action == "cancel" || action == "complete") &&
			(strings.EqualFold(turn.Phase, resources.SessionTurnPhaseQueued) || strings.EqualFold(turn.Phase, resources.SessionTurnPhaseRunning)) {
			turn.Phase = resources.SessionTurnPhaseCancelled
			turn.ClaimedBy = ""
			turn.LeaseUntil = ""
			turn.Fence = 0
			turn.CompletedAt = now.Format(time.RFC3339Nano)
			turn.LastError = turnReason
		}
	}
	events, err := applySessionControl(&session, action, reason, resetMessageID, now)
	if err != nil {
		return nil, resources.Session{}, err
	}
	if err := bumpSessionStatusMetadata(&session, s.items[key].Metadata); err != nil {
		return nil, resources.Session{}, err
	}
	s.items[key] = session.DeepCopy()
	s.events[key] = append(s.events[key], events...)
	return copySessionEvents(events), session.DeepCopy(), nil
}

func applySessionControl(session *resources.Session, action, reason, resetMessageID string, now time.Time) ([]resources.SessionEvent, error) {
	if err := validateSessionControl(*session, action); err != nil {
		return nil, err
	}
	var eventType string
	var previousActive string
	switch action {
	case "pause":
		previousActive = session.Status.ActiveTurnID
		session.Status.Phase = resources.SessionPhasePaused
		session.Status.Fence++
		eventType = resources.SessionEventSessionPaused
	case "resume":
		session.Status.Phase = resources.SessionPhaseWaitingInput
		eventType = resources.SessionEventSessionResumed
	case "cancel":
		session.Status.Phase = resources.SessionPhaseCancelled
		session.Status.CompletedAt = now.Format(time.RFC3339Nano)
		session.Status.Fence++
		eventType = resources.SessionEventSessionCancelled
	case "complete":
		session.Status.Phase = resources.SessionPhaseCompleted
		session.Status.CompletedAt = now.Format(time.RFC3339Nano)
		session.Status.Fence++
		eventType = resources.SessionEventSessionCompleted
	}
	events := make([]resources.SessionEvent, 0, 2)
	if action == "pause" && previousActive != "" {
		reset := newSessionEventAt(*session, resources.SessionEventMessageReset, previousActive, resetMessageID, 0, map[string]any{
			"reason": "session paused",
		}, now)
		reset.Sequence = session.Status.LastEventSequence + 1
		events = append(events, reset)
	}
	evt := newSessionEventAt(*session, eventType, previousActive, "", 0, map[string]any{"reason": reason}, now)
	evt.Sequence = session.Status.LastEventSequence + uint64(len(events)) + 1
	events = append(events, evt)
	session.Status.LastEventSequence = evt.Sequence
	session.Status.ActiveTurnID = ""
	session.Status.ClaimedBy = ""
	session.Status.LeaseUntil = ""
	session.Status.LastHeartbeat = ""
	if action == "cancel" || action == "complete" {
		session.Status.QueuedTurns = 0
	}
	touchSession(session, now)
	return events, nil
}

func validateSessionControl(session resources.Session, action string) error {
	if resources.IsTerminalSessionPhase(session.Status.Phase) {
		return fmt.Errorf("session %q is already %s", session.Metadata.Name, session.Status.Phase)
	}
	switch action {
	case "pause":
		if strings.EqualFold(session.Status.Phase, resources.SessionPhasePaused) {
			return fmt.Errorf("session %q is already paused", session.Metadata.Name)
		}
	case "resume":
		if !strings.EqualFold(session.Status.Phase, resources.SessionPhasePaused) {
			return fmt.Errorf("session %q is not paused", session.Metadata.Name)
		}
	case "cancel", "complete":
	default:
		return fmt.Errorf("unsupported session action %q", action)
	}
	return nil
}

func (s *SessionStore) ListEvents(ctx context.Context, name string, after uint64, limit int) ([]resources.SessionEvent, error) {
	key := normalizeLookupName(name)
	if limit <= 0 || limit > defaultSessionEventLimit {
		limit = defaultSessionEventLimit
	}
	if s.db != nil {
		if _, found, err := getFromTable[resources.Session](ctx, s.db, tableSessions, key); err != nil {
			return nil, err
		} else if !found {
			return nil, fmt.Errorf("session %q not found", name)
		}
		rows, err := s.db.QueryContext(ctx,
			`SELECT payload FROM session_events WHERE session_name = $1 AND seq > $2 ORDER BY seq ASC LIMIT $3`,
			key, after, limit,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]resources.SessionEvent, 0)
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return nil, err
			}
			var evt resources.SessionEvent
			if err := json.Unmarshal(raw, &evt); err != nil {
				return nil, err
			}
			out = append(out, evt)
		}
		return out, rows.Err()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.items[key]; !ok {
		return nil, fmt.Errorf("session %q not found", name)
	}
	out := make([]resources.SessionEvent, 0)
	for _, evt := range s.events[key] {
		if evt.Sequence <= after {
			continue
		}
		out = append(out, evt.DeepCopy())
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *SessionStore) ListTurns(ctx context.Context, name string) ([]resources.SessionTurn, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		if _, found, err := getFromTable[resources.Session](ctx, s.db, tableSessions, key); err != nil {
			return nil, err
		} else if !found {
			return nil, fmt.Errorf("session %q not found", name)
		}
		rows, err := s.db.QueryContext(ctx,
			`SELECT payload FROM session_turns WHERE session_name = $1 ORDER BY queue_sequence ASC`,
			key,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]resources.SessionTurn, 0)
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return nil, err
			}
			var turn resources.SessionTurn
			if err := json.Unmarshal(raw, &turn); err != nil {
				return nil, err
			}
			out = append(out, turn)
		}
		return out, rows.Err()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.items[key]; !ok {
		return nil, fmt.Errorf("session %q not found", name)
	}
	return append([]resources.SessionTurn(nil), s.turns[key]...), nil
}

func touchSession(session *resources.Session, now time.Time) {
	session.Status.LastActivityAt = now.Format(time.RFC3339Nano)
	if ttl, err := time.ParseDuration(session.Spec.IdleTTL); err == nil {
		session.Status.ExpiresAt = now.Add(ttl).Format(time.RFC3339Nano)
	}
	session.Status.ObservedGeneration = session.Metadata.Generation
}

func bumpSessionStatusMetadata(session *resources.Session, current resources.ObjectMeta) error {
	session.Metadata.ResourceVersion = current.ResourceVersion
	if err := initializeUpdateMetadata("Session", &session.Metadata, current, false); err != nil {
		return err
	}
	session.Status.ObservedGeneration = session.Metadata.Generation
	return nil
}

func newSessionEvent(session resources.Session, eventType, turnID, messageID string, attempt int, payload map[string]any) resources.SessionEvent {
	return newSessionEventAt(session, eventType, turnID, messageID, attempt, payload, time.Now().UTC())
}

func newSessionEventAt(session resources.Session, eventType, turnID, messageID string, attempt int, payload map[string]any, now time.Time) resources.SessionEvent {
	return resources.SessionEvent{
		ID:          newUUID(),
		SessionName: session.Metadata.Name,
		Namespace:   resources.NormalizeNamespace(session.Metadata.Namespace),
		TurnID:      turnID,
		MessageID:   messageID,
		Attempt:     attempt,
		Type:        eventType,
		Timestamp:   now.Format(time.RFC3339Nano),
		Payload:     payload,
	}
}

func newUUID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return id.String()
}

func copySessionEvents(events []resources.SessionEvent) []resources.SessionEvent {
	out := make([]resources.SessionEvent, len(events))
	for i, evt := range events {
		out[i] = evt.DeepCopy()
	}
	return out
}

func upsertSessionSQL(ctx context.Context, db dbExecer, name string, item resources.Session) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO sessions(name, namespace, system_ref, status_phase, claimed_by, lease_until, expires_at, spec_hash, payload, updated_at)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, NOW())
		ON CONFLICT(name) DO UPDATE SET
			namespace = EXCLUDED.namespace,
			system_ref = EXCLUDED.system_ref,
			status_phase = EXCLUDED.status_phase,
			claimed_by = EXCLUDED.claimed_by,
			lease_until = EXCLUDED.lease_until,
			expires_at = EXCLUDED.expires_at,
			spec_hash = EXCLUDED.spec_hash,
			payload = EXCLUDED.payload,
			updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.TrimSpace(item.Spec.System),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		strings.TrimSpace(item.Status.ClaimedBy),
		parseTimestampPtr(item.Status.LeaseUntil),
		parseTimestampPtr(item.Status.ExpiresAt),
		specHash(item.Spec),
		string(payload),
	)
	return err
}

func insertSessionTurnSQL(ctx context.Context, db dbExecer, sessionName string, turn resources.SessionTurn) error {
	payload, err := json.Marshal(turn)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO session_turns(session_name, turn_id, queue_sequence, idempotency_key, status_phase, claimed_by, lease_until, payload, created_at, updated_at)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, NOW())`,
		sessionName,
		turn.ID,
		turn.QueueSequence,
		turn.IdempotencyKey,
		strings.ToLower(turn.Phase),
		turn.ClaimedBy,
		parseTimestampPtr(turn.LeaseUntil),
		string(payload),
		parseTimestampPtr(turn.CreatedAt),
	)
	return err
}

func updateSessionTurnSQL(ctx context.Context, db dbExecer, sessionName string, turn resources.SessionTurn) error {
	payload, err := json.Marshal(turn)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		UPDATE session_turns SET
			status_phase = $3,
			claimed_by = $4,
			lease_until = $5,
			payload = $6::jsonb,
			updated_at = NOW()
		WHERE session_name = $1 AND turn_id = $2`,
		sessionName,
		turn.ID,
		strings.ToLower(turn.Phase),
		turn.ClaimedBy,
		parseTimestampPtr(turn.LeaseUntil),
		string(payload),
	)
	return err
}

func insertSessionEventSQL(ctx context.Context, db dbExecer, sessionName string, evt resources.SessionEvent) error {
	payload, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO session_events(session_name, seq, event_id, event_type, turn_id, message_id, payload, created_at)
		VALUES($1, $2, $3, $4, $5, $6, $7::jsonb, $8)`,
		sessionName,
		evt.Sequence,
		evt.ID,
		evt.Type,
		evt.TurnID,
		evt.MessageID,
		string(payload),
		parseTimestampPtr(evt.Timestamp),
	)
	return err
}

func requeueActiveSessionTurnSQL(ctx context.Context, tx *sql.Tx, sessionName, turnID string) (resources.SessionTurn, bool, error) {
	var raw []byte
	if err := tx.QueryRowContext(ctx,
		`SELECT payload FROM session_turns WHERE session_name = $1 AND turn_id = $2 FOR UPDATE`,
		sessionName, turnID,
	).Scan(&raw); err != nil {
		return resources.SessionTurn{}, false, err
	}
	var turn resources.SessionTurn
	if err := json.Unmarshal(raw, &turn); err != nil {
		return resources.SessionTurn{}, false, err
	}
	if !strings.EqualFold(turn.Phase, resources.SessionTurnPhaseRunning) {
		return turn, false, nil
	}
	turn.Phase = resources.SessionTurnPhaseQueued
	turn.ClaimedBy = ""
	turn.LeaseUntil = ""
	turn.Fence = 0
	if err := updateSessionTurnSQL(ctx, tx, sessionName, turn); err != nil {
		return resources.SessionTurn{}, false, err
	}
	return turn, true, nil
}

func cancelOpenSessionTurnsSQL(ctx context.Context, tx *sql.Tx, sessionName string, now time.Time, reason string, includeRunning bool) error {
	phases := `status_phase = 'queued'`
	if includeRunning {
		phases = `status_phase IN ('queued', 'running')`
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT payload FROM session_turns WHERE session_name = $1 AND `+phases+` FOR UPDATE`,
		sessionName,
	)
	if err != nil {
		return err
	}
	var turns []resources.SessionTurn
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var turn resources.SessionTurn
		if err := json.Unmarshal(raw, &turn); err != nil {
			rows.Close()
			return err
		}
		turns = append(turns, turn)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, turn := range turns {
		turn.Phase = resources.SessionTurnPhaseCancelled
		turn.ClaimedBy = ""
		turn.LeaseUntil = ""
		turn.Fence = 0
		turn.CompletedAt = now.Format(time.RFC3339Nano)
		turn.LastError = reason
		if err := updateSessionTurnSQL(ctx, tx, sessionName, turn); err != nil {
			return err
		}
	}
	return nil
}
