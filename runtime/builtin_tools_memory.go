package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const (
	ToolMemoryRead   = "memory.read"
	ToolMemoryWrite  = "memory.write"
	ToolMemorySearch = "memory.search"
	ToolMemoryList   = "memory.list"
	ToolMemoryIngest = "memory.ingest"
)

var builtinMemoryTools = map[string]struct{}{
	ToolMemoryRead:   {},
	ToolMemoryWrite:  {},
	ToolMemorySearch: {},
	ToolMemoryList:   {},
	ToolMemoryIngest: {},
}

// IsBuiltinMemoryTool returns true if the tool name is a built-in memory tool.
func IsBuiltinMemoryTool(name string) bool {
	_, ok := builtinMemoryTools[strings.TrimSpace(name)]
	return ok
}

// BuiltinMemoryToolNames returns the sorted list of built-in memory tool names.
func BuiltinMemoryToolNames() []string {
	names := make([]string, 0, len(builtinMemoryTools))
	for name := range builtinMemoryTools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SharedMemoryStore is a thread-safe key-value store shared across agents in a task.
type SharedMemoryStore struct {
	mu    sync.RWMutex
	store map[string]string
}

func NewSharedMemoryStore() *SharedMemoryStore {
	return &SharedMemoryStore{store: make(map[string]string)}
}

func (s *SharedMemoryStore) Put(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[key] = value
}

func (s *SharedMemoryStore) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.store[key]
	return v, ok
}

func (s *SharedMemoryStore) Snapshot() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.store))
	for k, v := range s.store {
		out[k] = v
	}
	return out
}

func (s *SharedMemoryStore) Search(query string, topK int) []memoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	query = strings.ToLower(query)
	if topK <= 0 {
		topK = 10
	}
	var results []memoryEntry
	for k, v := range s.store {
		if strings.Contains(strings.ToLower(k), query) || strings.Contains(strings.ToLower(v), query) {
			results = append(results, memoryEntry{Key: k, Value: v})
		}
	}
	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

func (s *SharedMemoryStore) List(prefix string) []memoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var results []memoryEntry
	for k, v := range s.store {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			results = append(results, memoryEntry{Key: k, Value: v})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Key < results[j].Key })
	return results
}

type memoryEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MemoryToolRuntime wraps a ToolRuntime and intercepts built-in memory tool calls.
// When a persistent backend is set, it takes priority over the ephemeral shared store.
type MemoryToolRuntime struct {
	delegate   ToolRuntime
	memory     *SharedMemoryStore
	persistent PersistentMemoryBackend
}

func NewMemoryToolRuntime(delegate ToolRuntime, memory *SharedMemoryStore) *MemoryToolRuntime {
	if memory == nil {
		memory = NewSharedMemoryStore()
	}
	return &MemoryToolRuntime{delegate: delegate, memory: memory}
}

// WithPersistentBackend returns a copy that delegates to the persistent backend.
func (r *MemoryToolRuntime) WithPersistentBackend(backend PersistentMemoryBackend) *MemoryToolRuntime {
	return &MemoryToolRuntime{
		delegate:   r.delegate,
		memory:     r.memory,
		persistent: backend,
	}
}

func (r *MemoryToolRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
	tool = strings.TrimSpace(tool)
	if !IsBuiltinMemoryTool(tool) {
		if r.delegate != nil {
			return r.delegate.Call(ctx, tool, input)
		}
		return "", fmt.Errorf("unsupported tool: %s", tool)
	}
	switch tool {
	case ToolMemoryWrite:
		return r.handleWrite(ctx, input)
	case ToolMemoryRead:
		return r.handleRead(ctx, input)
	case ToolMemorySearch:
		return r.handleSearch(ctx, input)
	case ToolMemoryList:
		return r.handleList(ctx, input)
	case ToolMemoryIngest:
		return r.handleIngest(ctx, input)
	default:
		return "", fmt.Errorf("unknown memory tool: %s", tool)
	}
}

