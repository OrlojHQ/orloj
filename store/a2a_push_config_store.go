package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	lf "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv/push"
	"github.com/google/uuid"
)

// A2APushConfigStore persists outbound A2A callback configuration separately
// from Orloj's inbound TaskWebhook resources.
type A2APushConfigStore struct {
	mu            sync.RWMutex
	items         map[lf.TaskID]map[string]*lf.PushConfig
	db            *sql.DB
	encryptionKey []byte
}

var _ push.ConfigStore = (*A2APushConfigStore)(nil)

func NewA2APushConfigStore() *A2APushConfigStore {
	return &A2APushConfigStore{items: make(map[lf.TaskID]map[string]*lf.PushConfig)}
}

func NewA2APushConfigStoreWithDB(db *sql.DB, encryptionKey []byte) *A2APushConfigStore {
	return &A2APushConfigStore{
		items:         make(map[lf.TaskID]map[string]*lf.PushConfig),
		db:            db,
		encryptionKey: append([]byte(nil), encryptionKey...),
	}
}

func (s *A2APushConfigStore) Save(ctx context.Context, taskID lf.TaskID, config *lf.PushConfig) (*lf.PushConfig, error) {
	if taskID == "" || config == nil || strings.TrimSpace(config.URL) == "" {
		return nil, lf.ErrInvalidParams
	}
	item, err := clonePushConfig(config)
	if err != nil {
		return nil, err
	}
	if item.ID == "" {
		item.ID = uuid.Must(uuid.NewV7()).String()
	}
	item.TaskID = taskID

	if s.db != nil {
		payload, err := s.encode(item)
		if err != nil {
			return nil, err
		}
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO a2a_push_configs(task_id, config_id, payload, updated_at)
			 VALUES($1, $2, $3, NOW())
			 ON CONFLICT(task_id, config_id)
			 DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`,
			string(taskID), item.ID, payload,
		)
		if err != nil {
			return nil, fmt.Errorf("save A2A push config: %w", err)
		}
		return clonePushConfig(item)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items[taskID] == nil {
		s.items[taskID] = make(map[string]*lf.PushConfig)
	}
	s.items[taskID][item.ID] = item
	return clonePushConfig(item)
}

func (s *A2APushConfigStore) Get(ctx context.Context, taskID lf.TaskID, configID string) (*lf.PushConfig, error) {
	if s.db != nil {
		var payload string
		err := s.db.QueryRowContext(ctx,
			`SELECT payload FROM a2a_push_configs WHERE task_id = $1 AND config_id = $2`,
			string(taskID), configID,
		).Scan(&payload)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, push.ErrPushConfigNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("get A2A push config: %w", err)
		}
		return s.decode(taskID, configID, payload)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item := s.items[taskID][configID]
	if item == nil {
		return nil, push.ErrPushConfigNotFound
	}
	return clonePushConfig(item)
}

func (s *A2APushConfigStore) List(ctx context.Context, taskID lf.TaskID) ([]*lf.PushConfig, error) {
	if s.db != nil {
		rows, err := s.db.QueryContext(ctx,
			`SELECT config_id, payload FROM a2a_push_configs WHERE task_id = $1 ORDER BY config_id`,
			string(taskID),
		)
		if err != nil {
			return nil, fmt.Errorf("list A2A push configs: %w", err)
		}
		defer rows.Close()
		result := make([]*lf.PushConfig, 0)
		for rows.Next() {
			var configID, payload string
			if err := rows.Scan(&configID, &payload); err != nil {
				return nil, err
			}
			item, err := s.decode(taskID, configID, payload)
			if err != nil {
				return nil, err
			}
			result = append(result, item)
		}
		return result, rows.Err()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*lf.PushConfig, 0, len(s.items[taskID]))
	for _, item := range s.items[taskID] {
		copy, err := clonePushConfig(item)
		if err != nil {
			return nil, err
		}
		result = append(result, copy)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (s *A2APushConfigStore) Delete(ctx context.Context, taskID lf.TaskID, configID string) error {
	if s.db != nil {
		_, err := s.db.ExecContext(ctx,
			`DELETE FROM a2a_push_configs WHERE task_id = $1 AND config_id = $2`,
			string(taskID), configID,
		)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items[taskID], configID)
	if len(s.items[taskID]) == 0 {
		delete(s.items, taskID)
	}
	return nil
}

func (s *A2APushConfigStore) DeleteAll(ctx context.Context, taskID lf.TaskID) error {
	if s.db != nil {
		_, err := s.db.ExecContext(ctx, `DELETE FROM a2a_push_configs WHERE task_id = $1`, string(taskID))
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, taskID)
	return nil
}

func (s *A2APushConfigStore) encode(item *lf.PushConfig) (string, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return "", err
	}
	if len(s.encryptionKey) == 0 {
		return string(raw), nil
	}
	return encryptSecretValue(s.encryptionKey, raw, pushConfigAAD(item.TaskID, item.ID))
}

func (s *A2APushConfigStore) decode(taskID lf.TaskID, configID, payload string) (*lf.PushConfig, error) {
	raw := []byte(payload)
	if strings.HasPrefix(payload, encryptedPrefix) {
		if len(s.encryptionKey) == 0 {
			return nil, errors.New("A2A push config is encrypted but no secret encryption key is configured")
		}
		var err error
		raw, err = decryptSecretValue(s.encryptionKey, payload, pushConfigAAD(taskID, configID))
		if err != nil {
			return nil, fmt.Errorf("decrypt A2A push config: %w", err)
		}
	}
	var item lf.PushConfig
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, fmt.Errorf("decode A2A push config: %w", err)
	}
	return &item, nil
}

func pushConfigAAD(taskID lf.TaskID, configID string) []byte {
	return []byte("a2a-push-config:" + string(taskID) + ":" + configID)
}

func clonePushConfig(item *lf.PushConfig) (*lf.PushConfig, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	var copy lf.PushConfig
	if err := json.Unmarshal(raw, &copy); err != nil {
		return nil, err
	}
	return &copy, nil
}
