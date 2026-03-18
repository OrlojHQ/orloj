package agentruntime

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

var hopPattern = regexp.MustCompile(`/h([0-9]+)(?:/|$)`) //nolint:gochecknoglobals

// AgentRegistry lists and resolves declared agents for message consumer subscriptions/execution.
type AgentRegistry interface {
	List() []resources.Agent
	Get(name string) (resources.Agent, bool)
}

// AgentSystemRegistry resolves AgentSystem resources for next-hop routing.
type AgentSystemRegistry interface {
	Get(name string) (resources.AgentSystem, bool)
}

// TaskStateStore stores task status updates produced by message consumers.
type TaskStateStore interface {
	Get(name string) (resources.Task, bool)
	Upsert(item resources.Task) (resources.Task, error)
	AppendLog(name, message string) error
}

// MemoryResourceLookup resolves Memory CRDs by name.
type MemoryResourceLookup interface {
	Get(name string) (resources.Memory, bool)
}

// AgentMessageConsumerOptions configures inbox consumers in a worker.
type AgentMessageConsumerOptions struct {
	WorkerID            string
	Namespace           string
	RefreshEvery        time.Duration
	DedupeWindow        time.Duration
	ConsumerDelay       time.Duration
	LeaseExtendDuration time.Duration
	Executor            *TaskExecutor
	Tools               ToolResourceLookup
	Roles               AgentRoleLookup
	ToolPermissions     ToolPermissionLookup
	IsolatedToolRuntime ToolRuntime
	Extensions          Extensions
	Memories            MemoryResourceLookup
	MemoryBackends      *PersistentMemoryBackendRegistry
}

// AgentMessageConsumerManager watches agents and consumes runtime inbox messages per agent.
type AgentMessageConsumerManager struct {
	bus         AgentMessageBus
	agents      AgentRegistry
	systems     AgentSystemRegistry
	tasks       TaskStateStore
	tools       ToolResourceLookup
	roles       AgentRoleLookup
	toolPerms   ToolPermissionLookup
	isolated    ToolRuntime
	executor    *TaskExecutor
	logger      *log.Logger
	workerID    string
	namespace   string
	refresh     time.Duration
	dedupeTTL   time.Duration
	retryDelay  time.Duration
	leaseExtend time.Duration
	extensions  Extensions
	memories       MemoryResourceLookup
	memBackends    *PersistentMemoryBackendRegistry
	mu             sync.Mutex
	consumers      map[string]context.CancelFunc
	seenMessage    map[string]time.Time
	taskMemory     map[string]*SharedMemoryStore
	taskMemoryMu   sync.Mutex
}

func NewAgentMessageConsumerManager(
	bus AgentMessageBus,
	agents AgentRegistry,
	systems AgentSystemRegistry,
	tasks TaskStateStore,
	logger *log.Logger,
	opts AgentMessageConsumerOptions,
) *AgentMessageConsumerManager {
	refresh := opts.RefreshEvery
	if refresh <= 0 {
		refresh = 10 * time.Second
	}
	dedupe := opts.DedupeWindow
	if dedupe <= 0 {
		dedupe = 10 * time.Minute
	}
	retry := opts.ConsumerDelay
	if retry <= 0 {
		retry = 1 * time.Second
	}
	lease := opts.LeaseExtendDuration
	if lease <= 0 {
		lease = 30 * time.Second
	}
	executor := opts.Executor
	if executor == nil {
		executor = NewTaskExecutor(logger)
	}
	return &AgentMessageConsumerManager{
		bus:         bus,
		agents:      agents,
		systems:     systems,
		tasks:       tasks,
		tools:       opts.Tools,
		roles:       opts.Roles,
		toolPerms:   opts.ToolPermissions,
		isolated:    opts.IsolatedToolRuntime,
		executor:    executor,
		logger:      logger,
		workerID:    strings.TrimSpace(opts.WorkerID),
		namespace:   strings.TrimSpace(opts.Namespace),
		refresh:     refresh,
		dedupeTTL:   dedupe,
		retryDelay:  retry,
		leaseExtend: lease,
		memories:    opts.Memories,
		memBackends: opts.MemoryBackends,
		extensions:  NormalizeExtensions(opts.Extensions),
		consumers:   make(map[string]context.CancelFunc),
		seenMessage: make(map[string]time.Time),
		taskMemory:  make(map[string]*SharedMemoryStore),
	}
}

func (m *AgentMessageConsumerManager) Start(ctx context.Context) {
	if m == nil || m.bus == nil || m.agents == nil || m.systems == nil || m.tasks == nil {
		return
	}

	m.reconcileConsumers(ctx)
	ticker := time.NewTicker(m.refresh)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.stopAllConsumers()
			return
		case <-ticker.C:
			m.reconcileConsumers(ctx)
		}
	}
}

func (m *AgentMessageConsumerManager) reconcileConsumers(ctx context.Context) {
	agents := m.agents.List()
	desired := make(map[string]AgentMessageSubscription, len(agents))
	for _, agent := range agents {
		name := strings.TrimSpace(agent.Metadata.Name)
		if name == "" {
			continue
		}
		namespace := resources.NormalizeNamespace(agent.Metadata.Namespace)
		if strings.TrimSpace(m.namespace) != "" && !strings.EqualFold(namespace, m.namespace) {
			continue
		}
		key := scopedTaskName(namespace, name)
		desired[key] = AgentMessageSubscription{
			Namespace: namespace,
			Agent:     name,
			Durable:   durableName(m.workerID, namespace, name),
		}
	}

	m.mu.Lock()
	for key, cancel := range m.consumers {
		if _, keep := desired[key]; keep {
			continue
		}
		cancel()
		delete(m.consumers, key)
		if m.logger != nil {
			m.logger.Printf("agent message consumer stopped agent=%s", key)
		}
	}
	for key, sub := range desired {
		if _, exists := m.consumers[key]; exists {
			continue
		}
		childCtx, cancel := context.WithCancel(ctx)
		m.consumers[key] = cancel
		go m.consumeLoop(childCtx, key, sub)
		if m.logger != nil {
			m.logger.Printf("agent message consumer started agent=%s durable=%s", key, sub.Durable)
		}
	}
	m.mu.Unlock()
}

func (m *AgentMessageConsumerManager) stopAllConsumers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, cancel := range m.consumers {
		cancel()
		delete(m.consumers, key)
		if m.logger != nil {
			m.logger.Printf("agent message consumer stopped agent=%s", key)
		}
	}
}

