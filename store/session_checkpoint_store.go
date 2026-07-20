package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

func (s *SessionStore) CreateCheckpoint(
	ctx context.Context,
	name, turnID, workerID string,
	fence int64,
	checkpoint resources.SessionCheckpoint,
) (resources.SessionCheckpoint, resources.SessionEvent, error) {
	key := normalizeLookupName(name)
	if len(checkpoint.State) == 0 || !json.Valid(checkpoint.State) {
		return resources.SessionCheckpoint{}, resources.SessionEvent{}, fmt.Errorf("checkpoint state must be valid JSON")
	}
	if checkpoint.StateVersion == 0 {
		checkpoint.StateVersion = resources.SessionCheckpointStateVersion
	}
	if checkpoint.StateVersion != resources.SessionCheckpointStateVersion {
		return resources.SessionCheckpoint{}, resources.SessionEvent{}, fmt.Errorf(
			"unsupported checkpoint state version %d",
			checkpoint.StateVersion,
		)
	}
	checkpoint.SafePoint = strings.TrimSpace(checkpoint.SafePoint)
	if checkpoint.SafePoint == "" {
		return resources.SessionCheckpoint{}, resources.SessionEvent{}, fmt.Errorf("checkpoint safe point is required")
	}
	checkpoint.StateHash = checkpointStateHash(checkpoint.State)

	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return resources.SessionCheckpoint{}, resources.SessionEvent{}, err
		}
		defer tx.Rollback()
		session, found, err := getFromTableForUpdate[resources.Session](ctx, tx, tableSessions, key)
		if err != nil || !found {
			if err == nil {
				err = fmt.Errorf("session %q not found", name)
			}
			return resources.SessionCheckpoint{}, resources.SessionEvent{}, err
		}
		if err := validateSessionFence(session, turnID, workerID, fence); err != nil {
			return resources.SessionCheckpoint{}, resources.SessionEvent{}, err
		}
		checkpoint, event, err := prepareSessionCheckpoint(session, checkpoint, turnID, time.Now().UTC())
		if err != nil {
			return resources.SessionCheckpoint{}, resources.SessionEvent{}, err
		}
		session.Status.LastCheckpointID = checkpoint.ID
		session.Status.LastEventSequence = event.Sequence
		touchSession(&session, time.Now().UTC())
		if err := bumpSessionStatusMetadata(&session, session.Metadata); err != nil {
			return resources.SessionCheckpoint{}, resources.SessionEvent{}, err
		}
		if err := insertSessionCheckpointSQL(ctx, tx, key, checkpoint); err != nil {
			return resources.SessionCheckpoint{}, resources.SessionEvent{}, err
		}
		if err := insertSessionEventSQL(ctx, tx, key, event); err != nil {
			return resources.SessionCheckpoint{}, resources.SessionEvent{}, err
		}
		if err := upsertSessionSQL(ctx, tx, key, session); err != nil {
			return resources.SessionCheckpoint{}, resources.SessionEvent{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.SessionCheckpoint{}, resources.SessionEvent{}, err
		}
		return checkpoint, event, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.items[key]
	if !ok {
		return resources.SessionCheckpoint{}, resources.SessionEvent{}, fmt.Errorf("session %q not found", name)
	}
	if err := validateSessionFence(session, turnID, workerID, fence); err != nil {
		return resources.SessionCheckpoint{}, resources.SessionEvent{}, err
	}
	checkpoint, event, err := prepareSessionCheckpoint(session, checkpoint, turnID, time.Now().UTC())
	if err != nil {
		return resources.SessionCheckpoint{}, resources.SessionEvent{}, err
	}
	session.Status.LastCheckpointID = checkpoint.ID
	session.Status.LastEventSequence = event.Sequence
	touchSession(&session, time.Now().UTC())
	if err := bumpSessionStatusMetadata(&session, s.items[key].Metadata); err != nil {
		return resources.SessionCheckpoint{}, resources.SessionEvent{}, err
	}
	s.items[key] = session.DeepCopy()
	s.checkpoints[key] = append(s.checkpoints[key], checkpoint.DeepCopy())
	s.events[key] = append(s.events[key], event.DeepCopy())
	return checkpoint.DeepCopy(), event.DeepCopy(), nil
}

