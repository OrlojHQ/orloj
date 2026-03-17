package store

import (
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

func (s *AgentStore) Upsert(agent resources.Agent) (resources.Agent, error) {
	if err := agent.Normalize(); err != nil {
		return resources.Agent{}, err
	}
	key := scopedNameFromMeta(agent.Metadata)
	if s.db != nil {
		existing, found, err := s.getWithErr(key)
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
		if err := upsertAgentSQL(s.db, key, agent); err != nil {
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

func (s *AgentStore) getWithErr(name string) (resources.Agent, bool, error) {
	if s.db != nil {
		return getFromTable[resources.Agent](s.db, tableAgents, name)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	agent, ok := s.agents[name]
	return agent, ok, nil
}

func (s *AgentStore) Get(name string) (resources.Agent, bool) {
	agent, ok, err := s.getWithErr(normalizeLookupName(name))
	if err != nil {
		return resources.Agent{}, false
	}
	return agent, ok
}

func (s *AgentStore) List() []resources.Agent {
	if s.db != nil {
		items, err := listFromTable[resources.Agent](s.db, tableAgents)
		if err != nil {
			return []resources.Agent{}
		}
		return items
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
	return out
}

func (s *AgentStore) Delete(name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(s.db, tableAgents, key)
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
