package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPMemoryBackend implements PersistentMemoryBackend by delegating to an
// external HTTP service that speaks the Orloj memory provider contract.
// See docs/pages/concepts/memory.md for the contract specification.
type HTTPMemoryBackend struct {
	endpoint  string
	authToken string
	client    *http.Client
}

func NewHTTPMemoryBackend(endpoint, authToken string) *HTTPMemoryBackend {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	return &HTTPMemoryBackend{
		endpoint:  endpoint,
		authToken: strings.TrimSpace(authToken),
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (b *HTTPMemoryBackend) Put(ctx context.Context, key, value string) error {
	body, _ := json.Marshal(map[string]string{"key": key, "value": value})
	_, err := b.post(ctx, "/put", body)
	return err
}

func (b *HTTPMemoryBackend) Get(ctx context.Context, key string) (string, bool, error) {
	body, _ := json.Marshal(map[string]string{"key": key})
	resp, err := b.post(ctx, "/get", body)
	if err != nil {
		return "", false, err
	}
	var result struct {
		Found bool   `json:"found"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", false, fmt.Errorf("memory http get: invalid response: %w", err)
	}
	return result.Value, result.Found, nil
}

func (b *HTTPMemoryBackend) Search(ctx context.Context, query string, topK int) ([]MemorySearchResult, error) {
	if topK <= 0 {
		topK = 10
	}
	body, _ := json.Marshal(map[string]any{"query": query, "top_k": topK})
	resp, err := b.post(ctx, "/search", body)
	if err != nil {
		return nil, err
	}
	var result struct {
		Results []MemorySearchResult `json:"results"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("memory http search: invalid response: %w", err)
	}
	return result.Results, nil
}

func (b *HTTPMemoryBackend) List(ctx context.Context, prefix string) ([]MemorySearchResult, error) {
	body, _ := json.Marshal(map[string]string{"prefix": prefix})
	resp, err := b.post(ctx, "/list", body)
	if err != nil {
		return nil, err
	}
	var result struct {
		Entries []MemorySearchResult `json:"entries"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("memory http list: invalid response: %w", err)
	}
	return result.Entries, nil
}

func (b *HTTPMemoryBackend) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.endpoint+"/ping", nil)
	if err != nil {
		return fmt.Errorf("memory http ping: %w", err)
	}
	b.setHeaders(req)
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("memory http ping: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("memory http ping: status %d", resp.StatusCode)
	}
	return nil
}

func (b *HTTPMemoryBackend) post(ctx context.Context, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("memory http %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	b.setHeaders(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("memory http %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("memory http %s: read body: %w", path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(respBody))
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			msg = errResp.Error
		}
		return nil, fmt.Errorf("memory http %s: status %d: %s", path, resp.StatusCode, msg)
	}
	return respBody, nil
}

func (b *HTTPMemoryBackend) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}
}
