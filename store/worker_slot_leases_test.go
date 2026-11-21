package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestWorkerSlotAndClaimRecovery(t *testing.T) {
	t.Run("memory", func(t *testing.T) { testWorkerSlotAndClaimRecovery(t, NewWorkerStore(), NewTaskStore()) })
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("ORLOJ_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("ORLOJ_POSTGRES_DSN is not set")
		}
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if err := EnsurePostgresSchema(db); err != nil {
			t.Fatal(err)
		}
		testWorkerSlotAndClaimRecovery(t, NewWorkerStoreWithDB(db), NewTaskStoreWithDB(db))
	})
}

func testWorkerSlotAndClaimRecovery(t *testing.T, workers *WorkerStore, tasks *TaskStore) {
	t.Helper()
	ctx := context.Background()
	id := "recovery-" + newUUID()
	_, err := workers.Upsert(ctx, resources.Worker{APIVersion: "orloj.dev/v1", Kind: "Worker", Metadata: resources.ObjectMeta{Name: id}, Spec: resources.WorkerSpec{MaxConcurrentTasks: 1}, Status: resources.WorkerStatus{Phase: "Ready", CurrentTasks: 1}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workers.Delete(context.Background(), id); _ = tasks.Delete(context.Background(), id) })
	// A legacy anonymous count must not prevent acquiring a new owned lease.
	_, old, ok, err := workers.AcquireLeasedSlot(ctx, id, 100*time.Millisecond)
	if err != nil || !ok || old == "" {
		t.Fatalf("acquire: token=%q acquired=%v err=%v", old, ok, err)
	}
	if _, _, ok, err := workers.AcquireLeasedSlot(ctx, id, time.Second); err != nil || ok {
		t.Fatalf("live slot not protected: acquired=%v err=%v", ok, err)
	}
	time.Sleep(150 * time.Millisecond)
	_, current, ok, err := workers.AcquireLeasedSlot(ctx, id, time.Second)
	if err != nil || !ok || current == old {
		t.Fatalf("recover: acquired=%v err=%v", ok, err)
	}
	if err := workers.RenewLeasedSlot(ctx, id, old, time.Second); err == nil {
		t.Fatal("old owner renewed slot")
	}
	if err := workers.ReleaseLeasedSlot(ctx, id, old); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := workers.AcquireLeasedSlot(ctx, id, time.Second); err != nil || ok {
		t.Fatalf("old release freed live slot: acquired=%v err=%v", ok, err)
	}
	if err := workers.ReleaseLeasedSlot(ctx, id, current); err != nil {
		t.Fatal(err)
	}
	worker, _, err := workers.Get(ctx, id)
	if err != nil || worker.Status.CurrentTasks != 0 {
		t.Fatalf("slot count: %d err=%v", worker.Status.CurrentTasks, err)
	}

	_, err = tasks.Upsert(ctx, resources.Task{APIVersion: "orloj.dev/v1", Kind: "Task", Metadata: resources.ObjectMeta{Name: id}, Spec: resources.TaskSpec{System: "sys"}})
	if err != nil {
		t.Fatal(err)
	}
	first, ok, err := tasks.ClaimIfDue(ctx, id, id, 100*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("first claim: %v %v", ok, err)
	}
	time.Sleep(150 * time.Millisecond)
	second, ok, err := tasks.ClaimIfDue(ctx, id, id, time.Second)
	if err != nil || !ok {
		t.Fatalf("same worker reclaim: %v %v", ok, err)
	}
	if first.Metadata.Annotations[TaskClaimAnnotation] == second.Metadata.Annotations[TaskClaimAnnotation] {
		t.Fatal("claim token reused")
	}
	if err := tasks.RenewClaimLease(ctx, id, id, first.Metadata.Annotations[TaskClaimAnnotation], time.Second); err == nil {
		t.Fatal("old claim renewed")
	}
	first.Status.Phase = "Succeeded"
	if _, err := tasks.UpsertClaim(ctx, first); err == nil {
		t.Fatal("old claim updated task")
	}
	if err := tasks.RenewClaimLease(ctx, id, id, second.Metadata.Annotations[TaskClaimAnnotation], time.Second); err != nil {
		t.Fatalf("current claim renewal: %v", err)
	}
}
