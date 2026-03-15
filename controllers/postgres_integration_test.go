package controllers

import (
	"context"
	"database/sql"
	"io"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/crds"
	"github.com/OrlojHQ/orloj/store"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresTaskClaimSingleExecutionAcrossWorkers(t *testing.T) {
	db := openPostgresForControllerTest(t)
	defer db.Close()

	agentStore := store.NewAgentStoreWithDB(db)
	agentSystemStore := store.NewAgentSystemStoreWithDB(db)
	toolStore := store.NewToolStoreWithDB(db)
	memoryStore := store.NewMemoryStoreWithDB(db)
	policyStore := store.NewAgentPolicyStoreWithDB(db)
	taskStore := store.NewTaskStoreWithDB(db)
	workerStore := store.NewWorkerStoreWithDB(db)
	logger := log.New(io.Discard, "", 0)

	if _, err := toolStore.Upsert(crds.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   crds.ObjectMeta{Name: "web_search"},
		Spec:       crds.ToolSpec{Type: "http", Endpoint: "https://example"},
	}); err != nil {
		t.Fatalf("upsert tool failed: %v", err)
	}
	if _, err := agentStore.Upsert(crds.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   crds.ObjectMeta{Name: "research-agent"},
		Spec: crds.AgentSpec{
			Model:  "gpt-4o",
			Prompt: "run",
			Tools:  []string{"web_search"},
			Limits: crds.AgentLimits{MaxSteps: 2, Timeout: "1s"},
		},
	}); err != nil {
		t.Fatalf("upsert agent failed: %v", err)
	}
	if _, err := agentSystemStore.Upsert(crds.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   crds.ObjectMeta{Name: "report-system"},
		Spec:       crds.AgentSystemSpec{Agents: []string{"research-agent"}},
	}); err != nil {
		t.Fatalf("upsert system failed: %v", err)
	}
	if _, err := taskStore.Upsert(crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "postgres-claim-task"},
		Spec:       crds.TaskSpec{System: "report-system", Input: map[string]string{"topic": "x"}},
		Status:     crds.TaskStatus{AssignedWorker: "worker-2"},
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := workerStore.Upsert(crds.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   crds.ObjectMeta{Name: "worker-1"},
		Spec:       crds.WorkerSpec{Region: "default", MaxConcurrentTasks: 1},
		Status:     crds.WorkerStatus{Phase: "Ready", LastHeartbeat: now},
	}); err != nil {
		t.Fatalf("upsert worker-1 failed: %v", err)
	}
	if _, err := workerStore.Upsert(crds.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   crds.ObjectMeta{Name: "worker-2"},
		Spec:       crds.WorkerSpec{Region: "default", MaxConcurrentTasks: 1},
		Status:     crds.WorkerStatus{Phase: "Ready", LastHeartbeat: now},
	}); err != nil {
		t.Fatalf("upsert worker-2 failed: %v", err)
	}

	worker1 := NewTaskController(taskStore, agentSystemStore, agentStore, toolStore, memoryStore, policyStore, workerStore, logger, 5*time.Millisecond)
	worker1.ConfigureWorker("worker-1", 100*time.Millisecond, 20*time.Millisecond)
	worker2 := NewTaskController(taskStore, agentSystemStore, agentStore, toolStore, memoryStore, policyStore, workerStore, logger, 5*time.Millisecond)
	worker2.ConfigureWorker("worker-2", 100*time.Millisecond, 20*time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = worker1.ReconcileOnce(context.Background())
	}()
	go func() {
		defer wg.Done()
		_ = worker2.ReconcileOnce(context.Background())
	}()
	wg.Wait()

	task, ok := taskStore.Get("postgres-claim-task")
	if !ok {
		t.Fatal("task not found")
	}
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected task Succeeded, got %q", task.Status.Phase)
	}
	if task.Status.Attempts != 1 {
		t.Fatalf("expected attempts=1, got %d", task.Status.Attempts)
	}

	foundWorker2Claim := false
	for _, event := range task.Status.History {
		if event.Type == "claim" && event.Worker == "worker-2" {
			foundWorker2Claim = true
			break
		}
	}
	if !foundWorker2Claim {
		t.Fatalf("expected claim history to show worker-2 claim, got %+v", task.Status.History)
	}
}

func openPostgresForControllerTest(t *testing.T) *sql.DB {
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
	if err := store.EnsurePostgresSchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("ensure schema failed: %v", err)
	}
	if _, err := db.Exec(`TRUNCATE TABLE task_logs`); err != nil {
		_ = db.Close()
		t.Fatalf("truncate task_logs failed: %v", err)
	}
	if _, err := db.Exec(`TRUNCATE TABLE webhook_dedupe`); err != nil {
		_ = db.Close()
		t.Fatalf("truncate webhook_dedupe failed: %v", err)
	}
	if _, err := db.Exec(`TRUNCATE TABLE resources`); err != nil {
		_ = db.Close()
		t.Fatalf("truncate resources failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`TRUNCATE TABLE task_logs`)
		_, _ = db.Exec(`TRUNCATE TABLE webhook_dedupe`)
		_, _ = db.Exec(`TRUNCATE TABLE resources`)
	})
	return db
}
