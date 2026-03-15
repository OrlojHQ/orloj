package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OrlojHQ/orloj/crds"
)

type AgentSystemStore struct {
	mu    sync.RWMutex
	items map[string]crds.AgentSystem
	db    *sql.DB
}

func NewAgentSystemStore() *AgentSystemStore {
	return &AgentSystemStore{items: make(map[string]crds.AgentSystem)}
}

func NewAgentSystemStoreWithDB(db *sql.DB) *AgentSystemStore {
	return &AgentSystemStore{items: make(map[string]crds.AgentSystem), db: db}
}

func (s *AgentSystemStore) Upsert(item crds.AgentSystem) (crds.AgentSystem, error) {
	if err := item.Normalize(); err != nil {
		return crds.AgentSystem{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		var existing crds.AgentSystem
		found, err := getResource(s.db, kindAgentSystem, key, &existing)
		if err != nil {
			return crds.AgentSystem{}, err
		}
		if !found {
			if err := initializeCreateMetadata("AgentSystem", &item.Metadata); err != nil {
				return crds.AgentSystem{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("AgentSystem", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return crds.AgentSystem{}, err
			}
		}
		if err := upsertResource(s.db, kindAgentSystem, key, item); err != nil {
			return crds.AgentSystem{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("AgentSystem", &item.Metadata); err != nil {
			s.mu.Unlock()
			return crds.AgentSystem{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("AgentSystem", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return crds.AgentSystem{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *AgentSystemStore) Get(name string) (crds.AgentSystem, bool) {
	key := normalizeLookupName(name)
	if s.db != nil {
		var item crds.AgentSystem
		ok, err := getResource(s.db, kindAgentSystem, key, &item)
		if err != nil {
			return crds.AgentSystem{}, false
		}
		return item, ok
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok
}

func (s *AgentSystemStore) List() []crds.AgentSystem {
	if s.db != nil {
		items, err := listResources[crds.AgentSystem](s.db, kindAgentSystem)
		if err != nil {
			return []crds.AgentSystem{}
		}
		return items
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]crds.AgentSystem, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

func (s *AgentSystemStore) Delete(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteResource(s.db, kindAgentSystem, key)
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
	items map[string]crds.ModelEndpoint
	db    *sql.DB
}

func NewModelEndpointStore() *ModelEndpointStore {
	return &ModelEndpointStore{items: make(map[string]crds.ModelEndpoint)}
}

func NewModelEndpointStoreWithDB(db *sql.DB) *ModelEndpointStore {
	return &ModelEndpointStore{items: make(map[string]crds.ModelEndpoint), db: db}
}

func (s *ModelEndpointStore) Upsert(item crds.ModelEndpoint) (crds.ModelEndpoint, error) {
	if err := item.Normalize(); err != nil {
		return crds.ModelEndpoint{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		var existing crds.ModelEndpoint
		found, err := getResource(s.db, kindModelEP, key, &existing)
		if err != nil {
			return crds.ModelEndpoint{}, err
		}
		if !found {
			if err := initializeCreateMetadata("ModelEndpoint", &item.Metadata); err != nil {
				return crds.ModelEndpoint{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("ModelEndpoint", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return crds.ModelEndpoint{}, err
			}
		}
		if err := upsertResource(s.db, kindModelEP, key, item); err != nil {
			return crds.ModelEndpoint{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("ModelEndpoint", &item.Metadata); err != nil {
			s.mu.Unlock()
			return crds.ModelEndpoint{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("ModelEndpoint", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return crds.ModelEndpoint{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *ModelEndpointStore) Get(name string) (crds.ModelEndpoint, bool) {
	key := normalizeLookupName(name)
	if s.db != nil {
		var item crds.ModelEndpoint
		ok, err := getResource(s.db, kindModelEP, key, &item)
		if err != nil {
			return crds.ModelEndpoint{}, false
		}
		return item, ok
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok
}

func (s *ModelEndpointStore) List() []crds.ModelEndpoint {
	if s.db != nil {
		items, err := listResources[crds.ModelEndpoint](s.db, kindModelEP)
		if err != nil {
			return []crds.ModelEndpoint{}
		}
		return items
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]crds.ModelEndpoint, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

func (s *ModelEndpointStore) Delete(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteResource(s.db, kindModelEP, key)
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
	items map[string]crds.Tool
	db    *sql.DB
}

func NewToolStore() *ToolStore {
	return &ToolStore{items: make(map[string]crds.Tool)}
}

func NewToolStoreWithDB(db *sql.DB) *ToolStore {
	return &ToolStore{items: make(map[string]crds.Tool), db: db}
}

func (s *ToolStore) Upsert(item crds.Tool) (crds.Tool, error) {
	if err := item.Normalize(); err != nil {
		return crds.Tool{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		var existing crds.Tool
		found, err := getResource(s.db, kindTool, key, &existing)
		if err != nil {
			return crds.Tool{}, err
		}
		if !found {
			if err := initializeCreateMetadata("Tool", &item.Metadata); err != nil {
				return crds.Tool{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("Tool", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return crds.Tool{}, err
			}
		}
		if err := upsertResource(s.db, kindTool, key, item); err != nil {
			return crds.Tool{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("Tool", &item.Metadata); err != nil {
			s.mu.Unlock()
			return crds.Tool{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("Tool", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return crds.Tool{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *ToolStore) Get(name string) (crds.Tool, bool) {
	key := normalizeLookupName(name)
	if s.db != nil {
		var item crds.Tool
		ok, err := getResource(s.db, kindTool, key, &item)
		if err != nil {
			return crds.Tool{}, false
		}
		return item, ok
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok
}

func (s *ToolStore) List() []crds.Tool {
	if s.db != nil {
		items, err := listResources[crds.Tool](s.db, kindTool)
		if err != nil {
			return []crds.Tool{}
		}
		return items
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]crds.Tool, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

func (s *ToolStore) Delete(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteResource(s.db, kindTool, key)
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
	mu    sync.RWMutex
	items map[string]crds.Secret
	db    *sql.DB
}

func NewSecretStore() *SecretStore {
	return &SecretStore{items: make(map[string]crds.Secret)}
}

func NewSecretStoreWithDB(db *sql.DB) *SecretStore {
	return &SecretStore{items: make(map[string]crds.Secret), db: db}
}

func (s *SecretStore) Upsert(item crds.Secret) (crds.Secret, error) {
	if err := item.Normalize(); err != nil {
		return crds.Secret{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		var existing crds.Secret
		found, err := getResource(s.db, kindSecret, key, &existing)
		if err != nil {
			return crds.Secret{}, err
		}
		if !found {
			if err := initializeCreateMetadata("Secret", &item.Metadata); err != nil {
				return crds.Secret{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("Secret", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return crds.Secret{}, err
			}
		}
		if err := upsertResource(s.db, kindSecret, key, item); err != nil {
			return crds.Secret{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("Secret", &item.Metadata); err != nil {
			s.mu.Unlock()
			return crds.Secret{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("Secret", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return crds.Secret{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *SecretStore) Get(name string) (crds.Secret, bool) {
	key := normalizeLookupName(name)
	if s.db != nil {
		var item crds.Secret
		ok, err := getResource(s.db, kindSecret, key, &item)
		if err != nil {
			return crds.Secret{}, false
		}
		return item, ok
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok
}

func (s *SecretStore) List() []crds.Secret {
	if s.db != nil {
		items, err := listResources[crds.Secret](s.db, kindSecret)
		if err != nil {
			return []crds.Secret{}
		}
		return items
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]crds.Secret, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

func (s *SecretStore) Delete(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteResource(s.db, kindSecret, key)
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
	items map[string]crds.Memory
	db    *sql.DB
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: make(map[string]crds.Memory)}
}

func NewMemoryStoreWithDB(db *sql.DB) *MemoryStore {
	return &MemoryStore{items: make(map[string]crds.Memory), db: db}
}

func (s *MemoryStore) Upsert(item crds.Memory) (crds.Memory, error) {
	if err := item.Normalize(); err != nil {
		return crds.Memory{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		var existing crds.Memory
		found, err := getResource(s.db, kindMemory, key, &existing)
		if err != nil {
			return crds.Memory{}, err
		}
		if !found {
			if err := initializeCreateMetadata("Memory", &item.Metadata); err != nil {
				return crds.Memory{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("Memory", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return crds.Memory{}, err
			}
		}
		if err := upsertResource(s.db, kindMemory, key, item); err != nil {
			return crds.Memory{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("Memory", &item.Metadata); err != nil {
			s.mu.Unlock()
			return crds.Memory{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("Memory", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return crds.Memory{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *MemoryStore) Get(name string) (crds.Memory, bool) {
	key := normalizeLookupName(name)
	if s.db != nil {
		var item crds.Memory
		ok, err := getResource(s.db, kindMemory, key, &item)
		if err != nil {
			return crds.Memory{}, false
		}
		return item, ok
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok
}

func (s *MemoryStore) List() []crds.Memory {
	if s.db != nil {
		items, err := listResources[crds.Memory](s.db, kindMemory)
		if err != nil {
			return []crds.Memory{}
		}
		return items
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]crds.Memory, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

func (s *MemoryStore) Delete(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteResource(s.db, kindMemory, key)
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
	items map[string]crds.AgentPolicy
	db    *sql.DB
}

func NewAgentPolicyStore() *AgentPolicyStore {
	return &AgentPolicyStore{items: make(map[string]crds.AgentPolicy)}
}

func NewAgentPolicyStoreWithDB(db *sql.DB) *AgentPolicyStore {
	return &AgentPolicyStore{items: make(map[string]crds.AgentPolicy), db: db}
}

func (s *AgentPolicyStore) Upsert(item crds.AgentPolicy) (crds.AgentPolicy, error) {
	if err := item.Normalize(); err != nil {
		return crds.AgentPolicy{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		var existing crds.AgentPolicy
		found, err := getResource(s.db, kindAgentPolicy, key, &existing)
		if err != nil {
			return crds.AgentPolicy{}, err
		}
		if !found {
			if err := initializeCreateMetadata("AgentPolicy", &item.Metadata); err != nil {
				return crds.AgentPolicy{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("AgentPolicy", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return crds.AgentPolicy{}, err
			}
		}
		if err := upsertResource(s.db, kindAgentPolicy, key, item); err != nil {
			return crds.AgentPolicy{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("AgentPolicy", &item.Metadata); err != nil {
			s.mu.Unlock()
			return crds.AgentPolicy{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("AgentPolicy", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return crds.AgentPolicy{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *AgentPolicyStore) Get(name string) (crds.AgentPolicy, bool) {
	key := normalizeLookupName(name)
	if s.db != nil {
		var item crds.AgentPolicy
		ok, err := getResource(s.db, kindAgentPolicy, key, &item)
		if err != nil {
			return crds.AgentPolicy{}, false
		}
		return item, ok
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok
}

func (s *AgentPolicyStore) List() []crds.AgentPolicy {
	if s.db != nil {
		items, err := listResources[crds.AgentPolicy](s.db, kindAgentPolicy)
		if err != nil {
			return []crds.AgentPolicy{}
		}
		return items
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]crds.AgentPolicy, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

func (s *AgentPolicyStore) Delete(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteResource(s.db, kindAgentPolicy, key)
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
	items map[string]crds.AgentRole
	db    *sql.DB
}

func NewAgentRoleStore() *AgentRoleStore {
	return &AgentRoleStore{items: make(map[string]crds.AgentRole)}
}

func NewAgentRoleStoreWithDB(db *sql.DB) *AgentRoleStore {
	return &AgentRoleStore{items: make(map[string]crds.AgentRole), db: db}
}

func (s *AgentRoleStore) Upsert(item crds.AgentRole) (crds.AgentRole, error) {
	if err := item.Normalize(); err != nil {
		return crds.AgentRole{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		var existing crds.AgentRole
		found, err := getResource(s.db, kindAgentRole, key, &existing)
		if err != nil {
			return crds.AgentRole{}, err
		}
		if !found {
			if err := initializeCreateMetadata("AgentRole", &item.Metadata); err != nil {
				return crds.AgentRole{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("AgentRole", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return crds.AgentRole{}, err
			}
		}
		if err := upsertResource(s.db, kindAgentRole, key, item); err != nil {
			return crds.AgentRole{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("AgentRole", &item.Metadata); err != nil {
			s.mu.Unlock()
			return crds.AgentRole{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("AgentRole", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return crds.AgentRole{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *AgentRoleStore) Get(name string) (crds.AgentRole, bool) {
	key := normalizeLookupName(name)
	if s.db != nil {
		var item crds.AgentRole
		ok, err := getResource(s.db, kindAgentRole, key, &item)
		if err != nil {
			return crds.AgentRole{}, false
		}
		return item, ok
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok
}

func (s *AgentRoleStore) List() []crds.AgentRole {
	if s.db != nil {
		items, err := listResources[crds.AgentRole](s.db, kindAgentRole)
		if err != nil {
			return []crds.AgentRole{}
		}
		return items
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]crds.AgentRole, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

func (s *AgentRoleStore) Delete(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteResource(s.db, kindAgentRole, key)
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
	items map[string]crds.ToolPermission
	db    *sql.DB
}

func NewToolPermissionStore() *ToolPermissionStore {
	return &ToolPermissionStore{items: make(map[string]crds.ToolPermission)}
}

func NewToolPermissionStoreWithDB(db *sql.DB) *ToolPermissionStore {
	return &ToolPermissionStore{items: make(map[string]crds.ToolPermission), db: db}
}

func (s *ToolPermissionStore) Upsert(item crds.ToolPermission) (crds.ToolPermission, error) {
	if err := item.Normalize(); err != nil {
		return crds.ToolPermission{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		var existing crds.ToolPermission
		found, err := getResource(s.db, kindToolPerm, key, &existing)
		if err != nil {
			return crds.ToolPermission{}, err
		}
		if !found {
			if err := initializeCreateMetadata("ToolPermission", &item.Metadata); err != nil {
				return crds.ToolPermission{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("ToolPermission", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return crds.ToolPermission{}, err
			}
		}
		if err := upsertResource(s.db, kindToolPerm, key, item); err != nil {
			return crds.ToolPermission{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("ToolPermission", &item.Metadata); err != nil {
			s.mu.Unlock()
			return crds.ToolPermission{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("ToolPermission", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return crds.ToolPermission{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *ToolPermissionStore) Get(name string) (crds.ToolPermission, bool) {
	key := normalizeLookupName(name)
	if s.db != nil {
		var item crds.ToolPermission
		ok, err := getResource(s.db, kindToolPerm, key, &item)
		if err != nil {
			return crds.ToolPermission{}, false
		}
		return item, ok
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok
}

func (s *ToolPermissionStore) List() []crds.ToolPermission {
	if s.db != nil {
		items, err := listResources[crds.ToolPermission](s.db, kindToolPerm)
		if err != nil {
			return []crds.ToolPermission{}
		}
		return items
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]crds.ToolPermission, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

func (s *ToolPermissionStore) Delete(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteResource(s.db, kindToolPerm, key)
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

type TaskStore struct {
	mu    sync.RWMutex
	items map[string]crds.Task
	logs  map[string][]string
	db    *sql.DB
}

type TaskScheduleStore struct {
	mu    sync.RWMutex
	items map[string]crds.TaskSchedule
	db    *sql.DB
}

type TaskWebhookStore struct {
	mu    sync.RWMutex
	items map[string]crds.TaskWebhook
	db    *sql.DB
}

type WorkerStore struct {
	mu    sync.RWMutex
	items map[string]crds.Worker
	db    *sql.DB
}

func NewTaskScheduleStore() *TaskScheduleStore {
	return &TaskScheduleStore{items: make(map[string]crds.TaskSchedule)}
}

func NewTaskScheduleStoreWithDB(db *sql.DB) *TaskScheduleStore {
	return &TaskScheduleStore{items: make(map[string]crds.TaskSchedule), db: db}
}

func NewTaskWebhookStore() *TaskWebhookStore {
	return &TaskWebhookStore{items: make(map[string]crds.TaskWebhook)}
}

func NewTaskWebhookStoreWithDB(db *sql.DB) *TaskWebhookStore {
	return &TaskWebhookStore{items: make(map[string]crds.TaskWebhook), db: db}
}

func (s *TaskScheduleStore) Upsert(item crds.TaskSchedule) (crds.TaskSchedule, error) {
	if err := item.Normalize(); err != nil {
		return crds.TaskSchedule{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		var existing crds.TaskSchedule
		found, err := getResource(s.db, kindTaskSchedule, key, &existing)
		if err != nil {
			return crds.TaskSchedule{}, err
		}
		if !found {
			if err := initializeCreateMetadata("TaskSchedule", &item.Metadata); err != nil {
				return crds.TaskSchedule{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("TaskSchedule", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return crds.TaskSchedule{}, err
			}
		}
		if err := upsertResource(s.db, kindTaskSchedule, key, item); err != nil {
			return crds.TaskSchedule{}, err
		}
		return item, nil
	}

	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("TaskSchedule", &item.Metadata); err != nil {
			s.mu.Unlock()
			return crds.TaskSchedule{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("TaskSchedule", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return crds.TaskSchedule{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *TaskScheduleStore) Get(name string) (crds.TaskSchedule, bool) {
	key := normalizeLookupName(name)
	if s.db != nil {
		var item crds.TaskSchedule
		ok, err := getResource(s.db, kindTaskSchedule, key, &item)
		if err != nil {
			return crds.TaskSchedule{}, false
		}
		return item, ok
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok
}

func (s *TaskScheduleStore) List() []crds.TaskSchedule {
	if s.db != nil {
		items, err := listResources[crds.TaskSchedule](s.db, kindTaskSchedule)
		if err != nil {
			return []crds.TaskSchedule{}
		}
		return items
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]crds.TaskSchedule, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

func (s *TaskScheduleStore) Delete(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteResource(s.db, kindTaskSchedule, key)
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

func (s *TaskWebhookStore) Upsert(item crds.TaskWebhook) (crds.TaskWebhook, error) {
	if err := item.Normalize(); err != nil {
		return crds.TaskWebhook{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		var existing crds.TaskWebhook
		found, err := getResource(s.db, kindTaskWebhook, key, &existing)
		if err != nil {
			return crds.TaskWebhook{}, err
		}
		if !found {
			if err := initializeCreateMetadata("TaskWebhook", &item.Metadata); err != nil {
				return crds.TaskWebhook{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("TaskWebhook", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return crds.TaskWebhook{}, err
			}
		}
		if err := upsertResource(s.db, kindTaskWebhook, key, item); err != nil {
			return crds.TaskWebhook{}, err
		}
		return item, nil
	}

	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("TaskWebhook", &item.Metadata); err != nil {
			s.mu.Unlock()
			return crds.TaskWebhook{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("TaskWebhook", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return crds.TaskWebhook{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *TaskWebhookStore) Get(name string) (crds.TaskWebhook, bool) {
	key := normalizeLookupName(name)
	if s.db != nil {
		var item crds.TaskWebhook
		ok, err := getResource(s.db, kindTaskWebhook, key, &item)
		if err != nil {
			return crds.TaskWebhook{}, false
		}
		return item, ok
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok
}

func (s *TaskWebhookStore) List() []crds.TaskWebhook {
	if s.db != nil {
		items, err := listResources[crds.TaskWebhook](s.db, kindTaskWebhook)
		if err != nil {
			return []crds.TaskWebhook{}
		}
		return items
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]crds.TaskWebhook, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

func (s *TaskWebhookStore) Delete(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteResource(s.db, kindTaskWebhook, key)
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
	return &WorkerStore{items: make(map[string]crds.Worker)}
}

func NewWorkerStoreWithDB(db *sql.DB) *WorkerStore {
	return &WorkerStore{items: make(map[string]crds.Worker), db: db}
}

func (s *WorkerStore) Upsert(item crds.Worker) (crds.Worker, error) {
	if err := item.Normalize(); err != nil {
		return crds.Worker{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		var existing crds.Worker
		found, err := getResource(s.db, kindWorker, key, &existing)
		if err != nil {
			return crds.Worker{}, err
		}
		if !found {
			if err := initializeCreateMetadata("Worker", &item.Metadata); err != nil {
				return crds.Worker{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("Worker", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return crds.Worker{}, err
			}
		}
		if err := upsertResource(s.db, kindWorker, key, item); err != nil {
			return crds.Worker{}, err
		}
		return item, nil
	}

	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("Worker", &item.Metadata); err != nil {
			s.mu.Unlock()
			return crds.Worker{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("Worker", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return crds.Worker{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *WorkerStore) Get(name string) (crds.Worker, bool) {
	key := normalizeLookupName(name)
	if s.db != nil {
		var item crds.Worker
		ok, err := getResource(s.db, kindWorker, key, &item)
		if err != nil {
			return crds.Worker{}, false
		}
		return item, ok
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok
}

func (s *WorkerStore) List() []crds.Worker {
	if s.db != nil {
		items, err := listResources[crds.Worker](s.db, kindWorker)
		if err != nil {
			return []crds.Worker{}
		}
		return items
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]crds.Worker, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

func (s *WorkerStore) Delete(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteResource(s.db, kindWorker, key)
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

func (s *WorkerStore) TryAcquireSlot(name string) (crds.Worker, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return tryAcquireWorkerSlotSQL(s.db, key)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	worker, ok := s.items[key]
	if !ok {
		return crds.Worker{}, false, nil
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
		return crds.Worker{}, false, err
	}
	s.items[key] = worker
	return worker, true, nil
}

func (s *WorkerStore) ReleaseSlot(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		return releaseWorkerSlotSQL(s.db, key)
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
		items: make(map[string]crds.Task),
		logs:  make(map[string][]string),
	}
}

func NewTaskStoreWithDB(db *sql.DB) *TaskStore {
	return &TaskStore{
		items: make(map[string]crds.Task),
		logs:  make(map[string][]string),
		db:    db,
	}
}

func (s *TaskStore) Upsert(item crds.Task) (crds.Task, error) {
	if err := item.Normalize(); err != nil {
		return crds.Task{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		var existing crds.Task
		found, err := getResource(s.db, kindTask, key, &existing)
		if err != nil {
			return crds.Task{}, err
		}
		if !found {
			if err := initializeCreateMetadata("Task", &item.Metadata); err != nil {
				return crds.Task{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("Task", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return crds.Task{}, err
			}
		}
		if err := upsertResource(s.db, kindTask, key, item); err != nil {
			return crds.Task{}, err
		}
		return item, nil
	}

	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("Task", &item.Metadata); err != nil {
			s.mu.Unlock()
			return crds.Task{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("Task", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return crds.Task{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *TaskStore) Get(name string) (crds.Task, bool) {
	key := normalizeLookupName(name)
	if s.db != nil {
		var item crds.Task
		ok, err := getResource(s.db, kindTask, key, &item)
		if err != nil {
			return crds.Task{}, false
		}
		return item, ok
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok
}

func (s *TaskStore) List() []crds.Task {
	if s.db != nil {
		items, err := listResources[crds.Task](s.db, kindTask)
		if err != nil {
			return []crds.Task{}
		}
		return items
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]crds.Task, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

func (s *TaskStore) Delete(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteResource(s.db, kindTask, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("task %q not found", name)
		}
		if err := deleteTaskLogsSQL(s.db, key); err != nil {
			return err
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

func (s *TaskStore) AppendLog(name, message string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		entry := fmt.Sprintf("%s %s", time.Now().UTC().Format(time.RFC3339), message)
		return appendTaskLogSQL(s.db, key, entry)
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

func (s *TaskStore) Logs(name string) ([]string, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return listTaskLogsSQL(s.db, key)
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

func (s *TaskStore) ClaimIfDue(name, workerID string, lease time.Duration) (crds.Task, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return claimTaskSQL(s.db, key, workerID, lease)
	}

	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.items[key]
	if !ok {
		return crds.Task{}, false, nil
	}
	if !isTaskClaimable(task, workerID, now) {
		return crds.Task{}, false, nil
	}

	claimedTask, err := applyTaskClaim(task, workerID, lease, now)
	if err != nil {
		return crds.Task{}, false, err
	}
	s.items[key] = claimedTask
	return claimedTask, true, nil
}

func (s *TaskStore) ClaimNextDue(workerID string, lease time.Duration, matches func(crds.Task) bool) (crds.Task, bool, error) {
	if s.db != nil {
		return claimNextDueTaskSQL(s.db, workerID, lease, matches)
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
			return crds.Task{}, false, err
		}
		s.items[name] = claimedTask
		return claimedTask, true, nil
	}
	return crds.Task{}, false, nil
}

func (s *TaskStore) RenewLease(name, workerID string, lease time.Duration) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		return renewTaskLeaseSQL(s.db, key, workerID, lease)
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

func applyTaskClaim(task crds.Task, workerID string, lease time.Duration, now time.Time) (crds.Task, error) {
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
		task.Status.History = append(task.Status.History, crds.TaskHistoryEvent{
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
		return crds.Task{}, err
	}
	return task, nil
}

func isTaskClaimable(task crds.Task, workerID string, now time.Time) bool {
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

func taskAttemptDue(task crds.Task, now time.Time) bool {
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

func claimTaskSQL(db *sql.DB, name, workerID string, lease time.Duration) (crds.Task, bool, error) {
	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return crds.Task{}, false, err
	}
	defer tx.Rollback()

	var payload []byte
	err = tx.QueryRow(`SELECT payload FROM resources WHERE kind = $1 AND name = $2 FOR UPDATE`, kindTask, name).Scan(&payload)
	if err == sql.ErrNoRows {
		return crds.Task{}, false, nil
	}
	if err != nil {
		return crds.Task{}, false, err
	}

	var task crds.Task
	if err := json.Unmarshal(payload, &task); err != nil {
		return crds.Task{}, false, err
	}
	if !isTaskClaimable(task, workerID, now) {
		if err := tx.Commit(); err != nil {
			return crds.Task{}, false, err
		}
		return crds.Task{}, false, nil
	}

	task, err = applyTaskClaim(task, workerID, lease, now)
	if err != nil {
		return crds.Task{}, false, err
	}
	nextPayload, err := json.Marshal(task)
	if err != nil {
		return crds.Task{}, false, err
	}
	if _, err := tx.Exec(
		`UPDATE resources SET payload = $3::jsonb, updated_at = NOW() WHERE kind = $1 AND name = $2`,
		kindTask,
		name,
		string(nextPayload),
	); err != nil {
		return crds.Task{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return crds.Task{}, false, err
	}
	return task, true, nil
}

func claimNextDueTaskSQL(db *sql.DB, workerID string, lease time.Duration, matches func(crds.Task) bool) (crds.Task, bool, error) {
	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return crds.Task{}, false, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(
		`SELECT name, payload
		 FROM resources
		 WHERE kind = $1
		   AND (
		     (
		       LOWER(COALESCE(payload->'status'->>'phase', 'pending')) IN ('', 'pending')
		       AND (
		         NULLIF(payload->'status'->>'nextAttemptAt', '') IS NULL
		         OR NULLIF(payload->'status'->>'nextAttemptAt', '')::timestamptz <= NOW()
		       )
		     )
		     OR (
		       LOWER(COALESCE(payload->'status'->>'phase', '')) = 'running'
		       AND (
		         COALESCE(payload->'status'->>'claimedBy', '') = ''
		         OR NULLIF(payload->'status'->>'leaseUntil', '') IS NULL
		         OR NULLIF(payload->'status'->>'leaseUntil', '')::timestamptz <= NOW()
		       )
		     )
		   )
		 ORDER BY updated_at ASC
		 FOR UPDATE SKIP LOCKED
		 LIMIT 64`,
		kindTask,
	)
	if err != nil {
		return crds.Task{}, false, err
	}
	defer rows.Close()

	var (
		selectedName string
		selectedTask crds.Task
		found        bool
	)
	for rows.Next() {
		var (
			name    string
			payload []byte
		)
		if err := rows.Scan(&name, &payload); err != nil {
			return crds.Task{}, false, err
		}
		var task crds.Task
		if err := json.Unmarshal(payload, &task); err != nil {
			return crds.Task{}, false, err
		}
		if !isTaskClaimable(task, workerID, now) {
			continue
		}
		if matches != nil && !matches(task) {
			continue
		}
		selectedName = name
		selectedTask = task
		found = true
		break
	}
	if err := rows.Err(); err != nil {
		return crds.Task{}, false, err
	}
	if !found {
		if err := tx.Commit(); err != nil {
			return crds.Task{}, false, err
		}
		return crds.Task{}, false, nil
	}

	task, err := applyTaskClaim(selectedTask, workerID, lease, now)
	if err != nil {
		return crds.Task{}, false, err
	}
	nextPayload, err := json.Marshal(task)
	if err != nil {
		return crds.Task{}, false, err
	}
	if _, err := tx.Exec(
		`UPDATE resources SET payload = $3::jsonb, updated_at = NOW() WHERE kind = $1 AND name = $2`,
		kindTask,
		selectedName,
		string(nextPayload),
	); err != nil {
		return crds.Task{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return crds.Task{}, false, err
	}
	return task, true, nil
}

func renewTaskLeaseSQL(db *sql.DB, name, workerID string, lease time.Duration) error {
	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var payload []byte
	err = tx.QueryRow(`SELECT payload FROM resources WHERE kind = $1 AND name = $2 FOR UPDATE`, kindTask, name).Scan(&payload)
	if err == sql.ErrNoRows {
		return fmt.Errorf("task %q not found", name)
	}
	if err != nil {
		return err
	}

	var task crds.Task
	if err := json.Unmarshal(payload, &task); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(task.Status.ClaimedBy), strings.TrimSpace(workerID)) {
		return fmt.Errorf("task %q is claimed by %q, not %q", name, task.Status.ClaimedBy, workerID)
	}
	if !strings.EqualFold(strings.TrimSpace(task.Status.Phase), "running") {
		return fmt.Errorf("task %q is not running", name)
	}

	currentMeta := task.Metadata
	task.Status.LeaseUntil = now.Add(lease).Format(time.RFC3339Nano)
	task.Status.LastHeartbeat = now.Format(time.RFC3339Nano)
	task.Status.ObservedGeneration = task.Metadata.Generation
	if err := initializeUpdateMetadata("Task", &task.Metadata, currentMeta, false); err != nil {
		return err
	}

	nextPayload, err := json.Marshal(task)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE resources SET payload = $3::jsonb, updated_at = NOW() WHERE kind = $1 AND name = $2`,
		kindTask,
		name,
		string(nextPayload),
	); err != nil {
		return err
	}
	return tx.Commit()
}

func tryAcquireWorkerSlotSQL(db *sql.DB, name string) (crds.Worker, bool, error) {
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return crds.Worker{}, false, err
	}
	defer tx.Rollback()

	var payload []byte
	err = tx.QueryRow(`SELECT payload FROM resources WHERE kind = $1 AND name = $2 FOR UPDATE`, kindWorker, name).Scan(&payload)
	if err == sql.ErrNoRows {
		return crds.Worker{}, false, nil
	}
	if err != nil {
		return crds.Worker{}, false, err
	}

	var worker crds.Worker
	if err := json.Unmarshal(payload, &worker); err != nil {
		return crds.Worker{}, false, err
	}
	phase := strings.ToLower(strings.TrimSpace(worker.Status.Phase))
	if phase != "ready" && phase != "pending" {
		if err := tx.Commit(); err != nil {
			return crds.Worker{}, false, err
		}
		return worker, false, nil
	}

	maxConcurrent := worker.Spec.MaxConcurrentTasks
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	if worker.Status.CurrentTasks >= maxConcurrent {
		if err := tx.Commit(); err != nil {
			return crds.Worker{}, false, err
		}
		return worker, false, nil
	}

	current := worker.Metadata
	worker.Status.CurrentTasks++
	worker.Status.ObservedGeneration = worker.Metadata.Generation
	if err := initializeUpdateMetadata("Worker", &worker.Metadata, current, false); err != nil {
		return crds.Worker{}, false, err
	}

	nextPayload, err := json.Marshal(worker)
	if err != nil {
		return crds.Worker{}, false, err
	}
	if _, err := tx.Exec(
		`UPDATE resources SET payload = $3::jsonb, updated_at = NOW() WHERE kind = $1 AND name = $2`,
		kindWorker,
		name,
		string(nextPayload),
	); err != nil {
		return crds.Worker{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return crds.Worker{}, false, err
	}
	return worker, true, nil
}

func releaseWorkerSlotSQL(db *sql.DB, name string) error {
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var payload []byte
	err = tx.QueryRow(`SELECT payload FROM resources WHERE kind = $1 AND name = $2 FOR UPDATE`, kindWorker, name).Scan(&payload)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}

	var worker crds.Worker
	if err := json.Unmarshal(payload, &worker); err != nil {
		return err
	}
	if worker.Status.CurrentTasks <= 0 {
		return tx.Commit()
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

	nextPayload, err := json.Marshal(worker)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE resources SET payload = $3::jsonb, updated_at = NOW() WHERE kind = $1 AND name = $2`,
		kindWorker,
		name,
		string(nextPayload),
	); err != nil {
		return err
	}
	return tx.Commit()
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