func (m *AgentMessageConsumerManager) consumeLoop(ctx context.Context, key string, sub AgentMessageSubscription) {
	for {
		err := m.bus.Consume(ctx, sub, func(ctx context.Context, delivery AgentMessageDelivery) error {
			return m.handleDelivery(ctx, key, delivery)
		})
		if err != nil && ctx.Err() == nil && m.logger != nil {
			m.logger.Printf("agent message consumer error agent=%s: %v", key, err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(m.retryDelay):
		}
	}
}

func (m *AgentMessageConsumerManager) handleDelivery(ctx context.Context, _ string, delivery AgentMessageDelivery) error {
	msg := delivery.Message()
	taskKey, ok := messageTaskKey(msg)
	if !ok {
		if m.logger != nil {
			m.logger.Printf("agent message dropped: missing task_id message_id=%s to=%s", msg.MessageID, msg.ToAgent)
		}
		return nil
	}

	task, ok := m.tasks.Get(taskKey)
	if !ok {
		if m.logger != nil {
			m.logger.Printf("agent message dropped: task not found task=%s message_id=%s", taskKey, msg.MessageID)
		}
		return nil
	}
	if isTerminalTaskPhase(task.Status.Phase) {
		return nil
	}
	if isMessageProcessed(task.Status.Trace, msg.MessageID) {
		return nil
	}

	return m.processMessage(ctx, taskKey, msg)
}

func (m *AgentMessageConsumerManager) processMessage(ctx context.Context, taskKey string, msg AgentMessage) error {
	ctx, msgSpan := telemetry.StartMessageSpan(ctx, taskKey,
		msg.MessageID, msg.FromAgent, msg.ToAgent, msg.BranchID)
	defer msgSpan.End()

	task, record, skip, retryAfter, err := m.beginMessageAttempt(taskKey, msg)
	if err != nil {
		telemetry.EndSpanError(msgSpan, err)
		return err
	}
	if skip {
		return nil
	}
	if retryAfter > 0 {
		return RetryAfter(retryAfter, nil)
	}

	ns, _ := splitTaskKey(taskKey)
	systemKey := scopedTaskName(ns, task.Spec.System)
	system, ok := m.systems.Get(systemKey)
	if !ok {
		retryScheduled, delay, markErr := m.recordRetryOrDeadLetter(taskKey, msg, record, fmt.Errorf("agentsystem %q not found", task.Spec.System))
		if markErr != nil {
			return markErr
		}
		if retryScheduled {
			return RetryAfter(delay, nil)
		}
		return nil
	}

	joinDecision, err := m.applyJoinGate(taskKey, msg, system)
	if err != nil {
		return err
	}
	if joinDecision.SkipExecution {
		return nil
	}

	agentKey := scopedTaskName(ns, msg.ToAgent)
	agent, ok := m.agents.Get(agentKey)
	if !ok {
		retryScheduled, delay, markErr := m.recordRetryOrDeadLetter(taskKey, msg, record, fmt.Errorf("agent %q not found for message processing", msg.ToAgent))
		if markErr != nil {
			return markErr
		}
		if retryScheduled {
			return RetryAfter(delay, nil)
		}
		return nil
	}

	input := copyStringMap(task.Spec.Input)
	input["inbox.from"] = strings.TrimSpace(msg.FromAgent)
	input["inbox.to"] = strings.TrimSpace(msg.ToAgent)
	input["inbox.content"] = strings.TrimSpace(msg.Payload)
	input["inbox.message_id"] = strings.TrimSpace(msg.MessageID)
	input["inbox.trace_id"] = strings.TrimSpace(msg.TraceID)
	input["inbox.branch_id"] = strings.TrimSpace(msg.BranchID)
	input["inbox.parent_branch_id"] = strings.TrimSpace(msg.ParentBranchID)
	input["previous_agent"] = strings.TrimSpace(msg.FromAgent)
	if joinDecision.JoinMode != "" {
		input["inbox.join.enabled"] = "true"
		input["inbox.join.mode"] = joinDecision.JoinMode
		input["inbox.join.received"] = strconv.Itoa(len(joinDecision.Sources))
		input["inbox.join.required"] = strconv.Itoa(joinDecision.Required)
		input["inbox.join.sources"] = joinDecision.SourceAgents()
		input["inbox.join.payloads"] = joinDecision.SourcePayloads()
	}

	var toolRT ToolRuntime = BuildGovernedToolRuntimeForAgentWithGovernance(
		nil,
		m.isolated,
		m.tools,
		m.roles,
		m.toolPerms,
		ns,
		agent,
	)
	if memRef := strings.TrimSpace(agent.Spec.Memory.Ref); memRef != "" {
		sharedMem := m.taskSharedMemory(taskKey)
		memRT := NewMemoryToolRuntime(toolRT, sharedMem)
		memKey := scopedTaskName(ns, memRef)
		if m.memBackends != nil {
			if backend, ok := m.memBackends.Get(memKey); ok {
				memRT = memRT.WithPersistentBackend(backend)
			}
		}
		toolRT = memRT
		for _, name := range BuiltinMemoryToolNames() {
			agent.Spec.Tools = append(agent.Spec.Tools, name)
		}
	}
	agentCtx, agentSpan := telemetry.StartAgentSpan(ctx, agent.Metadata.Name, msg.MessageID, msg.Attempt)
	result, err := m.executor.ExecuteAgentWithRuntime(agentCtx, agent, input, toolRT)
	if err != nil {
		telemetry.EndSpanError(agentSpan, err)
		retryScheduled, delay, markErr := m.recordRetryOrDeadLetter(taskKey, msg, record, fmt.Errorf("agent %s execution failed: %w", agent.Metadata.Name, err))
		if markErr != nil {
			return markErr
		}
		if m.logger != nil {
			m.logger.Printf("agent message execution failed task=%s agent=%s message_id=%s error=%v", taskKey, agent.Metadata.Name, msg.MessageID, err)
		}
		if retryScheduled {
			return RetryAfter(delay, err)
		}
		return nil
	}
	telemetry.EndSpanOK(agentSpan,
		attribute.Int("orloj.tokens.used", result.TokensUsed),
		attribute.Int("orloj.tokens.estimated", result.EstimatedTokens),
		attribute.Int("orloj.tool_calls", result.ToolCalls),
		attribute.Int64("orloj.latency_ms", result.Duration.Milliseconds()),
	)
	telemetry.RecordAgentExecution(agent.Metadata.Name, agent.Spec.Model, result.Duration.Seconds(), result.TokensUsed, result.EstimatedTokens)
	telemetry.RecordMessagePhase("succeeded", strings.TrimSpace(msg.ToAgent))
	if tokenBudgetExceeded(task, result) {
		reason := fmt.Errorf(
			"token budget exceeded after agent %s: used=%d budget=%d source=%s",
			agent.Metadata.Name,
			tokenUsageTotal(task, result),
			tokenBudget(task),
			strings.TrimSpace(result.TokenSource),
		)
		retryScheduled, delay, markErr := m.recordRetryOrDeadLetter(taskKey, msg, record, reason)
		if markErr != nil {
			return markErr
		}
		if retryScheduled {
			return RetryAfter(delay, reason)
		}
		return nil
	}

	if limitReached, branchTurns, maxTurns := shouldStopForTurnLimit(task, msg); limitReached {
		if err := m.completeTaskSuccess(taskKey, msg, record, result); err != nil {
			return err
		}
		_ = m.tasks.AppendLog(taskKey, fmt.Sprintf(
			"agent message turn limit reached: message_id=%s branch_id=%s turns=%d max_turns=%d",
			msg.MessageID,
			strings.TrimSpace(msg.BranchID),
			branchTurns,
			maxTurns,
		))
		if m.logger != nil {
			m.logger.Printf(
				"agent message turn limit reached task=%s branch=%s turns=%d max_turns=%d",
				taskKey,
				strings.TrimSpace(msg.BranchID),
				branchTurns,
				maxTurns,
			)
		}
		return nil
	}

	nextAgents := nextAgentsFromSystem(system, strings.TrimSpace(msg.ToAgent))
	if len(nextAgents) == 0 {
		if err := m.completeTaskSuccess(taskKey, msg, record, result); err != nil {
			return err
		}
		if m.logger != nil {
			m.logger.Printf("agent message execution complete task=%s final_agent=%s", taskKey, result.Agent)
		}
		return nil
	}

	nextMessages := buildNextAgentMessages(task, msg, result, nextAgents)
	for _, nextMessage := range nextMessages {
		if _, err := m.bus.Publish(ctx, nextMessage); err != nil {
			retryScheduled, delay, markErr := m.recordRetryOrDeadLetter(taskKey, msg, record, fmt.Errorf("publish next message to %s failed: %w", nextMessage.ToAgent, err))
			if markErr != nil {
				return markErr
			}
			if retryScheduled {
				return RetryAfter(delay, err)
			}
			return nil
		}
	}
	if err := m.recordForward(taskKey, msg, record, result, nextMessages); err != nil {
		return err
	}
	if m.logger != nil {
		targets := make([]string, 0, len(nextMessages))
		for _, next := range nextMessages {
			targets = append(targets, next.ToAgent)
		}
		m.logger.Printf("agent message forwarded task=%s from=%s to=%s message_id=%s", taskKey, result.Agent, strings.Join(targets, ","), msg.MessageID)
	}
	return nil
}

func (m *AgentMessageConsumerManager) emitMetering(ctx context.Context, event MeteringEvent) {
	if m == nil {
		return
	}
	m.extensions.Metering.RecordMetering(ctx, event)
}

func (m *AgentMessageConsumerManager) emitAudit(ctx context.Context, event AuditEvent) {
	if m == nil {
		return
	}
	m.extensions.Audit.RecordAudit(ctx, event)
}

type joinGateDecision struct {
	SkipExecution bool
	JoinMode      string
	Required      int
	Sources       []resources.TaskJoinSource
}

func (d joinGateDecision) SourceAgents() string {
	if len(d.Sources) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(d.Sources))
	out := make([]string, 0, len(d.Sources))
	for _, source := range d.Sources {
		agent := strings.TrimSpace(source.FromAgent)
		if agent == "" {
			continue
		}
		key := strings.ToLower(agent)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, agent)
	}
	return strings.Join(out, ",")
}

func (d joinGateDecision) SourcePayloads() string {
	if len(d.Sources) == 0 {
		return ""
	}
	values := make([]string, 0, len(d.Sources))
	for _, source := range d.Sources {
		from := strings.TrimSpace(source.FromAgent)
		payload := strings.TrimSpace(source.Payload)
		if from == "" && payload == "" {
			continue
		}
		values = append(values, fmt.Sprintf("%s:%s", from, payload))
	}
	return strings.Join(values, " | ")
}

