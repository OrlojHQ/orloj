package a2a

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"
)

// RemoteAgentConfig defines a static remote agent entry from Helm/server config.
type RemoteAgentConfig struct {
	Name      string `json:"name" yaml:"name"`
	URL       string `json:"url" yaml:"url"`
	AuthType  string `json:"authType,omitempty" yaml:"authType,omitempty"`
	SecretRef string `json:"secretRef,omitempty" yaml:"secretRef,omitempty"`
}

// Registry maintains a cache of remote A2A agent cards.
type Registry struct {
	client   *Client
	configs  []RemoteAgentConfig
	entries  map[string]*RemoteAgentEntry
	mu       sync.RWMutex
	logger   *log.Logger
	ttl      time.Duration
	stopCh   chan struct{}
	startOne sync.Once
	stopOne  sync.Once
}

// NewRegistry creates a registry backed by static config.
func NewRegistry(client *Client, configs []RemoteAgentConfig, ttl time.Duration, logger *log.Logger) *Registry {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	r := &Registry{
		client:  client,
		configs: configs,
		entries: make(map[string]*RemoteAgentEntry, len(configs)),
		logger:  logger,
		ttl:     ttl,
		stopCh:  make(chan struct{}),
	}
	for _, cfg := range configs {
		r.entries[cfg.Name] = &RemoteAgentEntry{
			Name:     cfg.Name,
			URL:      cfg.URL,
			CacheTTL: ttl.String(),
		}
	}
	return r
}

// Start begins periodic card refresh in the background. Safe to call multiple times.
func (r *Registry) Start(ctx context.Context) {
	r.startOne.Do(func() {
		go r.refreshLoop(ctx)
	})
}

// Stop signals the refresh loop to terminate. Safe to call multiple times.
func (r *Registry) Stop() {
	r.stopOne.Do(func() {
		close(r.stopCh)
	})
}

func (r *Registry) refreshLoop(ctx context.Context) {
	r.refreshAll(ctx)
	ticker := time.NewTicker(r.ttl)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.refreshAll(ctx)
		}
	}
}

func (r *Registry) refreshAll(ctx context.Context) {
	for _, cfg := range r.configs {
		card, err := r.client.FetchCard(ctx, cfg.URL, nil)
		r.mu.Lock()
		entry := r.entries[cfg.Name]
		if entry == nil {
			entry = &RemoteAgentEntry{Name: cfg.Name, URL: cfg.URL, CacheTTL: r.ttl.String()}
			r.entries[cfg.Name] = entry
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err != nil {
			entry.CacheStatus = "error"
			entry.Error = err.Error()
			entry.LastRefreshed = now
			if r.logger != nil {
				r.logger.Printf("a2a registry: refresh failed for %s: %v", cfg.Name, err)
			}
		} else {
			entry.Card = &card
			entry.CacheStatus = "ok"
			entry.Error = ""
			entry.LastRefreshed = now
			entry.ProtocolVersion = card.ProtocolVersion
		}
		r.mu.Unlock()
	}
}

// List returns all registered remote agents with their cache status, sorted by name.
func (r *Registry) List() []RemoteAgentEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RemoteAgentEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		cp := *entry
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns a specific remote agent entry by name.
func (r *Registry) Get(name string) (RemoteAgentEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.entries[name]
	if !ok {
		return RemoteAgentEntry{}, false
	}
	return *entry, true
}
