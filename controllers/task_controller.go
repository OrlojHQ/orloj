package controllers

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/crds"
	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
	"github.com/OrlojHQ/orloj/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

var traceStepIDPattern = regexp.MustCompile(`^a([0-9]+)\.s([0-9]+)$`) //nolint:gochecknoglobals

// TaskController reconciles Task resources.
type TaskController struct {
	taskStore        *store.TaskStore
	agentSystemStore *store.AgentSystemStore
	agentStore       *store.AgentStore
	toolStore        *store.ToolStore
	memoryStore      *store.MemoryStore
	policyStore      *store.AgentPolicyStore
	modelEPStore     *store.ModelEndpointStore
	roleStore        *store.AgentRoleStore
	toolPermStore    *store.ToolPermissionStore
	workerStore      *store.WorkerStore
	executor         *agentruntime.TaskExecutor
	reconcileEvery   time.Duration
	leaseDuration    time.Duration
	heartbeatEvery   time.Duration
	workerID         string
	logger           *log.Logger
	eventBus         eventbus.Bus
	agentMessageBus  agentruntime.AgentMessageBus
	executionMode    string
	isolatedTools    agentruntime.ToolRuntime
	extensions       agentruntime.Extensions
}

func NewTaskController(
	taskStore *store.TaskStore,
	agentSystemStore *store.AgentSystemStore,
	agentStore *store.AgentStore,
	toolStore *store.ToolStore,
	memoryStore *store.MemoryStore,
	policyStore *store.AgentPolicyStore,
	workerStore *store.WorkerStore,
	logger *log.Logger,
	reconcileEvery time.Duration,
) *TaskController {
	if reconcileEvery <= 0 {
		reconcileEvery = 2 * time.Second
	}
	return &TaskController{
		taskStore:        taskStore,
		agentSystemStore: agentSystemStore,
		agentStore:       agentStore,
		toolStore:        toolStore,
		memoryStore:      memoryStore,
		policyStore:      policyStore,
		workerStore:      workerStore,
		executor:         agentruntime.NewTaskExecutor(logger),
		reconcileEvery:   reconcileEvery,
		leaseDuration:    30 * time.Second,
		heartbeatEvery:   10 * time.Second,
		workerID:         defaultWorkerID(),
		logger:           logger,
		executionMode:    "sequential",
		extensions:       agentruntime.DefaultExtensions(),
	}
}

func (c *TaskController) ConfigureWorker(workerID string, leaseDuration, heartbeatEvery time.Duration) {
	if strings.TrimSpace(workerID) != "" {
		c.workerID = workerID
	}
	if leaseDuration > 0 {
		c.leaseDuration = leaseDuration
	}
	if heartbeatEvery > 0 {
		c.heartbeatEvery = heartbeatEvery
	}
}

func (c *TaskController) SetEventBus(bus eventbus.Bus) {
	c.eventBus = bus
}

func (c *TaskController) SetAgentMessageBus(bus agentruntime.AgentMessageBus) {
	c.agentMessageBus = bus
}

func (c *TaskController) SetExecutionMode(mode string) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "sequential"
	}
	c.executionMode = mode
}

func (c *TaskController) SetIsolatedToolRuntime(runtime agentruntime.ToolRuntime) {
	c.isolatedTools = runtime
}

func (c *TaskController) SetGovernanceStores(roleStore *store.AgentRoleStore, toolPermStore *store.ToolPermissionStore) {
	c.roleStore = roleStore
	c.toolPermStore = toolPermStore
}

func (c *TaskController) SetModelEndpointStore(modelEPStore *store.ModelEndpointStore) {
	c.modelEPStore = modelEPStore
}

func (c *TaskController) SetExecutor(executor *agentruntime.TaskExecutor) {
	if executor == nil {
		return
	}
	c.executor = executor
}

func (c *TaskController) SetExtensions(ext agentruntime.Extensions) {
	c.extensions = agentruntime.NormalizeExtensions(ext)
}

func (c *TaskController) Start(ctx context.Context) {
	ticker := time.NewTicker(c.reconcileEvery)
	defer ticker.Stop()
	var eventCh <-chan eventbus.Event
	if c.eventBus != nil {
		eventCh = c.eventBus.Subscribe(ctx, eventbus.Filter{
			Source: "apiserver",
			Kind:   "Task",
		})
	}

	for {
		if err := c.ReconcileOnce(ctx); err != nil && c.logger != nil {
			c.logger.Printf("task controller reconcile error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-eventCh:
		}
	}
}

func (c *TaskController) ReconcileOnce(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		slotAcquired := false
		if c.workerStore != nil {
			acquired, err := c.tryAcquireWorkerSlot()
			if err != nil {
				return err
			}
			if !acquired {
				return nil
			}
			slotAcquired = true
		}
		task, claimed, err := c.taskStore.ClaimNextDue(c.workerID, c.leaseDuration, c.taskMatchesWorker)
		if err != nil {
			if slotAcquired {
				_ = c.workerStore.ReleaseSlot(c.workerID)
			}
			return err
		}
		if !claimed {
			if slotAcquired {
				_ = c.workerStore.ReleaseSlot(c.workerID)
			}
			return nil
		}

		taskKey := taskScopedName(task)
		stopHeartbeat := c.startHeartbeat(ctx, taskKey)
		c.appendTaskLog(taskKey, fmt.Sprintf("task claimed by worker=%s lease=%s", c.workerID, c.leaseDuration))
		c.appendTaskHistory(&task, "claim", fmt.Sprintf("task claimed by worker=%s lease=%s", c.workerID, c.leaseDuration))
		c.publishTaskEvent(task, "task.claimed", "task claimed by worker")
		c.emitMetering(ctx, agentruntime.MeteringEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Component: "task-controller",
			Type:      "task.attempt_started",
			Namespace: crds.NormalizeNamespace(task.Metadata.Namespace),
			Task:      task.Metadata.Name,
			System:    task.Spec.System,
			Worker:    c.workerID,
			Attempt:   task.Status.Attempts,
			Status:    "running",
		})
		c.emitAudit(ctx, agentruntime.AuditEvent{
			Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
			Component:    "task-controller",
			Action:       "task.attempt_started",
			Outcome:      "success",
			Namespace:    crds.NormalizeNamespace(task.Metadata.Namespace),
			ResourceKind: "Task",
			ResourceName: task.Metadata.Name,
			Principal:    c.workerID,
			Message:      fmt.Sprintf("task %s claimed for attempt %d", task.Metadata.Name, task.Status.Attempts),
		})
		reconcileErr := c.reconcileTask(ctx, task)
		stopHeartbeat()
		if slotAcquired {
			if err := c.workerStore.ReleaseSlot(c.workerID); err != nil && c.logger != nil {
				c.logger.Printf("worker=%s release slot failed: %v", c.workerID, err)
			}
		}
		if reconcileErr != nil {
			if c.logger != nil {
				c.logger.Printf("task=%s reconcile failed: %v", task.Metadata.Name, reconcileErr)
			}
		}
	}
}

func (c *TaskController) reconcileTask(ctx context.Context, task crds.Task) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	phase := strings.ToLower(strings.TrimSpace(task.Status.Phase))
	switch phase {
	case "", "pending":
		if !isAttemptDue(task) {
			return nil
		}
		if err := c.reconcilePending(task); err != nil {
			return err
		}
		updated, ok := c.taskStore.Get(taskScopedName(task))
		if !ok {
			return nil
		}
		if strings.EqualFold(updated.Status.Phase, "running") && strings.EqualFold(updated.Status.ClaimedBy, c.workerID) {
			return c.reconcileRunning(ctx, updated)
		}
		return nil
	case "running":
		return c.reconcileRunning(ctx, task)
	case "succeeded", "failed":
		return nil
	default:
		invalidPhase := task.Status.Phase
		task.Status.Phase = "Failed"
		task.Status.LastError = fmt.Sprintf("unsupported task phase %q", invalidPhase)
		task.Status.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
		task.Status.ObservedGeneration = task.Metadata.Generation
		task.Status.AssignedWorker = ""
		task.Status.ClaimedBy = ""
		task.Status.LeaseUntil = ""
		task.Status.LastHeartbeat = ""
		_, err := c.upsertTask(task)
		c.appendTaskLog(taskScopedName(task), fmt.Sprintf("task failed due to unsupported phase: %s", invalidPhase))
		return err
	}
}

