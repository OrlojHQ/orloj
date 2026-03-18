package agentruntime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// PersistentMemoryBackend defines the interface for durable memory stores
// that persist across task runs. Implementations are selected based on the
// Memory CRD's spec.type and spec.provider fields.
type PersistentMemoryBackend interface {
	Put(ctx context.Context, key, value string) error
	Get(ctx context.Context, key string) (string, bool, error)
	Search(ctx context.Context, query string, topK int) ([]MemorySearchResult, error)
	List(ctx context.Context, prefix string) ([]MemorySearchResult, error)
	Ping(ctx context.Context) error
}

// MemorySearchResult is one entry returned by search or list.
type MemorySearchResult struct {
	Key   string  `json:"key"`
	Value string  `json:"value"`
	Score float64 `json:"score,omitempty"`
}

// InMemoryBackend is a PersistentMemoryBackend backed by an in-process map.
// Useful for testing and single-instance deployments.
type InMemoryBackend struct {
	mu    sync.RWMutex
	store map[string]string
}

func NewInMemoryBackend() *InMemoryBackend {
	return &InMemoryBackend{store: make(map[string]string)}
}

func (b *InMemoryBackend) Put(_ context.Context, key, value string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.store[key] = value
	return nil
}

func (b *InMemoryBackend) Get(_ context.Context, key string) (string, bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.store[key]
	return v, ok, nil
}

func (b *InMemoryBackend) Search(_ context.Context, query string, topK int) ([]MemorySearchResult, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if topK <= 0 {
		topK = 10
	}
	query = strings.ToLower(query)
	var results []MemorySearchResult
	for k, v := range b.store {
		if strings.Contains(strings.ToLower(k), query) || strings.Contains(strings.ToLower(v), query) {
			results = append(results, MemorySearchResult{Key: k, Value: v, Score: 1.0})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Key < results[j].Key })
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

func (b *InMemoryBackend) List(_ context.Context, prefix string) ([]MemorySearchResult, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var results []MemorySearchResult
	for k, v := range b.store {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			results = append(results, MemorySearchResult{Key: k, Value: v})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Key < results[j].Key })
	return results, nil
}

func (b *InMemoryBackend) Ping(_ context.Context) error {
	return nil
}

// MemoryProviderConfig holds the fields from a Memory CRD spec that a
// provider factory needs to construct a backend.
type MemoryProviderConfig struct {
	Type           string
	Provider       string
	EmbeddingModel string
	Endpoint       string
	AuthToken      string
	Options        map[string]string
	Embedder       EmbeddingProvider
}

// MemoryProviderFactory creates a PersistentMemoryBackend from CRD config.
// Implementations should validate their own config and return a descriptive
// error if required fields are missing.
type MemoryProviderFactory func(cfg MemoryProviderConfig) (PersistentMemoryBackend, error)

// MemoryProviderRegistry is a global registry of provider factories keyed by
// provider name (e.g. "pgvector", "qdrant", "weaviate"). Thread-safe.
type MemoryProviderRegistry struct {
	mu        sync.RWMutex
	factories map[string]MemoryProviderFactory
}

var defaultProviderRegistry = &MemoryProviderRegistry{
	factories: make(map[string]MemoryProviderFactory),
}

func init() {
	defaultProviderRegistry.Register("in-memory", func(_ MemoryProviderConfig) (PersistentMemoryBackend, error) {
		return NewInMemoryBackend(), nil
	})
	defaultProviderRegistry.Register("memory", func(_ MemoryProviderConfig) (PersistentMemoryBackend, error) {
		return NewInMemoryBackend(), nil
	})
	defaultProviderRegistry.Register("http", func(cfg MemoryProviderConfig) (PersistentMemoryBackend, error) {
		if strings.TrimSpace(cfg.Endpoint) == "" {
			return nil, fmt.Errorf("http memory provider requires spec.endpoint")
		}
		return NewHTTPMemoryBackend(cfg.Endpoint, cfg.AuthToken), nil
	})
	defaultProviderRegistry.Register("pgvector", func(cfg MemoryProviderConfig) (PersistentMemoryBackend, error) {
		if strings.TrimSpace(cfg.Endpoint) == "" {
			return nil, fmt.Errorf("pgvector memory provider requires spec.endpoint (postgres connection string)")
		}
		if cfg.Embedder == nil {
			return nil, fmt.Errorf("pgvector memory provider requires spec.embedding_model (reference to a ModelEndpoint)")
		}
		opts := PgvectorOptions{}
		if cfg.Options != nil {
			opts.Table = cfg.Options["table"]
		}
		dsn := cfg.Endpoint
		if strings.TrimSpace(cfg.AuthToken) != "" {
			dsn = injectPgPassword(dsn, cfg.AuthToken)
		}
		return NewPgvectorBackend(dsn, cfg.Embedder, opts)
	})
}

// DefaultMemoryProviderRegistry returns the global provider registry.
// Use this to register custom vector database providers at startup.
func DefaultMemoryProviderRegistry() *MemoryProviderRegistry {
	return defaultProviderRegistry
}

// Register adds a provider factory under the given name. Names are
// case-insensitive and trimmed. Registering the same name twice
// replaces the previous factory.
func (r *MemoryProviderRegistry) Register(name string, factory MemoryProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[strings.ToLower(strings.TrimSpace(name))] = factory
}

// Create looks up the factory for the given provider name and calls it.
// An empty provider name falls back to "in-memory".
func (r *MemoryProviderRegistry) Create(cfg MemoryProviderConfig) (PersistentMemoryBackend, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = "in-memory"
	}
	r.mu.RLock()
	factory, ok := r.factories[provider]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported memory provider %q (register it with DefaultMemoryProviderRegistry().Register)", provider)
	}
	return factory(cfg)
}

// Providers returns the sorted list of registered provider names.
func (r *MemoryProviderRegistry) Providers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// PersistentMemoryBackendRegistry manages named persistent backends keyed by Memory CRD name.
type PersistentMemoryBackendRegistry struct {
	mu       sync.RWMutex
	backends map[string]PersistentMemoryBackend
}

func NewPersistentMemoryBackendRegistry() *PersistentMemoryBackendRegistry {
	return &PersistentMemoryBackendRegistry{backends: make(map[string]PersistentMemoryBackend)}
}

func (r *PersistentMemoryBackendRegistry) Register(name string, backend PersistentMemoryBackend) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backends[strings.TrimSpace(name)] = backend
}

func (r *PersistentMemoryBackendRegistry) Get(name string) (PersistentMemoryBackend, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.backends[strings.TrimSpace(name)]
	return b, ok
}

// NewPersistentMemoryBackendFromConfig creates a backend using the global
// provider registry. This is the primary entry point used by the controller.
func NewPersistentMemoryBackendFromConfig(memType, provider, embeddingModel string) (PersistentMemoryBackend, error) {
	return defaultProviderRegistry.Create(MemoryProviderConfig{
		Type:           memType,
		Provider:       provider,
		EmbeddingModel: embeddingModel,
	})
}

// injectPgPassword inserts a password into a postgres:// DSN when the DSN
// does not already contain one.
func injectPgPassword(dsn, password string) string {
	if strings.Contains(dsn, "password=") {
		return dsn
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		atIdx := strings.Index(dsn, "@")
		schemeEnd := strings.Index(dsn, "://") + 3
		if atIdx > schemeEnd {
			userPart := dsn[schemeEnd:atIdx]
			if !strings.Contains(userPart, ":") {
				return dsn[:schemeEnd] + userPart + ":" + password + dsn[atIdx:]
			}
		}
	}
	return dsn
}
