package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

// AgentWorker runs the core execution loop for one agent.
type AgentWorker struct {
	agent        resources.Agent
	toolRuntime  ToolRuntime
	memory       MemoryStore
	modelGateway ModelGateway
	onEvent      func(string)
	stepEvery    time.Duration
	input        map[string]string
	toolSchemas  map[string]ToolSchemaInfo
	history      []ChatMessage
}

func NewAgentWorker(agent resources.Agent, toolRuntime ToolRuntime, memory MemoryStore, onEvent func(string)) *AgentWorker {
	return NewAgentWorkerWithInterval(agent, toolRuntime, memory, onEvent, 2*time.Second)
}

func NewAgentWorkerWithInterval(agent resources.Agent, toolRuntime ToolRuntime, memory MemoryStore, onEvent func(string), stepEvery time.Duration) *AgentWorker {
	return NewAgentWorkerWithIntervalAndGatewayAndInput(agent, toolRuntime, memory, &MockModelGateway{}, nil, onEvent, stepEvery)
}

func NewAgentWorkerWithIntervalAndGateway(
	agent resources.Agent,
	toolRuntime ToolRuntime,
	memory MemoryStore,
	modelGateway ModelGateway,
	onEvent func(string),
	stepEvery time.Duration,
) *AgentWorker {
	return NewAgentWorkerWithIntervalAndGatewayAndInput(agent, toolRuntime, memory, modelGateway, nil, onEvent, stepEvery)
}

func NewAgentWorkerWithIntervalAndGatewayAndInput(
	agent resources.Agent,
	toolRuntime ToolRuntime,
	memory MemoryStore,
	modelGateway ModelGateway,
	input map[string]string,
	onEvent func(string),
	stepEvery time.Duration,
) *AgentWorker {
	if stepEvery <= 0 {
		stepEvery = 2 * time.Second
	}
	if toolRuntime == nil {
		toolRuntime = &MockToolClient{}
	}
	if memory == nil {
		memory = NewMemoryManager()
	}
	if modelGateway == nil {
		modelGateway = &MockModelGateway{}
	}
	return &AgentWorker{
		agent:        agent,
		toolRuntime:  toolRuntime,
		memory:       memory,
		modelGateway: modelGateway,
		onEvent:      onEvent,
		stepEvery:    stepEvery,
		input:        copyStringMap(input),
	}
}

// SetToolSchemas attaches per-tool description and JSON Schema metadata.
// Model gateways use these to provide rich tool definitions to the LLM
// instead of the generic {input: string} fallback.
func (w *AgentWorker) SetToolSchemas(schemas map[string]ToolSchemaInfo) {
	w.toolSchemas = schemas
}