func (c *TaskController) reconcilePending(task crds.Task) error {
	system, errs := c.validateTask(task)
	if len(errs) > 0 {
		return c.markFailed(task, strings.Join(errs, "; "))
	}

	task.Status.Phase = "Running"
	task.Status.LastError = ""
	task.Status.CompletedAt = ""
	task.Status.NextAttemptAt = ""
	task.Status.Output = nil
	task.Status.Messages = nil
	task.Status.Attempts++
	task.Status.ObservedGeneration = task.Metadata.Generation
	task.Status.AssignedWorker = c.workerID
	task.Status.ClaimedBy = c.workerID
	task.Status.LeaseUntil = time.Now().UTC().Add(c.leaseDuration).Format(time.RFC3339Nano)
	task.Status.LastHeartbeat = time.Now().UTC().Format(time.RFC3339Nano)
	c.appendTaskHistory(&task, "running", fmt.Sprintf("task entered running on worker=%s attempt=%d", c.workerID, task.Status.Attempts))
	if task.Status.StartedAt == "" {
		task.Status.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if _, err := c.upsertTask(task); err != nil {
		return err
	}

	if c.logger != nil {
		c.logger.Printf("task=%s transitioned Pending->Running system=%s", task.Metadata.Name, system.Metadata.Name)
	}
	c.publishTaskEvent(task, "task.running", "task transitioned to running")
	c.appendTaskLog(taskScopedName(task), fmt.Sprintf("task transitioned Pending->Running system=%s attempt=%d", system.Metadata.Name, task.Status.Attempts))
	return nil
}

func (c *TaskController) reconcileRunning(ctx context.Context, task crds.Task) error {
	system, errs := c.validateTask(task)
	if len(errs) > 0 {
		return c.handleExecutionFailure(task, strings.Join(errs, "; "))
	}
	if strings.EqualFold(strings.TrimSpace(c.executionMode), "message-driven") {
		return c.reconcileRunningMessageDriven(ctx, task, system)
	}
	c.appendTaskLog(taskScopedName(task), fmt.Sprintf("task execution started system=%s", system.Metadata.Name))

	output, err := c.executeTask(ctx, &task, system)
	if err != nil {
		return c.handleExecutionFailure(task, err.Error())
	}

	task.Status.Phase = "Succeeded"
	task.Status.LastError = ""
	if task.Status.StartedAt == "" {
		task.Status.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	task.Status.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	task.Status.Output = output
	task.Status.ObservedGeneration = task.Metadata.Generation
	task.Status.AssignedWorker = ""
	task.Status.ClaimedBy = ""
	task.Status.LeaseUntil = ""
	task.Status.LastHeartbeat = ""
	if startT, parseErr := time.Parse(time.RFC3339Nano, task.Status.StartedAt); parseErr == nil {
		telemetry.RecordTaskCompletion(crds.NormalizeNamespace(task.Metadata.Namespace), task.Spec.System, "succeeded", time.Since(startT).Seconds())
	}
	c.appendTaskHistory(&task, "succeeded", "task execution completed successfully")
	_, err = c.upsertTask(task)
	if err != nil {
		return err
	}

	if c.logger != nil {
		c.logger.Printf("task=%s transitioned Running->Succeeded", task.Metadata.Name)
	}
	c.publishTaskEvent(task, "task.succeeded", "task execution succeeded")
	c.appendTaskLog(taskScopedName(task), fmt.Sprintf("task transitioned Running->Succeeded agents=%s tokens_used_total=%s tokens_estimated_total=%s",
		output["agents_executed"],
		output["tokens_used_total"],
		output["tokens_estimated_total"],
	))
	tokensUsedTotal, _ := strconv.Atoi(strings.TrimSpace(output["tokens_used_total"]))
	tokensEstimatedTotal, _ := strconv.Atoi(strings.TrimSpace(output["tokens_estimated_total"]))
	c.emitMetering(ctx, agentruntime.MeteringEvent{
		Timestamp:       time.Now().UTC().Format(time.RFC3339Nano),
		Component:       "task-controller",
		Type:            "task.completed",
		Namespace:       crds.NormalizeNamespace(task.Metadata.Namespace),
		Task:            task.Metadata.Name,
		System:          system.Metadata.Name,
		Worker:          c.workerID,
		Attempt:         task.Status.Attempts,
		Status:          "succeeded",
		TokensUsed:      tokensUsedTotal,
		TokensEstimated: tokensEstimatedTotal,
	})
	c.emitAudit(ctx, agentruntime.AuditEvent{
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		Component:    "task-controller",
		Action:       "task.completed",
		Outcome:      "success",
		Namespace:    crds.NormalizeNamespace(task.Metadata.Namespace),
		ResourceKind: "Task",
		ResourceName: task.Metadata.Name,
		Principal:    c.workerID,
		Message:      "task execution succeeded",
		Metadata: map[string]string{
			"tokens_used_total":      output["tokens_used_total"],
			"tokens_estimated_total": output["tokens_estimated_total"],
			"agents_executed":        output["agents_executed"],
		},
	})
	return nil
}

func (c *TaskController) reconcileRunningMessageDriven(ctx context.Context, task crds.Task, system crds.AgentSystem) error {
	if c.agentMessageBus == nil {
		return c.handleExecutionFailure(task, "task execution mode message-driven requires configured agent message bus")
	}
	order := executionOrder(system)
	if len(order) == 0 {
		return c.handleExecutionFailure(task, fmt.Sprintf("cannot derive execution order from agentsystem %q", system.Metadata.Name))
	}
	entryAgents := entryAgentsFromSystem(system)
	if len(entryAgents) == 0 {
		entryAgents = []string{order[0]}
	}
	if task.Status.Output == nil {
		task.Status.Output = map[string]string{}
	}
	allPolicies := []crds.AgentPolicy{}
	if c.policyStore != nil {
		allPolicies = c.policyStore.List()
	}
	policies := matchedPolicies(task, system, allPolicies)
	tokenBudget := minimumTokenBudget(policies)
	if tokenBudget > 0 {
		task.Status.Output["token_budget"] = strconv.Itoa(tokenBudget)
	} else {
		delete(task.Status.Output, "token_budget")
	}
	task.Status.Output["runtime.mode"] = "message-driven"
	task.Status.Output["runtime.entry_agent"] = strings.Join(entryAgents, ",")

	// Kickoff should happen once per attempt; consumers process the rest of the graph.
	if countTraceEventsForType(task.Status.Trace, "task_runtime_kickoff", task.Status.Attempts) >= len(entryAgents) {
		task.Status.ObservedGeneration = task.Metadata.Generation
		if _, err := c.upsertTask(task); err != nil {
			return err
		}
		c.appendTaskLog(taskScopedName(task), fmt.Sprintf("task runtime waiting for message processing attempt=%d", task.Status.Attempts))
		return nil
	}

	content := strings.TrimSpace(task.Spec.Input["topic"])
	if content == "" {
		content = fmt.Sprintf("task=%s attempt=%d", task.Metadata.Name, task.Status.Attempts)
	}
	published := 0
	for idx, entry := range entryAgents {
		if hasKickoffMessage(task.Status.Messages, task.Status.Attempts, entry) {
			continue
		}
		kickoff := crds.TaskMessage{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			FromAgent: "system",
			ToAgent:   entry,
			Type:      "task_start",
			Content:   content,
			BranchID:  fmt.Sprintf("b%03d", idx+1),
		}
		c.populateTaskMessageMetadata(&task, &kickoff, idx)
		c.appendTaskMessage(&task, kickoff)
		if err := c.publishAgentMessage(ctx, &task, kickoff); err != nil {
			return c.handleExecutionFailure(task, err.Error())
		}
		c.appendTaskTrace(&task, crds.TaskTraceEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Type:      "task_runtime_kickoff",
			Attempt:   task.Status.Attempts,
			Agent:     entry,
			BranchID:  kickoff.BranchID,
			Message:   fmt.Sprintf("message_id=%s branch_id=%s", kickoff.MessageID, kickoff.BranchID),
		})
		c.appendTaskHistory(&task, "runtime_kickoff", fmt.Sprintf("message-driven kickoff sent to %s branch=%s", entry, kickoff.BranchID))
		c.appendTaskLog(taskScopedName(task), fmt.Sprintf("task message-driven kickoff published to=%s message_id=%s branch_id=%s", entry, kickoff.MessageID, kickoff.BranchID))
		published++
	}
	task.Status.ObservedGeneration = task.Metadata.Generation
	if _, err := c.upsertTask(task); err != nil {
		return err
	}
	if published > 0 {
		c.publishTaskEvent(task, "task.runtime_kickoff", fmt.Sprintf("kickoff sent to %d entry agents", published))
	}
	return nil
}

func (c *TaskController) markFailed(task crds.Task, reason string) error {
	task.Status.Phase = "Failed"
	task.Status.LastError = reason
	if task.Status.StartedAt == "" {
		task.Status.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	task.Status.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	task.Status.NextAttemptAt = ""
	task.Status.Output = nil
	task.Status.ObservedGeneration = task.Metadata.Generation
	task.Status.AssignedWorker = ""
	task.Status.ClaimedBy = ""
	task.Status.LeaseUntil = ""
	task.Status.LastHeartbeat = ""
	if startT, parseErr := time.Parse(time.RFC3339Nano, task.Status.StartedAt); parseErr == nil {
		telemetry.RecordTaskCompletion(crds.NormalizeNamespace(task.Metadata.Namespace), task.Spec.System, "failed", time.Since(startT).Seconds())
	}
	c.appendTaskHistory(&task, "failed", reason)
	_, err := c.upsertTask(task)
	if err != nil {
		return err
	}
	if c.logger != nil {
		c.logger.Printf("task=%s transitioned to Failed reason=%s", task.Metadata.Name, reason)
	}
	c.publishTaskEvent(task, "task.failed", reason)
	c.appendTaskLog(taskScopedName(task), fmt.Sprintf("task transitioned to Failed reason=%s", reason))
	c.emitMetering(context.Background(), agentruntime.MeteringEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Component: "task-controller",
		Type:      "task.completed",
		Namespace: crds.NormalizeNamespace(task.Metadata.Namespace),
		Task:      task.Metadata.Name,
		System:    task.Spec.System,
		Worker:    c.workerID,
		Attempt:   task.Status.Attempts,
		Status:    "failed",
	})
	c.emitAudit(context.Background(), agentruntime.AuditEvent{
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		Component:    "task-controller",
		Action:       "task.completed",
		Outcome:      "failure",
		Namespace:    crds.NormalizeNamespace(task.Metadata.Namespace),
		ResourceKind: "Task",
		ResourceName: task.Metadata.Name,
		Principal:    c.workerID,
		Message:      reason,
	})
	return nil
}

func (c *TaskController) handleExecutionFailure(task crds.Task, reason string) error {
	reason = agentruntime.RedactSensitive(reason)
	if task.Spec.Retry.MaxAttempts > 0 && task.Status.Attempts >= task.Spec.Retry.MaxAttempts && isRetryableError(reason) {
		return c.markDeadLetter(task, reason)
	}
	if shouldRetryTask(task, reason) {
		delay, err := retryDelay(task)
		if err != nil {
			return c.markFailed(task, fmt.Sprintf("%s; retry configuration error: %v", reason, err))
		}
		next := time.Now().UTC().Add(delay)
		task.Status.Phase = "Pending"
		task.Status.LastError = fmt.Sprintf("%s (retry scheduled in %s)", reason, delay)
		task.Status.CompletedAt = ""
		task.Status.NextAttemptAt = next.Format(time.RFC3339Nano)
		task.Status.Output = nil
		task.Status.Messages = nil
		task.Status.ObservedGeneration = task.Metadata.Generation
		task.Status.AssignedWorker = ""
		task.Status.ClaimedBy = ""
		task.Status.LeaseUntil = ""
		task.Status.LastHeartbeat = ""
		c.appendTaskHistory(&task, "retry_scheduled", fmt.Sprintf("retry scheduled in %s reason=%s", delay, reason))
		if _, err := c.upsertTask(task); err != nil {
			return err
		}
		c.publishTaskEvent(task, "task.retry_scheduled", reason)
		c.appendTaskLog(taskScopedName(task), fmt.Sprintf(
			"retry scheduled: attempt=%d max_attempts=%d next_attempt_at=%s reason=%s",
			task.Status.Attempts,
			task.Spec.Retry.MaxAttempts,
			task.Status.NextAttemptAt,
			reason,
		))
		c.emitMetering(context.Background(), agentruntime.MeteringEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Component: "task-controller",
			Type:      "task.retry_scheduled",
			Namespace: crds.NormalizeNamespace(task.Metadata.Namespace),
			Task:      task.Metadata.Name,
			System:    task.Spec.System,
			Worker:    c.workerID,
			Attempt:   task.Status.Attempts,
			Status:    "pending",
			Metadata: map[string]string{
				"next_attempt_at": task.Status.NextAttemptAt,
				"reason":          reason,
			},
		})
		c.emitAudit(context.Background(), agentruntime.AuditEvent{
			Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
			Component:    "task-controller",
			Action:       "task.retry_scheduled",
			Outcome:      "success",
			Namespace:    crds.NormalizeNamespace(task.Metadata.Namespace),
			ResourceKind: "Task",
			ResourceName: task.Metadata.Name,
			Principal:    c.workerID,
			Message:      reason,
			Metadata: map[string]string{
				"next_attempt_at": task.Status.NextAttemptAt,
			},
		})
		return nil
	}
	return c.markFailed(task, reason)
}