func (m *AgentMessageConsumerManager) applyJoinGate(taskKey string, msg AgentMessage, system resources.AgentSystem) (joinGateDecision, error) {
	joinMode, expected, required, enabled := joinRequirements(system, strings.TrimSpace(msg.ToAgent))
	if !enabled {
		return joinGateDecision{}, nil
	}

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		task, ok := m.tasks.Get(taskKey)
		if !ok {
			return joinGateDecision{SkipExecution: true}, nil
		}
		if isTerminalTaskPhase(task.Status.Phase) {
			return joinGateDecision{SkipExecution: true}, nil
		}

		index := ensureTaskMessageRecord(&task, msg)
		current := task.Status.Messages[index]
		if isTerminalMessagePhase(current.Phase) {
			return joinGateDecision{SkipExecution: true}, nil
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		stateIdx := ensureTaskJoinState(&task, msg.Attempt, msg.ToAgent, joinMode, expected, required)
		state := task.Status.JoinStates[stateIdx]
		source := resources.TaskJoinSource{
			MessageID: strings.TrimSpace(msg.MessageID),
			FromAgent: strings.TrimSpace(msg.FromAgent),
			BranchID:  strings.TrimSpace(msg.BranchID),
			Timestamp: normalizeMessageTimestamp(msg.Timestamp),
			Payload:   strings.TrimSpace(msg.Payload),
		}
		state = appendJoinSource(state, source)
		state.Expected = expected
		state.QuorumRequired = required
		state.Mode = joinMode

		if state.Activated {
			current.Phase = "Succeeded"
			current.Worker = strings.TrimSpace(m.workerID)
			current.ProcessedAt = now
			current.NextAttemptAt = ""
			current.LastError = ""
			task.Status.Messages[index] = current
			markMessageIdempotency(&task, msg, "completed", m.workerID)
			task.Status.JoinStates[stateIdx] = state
			appendMessageTrace(&task, msg, "agent_message_processed", fmt.Sprintf("message_id=%s status=join_already_activated branch_id=%s", msg.MessageID, msg.BranchID))
			trimTaskMessages(&task)
			trimTaskJoinStates(&task)
			trimTaskIdempotency(&task)
			task.Status.ObservedGeneration = task.Metadata.Generation
			if _, err := m.tasks.Upsert(task); err != nil {
				lastErr = err
				time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
				continue
			}
			return joinGateDecision{SkipExecution: true, JoinMode: state.Mode, Required: state.QuorumRequired, Sources: state.Sources}, nil
		}

		ready := len(state.Sources) >= state.QuorumRequired
		if !ready {
			current.Phase = "Succeeded"
			current.Worker = strings.TrimSpace(m.workerID)
			current.ProcessedAt = now
			current.NextAttemptAt = ""
			current.LastError = ""
			task.Status.Messages[index] = current
			markMessageIdempotency(&task, msg, "completed", m.workerID)
			task.Status.JoinStates[stateIdx] = state
			appendMessageTrace(&task, msg, "agent_message_join_wait", fmt.Sprintf("message_id=%s received=%d required=%d", msg.MessageID, len(state.Sources), state.QuorumRequired))
			appendMessageTrace(&task, msg, "agent_message_processed", fmt.Sprintf("message_id=%s status=join_wait", msg.MessageID))
			trimTaskMessages(&task)
			trimTaskJoinStates(&task)
			trimTaskIdempotency(&task)
			task.Status.ObservedGeneration = task.Metadata.Generation
			if _, err := m.tasks.Upsert(task); err != nil {
				lastErr = err
				time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
				continue
			}
			return joinGateDecision{SkipExecution: true, JoinMode: state.Mode, Required: state.QuorumRequired, Sources: state.Sources}, nil
		}

		state.Activated = true
		state.ActivatedAt = now
		state.ActivatedBy = strings.TrimSpace(msg.MessageID)
		task.Status.JoinStates[stateIdx] = state
		trimTaskJoinStates(&task)
		task.Status.ObservedGeneration = task.Metadata.Generation
		if _, err := m.tasks.Upsert(task); err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
			continue
		}
		return joinGateDecision{SkipExecution: false, JoinMode: state.Mode, Required: state.QuorumRequired, Sources: state.Sources}, nil
	}
	if lastErr != nil {
		return joinGateDecision{}, lastErr
	}
	return joinGateDecision{}, nil
}

func (m *AgentMessageConsumerManager) completeTaskSuccess(taskKey string, msg AgentMessage, record resources.TaskMessage, result AgentExecutionResult) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		task, ok := m.tasks.Get(taskKey)
		if !ok {
			return nil
		}
		if isTerminalTaskPhase(task.Status.Phase) {
			return nil
		}

		index := ensureTaskMessageRecord(&task, msg)
		message := task.Status.Messages[index]
		message.Attempts = max(message.Attempts, record.Attempts)
		if message.MaxAttempts <= 0 {
			message.MaxAttempts = defaultMessageMaxAttempts(task)
		}
		message.Phase = "Succeeded"
		message.Worker = strings.TrimSpace(m.workerID)
		message.ProcessedAt = time.Now().UTC().Format(time.RFC3339Nano)
		message.NextAttemptAt = ""
		message.LastError = ""
		task.Status.Messages[index] = message
		markMessageIdempotency(&task, msg, "completed", m.workerID)
		trimTaskMessages(&task)
		trimTaskIdempotency(&task)

		appendMessageTrace(&task, msg, "agent_message_received", fmt.Sprintf("message_id=%s from=%s type=%s branch_id=%s", msg.MessageID, msg.FromAgent, msg.Type, msg.BranchID))
		appendRuntimeStepTrace(&task, result.Agent, result.StepEvents)
		appendMessageTrace(&task, msg, "agent_message_processed", fmt.Sprintf("message_id=%s status=succeeded branch_id=%s", msg.MessageID, msg.BranchID))
		updateTaskOutput(&task, result, "message-driven")

		now := time.Now().UTC().Format(time.RFC3339Nano)
		if allTaskMessagesTerminal(task.Status.Messages) {
			task.Status.Phase = "Succeeded"
			task.Status.LastError = ""
			if strings.TrimSpace(task.Status.StartedAt) == "" {
				task.Status.StartedAt = now
			}
			task.Status.CompletedAt = now
			task.Status.AssignedWorker = ""
			task.Status.ClaimedBy = ""
			task.Status.LeaseUntil = ""
			task.Status.LastHeartbeat = ""
			task.Status.History = append(task.Status.History, resources.TaskHistoryEvent{
				Timestamp: now,
				Type:      "succeeded",
				Worker:    m.workerID,
				Message:   fmt.Sprintf("all terminal branches complete; last agent %s", result.Agent),
			})
			trimTaskHistory(&task)
		}
		task.Status.ObservedGeneration = task.Metadata.Generation

		if _, err := m.tasks.Upsert(task); err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
			continue
		}
		_ = m.tasks.AppendLog(taskKey, fmt.Sprintf("agent message processed: id=%s terminal_agent=%s task_phase=%s", msg.MessageID, result.Agent, task.Status.Phase))
		namespace, taskName := splitTaskKey(taskKey)
		m.emitMetering(context.Background(), MeteringEvent{
			Timestamp:       time.Now().UTC().Format(time.RFC3339Nano),
			Component:       "agent-message-consumer",
			Type:            "message.completed",
			Namespace:       namespace,
			Task:            taskName,
			System:          strings.TrimSpace(task.Spec.System),
			Agent:           strings.TrimSpace(result.Agent),
			Worker:          strings.TrimSpace(m.workerID),
			Attempt:         max(msg.Attempt, task.Status.Attempts),
			MessageID:       strings.TrimSpace(msg.MessageID),
			Status:          strings.ToLower(strings.TrimSpace(task.Status.Phase)),
			TokensUsed:      result.TokensUsed,
			TokensEstimated: result.EstimatedTokens,
			ToolCalls:       result.ToolCalls,
		})
		m.emitAudit(context.Background(), AuditEvent{
			Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
			Component:    "agent-message-consumer",
			Action:       "message.completed",
			Outcome:      strings.ToLower(strings.TrimSpace(task.Status.Phase)),
			Namespace:    namespace,
			ResourceKind: "TaskMessage",
			ResourceName: strings.TrimSpace(msg.MessageID),
			Principal:    strings.TrimSpace(m.workerID),
			Message:      fmt.Sprintf("message completed by %s", result.Agent),
			Metadata: map[string]string{
				"task":             taskName,
				"tokens_used":      strconv.Itoa(result.TokensUsed),
				"tokens_estimated": strconv.Itoa(result.EstimatedTokens),
				"tool_calls":       strconv.Itoa(result.ToolCalls),
			},
		})
		return nil
	}
	return lastErr
}