func (w *AgentWorker) Run(ctx context.Context) {
	maxSteps := w.agent.Spec.Limits.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 10
	}
	contractEnabled := strings.EqualFold(strings.TrimSpace(w.agent.Spec.Execution.Profile), resources.AgentExecutionProfileContract)
	contractSequence := append([]string(nil), w.agent.Spec.Execution.ToolSequence...)
	contractRequiredMarkers := append([]string(nil), w.agent.Spec.Execution.RequiredOutputMarkers...)
	duplicatePolicy := strings.TrimSpace(w.agent.Spec.Execution.DuplicateToolCallPolicy)
	if duplicatePolicy == "" {
		duplicatePolicy = resources.AgentDuplicateToolCallPolicyShortCircuit
	}
	violationPolicy := strings.TrimSpace(w.agent.Spec.Execution.OnContractViolation)
	if violationPolicy == "" {
		violationPolicy = resources.AgentContractViolationPolicyNonRetryableError
	}
	toolUseBehavior := strings.TrimSpace(w.agent.Spec.Execution.ToolUseBehavior)

	contractRemaining := make(map[string]bool, len(contractSequence))
	contractSequenceSet := make(map[string]bool, len(contractSequence))
	for _, t := range contractSequence {
		key := normalizeToolKey(strings.TrimSpace(t))
		contractRemaining[key] = true
		contractSequenceSet[key] = true
	}

	toolResultCache := make(map[string]string)
	toolCalled := make(map[string]bool)

	normalizeContractMessage := func(message string) string {
		message = strings.TrimSpace(message)
		if message == "" {
			return "contract violation"
		}
		return strings.Join(strings.Fields(message), " ")
	}
	markersSatisfied := func(output string) bool {
		if len(contractRequiredMarkers) == 0 {
			return true
		}
		if strings.TrimSpace(output) == "" {
			return false
		}
		for _, marker := range contractRequiredMarkers {
			if !strings.Contains(output, marker) {
				return false
			}
		}
		return true
	}
	emitContractViolation := func(step int, message string) bool {
		if w.onEvent != nil {
			w.onEvent(fmt.Sprintf(
				"step=%d agent_contract_violation tool_code=%s tool_reason=%s retryable=false message=%s",
				step,
				ToolCodeRuntimePolicyInvalid,
				ToolReasonAgentContractViolation,
				normalizeContractMessage(message),
			))
		}
		if strings.EqualFold(violationPolicy, resources.AgentContractViolationPolicyNonRetryableError) {
			if w.onEvent != nil {
				w.onEvent("worker stopped contract violation")
			}
			return true
		}
		return false
	}
	if w.onEvent != nil {
		w.onEvent(fmt.Sprintf("worker started model=%s max_steps=%d", w.agent.Spec.Model, maxSteps))
	}

	if prompt := strings.TrimSpace(w.agent.Spec.Prompt); prompt != "" {
		w.history = append(w.history, ChatMessage{Role: "system", Content: prompt})
	}

	ticker := time.NewTicker(w.stepEvery)
	defer ticker.Stop()

	for step := 1; step <= maxSteps; step++ {
		select {
		case <-ctx.Done():
			if w.onEvent != nil {
				w.onEvent("worker stopped")
			}
			return
		case <-ticker.C:
			availableTools := w.agent.Spec.Tools
			if strings.EqualFold(duplicatePolicy, resources.AgentDuplicateToolCallPolicyShortCircuit) && len(toolCalled) > 0 {
				filtered := make([]string, 0, len(w.agent.Spec.Tools))
				for _, t := range w.agent.Spec.Tools {
					if !toolCalled[normalizeToolKey(t)] {
						filtered = append(filtered, t)
					}
				}
				availableTools = filtered
			}

			w.history = append(w.history, ChatMessage{
				Role:    "user",
				Content: buildOpenAIUserContent(ModelRequest{Step: step, Tools: availableTools, Context: w.modelContext(step)}),
			})
			modelStart := time.Now()
			modelResp, modelErr := w.modelGateway.Complete(ctx, ModelRequest{
				Model:       w.agent.Spec.Model,
				ModelRef:    w.agent.Spec.ModelRef,
				Namespace:   w.agent.Metadata.Namespace,
				Agent:       w.agent.Metadata.Name,
				Prompt:      w.agent.Spec.Prompt,
				Step:        step,
				Tools:       append([]string(nil), availableTools...),
				ToolSchemas: w.toolSchemas,
				Context:     w.modelContext(step),
				Messages:    append([]ChatMessage(nil), w.history...),
			})
			modelLatencyMS := time.Since(modelStart).Milliseconds()
			if modelErr != nil {
				if w.onEvent != nil {
					w.onEvent(fmt.Sprintf("step=%d model_error=%v latency_ms=%d", step, modelErr, modelLatencyMS))
				}
				w.history = w.history[:len(w.history)-1]
				continue
			}
			modelOutput := strings.TrimSpace(modelResp.Content)
			modelUsage := normalizeModelUsageWithFallback(modelResp.Usage, w.agent, modelResp, step)
			if w.onEvent != nil {
				w.onEvent(fmt.Sprintf(
					"step=%d model success tokens=%d input_tokens=%d output_tokens=%d usage_source=%s latency_ms=%d",
					step,
					modelUsage.TotalTokens,
					modelUsage.InputTokens,
					modelUsage.OutputTokens,
					modelUsage.Source,
					modelLatencyMS,
				))
				if modelOutput != "" {
					w.onEvent(fmt.Sprintf("step=%d model_output=%s", step, modelOutput))
				}
			}

			if len(modelResp.ToolCalls) > 0 {
				chatCalls := make([]ChatToolCall, len(modelResp.ToolCalls))
				for i, tc := range modelResp.ToolCalls {
					chatCalls[i] = ChatToolCall{ID: tc.ID, Name: tc.Name, Input: tc.Input}
				}
				w.history = append(w.history, ChatMessage{
					Role: "assistant", Content: modelOutput, ToolCalls: chatCalls,
				})
			} else if modelOutput != "" {
				w.history = append(w.history, ChatMessage{Role: "assistant", Content: modelOutput})
			}
			if contractEnabled && len(contractRemaining) == 0 && markersSatisfied(modelOutput) {
				if w.onEvent != nil {
					w.onEvent("worker completed")
				}
				return
			}

			if len(availableTools) == 0 {
				if w.onEvent != nil {
					w.onEvent(fmt.Sprintf("step=%d no tools configured", step))
				}
				if modelOutput != "" {
					if contractEnabled {
						if len(contractRemaining) > 0 {
							next := anyMapKey(contractRemaining)
							if emitContractViolation(step, fmt.Sprintf("expected tool=%s before completion", next)) {
								return
							}
						}
						if !markersSatisfied(modelOutput) {
							continue
						}
					}
					if w.onEvent != nil {
						w.onEvent("worker completed")
					}
					return
				}
				continue
			}

			requestedCalls, selectErr := selectAuthorizedToolCalls(modelResp, availableTools)
			if selectErr != nil {
				failedTool := "model_tool_selection"
				if toolErr, ok := AsToolError(selectErr); ok {
					if toolName := strings.TrimSpace(toolErr.Details["tool"]); toolName != "" {
						failedTool = toolName
					}
				}
				if w.onEvent != nil {
					if code, reason, retryable, ok := ToolErrorMeta(selectErr); ok {
						status := ToolStatusError
						if IsToolDeniedError(selectErr) {
							status = ToolStatusDenied
						}
						reqID := fmt.Sprintf(
							"%s/%s/s%03d/%s",
							resources.NormalizeNamespace(w.agent.Metadata.Namespace),
							strings.TrimSpace(w.agent.Metadata.Name),
							step,
							normalizeToolKey(failedTool),
						)
						w.onEvent(fmt.Sprintf("step=%d tool=%s tool_contract=%s tool_request_id=%s tool_attempt=%d status=%s tool_code=%s tool_reason=%s retryable=%t error=%s", step, failedTool, ToolContractVersionV1, reqID, 1, status, code, reason, retryable, selectErr))
					}
				}
				if IsToolDeniedError(selectErr) || errors.Is(selectErr, ErrToolPermissionDenied) || strings.Contains(strings.ToLower(selectErr.Error()), "permission denied") {
					if w.onEvent != nil {
						w.onEvent(fmt.Sprintf("step=%d tool=%s permission denied error=%v", step, failedTool, selectErr))
						w.onEvent("worker stopped permission denied")
					}
					return
				}
				if IsApprovalRequiredError(selectErr) {
					if w.onEvent != nil {
						w.onEvent(fmt.Sprintf("step=%d tool=%s approval required error=%v", step, failedTool, selectErr))
						w.onEvent("worker stopped approval required")
					}
					return
				}
				continue
			}
			if len(requestedCalls) == 0 {
				if w.onEvent != nil {
					w.onEvent(fmt.Sprintf("step=%d no tool call requested", step))
				}
				if modelOutput != "" && !contractEnabled {
					if w.onEvent != nil {
						w.onEvent("worker completed")
					}
					return
				}
				continue
			}

			for _, requested := range requestedCalls {
				tool := strings.TrimSpace(requested.Name)
				if tool == "" {
					continue
				}
				toolKey := normalizeToolKey(tool)

				input := strings.TrimSpace(requested.Input)
				if input == "" {
					input = fmt.Sprintf("agent=%s step=%d", w.agent.Metadata.Name, step)
				}
				cacheKey := toolKey + "\x00" + input

				if toolCalled[toolKey] {
					if contractEnabled && strings.EqualFold(duplicatePolicy, resources.AgentDuplicateToolCallPolicyDeny) {
						if emitContractViolation(step, fmt.Sprintf("duplicate successful tool call denied tool=%s", tool)) {
							return
						}
						continue
					}
					if priorResult, seen := toolResultCache[cacheKey]; seen {
						w.memory.Put(fmt.Sprintf("%s:%d", tool, step), priorResult)
						if w.onEvent != nil {
							w.onEvent(fmt.Sprintf("step=%d tool=%s tool_contract=%s tool_request_id=%s tool_attempt=%d duration_ms=%d success short_circuit=true", step, tool, ToolContractVersionV1, fmt.Sprintf("%s/%s/s%03d/%s", resources.NormalizeNamespace(w.agent.Metadata.Namespace), strings.TrimSpace(w.agent.Metadata.Name), step, normalizeToolKey(tool)), 1, 0))
						}
						w.history = append(w.history, ChatMessage{
							Role:       "tool",
							Content:    priorResult,
							ToolCallID: requested.ID,
						})
						continue
					}
				}

				if contractEnabled && !contractSequenceSet[toolKey] {
					if emitContractViolation(step, fmt.Sprintf("tool not in declared sequence tool=%s", tool)) {
						return
					}
				}
				reqID := fmt.Sprintf(
					"%s/%s/s%03d/%s",
					resources.NormalizeNamespace(w.agent.Metadata.Namespace),
					strings.TrimSpace(w.agent.Metadata.Name),
					step,
					normalizeToolKey(tool),
				)
				toolStart := time.Now()
				response, execErr := ExecuteToolContract(ctx, w.toolRuntime, ToolExecutionRequest{
					ToolContractVersion: ToolContractVersionV1,
					RequestID:           reqID,
					Namespace:           w.agent.Metadata.Namespace,
					Agent:               w.agent.Metadata.Name,
					Tool: ToolExecutionRequestTool{
						Name:      tool,
						Operation: ToolOperationInvoke,
					},
					InputRaw: input,
					Attempt:  1,
				})
				result := response.Output.Result
				err := execErr
				if err == nil {
					err = response.ToError()
				}
				contractVersion := strings.TrimSpace(response.ToolContractVersion)
				if contractVersion == "" {
					contractVersion = ToolContractVersionV1
				}
				toolRequestID := strings.TrimSpace(response.RequestID)
				if toolRequestID == "" {
					toolRequestID = reqID
				}
				toolAttempt := response.Usage.Attempt
				if toolAttempt <= 0 {
					toolAttempt = 1
				}
				toolDurationMS := response.Usage.DurationMS
				if toolDurationMS <= 0 {
					toolDurationMS = time.Since(toolStart).Milliseconds()
				}
				if err != nil {
					if code, reason, retryable, ok := ToolErrorMeta(err); ok {
						status := ToolStatusError
						if IsToolDeniedError(err) {
							status = ToolStatusDenied
						}
						if w.onEvent != nil {
							w.onEvent(fmt.Sprintf("step=%d tool=%s tool_contract=%s tool_request_id=%s tool_attempt=%d status=%s tool_code=%s tool_reason=%s retryable=%t duration_ms=%d error=%s", step, tool, contractVersion, toolRequestID, toolAttempt, status, code, reason, retryable, toolDurationMS, err))
						}
					}
					if IsToolDeniedError(err) || errors.Is(err, ErrToolPermissionDenied) || strings.Contains(strings.ToLower(err.Error()), "permission denied") {
						if w.onEvent != nil {
							w.onEvent(fmt.Sprintf("step=%d tool=%s tool_contract=%s tool_request_id=%s tool_attempt=%d permission denied duration_ms=%d error=%v", step, tool, contractVersion, toolRequestID, toolAttempt, toolDurationMS, err))
							w.onEvent("worker stopped permission denied")
						}
						return
					}
					if IsApprovalRequiredError(err) {
						if w.onEvent != nil {
							w.onEvent(fmt.Sprintf("step=%d tool=%s tool_contract=%s tool_request_id=%s tool_attempt=%d duration_ms=%d error=%v", step, tool, contractVersion, toolRequestID, toolAttempt, toolDurationMS, err))
							w.onEvent("worker stopped approval required")
						}
						return
					}
					if w.onEvent != nil {
						w.onEvent(fmt.Sprintf("step=%d tool=%s tool_contract=%s tool_request_id=%s tool_attempt=%d duration_ms=%d error=%v", step, tool, contractVersion, toolRequestID, toolAttempt, toolDurationMS, err))
					}
					continue
				}
				toolResultCache[cacheKey] = result
				toolCalled[toolKey] = true
				if contractEnabled {
					delete(contractRemaining, toolKey)
				}
				w.memory.Put(fmt.Sprintf("%s:%d", tool, step), result)
				if w.onEvent != nil {
					w.onEvent(fmt.Sprintf("step=%d tool=%s tool_contract=%s tool_request_id=%s tool_attempt=%d duration_ms=%d success", step, tool, contractVersion, toolRequestID, toolAttempt, toolDurationMS))
				}
				w.history = append(w.history, ChatMessage{
					Role:       "tool",
					Content:    result,
					ToolCallID: requested.ID,
				})
				if strings.EqualFold(toolUseBehavior, resources.AgentToolUseBehaviorStopOnFirstTool) {
					if w.onEvent != nil {
						w.onEvent(fmt.Sprintf("step=%d tool=%s stop_on_first_tool", step, tool))
						w.onEvent("worker completed")
					}
					return
				}
			}
			if contractEnabled && len(contractRemaining) == 0 && markersSatisfied(modelOutput) {
				if w.onEvent != nil {
					w.onEvent("worker completed")
				}
				return
			}
		}
	}

	if contractEnabled {
		if len(contractRemaining) > 0 {
			next := anyMapKey(contractRemaining)
			if emitContractViolation(maxSteps, fmt.Sprintf("max_steps reached before tool sequence completion expected tool=%s", next)) {
				return
			}
		} else {
			if w.onEvent != nil {
				w.onEvent("contract_warning max_steps reached before required output markers were satisfied")
				w.onEvent("worker completed")
			}
			return
		}
	}
	if w.onEvent != nil {
		w.onEvent("max steps reached")
	}
}

