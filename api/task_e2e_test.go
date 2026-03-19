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
	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

func TestApplyRunInspectTaskLogs(t *testing.T) {
	logger := log.New(io.Discard, "", 0)

	agentStore := store.NewAgentStore()
	agentSystemStore := store.NewAgentSystemStore()
	toolStore := store.NewToolStore()
	memoryStore := store.NewMemoryStore()
	policyStore := store.NewAgentPolicyStore()
	taskStore := store.NewTaskStore()
	modelEPStore := store.NewModelEndpointStore()

	runtimeMgr := agentruntime.NewManager(logger)
	server := api.NewServer(api.Stores{
		Agents:       agentStore,
		AgentSystems: agentSystemStore,
		Tools:        toolStore,
		Memories:     memoryStore,
		Policies:     policyStore,
		ModelEPs:     modelEPStore,
		Tasks:        taskStore,
		Workers:      store.NewWorkerStore(),
	}, runtimeMgr, logger)

	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	controller := controllers.NewTaskController(
		taskStore,
		agentSystemStore,
		agentStore,
		toolStore,
		memoryStore,
		policyStore,
		nil,
		logger,
		5*time.Millisecond,
	)
	controller.SetModelEndpointStore(modelEPStore)

	postJSON(t, httpServer.URL+"/v1/model-endpoints", resources.ModelEndpoint{
		APIVersion: "orloj.dev/v1",
		Kind:       "ModelEndpoint",
		Metadata:   resources.ObjectMeta{Name: "openai-default"},
		Spec: resources.ModelEndpointSpec{
			Provider:     "mock",
			DefaultModel: "gpt-4o",
		},
	})

	postJSON(t, httpServer.URL+"/v1/tools", resources.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   resources.ObjectMeta{Name: "web_search"},
		Spec: resources.ToolSpec{
			Type:     "http",
			Endpoint: "https://api.search.example",
		},
	})

	postJSON(t, httpServer.URL+"/v1/agents", resources.Agent{
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

	postJSON(t, httpServer.URL+"/v1/agent-systems", resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "report-system"},
		Spec:       resources.AgentSystemSpec{Agents: []string{"research-agent"}},
	})

	postJSON(t, httpServer.URL+"/v1/agent-policies", resources.AgentPolicy{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentPolicy",
		Metadata:   resources.ObjectMeta{Name: "cost-policy"},
		Spec: resources.AgentPolicySpec{
			ApplyMode:       "scoped",
			TargetSystems:   []string{"report-system"},
			MaxTokensPerRun: 100000,
			AllowedModels:   []string{"gpt-4o"},
		},
	})

	postJSON(t, httpServer.URL+"/v1/tasks", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "weekly-report"},
		Spec: resources.TaskSpec{
			System:   "report-system",
			Priority: "high",
			Input:    map[string]string{"topic": "AI startups"},
		},
	})

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile pending->running failed: %v", err)
	}
	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile running->succeeded failed: %v", err)
	}

	taskResp, err := http.Get(httpServer.URL + "/v1/tasks/weekly-report")
	if err != nil {
		t.Fatalf("get task failed: %v", err)
	}
	defer taskResp.Body.Close()
	if taskResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(taskResp.Body)
		t.Fatalf("get task status=%d body=%s", taskResp.StatusCode, string(body))
	}

	var task resources.Task
	if err := json.NewDecoder(taskResp.Body).Decode(&task); err != nil {
		t.Fatalf("decode task failed: %v", err)
	}
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected task phase Succeeded, got %q", task.Status.Phase)
	}
	if task.Status.Output["agents_executed"] != "1" {
		t.Fatalf("expected agents_executed=1, got %q", task.Status.Output["agents_executed"])
	}

	logsResp, err := http.Get(httpServer.URL + "/v1/tasks/weekly-report/logs")
	if err != nil {
		t.Fatalf("get task logs failed: %v", err)
	}
	defer logsResp.Body.Close()
	if logsResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(logsResp.Body)
		t.Fatalf("get task logs status=%d body=%s", logsResp.StatusCode, string(body))
	}

	var payload struct {
		Name string   `json:"name"`
		Logs []string `json:"logs"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode task logs failed: %v", err)
	}
	if payload.Name != "weekly-report" {
		t.Fatalf("expected logs name weekly-report, got %q", payload.Name)
	}
	if len(payload.Logs) == 0 {
		t.Fatal("expected task logs to be non-empty")
	}

	mustContainLog(t, payload.Logs, "policy selection: matched=cost-policy")
	mustContainLog(t, payload.Logs, "agent start: research-agent")
	mustContainLog(t, payload.Logs, "agent end: research-agent")
	mustContainLog(t, payload.Logs, "task execution summary")
}

func postJSON(t *testing.T, url string, payload any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("post %s status=%d body=%s", url, resp.StatusCode, string(respBody))
	}
}

func mustContainLog(t *testing.T, logs []string, pattern string) {
	t.Helper()
	for _, entry := range logs {
		if strings.Contains(entry, pattern) {
			return
		}
	}
	t.Fatalf("expected logs to contain %q, logs=%v", pattern, logs)
}
