package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/OrlojHQ/orloj/resources"
)

const tableSealedSecrets = "sealed_secrets"

type SealedSecretStore struct {
	mu    sync.RWMutex
	items map[string]resources.SealedSecret
	db    *sql.DB
}

func NewSealedSecretStore() *SealedSecretStore {
	return &SealedSecretStore{items: make(map[string]resources.SealedSecret)}
}

func NewSealedSecretStoreWithDB(db *sql.DB) *SealedSecretStore {
	return &SealedSecretStore{items: make(map[string]resources.SealedSecret), db: db}
}

func (s *SealedSecretStore) Upsert(ctx context.Context, item resources.SealedSecret) (resources.SealedSecret, error) {
	if err := item.Normalize(); err != nil {
		return resources.SealedSecret{}, err
	}
	key := scopedNameFromMeta(item.Metadata)
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.SealedSecret{}, err
		}
		defer tx.Rollback()

		existing, found, err := getFromTableForUpdate[resources.SealedSecret](ctx, tx, tableSealedSecrets, key)
		if err != nil {
			return resources.SealedSecret{}, err
		}
		if !found {
			if err := initializeCreateMetadata("SealedSecret", &item.Metadata); err != nil {
				return resources.SealedSecret{}, err
			}
		} else {
			specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
			if err := initializeUpdateMetadata("SealedSecret", &item.Metadata, existing.Metadata, specChanged); err != nil {
				return resources.SealedSecret{}, err
			}
		}
		if err := upsertSealedSecretSQL(ctx, tx, key, item); err != nil {
			return resources.SealedSecret{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.SealedSecret{}, err
		}
		return item, nil
	}

	s.mu.Lock()
	existing, found := s.items[key]
	if !found {
		if err := initializeCreateMetadata("SealedSecret", &item.Metadata); err != nil {
			s.mu.Unlock()
			return resources.SealedSecret{}, err
		}
	} else {
		specChanged := !reflect.DeepEqual(existing.Spec, item.Spec)
		if err := initializeUpdateMetadata("SealedSecret", &item.Metadata, existing.Metadata, specChanged); err != nil {
			s.mu.Unlock()
			return resources.SealedSecret{}, err
		}
	}
	s.items[key] = item
	s.mu.Unlock()
	return item, nil
}

func (s *SealedSecretStore) Get(ctx context.Context, name string) (resources.SealedSecret, bool, error) {
	key := normalizeLookupName(name)
	if s.db != nil {
		return getFromTable[resources.SealedSecret](ctx, s.db, tableSealedSecrets, key)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	return item, ok, nil
}

func (s *SealedSecretStore) List(ctx context.Context) ([]resources.SealedSecret, error) {
	if s.db != nil {
		return listFromTable[resources.SealedSecret](ctx, s.db, tableSealedSecrets)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.SealedSecret, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out, nil
}

func (s *SealedSecretStore) ListCursor(ctx context.Context, limit int, after, namespace string) ([]resources.SealedSecret, error) {
	if s.db != nil {
		return listFromTableCursor[resources.SealedSecret](ctx, s.db, tableSealedSecrets, limit, after, namespace)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]resources.SealedSecret, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return cursorFilter(out,
		func(a resources.SealedSecret) string { return a.Metadata.Name },
		func(a resources.SealedSecret) string { return resources.NormalizeNamespace(a.Metadata.Namespace) },
		limit, after, namespace,
	), nil
}

func (s *SealedSecretStore) Delete(ctx context.Context, name string) error {
	key := normalizeLookupName(name)
	if s.db != nil {
		deleted, err := deleteFromTable(ctx, s.db, tableSealedSecrets, key)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("sealedsecret %q not found", name)
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; !ok {
		return fmt.Errorf("sealedsecret %q not found", name)
	}
	delete(s.items, key)
	return nil
}

func (s *SealedSecretStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.SealedSecret) (resources.SealedSecret, error) {
	return jsonPayloadMovingKey(ctx, s.db, s.items, &s.mu, oldStoreKey, item,
		"SealedSecret", tableSealedSecrets,
		func(it resources.SealedSecret) error { return it.Normalize() },
		func(it resources.SealedSecret) string { return scopedNameFromMeta(it.Metadata) },
		func(it resources.SealedSecret) any { return it.Spec },
		upsertSealedSecretSQL,
		func(it *resources.SealedSecret) *resources.ObjectMeta { return &it.Metadata },
		func(ctx context.Context, it resources.SealedSecret) (resources.SealedSecret, error) {
			return s.Upsert(ctx, it)
		},
	)
}

func upsertSealedSecretSQL(ctx context.Context, db dbExecer, name string, item resources.SealedSecret) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO sealed_secrets(name, namespace, status_phase, payload, updated_at)
		 VALUES($1, $2, $3, $4, NOW())
		 ON CONFLICT (name) DO UPDATE SET
		   namespace = EXCLUDED.namespace,
		   status_phase = EXCLUDED.status_phase,
		   payload = EXCLUDED.payload,
		   updated_at = NOW()`,
		name, resources.NormalizeNamespace(item.Metadata.Namespace), item.Status.Phase, payload)
	return err
}
