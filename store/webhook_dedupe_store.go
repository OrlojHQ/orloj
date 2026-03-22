package store

import (
	"database/sql"
	"strings"
	"sync"
	"time"
)

type webhookDedupeItem struct {
	TaskName  string
	ExpiresAt time.Time
}

// WebhookDedupeStore tracks recently processed webhook events for idempotency.
type WebhookDedupeStore struct {
	mu    sync.RWMutex
	items map[string]webhookDedupeItem
	db    *sql.DB
}

func NewWebhookDedupeStore() *WebhookDedupeStore {
	return &WebhookDedupeStore{items: make(map[string]webhookDedupeItem)}
}

func NewWebhookDedupeStoreWithDB(db *sql.DB) *WebhookDedupeStore {
	return &WebhookDedupeStore{items: make(map[string]webhookDedupeItem), db: db}
}

func (s *WebhookDedupeStore) Put(endpointID, eventID, taskName string, expiresAt time.Time) error {
	endpointID = strings.TrimSpace(endpointID)
	eventID = strings.TrimSpace(eventID)
	taskName = strings.TrimSpace(taskName)
	if endpointID == "" || eventID == "" || taskName == "" {
		return nil
	}
	if s.db != nil {
		if err := pruneWebhookDedupeSQL(s.db, time.Now().UTC()); err != nil {
			return err
		}
		return upsertWebhookDedupeSQL(s.db, endpointID, eventID, taskName, expiresAt)
	}

	now := time.Now().UTC()
	s.mu.Lock()
	for key, item := range s.items {
		if !item.ExpiresAt.After(now) {
			delete(s.items, key)
		}
	}
	s.items[dedupeKey(endpointID, eventID)] = webhookDedupeItem{TaskName: taskName, ExpiresAt: expiresAt.UTC()}
	s.mu.Unlock()
	return nil
}

func (s *WebhookDedupeStore) Get(endpointID, eventID string, now time.Time) (string, bool, error) {
	endpointID = strings.TrimSpace(endpointID)
	eventID = strings.TrimSpace(eventID)
	if endpointID == "" || eventID == "" {
		return "", false, nil
	}
	if s.db != nil {
		if err := pruneWebhookDedupeSQL(s.db, now.UTC()); err != nil {
			return "", false, err
		}
		return getWebhookDedupeSQL(s.db, endpointID, eventID, now.UTC())
	}

	s.mu.Lock()
	for key, item := range s.items {
		if !item.ExpiresAt.After(now.UTC()) {
			delete(s.items, key)
		}
	}
	item, ok := s.items[dedupeKey(endpointID, eventID)]
	s.mu.Unlock()
	if !ok {
		return "", false, nil
	}
	return item.TaskName, true, nil
}

// TryInsert atomically checks for a live duplicate and inserts a new entry
// if none exists. Returns (existingTask, true) if a live duplicate was found.
// For in-memory mode, Get+Put is combined under the write lock.
func (s *WebhookDedupeStore) TryInsert(endpointID, eventID, taskName string, expiresAt, now time.Time) (string, bool, error) {
	endpointID = strings.TrimSpace(endpointID)
	eventID = strings.TrimSpace(eventID)
	taskName = strings.TrimSpace(taskName)
	if endpointID == "" || eventID == "" || taskName == "" {
		return "", false, nil
	}
	if s.db != nil {
		_ = pruneWebhookDedupeSQL(s.db, now.UTC())
		return tryInsertWebhookDedupeSQL(s.db, endpointID, eventID, taskName, expiresAt, now)
	}

	nowUTC := now.UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, item := range s.items {
		if !item.ExpiresAt.After(nowUTC) {
			delete(s.items, key)
		}
	}
	dk := dedupeKey(endpointID, eventID)
	if existing, ok := s.items[dk]; ok {
		return existing.TaskName, true, nil
	}
	s.items[dk] = webhookDedupeItem{TaskName: taskName, ExpiresAt: expiresAt.UTC()}
	return "", false, nil
}

func (s *WebhookDedupeStore) PruneExpired(now time.Time) error {
	if s.db != nil {
		return pruneWebhookDedupeSQL(s.db, now.UTC())
	}
	s.mu.Lock()
	for key, item := range s.items {
		if !item.ExpiresAt.After(now.UTC()) {
			delete(s.items, key)
		}
	}
	s.mu.Unlock()
	return nil
}

func dedupeKey(endpointID, eventID string) string {
	return strings.TrimSpace(endpointID) + "\x00" + strings.TrimSpace(eventID)
}
