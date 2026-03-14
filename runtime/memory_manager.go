package agentruntime

import "sync"

// MemoryManager stores short-lived runtime memory for an agent worker.
type MemoryManager struct {
	mu    sync.RWMutex
	store map[string]string
}

func NewMemoryManager() *MemoryManager {
	return &MemoryManager{store: make(map[string]string)}
}

func (m *MemoryManager) Put(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[key] = value
}

func (m *MemoryManager) Get(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.store[key]
	return v, ok
}

func (m *MemoryManager) Snapshot() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]string, len(m.store))
	for k, v := range m.store {
		out[k] = v
	}
	return out
}
