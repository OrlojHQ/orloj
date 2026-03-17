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

func (w *AgentWorker) Run(ctx context.Context) {
	maxSteps := w.agent.Spec.Limits.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 10
	}
	if w.onEvent != nil {
		w.onEvent(fmt.Sprintf("worker started model=%s max_steps=%d", w.agent.Spec.Model, maxSteps))
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
			modelResp, modelErr := w.modelGateway.Complete(ctx, ModelRequest{
				Model:     w.agent.Spec.Model,
				ModelRef:  w.agent.Spec.ModelRef,
				Namespace: w.agent.Metadata.Namespace,
				Agent:     w.agent.Metadata.Name,
				Prompt:    w.agent.Spec.Prompt,
				Step:      step,
				Tools:     append([]string(nil), w.agent.Spec.Tools...),
				Context:   w.modelContext(step),
			})
			if modelErr != nil {
				if w.onEvent != nil {
					w.onEvent(fmt.Sprintf("step=%d model_error=%v", step, modelErr))
				}
				continue
			}
			modelOutput := strings.TrimSpace(modelResp.Content)
			modelUsage := normalizeModelUsageWithFallback(modelResp.Usage, w.agent, modelResp, step)
			if w.onEvent != nil {
				w.onEvent(fmt.Sprintf("step=%d model success tokens=%d usage_source=%s", step, modelUsage.TotalTokens, modelUsage.Source))
				if modelOutput != "" {
					w.onEvent(fmt.Sprintf("step=%d model_output=%s", step, modelOutput))
				}
			}

			if len(w.agent.Spec.Tools) == 0 {
				if w.onEvent != nil {
					w.onEvent(fmt.Sprintf("step=%d no tools configured", step))
				}
				if modelOutput != "" {
					if w.onEvent != nil {
						w.onEvent("worker completed")
					}
					return
				}
				continue
			}

			requestedCalls, selectErr := selectAuthorizedToolCalls(modelResp, w.agent.Spec.Tools)
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
				continue
			}
			if len(requestedCalls) == 0 {
				if w.onEvent != nil {
					w.onEvent(fmt.Sprintf("step=%d no tool call requested", step))
				}
				continue
			}

			for _, requested := range requestedCalls {
				tool := strings.TrimSpace(requested.Name)
				if tool == "" {
					continue
				}
				input := strings.TrimSpace(requested.Input)
				if input == "" {
					input = fmt.Sprintf("agent=%s step=%d", w.agent.Metadata.Name, step)
				}
				reqID := fmt.Sprintf(
					"%s/%s/s%03d/%s",
					resources.NormalizeNamespace(w.agent.Metadata.Namespace),
					strings.TrimSpace(w.agent.Metadata.Name),
					step,
					normalizeToolKey(tool),
				)
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
				if err != nil {
					if code, reason, retryable, ok := ToolErrorMeta(err); ok {
						status := ToolStatusError
						if IsToolDeniedError(err) {
							status = ToolStatusDenied
						}
						if w.onEvent != nil {
							w.onEvent(fmt.Sprintf("step=%d tool=%s tool_contract=%s tool_request_id=%s tool_attempt=%d status=%s tool_code=%s tool_reason=%s retryable=%t error=%s", step, tool, contractVersion, toolRequestID, toolAttempt, status, code, reason, retryable, err))
						}
					}
					if IsToolDeniedError(err) || errors.Is(err, ErrToolPermissionDenied) || strings.Contains(strings.ToLower(err.Error()), "permission denied") {
						if w.onEvent != nil {
							w.onEvent(fmt.Sprintf("step=%d tool=%s tool_contract=%s tool_request_id=%s tool_attempt=%d permission denied error=%v", step, tool, contractVersion, toolRequestID, toolAttempt, err))
							w.onEvent("worker stopped permission denied")
						}
						return
					}
					if w.onEvent != nil {
						w.onEvent(fmt.Sprintf("step=%d tool=%s tool_contract=%s tool_request_id=%s tool_attempt=%d error=%v", step, tool, contractVersion, toolRequestID, toolAttempt, err))
					}
					continue
				}
				w.memory.Put(fmt.Sprintf("%s:%d", tool, step), result)
				if w.onEvent != nil {
					w.onEvent(fmt.Sprintf("step=%d tool=%s tool_contract=%s tool_request_id=%s tool_attempt=%d success", step, tool, contractVersion, toolRequestID, toolAttempt))
				}
			}
		}
	}

	if w.onEvent != nil {
		w.onEvent("max steps reached")
	}
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