func (m *AgentMessageConsumerManager) recordForward(taskKey string, msg AgentMessage, record resources.TaskMessage, result AgentExecutionResult, nextMessages []AgentMessage) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		task, ok := m.tasks.Get(taskKey)
		if !ok {
			return nil
		}
		if isTerminalTaskPhase(task.Status.Phase) {
			return nil
		}

		currentIndex := ensureTaskMessageRecord(&task, msg)
		current := task.Status.Messages[currentIndex]
		current.Attempts = max(current.Attempts, record.Attempts)
		if current.MaxAttempts <= 0 {
			current.MaxAttempts = defaultMessageMaxAttempts(task)
		}
		current.Phase = "Succeeded"
		current.Worker = strings.TrimSpace(m.workerID)
		current.ProcessedAt = time.Now().UTC().Format(time.RFC3339Nano)
		current.NextAttemptAt = ""
		current.LastError = ""
		task.Status.Messages[currentIndex] = current
		markMessageIdempotency(&task, msg, "completed", m.workerID)

		for _, next := range nextMessages {
			nextIndex := ensureTaskMessageRecord(&task, next)
			nextRecord := task.Status.Messages[nextIndex]
			if nextRecord.MaxAttempts <= 0 {
				nextRecord.MaxAttempts = defaultMessageMaxAttempts(task)
			}
			if strings.TrimSpace(nextRecord.Phase) == "" || strings.EqualFold(nextRecord.Phase, "RetryPending") {
				nextRecord.Phase = "Queued"
			}
			nextRecord.Worker = ""
			nextRecord.LastError = ""
			nextRecord.NextAttemptAt = ""
			task.Status.Messages[nextIndex] = nextRecord
		}
		trimTaskMessages(&task)
		trimTaskIdempotency(&task)

		appendMessageTrace(&task, msg, "agent_message_received", fmt.Sprintf("message_id=%s from=%s type=%s branch_id=%s", msg.MessageID, msg.FromAgent, msg.Type, msg.BranchID))
		appendRuntimeStepTrace(&task, result.Agent, result.StepEvents)
		targets := make([]string, 0, len(nextMessages))
		for _, next := range nextMessages {
			targets = append(targets, next.ToAgent)
			appendMessageTrace(&task, next, "agent_message", fmt.Sprintf("message_id=%s to=%s branch_id=%s parent_branch_id=%s", next.MessageID, next.ToAgent, next.BranchID, next.ParentBranchID))
		}
		appendMessageTrace(&task, msg, "agent_message_processed", fmt.Sprintf("message_id=%s status=forwarded to=%s branch_id=%s", msg.MessageID, strings.Join(targets, ","), msg.BranchID))
		updateTaskOutput(&task, result, "message-driven")
		extendWorkerLease(&task, m.workerID, m.leaseExtend)
		task.Status.ObservedGeneration = task.Metadata.Generation

		if _, err := m.tasks.Upsert(task); err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
			continue
		}
		targets = make([]string, 0, len(nextMessages))
		nextIDs := make([]string, 0, len(nextMessages))
		for _, next := range nextMessages {
			targets = append(targets, next.ToAgent)
			nextIDs = append(nextIDs, next.MessageID)
		}
		_ = m.tasks.AppendLog(taskKey, fmt.Sprintf("agent message processed: id=%s forwarded_to=%s next_messages=%s", msg.MessageID, strings.Join(targets, ","), strings.Join(nextIDs, ",")))
		namespace, taskName := splitTaskKey(taskKey)
		m.emitMetering(context.Background(), MeteringEvent{
			Timestamp:       time.Now().UTC().Format(time.RFC3339Nano),
			Component:       "agent-message-consumer",
			Type:            "message.completed",
			Namespace:       namespace,
			Task:            taskName,
			System:          strings.TrimSpace(task.Spec.System),
			Agent:           strings.TrimSpace(result.Agent),
			Worker:          strings.TrimSpace(m.workerID),
			Attempt:         max(msg.Attempt, task.Status.Attempts),
			MessageID:       strings.TrimSpace(msg.MessageID),
			Status:          "forwarded",
			TokensUsed:      result.TokensUsed,
			TokensEstimated: result.EstimatedTokens,
			ToolCalls:       result.ToolCalls,
			Metadata: map[string]string{
				"forwarded_to": strings.Join(targets, ","),
			},
		})
		m.emitAudit(context.Background(), AuditEvent{
			Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
			Component:    "agent-message-consumer",
			Action:       "message.forwarded",
			Outcome:      "success",
			Namespace:    namespace,
			ResourceKind: "TaskMessage",
			ResourceName: strings.TrimSpace(msg.MessageID),
			Principal:    strings.TrimSpace(m.workerID),
			Message:      fmt.Sprintf("forwarded to %s", strings.Join(targets, ",")),
			Metadata: map[string]string{
				"task": taskName,
			},
		})
		return nil
	}
	return lastErr
}

func (m *AgentMessageConsumerManager) beginMessageAttempt(taskKey string, msg AgentMessage) (resources.Task, resources.TaskMessage, bool, time.Duration, error) {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		task, ok := m.tasks.Get(taskKey)
		if !ok {
			return resources.Task{}, resources.TaskMessage{}, true, 0, nil
		}
		if isTerminalTaskPhase(task.Status.Phase) {
			return task, resources.TaskMessage{}, true, 0, nil
		}
		index := ensureTaskMessageRecord(&task, msg)
		record := task.Status.Messages[index]
		if isMessageIdempotent(task.Status.MessageIdempotency, messageIdempotencyKey(msg)) {
			return task, record, true, 0, nil
		}
		if isTerminalMessagePhase(record.Phase) {
			return task, record, true, 0, nil
		}
		if isMessageProcessed(task.Status.Trace, msg.MessageID) {
			return task, record, true, 0, nil
		}
		owned, ownershipRetryAfter, ownershipChanged, takeover := m.acquireMessageOwnership(&task, msg)
		if !owned {
			return task, record, false, ownershipRetryAfter, nil
		}
		if strings.EqualFold(strings.TrimSpace(record.Phase), "retrypending") {
			wait := retryWaitDuration(record.NextAttemptAt)
			if wait > 0 {
				if ownershipChanged {
					task.Status.ObservedGeneration = task.Metadata.Generation
					if _, err := m.tasks.Upsert(task); err != nil {
						lastErr = err
						time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
						continue
					}
					if takeover {
						_ = m.tasks.AppendLog(taskKey, fmt.Sprintf("agent message lease takeover: message_id=%s worker=%s", msg.MessageID, m.workerID))
					}
				}
				return task, record, false, wait, nil
			}
		}
		record.Attempts++
		if record.MaxAttempts <= 0 {
			record.MaxAttempts = defaultMessageMaxAttempts(task)
		}
		record.Phase = "Running"
		record.Worker = strings.TrimSpace(m.workerID)
		record.LastError = ""
		record.NextAttemptAt = ""
		task.Status.Messages[index] = record
		appendMessageTrace(&task, msg, "agent_message_received", fmt.Sprintf("message_id=%s from=%s type=%s branch_id=%s", msg.MessageID, msg.FromAgent, msg.Type, msg.BranchID))
		extendWorkerLease(&task, m.workerID, m.leaseExtend)
		task.Status.ObservedGeneration = task.Metadata.Generation

		if _, err := m.tasks.Upsert(task); err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
			continue
		}
		if takeover {
			_ = m.tasks.AppendLog(taskKey, fmt.Sprintf("agent message lease takeover: message_id=%s worker=%s", msg.MessageID, m.workerID))
		}
		_ = m.tasks.AppendLog(taskKey, fmt.Sprintf("agent message attempt started: id=%s attempt=%d/%d", msg.MessageID, record.Attempts, record.MaxAttempts))
		namespace, taskName := splitTaskKey(taskKey)
		m.emitMetering(context.Background(), MeteringEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Component: "agent-message-consumer",
			Type:      "message.attempt_started",
			Namespace: namespace,
			Task:      taskName,
			System:    strings.TrimSpace(task.Spec.System),
			Agent:     strings.TrimSpace(msg.ToAgent),
			Worker:    strings.TrimSpace(m.workerID),
			Attempt:   max(msg.Attempt, task.Status.Attempts),
			MessageID: strings.TrimSpace(msg.MessageID),
			Status:    "running",
			Metadata: map[string]string{
				"message_attempt": strconv.Itoa(record.Attempts),
				"message_type":    strings.TrimSpace(msg.Type),
			},
		})
		m.emitAudit(context.Background(), AuditEvent{
			Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
			Component:    "agent-message-consumer",
			Action:       "message.attempt_started",
			Outcome:      "success",
			Namespace:    namespace,
			ResourceKind: "TaskMessage",
			ResourceName: strings.TrimSpace(msg.MessageID),
			Principal:    strings.TrimSpace(m.workerID),
			Message:      fmt.Sprintf("message attempt %d started for %s", record.Attempts, msg.ToAgent),
			Metadata: map[string]string{
				"task":     taskName,
				"system":   strings.TrimSpace(task.Spec.System),
				"to_agent": strings.TrimSpace(msg.ToAgent),
			},
		})
		return task, record, false, 0, nil
	}
	return resources.Task{}, resources.TaskMessage{}, false, 0, lastErr
}

func (m *AgentMessageConsumerManager) taskSharedMemory(taskKey string) *SharedMemoryStore {
	m.taskMemoryMu.Lock()
	defer m.taskMemoryMu.Unlock()
	mem, ok := m.taskMemory[taskKey]
	if !ok {
		mem = NewSharedMemoryStore()
		m.taskMemory[taskKey] = mem
	}
	return mem
}