func anyMapKey(m map[string]bool) string {
	for key := range m {
		return key
	}
	return "unknown"
}

func normalizeModelUsageWithFallback(usage ModelUsage, agent resources.Agent, resp ModelResponse, step int) ModelUsage {
	normalized := normalizeModelUsage(usage)
	if normalized.TotalTokens > 0 {
		return normalized
	}
	estimated := estimateModelCallTokens(agent, resp, step)
	return ModelUsage{
		TotalTokens: estimated,
		Source:      "estimated",
	}
}

func normalizeModelUsage(usage ModelUsage) ModelUsage {
	usage.InputTokens = max(0, usage.InputTokens)
	usage.OutputTokens = max(0, usage.OutputTokens)
	usage.TotalTokens = max(0, usage.TotalTokens)
	usage.Source = strings.ToLower(strings.TrimSpace(usage.Source))
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = 0
	}
	if usage.Source == "" {
		usage.Source = "provider"
	}
	return usage
}

func estimateModelCallTokens(agent resources.Agent, resp ModelResponse, step int) int {
	promptTokens := len([]rune(strings.TrimSpace(agent.Spec.Prompt))) / 4
	outputTokens := len([]rune(strings.TrimSpace(resp.Content))) / 4
	toolTokens := 0
	for _, call := range resp.ToolCalls {
		toolTokens += 8
		toolTokens += len([]rune(strings.TrimSpace(call.Name))) / 4
		toolTokens += len([]rune(strings.TrimSpace(call.Input))) / 4
	}
	// Keep a stable floor so successful calls never report zero usage.
	total := 12 + promptTokens + outputTokens + toolTokens + (step * 2)
	if total < 1 {
		return 1
	}
	return total
}