func (c *TaskController) markDeadLetter(task crds.Task, reason string) error {
	task.Status.Phase = "DeadLetter"
	task.Status.LastError = reason
	if task.Status.StartedAt == "" {
		task.Status.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	task.Status.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	task.Status.NextAttemptAt = ""
	task.Status.Output = nil
	task.Status.AssignedWorker = ""
	task.Status.ClaimedBy = ""
	task.Status.LeaseUntil = ""
	task.Status.LastHeartbeat = ""
	task.Status.ObservedGeneration = task.Metadata.Generation
	c.appendTaskHistory(&task, "deadletter", reason)
	_, err := c.upsertTask(task)
	if err != nil {
		return err
	}
	if c.logger != nil {
		c.logger.Printf("task=%s moved to DeadLetter reason=%s", task.Metadata.Name, reason)
	}
	c.publishTaskEvent(task, "task.deadletter", reason)
	c.appendTaskLog(taskScopedName(task), fmt.Sprintf("task moved to DeadLetter reason=%s", reason))
	c.emitMetering(context.Background(), agentruntime.MeteringEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Component: "task-controller",
		Type:      "task.completed",
		Namespace: crds.NormalizeNamespace(task.Metadata.Namespace),
		Task:      task.Metadata.Name,
		System:    task.Spec.System,
		Worker:    c.workerID,
		Attempt:   task.Status.Attempts,
		Status:    "deadletter",
	})
	c.emitAudit(context.Background(), agentruntime.AuditEvent{
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		Component:    "task-controller",
		Action:       "task.completed",
		Outcome:      "deadletter",
		Namespace:    crds.NormalizeNamespace(task.Metadata.Namespace),
		ResourceKind: "Task",
		ResourceName: task.Metadata.Name,
		Principal:    c.workerID,
		Message:      reason,
	})
	return nil
}

