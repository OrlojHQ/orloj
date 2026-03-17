package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OAuth2TokenCache caches access tokens obtained via the client_credentials grant.
// Tokens are keyed by tokenURL+clientID and evicted on expiry or explicit eviction.
type OAuth2TokenCache struct {
	mu      sync.Mutex
	entries map[string]*oauth2CacheEntry
	client  HTTPDoer
}

type oauth2CacheEntry struct {
	accessToken string
	expiresAt   time.Time
}

type oauth2TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

func NewOAuth2TokenCache(client HTTPDoer) *OAuth2TokenCache {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &OAuth2TokenCache{
		entries: make(map[string]*oauth2CacheEntry),
		client:  client,
	}
}

func (c *OAuth2TokenCache) cacheKey(tokenURL, clientID string) string {
	return tokenURL + "\x00" + clientID
}

// GetToken returns a cached access token if valid, or performs a fresh exchange.
func (c *OAuth2TokenCache) GetToken(ctx context.Context, tokenURL, clientID, clientSecret, scope string) (string, error) {
	key := c.cacheKey(tokenURL, clientID)

	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && time.Now().Before(entry.expiresAt) {
		token := entry.accessToken
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	token, expiresIn, err := c.exchange(ctx, tokenURL, clientID, clientSecret, scope)
	if err != nil {
		return "", err
	}

	// Cache with a safety margin of 30 seconds before actual expiry
	ttl := time.Duration(expiresIn) * time.Second
	if ttl > 30*time.Second {
		ttl -= 30 * time.Second
	}
	if ttl < time.Second {
		ttl = time.Second
	}

	c.mu.Lock()
	c.entries[key] = &oauth2CacheEntry{
		accessToken: token,
		expiresAt:   time.Now().Add(ttl),
	}
	c.mu.Unlock()

	return token, nil
}

// Evict removes a cached token, forcing a fresh exchange on next GetToken.
func (c *OAuth2TokenCache) Evict(tokenURL, clientID string) {
	key := c.cacheKey(tokenURL, clientID)
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func (c *OAuth2TokenCache) exchange(ctx context.Context, tokenURL, clientID, clientSecret, scope string) (string, int64, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}
	if scope != "" {
		form.Set("scope", scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("oauth2 token request build failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("oauth2 token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return "", 0, fmt.Errorf("oauth2 token response read failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("oauth2 token endpoint returned HTTP %d: %s", resp.StatusCode, compactBody(string(body)))
	}

	var tokenResp oauth2TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", 0, fmt.Errorf("oauth2 token response parse failed: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", 0, fmt.Errorf("oauth2 token response missing access_token")
	}

	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	return tokenResp.AccessToken, expiresIn, nil
}
