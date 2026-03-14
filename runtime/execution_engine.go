package agentruntime

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AnonJon/orloj/crds"
)

// ExecutionEngine orchestrates one agent execution loop.
type ExecutionEngine interface {
	Execute(ctx context.Context, agent crds.Agent, input map[string]string) (AgentExecutionResult, error)
}

// ReActExecutionEngine is the default runtime engine: model call + optional tool actions in bounded steps.
type ReActExecutionEngine struct {
	toolRuntime    ToolRuntime
	modelGateway   ModelGateway
	newMemoryStore func() MemoryStore
	stepEvery      time.Duration
}

func NewReActExecutionEngine(
	toolRuntime ToolRuntime,
	modelGateway ModelGateway,
	newMemoryStore func() MemoryStore,
	stepEvery time.Duration,
) *ReActExecutionEngine {
	if toolRuntime == nil {
		toolRuntime = &MockToolClient{}
	}
	if modelGateway == nil {
		modelGateway = &MockModelGateway{}
	}
	if newMemoryStore == nil {
		newMemoryStore = func() MemoryStore { return NewMemoryManager() }
	}
	if stepEvery <= 0 {
		stepEvery = 25 * time.Millisecond
	}
	return &ReActExecutionEngine{
		toolRuntime:    toolRuntime,
		modelGateway:   modelGateway,
		newMemoryStore: newMemoryStore,
		stepEvery:      stepEvery,
	}
}

func (e *ReActExecutionEngine) Execute(ctx context.Context, agent crds.Agent, input map[string]string) (AgentExecutionResult, error) {
	if err := agent.Normalize(); err != nil {
		return AgentExecutionResult{}, err
	}

	runCtx := ctx
	cancel := func() {}
	if strings.TrimSpace(agent.Spec.Limits.Timeout) != "" {
		timeout, err := time.ParseDuration(agent.Spec.Limits.Timeout)
		if err != nil {
			return AgentExecutionResult{}, fmt.Errorf("invalid agent timeout %q: %w", agent.Spec.Limits.Timeout, err)
		}
		if timeout > 0 {
			runCtx, cancel = context.WithTimeout(ctx, timeout)
		}
	}
	defer cancel()

	start := time.Now()
	observed := make([]observedAgentEvent, 0, 64)
	appendObserved := func(msg string) {
		if len(observed) >= 300 {
			return
		}
		observed = append(observed, observedAgentEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Message:   msg,
		})
	}

	if from := strings.TrimSpace(input["inbox.from"]); from != "" {
		content := strings.TrimSpace(input["inbox.content"])
		inboxEvent := fmt.Sprintf("inbox from=%s", from)
		if content != "" {
			inboxEvent += " content=" + content
		}
		appendObserved(inboxEvent)
	}

	memory := e.newMemoryStore()
	worker := NewAgentWorkerWithIntervalAndGatewayAndInput(agent, e.toolRuntime, memory, e.modelGateway, input, appendObserved, e.stepEvery)
	worker.Run(runCtx)

	duration := time.Since(start)
	rawEvents := observedMessages(observed)
	stepEvents := parseAgentStepEvents(observed)
	steps := maxStep(rawEvents)
	toolCalls := countToolSuccesses(rawEvents)
	result := AgentExecutionResult{
		Agent:           agent.Metadata.Name,
		Model:           agent.Spec.Model,
		Steps:           steps,
		ToolCalls:       toolCalls,
		MemoryWrites:    len(memory.Snapshot()),
		EstimatedTokens: estimateTokens(agent, steps, toolCalls),
		Duration:        duration,
		Events:          rawEvents,
		StepEvents:      stepEvents,
	}
	result.TokensUsed, result.TokenSource = resolveTokenUsage(result.EstimatedTokens, stepEvents)
	if len(rawEvents) > 0 {
		result.LastEvent = rawEvents[len(rawEvents)-1]
	}
	result.Output = preferredAgentOutput(stepEvents)
	if denied, ok := firstToolPermissionDenied(stepEvents); ok {
		return result, fmt.Errorf(
			"%w: agent=%s step=%d tool=%s error=%s",
			ErrToolPermissionDenied,
			agent.Metadata.Name,
			denied.Step,
			denied.Tool,
			strings.TrimSpace(denied.Message),
		)
	}
	if modelErr := allModelCallsFailed(stepEvents); modelErr != "" {
		return result, fmt.Errorf("agent %s model execution failed: %s", agent.Metadata.Name, modelErr)
	}

	if runCtx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("agent %s timed out after %s", agent.Metadata.Name, agent.Spec.Limits.Timeout)
	}
	if runCtx.Err() != nil && runCtx.Err() != context.Canceled {
		return result, runCtx.Err()
	}
	return result, nil
}

func estimateTokens(agent crds.Agent, steps int, toolCalls int) int {
	promptTokens := 1 + len([]rune(agent.Spec.Prompt))/4
	stepTokens := steps * 120
	toolTokens := toolCalls * 40
	return promptTokens + stepTokens + toolTokens
}

func maxStep(events []string) int {
	max := 0
	for _, event := range events {
		matches := stepRegex.FindStringSubmatch(event)
		if len(matches) < 2 {
			continue
		}
		n, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return max
}

func countToolSuccesses(events []string) int {
	count := 0
	for _, event := range events {
		if strings.Contains(event, "tool=") && strings.Contains(event, "success") {
			count++
		}
	}
	return count
}

func firstToolPermissionDenied(events []AgentStepEvent) (AgentStepEvent, bool) {
	for _, event := range events {
		if strings.EqualFold(strings.TrimSpace(event.Type), "tool_permission_denied") {
			return event, true
		}
	}
	return AgentStepEvent{}, false
}

func preferredAgentOutput(events []AgentStepEvent) string {
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if !strings.EqualFold(strings.TrimSpace(event.Type), "model_output") {
			continue
		}
		value := strings.TrimSpace(event.Message)
		if value != "" {
			return value
		}
	}
	return ""
}

func allModelCallsFailed(events []AgentStepEvent) string {
	if len(events) == 0 {
		return ""
	}
	var sawModelCall bool
	var sawModelError bool
	var firstErr string
	for _, event := range events {
		switch strings.ToLower(strings.TrimSpace(event.Type)) {
		case "model_call":
			sawModelCall = true
		case "model_error":
			sawModelError = true
			if strings.TrimSpace(firstErr) == "" {
				firstErr = strings.TrimSpace(event.Message)
			}
		}
	}
	if sawModelCall || !sawModelError {
		return ""
	}
	if firstErr == "" {
		firstErr = "model gateway request failed"
	}
	return firstErr
}

func resolveTokenUsage(estimated int, events []AgentStepEvent) (int, string) {
	total := 0
	sources := map[string]struct{}{}
	for _, event := range events {
		if !strings.EqualFold(strings.TrimSpace(event.Type), "model_call") {
			continue
		}
		if event.Tokens <= 0 {
			continue
		}
		total += event.Tokens
		source := strings.ToLower(strings.TrimSpace(event.UsageSource))
		if source == "" {
			source = "provider"
		}
		sources[source] = struct{}{}
	}
	if total > 0 {
		return total, classifyUsageSources(sources)
	}
	if estimated > 0 {
		return estimated, "fixed"
	}
	return 0, "unknown"
}

func classifyUsageSources(sources map[string]struct{}) string {
	if len(sources) == 0 {
		return "unknown"
	}
	if len(sources) > 1 {
		return "mixed"
	}
	for source := range sources {
		return source
	}
	return "unknown"
}