func (c *TaskController) validateTask(task crds.Task) (crds.AgentSystem, []string) {
	errs := make([]string, 0)
	if strings.TrimSpace(task.Spec.System) == "" {
		errs = append(errs, "spec.system is required")
		return crds.AgentSystem{}, errs
	}

	system, ok := c.agentSystemStore.Get(store.ScopedName(task.Metadata.Namespace, task.Spec.System))
	if !ok {
		errs = append(errs, fmt.Sprintf("agentsystem %q not found", task.Spec.System))
		return crds.AgentSystem{}, errs
	}

	if len(system.Spec.Agents) == 0 {
		errs = append(errs, fmt.Sprintf("agentsystem %q has no spec.agents", system.Metadata.Name))
		return system, errs
	}

	agentSet := make(map[string]struct{}, len(system.Spec.Agents))
	for _, name := range system.Spec.Agents {
		if strings.TrimSpace(name) == "" {
			errs = append(errs, "agentsystem contains empty agent name")
			continue
		}
		agentSet[name] = struct{}{}
	}

	for _, name := range system.Spec.Agents {
		agent, ok := c.agentStore.Get(store.ScopedName(task.Metadata.Namespace, name))
		if !ok {
			errs = append(errs, fmt.Sprintf("agent %q not found", name))
			continue
		}

		for _, toolName := range agent.Spec.Tools {
			if _, ok := c.toolStore.Get(store.ScopedName(task.Metadata.Namespace, toolName)); !ok {
				errs = append(errs, fmt.Sprintf("agent %q references missing tool %q", name, toolName))
			}
		}
		if c.roleStore != nil {
			for _, roleName := range agent.Spec.Roles {
				if strings.TrimSpace(roleName) == "" {
					continue
				}
				roleKey := store.ScopedName(task.Metadata.Namespace, roleName)
				if _, ok := c.roleStore.Get(roleKey); !ok {
					if _, ok := c.roleStore.Get(roleName); !ok {
						errs = append(errs, fmt.Sprintf("agent %q references missing role %q", name, roleName))
					}
				}
			}
		}
		if c.modelEPStore != nil && strings.TrimSpace(agent.Spec.ModelRef) != "" {
			refNamespace, refName := parseModelEndpointRef(task.Metadata.Namespace, agent.Spec.ModelRef)
			if _, ok := c.modelEPStore.Get(store.ScopedName(refNamespace, refName)); !ok {
				errs = append(errs, fmt.Sprintf("agent %q references missing model endpoint %q", name, agent.Spec.ModelRef))
			}
		}

		if strings.TrimSpace(agent.Spec.Memory.Ref) != "" {
			if _, ok := c.memoryStore.Get(store.ScopedName(task.Metadata.Namespace, agent.Spec.Memory.Ref)); !ok {
				errs = append(errs, fmt.Sprintf("agent %q references missing memory %q", name, agent.Spec.Memory.Ref))
			}
		}
	}

	errs = append(errs, validateGraph(system, agentSet)...)
	if len(system.Spec.Graph) > 0 {
		hasCycle := hasGraphCycle(system.Spec.Graph)
		if hasCycle && task.Spec.MaxTurns <= 0 {
			errs = append(errs, "agentsystem graph contains a cycle; set task.spec.max_turns > 0 to allow cyclical agent handoffs")
		}
		if !hasGraphEntrypoint(system, agentSet) && !(hasCycle && task.Spec.MaxTurns > 0) {
			errs = append(errs, "agentsystem graph has no entrypoint (no zero-indegree agents)")
		}
	}
	return system, errs
}

func validateGraph(system crds.AgentSystem, agentSet map[string]struct{}) []string {
	errs := make([]string, 0)
	graph := system.Spec.Graph
	if len(graph) == 0 {
		return errs
	}

	for node, edge := range graph {
		if _, ok := agentSet[node]; !ok {
			errs = append(errs, fmt.Sprintf("graph node %q is not listed in spec.agents", node))
		}
		joinMode := strings.ToLower(strings.TrimSpace(edge.Join.Mode))
		if joinMode != "" && joinMode != "wait_for_all" && joinMode != "quorum" {
			errs = append(errs, fmt.Sprintf("graph node %q has unsupported join.mode %q", node, edge.Join.Mode))
		}
		if edge.Join.QuorumCount < 0 {
			errs = append(errs, fmt.Sprintf("graph node %q has invalid join.quorum_count %d", node, edge.Join.QuorumCount))
		}
		if edge.Join.QuorumPercent < 0 || edge.Join.QuorumPercent > 100 {
			errs = append(errs, fmt.Sprintf("graph node %q has invalid join.quorum_percent %d (expected 0-100)", node, edge.Join.QuorumPercent))
		}
		onFailure := strings.ToLower(strings.TrimSpace(edge.Join.OnFailure))
		if onFailure != "" && onFailure != "deadletter" && onFailure != "skip" && onFailure != "continue_partial" {
			errs = append(errs, fmt.Sprintf("graph node %q has unsupported join.on_failure %q", node, edge.Join.OnFailure))
		}
		for _, to := range crds.GraphOutgoingAgents(edge) {
			if _, ok := agentSet[to]; !ok {
				errs = append(errs, fmt.Sprintf("graph edge %q -> %q points to unknown agent", node, to))
			}
		}
	}

	return errs
}

func hasGraphCycle(graph map[string]crds.GraphEdge) bool {
	const (
		white = 0
		gray  = 1
		black = 2
	)

	color := make(map[string]int, len(graph))
	for node := range graph {
		color[node] = white
	}

	var visit func(string) bool
	visit = func(node string) bool {
		color[node] = gray
		for _, next := range crds.GraphOutgoingAgents(graph[node]) {
			c, ok := color[next]
			if !ok {
				continue
			}
			if c == gray {
				return true
			}
			if c == white && visit(next) {
				return true
			}
		}
		color[node] = black
		return false
	}

	for node := range graph {
		if color[node] == white {
			if visit(node) {
				return true
			}
		}
	}
	return false
}

func hasGraphEntrypoint(system crds.AgentSystem, agentSet map[string]struct{}) bool {
	if len(system.Spec.Graph) == 0 {
		return len(system.Spec.Agents) > 0
	}
	indegree := make(map[string]int, len(agentSet))
	for name := range agentSet {
		indegree[name] = 0
	}
	for _, edge := range system.Spec.Graph {
		for _, to := range crds.GraphOutgoingAgents(edge) {
			if _, ok := indegree[to]; ok {
				indegree[to]++
			}
		}
	}
	for _, in := range indegree {
		if in == 0 {
			return true
		}
	}
	return false
}

func parseModelEndpointRef(defaultNamespace string, ref string) (namespace string, name string) {
	ref = strings.TrimSpace(ref)
	namespace = crds.NormalizeNamespace(defaultNamespace)
	if strings.Contains(ref, "/") {
		parts := strings.SplitN(ref, "/", 2)
		return crds.NormalizeNamespace(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])
	}
	return namespace, ref
}

func executionOrder(system crds.AgentSystem) []string {
	if len(system.Spec.Agents) == 0 {
		return nil
	}

	// If no graph is present, preserve declaration order.
	if len(system.Spec.Graph) == 0 {
		order := make([]string, len(system.Spec.Agents))
		copy(order, system.Spec.Agents)
		return order
	}

	indegree := make(map[string]int, len(system.Spec.Agents))
	for _, agent := range system.Spec.Agents {
		indegree[agent] = 0
	}
	for _, node := range system.Spec.Graph {
		for _, to := range crds.GraphOutgoingAgents(node) {
			if _, ok := indegree[to]; ok {
				indegree[to]++
			}
		}
	}

	queue := make([]string, 0, len(system.Spec.Agents))
	queued := make(map[string]struct{}, len(system.Spec.Agents))
	for _, agent := range system.Spec.Agents {
		if indegree[agent] != 0 {
			continue
		}
		queue = append(queue, agent)
		queued[agent] = struct{}{}
	}
	if len(queue) == 0 {
		order := make([]string, len(system.Spec.Agents))
		copy(order, system.Spec.Agents)
		return order
	}

	order := make([]string, 0, len(system.Spec.Agents))
	visited := make(map[string]struct{}, len(system.Spec.Agents))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, seen := visited[current]; seen {
			continue
		}
		visited[current] = struct{}{}
		order = append(order, current)

		node, ok := system.Spec.Graph[current]
		if !ok {
			continue
		}
		for _, to := range crds.GraphOutgoingAgents(node) {
			if _, tracked := indegree[to]; !tracked {
				continue
			}
			indegree[to]--
			if indegree[to] == 0 {
				if _, alreadyQueued := queued[to]; alreadyQueued {
					continue
				}
				queue = append(queue, to)
				queued[to] = struct{}{}
			}
		}
	}

	for _, agent := range system.Spec.Agents {
		if _, seen := visited[agent]; seen {
			continue
		}
		order = append(order, agent)
	}
	return order
}

