package agentruntime

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

type staticSecretLookup struct {
	items map[string]resources.Secret
}

func (l staticSecretLookup) Get(name string) (resources.Secret, bool) {
	item, ok := l.items[name]
	return item, ok
}

func TestStoreSecretResolverResolvesNamespacedSecretValue(t *testing.T) {
	lookup := staticSecretLookup{
		items: map[string]resources.Secret{
			"team-a/openai-key": {
				Metadata: resources.ObjectMeta{Name: "openai-key", Namespace: "team-a"},
				Spec: resources.SecretSpec{
					Data: map[string]string{
						"value": base64.StdEncoding.EncodeToString([]byte("sk-abc-123")),
					},
				},
			},
		},
	}
	resolver := NewStoreSecretResolver(lookup, "value").WithNamespace("team-a")
	value, err := resolver.Resolve(context.Background(), "openai-key")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if value != "sk-abc-123" {
		t.Fatalf("expected secret value sk-abc-123, got %q", value)
	}
}

func TestStoreSecretResolverSupportsKeyOverride(t *testing.T) {
	lookup := staticSecretLookup{
		items: map[string]resources.Secret{
			"default/multi": {
				Metadata: resources.ObjectMeta{Name: "multi", Namespace: "default"},
				Spec: resources.SecretSpec{
					Data: map[string]string{
						"token": base64.StdEncoding.EncodeToString([]byte("tok-xyz")),
					},
				},
			},
		},
	}
	resolver := NewStoreSecretResolver(lookup, "value")
	value, err := resolver.Resolve(context.Background(), "multi:token")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if value != "tok-xyz" {
		t.Fatalf("expected key override tok-xyz, got %q", value)
	}
}

func TestChainSecretResolverFallsBackToEnvResolver(t *testing.T) {
	t.Setenv("ORLOJ_SECRET_SEARCH_API_KEY", "env-token")
	storeResolver := NewStoreSecretResolver(staticSecretLookup{items: map[string]resources.Secret{}}, "value")
	envResolver := NewEnvSecretResolver("ORLOJ_SECRET_")
	chain := NewChainSecretResolver(storeResolver, envResolver).WithNamespace("default")
	value, err := chain.Resolve(context.Background(), "search-api-key")
	if err != nil {
		t.Fatalf("chain resolve failed: %v", err)
	}
	if value != "env-token" {
		t.Fatalf("expected env fallback value env-token, got %q", value)
	}
}

func TestParseSecretRefDefaults(t *testing.T) {
	ns, name, key, err := parseSecretRef("openai", "team-a", "value")
	if err != nil {
		t.Fatalf("parseSecretRef failed: %v", err)
	}
	if ns != "team-a" || name != "openai" || key != "value" {
		t.Fatalf("unexpected parsed ref ns=%q name=%q key=%q", ns, name, key)
	}
}

func TestChainSecretResolverReturnsNotFoundWhenAllResolversMiss(t *testing.T) {
	chain := NewChainSecretResolver(NewStoreSecretResolver(staticSecretLookup{items: map[string]resources.Secret{}}, "value"))
	_, err := chain.Resolve(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !errors.Is(err, ErrToolSecretNotFound) {
		t.Fatalf("expected ErrToolSecretNotFound, got %v", err)
	}
}
