package store

import (
	"context"
	"testing"
	"time"
)

func TestWebhookDedupeStore_PutGetInMemory(t *testing.T) {
	ctx := context.Background()
	s := NewWebhookDedupeStore()
	exp := time.Now().UTC().Add(time.Hour)

	if err := s.Put(ctx, "ep1", "evt1", "task-a", exp); err != nil {
		t.Fatal(err)
	}
	name, ok, err := s.Get(ctx, "ep1", "evt1", time.Now().UTC())
	if err != nil || !ok || name != "task-a" {
		t.Fatalf("Get: ok=%v name=%q err=%v", ok, name, err)
	}
}

func TestWebhookDedupeStore_PutEmptyIDsNoOp(t *testing.T) {
	ctx := context.Background()
	s := NewWebhookDedupeStore()
	if err := s.Put(ctx, "", "e", "t", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, "ep", "", "t", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, "ep", "e", "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	_, ok, err := s.Get(ctx, "ep", "e", time.Now().UTC())
	if err != nil || ok {
		t.Fatalf("expected no entry, ok=%v err=%v", ok, err)
	}
}

func TestWebhookDedupeStore_GetEmptyIDs(t *testing.T) {
	ctx := context.Background()
	s := NewWebhookDedupeStore()
	_, ok, err := s.Get(ctx, "", "e", time.Now().UTC())
	if err != nil || ok {
		t.Fatalf("expected empty, ok=%v", ok)
	}
}

func TestWebhookDedupeStore_TryInsertDuplicate(t *testing.T) {
	ctx := context.Background()
	s := NewWebhookDedupeStore()
	now := time.Now().UTC()
	exp := now.Add(time.Hour)

	existing, dup, err := s.TryInsert(ctx, "ep", "ev", "first", exp, now)
	if err != nil || dup || existing != "" {
		t.Fatalf("first insert: existing=%q dup=%v err=%v", existing, dup, err)
	}
	existing, dup, err = s.TryInsert(ctx, "ep", "ev", "second", exp, now)
	if err != nil || !dup || existing != "first" {
		t.Fatalf("second insert: existing=%q dup=%v err=%v", existing, dup, err)
	}
}

func TestWebhookDedupeStore_TryInsertEmptyNoOp(t *testing.T) {
	ctx := context.Background()
	s := NewWebhookDedupeStore()
	now := time.Now().UTC()
	existing, dup, err := s.TryInsert(ctx, "", "e", "t", now, now)
	if err != nil || dup || existing != "" {
		t.Fatalf("expected noop, existing=%q dup=%v", existing, dup)
	}
}

func TestWebhookDedupeStore_PruneExpired(t *testing.T) {
	ctx := context.Background()
	s := NewWebhookDedupeStore()
	now := time.Now().UTC()
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)

	_ = s.Put(ctx, "ep-expired", "ev1", "gone", past)
	_ = s.Put(ctx, "ep-live", "ev2", "stay", future)

	if err := s.PruneExpired(ctx, now); err != nil {
		t.Fatal(err)
	}
	_, ok, _ := s.Get(ctx, "ep-expired", "ev1", now)
	if ok {
		t.Fatal("expected expired entry pruned by PruneExpired")
	}
	name, ok, err := s.Get(ctx, "ep-live", "ev2", now)
	if err != nil || !ok || name != "stay" {
		t.Fatalf("expected live entry to remain: ok=%v name=%q err=%v", ok, name, err)
	}
}

func TestDedupeKey(t *testing.T) {
	if dedupeKey("a", "b") != "a\x00b" {
		t.Fatalf("unexpected dedupeKey: %q", dedupeKey("a", "b"))
	}
}
