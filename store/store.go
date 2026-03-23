package store

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/OrlojHQ/orloj/resources"
)

// AgentStore keeps desired Agent state in memory for MVP.
type AgentStore struct {
	mu     sync.RWMutex
	agents map[string]resources.Agent
	db     *sql.DB
}

func NewAgentStore() *AgentStore {
	return &AgentStore{agents: make(map[string]resources.Agent)}
}

func NewAgentStoreWithDB(db *sql.DB) *AgentStore {
	return &AgentStore{
		agents: make(map[string]resources.Agent),
		db:     db,
	}
}

func (s *AgentStore) Upsert(ctx context.Context, agent resources.Agent) (resources.Agent, error) {
	if err := agent.Normalize(); err != nil {
		return resources.Agent{}, err
	}
	key := scopedNameFromMeta(agent.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.Agent{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.Agent](ctx, tx, tableAgents, key)
		if err != nil {
			return resources.Agent{}, err
		}
		if !found {
			if err := initializeCreateMetadata("Agent", &agent.Metadata); err != nil {
				return resources.Agent{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, agent.Spec)
			if err := initializeUpdateMetadata("Agent", &agent.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.Agent{}, err
			}
		}
		if err := upsertAgentSQL(ctx, tx, key, agent); err != nil {
			return resources.Agent{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.Agent{}, err
		}
		return agent, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, found := s.agents[key]
	if !found {
		if err := initializeCreateMetadata("Agent", &agent.Metadata); err != nil {
			return resources.Agent{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, agent.Spec)
		if err := initializeUpdateMetadata("Agent", &agent.Metadata, existing.Metadata, specChanged); err != nil {
			return resources.Agent{}, err
		}
	}
	s.agents[key] = agent
	return agent, nil
}

func (s *AgentStore) getWithErr(ctx context.Context, name string) (resources.Agent, bool, error) {
	if s.db != nil {
		return getFromTable[resources.Agent](ctx, s.db, tableAgents, name)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	agent, ok := s.agents[name]
	return agent, ok, nil
}

func (s *AgentStore) Get(ctx context.Context, name string) (resources.Agent, bool, error) {
	return s.getWithErr(ctx, normalizeLookupName(name))
}

func (s *AgentStore) List(ctx context.Context) ([]resources.Agent, error) {
	if s.db != nil {
		return listFromTable[resources.Agent](ctx, s.db, tableAgents)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]resources.Agent, 0, len(s.agents))
	for _, agent := range s.agents {
		out = append(out, agent)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *AgentStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.Agent, error) {
	if s.db != nil {
		return listFromTableCursor[resources.Agent](ctx, s.db, tableAgents, limit, after, namespace)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]resources.Agent, 0, len(s.agents))
	for _, agent := range s.agents {
		out = append(out, agent)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.Agent) string { return a.Metadata.Name },
		func(a resources.Agent) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *AgentStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableAgents, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("agent %q not found", strings.TrimSpace(name))
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[key]; !ok {
		return fmt.Errorf("agent %q not found", strings.TrimSpace(name))
	}
	delete(s.agents, key)
	return nil
}
