package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// EmbeddingProvider generates vector embeddings from text. Implementations
// are used by vector-database memory backends (e.g. pgvector) to embed
// values on write and queries on search.
type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

// OpenAIEmbeddingProvider calls an OpenAI-compatible /embeddings endpoint.
// Works with OpenAI, Azure OpenAI, Ollama, and any compatible API.
type OpenAIEmbeddingProvider struct {
	baseURL    string
	apiKey     string
	model      string
	dimensions atomic.Int64
	client     *http.Client
}

func NewOpenAIEmbeddingProvider(baseURL, apiKey, model string) *OpenAIEmbeddingProvider {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	// Embedding providers may legitimately point at Ollama / LM Studio /
	// vLLM on a private network, so allow private addresses. Loopback,
	// link-local, metadata, and unspecified addresses remain blocked.
	return &OpenAIEmbeddingProvider{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(apiKey),
		model:   strings.TrimSpace(model),
		client:  SafeHTTPClient(true, 60*time.Second),
	}
}

func (p *OpenAIEmbeddingProvider) Dimensions() int {
	if p == nil {
		return 0
	}
	return int(p.dimensions.Load())
}

type openAIEmbeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type openAIEmbeddingResponse struct {
	Data  []openAIEmbeddingData `json:"data"`
	Error *openAIEmbeddingError `json:"error,omitempty"`
}

type openAIEmbeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type openAIEmbeddingError struct {
	Message string `json:"message"`
}

func (p *OpenAIEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body := openAIEmbeddingRequest{
		Input: texts,
		Model: p.model,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("embedding: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("embedding: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("embedding: read response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("embedding: request failed status=%d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed openAIEmbeddingResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("embedding: decode response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("embedding: provider error: %s", parsed.Error.Message)
	}
	if len(parsed.Data) != len(texts) {
		return nil, fmt.Errorf("embedding: expected %d embeddings, got %d", len(texts), len(parsed.Data))
	}

	result := make([][]float32, len(texts))
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("embedding: out-of-range index %d", d.Index)
		}
		result[d.Index] = d.Embedding
	}

	if len(result) > 0 && len(result[0]) > 0 {
		p.dimensions.CompareAndSwap(0, int64(len(result[0])))
	}

	return result, nil
}