func (r *MemoryToolRuntime) handleWrite(ctx context.Context, input string) (string, error) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", fmt.Errorf("memory.write: invalid input: %w", err)
	}
	key := strings.TrimSpace(req.Key)
	if key == "" {
		return "", fmt.Errorf("memory.write: key is required")
	}
	if r.persistent != nil {
		if err := r.persistent.Put(ctx, key, req.Value); err != nil {
			return "", fmt.Errorf("memory.write: %w", err)
		}
	} else {
		r.memory.Put(key, req.Value)
	}
	resp, _ := json.Marshal(map[string]string{"status": "ok", "key": key})
	return string(resp), nil
}

func (r *MemoryToolRuntime) handleRead(ctx context.Context, input string) (string, error) {
	var req struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", fmt.Errorf("memory.read: invalid input: %w", err)
	}
	key := strings.TrimSpace(req.Key)
	if key == "" {
		return "", fmt.Errorf("memory.read: key is required")
	}
	var value string
	var ok bool
	if r.persistent != nil {
		var err error
		value, ok, err = r.persistent.Get(ctx, key)
		if err != nil {
			return "", fmt.Errorf("memory.read: %w", err)
		}
	} else {
		value, ok = r.memory.Get(key)
	}
	if !ok {
		resp, _ := json.Marshal(map[string]any{"found": false, "key": key})
		return string(resp), nil
	}
	resp, _ := json.Marshal(map[string]any{"found": true, "key": key, "value": value})
	return string(resp), nil
}

func (r *MemoryToolRuntime) handleSearch(ctx context.Context, input string) (string, error) {
	var req struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", fmt.Errorf("memory.search: invalid input: %w", err)
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return "", fmt.Errorf("memory.search: query is required")
	}
	if r.persistent != nil {
		results, err := r.persistent.Search(ctx, query, req.TopK)
		if err != nil {
			return "", fmt.Errorf("memory.search: %w", err)
		}
		resp, _ := json.Marshal(map[string]any{"results": results, "count": len(results)})
		return string(resp), nil
	}
	results := r.memory.Search(query, req.TopK)
	resp, _ := json.Marshal(map[string]any{"results": results, "count": len(results)})
	return string(resp), nil
}

func (r *MemoryToolRuntime) handleList(ctx context.Context, input string) (string, error) {
	var req struct {
		Prefix string `json:"prefix"`
	}
	if input != "" {
		_ = json.Unmarshal([]byte(input), &req)
	}
	if r.persistent != nil {
		results, err := r.persistent.List(ctx, req.Prefix)
		if err != nil {
			return "", fmt.Errorf("memory.list: %w", err)
		}
		resp, _ := json.Marshal(map[string]any{"entries": results, "count": len(results)})
		return string(resp), nil
	}
	results := r.memory.List(req.Prefix)
	resp, _ := json.Marshal(map[string]any{"entries": results, "count": len(results)})
	return string(resp), nil
}

func (r *MemoryToolRuntime) handleIngest(ctx context.Context, input string) (string, error) {
	var req struct {
		Source    string `json:"source"`
		Content   string `json:"content"`
		ChunkSize int    `json:"chunk_size"`
		Overlap   int    `json:"overlap"`
	}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", fmt.Errorf("memory.ingest: invalid input: %w", err)
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		return "", fmt.Errorf("memory.ingest: source is required")
	}
	if strings.TrimSpace(req.Content) == "" {
		return "", fmt.Errorf("memory.ingest: content is required")
	}

	chunks := ChunkText(req.Content, req.ChunkSize, req.Overlap)

	for _, chunk := range chunks {
		key := fmt.Sprintf("%s/chunk-%04d", source, chunk.Index)
		if r.persistent != nil {
			if err := r.persistent.Put(ctx, key, chunk.Text); err != nil {
				return "", fmt.Errorf("memory.ingest: failed writing chunk %d: %w", chunk.Index, err)
			}
		} else {
			r.memory.Put(key, chunk.Text)
		}
	}

	resp, _ := json.Marshal(map[string]any{
		"status":        "ok",
		"source":        source,
		"chunks_stored": len(chunks),
	})
	return string(resp), nil
}
