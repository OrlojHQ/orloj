package store

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

// cursorFilter applies keyset cursor pagination to a pre-sorted slice.
// Items are assumed sorted by name ASC already. It filters by namespace
// (when non-empty) and returns at most limit items with name > after.
func cursorFilter[T any](items []T, getName func(T) string, getNamespace func(T) string, limit int, after, namespace string) []T {
	if limit <= 0 {
		limit = defaultListLimit
	}
	result := make([]T, 0)
	for _, item := range items {
		if after != "" && getName(item) <= after {
			continue
		}
		if namespace != "" && !strings.EqualFold(getNamespace(item), namespace) {
			continue
		}
		result = append(result, item)
		if len(result) >= limit {
			break
		}
	}
	return result
}

type AgentSystemStore struct {
	mu    sync.RWMutex
	items map[string]resources.AgentSystem
	db    *sql.DB
}

func NewAgentSystemStore() *AgentSystemStore {
	return &AgentSystemStore{items: make(map[string]resources.AgentSystem)}
}

func NewAgentSystemStoreWithDB(db *sql.DB) *AgentSystemStore {
	return &AgentSystemStore{items: make(map[string]resources.AgentSystem), db: db}
}

func (s *AgentSystemStore) Upsert(ctx context.Context, item resources.AgentSystem) (resources.AgentSystem, error) {
	if err := item.Normalize(); err != nil {
		return resources.AgentSystem{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.AgentSystem{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.AgentSystem](ctx, tx, tableAgentSystems, key)
		if err != nil {
			return resources.AgentSystem{}, err
		}
		if !found {
			if err := initializeCreateMetadata("AgentSystem", &item.Metadata); err != nil {
				return resources.AgentSystem{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("AgentSystem", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.AgentSystem{}, err
			}
		}
		if err := upsertAgentSystemSQL(ctx, tx, key, item); err != nil {
			return resources.AgentSystem{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.AgentSystem{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("AgentSystem", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.AgentSystem{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("AgentSystem", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.AgentSystem{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *AgentSystemStore) Get(ctx context.Context, name string) (resources.AgentSystem, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.AgentSystem](ctx, s.db, tableAgentSystems, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *AgentSystemStore) List(ctx context.Context) ([]resources.AgentSystem, error) {
	if s.db != nil {
		return listFromTable[resources.AgentSystem](ctx, s.db, tableAgentSystems)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.AgentSystem, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *AgentSystemStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.AgentSystem, error) {
	if s.db != nil {
		return listFromTableCursor[resources.AgentSystem](ctx, s.db, tableAgentSystems, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.AgentSystem, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.AgentSystem) string { return a.Metadata.Name },
		func(a resources.AgentSystem) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *AgentSystemStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableAgentSystems, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("agentsystem %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("agentsystem %q not found", name)
	}
	delete(s.items, key)
	return nil
}

type ModelEndpointStore struct {
	mu    sync.RWMutex
	items map[string]resources.ModelEndpoint
	db    *sql.DB
}

func NewModelEndpointStore() *ModelEndpointStore {
	return &ModelEndpointStore{items: make(map[string]resources.ModelEndpoint)}
}

func NewModelEndpointStoreWithDB(db *sql.DB) *ModelEndpointStore {
	return &ModelEndpointStore{items: make(map[string]resources.ModelEndpoint), db: db}
}

func (s *ModelEndpointStore) Upsert(ctx context.Context, item resources.ModelEndpoint) (resources.ModelEndpoint, error) {
	if err := item.Normalize(); err != nil {
		return resources.ModelEndpoint{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.ModelEndpoint{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.ModelEndpoint](ctx, tx, tableModelEndpoints, key)
		if err != nil {
			return resources.ModelEndpoint{}, err
		}
		if !found {
			if err := initializeCreateMetadata("ModelEndpoint", &item.Metadata); err != nil {
				return resources.ModelEndpoint{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("ModelEndpoint", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.ModelEndpoint{}, err
			}
		}
		if err := upsertModelEndpointSQL(ctx, tx, key, item); err != nil {
			return resources.ModelEndpoint{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.ModelEndpoint{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("ModelEndpoint", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.ModelEndpoint{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("ModelEndpoint", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.ModelEndpoint{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *ModelEndpointStore) Get(ctx context.Context, name string) (resources.ModelEndpoint, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.ModelEndpoint](ctx, s.db, tableModelEndpoints, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *ModelEndpointStore) List(ctx context.Context) ([]resources.ModelEndpoint, error) {
	if s.db != nil {
		return listFromTable[resources.ModelEndpoint](ctx, s.db, tableModelEndpoints)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.ModelEndpoint, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *ModelEndpointStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.ModelEndpoint, error) {
	if s.db != nil {
		return listFromTableCursor[resources.ModelEndpoint](ctx, s.db, tableModelEndpoints, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.ModelEndpoint, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.ModelEndpoint) string { return a.Metadata.Name },
		func(a resources.ModelEndpoint) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *ModelEndpointStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableModelEndpoints, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("modelendpoint %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("modelendpoint %q not found", name)
	}
	delete(s.items, key)
	return nil
}

type ToolStore struct {
	mu    sync.RWMutex
	items map[string]resources.Tool
	db    *sql.DB
}

func NewToolStore() *ToolStore {
	return &ToolStore{items: make(map[string]resources.Tool)}
}

func NewToolStoreWithDB(db *sql.DB) *ToolStore {
	return &ToolStore{items: make(map[string]resources.Tool), db: db}
}

func (s *ToolStore) Upsert(ctx context.Context, item resources.Tool) (resources.Tool, error) {
	if err := item.Normalize(); err != nil {
		return resources.Tool{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.Tool{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.Tool](ctx, tx, tableTools, key)
		if err != nil {
			return resources.Tool{}, err
		}
		if !found {
			if err := initializeCreateMetadata("Tool", &item.Metadata); err != nil {
				return resources.Tool{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("Tool", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.Tool{}, err
			}
		}
		if err := upsertToolSQL(ctx, tx, key, item); err != nil {
			return resources.Tool{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.Tool{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("Tool", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.Tool{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("Tool", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.Tool{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *ToolStore) Get(ctx context.Context, name string) (resources.Tool, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.Tool](ctx, s.db, tableTools, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *ToolStore) List(ctx context.Context) ([]resources.Tool, error) {
	if s.db != nil {
		return listFromTable[resources.Tool](ctx, s.db, tableTools)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.Tool, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *ToolStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.Tool, error) {
	if s.db != nil {
		return listFromTableCursor[resources.Tool](ctx, s.db, tableTools, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.Tool, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.Tool) string { return a.Metadata.Name },
		func(a resources.Tool) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *ToolStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableTools, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("tool %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("tool %q not found", name)
	}
	delete(s.items, key)
	return nil
}

type SecretStore struct {
	mu                sync.RWMutex
	items             map[string]resources.Secret
	db                *sql.DB
	encryptionKey     []byte
	requireEncryption bool // if true, refuse to store secrets without a key
}

func NewSecretStore() *SecretStore {
	return &SecretStore{items: make(map[string]resources.Secret)}
}

func NewSecretStoreWithDB(db *sql.DB) *SecretStore {
	return &SecretStore{items: make(map[string]resources.Secret), db: db}
}

func NewSecretStoreWithEncryption(db *sql.DB, key []byte) *SecretStore {
	return &SecretStore{items: make(map[string]resources.Secret), db: db, encryptionKey: key}
}

func (s *SecretStore) SetEncryptionKey(key []byte)       { s.encryptionKey = key }
func (s *SecretStore) SetRequireEncryption(require bool) { s.requireEncryption = require }

func (s *SecretStore) Upsert(ctx context.Context, item resources.Secret) (resources.Secret, error) {
	if err := item.Normalize(); err != nil {
		return resources.Secret{}, err
	}
	if s.requireEncryption && len(s.encryptionKey) == 0 && len(item.Spec.Data) > 0 {
		return resources.Secret{}, fmt.Errorf("secret encryption is required but no encryption key is configured; set ORLOJ_SECRET_ENCRYPTION_KEY")
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.Secret{}, err
		}
		defer tx.Rollback()

		existing, found, err := s.getDecryptedFrom(ctx, tx, key)
		if err != nil {
			return resources.Secret{}, err
		}
		if !found {
			if err := initializeCreateMetadata("Secret", &item.Metadata); err != nil {
				return resources.Secret{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("Secret", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.Secret{}, err
			}
		}
		toStore := item
		if len(s.encryptionKey) > 0 && len(toStore.Spec.Data) > 0 {
			enc, err := encryptSecretData(s.encryptionKey, toStore.Spec.Data)
			if err != nil {
				return resources.Secret{}, err
			}
			toStore.Spec.Data = enc
		}
		if err := upsertSecretSQL(ctx, tx, key, toStore); err != nil {
			return resources.Secret{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.Secret{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("Secret", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.Secret{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("Secret", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.Secret{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *SecretStore) getDecrypted(ctx context.Context, key string) (resources.Secret, bool, error) {
	item, ok, err := getFromTable[resources.Secret](ctx, s.db, tableSecrets, key)
	if err != nil || !ok {
		return item, ok, err
	}
	if len(s.encryptionKey) > 0 && len(item.Spec.Data) > 0 {
		dec, err := decryptSecretData(s.encryptionKey, item.Spec.Data)
		if err != nil {
			return resources.Secret{}, false, err
		}
		item.Spec.Data = dec
	}
	return item, true, nil
}

// getDecryptedFrom reads a secret with FOR UPDATE within an existing transaction.
func (s *SecretStore) getDecryptedFrom(ctx context.Context, tx *sql.Tx, key string) (resources.Secret, bool, error) {
	item, ok, err := getFromTableForUpdate[resources.Secret](ctx, tx, tableSecrets, key)
	if err != nil || !ok {
		return item, ok, err
	}
	if len(s.encryptionKey) > 0 && len(item.Spec.Data) > 0 {
		dec, err := decryptSecretData(s.encryptionKey, item.Spec.Data)
		if err != nil {
			return resources.Secret{}, false, err
		}
		item.Spec.Data = dec
	}
	return item, true, nil
}

func (s *SecretStore) Get(ctx context.Context, name string) (resources.Secret, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return s.getDecrypted(ctx, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *SecretStore) List(ctx context.Context) ([]resources.Secret, error) {
	if s.db != nil {
		items, err := listFromTable[resources.Secret](ctx, s.db, tableSecrets)
		if err != nil {
			return nil, err
		}
		if len(s.encryptionKey) > 0 {
			for i := range items {
				if len(items[i].Spec.Data) > 0 {
					dec, err := decryptSecretData(s.encryptionKey, items[i].Spec.Data)
					if err != nil {
						return nil, err
					}
					items[i].Spec.Data = dec
				}
			}
		}
		return items, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.Secret, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *SecretStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.Secret, error) {
	if s.db != nil {
		items, err := listFromTableCursor[resources.Secret](ctx, s.db, tableSecrets, limit, after, namespace)
		if err != nil {
			return nil, err
		}
		if len(s.encryptionKey) > 0 {
			for i := range items {
				if len(items[i].Spec.Data) > 0 {
					dec, err := decryptSecretData(s.encryptionKey, items[i].Spec.Data)
					if err != nil {
						return nil, fmt.Errorf("decrypt secret %q: %w", items[i].Metadata.Name, err)
					}
					items[i].Spec.Data = dec
				}
			}
		}
		return items, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.Secret, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.Secret) string { return a.Metadata.Name },
		func(a resources.Secret) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

// ReEncryptAll re-encrypts all secrets with the current encryption key.
// This is used during key rotation: set the old key to decrypt, call
// SetEncryptionKey with the new key, then call ReEncryptAll.
func (s *SecretStore) ReEncryptAll(ctx context.Context, oldKey, newKey []byte) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("re-encryption requires a database backend")
	}
	if len(newKey) == 0 {
		return 0, fmt.Errorf("new encryption key is required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	items, err := listFromTable[resources.Secret](ctx, tx, tableSecrets)
	if err != nil {
		return 0, fmt.Errorf("list secrets: %w", err)
	}
	count := 0
	for _, item := range items {
		if len(item.Spec.Data) == 0 {
			continue
		}
		if len(oldKey) > 0 {
			dec, err := decryptSecretData(oldKey, item.Spec.Data)
			if err != nil {
				return 0, fmt.Errorf("decrypt secret %q: %w", item.Metadata.Name, err)
			}
			item.Spec.Data = dec
		}
		enc, err := encryptSecretData(newKey, item.Spec.Data)
		if err != nil {
			return 0, fmt.Errorf("re-encrypt secret %q: %w", item.Metadata.Name, err)
		}
		item.Spec.Data = enc
		key := scopedNameFromMeta(item.Metadata)
		if err := upsertSecretSQL(ctx, tx, key, item); err != nil {
			return 0, fmt.Errorf("store re-encrypted secret %q: %w", item.Metadata.Name, err)
		}
		count++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit re-encryption: %w", err)
	}
	return count, nil
}

func (s *SecretStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableSecrets, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("secret %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("secret %q not found", name)
	}
	delete(s.items, key)
	return nil
}

type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]resources.Memory
	db    *sql.DB
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: make(map[string]resources.Memory)}
}

func NewMemoryStoreWithDB(db *sql.DB) *MemoryStore {
	return &MemoryStore{items: make(map[string]resources.Memory), db: db}
}

func (s *MemoryStore) Upsert(ctx context.Context, item resources.Memory) (resources.Memory, error) {
	if err := item.Normalize(); err != nil {
		return resources.Memory{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.Memory{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.Memory](ctx, tx, tableMemories, key)
		if err != nil {
			return resources.Memory{}, err
		}
		if !found {
			if err := initializeCreateMetadata("Memory", &item.Metadata); err != nil {
				return resources.Memory{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("Memory", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.Memory{}, err
			}
		}
		if err := upsertMemorySQL(ctx, tx, key, item); err != nil {
			return resources.Memory{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.Memory{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("Memory", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.Memory{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("Memory", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.Memory{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *MemoryStore) Get(ctx context.Context, name string) (resources.Memory, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.Memory](ctx, s.db, tableMemories, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *MemoryStore) List(ctx context.Context) ([]resources.Memory, error) {
	if s.db != nil {
		return listFromTable[resources.Memory](ctx, s.db, tableMemories)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.Memory, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *MemoryStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.Memory, error) {
	if s.db != nil {
		return listFromTableCursor[resources.Memory](ctx, s.db, tableMemories, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.Memory, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.Memory) string { return a.Metadata.Name },
		func(a resources.Memory) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *MemoryStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableMemories, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("memory %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("memory %q not found", name)
	}
	delete(s.items, key)
	return nil
}

type AgentPolicyStore struct {
	mu    sync.RWMutex
	items map[string]resources.AgentPolicy
	db    *sql.DB
}

func NewAgentPolicyStore() *AgentPolicyStore {
	return &AgentPolicyStore{items: make(map[string]resources.AgentPolicy)}
}

func NewAgentPolicyStoreWithDB(db *sql.DB) *AgentPolicyStore {
	return &AgentPolicyStore{items: make(map[string]resources.AgentPolicy), db: db}
}

func (s *AgentPolicyStore) Upsert(ctx context.Context, item resources.AgentPolicy) (resources.AgentPolicy, error) {
	if err := item.Normalize(); err != nil {
		return resources.AgentPolicy{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.AgentPolicy{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.AgentPolicy](ctx, tx, tableAgentPolicies, key)
		if err != nil {
			return resources.AgentPolicy{}, err
		}
		if !found {
			if err := initializeCreateMetadata("AgentPolicy", &item.Metadata); err != nil {
				return resources.AgentPolicy{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("AgentPolicy", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.AgentPolicy{}, err
			}
		}
		if err := upsertAgentPolicySQL(ctx, tx, key, item); err != nil {
			return resources.AgentPolicy{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.AgentPolicy{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("AgentPolicy", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.AgentPolicy{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("AgentPolicy", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.AgentPolicy{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *AgentPolicyStore) Get(ctx context.Context, name string) (resources.AgentPolicy, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.AgentPolicy](ctx, s.db, tableAgentPolicies, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *AgentPolicyStore) List(ctx context.Context) ([]resources.AgentPolicy, error) {
	if s.db != nil {
		return listFromTable[resources.AgentPolicy](ctx, s.db, tableAgentPolicies)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.AgentPolicy, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *AgentPolicyStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.AgentPolicy, error) {
	if s.db != nil {
		return listFromTableCursor[resources.AgentPolicy](ctx, s.db, tableAgentPolicies, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.AgentPolicy, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.AgentPolicy) string { return a.Metadata.Name },
		func(a resources.AgentPolicy) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *AgentPolicyStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableAgentPolicies, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("agentpolicy %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("agentpolicy %q not found", name)
	}
	delete(s.items, key)
	return nil
}

type AgentRoleStore struct {
	mu    sync.RWMutex
	items map[string]resources.AgentRole
	db    *sql.DB
}

func NewAgentRoleStore() *AgentRoleStore {
	return &AgentRoleStore{items: make(map[string]resources.AgentRole)}
}

func NewAgentRoleStoreWithDB(db *sql.DB) *AgentRoleStore {
	return &AgentRoleStore{items: make(map[string]resources.AgentRole), db: db}
}

func (s *AgentRoleStore) Upsert(ctx context.Context, item resources.AgentRole) (resources.AgentRole, error) {
	if err := item.Normalize(); err != nil {
		return resources.AgentRole{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.AgentRole{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.AgentRole](ctx, tx, tableAgentRoles, key)
		if err != nil {
			return resources.AgentRole{}, err
		}
		if !found {
			if err := initializeCreateMetadata("AgentRole", &item.Metadata); err != nil {
				return resources.AgentRole{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("AgentRole", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.AgentRole{}, err
			}
		}
		if err := upsertAgentRoleSQL(ctx, tx, key, item); err != nil {
			return resources.AgentRole{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.AgentRole{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("AgentRole", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.AgentRole{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("AgentRole", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.AgentRole{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *AgentRoleStore) Get(ctx context.Context, name string) (resources.AgentRole, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.AgentRole](ctx, s.db, tableAgentRoles, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *AgentRoleStore) List(ctx context.Context) ([]resources.AgentRole, error) {
	if s.db != nil {
		return listFromTable[resources.AgentRole](ctx, s.db, tableAgentRoles)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.AgentRole, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *AgentRoleStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.AgentRole, error) {
	if s.db != nil {
		return listFromTableCursor[resources.AgentRole](ctx, s.db, tableAgentRoles, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.AgentRole, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.AgentRole) string { return a.Metadata.Name },
		func(a resources.AgentRole) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *AgentRoleStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableAgentRoles, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("agentrole %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("agentrole %q not found", name)
	}
	delete(s.items, key)
	return nil
}

type ToolPermissionStore struct {
	mu    sync.RWMutex
	items map[string]resources.ToolPermission
	db    *sql.DB
}

func NewToolPermissionStore() *ToolPermissionStore {
	return &ToolPermissionStore{items: make(map[string]resources.ToolPermission)}
}

func NewToolPermissionStoreWithDB(db *sql.DB) *ToolPermissionStore {
	return &ToolPermissionStore{items: make(map[string]resources.ToolPermission), db: db}
}

func (s *ToolPermissionStore) Upsert(ctx context.Context, item resources.ToolPermission) (resources.ToolPermission, error) {
	if err := item.Normalize(); err != nil {
		return resources.ToolPermission{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.ToolPermission{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.ToolPermission](ctx, tx, tableToolPermissions, key)
		if err != nil {
			return resources.ToolPermission{}, err
		}
		if !found {
			if err := initializeCreateMetadata("ToolPermission", &item.Metadata); err != nil {
				return resources.ToolPermission{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("ToolPermission", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.ToolPermission{}, err
			}
		}
		if err := upsertToolPermissionSQL(ctx, tx, key, item); err != nil {
			return resources.ToolPermission{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.ToolPermission{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("ToolPermission", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.ToolPermission{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("ToolPermission", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.ToolPermission{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *ToolPermissionStore) Get(ctx context.Context, name string) (resources.ToolPermission, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.ToolPermission](ctx, s.db, tableToolPermissions, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *ToolPermissionStore) List(ctx context.Context) ([]resources.ToolPermission, error) {
	if s.db != nil {
		return listFromTable[resources.ToolPermission](ctx, s.db, tableToolPermissions)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.ToolPermission, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *ToolPermissionStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.ToolPermission, error) {
	if s.db != nil {
		return listFromTableCursor[resources.ToolPermission](ctx, s.db, tableToolPermissions, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.ToolPermission, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.ToolPermission) string { return a.Metadata.Name },
		func(a resources.ToolPermission) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *ToolPermissionStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableToolPermissions, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("toolpermission %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("toolpermission %q not found", name)
	}
	delete(s.items, key)
	return nil
}

type ToolApprovalStore struct {
	mu    sync.RWMutex
	items map[string]resources.ToolApproval
	db    *sql.DB
}

func NewToolApprovalStore() *ToolApprovalStore {
	return &ToolApprovalStore{items: make(map[string]resources.ToolApproval)}
}

func NewToolApprovalStoreWithDB(db *sql.DB) *ToolApprovalStore {
	return &ToolApprovalStore{items: make(map[string]resources.ToolApproval), db: db}
}

func (s *ToolApprovalStore) Upsert(ctx context.Context, item resources.ToolApproval) (resources.ToolApproval, error) {
	if err := item.Normalize(); err != nil {
		return resources.ToolApproval{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.ToolApproval{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.ToolApproval](ctx, tx, tableToolApprovals, key)
		if err != nil {
			return resources.ToolApproval{}, err
		}
		if !found {
			if err := initializeCreateMetadata("ToolApproval", &item.Metadata); err != nil {
				return resources.ToolApproval{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("ToolApproval", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.ToolApproval{}, err
			}
		}
		if err := upsertToolApprovalSQL(ctx, tx, key, item); err != nil {
			return resources.ToolApproval{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.ToolApproval{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("ToolApproval", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.ToolApproval{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("ToolApproval", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.ToolApproval{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *ToolApprovalStore) Get(ctx context.Context, name string) (resources.ToolApproval, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.ToolApproval](ctx, s.db, tableToolApprovals, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *ToolApprovalStore) List(ctx context.Context) ([]resources.ToolApproval, error) {
	if s.db != nil {
		return listFromTable[resources.ToolApproval](ctx, s.db, tableToolApprovals)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.ToolApproval, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *ToolApprovalStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.ToolApproval, error) {
	if s.db != nil {
		return listFromTableCursor[resources.ToolApproval](ctx, s.db, tableToolApprovals, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.ToolApproval, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.ToolApproval) string { return a.Metadata.Name },
		func(a resources.ToolApproval) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *ToolApprovalStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableToolApprovals, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("toolapproval %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("toolapproval %q not found", name)
	}
	delete(s.items, key)
	return nil
}

type TaskStore struct {
	mu    sync.RWMutex
	items map[string]resources.Task
	logs  map[string][]string
	db    *sql.DB
}

type TaskScheduleStore struct {
	mu    sync.RWMutex
	items map[string]resources.TaskSchedule
	db    *sql.DB
}

type TaskWebhookStore struct {
	mu    sync.RWMutex
	items map[string]resources.TaskWebhook
	db    *sql.DB
}

type WorkerStore struct {
	mu    sync.RWMutex
	items map[string]resources.Worker
	db    *sql.DB
}

func NewTaskScheduleStore() *TaskScheduleStore {
	return &TaskScheduleStore{items: make(map[string]resources.TaskSchedule)}
}

func NewTaskScheduleStoreWithDB(db *sql.DB) *TaskScheduleStore {
	return &TaskScheduleStore{items: make(map[string]resources.TaskSchedule), db: db}
}

func NewTaskWebhookStore() *TaskWebhookStore {
	return &TaskWebhookStore{items: make(map[string]resources.TaskWebhook)}
}

func NewTaskWebhookStoreWithDB(db *sql.DB) *TaskWebhookStore {
	return &TaskWebhookStore{items: make(map[string]resources.TaskWebhook), db: db}
}

func (s *TaskScheduleStore) Upsert(ctx context.Context, item resources.TaskSchedule) (resources.TaskSchedule, error) {
	if err := item.Normalize(); err != nil {
		return resources.TaskSchedule{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.TaskSchedule{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.TaskSchedule](ctx, tx, tableTaskSchedules, key)
		if err != nil {
			return resources.TaskSchedule{}, err
		}
		if !found {
			if err := initializeCreateMetadata("TaskSchedule", &item.Metadata); err != nil {
				return resources.TaskSchedule{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("TaskSchedule", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.TaskSchedule{}, err
			}
		}
		if err := upsertTaskScheduleSQL(ctx, tx, key, item); err != nil {
			return resources.TaskSchedule{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.TaskSchedule{}, err
		}
		return item, nil
	}

	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("TaskSchedule", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.TaskSchedule{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("TaskSchedule", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.TaskSchedule{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *TaskScheduleStore) Get(ctx context.Context, name string) (resources.TaskSchedule, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.TaskSchedule](ctx, s.db, tableTaskSchedules, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *TaskScheduleStore) List(ctx context.Context) ([]resources.TaskSchedule, error) {
	if s.db != nil {
		return listFromTable[resources.TaskSchedule](ctx, s.db, tableTaskSchedules)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.TaskSchedule, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *TaskScheduleStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.TaskSchedule, error) {
	if s.db != nil {
		return listFromTableCursor[resources.TaskSchedule](ctx, s.db, tableTaskSchedules, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.TaskSchedule, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.TaskSchedule) string { return a.Metadata.Name },
		func(a resources.TaskSchedule) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *TaskScheduleStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableTaskSchedules, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("taskschedule %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("taskschedule %q not found", name)
	}
	delete(s.items, key)
	return nil
}

func (s *TaskWebhookStore) Upsert(ctx context.Context, item resources.TaskWebhook) (resources.TaskWebhook, error) {
	if err := item.Normalize(); err != nil {
		return resources.TaskWebhook{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.TaskWebhook{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.TaskWebhook](ctx, tx, tableTaskWebhooks, key)
		if err != nil {
			return resources.TaskWebhook{}, err
		}
		if !found {
			if err := initializeCreateMetadata("TaskWebhook", &item.Metadata); err != nil {
				return resources.TaskWebhook{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("TaskWebhook", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.TaskWebhook{}, err
			}
		}
		if err := upsertTaskWebhookSQL(ctx, tx, key, item); err != nil {
			return resources.TaskWebhook{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.TaskWebhook{}, err
		}
		return item, nil
	}

	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("TaskWebhook", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.TaskWebhook{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("TaskWebhook", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.TaskWebhook{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *TaskWebhookStore) Get(ctx context.Context, name string) (resources.TaskWebhook, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.TaskWebhook](ctx, s.db, tableTaskWebhooks, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *TaskWebhookStore) List(ctx context.Context) ([]resources.TaskWebhook, error) {
	if s.db != nil {
		return listFromTable[resources.TaskWebhook](ctx, s.db, tableTaskWebhooks)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.TaskWebhook, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *TaskWebhookStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.TaskWebhook, error) {
	if s.db != nil {
		return listFromTableCursor[resources.TaskWebhook](ctx, s.db, tableTaskWebhooks, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.TaskWebhook, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.TaskWebhook) string { return a.Metadata.Name },
		func(a resources.TaskWebhook) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *TaskWebhookStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableTaskWebhooks, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("taskwebhook %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("taskwebhook %q not found", name)
	}
	delete(s.items, key)
	return nil
}

func NewWorkerStore() *WorkerStore {
	return &WorkerStore{items: make(map[string]resources.Worker)}
}

func NewWorkerStoreWithDB(db *sql.DB) *WorkerStore {
	return &WorkerStore{items: make(map[string]resources.Worker), db: db}
}

func (s *WorkerStore) Upsert(ctx context.Context, item resources.Worker) (resources.Worker, error) {
	if err := item.Normalize(); err != nil {
		return resources.Worker{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.Worker{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.Worker](ctx, tx, tableWorkers, key)
		if err != nil {
			return resources.Worker{}, err
		}
		if !found {
			if err := initializeCreateMetadata("Worker", &item.Metadata); err != nil {
				return resources.Worker{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("Worker", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.Worker{}, err
			}
		}
		if err := upsertWorkerSQL(ctx, tx, key, item); err != nil {
			return resources.Worker{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.Worker{}, err
		}
		return item, nil
	}

	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("Worker", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.Worker{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("Worker", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.Worker{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *WorkerStore) Get(ctx context.Context, name string) (resources.Worker, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.Worker](ctx, s.db, tableWorkers, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *WorkerStore) List(ctx context.Context) ([]resources.Worker, error) {
	if s.db != nil {
		return listFromTable[resources.Worker](ctx, s.db, tableWorkers)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.Worker, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *WorkerStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.Worker, error) {
	if s.db != nil {
		return listFromTableCursor[resources.Worker](ctx, s.db, tableWorkers, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.Worker, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.Worker) string { return a.Metadata.Name },
		func(a resources.Worker) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *WorkerStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableWorkers, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("worker %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("worker %q not found", name)
	}
	delete(s.items, key)
	return nil
}

func (s *WorkerStore) TryAcquireSlot(ctx context.Context, name string) (resources.Worker, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return tryAcquireWorkerSlotSQL(ctx, s.db, key)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	worker, ok := s.items[key]
	if !ok {
		return resources.Worker{}, false, nil
	}
	phase := strings.ToLower(strings.TrimSpace(worker.Status.Phase))
	if phase != "ready" && phase != "pending" {
		return worker, false, nil
	}
	maxConcurrent := worker.Spec.MaxConcurrentTasks
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	if worker.Status.CurrentTasks >= maxConcurrent {
		return worker, false, nil
	}

	current := worker.Metadata
	worker.Status.CurrentTasks++
	worker.Status.ObservedGeneration = worker.Metadata.Generation
	if err := initializeUpdateMetadata("Worker", &worker.Metadata, current, false); err != nil {
		return resources.Worker{}, false, err
	}
	s.items[key] = worker
	return worker, true, nil
}

func (s *WorkerStore) ReleaseSlot(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		return releaseWorkerSlotSQL(ctx, s.db, key)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	worker, ok := s.items[key]
	if !ok {
		return nil
	}
	if worker.Status.CurrentTasks <= 0 {
		return nil
	}

	current := worker.Metadata
	worker.Status.CurrentTasks--
	if worker.Status.CurrentTasks < 0 {
		worker.Status.CurrentTasks = 0
	}
	worker.Status.ObservedGeneration = worker.Metadata.Generation
	if err := initializeUpdateMetadata("Worker", &worker.Metadata, current, false); err != nil {
		return err
	}
	s.items[key] = worker
	return nil
}

func NewTaskStore() *TaskStore {
	return &TaskStore{
		items: make(map[string]resources.Task),
		logs:  make(map[string][]string),
	}
}

func NewTaskStoreWithDB(db *sql.DB) *TaskStore {
	return &TaskStore{
		items: make(map[string]resources.Task),
		logs:  make(map[string][]string),
		db:    db,
	}
}

func (s *TaskStore) Upsert(ctx context.Context, item resources.Task) (resources.Task, error) {
	if err := item.Normalize(); err != nil {
		return resources.Task{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.Task{}, err
		}
		defer tx.Rollback()

		meta, found, err := getUpsertMetaForUpdate(ctx, tx, tableTasks, key)
		if err != nil {
			return resources.Task{}, err
		}
		if !found {
			if err := initializeCreateMetadata("Task", &item.Metadata); err != nil {
				return resources.Task{}, err
			}
		} else {
			newHash := specHash(item.Spec)
			specChanged := meta.SpecHash == "" || meta.SpecHash != newHash
			existing := resources.ObjectMeta{
				Generation:      meta.Generation,
				ResourceVersion: meta.ResourceVersion,
				CreatedAt:       meta.CreatedAt,
			}
			if err := initializeUpdateMetadata("Task", &item.Metadata, existing, specChanged); err != nil {
				return resources.Task{}, err
			}
		}
		if err := upsertTaskSQL(ctx, tx, key, item); err != nil {
			return resources.Task{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.Task{}, err
		}
		return item, nil
	}

	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("Task", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.Task{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("Task", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.Task{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *TaskStore) Get(ctx context.Context, name string) (resources.Task, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.Task](ctx, s.db, tableTasks, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *TaskStore) List(ctx context.Context) ([]resources.Task, error) {
	return s.ListPaged(ctx, 0, 0, "")
}

// ListPaged returns tasks with pagination. When namespace is non-empty the
// filter is pushed into SQL so LIMIT/OFFSET operate on the correct subset.
func (s *TaskStore) ListPaged(ctx context.Context, limit, offset int, namespace string) ([]resources.Task, error) {
	if s.db != nil {
		return listFromTableFiltered[resources.Task](ctx, s.db, tableTasks, limit, offset, namespace)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.Task, 0, len(s.items))
	for _, item := range s.items {
		if namespace != "" && !strings.EqualFold(resources.NormalizeNamespace(item.Metadata.Namespace), namespace) {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	if offset > 0 {
		if offset >= len(out) {
			return []resources.Task{}, nil
		}
		out = out[offset:]
	}
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

func (s *TaskStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.Task, error) {
	if s.db != nil {
		return listFromTableCursor[resources.Task](ctx, s.db, tableTasks, limit, after, namespace)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.Task, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.Task) string { return a.Metadata.Name },
		func(a resources.Task) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *TaskStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableTasks, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("task %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("task %q not found", name)
	}
	delete(s.items, key)
	delete(s.logs, key)
	return nil
}

func (s *TaskStore) AppendLog(ctx context.Context, name, message string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		entry := fmt.Sprintf("%s %s", time.Now().UTC().Format(time.RFC3339), message)
		return appendTaskLogSQL(ctx, s.db, key, entry)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("task %q not found", name)
	}
	entry := fmt.Sprintf("%s %s", time.Now().UTC().Format(time.RFC3339), message)
	s.logs[key] = append(s.logs[key], entry)
	if len(s.logs[key]) > 500 {
		s.logs[key] = s.logs[key][len(s.logs[key])-500:]
	}
	return nil
}

func (s *TaskStore) GetLogs(ctx context.Context, name string) ([]string, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return listTaskLogsSQL(ctx, s.db, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.items[key]; !ok {
		return nil, fmt.Errorf("task %q not found", name)
	}
	entries := s.logs[key]
	out := make([]string, len(entries))
	copy(out, entries)
	return out, nil
}

func (s *TaskStore) ClaimIfDue(ctx context.Context, name, workerID string, lease time.Duration) (resources.Task, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return claimTaskSQL(ctx, s.db, key, workerID, lease)
	}

	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.items[key]
	if !ok {
		return resources.Task{}, false, nil
	}
	if !isTaskClaimable(task, workerID, now) {
		return resources.Task{}, false, nil
	}

	claimedTask, err := applyTaskClaim(task, workerID, lease, now)
	if err != nil {
		return resources.Task{}, false, err
	}
	s.items[key] = claimedTask
	return claimedTask, true, nil
}

func (s *TaskStore) ClaimNextDue(ctx context.Context, workerID string, lease time.Duration, hints WorkerClaimHints, matches func(resources.Task) bool) (resources.Task, bool, error) {
	if s.db != nil {
		return claimNextDueTaskSQL(ctx, s.db, workerID, lease, hints, matches)
	}
	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	names := make([]string, 0, len(s.items))
	for name := range s.items {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		task := s.items[name]
		if !isTaskClaimable(task, workerID, now) {
			continue
		}
		if matches != nil && !matches(task) {
			continue
		}
		claimedTask, err := applyTaskClaim(task, workerID, lease, now)
		if err != nil {
			return resources.Task{}, false, err
		}
		s.items[name] = claimedTask
		return claimedTask, true, nil
	}
	return resources.Task{}, false, nil
}

func (s *TaskStore) RenewLease(ctx context.Context, name, workerID string, lease time.Duration) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		return renewTaskLeaseSQL(ctx, s.db, key, workerID, lease)
	}

	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.items[key]
	if !ok {
		return fmt.Errorf("task %q not found", name)
	}
	if !strings.EqualFold(strings.TrimSpace(task.Status.ClaimedBy), strings.TrimSpace(workerID)) {
		return fmt.Errorf("task %q is claimed by %q, not %q", name, task.Status.ClaimedBy, workerID)
	}
	if !strings.EqualFold(strings.TrimSpace(task.Status.Phase), "running") {
		return fmt.Errorf("task %q is not running", name)
	}

	task.Status.LeaseUntil = now.Add(lease).Format(time.RFC3339)
	task.Status.LastHeartbeat = now.Format(time.RFC3339)
	task.Status.ObservedGeneration = task.Metadata.Generation

	if err := initializeUpdateMetadata("Task", &task.Metadata, s.items[key].Metadata, false); err != nil {
		return err
	}
	s.items[key] = task
	return nil
}

func applyTaskClaim(task resources.Task, workerID string, lease time.Duration, now time.Time) (resources.Task, error) {
	current := task.Metadata
	previousPhase := strings.ToLower(strings.TrimSpace(task.Status.Phase))
	previousWorker := strings.TrimSpace(task.Status.ClaimedBy)
	takeover := previousPhase == "running" && previousWorker != "" && !strings.EqualFold(previousWorker, strings.TrimSpace(workerID))

	task.Status.Phase = "Running"
	task.Status.NextAttemptAt = ""
	task.Status.CompletedAt = ""
	task.Status.Output = nil
	task.Status.AssignedWorker = workerID
	task.Status.ClaimedBy = workerID
	task.Status.LeaseUntil = now.Add(lease).Format(time.RFC3339Nano)
	task.Status.LastHeartbeat = now.Format(time.RFC3339Nano)
	task.Status.ObservedGeneration = task.Metadata.Generation
	if previousPhase != "running" {
		task.Status.Attempts++
	}
	if strings.TrimSpace(task.Status.StartedAt) == "" {
		task.Status.StartedAt = now.Format(time.RFC3339Nano)
	}
	if takeover {
		task.Status.LastError = fmt.Sprintf("worker lease expired; task reassigned from %s to %s", previousWorker, workerID)
		task.Status.History = append(task.Status.History, resources.TaskHistoryEvent{
			Timestamp: now.Format(time.RFC3339Nano),
			Type:      "takeover",
			Worker:    workerID,
			Message:   task.Status.LastError,
		})
		if len(task.Status.History) > 200 {
			task.Status.History = task.Status.History[len(task.Status.History)-200:]
		}
	}

	if err := initializeUpdateMetadata("Task", &task.Metadata, current, false); err != nil {
		return resources.Task{}, err
	}
	return task, nil
}

func isTaskClaimable(task resources.Task, workerID string, now time.Time) bool {
	if strings.EqualFold(strings.TrimSpace(task.Spec.Mode), "template") {
		return false
	}
	phase := strings.ToLower(strings.TrimSpace(task.Status.Phase))
	switch phase {
	case "", "pending":
		return taskAttemptDue(task, now)
	case "running":
		claimedBy := strings.TrimSpace(task.Status.ClaimedBy)
		if claimedBy == "" {
			return true
		}
		if strings.EqualFold(claimedBy, strings.TrimSpace(workerID)) {
			return false
		}
		if strings.TrimSpace(task.Status.LeaseUntil) == "" {
			return true
		}
		expiry, err := parseTimestamp(task.Status.LeaseUntil)
		if err != nil {
			return true
		}
		return !now.Before(expiry)
	default:
		return false
	}
}

func taskAttemptDue(task resources.Task, now time.Time) bool {
	next := strings.TrimSpace(task.Status.NextAttemptAt)
	if next == "" {
		return true
	}
	when, err := parseTimestamp(next)
	if err != nil {
		return true
	}
	return !now.Before(when)
}

func parseTimestamp(value string) (time.Time, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, v)
}

// McpServerStore manages McpServer resources.
type McpServerStore struct {
	mu    sync.RWMutex
	items map[string]resources.McpServer
	db    *sql.DB
}

func NewMcpServerStore() *McpServerStore {
	return &McpServerStore{items: make(map[string]resources.McpServer)}
}

func NewMcpServerStoreWithDB(db *sql.DB) *McpServerStore {
	return &McpServerStore{items: make(map[string]resources.McpServer), db: db}
}

func (s *McpServerStore) Upsert(ctx context.Context, item resources.McpServer) (resources.McpServer, error) {
	if err := item.Normalize(); err != nil {
		return resources.McpServer{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.McpServer{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.McpServer](ctx, tx, tableMcpServers, key)
		if err != nil {
			return resources.McpServer{}, err
		}
		if !found {
			if err := initializeCreateMetadata("McpServer", &item.Metadata); err != nil {
				return resources.McpServer{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("McpServer", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.McpServer{}, err
			}
		}
		if err := upsertMcpServerSQL(ctx, tx, key, item); err != nil {
			return resources.McpServer{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.McpServer{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("McpServer", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.McpServer{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("McpServer", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.McpServer{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *McpServerStore) Get(ctx context.Context, name string) (resources.McpServer, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.McpServer](ctx, s.db, tableMcpServers, key)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *McpServerStore) List(ctx context.Context) ([]resources.McpServer, error) {
	if s.db != nil {
		return listFromTable[resources.McpServer](ctx, s.db, tableMcpServers)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.McpServer, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *McpServerStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.McpServer, error) {
	if s.db != nil {
		return listFromTableCursor[resources.McpServer](ctx, s.db, tableMcpServers, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.McpServer, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.McpServer) string { return a.Metadata.Name },
		func(a resources.McpServer) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *McpServerStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableMcpServers, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("mcp-server %q not found", name)
		}
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("mcp-server %q not found", name)
	}
	delete(s.items, key)
	return nil
}
