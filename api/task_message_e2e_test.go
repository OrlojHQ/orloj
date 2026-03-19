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

	"github.com/OrlojHQ/orloj/api"
	"github.com/OrlojHQ/orloj/controllers"
	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

func TestTaskExecutionPublishesAgentMessages(t *testing.T) {
	logger := log.New(io.Discard, "", 0)

	agentStore := store.NewAgentStore()
	agentSystemStore := store.NewAgentSystemStore()
	toolStore := store.NewToolStore()
	memoryStore := store.NewMemoryStore()
	policyStore := store.NewAgentPolicyStore()
	taskStore := store.NewTaskStore()
	modelEPStore := store.NewModelEndpointStore()

	server := api.NewServer(api.Stores{
		Agents:       agentStore,
		AgentSystems: agentSystemStore,
		Tools:        toolStore,
		Memories:     memoryStore,
		Policies:     policyStore,
		ModelEPs:     modelEPStore,
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
		Metadata:   resources.ObjectMeta{Name: "web-search"},
		Spec:       resources.ToolSpec{Type: "http", Endpoint: "https://search.example"},
	})

	postJSON(t, httpServer.URL+"/v1/agents", resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "planner"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "Plan steps.",
			Tools:    []string{"web-search"},
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	})
	postJSON(t, httpServer.URL+"/v1/agents", resources.Agent{
		APIVersion: "orloj.dev/v1",
		Kind:       "Agent",
		Metadata:   resources.ObjectMeta{Name: "writer"},
		Spec: resources.AgentSpec{
			ModelRef: "openai-default",
			Prompt:   "Write output.",
			Tools:    []string{"web-search"},
			Limits:   resources.AgentLimits{MaxSteps: 1, Timeout: "1s"},
		},
	})

	postJSON(t, httpServer.URL+"/v1/agent-systems", resources.AgentSystem{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentSystem",
		Metadata:   resources.ObjectMeta{Name: "report-system"},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"planner", "writer"},
			Graph: map[string]resources.GraphEdge{
				"planner": {Next: "writer"},
			},
		},
	})

	postJSON(t, httpServer.URL+"/v1/tasks", resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "task-with-messages"},
		Spec:       resources.TaskSpec{System: "report-system"},
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
	var task resources.Task
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