func (m *AgentMessageConsumerManager) recordRetryOrDeadLetter(taskKey string, msg AgentMessage, record resources.TaskMessage, processErr error) (bool, time.Duration, error) {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		task, ok := m.tasks.Get(taskKey)
		if !ok {
			return false, 0, nil
		}
		if isTerminalTaskPhase(task.Status.Phase) {
			return false, 0, nil
		}
		index := ensureTaskMessageRecord(&task, msg)
		current := task.Status.Messages[index]
		if current.Attempts < record.Attempts {
			current.Attempts = record.Attempts
		}
		if current.MaxAttempts <= 0 {
			current.MaxAttempts = defaultMessageMaxAttempts(task)
		}
		current.Worker = strings.TrimSpace(m.workerID)
		current.LastError = strings.TrimSpace(processErr.Error())
		current.ProcessedAt = ""
		extendWorkerLease(&task, m.workerID, m.leaseExtend)

		policy := effectiveMessageRetryPolicy(task)
		retryClass := classifyMessageRetryability(policy, processErr)
		retryable := retryClass.Retryable && current.Attempts < current.MaxAttempts
		if retryable {
			delay := computeMessageRetryDelay(policy, msg, current.Attempts)
			current.Phase = "RetryPending"
			next := time.Now().UTC().Add(delay)
			current.NextAttemptAt = next.Format(time.RFC3339Nano)
			task.Status.Messages[index] = current
			appendMessageTrace(&task, msg, "agent_message_retry_scheduled", fmt.Sprintf("message_id=%s attempt=%d/%d delay=%s error=%s", msg.MessageID, current.Attempts, current.MaxAttempts, delay.String(), current.LastError))
			task.Status.ObservedGeneration = task.Metadata.Generation
			if _, err := m.tasks.Upsert(task); err != nil {
				lastErr = err
				time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
				continue
			}
			_ = m.tasks.AppendLog(taskKey, fmt.Sprintf("agent message retry scheduled: id=%s attempt=%d/%d delay=%s error=%s", msg.MessageID, current.Attempts, current.MaxAttempts, delay.String(), current.LastError))
			telemetry.RecordRetry(strings.TrimSpace(msg.ToAgent))
			telemetry.RecordMessagePhase("retrypending", strings.TrimSpace(msg.ToAgent))
			namespace, taskName := splitTaskKey(taskKey)
			m.emitMetering(context.Background(), MeteringEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Component: "agent-message-consumer",
				Type:      "message.retry_scheduled",
				Namespace: namespace,
				Task:      taskName,
				System:    strings.TrimSpace(task.Spec.System),
				Agent:     strings.TrimSpace(msg.ToAgent),
				Worker:    strings.TrimSpace(m.workerID),
				Attempt:   max(msg.Attempt, task.Status.Attempts),
				MessageID: strings.TrimSpace(msg.MessageID),
				Status:    "retrypending",
				Metadata: map[string]string{
					"delay": delay.String(),
					"error": current.LastError,
				},
			})
			m.emitAudit(context.Background(), AuditEvent{
				Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
				Component:    "agent-message-consumer",
				Action:       "message.retry_scheduled",
				Outcome:      "success",
				Namespace:    namespace,
				ResourceKind: "TaskMessage",
				ResourceName: strings.TrimSpace(msg.MessageID),
				Principal:    strings.TrimSpace(m.workerID),
				Message:      current.LastError,
				Metadata: map[string]string{
					"task":  taskName,
					"delay": delay.String(),
				},
			})
			return true, delay, nil
		}
		if !retryClass.Retryable {
			appendMessageTrace(&task, msg, "agent_message_non_retryable", fmt.Sprintf("message_id=%s reason=%s error=%s", msg.MessageID, retryClass.Reason, current.LastError))
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		current.Phase = "DeadLetter"
		current.ProcessedAt = now
		current.NextAttemptAt = ""
		task.Status.Messages[index] = current
		markMessageIdempotency(&task, msg, "deadletter", m.workerID)
		appendMessageTrace(&task, msg, "agent_message_deadletter", fmt.Sprintf("message_id=%s attempts=%d error=%s branch_id=%s", msg.MessageID, current.Attempts, current.LastError, msg.BranchID))
		trimTaskIdempotency(&task)

		task.Status.Phase = "DeadLetter"
		task.Status.LastError = fmt.Sprintf("message %s dead-lettered after %d attempts: %s", msg.MessageID, current.Attempts, current.LastError)
		if strings.TrimSpace(task.Status.StartedAt) == "" {
			task.Status.StartedAt = now
		}
		task.Status.CompletedAt = now
		task.Status.AssignedWorker = ""
		task.Status.ClaimedBy = ""
		task.Status.LeaseUntil = ""
		task.Status.LastHeartbeat = ""
		task.Status.ObservedGeneration = task.Metadata.Generation
		task.Status.History = append(task.Status.History, resources.TaskHistoryEvent{
			Timestamp: now,
			Type:      "deadletter",
			Worker:    m.workerID,
			Message:   task.Status.LastError,
		})
		trimTaskHistory(&task)

		if _, err := m.tasks.Upsert(task); err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
			continue
		}
		_ = m.tasks.AppendLog(taskKey, fmt.Sprintf("agent message dead-lettered: id=%s attempts=%d error=%s", msg.MessageID, current.Attempts, current.LastError))
		telemetry.RecordDeadLetter(strings.TrimSpace(msg.ToAgent))
		telemetry.RecordMessagePhase("deadletter", strings.TrimSpace(msg.ToAgent))
		namespace, taskName := splitTaskKey(taskKey)
		m.emitMetering(context.Background(), MeteringEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Component: "agent-message-consumer",
			Type:      "message.completed",
			Namespace: namespace,
			Task:      taskName,
			System:    strings.TrimSpace(task.Spec.System),
			Agent:     strings.TrimSpace(msg.ToAgent),
			Worker:    strings.TrimSpace(m.workerID),
			Attempt:   max(msg.Attempt, task.Status.Attempts),
			MessageID: strings.TrimSpace(msg.MessageID),
			Status:    "deadletter",
			Metadata: map[string]string{
				"error": current.LastError,
			},
		})
		m.emitAudit(context.Background(), AuditEvent{
			Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
			Component:    "agent-message-consumer",
			Action:       "message.completed",
			Outcome:      "deadletter",
			Namespace:    namespace,
			ResourceKind: "TaskMessage",
			ResourceName: strings.TrimSpace(msg.MessageID),
			Principal:    strings.TrimSpace(m.workerID),
			Message:      current.LastError,
			Metadata: map[string]string{
				"task": taskName,
			},
		})
		return false, 0, nil
	}
	return false, 0, lastErr
}

func (m *AgentMessageConsumerManager) isDuplicate(msg AgentMessage) bool {
	now := time.Now().UTC()
	id := strings.TrimSpace(msg.MessageID)
	if id == "" {
		id = fmt.Sprintf("%s|%s|%s|%s|%d", strings.TrimSpace(msg.TaskID), strings.TrimSpace(msg.FromAgent), strings.TrimSpace(msg.ToAgent), strings.TrimSpace(msg.Timestamp), msg.Attempt)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := now.Add(-m.dedupeTTL)
	for seenID, seenAt := range m.seenMessage {
		if seenAt.Before(cutoff) {
			delete(m.seenMessage, seenID)
		}
	}
	if seenAt, exists := m.seenMessage[id]; exists && now.Sub(seenAt) <= m.dedupeTTL {
		return true
	}
	m.seenMessage[id] = now
	return false
}

func appendIncomingMessage(task *resources.Task, msg AgentMessage) {
	if task == nil {
		return
	}
	_ = ensureTaskMessageRecord(task, msg)
	trimTaskMessages(task)
}

func appendOutgoingMessage(task *resources.Task, msg AgentMessage) {
	appendIncomingMessage(task, msg)
}

func ensureTaskMessageRecord(task *resources.Task, msg AgentMessage) int {
	if task == nil {
		return -1
	}
	for idx, record := range task.Status.Messages {
		if strings.EqualFold(strings.TrimSpace(record.MessageID), strings.TrimSpace(msg.MessageID)) {
			if record.MaxAttempts <= 0 {
				record.MaxAttempts = defaultMessageMaxAttempts(*task)
			}
			if strings.TrimSpace(record.Phase) == "" {
				record.Phase = "Queued"
			}
			if strings.TrimSpace(record.Timestamp) == "" {
				record.Timestamp = normalizeMessageTimestamp(msg.Timestamp)
			}
			if strings.TrimSpace(record.TaskID) == "" {
				record.TaskID = strings.TrimSpace(msg.TaskID)
			}
			if strings.TrimSpace(record.IdempotencyKey) == "" {
				record.IdempotencyKey = strings.TrimSpace(msg.IdempotencyKey)
			}
			if strings.TrimSpace(record.BranchID) == "" {
				record.BranchID = strings.TrimSpace(msg.BranchID)
			}
			if strings.TrimSpace(record.ParentBranchID) == "" {
				record.ParentBranchID = strings.TrimSpace(msg.ParentBranchID)
			}
			task.Status.Messages[idx] = record
			return idx
		}
	}
	record := resources.TaskMessage{
		Timestamp:      normalizeMessageTimestamp(msg.Timestamp),
		MessageID:      strings.TrimSpace(msg.MessageID),
		IdempotencyKey: strings.TrimSpace(msg.IdempotencyKey),
		TaskID:         strings.TrimSpace(msg.TaskID),
		Attempt:        msg.Attempt,
		System:         strings.TrimSpace(msg.System),
		FromAgent:      strings.TrimSpace(msg.FromAgent),
		ToAgent:        strings.TrimSpace(msg.ToAgent),
		BranchID:       strings.TrimSpace(msg.BranchID),
		ParentBranchID: strings.TrimSpace(msg.ParentBranchID),
		Type:           strings.TrimSpace(msg.Type),
		Content:        strings.TrimSpace(msg.Payload),
		TraceID:        strings.TrimSpace(msg.TraceID),
		ParentID:       strings.TrimSpace(msg.ParentID),
		Phase:          "Queued",
		MaxAttempts:    defaultMessageMaxAttempts(*task),
	}
	if strings.TrimSpace(record.IdempotencyKey) == "" {
		record.IdempotencyKey = strings.TrimSpace(record.MessageID)
	}
	task.Status.Messages = append(task.Status.Messages, record)
	return len(task.Status.Messages) - 1
}

func defaultMessageMaxAttempts(task resources.Task) int {
	if task.Spec.MessageRetry.MaxAttempts > 0 {
		return task.Spec.MessageRetry.MaxAttempts
	}
	if task.Spec.Retry.MaxAttempts > 0 {
		return task.Spec.Retry.MaxAttempts
	}
	return 1
}

func isTerminalMessagePhase(phase string) bool {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "succeeded", "deadletter":
		return true
	default:
		return false
	}
}

