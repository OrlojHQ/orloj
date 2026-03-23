package store

import (
	"context"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestAgentStoreVersioningAndConflict(t *testing.T) {
	s := NewAgentStore()

	created, err := s.Upsert(context.Background(), resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "a1"},
		Spec:       resources.AgentSpec{ModelRef: "openai-default", Prompt: "p1"},
	})
	if err != nil {
		t.Fatalf("create upsert failed: %v", err)
	}
	if created.Metadata.ResourceVersion != "1" {
		t.Fatalf("expected rv=1, got %q", created.Metadata.ResourceVersion)
	}
	if created.Metadata.Generation != 1 {
		t.Fatalf("expected generation=1, got %d", created.Metadata.Generation)
	}

	// status-only update should not bump generation.
	created.Status.Phase = "Running"
	statusUpdated, err := s.Upsert(context.Background(), created)
	if err != nil {
		t.Fatalf("status upsert failed: %v", err)
	}
	if statusUpdated.Metadata.ResourceVersion != "2" {
		t.Fatalf("expected rv=2, got %q", statusUpdated.Metadata.ResourceVersion)
	}
	if statusUpdated.Metadata.Generation != 1 {
		t.Fatalf("expected generation to stay 1, got %d", statusUpdated.Metadata.Generation)
	}

	// spec update should bump generation.
	statusUpdated.Spec.ModelRef = "openai-v2"
	specUpdated, err := s.Upsert(context.Background(), statusUpdated)
	if err != nil {
		t.Fatalf("spec upsert failed: %v", err)
	}
	if specUpdated.Metadata.ResourceVersion != "3" {
		t.Fatalf("expected rv=3, got %q", specUpdated.Metadata.ResourceVersion)
	}
	if specUpdated.Metadata.Generation != 2 {
		t.Fatalf("expected generation=2, got %d", specUpdated.Metadata.Generation)
	}

	stale := specUpdated
	stale.Metadata.ResourceVersion = "1"
	if _, err := s.Upsert(context.Background(), stale); err == nil {
		t.Fatal("expected conflict error, got nil")
	} else if !IsConflict(err) {
		t.Fatalf("expected conflict error, got %T %v", err, err)
	}
}
