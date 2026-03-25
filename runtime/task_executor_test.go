package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

type staticToolRuntime struct{}

func (s *staticToolRuntime) Call(_ context.Context, tool string, input string) (string, error) {
	return tool + ":" + input, nil
}

type approvalRequiredToolRuntime struct{}

func (a *approvalRequiredToolRuntime) Call(_ context.Context, tool string, _ string) (string, error) {
	return "", fmt.Errorf("%w: tool=%s reason=approval required for tool=%s", ErrToolApprovalRequired, tool, tool)
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

type scriptedModelGateway struct {
	responses       map[int]ModelResponse
	captureMessages *[]ChatMessage
}

func (s *scriptedModelGateway) Complete(_ context.Context, req ModelRequest) (ModelResponse, error) {
	if s.captureMessages != nil {
		*s.captureMessages = append([]ChatMessage(nil), req.Messages...)
	}
	if response, ok := s.responses[req.Step]; ok {
		return response, nil
	}
	return ModelResponse{Content: fmt.Sprintf("step=%d", req.Step)}, nil
}

type countingToolRuntime struct {
	calls map[string]int
}

func (c *countingToolRuntime) Call(_ context.Context, tool string, input string) (string, error) {
	if c.calls == nil {
		c.calls = make(map[string]int)
	}
	c.calls[tool]++
	return tool + ":" + input, nil
}

func TestTaskExecutorStepEventsIncludeModelAndToolCalls(t *testing.T) {
	executor := NewTaskExecutorWithRuntime(nil, &staticToolRuntime{}, &MockModelGateway{}, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "research"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "test prompt",
			Tools:  []string{"web_search"},
			Limits: resources.AgentLimits{MaxSteps: 2},
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
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "planner"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "test prompt",
			Tools:  []string{"web_search"},
			Limits: resources.AgentLimits{MaxSteps: 1},
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
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "research"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "test prompt",
			Tools:  []string{"vector_db"},
			Limits: resources.AgentLimits{MaxSteps: 2},
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

func TestTaskExecutorContractModeHappyPath(t *testing.T) {
	toolRT := &countingToolRuntime{}
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "collecting",
				ToolCalls: []ModelToolCall{
					{Name: "tool.alpha", Input: `{"q":"one"}`},
				},
			},
			2: {
				Content: "analyzing",
				ToolCalls: []ModelToolCall{
					{Name: "tool.beta", Input: `{"q":"two"}`},
				},
			},
			3: {
				Content: "CONTRACT_OK FINALIZED",
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "contract-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "contract prompt",
			Tools:  []string{"tool.alpha", "tool.beta"},
			Execution: resources.AgentExecutionSpec{
				Profile:               resources.AgentExecutionProfileContract,
				ToolSequence:          []string{"tool.alpha", "tool.beta"},
				RequiredOutputMarkers: []string{"CONTRACT_OK", "FINALIZED"},
			},
			Limits: resources.AgentLimits{MaxSteps: 6},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err != nil {
		t.Fatalf("expected contract-mode execution success, got %v", err)
	}
	if result.Steps != 3 {
		t.Fatalf("expected 3 steps, got %d", result.Steps)
	}
	if strings.TrimSpace(result.Output) != "CONTRACT_OK FINALIZED" {
		t.Fatalf("unexpected final output %q", result.Output)
	}
	if toolRT.calls["tool.alpha"] != 1 || toolRT.calls["tool.beta"] != 1 {
		t.Fatalf("expected one call per tool, got alpha=%d beta=%d", toolRT.calls["tool.alpha"], toolRT.calls["tool.beta"])
	}
	for _, event := range result.StepEvents {
		if event.Type == "agent_contract_violation" {
			t.Fatalf("did not expect contract violation event: %+v", event)
		}
	}
}

func TestTaskExecutorContractModeAllowsNoToolIntermediateStep(t *testing.T) {
	toolRT := &countingToolRuntime{}
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "thinking before tool call",
			},
			2: {
				Content: "now calling tool",
				ToolCalls: []ModelToolCall{
					{Name: "tool.alpha", Input: `{"q":"one"}`},
				},
			},
			3: {
				Content: "DONE",
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "contract-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "contract prompt",
			Tools:  []string{"tool.alpha"},
			Execution: resources.AgentExecutionSpec{
				Profile:               resources.AgentExecutionProfileContract,
				ToolSequence:          []string{"tool.alpha"},
				RequiredOutputMarkers: []string{"DONE"},
			},
			Limits: resources.AgentLimits{MaxSteps: 6},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err != nil {
		t.Fatalf("expected contract-mode execution success after no-tool intermediate step, got %v", err)
	}
	if strings.TrimSpace(result.Output) != "DONE" {
		t.Fatalf("unexpected final output %q", result.Output)
	}
	if toolRT.calls["tool.alpha"] != 1 {
		t.Fatalf("expected one tool.alpha call, got %d", toolRT.calls["tool.alpha"])
	}
}

func TestTaskExecutorContractModeDuplicateShortCircuit(t *testing.T) {
	toolRT := &countingToolRuntime{}
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "first call",
				ToolCalls: []ModelToolCall{
					{Name: "tool.alpha", Input: `{"q":"one"}`},
				},
			},
			2: {
				Content: "duplicate call",
				ToolCalls: []ModelToolCall{
					{Name: "tool.alpha", Input: `{"q":"one"}`},
				},
			},
			3: {
				Content: "DONE",
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "contract-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "contract prompt",
			Tools:  []string{"tool.alpha"},
			Execution: resources.AgentExecutionSpec{
				Profile:                 resources.AgentExecutionProfileContract,
				ToolSequence:            []string{"tool.alpha"},
				RequiredOutputMarkers:   []string{"DONE"},
				DuplicateToolCallPolicy: resources.AgentDuplicateToolCallPolicyShortCircuit,
			},
			Limits: resources.AgentLimits{MaxSteps: 5},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err != nil {
		t.Fatalf("expected success with short-circuit policy, got %v", err)
	}
	if toolRT.calls["tool.alpha"] != 1 {
		t.Fatalf("expected duplicate call to be short-circuited, got tool.alpha calls=%d", toolRT.calls["tool.alpha"])
	}
	if strings.TrimSpace(result.Output) != "DONE" {
		t.Fatalf("unexpected final output %q", result.Output)
	}
}

func TestTaskExecutorContractModeDuplicateDeny(t *testing.T) {
	toolRT := &countingToolRuntime{}
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "first call",
				ToolCalls: []ModelToolCall{
					{Name: "tool.alpha", Input: `{"q":"one"}`},
				},
			},
			2: {
				Content: "duplicate call",
				ToolCalls: []ModelToolCall{
					{Name: "tool.alpha", Input: `{"q":"one"}`},
				},
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "contract-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "contract prompt",
			Tools:  []string{"tool.alpha"},
			Execution: resources.AgentExecutionSpec{
				Profile:                 resources.AgentExecutionProfileContract,
				ToolSequence:            []string{"tool.alpha"},
				RequiredOutputMarkers:   []string{"DONE"},
				DuplicateToolCallPolicy: resources.AgentDuplicateToolCallPolicyDeny,
			},
			Limits: resources.AgentLimits{MaxSteps: 4},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err == nil {
		t.Fatalf("expected contract violation error, got success events=%+v", result.StepEvents)
	}
	toolErr, ok := AsToolError(err)
	if !ok {
		t.Fatalf("expected ToolError, got %v", err)
	}
	if toolErr.Retryable {
		t.Fatal("expected duplicate deny contract violation to be non-retryable")
	}
	if toolErr.Code != ToolCodeRuntimePolicyInvalid {
		t.Fatalf("expected code %q, got %q", ToolCodeRuntimePolicyInvalid, toolErr.Code)
	}
	if toolErr.Reason != ToolReasonAgentContractViolation {
		t.Fatalf("expected reason %q, got %q", ToolReasonAgentContractViolation, toolErr.Reason)
	}
	if toolRT.calls["tool.alpha"] != 1 {
		t.Fatalf("expected only initial tool call to execute, got %d", toolRT.calls["tool.alpha"])
	}
	var sawViolation bool
	for _, event := range result.StepEvents {
		if event.Type == "agent_contract_violation" {
			sawViolation = true
			break
		}
	}
	if !sawViolation {
		t.Fatal("expected agent_contract_violation event")
	}
}

func TestTaskExecutorContractModeToolNotInSequence(t *testing.T) {
	toolRT := &countingToolRuntime{}
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "calling undeclared tool",
				ToolCalls: []ModelToolCall{
					{Name: "tool.gamma", Input: `{"q":"undeclared"}`},
				},
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "contract-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "contract prompt",
			Tools:  []string{"tool.alpha", "tool.beta", "tool.gamma"},
			Execution: resources.AgentExecutionSpec{
				Profile:                 resources.AgentExecutionProfileContract,
				ToolSequence:            []string{"tool.alpha", "tool.beta"},
				RequiredOutputMarkers:   []string{"DONE"},
				DuplicateToolCallPolicy: resources.AgentDuplicateToolCallPolicyShortCircuit,
			},
			Limits: resources.AgentLimits{MaxSteps: 3},
		},
	}

	_, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err == nil {
		t.Fatal("expected contract violation for tool not in sequence")
	}
	toolErr, ok := AsToolError(err)
	if !ok {
		t.Fatalf("expected ToolError, got %v", err)
	}
	if toolErr.Retryable {
		t.Fatal("expected tool-not-in-sequence violation to be non-retryable")
	}
}

func TestTaskExecutorContractModeOutOfOrderAllowed(t *testing.T) {
	toolRT := &countingToolRuntime{}
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "calling beta first",
				ToolCalls: []ModelToolCall{
					{Name: "tool.beta", Input: `{"q":"first"}`},
				},
			},
			2: {
				Content: "calling alpha second",
				ToolCalls: []ModelToolCall{
					{Name: "tool.alpha", Input: `{"q":"second"}`},
				},
			},
			3: {
				Content: "DONE",
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "contract-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "contract prompt",
			Tools:  []string{"tool.alpha", "tool.beta"},
			Execution: resources.AgentExecutionSpec{
				Profile:               resources.AgentExecutionProfileContract,
				ToolSequence:          []string{"tool.alpha", "tool.beta"},
				RequiredOutputMarkers: []string{"DONE"},
			},
			Limits: resources.AgentLimits{MaxSteps: 5},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err != nil {
		t.Fatalf("expected out-of-order tools to succeed with set-based tracking, got %v", err)
	}
	if toolRT.calls["tool.alpha"] != 1 || toolRT.calls["tool.beta"] != 1 {
		t.Fatalf("expected one call per tool, got alpha=%d beta=%d", toolRT.calls["tool.alpha"], toolRT.calls["tool.beta"])
	}
	if strings.TrimSpace(result.Output) != "DONE" {
		t.Fatalf("unexpected final output %q", result.Output)
	}
}

func TestTaskExecutorContractModeMaxStepsMarkersIncompleteCompletesWithWarning(t *testing.T) {
	toolRT := &countingToolRuntime{}
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "call required tool",
				ToolCalls: []ModelToolCall{
					{Name: "tool.alpha", Input: `{"q":"one"}`},
				},
			},
			2: {
				Content: "missing marker",
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "contract-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "contract prompt",
			Tools:  []string{"tool.alpha"},
			Execution: resources.AgentExecutionSpec{
				Profile:               resources.AgentExecutionProfileContract,
				ToolSequence:          []string{"tool.alpha"},
				RequiredOutputMarkers: []string{"DONE"},
			},
			Limits: resources.AgentLimits{MaxSteps: 2},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err != nil {
		t.Fatalf("expected success with warning when tools completed but markers missing, got %v", err)
	}
	var sawWarning bool
	for _, event := range result.StepEvents {
		if event.Type == "contract_warning" {
			sawWarning = true
			break
		}
		if strings.Contains(strings.ToLower(event.Message), "contract_warning") {
			sawWarning = true
			break
		}
	}
	if !sawWarning {
		t.Fatal("expected contract_warning event when markers not satisfied at max_steps")
	}
}

func TestTaskExecutorContractModeMaxStepsToolsIncomplete(t *testing.T) {
	toolRT := &countingToolRuntime{}
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "thinking",
			},
			2: {
				Content: "still thinking",
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "contract-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "contract prompt",
			Tools:  []string{"tool.alpha"},
			Execution: resources.AgentExecutionSpec{
				Profile:               resources.AgentExecutionProfileContract,
				ToolSequence:          []string{"tool.alpha"},
				RequiredOutputMarkers: []string{"DONE"},
			},
			Limits: resources.AgentLimits{MaxSteps: 2},
		},
	}

	_, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err == nil {
		t.Fatal("expected contract violation when required tools not called")
	}
	toolErr, ok := AsToolError(err)
	if !ok {
		t.Fatalf("expected ToolError, got %v", err)
	}
	if toolErr.Reason != ToolReasonAgentContractViolation {
		t.Fatalf("expected reason %q, got %q", ToolReasonAgentContractViolation, toolErr.Reason)
	}
}

