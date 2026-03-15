package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/crds"
)

type staticToolRuntime struct{}

func (s *staticToolRuntime) Call(_ context.Context, tool string, input string) (string, error) {
	return tool + ":" + input, nil
}

type denyingToolRuntime struct{}

func (d *denyingToolRuntime) Call(_ context.Context, tool string, _ string) (string, error) {
	return "", NewToolDeniedError(
		fmt.Sprintf("policy permission denied for tool=%s required=tool:%s:invoke", tool, tool),
		map[string]string{
			"tool":     tool,
			"required": fmt.Sprintf("tool:%s:invoke", tool),
		},
		ErrToolPermissionDenied,
	)
}

type failingModelGateway struct{}

func (f *failingModelGateway) Complete(_ context.Context, _ ModelRequest) (ModelResponse, error) {
	return ModelResponse{}, errors.New("temporary model error")
}

func TestTaskExecutorStepEventsIncludeModelAndToolCalls(t *testing.T) {
	executor := NewTaskExecutorWithRuntime(nil, &staticToolRuntime{}, &MockModelGateway{}, nil)
	agent := crds.Agent{
		Metadata: crds.ObjectMeta{Name: "research"},
		Spec: crds.AgentSpec{
			Model:  "gpt-4o",
			Prompt: "test prompt",
			Tools:  []string{"web_search"},
			Limits: crds.AgentLimits{MaxSteps: 2},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, map[string]string{"topic": "agents"})
	if err != nil {
		t.Fatalf("execute agent failed: %v", err)
	}
	if result.Steps != 2 {
		t.Fatalf("expected 2 steps, got %d", result.Steps)
	}
	if result.ToolCalls != 1 {
		t.Fatalf("expected 1 model-selected tool call, got %d", result.ToolCalls)
	}
	if len(result.StepEvents) == 0 {
		t.Fatal("expected structured step events")
	}

	var sawModelCall bool
	var modelCall AgentStepEvent
	var sawToolCall bool
	for _, event := range result.StepEvents {
		if event.Type == "model_call" {
			sawModelCall = true
			modelCall = event
		}
		if event.Type == "tool_call" {
			sawToolCall = true
		}
	}
	if !sawModelCall {
		t.Fatal("expected at least one model_call step event")
	}
	if modelCall.Tokens <= 0 {
		t.Fatalf("expected model_call tokens > 0, got %d", modelCall.Tokens)
	}
	if strings.TrimSpace(modelCall.UsageSource) == "" {
		t.Fatal("expected model_call usage_source metadata")
	}
	if !sawToolCall {
		t.Fatal("expected at least one tool_call step event")
	}
}

func TestTaskExecutorStepEventsCaptureModelErrors(t *testing.T) {
	executor := NewTaskExecutorWithRuntime(nil, &staticToolRuntime{}, &failingModelGateway{}, nil)
	agent := crds.Agent{
		Metadata: crds.ObjectMeta{Name: "planner"},
		Spec: crds.AgentSpec{
			Model:  "gpt-4o",
			Prompt: "test prompt",
			Tools:  []string{"web_search"},
			Limits: crds.AgentLimits{MaxSteps: 1},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err == nil {
		t.Fatal("expected execute agent to fail when all model calls fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "model execution failed") {
		t.Fatalf("expected model execution failure error, got %v", err)
	}
	if result.ToolCalls != 0 {
		t.Fatalf("expected no tool calls when model errors, got %d", result.ToolCalls)
	}
	if len(result.StepEvents) == 0 {
		t.Fatal("expected step events for model error")
	}
	if result.StepEvents[0].Type != "agent_worker_start" {
		t.Fatalf("expected first event agent_worker_start, got %q", result.StepEvents[0].Type)
	}
	var sawModelError bool
	for _, event := range result.StepEvents {
		if event.Type == "model_error" {
			sawModelError = true
			break
		}
	}
	if !sawModelError {
		t.Fatal("expected model_error step event")
	}
}

func TestTaskExecutorHardFailsOnPermissionDenied(t *testing.T) {
	executor := NewTaskExecutorWithRuntime(nil, &denyingToolRuntime{}, &MockModelGateway{}, nil)
	agent := crds.Agent{
		Metadata: crds.ObjectMeta{Name: "research"},
		Spec: crds.AgentSpec{
			Model:  "gpt-4o",
			Prompt: "test prompt",
			Tools:  []string{"vector_db"},
			Limits: crds.AgentLimits{MaxSteps: 2},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, map[string]string{"topic": "agents"})
	if err == nil {
		t.Fatal("expected permission denied execution error")
	}
	if !errors.Is(err, ErrToolPermissionDenied) {
		t.Fatalf("expected ErrToolPermissionDenied, got %v", err)
	}
	var sawDenied bool
	var deniedEvent AgentStepEvent
	for _, event := range result.StepEvents {
		if event.Type == "tool_permission_denied" {
			sawDenied = true
			deniedEvent = event
			break
		}
	}
	if !sawDenied {
		t.Fatal("expected tool_permission_denied step event")
	}
	if deniedEvent.ErrorCode != ToolCodePermissionDenied {
		t.Fatalf("expected denied event error code %q, got %q", ToolCodePermissionDenied, deniedEvent.ErrorCode)
	}
	if deniedEvent.ErrorReason != ToolReasonPermissionDenied {
		t.Fatalf("expected denied event error reason %q, got %q", ToolReasonPermissionDenied, deniedEvent.ErrorReason)
	}
	if deniedEvent.Retryable == nil {
		t.Fatal("expected denied event retryable metadata")
	}
	if *deniedEvent.Retryable {
		t.Fatal("expected denied event retryable=false")
	}
}

func TestTaskExecutorStepEventsCaptureToolContractMetadata(t *testing.T) {
	executor := NewTaskExecutorWithRuntime(nil, &staticToolRuntime{}, &MockModelGateway{}, nil)
	agent := crds.Agent{
		Metadata: crds.ObjectMeta{Name: "research", Namespace: "default"},
		Spec: crds.AgentSpec{
			Model:  "gpt-4o",
			Prompt: "test prompt",
			Tools:  []string{"web_search"},
			Limits: crds.AgentLimits{MaxSteps: 1},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, map[string]string{"topic": "agents"})
	if err != nil {
		t.Fatalf("execute agent failed: %v", err)
	}
	var toolEvent AgentStepEvent
	var found bool
	for _, event := range result.StepEvents {
		if event.Type == "tool_call" {
			toolEvent = event
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected tool_call event")
	}
	if toolEvent.ToolContractVersion != ToolContractVersionV1 {
		t.Fatalf("expected tool contract version %q, got %q", ToolContractVersionV1, toolEvent.ToolContractVersion)
	}
	if toolEvent.ToolRequestID == "" {
		t.Fatal("expected tool request id metadata")
	}
	if toolEvent.ToolAttempt != 1 {
		t.Fatalf("expected tool attempt=1, got %d", toolEvent.ToolAttempt)
	}
}

func TestTaskExecutorNoToolsReturnsModelOutput(t *testing.T) {
	executor := NewTaskExecutorWithRuntime(nil, &staticToolRuntime{}, &MockModelGateway{}, nil)
	agent := crds.Agent{
		Metadata: crds.ObjectMeta{Name: "writer"},
		Spec: crds.AgentSpec{
			Model:  "gpt-4o-mini",
			Prompt: "write summary",
			Limits: crds.AgentLimits{MaxSteps: 4},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, map[string]string{"topic": "incident response"})
	if err != nil {
		t.Fatalf("execute agent failed: %v", err)
	}
	if strings.TrimSpace(result.Output) == "" {
		t.Fatal("expected non-empty model output")
	}
	if result.Steps != 1 {
		t.Fatalf("expected one step for no-tools agent, got %d", result.Steps)
	}

	var sawModelOutput bool
	var sawMaxSteps bool
	for _, event := range result.StepEvents {
		if event.Type == "model_output" {
			sawModelOutput = true
		}
		if event.Type == "agent_worker_complete" && strings.Contains(strings.ToLower(event.Message), "max steps reached") {
			sawMaxSteps = true
		}
	}
	if !sawModelOutput {
		t.Fatal("expected model_output step event")
	}
	if sawMaxSteps {
		t.Fatal("did not expect max steps reached when no-tools model output is available")
	}
}
