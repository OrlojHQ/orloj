package agentruntime

import (
	"context"
	"strings"
	"testing"
)

func TestSharedMemorySearch_Deterministic(t *testing.T) {
	store := NewSharedMemoryStore()
	for i := 0; i < 20; i++ {
		key := strings.Repeat("x", i+1)
		store.Put("match-"+key, "value")
	}

	var firstResult []memoryEntry
	for run := 0; run < 10; run++ {
		results := store.Search("match", 5)
		if run == 0 {
			firstResult = results
			continue
		}
		if len(results) != len(firstResult) {
			t.Fatalf("run %d: got %d results, expected %d", run, len(results), len(firstResult))
		}
		for i := range results {
			if results[i].Key != firstResult[i].Key {
				t.Fatalf("run %d: result[%d] = %q, expected %q", run, i, results[i].Key, firstResult[i].Key)
			}
		}
	}
}

func TestMemoryIngest_ContentSizeLimit(t *testing.T) {
	store := NewSharedMemoryStore()
	rt := &MemoryToolRuntime{memory: store}

	largeContent := strings.Repeat("x", 11*1024*1024)
	input := `{"source": "test", "content": "` + largeContent + `"}`
	_, err := rt.handleIngest(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for oversized content")
	}
	if !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("expected byte limit error, got: %v", err)
	}
}