func TestTaskExecutorDynamicProfileUnaffectedByContractFields(t *testing.T) {
	toolRT := &countingToolRuntime{}
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "dynamic execution",
				ToolCalls: []ModelToolCall{
					{Name: "tool.beta", Input: `{"q":"allowed"}`},
				},
			},
			2: {
				Content: "DONE",
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "dynamic-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "dynamic prompt",
			Tools:  []string{"tool.alpha", "tool.beta"},
			Execution: resources.AgentExecutionSpec{
				Profile:      resources.AgentExecutionProfileDynamic,
				ToolSequence: []string{"tool.alpha", "tool.beta"},
			},
			Limits: resources.AgentLimits{MaxSteps: 4},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err != nil {
		t.Fatalf("expected dynamic profile success, got %v", err)
	}
	if toolRT.calls["tool.beta"] != 1 {
		t.Fatalf("expected dynamic execution to allow tool.beta call, got %d", toolRT.calls["tool.beta"])
	}
	for _, event := range result.StepEvents {
		if event.Type == "agent_contract_violation" {
			t.Fatalf("did not expect contract violation in dynamic profile: %+v", event)
		}
	}
}

func TestTaskExecutorDynamicProfileShortCircuitsDuplicates(t *testing.T) {
	toolRT := &countingToolRuntime{}
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "first call",
				ToolCalls: []ModelToolCall{
					{Name: "web_search", Input: `{"q":"one"}`},
				},
			},
			2: {
				Content: "final answer",
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "dynamic-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "test prompt",
			Tools:  []string{"web_search"},
			Limits: resources.AgentLimits{MaxSteps: 4},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err != nil {
		t.Fatalf("expected dynamic profile success, got %v", err)
	}
	if toolRT.calls["web_search"] != 1 {
		t.Fatalf("expected tool called only once, got web_search calls=%d", toolRT.calls["web_search"])
	}
	if result.Steps > 2 {
		t.Fatalf("expected completion by step 2 (tool filtered after first success), got %d steps", result.Steps)
	}
	var sawMaxSteps bool
	for _, event := range result.StepEvents {
		if strings.Contains(strings.ToLower(event.Message), "max steps reached") {
			sawMaxSteps = true
		}
	}
	if sawMaxSteps {
		t.Fatal("expected worker to complete before max_steps when all tools already succeeded")
	}
}