func entryAgentsFromSystem(system crds.AgentSystem) []string {
	if len(system.Spec.Agents) == 0 {
		return nil
	}
	if len(system.Spec.Graph) == 0 {
		out := make([]string, 0, 1)
		first := strings.TrimSpace(system.Spec.Agents[0])
		if first != "" {
			out = append(out, first)
		}
		return out
	}
	indegree := make(map[string]int, len(system.Spec.Agents))
	for _, agent := range system.Spec.Agents {
		indegree[agent] = 0
	}
	for _, edge := range system.Spec.Graph {
		for _, to := range crds.GraphOutgoingAgents(edge) {
			if _, ok := indegree[to]; ok {
				indegree[to]++
			}
		}
	}
	out := make([]string, 0, len(system.Spec.Agents))
	for _, agent := range system.Spec.Agents {
		if indegree[agent] == 0 {
			out = append(out, agent)
		}
	}
	return out
}

func (c *TaskController) executeTask(ctx context.Context, task *crds.Task, system crds.AgentSystem) (map[string]string, error) {
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}
	order := executionOrder(system)
	if len(order) == 0 {
		return nil, fmt.Errorf("cannot derive execution order from agentsystem %q", system.Metadata.Name)
	}

	ctx, taskSpan := telemetry.StartTaskSpan(ctx, task.Metadata.Name, system.Metadata.Name,
		crds.NormalizeNamespace(task.Metadata.Namespace), task.Status.Attempts)
	defer taskSpan.End()

	c.appendTaskTrace(task, crds.TaskTraceEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Type:      "task_start",
		Message:   fmt.Sprintf("system=%s agents=%d", system.Metadata.Name, len(order)),
	})

	policies := matchedPolicies(*task, system, c.policyStore.List())
	tokenBudget := minimumTokenBudget(policies)
	totalEstimatedTokens := 0
	totalUsedTokens := 0

	output := map[string]string{
		"system":            system.Metadata.Name,
		"priority":          task.Spec.Priority,
		"execution_order":   strings.Join(order, " -> "),
		"result":            "executed",
		"policies_enforced": strconv.Itoa(len(policies)),
	}
	if len(policies) > 0 {
		names := make([]string, 0, len(policies))
		for _, policy := range policies {
			names = append(names, policy.Metadata.Name)
		}
		sort.Strings(names)
		output["policies_matched"] = strings.Join(names, ",")
		c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("policy selection: matched=%s", output["policies_matched"]))
	} else {
		c.appendTaskLog(taskScopedName(*task), "policy selection: no policies matched")
	}
	if tokenBudget > 0 {
		output["token_budget"] = strconv.Itoa(tokenBudget)
		c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("token budget active: %d", tokenBudget))
	} else {
		c.appendTaskLog(taskScopedName(*task), "token budget disabled: no max_tokens_per_run policy")
	}
	if topic, ok := task.Spec.Input["topic"]; ok {
		output["topic"] = topic
	}

	runtimeInput := copyStringMap(task.Spec.Input)
	for idx, agentName := range order {
		agent, ok := c.agentStore.Get(store.ScopedName(task.Metadata.Namespace, agentName))
		if !ok {
			c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent missing before execution: %s", agentName))
			c.appendTaskTrace(task, crds.TaskTraceEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Type:      "agent_missing",
				Agent:     agentName,
				Message:   "agent not found",
			})
			return nil, fmt.Errorf("agent %q not found", agentName)
		}
		c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent start: %s model=%s tools=%d", agent.Metadata.Name, agent.Spec.Model, len(agent.Spec.Tools)))
		c.appendTaskTrace(task, crds.TaskTraceEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Type:      "agent_start",
			Agent:     agent.Metadata.Name,
			Message:   fmt.Sprintf("model=%s tools=%d", agent.Spec.Model, len(agent.Spec.Tools)),
		})

		agentCtx, agentSpan := telemetry.StartAgentSpan(ctx, agent.Metadata.Name,
			fmt.Sprintf("a%d.s%d", task.Status.Attempts, idx+1), task.Status.Attempts)

		if err := enforcePoliciesForAgent(agent, policies); err != nil {
			c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent policy violation: %s error=%s", agent.Metadata.Name, err))
			c.appendTaskTrace(task, crds.TaskTraceEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Type:      "policy_violation",
				Agent:     agent.Metadata.Name,
				Message:   err.Error(),
			})
			telemetry.EndSpanError(agentSpan, err)
			return nil, err
		}

		toolRuntime := agentruntime.BuildGovernedToolRuntimeForAgentWithGovernance(
			nil,
			c.isolatedTools,
			c.toolStore,
			c.roleStore,
			c.toolPermStore,
			task.Metadata.Namespace,
			agent,
		)
		result, err := c.executor.ExecuteAgentWithRuntime(agentCtx, agent, runtimeInput, toolRuntime)
		if err != nil {
			category := "failure"
			if strings.Contains(strings.ToLower(err.Error()), "timed out") {
				category = "timeout"
			}
			c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent %s: %s error=%s", agentName, category, err))
			c.appendTaskTrace(task, crds.TaskTraceEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Type:      "agent_error",
				Agent:     agentName,
				Message:   err.Error(),
			})
			telemetry.EndSpanError(agentSpan, fmt.Errorf("agent %q execution failed: %w", agentName, err))
			return nil, fmt.Errorf("agent %q execution failed: %w", agentName, err)
		}
		c.appendRuntimeStepTrace(task, result.Agent, result.StepEvents)
		c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent end: %s steps=%d tool_calls=%d tokens=%d usage_source=%s estimated_tokens=%d duration_ms=%d",
			result.Agent,
			result.Steps,
			result.ToolCalls,
			result.TokensUsed,
			result.TokenSource,
			result.EstimatedTokens,
			result.Duration.Milliseconds(),
		))
		c.appendTaskTrace(task, crds.TaskTraceEvent{
			Timestamp:        time.Now().UTC().Format(time.RFC3339Nano),
			Type:             "agent_end",
			Agent:            result.Agent,
			Message:          result.LastEvent,
			LatencyMS:        result.Duration.Milliseconds(),
			Tokens:           result.TokensUsed,
			TokenUsageSource: strings.TrimSpace(result.TokenSource),
			ToolCalls:        result.ToolCalls,
			MemoryWrites:     result.MemoryWrites,
		})
		c.emitMetering(ctx, agentruntime.MeteringEvent{
			Timestamp:       time.Now().UTC().Format(time.RFC3339Nano),
			Component:       "task-controller",
			Type:            "agent.execution",
			Namespace:       crds.NormalizeNamespace(task.Metadata.Namespace),
			Task:            task.Metadata.Name,
			System:          system.Metadata.Name,
			Agent:           result.Agent,
			Worker:          c.workerID,
			Attempt:         task.Status.Attempts,
			Status:          "succeeded",
			TokensUsed:      result.TokensUsed,
			TokensEstimated: result.EstimatedTokens,
			ToolCalls:       result.ToolCalls,
		})
		c.emitAudit(ctx, agentruntime.AuditEvent{
			Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
			Component:    "task-controller",
			Action:       "agent.execution",
			Outcome:      "success",
			Namespace:    crds.NormalizeNamespace(task.Metadata.Namespace),
			ResourceKind: "Agent",
			ResourceName: result.Agent,
			Principal:    c.workerID,
			Message:      fmt.Sprintf("agent completed in task %s", task.Metadata.Name),
			Metadata: map[string]string{
				"task":             task.Metadata.Name,
				"attempt":          strconv.Itoa(task.Status.Attempts),
				"tokens_used":      strconv.Itoa(result.TokensUsed),
				"tokens_estimated": strconv.Itoa(result.EstimatedTokens),
				"tool_calls":       strconv.Itoa(result.ToolCalls),
			},
		})
		telemetry.EndSpanOK(agentSpan,
			attribute.Int("orloj.tokens.used", result.TokensUsed),
			attribute.Int("orloj.tokens.estimated", result.EstimatedTokens),
			attribute.Int("orloj.tool_calls", result.ToolCalls),
			attribute.Int64("orloj.latency_ms", result.Duration.Milliseconds()),
		)
		telemetry.RecordAgentExecution(result.Agent, agent.Spec.Model, result.Duration.Seconds(), result.TokensUsed, result.EstimatedTokens)

		totalEstimatedTokens += result.EstimatedTokens
		totalUsedTokens += result.TokensUsed
		if tokenBudget > 0 && totalUsedTokens > tokenBudget {
			c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("token budget exceeded after %s: used=%d source=%s budget=%d estimated=%d", agentName, totalUsedTokens, result.TokenSource, tokenBudget, totalEstimatedTokens))
			c.appendTaskTrace(task, crds.TaskTraceEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Type:      "token_budget_exceeded",
				Agent:     agentName,
				Message:   fmt.Sprintf("used=%d source=%s budget=%d estimated=%d", totalUsedTokens, result.TokenSource, tokenBudget, totalEstimatedTokens),
			})
			return nil, fmt.Errorf("token budget exceeded after agent %q: used=%d source=%s budget=%d estimated=%d", agentName, totalUsedTokens, result.TokenSource, tokenBudget, totalEstimatedTokens)
		}

		prefix := fmt.Sprintf("agent.%d", idx+1)
		output[prefix+".name"] = result.Agent
		output[prefix+".model"] = result.Model
		output[prefix+".steps"] = strconv.Itoa(result.Steps)
		output[prefix+".tool_calls"] = strconv.Itoa(result.ToolCalls)
		output[prefix+".memory_writes"] = strconv.Itoa(result.MemoryWrites)
		output[prefix+".duration_ms"] = strconv.FormatInt(result.Duration.Milliseconds(), 10)
		output[prefix+".estimated_tokens"] = strconv.Itoa(result.EstimatedTokens)
		output[prefix+".tokens_used"] = strconv.Itoa(result.TokensUsed)
		output[prefix+".token_usage_source"] = strings.TrimSpace(result.TokenSource)
		output[prefix+".last_event"] = result.LastEvent

		if idx+1 < len(order) {
			nextAgent := order[idx+1]
			content := strings.TrimSpace(result.LastEvent)
			if content == "" {
				content = fmt.Sprintf("steps=%d tool_calls=%d tokens=%d usage_source=%s", result.Steps, result.ToolCalls, result.TokensUsed, strings.TrimSpace(result.TokenSource))
			}
			message := crds.TaskMessage{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				FromAgent: result.Agent,
				ToAgent:   nextAgent,
				Type:      "task_handoff",
				Content:   content,
			}
			c.populateTaskMessageMetadata(task, &message, idx)
			c.appendTaskMessage(task, message)
			if err := c.publishAgentMessage(ctx, task, message); err != nil {
				c.appendTaskTrace(task, crds.TaskTraceEvent{
					Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
					Type:      "agent_message_error",
					Agent:     result.Agent,
					Message:   err.Error(),
				})
				return nil, err
			}
			c.appendTaskTrace(task, crds.TaskTraceEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Type:      "agent_message",
				Agent:     result.Agent,
				BranchID:  message.BranchID,
				Message:   fmt.Sprintf("to=%s content=%s branch_id=%s", nextAgent, content, message.BranchID),
			})
			c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent message: %s -> %s content=%s", result.Agent, nextAgent, content))

			output[prefix+".message_to"] = nextAgent
			output[prefix+".message_content"] = content
			runtimeInput["inbox.from"] = result.Agent
			runtimeInput["inbox.to"] = nextAgent
			runtimeInput["inbox.content"] = content
			runtimeInput["inbox.message_id"] = message.MessageID
			runtimeInput["inbox.trace_id"] = message.TraceID
			runtimeInput["inbox.branch_id"] = message.BranchID
			runtimeInput["inbox.parent_branch_id"] = message.ParentBranchID
		}

		runtimeInput["previous_agent"] = result.Agent
		runtimeInput["previous_agent_last_event"] = result.LastEvent
	}

	output["agents_executed"] = strconv.Itoa(len(order))
	output["tokens_used_total"] = strconv.Itoa(totalUsedTokens)
	output["tokens_estimated_total"] = strconv.Itoa(totalEstimatedTokens)
	if tokenBudget > 0 {
		remainingUsed := max(0, tokenBudget-totalUsedTokens)
		remainingEstimated := max(0, tokenBudget-totalEstimatedTokens)
		output["tokens_used_remaining"] = strconv.Itoa(remainingUsed)
		output["tokens_estimated_remaining"] = strconv.Itoa(remainingEstimated)
	}
	c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("task execution summary: agents=%s tokens_used_total=%d tokens_estimated_total=%d token_budget=%s",
		output["agents_executed"],
		totalUsedTokens,
		totalEstimatedTokens,
		output["token_budget"],
	))
	c.appendTaskTrace(task, crds.TaskTraceEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Type:      "task_summary",
		Message:   fmt.Sprintf("agents=%s tokens_used_total=%d tokens_estimated_total=%d", output["agents_executed"], totalUsedTokens, totalEstimatedTokens),
		Tokens:    totalUsedTokens,
	})
	return output, nil
}

