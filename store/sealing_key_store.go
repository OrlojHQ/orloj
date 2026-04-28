package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

type SealingKey struct {
	KeyID         string
	Status        string
	PublicKeyPEM  string
	PrivateKeyPEM string
	CreatedAt     string
}

type SealingKeyStore struct {
	mu            sync.RWMutex
	items         map[string]SealingKey
	db            *sql.DB
	encryptionKey []byte
}

func NewSealingKeyStore() *SealingKeyStore {
	return &SealingKeyStore{items: make(map[string]SealingKey)}
}

func NewSealingKeyStoreWithDB(db *sql.DB, key []byte) *SealingKeyStore {
	return &SealingKeyStore{items: make(map[string]SealingKey), db: db, encryptionKey: key}
}

func (s *SealingKeyStore) SetEncryptionKey(key []byte) { s.encryptionKey = key }

func (s *SealingKeyStore) GetActive(ctx context.Context) (SealingKey, bool, error) {
	if s.db != nil {
		var row SealingKey
		err := s.db.QueryRowContext(ctx,
			`SELECT key_id, status, public_key_pem, private_key_ciphertext, created_at
			 FROM sealing_keys WHERE status = 'active' LIMIT 1`,
		).Scan(&row.KeyID, &row.Status, &row.PublicKeyPEM, &row.PrivateKeyPEM, &row.CreatedAt)
		if err == sql.ErrNoRows {
			return SealingKey{}, false, nil
		}
		if err != nil {
			return SealingKey{}, false, err
		}
		if len(s.encryptionKey) == 0 {
			return SealingKey{}, false, fmt.Errorf("sealing key is configured but ORLOJ_SECRET_ENCRYPTION_KEY is not set")
		}
		privatePEM, err := decryptSecretValue(s.encryptionKey, row.PrivateKeyPEM, []byte("sealing-key:"+row.KeyID))
		if err != nil {
			return SealingKey{}, false, fmt.Errorf("decrypt sealing key %q: %w", row.KeyID, err)
		}
		row.PrivateKeyPEM = string(privatePEM)
		return row, true, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.items {
		if strings.EqualFold(item.Status, "active") {
			return item, true, nil
		}
	}
	return SealingKey{}, false, nil
}

func (s *SealingKeyStore) CreateActive(ctx context.Context, item SealingKey) (SealingKey, error) {
	item.KeyID = strings.TrimSpace(item.KeyID)
	item.Status = "active"
	item.PublicKeyPEM = strings.TrimSpace(item.PublicKeyPEM)
	item.PrivateKeyPEM = strings.TrimSpace(item.PrivateKeyPEM)
	if item.KeyID == "" {
		return SealingKey{}, fmt.Errorf("key_id is required")
	}
	if item.PublicKeyPEM == "" {
		return SealingKey{}, fmt.Errorf("public_key_pem is required")
	}
	if item.PrivateKeyPEM == "" {
		return SealingKey{}, fmt.Errorf("private_key_pem is required")
	}
	if strings.TrimSpace(item.CreatedAt) == "" {
		item.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	if s.db != nil {
		if len(s.encryptionKey) == 0 {
			return SealingKey{}, fmt.Errorf("ORLOJ_SECRET_ENCRYPTION_KEY is required to store sealing keys")
		}
		ciphertext, err := encryptSecretValue(s.encryptionKey, []byte(item.PrivateKeyPEM), []byte("sealing-key:"+item.KeyID))
		if err != nil {
			return SealingKey{}, err
		}
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO sealing_keys(key_id, status, public_key_pem, private_key_ciphertext, created_at)
			 VALUES($1, 'active', $2, $3, NOW())`,
			item.KeyID, item.PublicKeyPEM, ciphertext,
		)
		if err != nil {
			if isDuplicateStoreErr(err) {
				return SealingKey{}, fmt.Errorf("create active sealing key %q: %w", item.KeyID, ErrResourceAlreadyExists)
			}
			return SealingKey{}, err
		}
		return item, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.items {
		if strings.EqualFold(existing.Status, "active") {
			return SealingKey{}, fmt.Errorf("create active sealing key %q: %w", item.KeyID, ErrResourceAlreadyExists)
		}
	}
	s.items[item.KeyID] = item
	return item, nil
}

func (s *SealingKeyStore) EnsureActive(ctx context.Context) (SealingKey, error) {
	active, ok, err := s.GetActive(ctx)
	if err != nil {
		return SealingKey{}, err
	}
	if ok {
		return active, nil
	}
	material, err := resources.GenerateSealingKeyMaterial()
	if err != nil {
		return SealingKey{}, err
	}
	created, err := s.CreateActive(ctx, SealingKey{
		KeyID:         material.KeyID,
		Status:        "active",
		PublicKeyPEM:  material.PublicKeyPEM,
		PrivateKeyPEM: material.PrivateKeyPEM,
	})
	if err == nil {
		return created, nil
	}
	if !errorsIsResourceAlreadyExists(err) {
		return SealingKey{}, err
	}
	active, ok, retryErr := s.GetActive(ctx)
	if retryErr != nil {
		return SealingKey{}, retryErr
	}
	if !ok {
		return SealingKey{}, err
	}
	return active, nil
}

func isDuplicateStoreErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint")
}

func errorsIsResourceAlreadyExists(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrResourceAlreadyExists.Error())
}
