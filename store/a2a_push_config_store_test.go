package store

import (
	"context"
	"errors"
	"testing"

	lf "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv/push"
)

func TestA2APushConfigStoreCRUDAndCopies(t *testing.T) {
	ctx := context.Background()
	store := NewA2APushConfigStore()
	input := &lf.PushConfig{
		TaskID: "ignored",
		URL:    "https://callbacks.example.com/a2a",
		Token:  "secret-token",
		Auth:   &lf.PushAuthInfo{Scheme: "Bearer", Credentials: "credential"},
	}

	saved, err := store.Save(ctx, "task-1", input)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.ID == "" || saved.TaskID != "task-1" {
		t.Fatalf("saved config = %#v", saved)
	}
	saved.URL = "https://mutated.example.com"

	got, err := store.Get(ctx, "task-1", saved.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.URL != input.URL {
		t.Fatalf("stored URL mutated through returned pointer: %q", got.URL)
	}

	list, err := store.List(ctx, "task-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].ID != saved.ID {
		t.Fatalf("List() = %#v", list)
	}

	if err := store.Delete(ctx, "task-1", saved.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(ctx, "task-1", saved.ID); !errors.Is(err, push.ErrPushConfigNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrPushConfigNotFound", err)
	}
}

func TestA2APushConfigStoreRejectsInvalidConfig(t *testing.T) {
	store := NewA2APushConfigStore()
	if _, err := store.Save(context.Background(), "", &lf.PushConfig{}); !errors.Is(err, lf.ErrInvalidParams) {
		t.Fatalf("Save() error = %v, want ErrInvalidParams", err)
	}
}