func (w *AgentWorker) modelContext(step int) map[string]string {
	context := map[string]string{
		"agent":     w.agent.Metadata.Name,
		"namespace": w.agent.Metadata.Namespace,
		"model_ref": w.agent.Spec.ModelRef,
		"step":      strconv.Itoa(step),
	}
	for key, value := range w.input {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		context[key] = value
	}
	return context
}

type workerHandle struct {
	agent  resources.Agent
	cancel context.CancelFunc
	done   chan struct{}
}

// Manager tracks and reconciles running workers.
type Manager struct {
	mu           sync.RWMutex
	workers      map[string]workerHandle
	logs         map[string][]string
	toolRuntime  ToolRuntime
	modelGateway ModelGateway
	logger       *log.Logger
}

func NewManager(logger *log.Logger) *Manager {
	return &Manager{
		workers:      make(map[string]workerHandle),
		logs:         make(map[string][]string),
		toolRuntime:  &MockToolClient{},
		modelGateway: &MockModelGateway{},
		logger:       logger,
	}
}

func (m *Manager) EnsureRunning(agent resources.Agent) {
	if err := agent.Normalize(); err != nil {
		m.recordLog(agent.Metadata.Name, fmt.Sprintf("invalid agent manifest: %v", err))
		return
	}
	runtimeKey := agentRuntimeKey(agent.Metadata.Namespace, agent.Metadata.Name)

	m.mu.RLock()
	handle, exists := m.workers[runtimeKey]
	m.mu.RUnlock()

	if exists && reflect.DeepEqual(handle.agent.Spec, agent.Spec) {
		return
	}

	if exists {
		m.Stop(runtimeKey)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	worker := NewAgentWorkerWithIntervalAndGateway(agent, m.toolRuntime, NewMemoryManager(), m.modelGateway, func(msg string) {
		m.recordLog(runtimeKey, msg)
	}, 2*time.Second)

	m.mu.Lock()
	m.workers[runtimeKey] = workerHandle{agent: agent, cancel: cancel, done: done}
	m.mu.Unlock()

	go func() {
		defer close(done)
		worker.Run(ctx)
		m.mu.Lock()
		if current, ok := m.workers[runtimeKey]; ok {
			if current.done == done {
				delete(m.workers, runtimeKey)
			}
		}
		m.mu.Unlock()
	}()
}

func (m *Manager) Stop(name string) {
	key := normalizeRuntimeName(name)
	m.mu.Lock()
	handle, ok := m.workers[key]
	if ok {
		delete(m.workers, key)
	}
	m.mu.Unlock()

	if ok {
		handle.cancel()
		m.recordLog(key, "worker stop requested")
	}
}

func (m *Manager) IsRunning(name string) bool {
	key := normalizeRuntimeName(name)
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.workers[key]
	return ok
}

func (m *Manager) RunningAgents() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.workers))
	for name := range m.workers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m *Manager) Logs(name string) []string {
	key := normalizeRuntimeName(name)
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := m.logs[key]
	out := make([]string, len(entries))
	copy(out, entries)
	return out
}

func (m *Manager) recordLog(name, msg string) {
	name = normalizeRuntimeName(name)
	if name == "" {
		name = "unknown"
	}
	entry := fmt.Sprintf("%s %s", time.Now().UTC().Format(time.RFC3339), msg)

	m.mu.Lock()
	m.logs[name] = append(m.logs[name], entry)
	if len(m.logs[name]) > 200 {
		m.logs[name] = m.logs[name][len(m.logs[name])-200:]
	}
	m.mu.Unlock()

	if m.logger != nil {
		m.logger.Printf("agent=%s %s", name, msg)
	}
}

func agentRuntimeKey(namespace, name string) string {
	return resources.NormalizeNamespace(namespace) + "/" + strings.TrimSpace(name)
}

func normalizeRuntimeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return agentRuntimeKey(resources.DefaultNamespace, "")
	}
	if strings.Contains(name, "/") {
		parts := strings.SplitN(name, "/", 2)
		return agentRuntimeKey(parts[0], parts[1])
	}
	return agentRuntimeKey(resources.DefaultNamespace, name)
}
