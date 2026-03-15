package store

import (
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/crds"
)

func TestTaskStoreClaimAndRenewLease(t *testing.T) {
	s := NewTaskStore()
	task := crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "t1"},
		Spec: crds.TaskSpec{
			System: "sys",
			Input:  map[string]string{"topic": "x"},
			Retry:  crds.TaskRetryPolicy{MaxAttempts: 3, Backoff: "10ms"},
		},
	}
	if _, err := s.Upsert(task); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	claimed, ok, err := s.ClaimIfDue("t1", "worker-a", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if !ok {
		t.Fatal("expected claim success")
	}
	if claimed.Status.Phase != "Running" {
		t.Fatalf("expected Running, got %q", claimed.Status.Phase)
	}
	if claimed.Status.ClaimedBy != "worker-a" {
		t.Fatalf("expected claimedBy=worker-a, got %q", claimed.Status.ClaimedBy)
	}

	if err := s.RenewLease("t1", "worker-a", 50*time.Millisecond); err != nil {
		t.Fatalf("renew lease failed: %v", err)
	}

	if err := s.RenewLease("t1", "worker-b", 50*time.Millisecond); err == nil {
		t.Fatal("expected renew lease to fail for non-owner worker")
	}
}

func TestTaskStoreClaimFailoverOnLeaseExpiry(t *testing.T) {
	s := NewTaskStore()
	task := crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "t2"},
		Spec: crds.TaskSpec{
			System: "sys",
			Input:  map[string]string{"topic": "x"},
		},
	}
	if _, err := s.Upsert(task); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	if _, ok, err := s.ClaimIfDue("t2", "worker-a", 20*time.Millisecond); err != nil {
		t.Fatalf("first claim failed: %v", err)
	} else if !ok {
		t.Fatal("expected first claim success")
	}

	if _, ok, err := s.ClaimIfDue("t2", "worker-b", 20*time.Millisecond); err != nil {
		t.Fatalf("second claim before expiry failed: %v", err)
	} else if ok {
		t.Fatal("expected second claim to fail before lease expiry")
	}

	time.Sleep(30 * time.Millisecond)
	claimed, ok, err := s.ClaimIfDue("t2", "worker-b", 20*time.Millisecond)
	if err != nil {
		t.Fatalf("claim after expiry failed: %v", err)
	}
	if !ok {
		t.Fatal("expected claim success after lease expiry")
	}
	if claimed.Status.ClaimedBy != "worker-b" {
		t.Fatalf("expected claimedBy=worker-b, got %q", claimed.Status.ClaimedBy)
	}
}
