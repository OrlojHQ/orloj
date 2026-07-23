package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	lf "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv/push"

	"github.com/OrlojHQ/orloj/resources"
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

	saved, err := store.SaveForTask(ctx, "team/internal-1", "task-1", input)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.ID == "" || saved.TaskID != "task-1" {
		t.Fatalf("saved config = %#v", saved)
	}
	saved.URL = "https://mutated.example.com"

	got, err := store.GetForTask(ctx, "team/internal-1", "task-1", saved.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.URL != input.URL {
		t.Fatalf("stored URL mutated through returned pointer: %q", got.URL)
	}

	list, err := store.ListForTask(ctx, "team/internal-1", "task-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].ID != saved.ID {
		t.Fatalf("List() = %#v", list)
	}

	if err := store.DeleteForTask(ctx, "team/internal-1", "task-1", saved.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.GetForTask(ctx, "team/internal-1", "task-1", saved.ID); !errors.Is(err, push.ErrPushConfigNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrPushConfigNotFound", err)
	}
}

func TestA2APushConfigStoreRejectsInvalidConfig(t *testing.T) {
	store := NewA2APushConfigStore()
	if _, err := store.SaveForTask(context.Background(), "", "", &lf.PushConfig{}); !errors.Is(err, lf.ErrInvalidParams) {
		t.Fatalf("Save() error = %v, want ErrInvalidParams", err)
	}
}

func TestA2APushConfigStoreScopesDuplicateExternalTaskIDs(t *testing.T) {
	ctx := context.Background()
	configs := NewA2APushConfigStore()
	first, err := configs.SaveForTask(ctx, "red/internal", "shared", &lf.PushConfig{
		URL: "https://red.example.com/a2a",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := configs.SaveForTask(ctx, "blue/internal", "shared", &lf.PushConfig{
		ID:  first.ID,
		URL: "https://blue.example.com/a2a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("config IDs differ: %q != %q", second.ID, first.ID)
	}
	red, err := configs.GetForTask(ctx, "red/internal", "shared", first.ID)
	if err != nil || red.URL != "https://red.example.com/a2a" {
		t.Fatalf("red config = %#v, err = %v", red, err)
	}
	blue, err := configs.GetForTask(ctx, "blue/internal", "shared", first.ID)
	if err != nil || blue.URL != "https://blue.example.com/a2a" {
		t.Fatalf("blue config = %#v, err = %v", blue, err)
	}
}

func TestA2APushConfigStorePostgresScopesEncryptedRowsAndCascades(t *testing.T) {
	db := openPostgresForStoreTest(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	redKey := fmt.Sprintf("red/a2a-red-%d", suffix)
	blueKey := fmt.Sprintf("blue/a2a-blue-%d", suffix)
	tasks := NewTaskStoreWithDB(db)
	for _, task := range []resources.Task{
		{
			Metadata: resources.ObjectMeta{
				Name:      strings.TrimPrefix(redKey, "red/"),
				Namespace: "red",
				Labels:    map[string]string{"orloj.dev/a2a-task-id": "shared"},
			},
			Spec: resources.TaskSpec{System: "system", Input: map[string]string{"prompt": "red"}},
		},
		{
			Metadata: resources.ObjectMeta{
				Name:      strings.TrimPrefix(blueKey, "blue/"),
				Namespace: "blue",
				Labels:    map[string]string{"orloj.dev/a2a-task-id": "shared"},
			},
			Spec: resources.TaskSpec{System: "system", Input: map[string]string{"prompt": "blue"}},
		},
	} {
		if _, err := tasks.Upsert(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = tasks.Delete(ctx, redKey)
		_ = tasks.Delete(ctx, blueKey)
	})

	configs := NewA2APushConfigStoreWithDB(db, []byte("0123456789abcdef0123456789abcdef"))
	red, err := configs.SaveForTask(ctx, redKey, "shared", &lf.PushConfig{
		ID:  "same-config",
		URL: "https://red.example.com/a2a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := configs.SaveForTask(ctx, blueKey, "shared", &lf.PushConfig{
		ID:  red.ID,
		URL: "https://blue.example.com/a2a",
	}); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Delete(ctx, redKey); err != nil {
		t.Fatal(err)
	}
	if _, err := configs.GetForTask(ctx, redKey, "shared", red.ID); !errors.Is(err, push.ErrPushConfigNotFound) {
		t.Fatalf("red config survived task deletion: %v", err)
	}
	blue, err := configs.GetForTask(ctx, blueKey, "shared", red.ID)
	if err != nil || blue.URL != "https://blue.example.com/a2a" {
		t.Fatalf("blue config = %#v, err = %v", blue, err)
	}
}
