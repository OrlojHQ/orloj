package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AnonJon/orloj/api"
	"github.com/AnonJon/orloj/controllers"
	"github.com/AnonJon/orloj/crds"
	agentruntime "github.com/AnonJon/orloj/runtime"
	"github.com/AnonJon/orloj/store"
)

func TestTaskExecutionPublishesAgentMessages(t *testing.T) {
	logger := log.New(io.Discard, "", 0)

	agentStore := store.NewAgentStore()
	agentSystemStore := store.NewAgentSystemStore()
	toolStore := store.NewToolStore()
	memoryStore := store.NewMemoryStore()
	policyStore := store.NewAgentPolicyStore()
	taskStore := store.NewTaskStore()

	server := api.NewServer(api.Stores{
		Agents:       agentStore,
		AgentSystems: agentSystemStore,
		Tools:        toolStore,
		Memories:     memoryStore,
		Policies:     policyStore,
		Tasks:        taskStore,
		Workers:      store.NewWorkerStore(),
	}, agentruntime.NewManager(logger), logger)

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

	postJSON(t, httpServer.URL+"/v1/tools", crds.Tool{
		APIVersion: "orloj.dev/v1",
		Kind:       "Tool",
		Metadata:   crds.ObjectMeta{Name: "web-search"},
		Spec:       crds.ToolSpec{Type: "http", Endpoint: "https://search.example"},
	})

	postJSON(t, httpServer.URL+"/v1/agents", crds.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   crds.ObjectMeta{Name: "planner"},
		Spec: crds.AgentSpec{
			Model:  "gpt-4o",
			Prompt: "Plan steps.",
			Tools:  []string{"web-search"},
			Limits: crds.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	})
	postJSON(t, httpServer.URL+"/v1/agents", crds.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   crds.ObjectMeta{Name: "writer"},
		Spec: crds.AgentSpec{
			Model:  "gpt-4o",
			Prompt: "Write output.",
			Tools:  []string{"web-search"},
			Limits: crds.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	})

	postJSON(t, httpServer.URL+"/v1/agent-systems", crds.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   crds.ObjectMeta{Name: "report-system"},
		Spec: crds.AgentSystemSpec{
			Agents: []string{"planner", "writer"},
			Graph: map[string]crds.GraphEdge{
				"planner": {Next: "writer"},
			},
		},
	})

	postJSON(t, httpServer.URL+"/v1/tasks", crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "task-with-messages"},
		Spec:       crds.TaskSpec{System: "report-system"},
	})

	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile pending->running failed: %v", err)
	}
	if err := controller.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile running->succeeded failed: %v", err)
	}

	resp, err := http.Get(httpServer.URL + "/v1/tasks/task-with-messages")
	if err != nil {
		t.Fatalf("get task failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("get task status=%d body=%s", resp.StatusCode, string(body))
	}
	var task crds.Task
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		t.Fatalf("decode task failed: %v", err)
	}
	if task.Status.Phase != "Succeeded" {
		t.Fatalf("expected phase Succeeded, got %q", task.Status.Phase)
	}
	if len(task.Status.Messages) == 0 {
		t.Fatal("expected task messages to be populated")
	}
	first := task.Status.Messages[0]
	if first.FromAgent != "planner" || first.ToAgent != "writer" {
		t.Fatalf("unexpected first message routing: %+v", first)
	}

	assertTraceContainsType(t, task.Status.Trace, "agent_message")
}