func TestTaskExecutorMultiToolAgentFiltersIndependently(t *testing.T) {
	toolRT := &countingToolRuntime{}
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "searching",
				ToolCalls: []ModelToolCall{
					{Name: "web_search", Input: `{"q":"cats"}`},
				},
			},
			2: {
				Content: "querying db",
				ToolCalls: []ModelToolCall{
					{Name: "database", Input: `{"sql":"SELECT 1"}`},
				},
			},
			3: {
				Content: "final answer about cats",
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "multi-tool-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "search and query",
			Tools:  []string{"web_search", "database"},
			Limits: resources.AgentLimits{MaxSteps: 4},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if toolRT.calls["web_search"] != 1 {
		t.Fatalf("expected web_search called once, got %d", toolRT.calls["web_search"])
	}
	if toolRT.calls["database"] != 1 {
		t.Fatalf("expected database called once, got %d", toolRT.calls["database"])
	}
	if result.Steps != 3 {
		t.Fatalf("expected 3 steps (search, database, text output), got %d", result.Steps)
	}
}

func TestTaskExecutorContractModeObservePolicyDoesNotError(t *testing.T) {
	toolRT := &countingToolRuntime{}
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "calling undeclared tool",
				ToolCalls: []ModelToolCall{
					{Name: "tool.gamma", Input: `{"q":"undeclared"}`},
				},
			},
			2: {
				Content: "calling required tool",
				ToolCalls: []ModelToolCall{
					{Name: "tool.alpha", Input: `{"q":"one"}`},
				},
			},
			3: {
				Content: "DONE",
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "contract-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "contract prompt",
			Tools:  []string{"tool.alpha", "tool.gamma"},
			Execution: resources.AgentExecutionSpec{
				Profile:               resources.AgentExecutionProfileContract,
				ToolSequence:          []string{"tool.alpha"},
				RequiredOutputMarkers: []string{"DONE"},
				OnContractViolation:   resources.AgentContractViolationPolicyObserve,
			},
			Limits: resources.AgentLimits{MaxSteps: 5},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err != nil {
		t.Fatalf("expected observe policy to not produce error, got %v", err)
	}
	var sawViolation bool
	for _, event := range result.StepEvents {
		if event.Type == "agent_contract_violation" {
			sawViolation = true
			break
		}
	}
	if !sawViolation {
		t.Fatal("expected agent_contract_violation event even in observe mode")
	}
	if strings.TrimSpace(result.Output) != "DONE" {
		t.Fatalf("expected output DONE, got %q", result.Output)
	}
}

