package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresTaskStoreClaimNextDueFiltersWorkerHints(t *testing.T) {
	db := openPostgresForStoreTest(t)
	defer db.Close()

	ctx := context.Background()
	taskStore := NewTaskStoreWithDB(db)
	workerID := "worker-a"

	testCases := []struct {
		name         string
		hints        WorkerClaimHints
		mismatchTask func(int) resources.Task
		matchTask    resources.Task
		matches      func(resources.Task) bool
		wantName     string
	}{
		{
			name:  "assigned_worker",
			hints: WorkerClaimHints{AssignedWorker: workerID},
			mismatchTask: func(i int) resources.Task {
				return resources.Task{
					APIVersion: "orloj.dev/v1",
					Kind:       "Task",
					Metadata:   resources.ObjectMeta{Name: fmt.Sprintf("assigned-worker-mismatch-%02d", i)},
					Status:     resources.TaskStatus{AssignedWorker: "worker-b"},
				}
			},
			matchTask: resources.Task{
				APIVersion: "orloj.dev/v1",
				Kind:       "Task",
				Metadata:   resources.ObjectMeta{Name: "assigned-worker-match"},
				Status:     resources.TaskStatus{AssignedWorker: workerID},
			},
			matches: func(task resources.Task) bool {
				assigned := strings.TrimSpace(task.Status.AssignedWorker)
				return assigned == "" || strings.EqualFold(assigned, workerID)
			},
			wantName: "assigned-worker-match",
		},
		{
			name:  "region",
			hints: WorkerClaimHints{Region: "us-west"},
			mismatchTask: func(i int) resources.Task {
				return resources.Task{
					APIVersion: "orloj.dev/v1",
					Kind:       "Task",
					Metadata:   resources.ObjectMeta{Name: fmt.Sprintf("region-mismatch-%02d", i)},
					Spec:       resources.TaskSpec{Requirements: resources.TaskRequirements{Region: "us-east"}},
				}
			},
			matchTask: resources.Task{
				APIVersion: "orloj.dev/v1",
				Kind:       "Task",
				Metadata:   resources.ObjectMeta{Name: "region-match"},
				Spec:       resources.TaskSpec{Requirements: resources.TaskRequirements{Region: "us-west"}},
			},
			matches: func(task resources.Task) bool {
				region := strings.TrimSpace(task.Spec.Requirements.Region)
				return region == "" || strings.EqualFold(region, "us-west")
			},
			wantName: "region-match",
		},
		{
			name:  "gpu",
			hints: WorkerClaimHints{RequiresGPU: true},
			mismatchTask: func(i int) resources.Task {
				return resources.Task{
					APIVersion: "orloj.dev/v1",
					Kind:       "Task",
					Metadata:   resources.ObjectMeta{Name: fmt.Sprintf("gpu-mismatch-%02d", i)},
					Spec:       resources.TaskSpec{Requirements: resources.TaskRequirements{GPU: false}},
				}
			},
			matchTask: resources.Task{
				APIVersion: "orloj.dev/v1",
				Kind:       "Task",
				Metadata:   resources.ObjectMeta{Name: "gpu-match"},
				Spec:       resources.TaskSpec{Requirements: resources.TaskRequirements{GPU: true}},
			},
			matches: func(task resources.Task) bool {
				return task.Spec.Requirements.GPU
			},
			wantName: "gpu-match",
		},
		{
			name:  "supported_model",
			hints: WorkerClaimHints{SupportedModels: []string{"gpt-4o"}},
			mismatchTask: func(i int) resources.Task {
				return resources.Task{
					APIVersion: "orloj.dev/v1",
					Kind:       "Task",
					Metadata:   resources.ObjectMeta{Name: fmt.Sprintf("model-mismatch-%02d", i)},
					Spec:       resources.TaskSpec{Requirements: resources.TaskRequirements{Model: "gpt-4.1"}},
				}
			},
			matchTask: resources.Task{
				APIVersion: "orloj.dev/v1",
				Kind:       "Task",
				Metadata:   resources.ObjectMeta{Name: "model-match"},
				Spec:       resources.TaskSpec{Requirements: resources.TaskRequirements{Model: "gpt-4o"}},
			},
			matches: func(task resources.Task) bool {
				model := strings.TrimSpace(task.Spec.Requirements.Model)
				return model == "" || strings.EqualFold(model, "gpt-4o")
			},
			wantName: "model-match",
		},
		{
			name:  "supported_model_allows_unset",
			hints: WorkerClaimHints{SupportedModels: []string{"gpt-4o"}},
			mismatchTask: func(i int) resources.Task {
				return resources.Task{
					APIVersion: "orloj.dev/v1",
					Kind:       "Task",
					Metadata:   resources.ObjectMeta{Name: fmt.Sprintf("unset-model-mismatch-%02d", i)},
					Spec:       resources.TaskSpec{Requirements: resources.TaskRequirements{Model: "gpt-4.1"}},
				}
			},
			matchTask: resources.Task{
				APIVersion: "orloj.dev/v1",
				Kind:       "Task",
				Metadata:   resources.ObjectMeta{Name: "unset-model-match"},
			},
			matches: func(task resources.Task) bool {
				model := strings.TrimSpace(task.Spec.Requirements.Model)
				return model == "" || strings.EqualFold(model, "gpt-4o")
			},
			wantName: "unset-model-match",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resetPostgresStoreTestTables(t, db)

			for i := 0; i < 64; i++ {
				if _, err := taskStore.Upsert(ctx, tc.mismatchTask(i)); err != nil {
					t.Fatalf("upsert mismatch task %d failed: %v", i, err)
				}
			}
			time.Sleep(20 * time.Millisecond)
			if _, err := taskStore.Upsert(ctx, tc.matchTask); err != nil {
				t.Fatalf("upsert match task failed: %v", err)
			}

			claimed, ok, err := taskStore.ClaimNextDue(ctx, workerID, time.Minute, tc.hints, tc.matches)
			if err != nil {
				t.Fatalf("claim next due failed: %v", err)
			}
			if !ok {
				t.Fatal("expected claim to succeed")
			}
			if claimed.Metadata.Name != tc.wantName {
				t.Fatalf("expected claim %q, got %q", tc.wantName, claimed.Metadata.Name)
			}
		})
	}
}

func openPostgresForStoreTest(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ORLOJ_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("ORLOJ_POSTGRES_DSN is not set; skipping Postgres integration test")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres failed: %v", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Skipf("postgres not reachable at ORLOJ_POSTGRES_DSN: %v", err)
	}
	if err := EnsurePostgresSchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("ensure schema failed: %v", err)
	}
	resetPostgresStoreTestTables(t, db)
	t.Cleanup(func() {
		resetPostgresStoreTestTables(t, db)
	})
	return db
}

func resetPostgresStoreTestTables(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, statement := range []string{
		`TRUNCATE TABLE task_logs`,
		`TRUNCATE TABLE webhook_dedupe`,
		`TRUNCATE TABLE resources`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("reset postgres test tables failed for %q: %v", statement, err)
		}
	}
}