func allTaskMessagesTerminal(messages []resources.TaskMessage) bool {
	if len(messages) == 0 {
		return true
	}
	for _, message := range messages {
		if !isTerminalMessagePhase(message.Phase) {
			return false
		}
	}
	return true
}

func appendMessageTrace(task *resources.Task, msg AgentMessage, eventType, message string) {
	if task == nil {
		return
	}
	if hasTraceMarker(task.Status.Trace, eventType, msg.MessageID) {
		return
	}
	task.Status.Trace = append(task.Status.Trace, resources.TaskTraceEvent{
		Timestamp: normalizeMessageTimestamp(msg.Timestamp),
		Attempt:   max(msg.Attempt, task.Status.Attempts),
		BranchID:  strings.TrimSpace(msg.BranchID),
		Type:      strings.TrimSpace(eventType),
		Agent:     strings.TrimSpace(msg.ToAgent),
		Message:   strings.TrimSpace(message),
	})
	trimTaskTrace(task)
}

func appendRuntimeStepTrace(task *resources.Task, agentName string, events []AgentStepEvent) {
	if task == nil || len(events) == 0 {
		return
	}
	for _, runtimeEvent := range events {
		traceEvent := resources.TaskTraceEvent{
			Timestamp:           runtimeEvent.Timestamp,
			Type:                runtimeEvent.Type,
			Agent:               strings.TrimSpace(agentName),
			Tool:                strings.TrimSpace(runtimeEvent.Tool),
			ToolContractVersion: strings.TrimSpace(runtimeEvent.ToolContractVersion),
			ToolRequestID:       strings.TrimSpace(runtimeEvent.ToolRequestID),
			ToolAttempt:         runtimeEvent.ToolAttempt,
			ErrorCode:           strings.TrimSpace(runtimeEvent.ErrorCode),
			ErrorReason:         strings.TrimSpace(runtimeEvent.ErrorReason),
			Retryable:           runtimeEvent.Retryable,
			Message:             strings.TrimSpace(runtimeEvent.Message),
			Step:                runtimeEvent.Step,
			ToolAuthProfile:     strings.TrimSpace(runtimeEvent.ToolAuthProfile),
			ToolAuthSecretRef:   strings.TrimSpace(runtimeEvent.ToolAuthSecretRef),
		}
		if strings.EqualFold(runtimeEvent.Type, "tool_call") {
			traceEvent.ToolCalls = 1
		}
		if strings.EqualFold(runtimeEvent.Type, "model_call") {
			traceEvent.Tokens = runtimeEvent.Tokens
			traceEvent.TokenUsageSource = strings.TrimSpace(runtimeEvent.UsageSource)
			if source := strings.TrimSpace(runtimeEvent.UsageSource); source != "" {
				traceEvent.Message = strings.TrimSpace(traceEvent.Message + " usage_source=" + source)
			}
		}
		task.Status.Trace = append(task.Status.Trace, traceEvent)
	}
	trimTaskTrace(task)
}

func updateTaskOutput(task *resources.Task, result AgentExecutionResult, mode string) {
	if task == nil {
		return
	}
	cloned := make(map[string]string, len(task.Status.Output)+8)
	for key, value := range task.Status.Output {
		cloned[key] = value
	}
	cloned["runtime.mode"] = strings.TrimSpace(mode)
	cloned["last_agent"] = strings.TrimSpace(result.Agent)
	cloned["last_event"] = strings.TrimSpace(result.LastEvent)
	cloned["last_output"] = strings.TrimSpace(result.Output)
	cloned["last_duration_ms"] = strconv.FormatInt(result.Duration.Milliseconds(), 10)
	cloned["last_tool_calls"] = strconv.Itoa(result.ToolCalls)
	cloned["last_steps"] = strconv.Itoa(result.Steps)
	cloned["last_estimated_tokens"] = strconv.Itoa(result.EstimatedTokens)
	cloned["last_tokens_used"] = strconv.Itoa(result.TokensUsed)
	cloned["last_token_usage_source"] = strings.TrimSpace(result.TokenSource)
	prevTotal, _ := strconv.Atoi(strings.TrimSpace(cloned["tokens_estimated_total"]))
	cloned["tokens_estimated_total"] = strconv.Itoa(prevTotal + result.EstimatedTokens)
	prevUsed, _ := strconv.Atoi(strings.TrimSpace(cloned["tokens_used_total"]))
	cloned["tokens_used_total"] = strconv.Itoa(prevUsed + result.TokensUsed)
	cloned["result"] = "executed"
	cloned["system"] = strings.TrimSpace(task.Spec.System)
	task.Status.Output = cloned
}

func tokenBudget(task resources.Task) int {
	return parsePositiveInt(task.Status.Output["token_budget"])
}

func tokenUsageTotal(task resources.Task, result AgentExecutionResult) int {
	current := parsePositiveInt(task.Status.Output["tokens_used_total"])
	return current + max(0, result.TokensUsed)
}

func tokenBudgetExceeded(task resources.Task, result AgentExecutionResult) bool {
	budget := tokenBudget(task)
	if budget <= 0 {
		return false
	}
	return tokenUsageTotal(task, result) > budget
}

func parsePositiveInt(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func shouldStopForTurnLimit(task resources.Task, msg AgentMessage) (bool, int, int) {
	maxTurns := task.Spec.MaxTurns
	if maxTurns <= 0 {
		return false, 0, 0
	}
	turns := countBranchMessages(task.Status.Messages, msg.BranchID)
	if turns <= 0 {
		turns = 1
	}
	return turns >= maxTurns, turns, maxTurns
}

func countBranchMessages(messages []resources.TaskMessage, branchID string) int {
	branch := strings.TrimSpace(branchID)
	if branch == "" {
		branch = "b001"
	}
	count := 0
	for _, message := range messages {
		id := strings.TrimSpace(message.BranchID)
		if id == "" {
			id = "b001"
		}
		if strings.EqualFold(id, branch) {
			count++
		}
	}
	return count
}

func extendWorkerLease(task *resources.Task, workerID string, duration time.Duration) {
	if task == nil {
		return
	}
	if duration <= 0 {
		duration = 30 * time.Second
	}
	if strings.TrimSpace(workerID) == "" {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(task.Status.ClaimedBy), strings.TrimSpace(workerID)) {
		return
	}
	now := time.Now().UTC()
	task.Status.LastHeartbeat = now.Format(time.RFC3339Nano)
	task.Status.LeaseUntil = now.Add(duration).Format(time.RFC3339Nano)
}

func hasTaskMessage(messages []resources.TaskMessage, messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.MessageID), messageID) {
			return true
		}
	}
	return false
}

func hasTraceMarker(trace []resources.TaskTraceEvent, eventType, messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	needle := "message_id=" + messageID
	for _, event := range trace {
		if !strings.EqualFold(strings.TrimSpace(event.Type), strings.TrimSpace(eventType)) {
			continue
		}
		if strings.Contains(event.Message, needle) {
			return true
		}
	}
	return false
}

func isMessageProcessed(trace []resources.TaskTraceEvent, messageID string) bool {
	return hasTraceMarker(trace, "agent_message_processed", messageID)
}

