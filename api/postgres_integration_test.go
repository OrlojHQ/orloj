package api_test

import (
	"context"
	"database/sql"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/api"
	"github.com/OrlojHQ/orloj/controllers"
	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresTaskLifecycleApplyScheduleRunTrace(t *testing.T) {
	h := newPostgresPhase1Harness(t, "worker-a")
	defer h.Close()

	postJSON(t, h.url+"/v1/workers", resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-a"},
		Spec: resources.WorkerSpec{
			Region:             "default",
			MaxConcurrentTasks: 1,
			Capabilities: resources.WorkerCapabilities{
				SupportedModels: []string{"gpt-4o"},
			},
		},
	})
	patchWorkerStatus(t, h.url, "worker-a", resources.WorkerStatus{
		Phase:         "Ready",
		LastHeartbeat: time.Now().UTC().Format(time.RFC3339Nano),
	})

	postJSON(t, h.url+"/v1/tools", resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "web_search"},
		Spec: resources.ToolSpec{
			Type:     "http",
			Endpoint: "https://api.search.example",
		},
	})
	postJSON(t, h.url+"/v1/model-endpoints", resources.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata:   resources.ObjectMeta{Name: "openai-default"},
		Spec: resources.ModelEndpointSpec{
			Provider:     "mock",
			DefaultModel: "gpt-4o",
		},
	})
	postJSON(t, h.url+"/v1/agents", resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "research-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "You are a research assistant.",
			Tools:    []string{"web_search"},
			Limits:   resources.AgentLimits{MaxSteps: 2, Timeout: "1s"},
		},
	})
	postJSON(t, h.url+"/v1/agent-systems", resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "report-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"research-agent"}},
	})
	postJSON(t, h.url+"/v1/tasks", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "weekly-report-pg"},
		Spec: resources.TaskSpec{
			System:   "report-system",
			Priority: "high",
			Input:    map[string]string{"topic": "AI startups"},
			Requirements: resources.TaskRequirements{
				Region: "default",
				Model:  "gpt-4o",
			},
		},
	})

	if err := h.scheduler.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("scheduler reconcile failed: %v", err)
	}
	if err := h.taskController.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("worker reconcile failed: %v", err)
	}

	final := getTaskResource(t, h.url, "weekly-report-pg")
	if final.Status.Phase != "Succeeded" {
		t.Fatalf("expected Succeeded, got %q", final.Status.Phase)
	}
	if final.Status.Attempts != 1 {
		t.Fatalf("expected attempts=1, got %d", final.Status.Attempts)
	}
	if final.Status.Output["agents_executed"] != "1" {
		t.Fatalf("expected agents_executed=1, got %q", final.Status.Output["agents_executed"])
	}

	assertHistoryContainsType(t, final.Status.History, "assigned")
	assertHistoryContainsType(t, final.Status.History, "claim")
	assertHistoryContainsType(t, final.Status.History, "succeeded")
	assertTraceContainsType(t, final.Status.Trace, "task_summary")
}

