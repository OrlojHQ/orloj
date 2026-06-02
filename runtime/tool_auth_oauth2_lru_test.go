package agentruntime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type countingOAuth2Doer struct {
	calls int
}

func (d *countingOAuth2Doer) Do(_ *http.Request) (*http.Response, error) {
	d.calls++
	body := fmt.Sprintf(`{"access_token":"tok-%d","expires_in":3600}`, d.calls)
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func TestOAuth2TokenCacheLRUEviction(t *testing.T) {
	doer := &countingOAuth2Doer{}
	cache := NewOAuth2TokenCache(doer)
	cache.maxSize = 2

	ctx := context.Background()
	if _, err := cache.GetToken(ctx, "https://auth.example/a", "client-a", "secret", ""); err != nil {
		t.Fatalf("first token: %v", err)
	}
	if _, err := cache.GetToken(ctx, "https://auth.example/b", "client-b", "secret", ""); err != nil {
		t.Fatalf("second token: %v", err)
	}
	if _, err := cache.GetToken(ctx, "https://auth.example/c", "client-c", "secret", ""); err != nil {
		t.Fatalf("third token: %v", err)
	}

	// client-a was LRU-evicted; fetching again should exchange again.
	doer.calls = 0
	if _, err := cache.GetToken(ctx, "https://auth.example/a", "client-a", "secret", ""); err != nil {
		t.Fatalf("re-fetch evicted token: %v", err)
	}
	if doer.calls != 1 {
		t.Fatalf("expected one exchange after eviction, got %d", doer.calls)
	}
}