func normalizeMessageTimestamp(ts string) string {
	value := strings.TrimSpace(ts)
	if value == "" {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	return value
}

func retryWaitDuration(nextAttemptAt string) time.Duration {
	when := strings.TrimSpace(nextAttemptAt)
	if when == "" {
		return 0
	}
	next, err := time.Parse(time.RFC3339Nano, when)
	if err != nil {
		return 0
	}
	wait := time.Until(next.UTC())
	if wait < 0 {
		return 0
	}
	return wait
}

func computeMessageRetryDelay(policy resources.TaskMessageRetryPolicy, msg AgentMessage, messageAttempts int) time.Duration {
	base := parseDurationOrDefault(policy.Backoff, 0)
	if base <= 0 {
		return 0
	}
	maxBackoff := parseDurationOrDefault(policy.MaxBackoff, 24*time.Hour)
	if maxBackoff <= 0 {
		maxBackoff = 24 * time.Hour
	}
	if messageAttempts <= 0 {
		messageAttempts = 1
	}
	exp := messageAttempts - 1
	if exp > 20 {
		exp = 20
	}
	delay := base * time.Duration(1<<exp)
	if delay > maxBackoff {
		delay = maxBackoff
	}
	if delay <= 0 {
		return 0
	}

	jitter := strings.ToLower(strings.TrimSpace(policy.Jitter))
	switch jitter {
	case "", "full":
		unit := messageRetryJitterUnit(msg, messageAttempts)
		delay = time.Duration(float64(delay) * unit)
	case "equal":
		unit := messageRetryJitterUnit(msg, messageAttempts)
		half := float64(delay) / 2
		delay = time.Duration(half + half*unit)
	case "none":
		return delay
	default:
		unit := messageRetryJitterUnit(msg, messageAttempts)
		delay = time.Duration(float64(delay) * unit)
	}
	if delay <= 0 {
		return time.Millisecond
	}
	return delay
}

type messageRetryClassification struct {
	Retryable bool
	Reason    string
}

func classifyMessageRetryability(policy resources.TaskMessageRetryPolicy, processErr error) messageRetryClassification {
	if processErr == nil {
		return messageRetryClassification{Retryable: true, Reason: "none"}
	}
	if code, reason, retryable, ok := ToolErrorMeta(processErr); ok {
		classificationReason := strings.TrimSpace(reason)
		if classificationReason == "" {
			classificationReason = strings.TrimSpace(code)
		}
		if classificationReason == "" {
			classificationReason = "tool_error"
		}
		return messageRetryClassification{Retryable: retryable, Reason: classificationReason}
	}
	reason := strings.ToLower(strings.TrimSpace(processErr.Error()))
	if reason == "" {
		return messageRetryClassification{Retryable: true, Reason: "empty_error"}
	}
	for _, marker := range policy.NonRetryable {
		token := strings.ToLower(strings.TrimSpace(marker))
		if token == "" {
			continue
		}
		if strings.Contains(reason, token) {
			return messageRetryClassification{Retryable: false, Reason: "configured_non_retryable"}
		}
	}
	switch {
	case strings.Contains(reason, "policy "),
		strings.Contains(reason, "disallows model"),
		strings.Contains(reason, "blocks tool"),
		strings.Contains(reason, "permission denied"),
		strings.Contains(reason, "token budget exceeded"):
		return messageRetryClassification{Retryable: false, Reason: "policy_error"}
	case strings.Contains(reason, "agentsystem ") && strings.Contains(reason, " not found"):
		return messageRetryClassification{Retryable: false, Reason: "invalid_system_ref"}
	case strings.Contains(reason, "agent ") && strings.Contains(reason, " not found for message processing"):
		return messageRetryClassification{Retryable: false, Reason: "invalid_agent_ref"}
	case strings.Contains(reason, "invalid graph"),
		strings.Contains(reason, "graph node ") && strings.Contains(reason, "unsupported"):
		return messageRetryClassification{Retryable: false, Reason: "invalid_graph_ref"}
	default:
		return messageRetryClassification{Retryable: true, Reason: "retryable"}
	}
}

func effectiveMessageRetryPolicy(task resources.Task) resources.TaskMessageRetryPolicy {
	policy := task.Spec.MessageRetry
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = task.Spec.Retry.MaxAttempts
	}
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}
	if strings.TrimSpace(policy.Backoff) == "" {
		policy.Backoff = task.Spec.Retry.Backoff
	}
	if strings.TrimSpace(policy.Backoff) == "" {
		policy.Backoff = "0s"
	}
	if strings.TrimSpace(policy.MaxBackoff) == "" {
		policy.MaxBackoff = "24h"
	}
	jitter := strings.ToLower(strings.TrimSpace(policy.Jitter))
	switch jitter {
	case "none", "full", "equal":
		policy.Jitter = jitter
	default:
		policy.Jitter = "full"
	}
	return policy
}

func parseDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func messageRetryJitterUnit(msg AgentMessage, messageAttempts int) float64 {
	seed := strings.TrimSpace(messageIdempotencyKey(msg))
	if seed == "" {
		seed = strings.TrimSpace(msg.MessageID)
	}
	if seed == "" {
		seed = fmt.Sprintf("%s|%s|%s", msg.TaskID, msg.FromAgent, msg.ToAgent)
	}
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(strings.ToLower(seed)))
	_, _ = hasher.Write([]byte(fmt.Sprintf("|%d", messageAttempts)))
	return float64(hasher.Sum64()%1_000_000) / 1_000_000.0
}

func (m *AgentMessageConsumerManager) acquireMessageOwnership(task *resources.Task, msg AgentMessage) (owned bool, retryAfter time.Duration, changed bool, takeover bool) {
	if task == nil {
		return false, 0, false, false
	}
	worker := strings.TrimSpace(m.workerID)
	if worker == "" {
		return true, 0, false, false
	}
	now := time.Now().UTC()
	claimedBy := strings.TrimSpace(task.Status.ClaimedBy)
	lease := m.leaseExtend
	if lease <= 0 {
		lease = 30 * time.Second
	}
	if claimedBy == "" || strings.EqualFold(claimedBy, worker) {
		task.Status.ClaimedBy = worker
		task.Status.AssignedWorker = worker
		task.Status.LastHeartbeat = now.Format(time.RFC3339Nano)
		task.Status.LeaseUntil = now.Add(lease).Format(time.RFC3339Nano)
		if strings.EqualFold(strings.TrimSpace(task.Status.Phase), "pending") || strings.TrimSpace(task.Status.Phase) == "" {
			task.Status.Phase = "Running"
		}
		return true, 0, true, false
	}
	leaseUntil, ok := parseTaskLeaseUntil(task.Status.LeaseUntil)
	if ok && now.Before(leaseUntil) {
		wait := time.Until(leaseUntil)
		if wait <= 0 {
			wait = m.retryDelay
		}
		if wait <= 0 {
			wait = 100 * time.Millisecond
		}
		return false, wait, false, false
	}
	task.Status.ClaimedBy = worker
	task.Status.AssignedWorker = worker
	task.Status.LastHeartbeat = now.Format(time.RFC3339Nano)
	task.Status.LeaseUntil = now.Add(lease).Format(time.RFC3339Nano)
	task.Status.History = append(task.Status.History, resources.TaskHistoryEvent{
		Timestamp: now.Format(time.RFC3339Nano),
		Type:      "takeover",
		Worker:    worker,
		Message:   fmt.Sprintf("message lease expired; message_id=%s reassigned from %s to %s", strings.TrimSpace(msg.MessageID), claimedBy, worker),
	})
	trimTaskHistory(task)
	return true, 0, true, true
}

func parseTaskLeaseUntil(raw string) (time.Time, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed.UTC(), true
	}
	parsed, err = time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed.UTC(), true
	}
	return time.Time{}, false
}

func messageTaskKey(msg AgentMessage) (string, bool) {
	taskID := strings.TrimSpace(msg.TaskID)
	if taskID == "" {
		return "", false
	}
	if strings.Contains(taskID, "/") {
		parts := strings.SplitN(taskID, "/", 2)
		return scopedTaskName(parts[0], parts[1]), true
	}
	return scopedTaskName(msg.Namespace, taskID), true
}

func scopedTaskName(namespace, name string) string {
	return resources.NormalizeNamespace(namespace) + "/" + strings.TrimSpace(name)
}

func splitTaskKey(taskKey string) (string, string) {
	if strings.Contains(taskKey, "/") {
		parts := strings.SplitN(taskKey, "/", 2)
		return resources.NormalizeNamespace(parts[0]), strings.TrimSpace(parts[1])
	}
	return resources.DefaultNamespace, strings.TrimSpace(taskKey)
}

func durableName(workerID, namespace, agent string) string {
	base := strings.TrimSpace(workerID)
	if base == "" {
		base = "worker"
	}
	return sanitizeSubjectToken(base) + "-" + sanitizeSubjectToken(namespace) + "-" + sanitizeSubjectToken(agent)
}

func nextAgentsFromSystem(system resources.AgentSystem, current string) []string {
	current = strings.TrimSpace(current)
	if current == "" {
		return nil
	}
	if edge, ok := system.Spec.Graph[current]; ok {
		if targets := resources.GraphOutgoingAgents(edge); len(targets) > 0 {
			return targets
		}
	}
	if len(system.Spec.Graph) == 0 {
		for idx, agent := range system.Spec.Agents {
			if !strings.EqualFold(strings.TrimSpace(agent), current) {
				continue
			}
			if idx+1 < len(system.Spec.Agents) {
				next := strings.TrimSpace(system.Spec.Agents[idx+1])
				if next != "" {
					return []string{next}
				}
			}
			break
		}
	}
	return nil
}

