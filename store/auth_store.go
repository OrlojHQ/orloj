package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrAuthUserExists     = errors.New("auth user already exists")
	ErrAuthUserNotFound   = errors.New("auth user not found")
	ErrAuthLastAdmin      = errors.New("cannot delete last admin user")
	ErrAPITokenExists     = errors.New("api token already exists")
	ErrAPITokenNotFound   = errors.New("api token not found")
	ErrInvalidAuthRole    = errors.New("invalid auth role")
	allowedAuthRoleValues = map[string]struct{}{
		"admin":      {},
		"writer":     {},
		"reader":     {},
		"controller": {},
	}
	allowedAPITokenRoleValues = map[string]struct{}{
		"admin":      {},
		"writer":     {},
		"reader":     {},
		"controller": {},
		"a2a":        {},
	}
)

func normalizeAuthRoleWithDefault(role, defaultRole string) (string, error) {
	r := strings.ToLower(strings.TrimSpace(role))
	if r == "" {
		r = strings.ToLower(strings.TrimSpace(defaultRole))
	}
	if _, ok := allowedAuthRoleValues[r]; !ok {
		return "", fmt.Errorf("%w: %q", ErrInvalidAuthRole, strings.TrimSpace(role))
	}
	return r, nil
}

func normalizeAuthRole(role string) (string, error) {
	return normalizeAuthRoleWithDefault(role, "")
}

func normalizeAPITokenRoleWithDefault(role, defaultRole string) (string, error) {
	r := strings.ToLower(strings.TrimSpace(role))
	if r == "" {
		r = strings.ToLower(strings.TrimSpace(defaultRole))
	}
	if _, ok := allowedAPITokenRoleValues[r]; !ok {
		return "", fmt.Errorf("%w: %q", ErrInvalidAuthRole, strings.TrimSpace(role))
	}
	return r, nil
}

