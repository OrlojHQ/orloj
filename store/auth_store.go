package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	tableAuthLocalAdmin = "auth_local_admin"
	tableAuthSessions   = "auth_sessions"
)

type LocalAdminAccount struct {
	Username     string
	PasswordHash string
	UpdatedAt    string
}

type LocalAdminStore struct {
	mu      sync.RWMutex
	account *LocalAdminAccount
	db      *sql.DB
}

func NewLocalAdminStore() *LocalAdminStore {
	return &LocalAdminStore{}
}

func NewLocalAdminStoreWithDB(db *sql.DB) *LocalAdminStore {
	return &LocalAdminStore{db: db}
}

func (s *LocalAdminStore) HasAdmin() (bool, error) {
	_, ok, err := s.Get()
	return ok, err
}

func (s *LocalAdminStore) Get() (LocalAdminAccount, bool, error) {
	if s.db != nil {
		var (
			account   LocalAdminAccount
			updatedAt time.Time
		)
		err := s.db.QueryRow(`SELECT username, password_hash, updated_at FROM auth_local_admin WHERE id = 1`).Scan(
			&account.Username,
			&account.PasswordHash,
			&updatedAt,
		)
		if err == sql.ErrNoRows {
			return LocalAdminAccount{}, false, nil
		}
		if err != nil {
			return LocalAdminAccount{}, false, err
		}
		account.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
		return account, true, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.account == nil {
		return LocalAdminAccount{}, false, nil
	}
	return *s.account, true, nil
}

func (s *LocalAdminStore) Upsert(username, passwordHash string) error {
	username = strings.TrimSpace(username)
	passwordHash = strings.TrimSpace(passwordHash)
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if passwordHash == "" {
		return fmt.Errorf("password hash is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if s.db != nil {
		_, err := s.db.Exec(
			`INSERT INTO auth_local_admin(id, username, password_hash, updated_at)
			 VALUES(1, $1, $2, NOW())
			 ON CONFLICT(id) DO UPDATE SET
				 username = EXCLUDED.username,
				 password_hash = EXCLUDED.password_hash,
				 updated_at = NOW()`,
			username,
			passwordHash,
		)
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.account = &LocalAdminAccount{
		Username:     username,
		PasswordHash: passwordHash,
		UpdatedAt:    now,
	}
	return nil
}

type AuthSession struct {
	ID        string
	Username  string
	CreatedAt string
	LastSeen  string
	ExpiresAt string
}

type AuthSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]AuthSession
	db       *sql.DB
}

func NewAuthSessionStore() *AuthSessionStore {
	return &AuthSessionStore{sessions: make(map[string]AuthSession)}
}

func NewAuthSessionStoreWithDB(db *sql.DB) *AuthSessionStore {
	return &AuthSessionStore{sessions: make(map[string]AuthSession), db: db}
}

func (s *AuthSessionStore) Create(username string, ttl time.Duration, now time.Time) (AuthSession, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return AuthSession{}, fmt.Errorf("username is required")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	token, err := randomToken(32)
	if err != nil {
		return AuthSession{}, err
	}
	hashed := hashSessionToken(token)
	createdAt := now.UTC()
	expiresAt := createdAt.Add(ttl)
	session := AuthSession{
		ID:        token,
		Username:  username,
		CreatedAt: createdAt.Format(time.RFC3339Nano),
		LastSeen:  createdAt.Format(time.RFC3339Nano),
		ExpiresAt: expiresAt.Format(time.RFC3339Nano),
	}
	if s.db != nil {
		_, err := s.db.Exec(
			`INSERT INTO auth_sessions(session_id_hash, username, created_at, last_seen_at, expires_at)
			 VALUES($1, $2, $3, $4, $5)
			 ON CONFLICT(session_id_hash) DO UPDATE SET
				 username = EXCLUDED.username,
				 last_seen_at = EXCLUDED.last_seen_at,
				 expires_at = EXCLUDED.expires_at`,
			hashed,
			username,
			createdAt,
			createdAt,
			expiresAt,
		)
		if err != nil {
			return AuthSession{}, err
		}
		return session, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[hashed] = session
	return session, nil
}

func (s *AuthSessionStore) Get(token string) (AuthSession, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return AuthSession{}, false, nil
	}
	hashed := hashSessionToken(token)
	if s.db != nil {
		var (
			out       AuthSession
			createdAt time.Time
			lastSeen  time.Time
			expiresAt time.Time
		)
		err := s.db.QueryRow(
			`SELECT username, created_at, last_seen_at, expires_at
			 FROM auth_sessions
			 WHERE session_id_hash = $1`,
			hashed,
		).Scan(&out.Username, &createdAt, &lastSeen, &expiresAt)
		if err == sql.ErrNoRows {
			return AuthSession{}, false, nil
		}
		if err != nil {
			return AuthSession{}, false, err
		}
		out.ID = token
		out.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		out.LastSeen = lastSeen.UTC().Format(time.RFC3339Nano)
		out.ExpiresAt = expiresAt.UTC().Format(time.RFC3339Nano)
		return out, true, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.sessions[hashed]
	if !ok {
		return AuthSession{}, false, nil
	}
	item.ID = token
	return item, true, nil
}

func (s *AuthSessionStore) Touch(token string, ttl time.Duration, now time.Time) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	hashed := hashSessionToken(token)
	lastSeen := now.UTC()
	expiresAt := lastSeen.Add(ttl)
	if s.db != nil {
		_, err := s.db.Exec(
			`UPDATE auth_sessions
			 SET last_seen_at = $2,
				 expires_at = $3
			 WHERE session_id_hash = $1`,
			hashed,
			lastSeen,
			expiresAt,
		)
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[hashed]
	if !ok {
		return nil
	}
	session.LastSeen = lastSeen.Format(time.RFC3339Nano)
	session.ExpiresAt = expiresAt.Format(time.RFC3339Nano)
	s.sessions[hashed] = session
	return nil
}

func (s *AuthSessionStore) Delete(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	hashed := hashSessionToken(token)
	if s.db != nil {
		_, err := s.db.Exec(`DELETE FROM auth_sessions WHERE session_id_hash = $1`, hashed)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, hashed)
	return nil
}

func (s *AuthSessionStore) DeleteByUsername(username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	if s.db != nil {
		_, err := s.db.Exec(`DELETE FROM auth_sessions WHERE username = $1`, username)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, session := range s.sessions {
		if strings.EqualFold(strings.TrimSpace(session.Username), username) {
			delete(s.sessions, key)
		}
	}
	return nil
}

func (s *AuthSessionStore) DeleteExpired(now time.Time) error {
	cutoff := now.UTC()
	if s.db != nil {
		_, err := s.db.Exec(`DELETE FROM auth_sessions WHERE expires_at <= $1`, cutoff)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, session := range s.sessions {
		exp, err := time.Parse(time.RFC3339Nano, session.ExpiresAt)
		if err != nil || !exp.After(cutoff) {
			delete(s.sessions, key)
		}
	}
	return nil
}

func randomToken(size int) (string, error) {
	if size <= 0 {
		size = 32
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