func buildNextAgentMessages(task resources.Task, current AgentMessage, result AgentExecutionResult, nextAgents []string) []AgentMessage {
	if len(nextAgents) == 0 {
		return nil
	}
	ns := resources.NormalizeNamespace(task.Metadata.Namespace)
	attempt := task.Status.Attempts
	if attempt <= 0 {
		attempt = max(1, current.Attempt)
	}
	nextHop := hopFromMessageID(current.MessageID) + 1
	if nextHop <= 1 {
		nextHop = 2
	}
	content := strings.TrimSpace(result.Output)
	if content == "" {
		content = strings.TrimSpace(result.LastEvent)
	}
	if content == "" {
		content = fmt.Sprintf("steps=%d tool_calls=%d tokens=%d usage_source=%s", result.Steps, result.ToolCalls, result.TokensUsed, strings.TrimSpace(result.TokenSource))
	}
	traceID := strings.TrimSpace(current.TraceID)
	if traceID == "" {
		traceID = fmt.Sprintf("%s/%s/a%03d", ns, strings.TrimSpace(task.Metadata.Name), attempt)
	}
	parentBranch := strings.TrimSpace(current.BranchID)
	if parentBranch == "" {
		parentBranch = "b001"
	}
	out := make([]AgentMessage, 0, len(nextAgents))
	for idx, nextAgent := range nextAgents {
		next := strings.TrimSpace(nextAgent)
		if next == "" {
			continue
		}
		branchID := parentBranch
		if len(nextAgents) > 1 {
			branchID = fmt.Sprintf("%s.%03d", parentBranch, idx+1)
		}
		message := AgentMessage{
			MessageID:      deterministicMessageID(ns, strings.TrimSpace(task.Metadata.Name), attempt, nextHop, result.Agent, next),
			IdempotencyKey: deterministicMessageID(ns, strings.TrimSpace(task.Metadata.Name), attempt, nextHop, result.Agent, next),
			TaskID:         scopedTaskName(ns, task.Metadata.Name),
			Attempt:        attempt,
			System:         strings.TrimSpace(task.Spec.System),
			Namespace:      ns,
			FromAgent:      strings.TrimSpace(result.Agent),
			ToAgent:        next,
			BranchID:       branchID,
			ParentBranchID: parentBranch,
			Type:           "task_handoff",
			Payload:        content,
			Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
			TraceID:        traceID,
			ParentID:       strings.TrimSpace(current.MessageID),
		}
		out = append(out, message)
	}
	return out
}

func deterministicMessageID(namespace, taskName string, attempt int, hop int, fromAgent, toAgent string) string {
	if attempt <= 0 {
		attempt = 1
	}
	if hop <= 0 {
		hop = 1
	}
	return fmt.Sprintf(
		"%s/%s/a%03d/h%03d/%s/%s",
		resources.NormalizeNamespace(namespace),
		strings.TrimSpace(taskName),
		attempt,
		hop,
		sanitizeSubjectToken(fromAgent),
		sanitizeSubjectToken(toAgent),
	)
}

func hopFromMessageID(messageID string) int {
	id := strings.TrimSpace(messageID)
	if id == "" {
		return 0
	}
	matches := hopPattern.FindStringSubmatch(id)
	if len(matches) != 2 {
		return 0
	}
	hop, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return hop
}

func trimTaskMessages(task *resources.Task) {
	if task == nil {
		return
	}
	if len(task.Status.Messages) > 500 {
		task.Status.Messages = task.Status.Messages[len(task.Status.Messages)-500:]
	}
}

func trimTaskTrace(task *resources.Task) {
	if task == nil {
		return
	}
	if len(task.Status.Trace) > 1000 {
		task.Status.Trace = task.Status.Trace[len(task.Status.Trace)-1000:]
	}
}

func trimTaskHistory(task *resources.Task) {
	if task == nil {
		return
	}
	if len(task.Status.History) > 200 {
		task.Status.History = task.Status.History[len(task.Status.History)-200:]
	}
}

func trimTaskIdempotency(task *resources.Task) {
	if task == nil {
		return
	}
	if len(task.Status.MessageIdempotency) > 2000 {
		task.Status.MessageIdempotency = task.Status.MessageIdempotency[len(task.Status.MessageIdempotency)-2000:]
	}
}

func trimTaskJoinStates(task *resources.Task) {
	if task == nil {
		return
	}
	if len(task.Status.JoinStates) > 200 {
		task.Status.JoinStates = task.Status.JoinStates[len(task.Status.JoinStates)-200:]
	}
}

func messageIdempotencyKey(msg AgentMessage) string {
	key := strings.TrimSpace(msg.IdempotencyKey)
	if key != "" {
		return key
	}
	if id := strings.TrimSpace(msg.MessageID); id != "" {
		return id
	}
	return fmt.Sprintf("%s|%s|%s|%d", strings.TrimSpace(msg.TaskID), strings.TrimSpace(msg.FromAgent), strings.TrimSpace(msg.ToAgent), msg.Attempt)
}

func isMessageIdempotent(records []resources.TaskMessageIdempotency, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	now := time.Now().UTC()
	for _, record := range records {
		if !strings.EqualFold(strings.TrimSpace(record.Key), key) {
			continue
		}
		state := strings.ToLower(strings.TrimSpace(record.State))
		if state != "completed" && state != "deadletter" {
			continue
		}
		if expiry := strings.TrimSpace(record.ExpiresAt); expiry != "" {
			when, err := time.Parse(time.RFC3339Nano, expiry)
			if err == nil && now.After(when) {
				continue
			}
		}
		return true
	}
	return false
}

func markMessageIdempotency(task *resources.Task, msg AgentMessage, state, worker string) {
	if task == nil {
		return
	}
	key := messageIdempotencyKey(msg)
	if key == "" {
		return
	}
	now := time.Now().UTC()
	updated := resources.TaskMessageIdempotency{
		Key:       key,
		MessageID: strings.TrimSpace(msg.MessageID),
		State:     strings.ToLower(strings.TrimSpace(state)),
		UpdatedAt: now.Format(time.RFC3339Nano),
		ExpiresAt: now.Add(30 * 24 * time.Hour).Format(time.RFC3339Nano),
		Worker:    strings.TrimSpace(worker),
	}
	for idx, record := range task.Status.MessageIdempotency {
		if strings.EqualFold(strings.TrimSpace(record.Key), key) {
			task.Status.MessageIdempotency[idx] = updated
			return
		}
	}
	task.Status.MessageIdempotency = append(task.Status.MessageIdempotency, updated)
}

func ensureTaskJoinState(task *resources.Task, attempt int, node, mode string, expected, required int) int {
	if task == nil {
		return -1
	}
	if attempt <= 0 {
		attempt = 1
	}
	node = strings.TrimSpace(node)
	for idx, state := range task.Status.JoinStates {
		if state.Attempt != attempt {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(state.Node), node) {
			continue
		}
		state.Mode = strings.TrimSpace(mode)
		state.Expected = expected
		state.QuorumRequired = required
		task.Status.JoinStates[idx] = state
		return idx
	}
	state := resources.TaskJoinState{
		Attempt:        attempt,
		Node:           node,
		Mode:           strings.TrimSpace(mode),
		Expected:       expected,
		QuorumRequired: required,
		Sources:        make([]resources.TaskJoinSource, 0, expected),
	}
	task.Status.JoinStates = append(task.Status.JoinStates, state)
	return len(task.Status.JoinStates) - 1
}

func appendJoinSource(state resources.TaskJoinState, source resources.TaskJoinSource) resources.TaskJoinState {
	for idx, existing := range state.Sources {
		if strings.EqualFold(strings.TrimSpace(existing.MessageID), strings.TrimSpace(source.MessageID)) {
			state.Sources[idx] = source
			return state
		}
	}
	state.Sources = append(state.Sources, source)
	return state
}

func joinRequirements(system resources.AgentSystem, target string) (mode string, expected int, required int, enabled bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", 0, 0, false
	}
	incoming := incomingAgentsForNode(system, target)
	expected = len(incoming)
	if expected <= 1 && len(system.Spec.Graph) == 0 {
		return "", expected, 0, false
	}

	mode = "wait_for_all"
	if node, ok := system.Spec.Graph[target]; ok {
		normalized := resources.NormalizeGraphJoin(node.Join)
		mode = normalized.Mode
		if mode == "" {
			mode = "wait_for_all"
		}
		if mode == "quorum" {
			required = quorumRequired(expected, normalized.QuorumCount, normalized.QuorumPercent)
		}
	}
	if expected <= 1 && mode == "wait_for_all" {
		return "", expected, expected, false
	}
	if mode == "wait_for_all" {
		required = expected
	}
	if required <= 0 {
		required = expected
	}
	if required > expected {
		required = expected
	}
	if required <= 0 {
		required = 1
	}
	return mode, expected, required, true
}

func quorumRequired(expected, absolute, percent int) int {
	if expected <= 0 {
		return 0
	}
	required := 0
	if absolute > required {
		required = absolute
	}
	if percent > 0 {
		value := int(math.Ceil(float64(expected) * (float64(percent) / 100.0)))
		if value > required {
			required = value
		}
	}
	if required <= 0 {
		required = expected
	}
	if required > expected {
		required = expected
	}
	return required
}

func incomingAgentsForNode(system resources.AgentSystem, target string) []string {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	if len(system.Spec.Graph) == 0 {
		for idx, agent := range system.Spec.Agents {
			if !strings.EqualFold(strings.TrimSpace(agent), target) {
				continue
			}
			if idx == 0 {
				return nil
			}
			prev := strings.TrimSpace(system.Spec.Agents[idx-1])
			if prev == "" {
				return nil
			}
			return []string{prev}
		}
		return nil
	}
	seen := make(map[string]struct{}, 4)
	out := make([]string, 0, 4)
	for from, node := range system.Spec.Graph {
		for _, to := range resources.GraphOutgoingAgents(node) {
			if !strings.EqualFold(strings.TrimSpace(to), target) {
				continue
			}
			f := strings.TrimSpace(from)
			key := strings.ToLower(f)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, f)
		}
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func isTerminalTaskPhase(phase string) bool {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "succeeded", "failed", "deadletter":
		return true
	default:
		return false
	}
}

func max(a, b int) int {
	if a >= b {
		return a
	}
	return b
}
