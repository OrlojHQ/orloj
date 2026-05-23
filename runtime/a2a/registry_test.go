package a2a

import (
	"log"
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	client := NewClient(ClientConfig{CardCacheTTL: 1 * time.Minute})
	configs := []RemoteAgentConfig{
		{Name: "agent-a", URL: "https://a.example.com/.well-known/agent-card.json"},
		{Name: "agent-b", URL: "https://b.example.com/.well-known/agent-card.json"},
	}

	registry := NewRegistry(client, configs, 5*time.Minute, log.Default())

	if registry == nil {
		t.Fatal("expected non-nil registry")
	}

	entries := registry.List()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestRegistryGet(t *testing.T) {
	client := NewClient(ClientConfig{})
	configs := []RemoteAgentConfig{
		{Name: "existing", URL: "https://existing.example.com"},
	}

	registry := NewRegistry(client, configs, 5*time.Minute, nil)

	entry, ok := registry.Get("existing")
	if !ok {
		t.Fatal("expected to find entry 'existing'")
	}
	if entry.Name != "existing" {
		t.Errorf("expected name 'existing', got %q", entry.Name)
	}

	_, ok = registry.Get("missing")
	if ok {
		t.Error("expected missing entry to not be found")
	}
}

func TestRegistryDefaultTTL(t *testing.T) {
	client := NewClient(ClientConfig{})
	registry := NewRegistry(client, nil, 0, nil)

	if registry.ttl != 5*time.Minute {
		t.Errorf("expected 5m default TTL, got %s", registry.ttl)
	}
}