func TestPostgresMultiWorkerSingleExecutionWithAssignment(t *testing.T) {
	h := newPostgresPhase1Harness(t, "worker-east")
	defer h.Close()

	postJSON(t, h.url+"/v1/workers", resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-east"},
		Spec: resources.WorkerSpec{
			Region:             "us-east",
			MaxConcurrentTasks: 1,
			Capabilities: resources.WorkerCapabilities{
				SupportedModels: []string{"gpt-4o"},
			},
		},
	})
	patchWorkerStatus(t, h.url, "worker-east", resources.WorkerStatus{Phase: "Ready", LastHeartbeat: time.Now().UTC().Format(time.RFC3339Nano)})

	postJSON(t, h.url+"/v1/workers", resources.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   resources.ObjectMeta{Name: "worker-west"},
		Spec: resources.WorkerSpec{
			Region:             "us-west",
			MaxConcurrentTasks: 1,
			Capabilities: resources.WorkerCapabilities{
				SupportedModels: []string{"gpt-4o"},
			},
		},
	})
	patchWorkerStatus(t, h.url, "worker-west", resources.WorkerStatus{Phase: "Ready", LastHeartbeat: time.Now().UTC().Format(time.RFC3339Nano)})

	postJSON(t, h.url+"/v1/tools", resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "web_search"},
		Spec:       resources.ToolSpec{Type: "http", Endpoint: "https://example"},
	})
	postJSON(t, h.url+"/v1/model-endpoints", resources.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata:   resources.ObjectMeta{Name: "openai-default"},
		Spec: resources.ModelEndpointSpec{
			Provider:     "mock",
			DefaultModel: "gpt-4o",
		},
	})
	postJSON(t, h.url+"/v1/agents", resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "research-agent"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "run",
			Tools:    []string{"web_search"},
			Limits:   resources.AgentLimits{MaxSteps: 2, Timeout: "1s"},
		},
	})
	postJSON(t, h.url+"/v1/agent-systems", resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "report-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"research-agent"}},
	})
	postJSON(t, h.url+"/v1/tasks", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "multi-worker-task-pg"},
		Spec: resources.TaskSpec{
			System: "report-system",
			Requirements: resources.TaskRequirements{
				Region: "us-west",
				Model:  "gpt-4o",
			},
		},
	})

	if err := h.scheduler.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("scheduler reconcile failed: %v", err)
	}

	east := controllers.NewTaskController(h.taskStore, h.systemStore, h.agentStore, h.toolStore, h.memoryStore, h.policyStore, h.workerStore, log.New(io.Discard, "", 0), 5*time.Millisecond)
	east.SetModelEndpointStore(h.modelEPStore)
	east.ConfigureWorker("worker-east", 50*time.Millisecond, 10*time.Millisecond)
	west := controllers.NewTaskController(h.taskStore, h.systemStore, h.agentStore, h.toolStore, h.memoryStore, h.policyStore, h.workerStore, log.New(io.Discard, "", 0), 5*time.Millisecond)
	west.SetModelEndpointStore(h.modelEPStore)
	west.ConfigureWorker("worker-west", 50*time.Millisecond, 10*time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = east.ReconcileOnce(context.Background())
	}()
	go func() {
		defer wg.Done()
		_ = west.ReconcileOnce(context.Background())
	}()
	wg.Wait()

	final := getTaskResource(t, h.url, "multi-worker-task-pg")
	if final.Status.Phase != "Succeeded" {
		t.Fatalf("expected Succeeded, got %q", final.Status.Phase)
	}
	if final.Status.Attempts != 1 {
		t.Fatalf("expected attempts=1, got %d", final.Status.Attempts)
	}

	foundWestClaim := false
	for _, item := range final.Status.History {
		if item.Type == "claim" && item.Worker == "worker-west" {
			foundWestClaim = true
			break
		}
	}
	if !foundWestClaim {
		t.Fatalf("expected worker-west claim event, history=%+v", final.Status.History)
	}
}

func newPostgresPhase1Harness(t *testing.T, workerID string) *phase1Harness {
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
	resetPostgresState(t, db)

	t.Cleanup(func() {
		resetPostgresState(t, db)
		_ = db.Close()
	})

	logger := log.New(io.Discard, "", 0)
	h := &phase1Harness{
		agentStore:   store.NewAgentStoreWithDB(db),
		systemStore:  store.NewAgentSystemStoreWithDB(db),
		toolStore:    store.NewToolStoreWithDB(db),
		memoryStore:  store.NewMemoryStoreWithDB(db),
		policyStore:  store.NewAgentPolicyStoreWithDB(db),
		modelEPStore: store.NewModelEndpointStoreWithDB(db),
		taskStore:    store.NewTaskStoreWithDB(db),
		workerStore:  store.NewWorkerStoreWithDB(db),
	}

	runtimeMgr := agentruntime.NewManager(logger)
	server := api.NewServer(api.Stores{
		Agents:       h.agentStore,
		AgentSystems: h.systemStore,
		Tools:        h.toolStore,
		Memories:     h.memoryStore,
		Policies:     h.policyStore,
		ModelEPs:     h.modelEPStore,
		Tasks:        h.taskStore,
		Workers:      h.workerStore,
	}, runtimeMgr, logger)
	h.server = httptest.NewServer(server.Handler())
	h.url = h.server.URL

	h.scheduler = controllers.NewTaskSchedulerController(h.taskStore, h.workerStore, logger, 5*time.Millisecond, 100*time.Millisecond)
	h.workerCtl = controllers.NewWorkerController(h.workerStore, logger, 5*time.Millisecond, 100*time.Millisecond)
	h.taskController = controllers.NewTaskController(
		h.taskStore,
		h.systemStore,
		h.agentStore,
		h.toolStore,
		h.memoryStore,
		h.policyStore,
		h.workerStore,
		logger,
		5*time.Millisecond,
	)
	h.taskController.SetModelEndpointStore(h.modelEPStore)
	h.taskController.ConfigureWorker(workerID, 50*time.Millisecond, 10*time.Millisecond)
	return h
}

func resetPostgresState(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`TRUNCATE TABLE webhook_dedupe`); err != nil {
		t.Fatalf("truncate webhook_dedupe failed: %v", err)
	}
	if _, err := db.Exec(`TRUNCATE TABLE task_logs`); err != nil {
		t.Fatalf("truncate task_logs failed: %v", err)
	}
	if _, err := db.Exec(`TRUNCATE TABLE resources`); err != nil {
		t.Fatalf("truncate resources failed: %v", err)
	}
}