func prepareSessionCheckpoint(
	session resources.Session,
	checkpoint resources.SessionCheckpoint,
	turnID string,
	now time.Time,
) (resources.SessionCheckpoint, resources.SessionEvent, error) {
	if strings.TrimSpace(checkpoint.ID) == "" {
		checkpoint.ID = newUUID()
	}
	checkpoint.SessionName = session.Metadata.Name
	checkpoint.Namespace = resources.NormalizeNamespace(session.Metadata.Namespace)
	if strings.TrimSpace(checkpoint.TurnID) == "" {
		checkpoint.TurnID = strings.TrimSpace(turnID)
	}
	if strings.TrimSpace(checkpoint.ParentCheckpointID) == "" {
		checkpoint.ParentCheckpointID = session.Status.LastCheckpointID
	}
	checkpoint.Fence = session.Status.Fence
	if checkpoint.SystemGeneration == 0 {
		checkpoint.SystemGeneration = session.Status.SystemGeneration
	}
	checkpoint.CreatedAt = now.Format(time.RFC3339Nano)
	maxAgeValue := strings.TrimSpace(session.Spec.CheckpointRetention.MaxAge)
	if maxAgeValue == "" {
		maxAgeValue = "168h"
	}
	maxAge, err := time.ParseDuration(maxAgeValue)
	if err != nil || maxAge <= 0 {
		return resources.SessionCheckpoint{}, resources.SessionEvent{}, fmt.Errorf(
			"invalid checkpoint retention max age %q",
			maxAgeValue,
		)
	}
	checkpoint.ExpiresAt = now.Add(maxAge).Format(time.RFC3339Nano)
	event := newSessionEventAt(
		session,
		resources.SessionEventCheckpointCreated,
		checkpoint.TurnID,
		"",
		checkpoint.Attempt,
		map[string]any{
			"checkpoint_id": checkpoint.ID,
			"safe_point":    checkpoint.SafePoint,
			"state_version": checkpoint.StateVersion,
			"state_hash":    checkpoint.StateHash,
			"agent":         checkpoint.Agent,
			"agent_index":   checkpoint.AgentIndex,
			"task":          checkpoint.TaskName,
		},
		now,
	)
	event.Sequence = session.Status.LastEventSequence + 1
	checkpoint.EventSequence = event.Sequence
	return checkpoint, event, nil
}

func (s *SessionStore) ListCheckpoints(ctx context.Context, name string) ([]resources.SessionCheckpoint, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		if _, found, err := getFromTable[resources.Session](ctx, s.db, tableSessions, key); err != nil || !found {
			if err == nil {
				err = fmt.Errorf("session %q not found", name)
			}
			return nil, err
		}
		rows, err := s.db.QueryContext(ctx, `
			SELECT payload
			FROM session_checkpoints
			WHERE session_name = $1
			ORDER BY event_seq DESC, created_at DESC`, key)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]resources.SessionCheckpoint, 0)
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return nil, err
			}
			var checkpoint resources.SessionCheckpoint
			if err := json.Unmarshal(raw, &checkpoint); err != nil {
				return nil, err
			}
			out = append(out, checkpoint)
		}
		return out, rows.Err()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.items[key]; !ok {
		return nil, fmt.Errorf("session %q not found", name)
	}
	out := copySessionCheckpoints(s.checkpoints[key])
	sort.Slice(out, func(i, j int) bool {
		if out[i].EventSequence == out[j].EventSequence {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return out[i].EventSequence > out[j].EventSequence
	})
	return out, nil
}

func (s *SessionStore) GetCheckpoint(ctx context.Context, name, checkpointID string) (resources.SessionCheckpoint, bool, error) {
	checkpointID = strings.TrimSpace(checkpointID)
	if checkpointID == "" {
		return resources.SessionCheckpoint{}, false, fmt.Errorf("checkpoint ID is required")
	}
	checkpoints, err := s.ListCheckpoints(ctx, name)
	if err != nil {
		return resources.SessionCheckpoint{}, false, err
	}
	for _, checkpoint := range checkpoints {
		if checkpoint.ID == checkpointID {
			return checkpoint.DeepCopy(), true, nil
		}
	}
	return resources.SessionCheckpoint{}, false, nil
}

func (s *SessionStore) LatestCheckpoint(
	ctx context.Context,
	name, turnID, agent string,
) (resources.SessionCheckpoint, bool, error) {
	return s.LatestExecutionCheckpoint(ctx, name, turnID, agent, -1, "", "")
}

func (s *SessionStore) LatestExecutionCheckpoint(
	ctx context.Context,
	name, turnID, agent string,
	agentIndex int,
	messageID, branchID string,
) (resources.SessionCheckpoint, bool, error) {
	session, found, err := s.Get(ctx, name)
	if err != nil || !found {
		if err == nil {
			err = fmt.Errorf("session %q not found", name)
		}
		return resources.SessionCheckpoint{}, false, err
	}
	checkpoints, err := s.ListCheckpoints(ctx, name)
	if err != nil {
		return resources.SessionCheckpoint{}, false, err
	}
	turnID = strings.TrimSpace(turnID)
	agent = strings.TrimSpace(agent)
	messageID = strings.TrimSpace(messageID)
	branchID = strings.TrimSpace(branchID)
	checkpoint, found, err := selectExecutionCheckpointFromLineage(
		session,
		checkpoints,
		turnID,
		agent,
		agentIndex,
		messageID,
		branchID,
	)
	if err != nil || !found {
		return resources.SessionCheckpoint{}, found, err
	}
	if checkpointStateHash(checkpoint.State) != checkpoint.StateHash {
		return resources.SessionCheckpoint{}, false, fmt.Errorf(
			"checkpoint %q state hash mismatch",
			checkpoint.ID,
		)
	}
	return checkpoint.DeepCopy(), true, nil
}

