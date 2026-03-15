package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/api"
	"github.com/OrlojHQ/orloj/controllers"
	"github.com/OrlojHQ/orloj/crds"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

type phase1Harness struct {
	server         *httptest.Server
	url            string
	agentStore     *store.AgentStore
	systemStore    *store.AgentSystemStore
	toolStore      *store.ToolStore
	memoryStore    *store.MemoryStore
	policyStore    *store.AgentPolicyStore
	taskStore      *store.TaskStore
	workerStore    *store.WorkerStore
	scheduler      *controllers.TaskSchedulerController
	workerCtl      *controllers.WorkerController
	taskController *controllers.TaskController
}

func newPhase1Harness(t *testing.T, workerID string) *phase1Harness {
	t.Helper()

	logger := log.New(io.Discard, "", 0)
	h := &phase1Harness{
		agentStore:  store.NewAgentStore(),
		systemStore: store.NewAgentSystemStore(),
		toolStore:   store.NewToolStore(),
		memoryStore: store.NewMemoryStore(),
		policyStore: store.NewAgentPolicyStore(),
		taskStore:   store.NewTaskStore(),
		workerStore: store.NewWorkerStore(),
	}

	runtimeMgr := agentruntime.NewManager(logger)
	server := api.NewServer(api.Stores{
		Agents:       h.agentStore,
		AgentSystems: h.systemStore,
		Tools:        h.toolStore,
		Memories:     h.memoryStore,
		Policies:     h.policyStore,
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
	h.taskController.ConfigureWorker(workerID, 50*time.Millisecond, 10*time.Millisecond)
	return h
}

func (h *phase1Harness) Close() {
	if h.server != nil {
		h.server.Close()
	}
}

func TestTaskLifecycleApplyScheduleClaimRunTrace(t *testing.T) {
	h := newPhase1Harness(t, "worker-a")
	defer h.Close()

	postJSON(t, h.url+"/v1/workers", crds.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   crds.ObjectMeta{Name: "worker-a"},
		Spec: crds.WorkerSpec{
			Region:             "default",
			MaxConcurrentTasks: 1,
			Capabilities: crds.WorkerCapabilities{
				SupportedModels: []string{"gpt-4o"},
			},
		},
	})
	patchWorkerStatus(t, h.url, "worker-a", crds.WorkerStatus{
		Phase:         "Ready",
		LastHeartbeat: time.Now().UTC().Format(time.RFC3339Nano),
	})

	postJSON(t, h.url+"/v1/tools", crds.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   crds.ObjectMeta{Name: "web_search"},
		Spec: crds.ToolSpec{
			Type:     "http",
			Endpoint: "https://api.search.example",
		},
	})
	postJSON(t, h.url+"/v1/agents", crds.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   crds.ObjectMeta{Name: "research-agent"},
		Spec: crds.AgentSpec{
			Model:  "gpt-4o",
			Prompt: "You are a research assistant.",
			Tools:  []string{"web_search"},
			Limits: crds.AgentLimits{MaxSteps: 2, Timeout: "1s"},
		},
	})
	postJSON(t, h.url+"/v1/agent-systems", crds.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   crds.ObjectMeta{Name: "report-system"},
		Spec:       crds.AgentSystemSpec{Agents: []string{"research-agent"}},
	})
	postJSON(t, h.url+"/v1/tasks", crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "weekly-report"},
		Spec: crds.TaskSpec{
			System:   "report-system",
			Priority: "high",
			Input:    map[string]string{"topic": "AI startups"},
			Requirements: crds.TaskRequirements{
				Region: "default",
				Model:  "gpt-4o",
			},
		},
	})

	if err := h.scheduler.ReconcileOnce(); err != nil {
		t.Fatalf("scheduler reconcile failed: %v", err)
	}

	scheduled, ok := h.taskStore.Get("weekly-report")
	if !ok {
		t.Fatal("task not found after schedule")
	}
	if scheduled.Status.AssignedWorker != "worker-a" {
		t.Fatalf("expected assignedWorker=worker-a, got %q", scheduled.Status.AssignedWorker)
	}

	if err := h.taskController.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("worker reconcile failed: %v", err)
	}

	final := getTaskResource(t, h.url, "weekly-report")
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

	assertTraceContainsType(t, final.Status.Trace, "task_start")
	assertTraceContainsType(t, final.Status.Trace, "agent_start")
	assertTraceContainsType(t, final.Status.Trace, "agent_end")
	assertTraceContainsType(t, final.Status.Trace, "task_summary")
	assertTraceHasStepIDs(t, final.Status.Trace)
}

