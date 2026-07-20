package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

func createCheckpointedTurn(t *testing.T, s *SessionStore, name string) (SessionClaim, resources.SessionCheckpoint) {
	t.Helper()
	newTestSession(t, s, name)
	if _, _, _, err := s.EnqueueTurn(context.Background(), name, resources.SessionTurn{
		Content:        "investigate",
		IdempotencyKey: name + "-turn",
	}); err != nil {
		t.Fatal(err)
	}
	claim, claimed, _, err := s.ClaimNextTurn(context.Background(), "worker-a", time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim claimed=%v err=%v", claimed, err)
	}
	state, _ := json.Marshal(map[string]any{"version": 1, "next_step": 2, "history": []any{}})
	checkpoint, _, err := s.CreateCheckpoint(
		context.Background(),
		name,
		claim.Turn.ID,
		claim.Turn.ClaimedBy,
		claim.Turn.Fence,
		resources.SessionCheckpoint{
			TaskName:     "session-" + name,
			Agent:        "researcher",
			SafePoint:    resources.SessionCheckpointSafePointStep,
			StateVersion: resources.SessionCheckpointStateVersion,
			State:        state,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return claim, checkpoint
}

func TestSessionCheckpointCreateReplayAndLeaseRecovery(t *testing.T) {
	s := NewSessionStore()
	claim, checkpoint := createCheckpointedTurn(t, s, "recoverable")
	if checkpoint.StateHash == "" || checkpoint.EventSequence == 0 {
		t.Fatalf("checkpoint metadata = %#v", checkpoint)
	}
	replay, err := s.ReplayCheckpoint(context.Background(), "recoverable", checkpoint.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Verified || replay.FinalCheckpoint == nil || replay.CheckpointCount != 1 {
		t.Fatalf("replay = %#v", replay)
	}

	key := normalizeLookupName("recoverable")
	session := s.items[key]
	session.Status.LeaseUntil = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	s.items[key] = session
	s.turns[key][0].LeaseUntil = session.Status.LeaseUntil
	recovered, claimed, events, err := s.ClaimNextTurn(context.Background(), "worker-b", time.Minute)
	if err != nil || !claimed {
		t.Fatalf("reclaim claimed=%v err=%v", claimed, err)
	}
	if recovered.Checkpoint == nil || recovered.Checkpoint.ID != checkpoint.ID {
		t.Fatalf("recovered checkpoint = %#v", recovered.Checkpoint)
	}
	if len(events) < 3 ||
		events[1].Type != resources.SessionEventMessageReset ||
		events[2].Type != resources.SessionEventSessionRecovered {
		t.Fatalf("recovery events = %#v", events)
	}
	if recovered.Turn.Fence == claim.Turn.Fence {
		t.Fatal("recovery did not advance fence")
	}
}

func TestSessionCheckpointRewindAndForkAreIndependent(t *testing.T) {
	s := NewSessionStore()
	_, checkpoint := createCheckpointedTurn(t, s, "source")
	if _, _, _, err := s.EnqueueTurn(context.Background(), "source", resources.SessionTurn{
		Content:        "later turn",
		IdempotencyKey: "source-later",
	}); err != nil {
		t.Fatal(err)
	}
	rewound, event, err := s.RewindSession(context.Background(), "source", checkpoint.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if rewound.Status.Phase != resources.SessionPhasePaused ||
		rewound.Status.RestoredCheckpoint != checkpoint.ID ||
		event.Type != resources.SessionEventSessionRewound {
		t.Fatalf("rewind session=%#v event=%#v", rewound.Status, event)
	}
	turns, err := s.ListTurns(context.Background(), "source")
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 ||
		turns[0].Phase != resources.SessionTurnPhaseQueued ||
		turns[1].Phase != resources.SessionTurnPhaseCancelled ||
		rewound.Status.QueuedTurns != 1 {
		t.Fatalf("rewound turns = %#v", turns)
	}

	forked, forkCheckpoint, events, err := s.ForkSession(
		context.Background(),
		"source",
		checkpoint.ID,
		"forked",
	)
	if err != nil {
		t.Fatal(err)
	}
	if forked.Status.Phase != resources.SessionPhasePaused ||
		forked.Status.LastCheckpointID != forkCheckpoint.ID ||
		forkCheckpoint.ID == checkpoint.ID {
		t.Fatalf("forked session=%#v checkpoint=%#v", forked.Status, forkCheckpoint)
	}
	var inheritedUserMessage bool
	for _, event := range events {
		if event.Type == resources.SessionEventMessageCreated {
			inheritedUserMessage = true
		}
	}
	if !inheritedUserMessage {
		t.Fatalf("fork events did not materialize conversation history: %#v", events)
	}
	if err := s.Delete(context.Background(), "source"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.GetCheckpoint(context.Background(), "forked", forkCheckpoint.ID); err != nil || !found {
		t.Fatalf("fork checkpoint after source deletion found=%v err=%v", found, err)
	}
}

func TestSessionCheckpointRewindSelectsActiveLineage(t *testing.T) {
	s := NewSessionStore()
	claim, first := createCheckpointedTurn(t, s, "lineage")
	state := json.RawMessage(`{"version":1,"next_step":3}`)
	second, _, err := s.CreateCheckpoint(
		context.Background(),
		"lineage",
		claim.Turn.ID,
		claim.Turn.ClaimedBy,
		claim.Turn.Fence,
		resources.SessionCheckpoint{
			Agent:        "researcher",
			SafePoint:    resources.SessionCheckpointSafePointStep,
			StateVersion: resources.SessionCheckpointStateVersion,
			State:        state,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RewindSession(context.Background(), "lineage", first.ID, true); err != nil {
		t.Fatal(err)
	}
	latest, found, err := s.LatestExecutionCheckpoint(
		context.Background(),
		"lineage",
		claim.Turn.ID,
		"researcher",
		0,
		"",
		"",
	)
	if err != nil || !found {
		t.Fatalf("latest found=%v err=%v", found, err)
	}
	if latest.ID != first.ID || latest.ID == second.ID {
		t.Fatalf("latest active checkpoint=%s, want %s", latest.ID, first.ID)
	}
	if _, _, err := s.ApplyControl(context.Background(), "lineage", "resume", "continue selected lineage"); err != nil {
		t.Fatal(err)
	}
	recovered, claimed, _, err := s.ClaimNextTurn(context.Background(), "worker-b", time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim selected lineage claimed=%v err=%v", claimed, err)
	}
	if recovered.Checkpoint == nil || recovered.Checkpoint.ID != first.ID {
		t.Fatalf("claim restored checkpoint = %#v, want %s", recovered.Checkpoint, first.ID)
	}
}

func TestCheckpointStateHashCanonicalizesJSONObjects(t *testing.T) {
	first := json.RawMessage(`{"b":1,"a":{"z":2,"y":3}}`)
	second := json.RawMessage(`{"a":{"y":3,"z":2},"b":1}`)
	if checkpointStateHash(first) != checkpointStateHash(second) {
		t.Fatal("equivalent JSON objects produced different checkpoint hashes")
	}
}

func TestLatestCheckpointRejectsCorruptedState(t *testing.T) {
	s := NewSessionStore()
	claim, checkpoint := createCheckpointedTurn(t, s, "corrupted")
	key := normalizeLookupName("corrupted")
	s.checkpoints[key][0].State = json.RawMessage(`{"version":1,"next_step":999}`)
	if _, found, err := s.LatestExecutionCheckpoint(
		context.Background(),
		"corrupted",
		claim.Turn.ID,
		"researcher",
		0,
		"",
		"",
	); err == nil || found {
		t.Fatalf("corrupted checkpoint %s found=%v err=%v", checkpoint.ID, found, err)
	}
}

func TestSessionCheckpointRetentionExpiresHeadAndClearsStatus(t *testing.T) {
	s := NewSessionStore()
	_, checkpoint := createCheckpointedTurn(t, s, "expired-head")
	key := normalizeLookupName("expired-head")
	expiredAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	s.checkpoints[key][0].ExpiresAt = expiredAt
	pruned, err := s.PruneCheckpoints(context.Background(), "expired-head")
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned) != 1 || pruned[0].ID != checkpoint.ID {
		t.Fatalf("pruned checkpoints = %#v", pruned)
	}
	session := s.items[key]
	if session.Status.LastCheckpointID != "" {
		t.Fatalf("last checkpoint after expiry = %q", session.Status.LastCheckpointID)
	}
	events := s.events[key]
	if events[len(events)-1].Type != resources.SessionEventCheckpointPruned {
		t.Fatalf("last event = %#v", events[len(events)-1])
	}
}

func TestSessionCheckpointRetentionKeepsLatest(t *testing.T) {
	s := NewSessionStore()
	claim, _ := createCheckpointedTurn(t, s, "retained")
	key := normalizeLookupName("retained")
	session := s.items[key]
	session.Spec.CheckpointRetention.MaxCount = 2
	s.items[key] = session
	for step := 2; step <= 4; step++ {
		state, _ := json.Marshal(map[string]any{"version": 1, "next_step": step})
		if _, _, err := s.CreateCheckpoint(
			context.Background(),
			"retained",
			claim.Turn.ID,
			claim.Turn.ClaimedBy,
			claim.Turn.Fence,
			resources.SessionCheckpoint{
				Agent:        "researcher",
				SafePoint:    resources.SessionCheckpointSafePointStep,
				StateVersion: resources.SessionCheckpointStateVersion,
				State:        state,
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	latest := s.items[key].Status.LastCheckpointID
	pruned, err := s.PruneCheckpoints(context.Background(), "retained")
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned) != 2 {
		t.Fatalf("pruned %d checkpoints, want 2", len(pruned))
	}
	remaining, err := s.ListCheckpoints(context.Background(), "retained")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 || remaining[0].ID != latest {
		t.Fatalf("remaining checkpoints = %#v latest=%s", remaining, latest)
	}
}