func selectExecutionCheckpointFromLineage(
	session resources.Session,
	checkpoints []resources.SessionCheckpoint,
	turnID, agent string,
	agentIndex int,
	messageID, branchID string,
) (resources.SessionCheckpoint, bool, error) {
	byID := make(map[string]resources.SessionCheckpoint, len(checkpoints))
	for _, checkpoint := range checkpoints {
		byID[checkpoint.ID] = checkpoint
	}
	lineage := make([]resources.SessionCheckpoint, 0, len(checkpoints))
	headID := strings.TrimSpace(session.Status.LastCheckpointID)
	seen := make(map[string]struct{}, len(checkpoints))
	for headID != "" {
		if _, duplicate := seen[headID]; duplicate {
			return resources.SessionCheckpoint{}, false, fmt.Errorf("checkpoint lineage cycle at %q", headID)
		}
		seen[headID] = struct{}{}
		checkpoint, ok := byID[headID]
		if !ok {
			return resources.SessionCheckpoint{}, false, fmt.Errorf(
				"checkpoint lineage head %q is missing",
				headID,
			)
		}
		lineage = append(lineage, checkpoint)
		headID = strings.TrimSpace(checkpoint.ParentCheckpointID)
		if headID != "" {
			if _, internal := byID[headID]; !internal {
				break
			}
		}
	}
	if len(lineage) == 0 {
		lineage = checkpoints
	}
	for _, checkpoint := range lineage {
		if turnID != "" && checkpoint.TurnID != turnID {
			continue
		}
		if agent != "" && !strings.EqualFold(checkpoint.Agent, agent) {
			continue
		}
		if agentIndex >= 0 && checkpoint.AgentIndex != agentIndex {
			continue
		}
		if messageID != "" && checkpoint.MessageID != messageID {
			continue
		}
		if branchID != "" && checkpoint.BranchID != branchID {
			continue
		}
		return checkpoint, true, nil
	}
	return resources.SessionCheckpoint{}, false, nil
}

func (s *SessionStore) ReplayCheckpoint(
	ctx context.Context,
	name, checkpointID string,
) (resources.SessionReplayResult, error) {
	checkpoints, err := s.ListCheckpoints(ctx, name)
	if err != nil {
		return resources.SessionReplayResult{}, err
	}
	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].EventSequence < checkpoints[j].EventSequence
	})
	checkpointsByID := make(map[string]resources.SessionCheckpoint, len(checkpoints))
	for _, checkpoint := range checkpoints {
		checkpointsByID[checkpoint.ID] = checkpoint
	}
	result := resources.SessionReplayResult{
		SessionName: resources.NormalizeNamespace(resources.DefaultNamespace) + "/" + strings.TrimSpace(name),
		Verified:    true,
	}
	for i := range checkpoints {
		checkpoint := checkpoints[i]
		if checkpointStateHash(checkpoint.State) != checkpoint.StateHash {
			result.Verified = false
			return result, fmt.Errorf("checkpoint %q state hash mismatch", checkpoint.ID)
		}
		if parentID := strings.TrimSpace(checkpoint.ParentCheckpointID); parentID != "" {
			if parent, ok := checkpointsByID[parentID]; ok && parent.EventSequence >= checkpoint.EventSequence {
				result.Verified = false
				return result, fmt.Errorf("checkpoint %q has invalid lineage parent %q", checkpoint.ID, parentID)
			}
		}
		result.CheckpointCount++
		if checkpoint.ID != checkpointID {
			continue
		}
		result.SessionName = checkpoint.SessionName
		result.CheckpointID = checkpoint.ID
		result.StateVersion = checkpoint.StateVersion
		result.StateHash = checkpoint.StateHash
		final := checkpoint.DeepCopy()
		result.FinalCheckpoint = &final
		events, listErr := s.listEventsThrough(ctx, name, checkpoint.EventSequence)
		if listErr != nil {
			return resources.SessionReplayResult{}, listErr
		}
		var previousSequence uint64
		var foundCheckpointEvent bool
		for _, event := range events {
			if event.Sequence <= checkpoint.EventSequence {
				result.Events = append(result.Events, event)
			}
			if previousSequence > 0 && event.Sequence != previousSequence+1 {
				result.Verified = false
				return result, fmt.Errorf("event replay sequence gap after %d", previousSequence)
			}
			previousSequence = event.Sequence
			eventCheckpointID, _ := event.Payload["checkpoint_id"].(string)
			eventStateHash, _ := event.Payload["state_hash"].(string)
			if event.Type == resources.SessionEventCheckpointCreated &&
				eventCheckpointID == checkpoint.ID &&
				eventStateHash == checkpoint.StateHash {
				foundCheckpointEvent = true
			}
		}
		if !foundCheckpointEvent {
			result.Verified = false
			return result, fmt.Errorf("checkpoint %q is missing its durable event", checkpoint.ID)
		}
		return result, nil
	}
	return resources.SessionReplayResult{}, fmt.Errorf("checkpoint %q not found", checkpointID)
}

