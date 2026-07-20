package store

import (
	"context"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

func newTestSession(t *testing.T, store *SessionStore, name string) resources.Session {
	t.Helper()
	session, err := store.Upsert(context.Background(), resources.Session{
		Metadata: resources.ObjectMeta{Name: name},
		Spec: resources.SessionSpec{
			System:  "support",
			IdleTTL: "1h",
		},
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return session
}

func TestSessionStoreTurnLifecycleAndIdempotency(t *testing.T) {
	s := NewSessionStore()
	session := newTestSession(t, s, "chat")
	if session.Status.LastEventSequence != 1 {
		t.Fatalf("initial event sequence = %d, want 1", session.Status.LastEventSequence)
	}

	turn, events, created, err := s.EnqueueTurn(context.Background(), "chat", resources.SessionTurn{
		Content:        "hello",
		IdempotencyKey: "request-1",
	})
	if err != nil {
		t.Fatalf("enqueue turn: %v", err)
	}
	if !created || len(events) != 2 {
		t.Fatalf("enqueue created=%v events=%d, want true/2", created, len(events))
	}
	duplicate, duplicateEvents, created, err := s.EnqueueTurn(context.Background(), "chat", resources.SessionTurn{
		Content:        "different content is ignored",
		IdempotencyKey: "request-1",
	})
	if err != nil {
		t.Fatalf("duplicate enqueue: %v", err)
	}
	if created || len(duplicateEvents) != 0 || duplicate.ID != turn.ID {
		t.Fatalf("duplicate result created=%v events=%d id=%q, want existing %q", created, len(duplicateEvents), duplicate.ID, turn.ID)
	}

	claim, ok, claimEvents, err := s.ClaimNextTurn(context.Background(), "worker-a", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if len(claimEvents) != 1 || claimEvents[0].Type != resources.SessionEventTurnStarted {
		t.Fatalf("claim events = %#v", claimEvents)
	}
	if claim.Turn.Fence == 0 || claim.Session.Status.Phase != resources.SessionPhaseRunning {
		t.Fatalf("claim = %#v", claim)
	}

	delta, err := s.AppendEvent(context.Background(), "chat", claim.Turn.ID, claim.Turn.ClaimedBy, claim.Turn.Fence, resources.SessionEvent{
		Type:      resources.SessionEventMessageDelta,
		MessageID: claim.Turn.AssistantMessageID,
		Payload:   map[string]any{"delta": "hi"},
	})
	if err != nil {
		t.Fatalf("append delta: %v", err)
	}
	if delta.Sequence <= claimEvents[0].Sequence {
		t.Fatalf("delta sequence %d did not advance", delta.Sequence)
	}

	completedEvents, completed, err := s.CompleteTurn(context.Background(), claim, "hi there", map[string]any{"total_tokens": 2})
	if err != nil {
		t.Fatalf("complete turn: %v", err)
	}
	if len(completedEvents) != 2 || completed.Status.Phase != resources.SessionPhaseWaitingInput {
		t.Fatalf("complete events=%d session=%#v", len(completedEvents), completed.Status)
	}
	if completed.Status.CompletedTurns != 1 || completed.Status.ActiveTurnID != "" {
		t.Fatalf("unexpected completed status: %#v", completed.Status)
	}
}

func TestSessionStoreApprovalStatePreservesLeaseAndFence(t *testing.T) {
	s := NewSessionStore()
	newTestSession(t, s, "approval")
	if _, _, _, err := s.EnqueueTurn(context.Background(), "approval", resources.SessionTurn{
		Content:        "deploy",
		IdempotencyKey: "approval-turn",
	}); err != nil {
		t.Fatal(err)
	}
	claim, ok, _, err := s.ClaimNextTurn(context.Background(), "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	events, waiting, err := s.SetApprovalState(context.Background(), claim, true, &resources.TaskBlockedOn{
		Kind: "ToolApproval", Name: "deploy", Reason: "review required",
	})
	if err != nil {
		t.Fatalf("set waiting approval: %v", err)
	}
	if len(events) != 1 || events[0].Type != resources.SessionEventApprovalRequested {
		t.Fatalf("waiting events = %#v", events)
	}
	if waiting.Status.Phase != resources.SessionPhaseWaitingApproval || waiting.Status.BlockedOn == nil {
		t.Fatalf("waiting status = %#v", waiting.Status)
	}
	if err := s.RenewLease(context.Background(), "approval", claim.Turn.ID, claim.Turn.ClaimedBy, claim.Turn.Fence, time.Minute); err != nil {
		t.Fatalf("renew approval lease: %v", err)
	}
	events, resumed, err := s.SetApprovalState(context.Background(), claim, false, nil)
	if err != nil {
		t.Fatalf("resolve approval: %v", err)
	}
	if len(events) != 1 || events[0].Type != resources.SessionEventApprovalResolved {
		t.Fatalf("resolved events = %#v", events)
	}
	if resumed.Status.Phase != resources.SessionPhaseRunning || resumed.Status.BlockedOn != nil {
		t.Fatalf("resumed status = %#v", resumed.Status)
	}
}

func TestSessionStoreReclaimsExpiredApprovalLease(t *testing.T) {
	s := NewSessionStore()
	newTestSession(t, s, "approval-reclaim")
	if _, _, _, err := s.EnqueueTurn(context.Background(), "approval-reclaim", resources.SessionTurn{
		Content:        "deploy",
		IdempotencyKey: "approval-reclaim-turn",
	}); err != nil {
		t.Fatal(err)
	}
	first, ok, _, err := s.ClaimNextTurn(context.Background(), "worker-a", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first claim ok=%v err=%v", ok, err)
	}
	if _, _, err := s.SetApprovalState(context.Background(), first, true, &resources.TaskBlockedOn{
		Kind: "ToolApproval", Name: "deploy",
	}); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	key := normalizeLookupName("approval-reclaim")
	session := s.items[key]
	session.Status.LeaseUntil = past
	s.items[key] = session
	s.turns[key][0].LeaseUntil = past

	second, ok, events, err := s.ClaimNextTurn(context.Background(), "worker-b", time.Minute)
	if err != nil || !ok {
		t.Fatalf("reclaim ok=%v err=%v", ok, err)
	}
	if second.Turn.Attempt != 2 || second.Turn.Fence == first.Turn.Fence {
		t.Fatalf("reclaimed turn = %#v", second.Turn)
	}
	if len(events) < 3 || events[0].Type != resources.SessionEventTurnRetrying ||
		events[1].Type != resources.SessionEventMessageReset {
		t.Fatalf("reclaim events = %#v", events)
	}
}

func TestSessionStoreExpiresIdleSessionAndQueuedTurns(t *testing.T) {
	s := NewSessionStore()
	session, err := s.Upsert(context.Background(), resources.Session{
		Metadata: resources.ObjectMeta{Name: "idle"},
		Spec: resources.SessionSpec{
			System:  "support",
			IdleTTL: "1h",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.EnqueueTurn(context.Background(), session.Metadata.Name, resources.SessionTurn{
		Content:        "queued",
		IdempotencyKey: "queued-turn",
	}); err != nil {
		t.Fatal(err)
	}
	session, _, err = s.Get(context.Background(), session.Metadata.Name)
	if err != nil {
		t.Fatal(err)
	}
	session.Status.ExpiresAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	if _, err := s.Upsert(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	events, err := s.ExpireIdleSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != resources.SessionEventSessionExpired {
		t.Fatalf("expiry events = %#v", events)
	}
	expired, ok, err := s.Get(context.Background(), session.Metadata.Name)
	if err != nil || !ok {
		t.Fatalf("get expired session ok=%v err=%v", ok, err)
	}
	if expired.Status.Phase != resources.SessionPhaseExpired || expired.Status.QueuedTurns != 0 {
		t.Fatalf("expired status = %#v", expired.Status)
	}
	turns, err := s.ListTurns(context.Background(), session.Metadata.Name)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Phase != resources.SessionTurnPhaseCancelled {
		t.Fatalf("expired turns = %#v", turns)
	}
}

func TestSessionStorePauseFencesAndRequeuesActiveTurn(t *testing.T) {
	s := NewSessionStore()
	newTestSession(t, s, "pausable")
	_, _, _, err := s.EnqueueTurn(context.Background(), "pausable", resources.SessionTurn{
		Content:        "do work",
		IdempotencyKey: "request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, ok, _, err := s.ClaimNextTurn(context.Background(), "worker-a", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first claim ok=%v err=%v", ok, err)
	}

	controlEvents, paused, err := s.ApplyControl(context.Background(), "pausable", "pause", "user requested")
	if err != nil {
		t.Fatalf("pause: %v", err)
	}
	if paused.Status.Phase != resources.SessionPhasePaused || paused.Status.QueuedTurns != 1 {
		t.Fatalf("paused status = %#v", paused.Status)
	}
	if len(controlEvents) != 2 || controlEvents[0].Type != resources.SessionEventMessageReset {
		t.Fatalf("pause events = %#v", controlEvents)
	}
	if _, err := s.AppendEvent(context.Background(), "pausable", first.Turn.ID, first.Turn.ClaimedBy, first.Turn.Fence, resources.SessionEvent{Type: resources.SessionEventMessageDelta}); err == nil {
		t.Fatal("stale worker append unexpectedly succeeded")
	}

	if _, _, err := s.ApplyControl(context.Background(), "pausable", "resume", ""); err != nil {
		t.Fatalf("resume: %v", err)
	}
	second, ok, events, err := s.ClaimNextTurn(context.Background(), "worker-b", time.Minute)
	if err != nil || !ok {
		t.Fatalf("second claim ok=%v err=%v", ok, err)
	}
	if second.Turn.Attempt != 2 || second.Turn.Fence == first.Turn.Fence {
		t.Fatalf("retry claim = %#v, first fence=%d", second.Turn, first.Turn.Fence)
	}
	if len(events) != 1 || events[0].Type != resources.SessionEventTurnStarted {
		t.Fatalf("resume claim events = %#v", events)
	}
}

func TestSessionStoreInterruptCancelsActiveTurnAndQueuesReplacement(t *testing.T) {
	s := NewSessionStore()
	newTestSession(t, s, "interrupt")
	if _, _, _, err := s.EnqueueTurn(context.Background(), "interrupt", resources.SessionTurn{
		Content:        "first",
		IdempotencyKey: "first",
	}); err != nil {
		t.Fatal(err)
	}
	first, ok, _, err := s.ClaimNextTurn(context.Background(), "worker-a", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first claim ok=%v err=%v", ok, err)
	}
	replacement, events, created, err := s.EnqueueTurn(context.Background(), "interrupt", resources.SessionTurn{
		Content:        "stop and do this",
		Interrupt:      true,
		IdempotencyKey: "replacement",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created || len(events) != 4 {
		t.Fatalf("replacement created=%v events=%#v", created, events)
	}
	if events[0].Type != resources.SessionEventMessageReset ||
		events[1].Type != resources.SessionEventTurnCancelled ||
		events[2].Type != resources.SessionEventTurnQueued {
		t.Fatalf("interrupt events = %#v", events)
	}
	if _, err := s.AppendEvent(context.Background(), "interrupt", first.Turn.ID, first.Turn.ClaimedBy, first.Turn.Fence, resources.SessionEvent{
		Type: resources.SessionEventMessageDelta,
	}); err == nil {
		t.Fatal("interrupted worker append unexpectedly succeeded")
	}
	next, ok, _, err := s.ClaimNextTurn(context.Background(), "worker-b", time.Minute)
	if err != nil || !ok {
		t.Fatalf("replacement claim ok=%v err=%v", ok, err)
	}
	if next.Turn.ID != replacement.ID {
		t.Fatalf("claimed turn %q, want replacement %q", next.Turn.ID, replacement.ID)
	}
	turns, err := s.ListTurns(context.Background(), "interrupt")
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Phase != resources.SessionTurnPhaseCancelled {
		t.Fatalf("turns after interrupt = %#v", turns)
	}
}

func TestSessionStoreSerializesConcurrentTurns(t *testing.T) {
	s := NewSessionStore()
	newTestSession(t, s, "ordered")
	for _, item := range []struct {
		content string
		key     string
	}{
		{"first", "one"},
		{"second", "two"},
	} {
		if _, _, _, err := s.EnqueueTurn(context.Background(), "ordered", resources.SessionTurn{
			Content:        item.content,
			IdempotencyKey: item.key,
		}); err != nil {
			t.Fatal(err)
		}
	}
	first, ok, _, err := s.ClaimNextTurn(context.Background(), "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first claim ok=%v err=%v", ok, err)
	}
	if first.Turn.Content != "first" {
		t.Fatalf("claimed %q first", first.Turn.Content)
	}
	if _, ok, _, err := s.ClaimNextTurn(context.Background(), "other", time.Minute); err != nil || ok {
		t.Fatalf("second concurrent claim ok=%v err=%v, want unavailable", ok, err)
	}
}