func enforcePoliciesForAgent(agent crds.Agent, policies []crds.AgentPolicy) error {
	for _, policy := range policies {
		if len(policy.Spec.AllowedModels) > 0 && !containsFold(policy.Spec.AllowedModels, agent.Spec.Model) {
			return fmt.Errorf("policy %q disallows model %q for agent %q", policy.Metadata.Name, agent.Spec.Model, agent.Metadata.Name)
		}
		for _, tool := range agent.Spec.Tools {
			if containsFold(policy.Spec.BlockedTools, tool) {
				return fmt.Errorf("policy %q blocks tool %q for agent %q", policy.Metadata.Name, tool, agent.Metadata.Name)
			}
		}
	}
	return nil
}

func matchedPolicies(task crds.Task, system crds.AgentSystem, all []crds.AgentPolicy) []crds.AgentPolicy {
	out := make([]crds.AgentPolicy, 0, len(all))
	for _, policy := range all {
		if policyAppliesTo(policy, task, system) {
			out = append(out, policy)
		}
	}
	return out
}

func policyAppliesTo(policy crds.AgentPolicy, task crds.Task, system crds.AgentSystem) bool {
	mode := strings.ToLower(strings.TrimSpace(policy.Spec.ApplyMode))
	if mode == "" {
		mode = "scoped"
	}
	if mode == "global" {
		return true
	}

	// scoped mode: explicit task/system matches only
	if len(policy.Spec.TargetTasks) > 0 && containsFold(policy.Spec.TargetTasks, task.Metadata.Name) {
		return true
	}
	if len(policy.Spec.TargetSystems) > 0 && containsFold(policy.Spec.TargetSystems, system.Metadata.Name) {
		return true
	}
	return false
}

func minimumTokenBudget(policies []crds.AgentPolicy) int {
	min := 0
	for _, policy := range policies {
		if policy.Spec.MaxTokensPerRun <= 0 {
			continue
		}
		if min == 0 || policy.Spec.MaxTokensPerRun < min {
			min = policy.Spec.MaxTokensPerRun
		}
	}
	return min
}