func (s *SessionStore) RewindSession(
	ctx context.Context,
	name, checkpointID string,
	interrupt bool,
) (resources.Session, resources.SessionEvent, error) {
	key := normalizeLookupName(name)
	checkpoint, found, err := s.GetCheckpoint(ctx, key, checkpointID)
	if err != nil || !found {
		if err == nil {
			err = fmt.Errorf("checkpoint %q not found", checkpointID)
		}
		return resources.Session{}, resources.SessionEvent{}, err
	}
	now := time.Now().UTC()
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return resources.Session{}, resources.SessionEvent{}, err
		}
		defer tx.Rollback()
		session, found, err := getFromTableForUpdate[resources.Session](ctx, tx, tableSessions, key)
		if err != nil || !found {
			if err == nil {
				err = fmt.Errorf("session %q not found", name)
			}
			return resources.Session{}, resources.SessionEvent{}, err
		}
		var checkpointRaw []byte
		if err := tx.QueryRowContext(ctx, `
			SELECT payload
			FROM session_checkpoints
			WHERE session_name = $1 AND checkpoint_id = $2
			FOR UPDATE`,
			key, checkpointID,
		).Scan(&checkpointRaw); err != nil {
			if err == sql.ErrNoRows {
				return resources.Session{}, resources.SessionEvent{}, fmt.Errorf(
					"checkpoint %q not found",
					checkpointID,
				)
			}
			return resources.Session{}, resources.SessionEvent{}, err
		}
		if err := json.Unmarshal(checkpointRaw, &checkpoint); err != nil {
			return resources.Session{}, resources.SessionEvent{}, err
		}
		if session.Status.ActiveTurnID != "" && !interrupt {
			return resources.Session{}, resources.SessionEvent{}, fmt.Errorf(
				"session %q has an active turn; set interrupt=true to rewind",
				session.Metadata.Name,
			)
		}
		if err := rewindSessionTurnSQL(ctx, tx, key, checkpoint.TurnID, &session, now); err != nil {
			return resources.Session{}, resources.SessionEvent{}, err
		}
		event := applySessionRewind(&session, checkpoint, now)
		if err := bumpSessionStatusMetadata(&session, session.Metadata); err != nil {
			return resources.Session{}, resources.SessionEvent{}, err
		}
		if err := insertSessionEventSQL(ctx, tx, key, event); err != nil {
			return resources.Session{}, resources.SessionEvent{}, err
		}
		if err := upsertSessionSQL(ctx, tx, key, session); err != nil {
			return resources.Session{}, resources.SessionEvent{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.Session{}, resources.SessionEvent{}, err
		}
		return session, event, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.items[key]
	if !ok {
		return resources.Session{}, resources.SessionEvent{}, fmt.Errorf("session %q not found", name)
	}
	found = false
	for _, candidate := range s.checkpoints[key] {
		if candidate.ID == checkpointID {
			checkpoint = candidate.DeepCopy()
			found = true
			break
		}
	}
	if !found {
		return resources.Session{}, resources.SessionEvent{}, fmt.Errorf("checkpoint %q not found", checkpointID)
	}
	if session.Status.ActiveTurnID != "" && !interrupt {
		return resources.Session{}, resources.SessionEvent{}, fmt.Errorf(
			"session %q has an active turn; set interrupt=true to rewind",
			session.Metadata.Name,
		)
	}
	rewindSessionTurnMemory(s.turns[key], checkpoint.TurnID, &session, now)
	event := applySessionRewind(&session, checkpoint, now)
	if err := bumpSessionStatusMetadata(&session, s.items[key].Metadata); err != nil {
		return resources.Session{}, resources.SessionEvent{}, err
	}
	s.items[key] = session.DeepCopy()
	s.events[key] = append(s.events[key], event.DeepCopy())
	return session.DeepCopy(), event.DeepCopy(), nil
}

func applySessionRewind(
	session *resources.Session,
	checkpoint resources.SessionCheckpoint,
	now time.Time,
) resources.SessionEvent {
	previousActive := session.Status.ActiveTurnID
	session.Status.Fence++
	session.Status.Phase = resources.SessionPhasePaused
	session.Status.ActiveTurnID = ""
	session.Status.ClaimedBy = ""
	session.Status.LeaseUntil = ""
	session.Status.LastHeartbeat = ""
	session.Status.BlockedOn = nil
	session.Status.LastError = ""
	session.Status.CompletedAt = ""
	session.Status.LastCheckpointID = checkpoint.ID
	session.Status.RestoredCheckpoint = checkpoint.ID
	event := newSessionEventAt(
		*session,
		resources.SessionEventSessionRewound,
		checkpoint.TurnID,
		"",
		checkpoint.Attempt,
		map[string]any{
			"checkpoint_id":        checkpoint.ID,
			"checkpoint_sequence":  checkpoint.EventSequence,
			"previous_active_turn": previousActive,
		},
		now,
	)
	event.Sequence = session.Status.LastEventSequence + 1
	session.Status.LastEventSequence = event.Sequence
	return event
}

func rewindSessionTurnMemory(
	turns []resources.SessionTurn,
	turnID string,
	session *resources.Session,
	now time.Time,
) {
	var targetSequence uint64
	for i := range turns {
		if turns[i].ID == turnID {
			targetSequence = turns[i].QueueSequence
			break
		}
	}
	for i := range turns {
		switch {
		case turns[i].ID == turnID:
			turns[i].Phase = resources.SessionTurnPhaseQueued
			turns[i].ClaimedBy = ""
			turns[i].LeaseUntil = ""
			turns[i].CompletedAt = ""
			turns[i].LastError = ""
			turns[i].Fence = session.Status.Fence + 1
		case targetSequence > 0 && turns[i].QueueSequence > targetSequence:
			if !strings.EqualFold(turns[i].Phase, resources.SessionTurnPhaseCancelled) {
				turns[i].Phase = resources.SessionTurnPhaseCancelled
				turns[i].ClaimedBy = ""
				turns[i].LeaseUntil = ""
				turns[i].CompletedAt = now.Format(time.RFC3339Nano)
				turns[i].LastError = "superseded by Session rewind"
				turns[i].Fence = session.Status.Fence + 1
			}
		}
	}
	session.Status.QueuedTurns = 0
	session.Status.CompletedTurns = 0
	for _, turn := range turns {
		if strings.EqualFold(turn.Phase, resources.SessionTurnPhaseQueued) {
			session.Status.QueuedTurns++
		}
		if strings.EqualFold(turn.Phase, resources.SessionTurnPhaseSucceeded) {
			session.Status.CompletedTurns++
		}
	}
}

