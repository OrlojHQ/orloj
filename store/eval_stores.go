package store

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/OrlojHQ/orloj/resources"
)

// ---------------------------------------------------------------------------
// EvalDatasetStore
// ---------------------------------------------------------------------------

type EvalDatasetStore struct {
	mu    sync.RWMutex
	items map[string]resources.EvalDataset
	db    *sql.DB
}

func NewEvalDatasetStore() *EvalDatasetStore {
	return &EvalDatasetStore{items: make(map[string]resources.EvalDataset)}
}

func NewEvalDatasetStoreWithDB(db *sql.DB) *EvalDatasetStore {
	return &EvalDatasetStore{items: make(map[string]resources.EvalDataset), db: db}
}

func (s *EvalDatasetStore) Upsert(ctx context.Context, item resources.EvalDataset) (resources.EvalDataset, error) {
	if err := item.Normalize(); err != nil {
		return resources.EvalDataset{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.EvalDataset{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.EvalDataset](ctx, tx, tableEvalDatasets, key)
		if err != nil {
			return resources.EvalDataset{}, err
		}
		if !found {
			if err := initializeCreateMetadata("EvalDataset", &item.Metadata); err != nil {
				return resources.EvalDataset{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("EvalDataset", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.EvalDataset{}, err
			}
		}
		if err := upsertEvalDatasetSQL(ctx, tx, key, item); err != nil {
			return resources.EvalDataset{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.EvalDataset{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("EvalDataset", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.EvalDataset{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("EvalDataset", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.EvalDataset{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *EvalDatasetStore) Get(ctx context.Context, name string) (resources.EvalDataset, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.EvalDataset](ctx, s.db, tableEvalDatasets, key)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *EvalDatasetStore) List(ctx context.Context) ([]resources.EvalDataset, error) {
	if s.db != nil {
		return listFromTable[resources.EvalDataset](ctx, s.db, tableEvalDatasets)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.EvalDataset, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *EvalDatasetStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.EvalDataset, error) {
	if s.db != nil {
		return listFromTableCursor[resources.EvalDataset](ctx, s.db, tableEvalDatasets, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.EvalDataset, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.EvalDataset) string { return a.Metadata.Name },
		func(a resources.EvalDataset) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *EvalDatasetStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableEvalDatasets, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("evaldataset %q not found", name)
		}
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("evaldataset %q not found", name)
	}
	delete(s.items, key)
	return nil
}

// ---------------------------------------------------------------------------
// EvalRunStore
// ---------------------------------------------------------------------------

type EvalRunStore struct {
	mu    sync.RWMutex
	items map[string]resources.EvalRun
	db    *sql.DB
}

func NewEvalRunStore() *EvalRunStore {
	return &EvalRunStore{items: make(map[string]resources.EvalRun)}
}

func NewEvalRunStoreWithDB(db *sql.DB) *EvalRunStore {
	return &EvalRunStore{items: make(map[string]resources.EvalRun), db: db}
}

func (s *EvalRunStore) Upsert(ctx context.Context, item resources.EvalRun) (resources.EvalRun, error) {
	if err := item.Normalize(); err != nil {
		return resources.EvalRun{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.EvalRun{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.EvalRun](ctx, tx, tableEvalRuns, key)
		if err != nil {
			return resources.EvalRun{}, err
		}
		if !found {
			if err := initializeCreateMetadata("EvalRun", &item.Metadata); err != nil {
				return resources.EvalRun{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("EvalRun", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.EvalRun{}, err
			}
		}
		if err := upsertEvalRunSQL(ctx, tx, key, item); err != nil {
			return resources.EvalRun{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.EvalRun{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("EvalRun", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.EvalRun{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("EvalRun", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.EvalRun{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

// UpdateStatus atomically updates only the status fields of an EvalRun.
func (s *EvalRunStore) UpdateStatus(ctx context.Context, name string, status resources.EvalRunStatus) (resources.EvalRun, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.EvalRun{}, err
		}
		defer tx.Rollback()

		item, found, err := getFromTableForUpdate[resources.EvalRun](ctx, tx, tableEvalRuns, key)
		if err != nil {
			return resources.EvalRun{}, err
		}
		if !found {
			return resources.EvalRun{}, fmt.Errorf("evalrun %q not found", name)
		}
		item.Status = status
		if err := upsertEvalRunSQL(ctx, tx, key, item); err != nil {
			return resources.EvalRun{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.EvalRun{}, err
		}
		return item, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[key]
	if !ok {
		return resources.EvalRun{}, fmt.Errorf("evalrun %q not found", name)
	}
	item.Status = status
	s.items[key] = item
	return item, nil
}

func (s *EvalRunStore) Get(ctx context.Context, name string) (resources.EvalRun, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.EvalRun](ctx, s.db, tableEvalRuns, key)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *EvalRunStore) List(ctx context.Context) ([]resources.EvalRun, error) {
	if s.db != nil {
		return listFromTable[resources.EvalRun](ctx, s.db, tableEvalRuns)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.EvalRun, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *EvalRunStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.EvalRun, error) {
	if s.db != nil {
		return listFromTableCursor[resources.EvalRun](ctx, s.db, tableEvalRuns, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.EvalRun, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.EvalRun) string { return a.Metadata.Name },
		func(a resources.EvalRun) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *EvalRunStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableEvalRuns, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("evalrun %q not found", name)
		}
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("evalrun %q not found", name)
	}
	delete(s.items, key)
	return nil
}