func TestTaskExecutorReturnsApprovalRequiredWhenToolNeedsApproval(t *testing.T) {
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "calling tool",
				ToolCalls: []ModelToolCall{
					{ID: "call_1", Name: "smoke", Input: `{}`},
				},
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, &approvalRequiredToolRuntime{}, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "use the smoke tool once",
			Tools:  []string{"smoke"},
			Limits: resources.AgentLimits{MaxSteps: 6},
		},
	}
	_, err := executor.ExecuteAgent(context.Background(), agent, map[string]string{"topic": "t"})
	if err == nil {
		t.Fatal("expected approval required error")
	}
	if !IsApprovalRequiredError(err) {
		t.Fatalf("expected ErrToolApprovalRequired chain, got %v", err)
	}
}

func TestTaskExecutorStepEventsCaptureToolContractMetadata(t *testing.T) {
	executor := NewTaskExecutorWithRuntime(nil, &staticToolRuntime{}, &MockModelGateway{}, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "research", Namespace: "default"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "test prompt",
			Tools:  []string{"web_search"},
			Limits: resources.AgentLimits{MaxSteps: 1},
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
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "writer"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o-mini",
			ModelRef: "test-endpoint",
			Prompt: "write summary",
			Limits: resources.AgentLimits{MaxSteps: 4},
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

func TestTaskExecutorStopOnFirstToolCompletesImmediately(t *testing.T) {
	toolRT := &countingToolRuntime{}
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "calling tool",
				ToolCalls: []ModelToolCall{
					{ID: "call_1", Name: "web_search", Input: `{"q":"cats"}`},
				},
			},
			2: {
				Content: "should not be reached",
			},
		},
	}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "pipeline-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "search once",
			Tools:  []string{"web_search"},
			Execution: resources.AgentExecutionSpec{
				ToolUseBehavior: resources.AgentToolUseBehaviorStopOnFirstTool,
			},
			Limits: resources.AgentLimits{MaxSteps: 4},
		},
	}

	result, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if toolRT.calls["web_search"] != 1 {
		t.Fatalf("expected web_search called once, got %d", toolRT.calls["web_search"])
	}
	if result.Steps != 1 {
		t.Fatalf("expected completion on step 1 (stop_on_first_tool), got %d steps", result.Steps)
	}
	var sawStopEvent bool
	for _, event := range result.StepEvents {
		if strings.Contains(event.Message, "stop_on_first_tool") {
			sawStopEvent = true
		}
	}
	if !sawStopEvent {
		t.Fatal("expected stop_on_first_tool event")
	}
}

