package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
)

// fakeAgentJobStore implements AgentJobStore for testing.
type fakeAgentJobStore struct {
	tasks     map[string]resources.Task
	inputs    map[string]map[string]string
	results   map[string]*resources.AgentJobResult
	setErr    error
	getErr    error
	resultErr error
	clearErr  error
}

func newFakeAgentJobStore() *fakeAgentJobStore {
	return &fakeAgentJobStore{
		tasks:   make(map[string]resources.Task),
		inputs:  make(map[string]map[string]string),
		results: make(map[string]*resources.AgentJobResult),
	}
}

func (s *fakeAgentJobStore) Get(_ context.Context, name string) (resources.Task, bool, error) {
	t, ok := s.tasks[name]
	return t, ok, nil
}

func (s *fakeAgentJobStore) SetAgentJobInput(_ context.Context, name string, input map[string]string, _, _ string) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.inputs[name] = input
	return nil
}

func (s *fakeAgentJobStore) SetAgentJobResult(_ context.Context, name string, result *resources.AgentJobResult) error {
	if s.resultErr != nil {
		return s.resultErr
	}
	s.results[name] = result
	return nil
}

func (s *fakeAgentJobStore) GetAgentJobResult(_ context.Context, name string) (*resources.AgentJobResult, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.results[name], nil
}

func (s *fakeAgentJobStore) ClearAgentJobFields(_ context.Context, name string) error {
	if s.clearErr != nil {
		return s.clearErr
	}
	delete(s.inputs, name)
	delete(s.results, name)
	return nil
}

func newTestKubernetesAgentRuntime(client KubernetesJobClient, store AgentJobStore) *KubernetesAgentRuntime {
	return NewKubernetesAgentRuntime(client, KubernetesAgentConfig{
		Namespace:      "test-ns",
		Image:          "orloj:test",
		JobTTLSeconds:  60,
		DefaultMemory:  "256Mi",
		DefaultCPU:     "250m",
	}, store, nil)
}

func newTestTask() resources.Task {
	return resources.Task{
		Metadata: resources.ObjectMeta{
			Name:      "my-task",
			Namespace: "default",
		},
	}
}

func newTestAgent() resources.Agent {
	return resources.Agent{
		Metadata: resources.ObjectMeta{
			Name: "planner",
		},
		Spec: resources.AgentSpec{
			Tools: []string{"search"},
		},
	}
}

