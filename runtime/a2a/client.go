package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/telemetry"
)

// Client handles outbound A2A requests to remote agents.
type Client struct {
	httpClient        *http.Client
	allowPrivate      bool
	skipURLValidation bool // test-only: bypasses ValidateEndpointURL
	cardCache         map[string]*cachedCard
	cardCacheTTL      time.Duration
	mu                sync.RWMutex
}

type cachedCard struct {
	card      AgentCard
	fetchedAt time.Time
	err       error
}

// ClientConfig configures the outbound A2A client.
type ClientConfig struct {
	AllowPrivate bool
	CardCacheTTL time.Duration
}

// NewClient creates a new outbound A2A client with SSRF-safe HTTP.
func NewClient(config ClientConfig) *Client {
	ttl := config.CardCacheTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Client{
		httpClient:   agentruntime.SafeHTTPClient(config.AllowPrivate, 60*time.Second),
		allowPrivate: config.AllowPrivate,
		cardCache:    make(map[string]*cachedCard),
		cardCacheTTL: ttl,
	}
}

// FetchCard retrieves and caches a remote Agent Card.
// Optional extraHeaders are applied to the HTTP request (e.g. auth).
func (c *Client) FetchCard(ctx context.Context, agentURL string, extraHeaders map[string]string) (AgentCard, error) {
	if !c.skipURLValidation {
		if err := agentruntime.ValidateEndpointURL(agentURL, c.allowPrivate); err != nil {
			return AgentCard{}, fmt.Errorf("a2a: unsafe agent URL: %w", err)
		}
	}

	c.mu.RLock()
	if cached, ok := c.cardCache[agentURL]; ok {
		if time.Since(cached.fetchedAt) < c.cardCacheTTL && cached.err == nil {
			c.mu.RUnlock()
			telemetry.RecordA2ACardCacheHit()
			return cached.card, nil
		}
	}
	c.mu.RUnlock()

	telemetry.RecordA2ACardCacheMiss()
	cardURL := resolveCardURL(agentURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cardURL, nil)
	if err != nil {
		return AgentCard{}, fmt.Errorf("a2a: build card request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.cacheError(agentURL, err)
		return AgentCard{}, fmt.Errorf("a2a: fetch card from %s: %w", cardURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		err := fmt.Errorf("a2a: card fetch returned %d: %s", resp.StatusCode, string(body))
		c.cacheError(agentURL, err)
		return AgentCard{}, err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return AgentCard{}, fmt.Errorf("a2a: read card body: %w", err)
	}

	var card AgentCard
	if err := json.Unmarshal(body, &card); err != nil {
		return AgentCard{}, fmt.Errorf("a2a: decode card: %w", err)
	}

	c.mu.Lock()
	c.cardCache[agentURL] = &cachedCard{card: card, fetchedAt: time.Now()}
	c.mu.Unlock()

	return card, nil
}

func (c *Client) cacheError(url string, err error) {
	c.mu.Lock()
	c.cardCache[url] = &cachedCard{err: err, fetchedAt: time.Now()}
	c.mu.Unlock()
}

// SendTask sends a task to a remote A2A agent via JSON-RPC.
func (c *Client) SendTask(ctx context.Context, agentURL string, params TaskSendParams, extraHeaders map[string]string) (TaskResult, error) {
	return c.callMethod(ctx, agentURL, MethodTaskSend, params, extraHeaders)
}

// GetTask retrieves a task status from a remote A2A agent.
func (c *Client) GetTask(ctx context.Context, agentURL string, params TaskGetParams, extraHeaders map[string]string) (TaskResult, error) {
	return c.callMethod(ctx, agentURL, MethodTaskGet, params, extraHeaders)
}

// CancelTask cancels a task on a remote A2A agent.
func (c *Client) CancelTask(ctx context.Context, agentURL string, params TaskCancelParams, extraHeaders map[string]string) (TaskResult, error) {
	return c.callMethod(ctx, agentURL, MethodTaskCancel, params, extraHeaders)
}

func (c *Client) callMethod(ctx context.Context, agentURL string, method string, params any, extraHeaders map[string]string) (TaskResult, error) {
	start := time.Now()
	outStatus := "ok"
	defer func() {
		telemetry.RecordA2AOutbound(agentURL, outStatus, time.Since(start).Seconds())
	}()

	if !c.skipURLValidation {
		if err := agentruntime.ValidateEndpointURL(agentURL, c.allowPrivate); err != nil {
			outStatus = "error"
			return TaskResult{}, fmt.Errorf("a2a: unsafe URL: %w", err)
		}
	}

	rpcReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(rpcReq)
	if err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, agentURL, bytes.NewReader(body))
	if err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: HTTP %d: %s", resp.StatusCode, truncateBody(respBody, 256))
	}

	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: decode response: %w", err)
	}

	if rpcResp.Error != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: remote error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	resultBytes, err := json.Marshal(rpcResp.Result)
	if err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: re-marshal result: %w", err)
	}

	var result TaskResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: decode result: %w", err)
	}

	return result, nil
}

// resolveCardURL derives the well-known card URL from an A2A agent URL.
func resolveCardURL(agentURL string) string {
	agentURL = strings.TrimSuffix(agentURL, "/")
	if strings.HasSuffix(agentURL, "/a2a") {
		base := strings.TrimSuffix(agentURL, "/a2a")
		return base + "/.well-known/agent-card.json"
	}
	if strings.HasSuffix(agentURL, "/.well-known/agent-card.json") || strings.HasSuffix(agentURL, "/.well-known/agent.json") {
		return agentURL
	}
	return agentURL + "/.well-known/agent-card.json"
}

func truncateBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}

// CacheStatus returns info about a cached card entry for observability.
func (c *Client) CacheStatus(agentURL string) (lastRefreshed time.Time, hasError bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if cached, ok := c.cardCache[agentURL]; ok {
		return cached.fetchedAt, cached.err != nil
	}
	return time.Time{}, false
}
