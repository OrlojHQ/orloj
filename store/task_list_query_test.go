package store

import (
	"context"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

func TestTaskStoreListQuerySortPhaseAndCreatedAt(t *testing.T) {
	s := NewTaskStore()
	ctx := context.Background()
	now := time.Now().UTC()

	seed := []resources.Task{
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Task",
			Metadata:   resources.ObjectMeta{Name: "alpha", CreatedAt: now.Add(-2 * time.Hour).Format(time.RFC3339Nano)},
			Spec:       resources.TaskSpec{System: "demo"},
			Status:     resources.TaskStatus{Phase: "Succeeded"},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Task",
			Metadata:   resources.ObjectMeta{Name: "bravo", CreatedAt: now.Add(-1 * time.Hour).Format(time.RFC3339Nano)},
			Spec:       resources.TaskSpec{System: "demo"},
			Status:     resources.TaskStatus{Phase: "Running"},
		},
		{
			APIVersion: "orloj.dev/v1",
			Kind:       "Task",
			Metadata:   resources.ObjectMeta{Name: "charlie", CreatedAt: now.Format(time.RFC3339Nano)},
			Spec:       resources.TaskSpec{System: "demo"},
			Status:     resources.TaskStatus{Phase: "Running"},
		},
	}
	for _, task := range seed {
		if _, err := s.Upsert(ctx, task); err != nil {
			t.Fatalf("upsert %s: %v", task.Metadata.Name, err)
		}
	}

	byCreated, err := s.ListQuery(ctx, TaskListOptions{Sort: "created_at", Order: "desc", Limit: 10})
	if err != nil {
		t.Fatalf("ListQuery created_at: %v", err)
	}
	if len(byCreated) != 3 || byCreated[0].Metadata.Name != "charlie" || byCreated[2].Metadata.Name != "alpha" {
		t.Fatalf("unexpected created_at desc order: %#v", namesOf(byCreated))
	}

	running, err := s.ListQuery(ctx, TaskListOptions{Phase: "Running", Sort: "name", Order: "asc", Limit: 10})
	if err != nil {
		t.Fatalf("ListQuery phase: %v", err)
	}
	if len(running) != 2 || running[0].Metadata.Name != "bravo" || running[1].Metadata.Name != "charlie" {
		t.Fatalf("unexpected phase filter result: %#v", namesOf(running))
	}

	page1, err := s.ListQuery(ctx, TaskListOptions{Sort: "created_at", Order: "desc", Limit: 1})
	if err != nil {
		t.Fatalf("ListQuery page1: %v", err)
	}
	if len(page1) != 1 || page1[0].Metadata.Name != "charlie" {
		t.Fatalf("unexpected page1: %#v", namesOf(page1))
	}
	page2, err := s.ListQuery(ctx, TaskListOptions{
		Sort:  "created_at",
		Order: "desc",
		Limit: 10,
		After: scopedNameFromMeta(page1[0].Metadata),
	})
	if err != nil {
		t.Fatalf("ListQuery page2: %v", err)
	}
	if len(page2) != 2 || page2[0].Metadata.Name != "bravo" {
		t.Fatalf("unexpected page2 after cursor: %#v", namesOf(page2))
	}
}

func namesOf(tasks []resources.Task) []string {
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, task.Metadata.Name)
	}
	return out
}