func normalizeA2AAgentSystems(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func encodeA2AAgentSystems(values []string) string {
	raw, err := json.Marshal(normalizeA2AAgentSystems(values))
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func decodeA2AAgentSystems(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return normalizeA2AAgentSystems(values)
}

func normalizeLocalUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

type LocalAdminAccount struct {
	Username     string
	Role         string
	PasswordHash string
	CreatedAt    string
	UpdatedAt    string
}

type LocalAdminStore struct {
	mu    sync.RWMutex
	users map[string]LocalAdminAccount
	db    *sql.DB
}

func NewLocalAdminStore() *LocalAdminStore {
	return &LocalAdminStore{users: make(map[string]LocalAdminAccount)}
}

func NewLocalAdminStoreWithDB(db *sql.DB) *LocalAdminStore {
	return &LocalAdminStore{users: make(map[string]LocalAdminAccount), db: db}
}

func (s *LocalAdminStore) HasAdmin() (bool, error) {
	count, err := s.CountByRole("admin")
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *LocalAdminStore) CountUsers() (int, error) {
	if s.db != nil {
		var count int
		err := s.db.QueryRow(`SELECT COUNT(*) FROM auth_local_users`).Scan(&count)
		if err != nil {
			return 0, err
		}
		return count, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users), nil
}

func (s *LocalAdminStore) CountByRole(role string) (int, error) {
	role, err := normalizeAuthRole(role)
	if err != nil {
		return 0, err
	}
	if s.db != nil {
		var count int
		err := s.db.QueryRow(`SELECT COUNT(*) FROM auth_local_users WHERE role = $1`, role).Scan(&count)
		if err != nil {
			return 0, err
		}
		return count, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, user := range s.users {
		if strings.EqualFold(user.Role, role) {
			count++
		}
	}
	return count, nil
}

// Get returns one admin account for backward compatibility with legacy callers.
func (s *LocalAdminStore) Get() (LocalAdminAccount, bool, error) {
	if s.db != nil {
		var (
			account            LocalAdminAccount
			createdAt, updated time.Time
		)
		err := s.db.QueryRow(`
			SELECT username, role, password_hash, created_at, updated_at
			FROM auth_local_users
			WHERE role = 'admin'
			ORDER BY username ASC
			LIMIT 1`).Scan(
			&account.Username,
			&account.Role,
			&account.PasswordHash,
			&createdAt,
			&updated,
		)
		if err == sql.ErrNoRows {
			return LocalAdminAccount{}, false, nil
		}
		if err != nil {
			return LocalAdminAccount{}, false, err
		}
		account.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		account.UpdatedAt = updated.UTC().Format(time.RFC3339Nano)
		return account, true, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	admins := make([]LocalAdminAccount, 0)
	for _, user := range s.users {
		if strings.EqualFold(user.Role, "admin") {
			admins = append(admins, user)
		}
	}
	if len(admins) == 0 {
		return LocalAdminAccount{}, false, nil
	}
	sort.Slice(admins, func(i, j int) bool { return admins[i].Username < admins[j].Username })
	return admins[0], true, nil
}

func (s *LocalAdminStore) GetByUsername(username string) (LocalAdminAccount, bool, error) {
	username = normalizeLocalUsername(username)
	if username == "" {
		return LocalAdminAccount{}, false, nil
	}
	if s.db != nil {
		var (
			account            LocalAdminAccount
			createdAt, updated time.Time
		)
		err := s.db.QueryRow(`
			SELECT username, role, password_hash, created_at, updated_at
			FROM auth_local_users
			WHERE username = $1`,
			username,
		).Scan(&account.Username, &account.Role, &account.PasswordHash, &createdAt, &updated)
		if err == sql.ErrNoRows {
			return LocalAdminAccount{}, false, nil
		}
		if err != nil {
			return LocalAdminAccount{}, false, err
		}
		account.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		account.UpdatedAt = updated.UTC().Format(time.RFC3339Nano)
		return account, true, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	account, ok := s.users[username]
	if !ok {
		return LocalAdminAccount{}, false, nil
	}
	return account, true, nil
}

func (s *LocalAdminStore) ListUsers() ([]LocalAdminAccount, error) {
	if s.db != nil {
		rows, err := s.db.Query(`
			SELECT username, role, password_hash, created_at, updated_at
			FROM auth_local_users
			ORDER BY username ASC`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]LocalAdminAccount, 0)
		for rows.Next() {
			var (
				account            LocalAdminAccount
				createdAt, updated time.Time
			)
			if err := rows.Scan(&account.Username, &account.Role, &account.PasswordHash, &createdAt, &updated); err != nil {
				return nil, err
			}
			account.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
			account.UpdatedAt = updated.UTC().Format(time.RFC3339Nano)
			out = append(out, account)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return out, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LocalAdminAccount, 0, len(s.users))
	for _, user := range s.users {
		out = append(out, user)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out, nil
}

// Upsert keeps backward compatibility for legacy single-admin callers by
// forcing role=admin.
func (s *LocalAdminStore) Upsert(username, passwordHash string) error {
	_, err := s.UpsertUser(username, passwordHash, "admin")
	return err
}

func (s *LocalAdminStore) UpsertUser(username, passwordHash, role string) (LocalAdminAccount, error) {
	username = normalizeLocalUsername(username)
	passwordHash = strings.TrimSpace(passwordHash)
	if username == "" {
		return LocalAdminAccount{}, fmt.Errorf("username is required")
	}
	if passwordHash == "" {
		return LocalAdminAccount{}, fmt.Errorf("password hash is required")
	}
	r, err := normalizeAuthRoleWithDefault(role, "admin")
	if err != nil {
		return LocalAdminAccount{}, err
	}

	now := time.Now().UTC()
	if s.db != nil {
		_, err := s.db.Exec(
			`INSERT INTO auth_local_users(username, role, password_hash, created_at, updated_at)
			 VALUES($1, $2, $3, NOW(), NOW())
			 ON CONFLICT(username) DO UPDATE SET
				 role = EXCLUDED.role,
				 password_hash = EXCLUDED.password_hash,
				 updated_at = NOW()`,
			username,
			r,
			passwordHash,
		)
		if err != nil {
			return LocalAdminAccount{}, err
		}
		user, ok, err := s.GetByUsername(username)
		if err != nil {
			return LocalAdminAccount{}, err
		}
		if !ok {
			return LocalAdminAccount{}, fmt.Errorf("upserted user %q not found", username)
		}
		return user, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.users[username]
	createdAt := now
	if ok {
		if parsed, parseErr := time.Parse(time.RFC3339Nano, existing.CreatedAt); parseErr == nil {
			createdAt = parsed
		} else {
			log.Printf("WARNING: user %q has unparseable CreatedAt %q, falling back to current time", username, existing.CreatedAt)
		}
	}
	account := LocalAdminAccount{
		Username:     username,
		Role:         r,
		PasswordHash: passwordHash,
		CreatedAt:    createdAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:    now.Format(time.RFC3339Nano),
	}
	s.users[username] = account
	return account, nil
}

func (s *LocalAdminStore) CreateUser(username, passwordHash, role string) (LocalAdminAccount, error) {
	username = normalizeLocalUsername(username)
	passwordHash = strings.TrimSpace(passwordHash)
	if username == "" {
		return LocalAdminAccount{}, fmt.Errorf("username is required")
	}
	if passwordHash == "" {
		return LocalAdminAccount{}, fmt.Errorf("password hash is required")
	}
	r, err := normalizeAuthRoleWithDefault(role, "reader")
	if err != nil {
		return LocalAdminAccount{}, err
	}
	if s.db != nil {
		_, err := s.db.Exec(
			`INSERT INTO auth_local_users(username, role, password_hash, created_at, updated_at)
			 VALUES($1, $2, $3, NOW(), NOW())`,
			username,
			r,
			passwordHash,
		)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
				return LocalAdminAccount{}, ErrAuthUserExists
			}
			return LocalAdminAccount{}, err
		}
		user, ok, err := s.GetByUsername(username)
		if err != nil {
			return LocalAdminAccount{}, err
		}
		if !ok {
			return LocalAdminAccount{}, fmt.Errorf("created user %q not found", username)
		}
		return user, nil
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.users[username]; exists {
		return LocalAdminAccount{}, ErrAuthUserExists
	}
	account := LocalAdminAccount{
		Username:     username,
		Role:         r,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.users[username] = account
	return account, nil
}

// CreateFirstAdmin atomically creates the first admin account. It returns
// ErrAuthUserExists if any user already exists, preventing the TOCTOU race
// where concurrent setup requests both observe zero users.
func (s *LocalAdminStore) CreateFirstAdmin(username, passwordHash string) (LocalAdminAccount, error) {
	username = normalizeLocalUsername(username)
	passwordHash = strings.TrimSpace(passwordHash)
	if username == "" {
		return LocalAdminAccount{}, fmt.Errorf("username is required")
	}
	if passwordHash == "" {
		return LocalAdminAccount{}, fmt.Errorf("password hash is required")
	}

	if s.db != nil {
		// Atomic insert-if-no-users via INSERT ... WHERE NOT EXISTS.
		result, err := s.db.Exec(
			`INSERT INTO auth_local_users(username, role, password_hash, created_at, updated_at)
			 SELECT $1, $2, $3, NOW(), NOW()
			 WHERE NOT EXISTS (SELECT 1 FROM auth_local_users)`,
			username,
			"admin",
			passwordHash,
		)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
				return LocalAdminAccount{}, ErrAuthUserExists
			}
			return LocalAdminAccount{}, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return LocalAdminAccount{}, err
		}
		if rows == 0 {
			return LocalAdminAccount{}, ErrAuthUserExists
		}
		user, ok, err := s.GetByUsername(username)
		if err != nil {
			return LocalAdminAccount{}, err
		}
		if !ok {
			return LocalAdminAccount{}, fmt.Errorf("created admin %q not found", username)
		}
		return user, nil
	}

	// In-memory path: hold the write lock across check-and-insert.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.users) > 0 {
		return LocalAdminAccount{}, ErrAuthUserExists
	}
	account := LocalAdminAccount{
		Username:     username,
		Role:         "admin",
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.users[username] = account
	return account, nil
}

func (s *LocalAdminStore) SetPassword(username, passwordHash string) error {
	username = normalizeLocalUsername(username)
	passwordHash = strings.TrimSpace(passwordHash)
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if passwordHash == "" {
		return fmt.Errorf("password hash is required")
	}
	if s.db != nil {
		result, err := s.db.Exec(
			`UPDATE auth_local_users SET password_hash = $2, updated_at = NOW() WHERE username = $1`,
			username,
			passwordHash,
		)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err == nil && affected == 0 {
			return ErrAuthUserNotFound
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[username]
	if !ok {
		return ErrAuthUserNotFound
	}
	user.PasswordHash = passwordHash
	user.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.users[username] = user
	return nil
}

func (s *LocalAdminStore) DeleteUser(username string) error {
	username = normalizeLocalUsername(username)
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		var role string
		err = tx.QueryRow(`SELECT role FROM auth_local_users WHERE username = $1 FOR UPDATE`, username).Scan(&role)
		if err == sql.ErrNoRows {
			return ErrAuthUserNotFound
		}
		if err != nil {
			return err
		}
		if strings.EqualFold(strings.TrimSpace(role), "admin") {
			var adminCount int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM auth_local_users WHERE role = 'admin'`).Scan(&adminCount); err != nil {
				return err
			}
			if adminCount <= 1 {
				return ErrAuthLastAdmin
			}
		}
		if _, err := tx.Exec(`DELETE FROM auth_local_users WHERE username = $1`, username); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[username]
	if !ok {
		return ErrAuthUserNotFound
	}
	if strings.EqualFold(user.Role, "admin") {
		adminCount := 0
		for _, item := range s.users {
			if strings.EqualFold(item.Role, "admin") {
				adminCount++
			}
		}
		if adminCount <= 1 {
			return ErrAuthLastAdmin
		}
	}
	delete(s.users, username)
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
	username = normalizeLocalUsername(username)
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
	username = normalizeLocalUsername(username)
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

type APITokenRecord struct {
	Name            string
	Role            string
	TokenHash       string
	CreatedAt       string
	UpdatedAt       string
	A2AAgentSystems []string
}

type APITokenStore struct {
	mu     sync.RWMutex
	tokens map[string]APITokenRecord
	db     *sql.DB
}

func NewAPITokenStore() *APITokenStore {
	return &APITokenStore{tokens: make(map[string]APITokenRecord)}
}

func NewAPITokenStoreWithDB(db *sql.DB) *APITokenStore {
	return &APITokenStore{tokens: make(map[string]APITokenRecord), db: db}
}

func normalizeTokenName(name string) string {
	return strings.TrimSpace(name)
}

func (s *APITokenStore) Create(name, tokenHash, role string, now time.Time) (APITokenRecord, error) {
	return s.CreateWithA2AAgentSystems(name, tokenHash, role, nil, now)
}

func (s *APITokenStore) CreateWithA2AAgentSystems(name, tokenHash, role string, a2aAgentSystems []string, now time.Time) (APITokenRecord, error) {
	name = normalizeTokenName(name)
	tokenHash = strings.TrimSpace(tokenHash)
	if name == "" {
		return APITokenRecord{}, fmt.Errorf("name is required")
	}
	if tokenHash == "" {
		return APITokenRecord{}, fmt.Errorf("token hash is required")
	}
	r, err := normalizeAPITokenRoleWithDefault(role, "reader")
	if err != nil {
		return APITokenRecord{}, err
	}
	scopes := normalizeA2AAgentSystems(a2aAgentSystems)
	if r == "a2a" && len(scopes) == 0 {
		return APITokenRecord{}, fmt.Errorf("a2a_agent_systems is required for role %q", r)
	}
	if r != "a2a" {
		scopes = nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if s.db != nil {
		_, err := s.db.Exec(
			`INSERT INTO auth_api_tokens(name, token_hash, role, a2a_agent_systems, created_at, updated_at)
			 VALUES($1, $2, $3, $4, NOW(), NOW())`,
			name,
			tokenHash,
			r,
			encodeA2AAgentSystems(scopes),
		)
		if err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique") {
				return APITokenRecord{}, ErrAPITokenExists
			}
			return APITokenRecord{}, err
		}
		record, ok, err := s.Get(name)
		if err != nil {
			return APITokenRecord{}, err
		}
		if !ok {
			return APITokenRecord{}, fmt.Errorf("created token %q not found", name)
		}
		return record, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tokens[name]; exists {
		return APITokenRecord{}, ErrAPITokenExists
	}
	for _, existing := range s.tokens {
		if existing.TokenHash == tokenHash {
			return APITokenRecord{}, ErrAPITokenExists
		}
	}
	record := APITokenRecord{
		Name:            name,
		Role:            r,
		TokenHash:       tokenHash,
		CreatedAt:       now.UTC().Format(time.RFC3339Nano),
		UpdatedAt:       now.UTC().Format(time.RFC3339Nano),
		A2AAgentSystems: scopes,
	}
	s.tokens[name] = record
	return record, nil
}

func (s *APITokenStore) Get(name string) (APITokenRecord, bool, error) {
	name = normalizeTokenName(name)
	if name == "" {
		return APITokenRecord{}, false, nil
	}
	if s.db != nil {
		var (
			record             APITokenRecord
			createdAt, updated time.Time
			scopesJSON         string
		)
		err := s.db.QueryRow(
			`SELECT name, token_hash, role, a2a_agent_systems, created_at, updated_at
			 FROM auth_api_tokens
			 WHERE name = $1`,
			name,
		).Scan(&record.Name, &record.TokenHash, &record.Role, &scopesJSON, &createdAt, &updated)
		if err == sql.ErrNoRows {
			return APITokenRecord{}, false, nil
		}
		if err != nil {
			return APITokenRecord{}, false, err
		}
		record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		record.UpdatedAt = updated.UTC().Format(time.RFC3339Nano)
		record.A2AAgentSystems = decodeA2AAgentSystems(scopesJSON)
		return record, true, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.tokens[name]
	if !ok {
		return APITokenRecord{}, false, nil
	}
	return record, true, nil
}

func (s *APITokenStore) GetByHash(tokenHash string) (APITokenRecord, bool, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return APITokenRecord{}, false, nil
	}
	if s.db != nil {
		var (
			record             APITokenRecord
			createdAt, updated time.Time
			scopesJSON         string
		)
		err := s.db.QueryRow(
			`SELECT name, token_hash, role, a2a_agent_systems, created_at, updated_at
			 FROM auth_api_tokens
			 WHERE token_hash = $1`,
			tokenHash,
		).Scan(&record.Name, &record.TokenHash, &record.Role, &scopesJSON, &createdAt, &updated)
		if err == sql.ErrNoRows {
			return APITokenRecord{}, false, nil
		}
		if err != nil {
			return APITokenRecord{}, false, err
		}
		record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		record.UpdatedAt = updated.UTC().Format(time.RFC3339Nano)
		record.A2AAgentSystems = decodeA2AAgentSystems(scopesJSON)
		return record, true, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.tokens {
		if strings.EqualFold(record.TokenHash, tokenHash) {
			return record, true, nil
		}
	}
	return APITokenRecord{}, false, nil
}

func (s *APITokenStore) List() ([]APITokenRecord, error) {
	if s.db != nil {
		rows, err := s.db.Query(`
			SELECT name, token_hash, role, a2a_agent_systems, created_at, updated_at
			FROM auth_api_tokens
			ORDER BY name ASC`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]APITokenRecord, 0)
		for rows.Next() {
			var (
				record             APITokenRecord
				createdAt, updated time.Time
				scopesJSON         string
			)
			if err := rows.Scan(&record.Name, &record.TokenHash, &record.Role, &scopesJSON, &createdAt, &updated); err != nil {
				return nil, err
			}
			record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
			record.UpdatedAt = updated.UTC().Format(time.RFC3339Nano)
			record.A2AAgentSystems = decodeA2AAgentSystems(scopesJSON)
			out = append(out, record)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return out, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]APITokenRecord, 0, len(s.tokens))
	for _, record := range s.tokens {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *APITokenStore) HasAny() (bool, error) {
	if s.db != nil {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM auth_api_tokens`).Scan(&count); err != nil {
			return false, err
		}
		return count > 0, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tokens) > 0, nil
}

func (s *APITokenStore) Delete(name string) error {
	name = normalizeTokenName(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if s.db != nil {
		result, err := s.db.Exec(`DELETE FROM auth_api_tokens WHERE name = $1`, name)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err == nil && affected == 0 {
			return ErrAPITokenNotFound
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tokens[name]; !ok {
		return ErrAPITokenNotFound
	}
	delete(s.tokens, name)
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

// GenerateOpaqueCredential returns a random base64url credential suitable for
// bearer tokens or one-time bootstrap passwords.
func GenerateOpaqueCredential(size int) (string, error) {
	return randomToken(size)
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
