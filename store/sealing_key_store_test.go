package store

import (
	"context"
	"testing"
)

func TestSealingKeyStoreEnsureActiveStable(t *testing.T) {
	ctx := context.Background()
	s := NewSealingKeyStore()

	first, err := s.EnsureActive(ctx)
	if err != nil {
		t.Fatalf("ensure active first call: %v", err)
	}
	if first.KeyID == "" {
		t.Fatal("expected generated key id")
	}
	if first.PublicKeyPEM == "" || first.PrivateKeyPEM == "" {
		t.Fatal("expected generated key material")
	}

	second, err := s.EnsureActive(ctx)
	if err != nil {
		t.Fatalf("ensure active second call: %v", err)
	}
	if second.KeyID != first.KeyID {
		t.Fatalf("expected stable active key, got %q then %q", first.KeyID, second.KeyID)
	}

	active, ok, err := s.GetActive(ctx)
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if !ok {
		t.Fatal("expected active key to be present")
	}
	if active.KeyID != first.KeyID {
		t.Fatalf("expected active key %q, got %q", first.KeyID, active.KeyID)
	}
}