func TestKubernetesAgentRuntimeExecuteSuccess(t *testing.T) {
	watchCh := make(chan watch.Event, 1)
	watchCh <- newCompletedJobEvent("orloj-agent-my-task-planner-a0")

	store := newFakeAgentJobStore()
	store.results["my-task"] = &resources.AgentJobResult{
		Output: "agent output",
		Steps:  3,
	}

	client := &fakeKubernetesJobClient{
		watchCh: watchCh,
		getJob:  nil,
		getJobErr: k8serrors.NewNotFound(schema.GroupResource{
			Group: "batch", Resource: "jobs",
		}, "not-found"),
	}

	rt := newTestKubernetesAgentRuntime(client, store)
	result, err := rt.ExecuteAgent(context.Background(), newTestTask(), newTestAgent(), map[string]string{"key": "val"}, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "agent output" {
		t.Errorf("expected output='agent output', got %q", result.Output)
	}
	if result.Steps != 3 {
		t.Errorf("expected steps=3, got %d", result.Steps)
	}
	if client.createdJob == nil {
		t.Fatal("expected job to be created")
	}
	if client.createdJob.Namespace != "test-ns" {
		t.Errorf("expected namespace=test-ns, got %s", client.createdJob.Namespace)
	}
	container := client.createdJob.Spec.Template.Spec.Containers[0]
	if container.Image != "orloj:test" {
		t.Errorf("expected image=orloj:test, got %s", container.Image)
	}
}

func TestKubernetesAgentRuntimeExecuteJobFailed(t *testing.T) {
	watchCh := make(chan watch.Event, 1)
	watchCh <- newFailedJobEvent("orloj-agent-my-task-planner-a0", "BackoffLimitExceeded")

	store := newFakeAgentJobStore()
	client := &fakeKubernetesJobClient{
		watchCh: watchCh,
		getJob:  nil,
		getJobErr: k8serrors.NewNotFound(schema.GroupResource{
			Group: "batch", Resource: "jobs",
		}, "not-found"),
	}

	rt := newTestKubernetesAgentRuntime(client, store)
	_, err := rt.ExecuteAgent(context.Background(), newTestTask(), newTestAgent(), nil, 0, "")
	if err == nil {
		t.Fatal("expected error for failed job")
	}
}

func TestKubernetesAgentRuntimeExecuteJobFailedWithResult(t *testing.T) {
	watchCh := make(chan watch.Event, 1)
	watchCh <- newFailedJobEvent("orloj-agent-my-task-planner-a0", "BackoffLimitExceeded")

	store := newFakeAgentJobStore()
	store.results["my-task"] = &resources.AgentJobResult{
		Output: "partial output",
		Error:  "agent exceeded limit",
	}

	client := &fakeKubernetesJobClient{
		watchCh: watchCh,
		getJob:  nil,
		getJobErr: k8serrors.NewNotFound(schema.GroupResource{
			Group: "batch", Resource: "jobs",
		}, "not-found"),
	}

	rt := newTestKubernetesAgentRuntime(client, store)
	result, err := rt.ExecuteAgent(context.Background(), newTestTask(), newTestAgent(), nil, 0, "")
	if err != nil {
		t.Fatalf("expected result even on job failure, got error: %v", err)
	}
	if result.Output != "partial output" {
		t.Errorf("expected output='partial output', got %q", result.Output)
	}
}

func TestKubernetesAgentRuntimeContextCancel(t *testing.T) {
	watchCh := make(chan watch.Event)

	store := newFakeAgentJobStore()
	client := &fakeKubernetesJobClient{
		watchCh: watchCh,
		getJob:  nil,
		getJobErr: k8serrors.NewNotFound(schema.GroupResource{
			Group: "batch", Resource: "jobs",
		}, "not-found"),
	}

	rt := newTestKubernetesAgentRuntime(client, store)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := rt.ExecuteAgent(ctx, newTestTask(), newTestAgent(), nil, 0, "")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestKubernetesAgentRuntimeSetInputError(t *testing.T) {
	store := newFakeAgentJobStore()
	store.setErr = errors.New("db connection lost")

	client := &fakeKubernetesJobClient{
		getJob: nil,
		getJobErr: k8serrors.NewNotFound(schema.GroupResource{
			Group: "batch", Resource: "jobs",
		}, "not-found"),
	}

	rt := newTestKubernetesAgentRuntime(client, store)
	_, err := rt.ExecuteAgent(context.Background(), newTestTask(), newTestAgent(), nil, 0, "")
	if err == nil {
		t.Fatal("expected error for store failure")
	}
}

func TestKubernetesAgentRuntimeCrashRecoveryComplete(t *testing.T) {
	store := newFakeAgentJobStore()
	store.results["my-task"] = &resources.AgentJobResult{
		Output: "recovered output",
		Steps:  5,
	}

	completedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "orloj-agent-my-task-planner-a0"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
			},
		},
	}

	client := &fakeKubernetesJobClient{
		getJob: completedJob,
	}

	rt := newTestKubernetesAgentRuntime(client, store)
	result, err := rt.ExecuteAgent(context.Background(), newTestTask(), newTestAgent(), nil, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "recovered output" {
		t.Errorf("expected output='recovered output', got %q", result.Output)
	}
	if client.createdJob != nil {
		t.Error("should not create a new job during crash recovery")
	}
}