func TestTaskLifecycleRetryThenDeadLetterWithTrace(t *testing.T) {
	h := newPhase1Harness(t, "worker-a")
	defer h.Close()

	postJSON(t, h.url+"/v1/workers", crds.Worker{
		APIVersion: "orloj.dev/v1",
		Kind:       "Worker",
		Metadata:   crds.ObjectMeta{Name: "worker-a"},
		Spec: crds.WorkerSpec{
			Region:             "default",
			MaxConcurrentTasks: 1,
			Capabilities: crds.WorkerCapabilities{
				SupportedModels: []string{"gpt-4o"},
			},
		},
	})
	patchWorkerStatus(t, h.url, "worker-a", crds.WorkerStatus{
		Phase:         "Ready",
		LastHeartbeat: time.Now().UTC().Format(time.RFC3339Nano),
	})

	postJSON(t, h.url+"/v1/agents", crds.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   crds.ObjectMeta{Name: "timeout-agent"},
		Spec: crds.AgentSpec{
			Model:  "gpt-4o",
			Prompt: "Timeout test.",
			Limits: crds.AgentLimits{MaxSteps: 5, Timeout: "1ms"},
		},
	})
	postJSON(t, h.url+"/v1/agent-systems", crds.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   crds.ObjectMeta{Name: "timeout-system"},
		Spec:       crds.AgentSystemSpec{Agents: []string{"timeout-agent"}},
	})
	postJSON(t, h.url+"/v1/tasks", crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "timeout-task"},
		Spec: crds.TaskSpec{
			System: "timeout-system",
			Retry: crds.TaskRetryPolicy{
				MaxAttempts: 2,
				Backoff:     "1ms",
			},
			Requirements: crds.TaskRequirements{
				Region: "default",
				Model:  "gpt-4o",
			},
		},
	})

	for i := 0; i < 10; i++ {
		if err := h.scheduler.ReconcileOnce(); err != nil {
			t.Fatalf("scheduler reconcile failed: %v", err)
		}
		if err := h.taskController.ReconcileOnce(context.Background()); err != nil {
			t.Fatalf("worker reconcile failed: %v", err)
		}
		task, ok := h.taskStore.Get("timeout-task")
		if !ok {
			t.Fatal("timeout task disappeared")
		}
		if strings.EqualFold(task.Status.Phase, "DeadLetter") {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	final := getTaskResource(t, h.url, "timeout-task")
	if final.Status.Phase != "DeadLetter" {
		t.Fatalf("expected DeadLetter, got %q", final.Status.Phase)
	}
	if final.Status.Attempts != 2 {
		t.Fatalf("expected attempts=2, got %d", final.Status.Attempts)
	}
	assertHistoryContainsType(t, final.Status.History, "retry_scheduled")
	assertHistoryContainsType(t, final.Status.History, "deadletter")
	assertTraceContainsType(t, final.Status.Trace, "agent_error")

	logs := getTaskLogs(t, h.url, "timeout-task")
	mustContainLog(t, logs, "retry scheduled")
	mustContainLog(t, logs, "task moved to DeadLetter")
}

func patchWorkerStatus(t *testing.T, baseURL, workerName string, status crds.WorkerStatus) {
	t.Helper()
	worker := getWorkerResource(t, baseURL, workerName)
	patch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": worker.Metadata.ResourceVersion,
		},
		"status": status,
	}
	body, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("marshal worker status patch failed: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, baseURL+"/v1/workers/"+workerName+"/status", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new worker status request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("worker status request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("worker status patch failed status=%d body=%s", resp.StatusCode, string(b))
	}
}

func getWorkerResource(t *testing.T, baseURL, workerName string) crds.Worker {
	t.Helper()
	resp, err := http.Get(baseURL + "/v1/workers/" + workerName)
	if err != nil {
		t.Fatalf("get worker failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("get worker status=%d body=%s", resp.StatusCode, string(b))
	}
	var out crds.Worker
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode worker failed: %v", err)
	}
	return out
}

func getTaskResource(t *testing.T, baseURL, taskName string) crds.Task {
	t.Helper()
	resp, err := http.Get(baseURL + "/v1/tasks/" + taskName)
	if err != nil {
		t.Fatalf("get task failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("get task status=%d body=%s", resp.StatusCode, string(b))
	}
	var out crds.Task
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode task failed: %v", err)
	}
	return out
}

func getTaskLogs(t *testing.T, baseURL, taskName string) []string {
	t.Helper()
	resp, err := http.Get(baseURL + "/v1/tasks/" + taskName + "/logs")
	if err != nil {
		t.Fatalf("get task logs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("get task logs status=%d body=%s", resp.StatusCode, string(b))
	}
	var payload struct {
		Name string   `json:"name"`
		Logs []string `json:"logs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode task logs failed: %v", err)
	}
	if payload.Name != taskName {
		t.Fatalf("expected logs for %s, got %s", taskName, payload.Name)
	}
	return payload.Logs
}

func assertHistoryContainsType(t *testing.T, history []crds.TaskHistoryEvent, eventType string) {
	t.Helper()
	for _, item := range history {
		if strings.EqualFold(item.Type, eventType) {
			return
		}
	}
	t.Fatalf("expected history to contain type=%q, history=%+v", eventType, history)
}

func assertTraceContainsType(t *testing.T, trace []crds.TaskTraceEvent, eventType string) {
	t.Helper()
	for _, item := range trace {
		if strings.EqualFold(item.Type, eventType) {
			return
		}
	}
	t.Fatalf("expected trace to contain type=%q, trace=%+v", eventType, trace)
}

func assertTraceHasStepIDs(t *testing.T, trace []crds.TaskTraceEvent) {
	t.Helper()
	if len(trace) == 0 {
		t.Fatal("expected trace entries")
	}
	for i, item := range trace {
		if strings.TrimSpace(item.StepID) == "" {
			t.Fatalf("expected step_id at trace index=%d type=%q", i, item.Type)
		}
		if item.Attempt <= 0 {
			t.Fatalf("expected attempt>0 at trace index=%d step_id=%q", i, item.StepID)
		}
	}
}