func containsFold(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}

func isAttemptDue(task crds.Task) bool {
	if strings.TrimSpace(task.Status.NextAttemptAt) == "" {
		return true
	}
	next, err := parseControllerTimestamp(task.Status.NextAttemptAt)
	if err != nil {
		return true
	}
	return !time.Now().UTC().Before(next)
}

func shouldRetryTask(task crds.Task, reason string) bool {
	if task.Spec.Retry.MaxAttempts <= 0 {
		return false
	}
	if task.Status.Attempts >= task.Spec.Retry.MaxAttempts {
		return false
	}
	return isRetryableError(reason)
}

func isRetryableError(reason string) bool {
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "retryable=true"):
		return true
	case strings.Contains(lower, "retryable=false"):
		return false
	}

	nonRetryableMarkers := []string{
		"policy ",
		"disallows model",
		"blocks tool",
		"permission denied",
		"tool_reason=tool_permission_denied",
		"tool_reason=tool_runtime_policy_invalid",
		"tool_reason=tool_unsupported",
		"tool_reason=tool_isolation_unavailable",
		"tool_reason=tool_secret_resolution_failed",
		"token budget exceeded",
		"unsupported task phase",
		"invalid ",
	}
	for _, marker := range nonRetryableMarkers {
		if strings.Contains(lower, marker) {
			return false
		}
	}

	retryableMarkers := []string{
		"timed out",
		"timeout",
		"temporary",
		"transient",
		"connection reset",
		"connection refused",
		"i/o timeout",
		"retryable",
	}
	for _, marker := range retryableMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func retryDelay(task crds.Task) (time.Duration, error) {
	base, err := time.ParseDuration(task.Spec.Retry.Backoff)
	if err != nil {
		return 0, err
	}
	if base <= 0 {
		return 0, nil
	}

	exp := task.Status.Attempts - 1
	if exp < 0 {
		exp = 0
	}
	if exp > 10 {
		exp = 10
	}
	multiplier := 1 << exp
	delay := base * time.Duration(multiplier)
	if delay > 24*time.Hour {
		delay = 24 * time.Hour
	}
	return delay, nil
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

func taskScopedName(task crds.Task) string {
	return store.ScopedName(task.Metadata.Namespace, task.Metadata.Name)
}

func (c *TaskController) taskMatchesWorker(task crds.Task) bool {
	if strings.EqualFold(strings.TrimSpace(task.Spec.Mode), "template") {
		return false
	}
	phase := strings.ToLower(strings.TrimSpace(task.Status.Phase))
	assigned := strings.TrimSpace(task.Status.AssignedWorker)
	// Assignment is a pending-task placement hint. Running tasks may fail over on lease expiry.
	if phase != "running" && assigned != "" && !strings.EqualFold(assigned, c.workerID) {
		return false
	}
	if c.workerStore == nil {
		return true
	}
	worker, ok := c.workerStore.Get(c.workerID)
	if !ok {
		// Allow scheduling when worker registration is not present (for embedded/single-process use).
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(worker.Status.Phase), "ready") &&
		!strings.EqualFold(strings.TrimSpace(worker.Status.Phase), "pending") {
		return false
	}

	req := task.Spec.Requirements
	if strings.TrimSpace(req.Region) != "" && !strings.EqualFold(strings.TrimSpace(req.Region), strings.TrimSpace(worker.Spec.Region)) {
		return false
	}
	if req.GPU && !worker.Spec.Capabilities.GPU {
		return false
	}
	if strings.TrimSpace(req.Model) != "" && len(worker.Spec.Capabilities.SupportedModels) > 0 &&
		!containsFold(worker.Spec.Capabilities.SupportedModels, req.Model) {
		return false
	}
	return true
}

func (c *TaskController) tryAcquireWorkerSlot() (bool, error) {
	worker, acquired, err := c.workerStore.TryAcquireSlot(c.workerID)
	if err != nil {
		return false, err
	}
	if !acquired && worker.Metadata.Name == "" {
		// Embedded/single-process worker can run without explicit Worker registration.
		return true, nil
	}
	if !acquired {
		if c.logger != nil && worker.Metadata.Name != "" {
			maxConcurrent := worker.Spec.MaxConcurrentTasks
			if maxConcurrent <= 0 {
				maxConcurrent = 1
			}
			c.logger.Printf("worker=%s at capacity current=%d max=%d", c.workerID, worker.Status.CurrentTasks, maxConcurrent)
		}
		return false, nil
	}
	return true, nil
}

func (c *TaskController) appendTaskHistory(task *crds.Task, eventType string, message string) {
	if task == nil {
		return
	}
	task.Status.History = append(task.Status.History, crds.TaskHistoryEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Type:      eventType,
		Worker:    c.workerID,
		Message:   message,
	})
	if len(task.Status.History) > 200 {
		task.Status.History = task.Status.History[len(task.Status.History)-200:]
	}
}

