package a2a

import (
	"testing"
	"time"
)

func TestResolveCardURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/v1/agents/foo/a2a", "https://example.com/v1/agents/foo/.well-known/agent-card.json"},
		{"https://example.com", "https://example.com/.well-known/agent-card.json"},
		{"https://example.com/", "https://example.com/.well-known/agent-card.json"},
		{"https://example.com/.well-known/agent-card.json", "https://example.com/.well-known/agent-card.json"},
		{"https://example.com/.well-known/agent.json", "https://example.com/.well-known/agent.json"},
	}

	for _, tt := range tests {
		got := resolveCardURL(tt.input)
		if got != tt.expected {
			t.Errorf("resolveCardURL(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient(ClientConfig{
		AllowPrivate: false,
		CardCacheTTL: 10 * time.Second,
	})

	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.cardCacheTTL != 10*time.Second {
		t.Errorf("expected 10s TTL, got %s", client.cardCacheTTL)
	}
}

func TestNewClientDefaultTTL(t *testing.T) {
	client := NewClient(ClientConfig{})

	if client.cardCacheTTL != 5*time.Minute {
		t.Errorf("expected 5m default TTL, got %s", client.cardCacheTTL)
	}
}

func TestClientCacheStatus_Empty(t *testing.T) {
	client := NewClient(ClientConfig{})

	ts, hasErr := client.CacheStatus("https://not-cached.example.com")
	if !ts.IsZero() {
		t.Errorf("expected zero time for uncached URL, got %v", ts)
	}
	if hasErr {
		t.Error("expected no error for uncached URL")
	}
}