func TestKubernetesAgentRuntimeCrashRecoveryFailed(t *testing.T) {
	store := newFakeAgentJobStore()

	failedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "orloj-agent-my-task-planner-a0"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobFailed, Status: corev1.ConditionTrue},
			},
		},
	}

	client := &fakeKubernetesJobClient{
		getJob: failedJob,
	}

	rt := newTestKubernetesAgentRuntime(client, store)
	_, err := rt.ExecuteAgent(context.Background(), newTestTask(), newTestAgent(), nil, 0, "")
	if err == nil {
		t.Fatal("expected error for failed recovered job with no result")
	}
}

func TestKubernetesAgentRuntimeCrashRecoveryRunning(t *testing.T) {
	watchCh := make(chan watch.Event, 1)
	watchCh <- newCompletedJobEvent("orloj-agent-my-task-planner-a0")

	store := newFakeAgentJobStore()
	store.results["my-task"] = &resources.AgentJobResult{
		Output: "resumed result",
	}

	runningJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "orloj-agent-my-task-planner-a0"},
		Status:     batchv1.JobStatus{},
	}

	client := &fakeKubernetesJobClient{
		getJob:  runningJob,
		watchCh: watchCh,
	}

	rt := newTestKubernetesAgentRuntime(client, store)
	result, err := rt.ExecuteAgent(context.Background(), newTestTask(), newTestAgent(), nil, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "resumed result" {
		t.Errorf("expected output='resumed result', got %q", result.Output)
	}
	if client.createdJob != nil {
		t.Error("should not create a new job during crash recovery of running job")
	}
}