func TestTaskExecutorStructuredToolHistoryPreservesIDs(t *testing.T) {
	var capturedMessages []ChatMessage
	model := &scriptedModelGateway{
		responses: map[int]ModelResponse{
			1: {
				Content: "calling tool",
				ToolCalls: []ModelToolCall{
					{ID: "call_abc123", Name: "web_search", Input: `{"q":"test"}`},
				},
			},
			2: {
				Content: "final answer",
			},
		},
		captureMessages: &capturedMessages,
	}
	toolRT := &countingToolRuntime{}
	executor := NewTaskExecutorWithRuntime(nil, toolRT, model, nil)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "structured-agent"},
		Spec: resources.AgentSpec{
			Model:    "gpt-4o",
			ModelRef: "test-endpoint",
			Prompt: "test",
			Tools:  []string{"web_search"},
			Limits: resources.AgentLimits{MaxSteps: 3},
		},
	}

	_, err := executor.ExecuteAgent(context.Background(), agent, nil)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if capturedMessages == nil {
		t.Skip("scriptedModelGateway does not capture messages")
	}
	var foundToolMsg, foundAssistantToolCalls bool
	for _, msg := range capturedMessages {
		if msg.Role == "tool" && msg.ToolCallID == "call_abc123" {
			foundToolMsg = true
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				if tc.ID == "call_abc123" && tc.Name == "web_search" {
					foundAssistantToolCalls = true
				}
			}
		}
	}
	if !foundToolMsg {
		t.Fatal("expected tool message with ToolCallID=call_abc123 in history")
	}
	if !foundAssistantToolCalls {
		t.Fatal("expected assistant message with ToolCalls containing call_abc123")
	}
}

