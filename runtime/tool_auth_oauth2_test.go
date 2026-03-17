package agentruntime_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
)

type fakeOAuth2Doer struct {
	accessToken string
	expiresIn   int64
	statusCode  int
}

func (d *fakeOAuth2Doer) Do(req *http.Request) (*http.Response, error) {
	if d.statusCode >= 400 {
		return &http.Response{
			StatusCode: d.statusCode,
			Body:       io.NopCloser(strings.NewReader("error")),
		}, nil
	}
	body, _ := json.Marshal(map[string]any{
		"access_token": d.accessToken,
		"token_type":   "Bearer",
		"expires_in":   d.expiresIn,
	})
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}, nil
}

func TestOAuth2TokenCacheGetToken(t *testing.T) {
	doer := &fakeOAuth2Doer{accessToken: "tok-abc", expiresIn: 3600}
	cache := agentruntime.NewOAuth2TokenCache(doer)

	token, err := cache.GetToken(context.Background(), "https://auth.example/token", "cid", "csec", "read write")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "tok-abc" {
		t.Fatalf("expected tok-abc, got %q", token)
	}
}

func TestOAuth2TokenCacheReturnsCached(t *testing.T) {
	doer := &fakeOAuth2Doer{accessToken: "tok-first", expiresIn: 3600}
	cache := agentruntime.NewOAuth2TokenCache(doer)

	token1, _ := cache.GetToken(context.Background(), "https://auth.example/token", "cid", "csec", "")
	doer.accessToken = "tok-second"
	token2, _ := cache.GetToken(context.Background(), "https://auth.example/token", "cid", "csec", "")

	if token1 != token2 {
		t.Fatalf("expected cached token %q, got %q", token1, token2)
	}
}

func TestOAuth2TokenCacheEvict(t *testing.T) {
	doer := &fakeOAuth2Doer{accessToken: "tok-first", expiresIn: 3600}
	cache := agentruntime.NewOAuth2TokenCache(doer)

	_, _ = cache.GetToken(context.Background(), "https://auth.example/token", "cid", "csec", "")
	cache.Evict("https://auth.example/token", "cid")

	doer.accessToken = "tok-refreshed"
	token, _ := cache.GetToken(context.Background(), "https://auth.example/token", "cid", "csec", "")
	if token != "tok-refreshed" {
		t.Fatalf("expected tok-refreshed after evict, got %q", token)
	}
}

func TestOAuth2TokenCacheTokenEndpointError(t *testing.T) {
	doer := &fakeOAuth2Doer{statusCode: 400}
	cache := agentruntime.NewOAuth2TokenCache(doer)

	_, err := cache.GetToken(context.Background(), "https://auth.example/token", "cid", "csec", "")
	if err == nil {
		t.Fatal("expected error on 400 from token endpoint")
	}
}

func TestOAuth2TokenCacheShortExpiry(t *testing.T) {
	doer := &fakeOAuth2Doer{accessToken: "tok-short", expiresIn: 1}
	cache := agentruntime.NewOAuth2TokenCache(doer)

	token, _ := cache.GetToken(context.Background(), "https://auth.example/token", "cid", "csec", "")
	if token != "tok-short" {
		t.Fatalf("expected tok-short, got %q", token)
	}
	time.Sleep(1100 * time.Millisecond)

	doer.accessToken = "tok-renewed"
	token2, _ := cache.GetToken(context.Background(), "https://auth.example/token", "cid", "csec", "")
	if token2 != "tok-renewed" {
		t.Fatalf("expected tok-renewed after expiry, got %q", token2)
	}
}