func (c *TaskController) appendTaskMessage(task *crds.Task, message crds.TaskMessage) {
	if task == nil {
		return
	}
	if strings.TrimSpace(message.Timestamp) == "" {
		message.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if message.MaxAttempts <= 0 {
		message.MaxAttempts = task.Spec.Retry.MaxAttempts
	}
	if message.MaxAttempts <= 0 {
		message.MaxAttempts = 1
	}
	if strings.TrimSpace(message.Phase) == "" {
		message.Phase = "Queued"
	}
	if strings.TrimSpace(message.IdempotencyKey) == "" {
		message.IdempotencyKey = strings.TrimSpace(message.MessageID)
	}
	task.Status.Messages = append(task.Status.Messages, message)
	if len(task.Status.Messages) > 500 {
		task.Status.Messages = task.Status.Messages[len(task.Status.Messages)-500:]
	}
}

func (c *TaskController) populateTaskMessageMetadata(task *crds.Task, message *crds.TaskMessage, hopIndex int) {
	if task == nil || message == nil {
		return
	}
	namespace := crds.NormalizeNamespace(task.Metadata.Namespace)
	attempt := task.Status.Attempts
	if attempt <= 0 {
		attempt = 1
	}
	if strings.TrimSpace(message.System) == "" {
		message.System = strings.TrimSpace(task.Spec.System)
	}
	if strings.TrimSpace(message.TaskID) == "" {
		message.TaskID = fmt.Sprintf("%s/%s", namespace, task.Metadata.Name)
	}
	if message.Attempt <= 0 {
		message.Attempt = attempt
	}
	if strings.TrimSpace(message.BranchID) == "" {
		message.BranchID = fmt.Sprintf("b%03d", hopIndex+1)
	}
	if strings.TrimSpace(message.MessageID) == "" {
		message.MessageID = deterministicTaskMessageID(namespace, task.Metadata.Name, message.Attempt, hopIndex+1, message.FromAgent, message.ToAgent)
	}
	if strings.TrimSpace(message.IdempotencyKey) == "" {
		message.IdempotencyKey = strings.TrimSpace(message.MessageID)
	}
	if strings.TrimSpace(message.TraceID) == "" {
		message.TraceID = fmt.Sprintf("%s/a%03d", message.TaskID, message.Attempt)
	}
}

func deterministicTaskMessageID(namespace, taskName string, attempt int, hop int, fromAgent, toAgent string) string {
	if attempt <= 0 {
		attempt = 1
	}
	if hop <= 0 {
		hop = 1
	}
	return fmt.Sprintf(
		"%s/%s/a%03d/h%03d/%s/%s",
		crds.NormalizeNamespace(namespace),
		strings.TrimSpace(taskName),
		attempt,
		hop,
		sanitizeMessageToken(fromAgent),
		sanitizeMessageToken(toAgent),
	)
}

func sanitizeMessageToken(raw string) string {
	token := strings.TrimSpace(strings.ToLower(raw))
	if token == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer(" ", "_", "/", "_", ":", "_")
	return replacer.Replace(token)
}

func (c *TaskController) publishAgentMessage(ctx context.Context, task *crds.Task, message crds.TaskMessage) error {
	if c == nil || c.agentMessageBus == nil || task == nil {
		return nil
	}
	envelope := agentruntime.AgentMessage{
		MessageID:      message.MessageID,
		IdempotencyKey: message.IdempotencyKey,
		TaskID:         message.TaskID,
		Attempt:        message.Attempt,
		System:         message.System,
		Namespace:      task.Metadata.Namespace,
		FromAgent:      message.FromAgent,
		ToAgent:        message.ToAgent,
		BranchID:       message.BranchID,
		ParentBranchID: message.ParentBranchID,
		Type:           message.Type,
		Payload:        message.Content,
		Timestamp:      message.Timestamp,
		TraceID:        message.TraceID,
		ParentID:       message.ParentID,
	}
	_, err := c.agentMessageBus.Publish(ctx, envelope)
	if err != nil {
		c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent message publish failed: id=%s to=%s error=%v", message.MessageID, message.ToAgent, err))
		return fmt.Errorf("publish agent message to %s failed: %w", message.ToAgent, err)
	}
	c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent message published: id=%s to=%s", message.MessageID, message.ToAgent))
	return nil
}

func (c *TaskController) appendRuntimeStepTrace(task *crds.Task, agentName string, events []agentruntime.AgentStepEvent) {
	if task == nil || len(events) == 0 {
		return
	}
	for _, runtimeEvent := range events {
		traceEvent := crds.TaskTraceEvent{
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
			Message:             agentruntime.RedactSensitive(strings.TrimSpace(runtimeEvent.Message)),
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
		c.appendTaskTrace(task, traceEvent)
	}
}

func (c *TaskController) appendTaskTrace(task *crds.Task, event crds.TaskTraceEvent) {
	if task == nil {
		return
	}
	if strings.TrimSpace(event.Timestamp) == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Attempt <= 0 {
		event.Attempt = task.Status.Attempts
	}
	if event.Attempt <= 0 {
		event.Attempt = 1
	}
	if event.Step < 0 {
		event.Step = 0
	}
	if strings.TrimSpace(event.StepID) == "" {
		event.StepID = nextTraceStepID(task.Status.Trace, event.Attempt)
	}
	task.Status.Trace = append(task.Status.Trace, event)
	if len(task.Status.Trace) > 500 {
		task.Status.Trace = task.Status.Trace[len(task.Status.Trace)-500:]
	}
}

func hasTraceEventForType(trace []crds.TaskTraceEvent, eventType string, attempt int) bool {
	for _, event := range trace {
		if !strings.EqualFold(strings.TrimSpace(event.Type), strings.TrimSpace(eventType)) {
			continue
		}
		if attempt <= 0 || event.Attempt == 0 || event.Attempt == attempt {
			return true
		}
	}
	return false
}

func countTraceEventsForType(trace []crds.TaskTraceEvent, eventType string, attempt int) int {
	count := 0
	for _, event := range trace {
		if !strings.EqualFold(strings.TrimSpace(event.Type), strings.TrimSpace(eventType)) {
			continue
		}
		if attempt <= 0 || event.Attempt == 0 || event.Attempt == attempt {
			count++
		}
	}
	return count
}

func hasKickoffMessage(messages []crds.TaskMessage, attempt int, agent string) bool {
	target := strings.TrimSpace(agent)
	if target == "" {
		return false
	}
	for _, message := range messages {
		if !strings.EqualFold(strings.TrimSpace(message.Type), "task_start") {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(message.ToAgent), target) {
			continue
		}
		if attempt <= 0 || message.Attempt == 0 || message.Attempt == attempt {
			return true
		}
	}
	return false
}

func nextTraceStepID(trace []crds.TaskTraceEvent, attempt int) string {
	if attempt <= 0 {
		attempt = 1
	}
	maxSeq := 0
	for _, event := range trace {
		eventAttempt := event.Attempt
		if eventAttempt <= 0 {
			if parsedAttempt, seq, ok := parseTraceStepID(event.StepID); ok {
				eventAttempt = parsedAttempt
				if eventAttempt == attempt && seq > maxSeq {
					maxSeq = seq
				}
				continue
			}
		}
		if eventAttempt != attempt {
			continue
		}
		if _, seq, ok := parseTraceStepID(event.StepID); ok {
			if seq > maxSeq {
				maxSeq = seq
			}
			continue
		}
		if event.Step > maxSeq {
			maxSeq = event.Step
		}
	}
	return fmt.Sprintf("a%03d.s%04d", attempt, maxSeq+1)
}

func parseTraceStepID(stepID string) (attempt int, sequence int, ok bool) {
	matches := traceStepIDPattern.FindStringSubmatch(strings.TrimSpace(stepID))
	if len(matches) != 3 {
		return 0, 0, false
	}
	parsedAttempt, err := strconv.Atoi(matches[1])
	if err != nil || parsedAttempt <= 0 {
		return 0, 0, false
	}
	parsedSequence, err := strconv.Atoi(matches[2])
	if err != nil || parsedSequence <= 0 {
		return 0, 0, false
	}
	return parsedAttempt, parsedSequence, true
}

func (c *TaskController) upsertTask(task crds.Task) (crds.Task, error) {
	var lastErr error
	for i := 0; i < 5; i++ {
		updated, err := c.taskStore.Upsert(task)
		if err == nil {
			return updated, nil
		}
		if !store.IsConflict(err) {
			return crds.Task{}, err
		}
		lastErr = err
		current, ok := c.taskStore.Get(taskScopedName(task))
		if !ok {
			return crds.Task{}, err
		}
		task.Metadata.ResourceVersion = current.Metadata.ResourceVersion
		task.Metadata.Generation = current.Metadata.Generation
		task.Metadata.CreatedAt = current.Metadata.CreatedAt
		task.Spec = current.Spec
	}
	if lastErr != nil {
		return crds.Task{}, lastErr
	}
	return c.taskStore.Upsert(task)
}

func (c *TaskController) appendTaskLog(taskName, message string) {
	if strings.TrimSpace(taskName) == "" || strings.TrimSpace(message) == "" {
		return
	}
	if err := c.taskStore.AppendLog(taskName, message); err != nil && c.logger != nil {
		c.logger.Printf("task=%s append log failed: %v", taskName, err)
	}
}

func (c *TaskController) emitMetering(ctx context.Context, event agentruntime.MeteringEvent) {
	if c == nil {
		return
	}
	c.extensions.Metering.RecordMetering(ctx, event)
}

func (c *TaskController) emitAudit(ctx context.Context, event agentruntime.AuditEvent) {
	if c == nil {
		return
	}
	c.extensions.Audit.RecordAudit(ctx, event)
}

func (c *TaskController) startHeartbeat(ctx context.Context, taskName string) func() {
	if c.heartbeatEvery <= 0 {
		return func() {}
	}
	hbCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(c.heartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
				if err := c.taskStore.RenewLease(taskName, c.workerID, c.leaseDuration); err != nil {
					if c.logger != nil {
						c.logger.Printf("task=%s lease heartbeat failed: %v", taskName, err)
					}
					return
				}
			}
		}
	}()
	return cancel
}

func (c *TaskController) publishTaskEvent(task crds.Task, eventType string, message string) {
	if c.eventBus == nil {
		return
	}
	c.eventBus.Publish(eventbus.Event{
		Source:    "task-controller",
		Type:      strings.TrimSpace(eventType),
		Kind:      "Task",
		Name:      task.Metadata.Name,
		Namespace: crds.NormalizeNamespace(task.Metadata.Namespace),
		Action:    strings.ToLower(strings.TrimSpace(task.Status.Phase)),
		Message:   strings.TrimSpace(message),
		Data: map[string]any{
			"phase":          task.Status.Phase,
			"attempts":       task.Status.Attempts,
			"assignedWorker": task.Status.AssignedWorker,
			"claimedBy":      task.Status.ClaimedBy,
			"lastError":      task.Status.LastError,
		},
	})
}

func defaultWorkerID() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "worker"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

func parseControllerTimestamp(value string) (time.Time, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, v)
}
