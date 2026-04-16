package agentruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestOpenAIEmbeddingProviderConcurrentDimensionsAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"index":     0,
					"embedding": []float32{0.1, 0.2, 0.3},
				},
			},
		})
	}))
	defer server.Close()

	provider := &OpenAIEmbeddingProvider{
		baseURL: server.URL,
		model:   "text-embedding-3-small",
		client:  server.Client(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, 32)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				if _, err := provider.Embed(ctx, []string{"hello"}); err != nil {
					errCh <- err
					return
				}
				if got := provider.Dimensions(); got != 0 && got != 3 {
					errCh <- context.Canceled
					t.Errorf("unexpected dimensions value %d", got)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent embedding call failed: %v", err)
		}
	}
	if got := provider.Dimensions(); got != 3 {
		t.Fatalf("expected cached dimensions=3, got %d", got)
	}
}