func TestParseAgentStepEventsCapturesLatencyFields(t *testing.T) {
	events := []observedAgentEvent{
		{
			Timestamp: "2026-03-18T00:00:00Z",
			Message:   "step=1 model success tokens=42 input_tokens=30 output_tokens=12 usage_source=provider latency_ms=27",
		},
		{
			Timestamp: "2026-03-18T00:00:01Z",
			Message:   "step=1 tool=memory.write tool_contract=v1 tool_request_id=req-1 tool_attempt=1 duration_ms=14 success",
		},
	}

	parsed := parseAgentStepEvents(events)
	if len(parsed) != 2 {
		t.Fatalf("expected 2 parsed events, got %d", len(parsed))
	}
	if parsed[0].Type != "model_call" {
		t.Fatalf("expected first event type model_call, got %q", parsed[0].Type)
	}
	if parsed[0].LatencyMS != 27 {
		t.Fatalf("expected model_call latency_ms=27, got %d", parsed[0].LatencyMS)
	}
	if parsed[0].InputTokens != 30 {
		t.Fatalf("expected model_call input_tokens=30, got %d", parsed[0].InputTokens)
	}
	if parsed[0].OutputTokens != 12 {
		t.Fatalf("expected model_call output_tokens=12, got %d", parsed[0].OutputTokens)
	}
	if parsed[1].Type != "tool_call" {
		t.Fatalf("expected second event type tool_call, got %q", parsed[1].Type)
	}
	if parsed[1].LatencyMS != 14 {
		t.Fatalf("expected tool_call latency_ms=14, got %d", parsed[1].LatencyMS)
	}
}