func rewindSessionTurnSQL(
	ctx context.Context,
	tx *sql.Tx,
	key, turnID string,
	session *resources.Session,
	now time.Time,
) error {
	if strings.TrimSpace(turnID) == "" {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT payload
		FROM session_turns
		WHERE session_name = $1
		ORDER BY queue_sequence
		FOR UPDATE`, key)
	if err != nil {
		return err
	}
	defer rows.Close()
	turns := make([]resources.SessionTurn, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var turn resources.SessionTurn
		if err := json.Unmarshal(raw, &turn); err != nil {
			return err
		}
		turns = append(turns, turn)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rewindSessionTurnMemory(turns, turnID, session, now)
	for _, turn := range turns {
		if err := updateSessionTurnSQL(ctx, tx, key, turn); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionStore) ForkSession(
	ctx context.Context,
	sourceName, checkpointID, targetName string,
) (resources.Session, resources.SessionCheckpoint, []resources.SessionEvent, error) {
	sourceKey := normalizeLookupName(sourceName)
	source, found, err := s.Get(ctx, sourceKey)
	if err != nil || !found {
		if err == nil {
			err = fmt.Errorf("session %q not found", sourceName)
		}
		return resources.Session{}, resources.SessionCheckpoint{}, nil, err
	}
	checkpoint, found, err := s.GetCheckpoint(ctx, sourceKey, checkpointID)
	if err != nil || !found {
		if err == nil {
			err = fmt.Errorf("checkpoint %q not found", checkpointID)
		}
		return resources.Session{}, resources.SessionCheckpoint{}, nil, err
	}
	target := resources.Session{
		APIVersion: source.APIVersion,
		Kind:       source.Kind,
		Metadata: resources.ObjectMeta{
			Name:      strings.TrimSpace(targetName),
			Namespace: source.Metadata.Namespace,
		},
		Spec: source.Spec,
	}
	if err := target.Normalize(); err != nil {
		return resources.Session{}, resources.SessionCheckpoint{}, nil, err
	}
	sourceEvents, err := s.listEventsThrough(ctx, sourceKey, checkpoint.EventSequence)
	if err != nil {
		return resources.Session{}, resources.SessionCheckpoint{}, nil, err
	}
	return s.createForkAtomically(
		ctx,
		target,
		source,
		checkpoint,
		sourceEvents,
	)
}

func (s *SessionStore) listEventsThrough(
	ctx context.Context,
	name string,
	maxSequence uint64,
) ([]resources.SessionEvent, error) {
	const pageSize = 1000
	after := uint64(0)
	out := make([]resources.SessionEvent, 0)
	for {
		page, err := s.ListEvents(ctx, name, after, pageSize)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			return out, nil
		}
		for _, event := range page {
			if maxSequence > 0 && event.Sequence > maxSequence {
				return out, nil
			}
			out = append(out, event)
			after = event.Sequence
			if maxSequence > 0 && event.Sequence >= maxSequence {
				return out, nil
			}
		}
		if len(page) < pageSize {
			return out, nil
		}
	}
}

func (s *SessionStore) createForkAtomically(
	ctx context.Context,
	target resources.Session,
	source resources.Session,
	sourceCheckpoint resources.SessionCheckpoint,
	sourceEvents []resources.SessionEvent,
) (resources.Session, resources.SessionCheckpoint, []resources.SessionEvent, error) {
	targetKey := scopedNameFromMeta(target.Metadata)
	now := time.Now().UTC()
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return resources.Session{}, resources.SessionCheckpoint{}, nil, err
		}
		defer tx.Rollback()
		if _, found, err := getFromTableForUpdate[resources.Session](ctx, tx, tableSessions, targetKey); err != nil {
			return resources.Session{}, resources.SessionCheckpoint{}, nil, err
		} else if found {
			return resources.Session{}, resources.SessionCheckpoint{}, nil, fmt.Errorf(
				"session %q already exists",
				target.Metadata.Name,
			)
		}
		if err := initializeCreateMetadata("Session", &target.Metadata); err != nil {
			return resources.Session{}, resources.SessionCheckpoint{}, nil, err
		}
		initializeSessionStatus(&target, now)
		target.Status.LastEventSequence = 1
		target.Status.ObservedGeneration = target.Metadata.Generation
		if err := upsertSessionSQL(ctx, tx, targetKey, target); err != nil {
			return resources.Session{}, resources.SessionCheckpoint{}, nil, err
		}
		created := newSessionEvent(target, resources.SessionEventSessionCreated, "", "", 0, map[string]any{
			"system": target.Spec.System,
		})
		created.Sequence = 1
		if err := insertSessionEventSQL(ctx, tx, targetKey, created); err != nil {
			return resources.Session{}, resources.SessionCheckpoint{}, nil, err
		}
		imported := materializeForkEvents(&target, sourceCheckpoint, sourceEvents, now)
		for _, event := range imported {
			if err := insertSessionEventSQL(ctx, tx, targetKey, event); err != nil {
				return resources.Session{}, resources.SessionCheckpoint{}, nil, err
			}
		}
		forkedCheckpoint, checkpointEvent, err := materializeForkCheckpoint(
			&target,
			source,
			sourceCheckpoint,
			now,
		)
		if err != nil {
			return resources.Session{}, resources.SessionCheckpoint{}, nil, err
		}
		if err := insertSessionCheckpointSQL(ctx, tx, targetKey, forkedCheckpoint); err != nil {
			return resources.Session{}, resources.SessionCheckpoint{}, nil, err
		}
		if err := insertSessionEventSQL(ctx, tx, targetKey, checkpointEvent); err != nil {
			return resources.Session{}, resources.SessionCheckpoint{}, nil, err
		}
		forkEvent := materializeSessionForkedEvent(&target, source, sourceCheckpoint, forkedCheckpoint, now)
		if err := insertSessionEventSQL(ctx, tx, targetKey, forkEvent); err != nil {
			return resources.Session{}, resources.SessionCheckpoint{}, nil, err
		}
		if err := bumpSessionStatusMetadata(&target, target.Metadata); err != nil {
			return resources.Session{}, resources.SessionCheckpoint{}, nil, err
		}
		if err := upsertSessionSQL(ctx, tx, targetKey, target); err != nil {
			return resources.Session{}, resources.SessionCheckpoint{}, nil, err
		}
		if err := tx.Commit(); err != nil {
			return resources.Session{}, resources.SessionCheckpoint{}, nil, err
		}
		events := append([]resources.SessionEvent{created}, imported...)
		events = append(events, checkpointEvent, forkEvent)
		return target, forkedCheckpoint, events, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[targetKey]; exists {
		return resources.Session{}, resources.SessionCheckpoint{}, nil, fmt.Errorf(
			"session %q already exists",
			target.Metadata.Name,
		)
	}
	if err := initializeCreateMetadata("Session", &target.Metadata); err != nil {
		return resources.Session{}, resources.SessionCheckpoint{}, nil, err
	}
	initializeSessionStatus(&target, now)
	target.Status.LastEventSequence = 1
	target.Status.ObservedGeneration = target.Metadata.Generation
	created := newSessionEvent(target, resources.SessionEventSessionCreated, "", "", 0, map[string]any{
		"system": target.Spec.System,
	})
	created.Sequence = 1
	imported := materializeForkEvents(&target, sourceCheckpoint, sourceEvents, now)
	forkedCheckpoint, checkpointEvent, err := materializeForkCheckpoint(
		&target,
		source,
		sourceCheckpoint,
		now,
	)
	if err != nil {
		return resources.Session{}, resources.SessionCheckpoint{}, nil, err
	}
	forkEvent := materializeSessionForkedEvent(&target, source, sourceCheckpoint, forkedCheckpoint, now)
	if err := bumpSessionStatusMetadata(&target, target.Metadata); err != nil {
		return resources.Session{}, resources.SessionCheckpoint{}, nil, err
	}
	s.items[targetKey] = target.DeepCopy()
	s.events[targetKey] = append([]resources.SessionEvent{created}, copySessionEvents(imported)...)
	s.events[targetKey] = append(s.events[targetKey], checkpointEvent.DeepCopy(), forkEvent.DeepCopy())
	s.checkpoints[targetKey] = []resources.SessionCheckpoint{forkedCheckpoint.DeepCopy()}
	events := copySessionEvents(s.events[targetKey])
	return target.DeepCopy(), forkedCheckpoint.DeepCopy(), events, nil
}

func materializeForkEvents(
	target *resources.Session,
	sourceCheckpoint resources.SessionCheckpoint,
	sourceEvents []resources.SessionEvent,
	now time.Time,
) []resources.SessionEvent {
	out := make([]resources.SessionEvent, 0)
	for _, sourceEvent := range sourceEvents {
		if sourceEvent.Sequence > sourceCheckpoint.EventSequence {
			continue
		}
		if sourceEvent.Type != resources.SessionEventMessageCreated &&
			sourceEvent.Type != resources.SessionEventMessageCompleted {
			continue
		}
		event := sourceEvent.DeepCopy()
		event.ID = newUUID()
		event.SessionName = target.Metadata.Name
		event.Namespace = resources.NormalizeNamespace(target.Metadata.Namespace)
		event.CausationID = sourceEvent.ID
		event.Sequence = target.Status.LastEventSequence + 1
		event.Timestamp = now.Format(time.RFC3339Nano)
		target.Status.LastEventSequence = event.Sequence
		out = append(out, event)
	}
	return out
}

func materializeForkCheckpoint(
	target *resources.Session,
	source resources.Session,
	sourceCheckpoint resources.SessionCheckpoint,
	now time.Time,
) (resources.SessionCheckpoint, resources.SessionEvent, error) {
	forked := sourceCheckpoint.DeepCopy()
	forked.ID = ""
	forked.SessionName = target.Metadata.Name
	forked.Namespace = resources.NormalizeNamespace(target.Metadata.Namespace)
	forked.TurnID = ""
	forked.TaskName = ""
	forked.Attempt = 0
	forked.Fence = target.Status.Fence
	forked.EventSequence = 0
	forked.ParentCheckpointID = source.Metadata.Name + "/" + sourceCheckpoint.ID
	forked.CreatedAt = ""
	forked.ExpiresAt = ""
	forked, event, err := prepareSessionCheckpoint(*target, forked, "", now)
	if err != nil {
		return resources.SessionCheckpoint{}, resources.SessionEvent{}, err
	}
	target.Status.LastCheckpointID = forked.ID
	target.Status.RestoredCheckpoint = forked.ID
	target.Status.SystemGeneration = sourceCheckpoint.SystemGeneration
	if target.Status.SystemGeneration == 0 {
		target.Status.SystemGeneration = source.Status.SystemGeneration
	}
	target.Status.LastEventSequence = event.Sequence
	target.Status.Phase = resources.SessionPhasePaused
	target.Status.ActiveTurnID = ""
	target.Status.ClaimedBy = ""
	target.Status.LeaseUntil = ""
	target.Status.LastHeartbeat = ""
	target.Status.BlockedOn = nil
	return forked, event, nil
}

func materializeSessionForkedEvent(
	target *resources.Session,
	source resources.Session,
	sourceCheckpoint resources.SessionCheckpoint,
	forkedCheckpoint resources.SessionCheckpoint,
	now time.Time,
) resources.SessionEvent {
	event := newSessionEventAt(
		*target,
		resources.SessionEventSessionForked,
		"",
		"",
		0,
		map[string]any{
			"source_session":       source.Metadata.Name,
			"source_checkpoint_id": sourceCheckpoint.ID,
			"checkpoint_id":        forkedCheckpoint.ID,
		},
		now,
	)
	event.Sequence = target.Status.LastEventSequence + 1
	target.Status.LastEventSequence = event.Sequence
	touchSession(target, now)
	return event
}

func (s *SessionStore) PruneCheckpoints(ctx context.Context, name string) ([]resources.SessionCheckpoint, error) {
	key := normalizeLookupName(name)
	now := time.Now().UTC()
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		session, found, err := getFromTableForUpdate[resources.Session](ctx, tx, tableSessions, key)
		if err != nil || !found {
			if err == nil {
				err = fmt.Errorf("session %q not found", name)
			}
			return nil, err
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT payload
			FROM session_checkpoints
			WHERE session_name = $1
			ORDER BY event_seq DESC, created_at DESC
			FOR UPDATE`, key)
		if err != nil {
			return nil, err
		}
		checkpoints := make([]resources.SessionCheckpoint, 0)
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				return nil, err
			}
			var checkpoint resources.SessionCheckpoint
			if err := json.Unmarshal(raw, &checkpoint); err != nil {
				rows.Close()
				return nil, err
			}
			checkpoints = append(checkpoints, checkpoint)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		prune := selectSessionCheckpointsToPrune(session, checkpoints, now)
		for _, checkpoint := range prune {
			if _, err = tx.ExecContext(ctx,
				`DELETE FROM session_checkpoints WHERE session_name = $1 AND checkpoint_id = $2`,
				key, checkpoint.ID,
			); err != nil {
				return nil, err
			}
		}
		advanceCheckpointHeadAfterPrune(&session, checkpoints, prune)
		if len(prune) > 0 {
			event := checkpointPrunedEvent(&session, prune, now)
			if err := bumpSessionStatusMetadata(&session, session.Metadata); err != nil {
				return nil, err
			}
			if err := insertSessionEventSQL(ctx, tx, key, event); err != nil {
				return nil, err
			}
			if err := upsertSessionSQL(ctx, tx, key, session); err != nil {
				return nil, err
			}
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return prune, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, found := s.items[key]
	if !found {
		return nil, fmt.Errorf("session %q not found", name)
	}
	checkpoints := copySessionCheckpoints(s.checkpoints[key])
	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].EventSequence > checkpoints[j].EventSequence
	})
	prune := selectSessionCheckpointsToPrune(session, checkpoints, now)
	pruned := make(map[string]struct{}, len(prune))
	for _, checkpoint := range prune {
		pruned[checkpoint.ID] = struct{}{}
	}
	current := s.checkpoints[key]
	next := current[:0]
	for _, checkpoint := range current {
		if _, remove := pruned[checkpoint.ID]; !remove {
			next = append(next, checkpoint)
		}
	}
	s.checkpoints[key] = next
	advanceCheckpointHeadAfterPrune(&session, checkpoints, prune)
	if len(prune) > 0 {
		event := checkpointPrunedEvent(&session, prune, now)
		if err := bumpSessionStatusMetadata(&session, s.items[key].Metadata); err != nil {
			return nil, err
		}
		s.items[key] = session.DeepCopy()
		s.events[key] = append(s.events[key], event.DeepCopy())
	}
	return copySessionCheckpoints(prune), nil
}