func TestCanRunAsJob(t *testing.T) {
	tests := []struct {
		name       string
		agent      resources.Agent
		tools      []resources.Tool
		mcpServers map[string]resources.McpServer
		want       bool
	}{
		{
			name: "simple http tool",
			agent: resources.Agent{
				Spec: resources.AgentSpec{Tools: []string{"web-search"}},
			},
			tools: []resources.Tool{{
				Metadata: resources.ObjectMeta{Name: "web-search"},
				Spec:     resources.ToolSpec{Type: "http"},
			}},
			want: true,
		},
		{
			name: "container isolation blocks",
			agent: resources.Agent{
				Spec: resources.AgentSpec{Tools: []string{"docker-tool"}},
			},
			tools: []resources.Tool{{
				Metadata: resources.ObjectMeta{Name: "docker-tool"},
				Spec: resources.ToolSpec{
					Type:    "cli",
					Runtime: resources.ToolRuntimePolicy{IsolationMode: "container"},
				},
			}},
			want: false,
		},
		{
			name: "stdio mcp with image blocks",
			agent: resources.Agent{
				Spec: resources.AgentSpec{Tools: []string{"mcp-tool"}},
			},
			tools: []resources.Tool{{
				Metadata: resources.ObjectMeta{Name: "mcp-tool"},
				Spec: resources.ToolSpec{
					Type:         "mcp",
					McpServerRef: "my-server",
				},
			}},
			mcpServers: map[string]resources.McpServer{
				"my-server": {
					Spec: resources.McpServerSpec{
						Transport: "stdio",
						Image:     "my-mcp:latest",
					},
				},
			},
			want: false,
		},
		{
			name: "stdio mcp without image ok",
			agent: resources.Agent{
				Spec: resources.AgentSpec{Tools: []string{"mcp-tool"}},
			},
			tools: []resources.Tool{{
				Metadata: resources.ObjectMeta{Name: "mcp-tool"},
				Spec: resources.ToolSpec{
					Type:         "mcp",
					McpServerRef: "my-server",
				},
			}},
			mcpServers: map[string]resources.McpServer{
				"my-server": {
					Spec: resources.McpServerSpec{
						Transport: "stdio",
					},
				},
			},
			want: true,
		},
		{
			name: "http mcp with image ok",
			agent: resources.Agent{
				Spec: resources.AgentSpec{Tools: []string{"mcp-tool"}},
			},
			tools: []resources.Tool{{
				Metadata: resources.ObjectMeta{Name: "mcp-tool"},
				Spec: resources.ToolSpec{
					Type:         "mcp",
					McpServerRef: "my-server",
				},
			}},
			mcpServers: map[string]resources.McpServer{
				"my-server": {
					Spec: resources.McpServerSpec{
						Transport: "http",
						Image:     "my-mcp:latest",
					},
				},
			},
			want: true,
		},
		{
			name: "no tools",
			agent: resources.Agent{
				Spec: resources.AgentSpec{Tools: nil},
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CanRunAsJob(tc.agent, tc.tools, tc.mcpServers)
			if got != tc.want {
				t.Errorf("CanRunAsJob() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildAgentJob(t *testing.T) {
	store := newFakeAgentJobStore()
	client := &fakeKubernetesJobClient{}
	rt := NewKubernetesAgentRuntime(client, KubernetesAgentConfig{
		Namespace:      "agent-ns",
		ServiceAccount: "agent-sa",
		Image:          "orloj:v1.2.3",
		JobTTLSeconds:  120,
		DefaultMemory:  "1Gi",
		DefaultCPU:     "1",
	}, store, nil)

	task := resources.Task{
		Metadata: resources.ObjectMeta{
			Name:      "my-task",
			Namespace: "prod",
		},
	}
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "planner"},
		Spec: resources.AgentSpec{
			Limits: resources.AgentLimits{Timeout: "5m"},
		},
	}

	job := rt.buildAgentJob(task, agent, 2, "msg-123")

	if job.Namespace != "agent-ns" {
		t.Errorf("expected namespace=agent-ns, got %s", job.Namespace)
	}

	if job.Spec.Template.Spec.ServiceAccountName != "agent-sa" {
		t.Errorf("expected sa=agent-sa, got %s", job.Spec.Template.Spec.ServiceAccountName)
	}

	if *job.Spec.TTLSecondsAfterFinished != 120 {
		t.Errorf("expected TTL=120, got %d", *job.Spec.TTLSecondsAfterFinished)
	}

	if job.Spec.ActiveDeadlineSeconds == nil || *job.Spec.ActiveDeadlineSeconds != 300 {
		t.Errorf("expected active deadline=300 (5m), got %v", job.Spec.ActiveDeadlineSeconds)
	}

	container := job.Spec.Template.Spec.Containers[0]
	if container.Image != "orloj:v1.2.3" {
		t.Errorf("expected image=orloj:v1.2.3, got %s", container.Image)
	}

	if container.Resources.Limits.Memory().String() != "1Gi" {
		t.Errorf("expected memory=1Gi, got %s", container.Resources.Limits.Memory().String())
	}
	if container.Resources.Limits.Cpu().String() != "1" {
		t.Errorf("expected cpu=1, got %s", container.Resources.Limits.Cpu().String())
	}

	expectedArgs := []string{
		"--single-agent", "--task-id", "prod/my-task", "--agent-name", "planner",
		"--attempt", "2", "--message-id", "msg-123",
	}
	if fmt.Sprintf("%v", container.Args) != fmt.Sprintf("%v", expectedArgs) {
		t.Errorf("expected args=%v, got %v", expectedArgs, container.Args)
	}

	if job.Labels["orloj.dev/component"] != "agent-job" {
		t.Errorf("expected label component=agent-job, got %s", job.Labels["orloj.dev/component"])
	}
}

func TestKubernetesAgentConfigNormalized(t *testing.T) {
	cfg := KubernetesAgentConfig{}.normalized()

	if cfg.Namespace != "default" {
		t.Errorf("expected namespace=default, got %s", cfg.Namespace)
	}
	if cfg.JobTTLSeconds != 600 {
		t.Errorf("expected TTL=600, got %d", cfg.JobTTLSeconds)
	}
	if cfg.DefaultMemory != "512Mi" {
		t.Errorf("expected memory=512Mi, got %s", cfg.DefaultMemory)
	}
	if cfg.DefaultCPU != "500m" {
		t.Errorf("expected cpu=500m, got %s", cfg.DefaultCPU)
	}

	cfg2 := KubernetesAgentConfig{
		Namespace:     "custom",
		JobTTLSeconds: 100,
		DefaultMemory: "2Gi",
		DefaultCPU:    "2",
	}.normalized()
	if cfg2.Namespace != "custom" {
		t.Errorf("expected custom namespace preserved, got %s", cfg2.Namespace)
	}
	if cfg2.JobTTLSeconds != 100 {
		t.Errorf("expected TTL=100, got %d", cfg2.JobTTLSeconds)
	}
}

func TestAgentJobResultConversions(t *testing.T) {
	exec := AgentExecutionResult{
		Agent:           "planner",
		Steps:           5,
		ToolCalls:       3,
		MemoryWrites:    1,
		EstimatedTokens: 100,
		TokensUsed:      80,
		TokenSource:     "openai",
		Duration:        2500 * time.Millisecond,
		Output:          "done",
		LastEvent:       "tool_call",
		Events:          []string{"event1", "event2"},
		StepEvents: []AgentStepEvent{
			{
				Type:    "tool_start",
				Step:    1,
				Tool:    "search",
				Message: "searching",
			},
			{
				Type:    "tool_end",
				Step:    1,
				Tool:    "search",
				Message: "found it",
			},
		},
	}

	jobResult := ExecutionToAgentJobResult(exec, errors.New("partial failure"))
	if jobResult.Output != "done" {
		t.Errorf("expected output='done', got %q", jobResult.Output)
	}
	if jobResult.Error != "partial failure" {
		t.Errorf("expected error='partial failure', got %q", jobResult.Error)
	}
	if jobResult.DurationMS != 2500 {
		t.Errorf("expected duration=2500ms, got %d", jobResult.DurationMS)
	}
	if len(jobResult.StepEvents) != 2 {
		t.Errorf("expected 2 step events, got %d", len(jobResult.StepEvents))
	}

	back := AgentJobResultToExecution(jobResult, "planner")
	if back.Agent != "planner" {
		t.Errorf("expected agent=planner, got %s", back.Agent)
	}
	if back.Steps != 5 {
		t.Errorf("expected steps=5, got %d", back.Steps)
	}
	if back.Duration != 2500*time.Millisecond {
		t.Errorf("expected duration=2500ms, got %v", back.Duration)
	}
	if len(back.StepEvents) != 2 {
		t.Errorf("expected 2 step events, got %d", len(back.StepEvents))
	}
	if back.StepEvents[0].Tool != "search" {
		t.Errorf("expected tool=search, got %s", back.StepEvents[0].Tool)
	}
}

func TestAgentJobResultConversionNoError(t *testing.T) {
	exec := AgentExecutionResult{Output: "clean"}
	jobResult := ExecutionToAgentJobResult(exec, nil)
	if jobResult.Error != "" {
		t.Errorf("expected empty error, got %q", jobResult.Error)
	}
}

func TestTaskStoreKey(t *testing.T) {
	tests := []struct {
		task resources.Task
		want string
	}{
		{
			task: resources.Task{Metadata: resources.ObjectMeta{Name: "my-task", Namespace: "default"}},
			want: "my-task",
		},
		{
			task: resources.Task{Metadata: resources.ObjectMeta{Name: "my-task", Namespace: ""}},
			want: "my-task",
		},
		{
			task: resources.Task{Metadata: resources.ObjectMeta{Name: "my-task", Namespace: "prod"}},
			want: "prod/my-task",
		},
	}
	for _, tc := range tests {
		got := taskStoreKey(tc.task)
		if got != tc.want {
			t.Errorf("taskStoreKey(%v) = %q, want %q", tc.task.Metadata, got, tc.want)
		}
	}
}

func TestAgentJobName(t *testing.T) {
	name := agentJobName("my-task", "planner", 0)
	if name != "orloj-agent-my-task-planner-a0" {
		t.Errorf("unexpected job name: %s", name)
	}

	name2 := agentJobName("prod/my-task", "Planner Agent", 3)
	expected := sanitizeK8sName("orloj-agent-prod/my-task-Planner Agent-a3")
	if name2 != expected {
		t.Errorf("unexpected job name: got %s, want %s", name2, expected)
	}
}
