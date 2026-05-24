package store

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestUpsertUserPreservesCreatedAt(t *testing.T) {
	s := NewLocalAdminStore()
	first, err := s.UpsertUser("alice", "$2a$10$fakehash1234567890abcdefghijklmnopqrstuvwxyz012345678", "admin")
	if err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}
	if first.CreatedAt == "" {
		t.Fatal("expected CreatedAt on first upsert")
	}

	time.Sleep(10 * time.Millisecond)

	second, err := s.UpsertUser("alice", "$2a$10$fakehash1234567890abcdefghijklmnopqrstuvwxyz012345678", "writer")
	if err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}
	if second.CreatedAt != first.CreatedAt {
		t.Fatalf("CreatedAt changed on upsert: first=%q second=%q", first.CreatedAt, second.CreatedAt)
	}
	if second.Role != "writer" {
		t.Fatalf("expected role to update to writer, got %q", second.Role)
	}
	if second.UpdatedAt == first.UpdatedAt {
		t.Fatal("expected UpdatedAt to change on re-upsert")
	}
}

func TestUpsertUserCorruptedCreatedAtFallsBack(t *testing.T) {
	s := NewLocalAdminStore()

	s.mu.Lock()
	s.users["bob"] = LocalAdminAccount{
		Username:     "bob",
		Role:         "admin",
		PasswordHash: "hash",
		CreatedAt:    "not-a-timestamp",
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	s.mu.Unlock()

	account, err := s.UpsertUser("bob", "$2a$10$fakehash1234567890abcdefghijklmnopqrstuvwxyz012345678", "admin")
	if err != nil {
		t.Fatalf("upsert with corrupted CreatedAt failed: %v", err)
	}

	if _, parseErr := time.Parse(time.RFC3339Nano, account.CreatedAt); parseErr != nil {
		t.Fatalf("expected valid CreatedAt after fallback, got %q: %v", account.CreatedAt, parseErr)
	}
}

func TestUpsertUserConcurrentSafe(t *testing.T) {
	s := NewLocalAdminStore()
	const goroutines = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := s.UpsertUser("concurrent-user", "$2a$10$fakehash1234567890abcdefghijklmnopqrstuvwxyz012345678", "admin")
			if err != nil {
				t.Errorf("concurrent upsert failed: %v", err)
			}
		}()
	}
	wg.Wait()

	account, ok, err := s.GetByUsername("concurrent-user")
	if err != nil {
		t.Fatalf("get after concurrent upserts failed: %v", err)
	}
	if !ok {
		t.Fatal("expected user to exist after concurrent upserts")
	}
	if account.Username != "concurrent-user" || account.Role != "admin" {
		t.Fatalf("unexpected account state: %+v", account)
	}
}

func TestCreateUserDuplicateReturnsError(t *testing.T) {
	s := NewLocalAdminStore()
	_, err := s.CreateUser("dup-user", "$2a$10$fakehash1234567890abcdefghijklmnopqrstuvwxyz012345678", "reader")
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	_, err = s.CreateUser("dup-user", "$2a$10$fakehash1234567890abcdefghijklmnopqrstuvwxyz012345678", "writer")
	if err != ErrAuthUserExists {
		t.Fatalf("expected ErrAuthUserExists on duplicate, got %v", err)
	}
}

func TestDeleteUserNotFound(t *testing.T) {
	s := NewLocalAdminStore()
	err := s.DeleteUser("ghost")
	if err != ErrAuthUserNotFound {
		t.Fatalf("expected ErrAuthUserNotFound, got %v", err)
	}
}

func TestDeleteLastAdminBlocked(t *testing.T) {
	s := NewLocalAdminStore()
	_, err := s.CreateUser("only-admin", "$2a$10$fakehash1234567890abcdefghijklmnopqrstuvwxyz012345678", "admin")
	if err != nil {
		t.Fatalf("create admin failed: %v", err)
	}
	err = s.DeleteUser("only-admin")
	if err != ErrAuthLastAdmin {
		t.Fatalf("expected ErrAuthLastAdmin, got %v", err)
	}
}

func TestTokenStoreDuplicateHashRejected(t *testing.T) {
	s := NewAPITokenStore()
	now := time.Now().UTC()
	_, err := s.Create("tok-a", "samehash", "reader", now)
	if err != nil {
		t.Fatalf("first token create failed: %v", err)
	}
	_, err = s.Create("tok-b", "samehash", "writer", now)
	if err != ErrAPITokenExists {
		t.Fatalf("expected ErrAPITokenExists for duplicate hash, got %v", err)
	}
}

func TestTokenStoreCreateA2ATokenRequiresScopes(t *testing.T) {
	s := NewAPITokenStore()
	_, err := s.CreateWithA2AAgentSystems("a2a-token", "hash-a2a", "a2a", nil, time.Now().UTC())
	if err == nil {
		t.Fatal("expected error for a2a token without scopes")
	}
}

func TestTokenStoreCreateA2ATokenStoresNormalizedScopes(t *testing.T) {
	s := NewAPITokenStore()
	now := time.Now().UTC()
	record, err := s.CreateWithA2AAgentSystems(
		"a2a-token",
		"hash-a2a",
		"a2a",
		[]string{" team-b/report ", "default/research", "team-b/report", ""},
		now,
	)
	if err != nil {
		t.Fatalf("create a2a token failed: %v", err)
	}
	want := []string{"default/research", "team-b/report"}
	if !reflect.DeepEqual(record.A2AAgentSystems, want) {
		t.Fatalf("unexpected scopes: got=%v want=%v", record.A2AAgentSystems, want)
	}

	got, ok, err := s.GetByHash("hash-a2a")
	if err != nil {
		t.Fatalf("get token by hash failed: %v", err)
	}
	if !ok {
		t.Fatal("expected token by hash")
	}
	if got.Role != "a2a" {
		t.Fatalf("expected role a2a, got %q", got.Role)
	}
	if !reflect.DeepEqual(got.A2AAgentSystems, want) {
		t.Fatalf("unexpected stored scopes: got=%v want=%v", got.A2AAgentSystems, want)
	}
}

func TestCreateUserRejectsA2ARole(t *testing.T) {
	s := NewLocalAdminStore()
	_, err := s.CreateUser("a2a-user", "$2a$10$fakehash1234567890abcdefghijklmnopqrstuvwxyz012345678", "a2a")
	if !errors.Is(err, ErrInvalidAuthRole) {
		t.Fatalf("expected ErrInvalidAuthRole for local a2a user, got %v", err)
	}
}

func TestTokenDeleteNotFoundError(t *testing.T) {
	s := NewAPITokenStore()
	err := s.Delete("nonexistent")
	if err != ErrAPITokenNotFound {
		t.Fatalf("expected ErrAPITokenNotFound, got %v", err)
	}
}