func checkpointPrunedEvent(
	session *resources.Session,
	pruned []resources.SessionCheckpoint,
	now time.Time,
) resources.SessionEvent {
	ids := make([]string, 0, len(pruned))
	for _, checkpoint := range pruned {
		ids = append(ids, checkpoint.ID)
	}
	event := newSessionEventAt(
		*session,
		resources.SessionEventCheckpointPruned,
		"",
		"",
		0,
		map[string]any{
			"checkpoint_ids": ids,
			"count":          len(ids),
		},
		now,
	)
	event.Sequence = session.Status.LastEventSequence + 1
	session.Status.LastEventSequence = event.Sequence
	touchSession(session, now)
	return event
}

func selectSessionCheckpointsToPrune(
	session resources.Session,
	checkpoints []resources.SessionCheckpoint,
	now time.Time,
) []resources.SessionCheckpoint {
	maxCount := session.Spec.CheckpointRetention.MaxCount
	if maxCount <= 0 {
		maxCount = 100
	}
	byID := make(map[string]resources.SessionCheckpoint, len(checkpoints))
	for _, checkpoint := range checkpoints {
		byID[checkpoint.ID] = checkpoint
	}
	keep := make(map[string]struct{}, maxCount)
	headID := strings.TrimSpace(session.Status.LastCheckpointID)
	for len(keep) < maxCount && headID != "" {
		checkpoint, ok := byID[headID]
		if !ok {
			break
		}
		if !checkpointExpired(checkpoint, now) {
			keep[checkpoint.ID] = struct{}{}
		}
		headID = strings.TrimSpace(checkpoint.ParentCheckpointID)
	}
	for _, checkpoint := range checkpoints {
		if len(keep) >= maxCount {
			break
		}
		if _, retained := keep[checkpoint.ID]; retained || checkpointExpired(checkpoint, now) {
			continue
		}
		keep[checkpoint.ID] = struct{}{}
	}
	prune := make([]resources.SessionCheckpoint, 0)
	for _, checkpoint := range checkpoints {
		if _, retained := keep[checkpoint.ID]; !retained {
			prune = append(prune, checkpoint)
		}
	}
	return prune
}

func checkpointExpired(checkpoint resources.SessionCheckpoint, now time.Time) bool {
	expiresAt, err := time.Parse(time.RFC3339Nano, checkpoint.ExpiresAt)
	return err == nil && !expiresAt.After(now)
}

func advanceCheckpointHeadAfterPrune(
	session *resources.Session,
	checkpoints, pruned []resources.SessionCheckpoint,
) {
	prunedIDs := make(map[string]struct{}, len(pruned))
	for _, checkpoint := range pruned {
		prunedIDs[checkpoint.ID] = struct{}{}
	}
	if _, removed := prunedIDs[session.Status.LastCheckpointID]; !removed {
		return
	}
	remaining := make(map[string]resources.SessionCheckpoint, len(checkpoints)-len(pruned))
	for _, checkpoint := range checkpoints {
		if _, removed := prunedIDs[checkpoint.ID]; !removed {
			remaining[checkpoint.ID] = checkpoint
		}
	}
	nextID := ""
	for _, checkpoint := range checkpoints {
		if checkpoint.ID != session.Status.LastCheckpointID {
			continue
		}
		parentID := strings.TrimSpace(checkpoint.ParentCheckpointID)
		if _, exists := remaining[parentID]; exists {
			nextID = parentID
		}
		break
	}
	if nextID == "" {
		for _, checkpoint := range checkpoints {
			if _, exists := remaining[checkpoint.ID]; exists {
				nextID = checkpoint.ID
				break
			}
		}
	}
	session.Status.LastCheckpointID = nextID
	if _, removed := prunedIDs[session.Status.RestoredCheckpoint]; removed {
		session.Status.RestoredCheckpoint = ""
	}
}

func insertSessionCheckpointSQL(
	ctx context.Context,
	tx *sql.Tx,
	key string,
	checkpoint resources.SessionCheckpoint,
) error {
	raw, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	var expiresAt any
	if value := strings.TrimSpace(checkpoint.ExpiresAt); value != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, value)
		if parseErr != nil {
			return parseErr
		}
		expiresAt = parsed
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO session_checkpoints (
			session_name, checkpoint_id, turn_id, task_name, agent_name,
			agent_index, message_id, event_seq, safe_point, state_version, state_hash,
			payload, expires_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		key,
		checkpoint.ID,
		checkpoint.TurnID,
		checkpoint.TaskName,
		checkpoint.Agent,
		checkpoint.AgentIndex,
		checkpoint.MessageID,
		checkpoint.EventSequence,
		checkpoint.SafePoint,
		checkpoint.StateVersion,
		checkpoint.StateHash,
		raw,
		expiresAt,
		checkpoint.CreatedAt,
	)
	return err
}

func checkpointStateHash(state json.RawMessage) string {
	var value any
	canonical := state
	if err := json.Unmarshal(state, &value); err == nil {
		if encoded, marshalErr := json.Marshal(value); marshalErr == nil {
			canonical = encoded
		}
	} else {
		var compact bytes.Buffer
		if compactErr := json.Compact(&compact, state); compactErr == nil {
			canonical = compact.Bytes()
		}
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func copySessionCheckpoints(in []resources.SessionCheckpoint) []resources.SessionCheckpoint {
	out := make([]resources.SessionCheckpoint, len(in))
	for i := range in {
		out[i] = in[i].DeepCopy()
	}
	return out
}
