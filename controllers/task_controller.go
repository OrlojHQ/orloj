package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
	"github.com/OrlojHQ/orloj/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

var traceStepIDPattern = regexp.MustCompile(`^a([0-9]+)\.s([0-9]+)$`) //nolint:gochecknoglobals

// TaskController reconciles Task resources.
type TaskController struct {
	taskStore           *store.TaskStore
	agentSystemStore    *store.AgentSystemStore
	agentStore          *store.AgentStore
	toolStore           *store.ToolStore
	memoryStore         *store.MemoryStore
	policyStore         *store.AgentPolicyStore
	modelEPStore        *store.ModelEndpointStore
	roleStore           *store.AgentRoleStore
	toolPermStore       *store.ToolPermissionStore
	toolApprovalStore   *store.ToolApprovalStore
	taskApprovalStore   *store.TaskApprovalStore
	workerStore         *store.WorkerStore
	executor            *agentruntime.TaskExecutor
	reconcileEvery      time.Duration
	leaseDuration       time.Duration
	heartbeatEvery      time.Duration
	workerID            string
	logger              *log.Logger
	debugLogger         *log.Logger
	eventBus            eventbus.Bus
	agentMessageBus     agentruntime.AgentMessageBus
	executionMode       string
	isolatedTools       agentruntime.ToolRuntime
	wasmTools           agentruntime.ToolRuntime
	kubernetesTools     agentruntime.ToolRuntime
	a2aTools            agentruntime.ToolRuntime
	cliToolConfig       agentruntime.CLIToolRuntimeConfig
	cliSecretResolver   agentruntime.SecretResolver
	mcpSessionMgr       *agentruntime.McpSessionManager
	mcpServerStore      *store.McpServerStore
	contextAdapterStore *store.ContextAdapterStore
	extensions          agentruntime.Extensions
	agentK8sRuntime     *agentruntime.KubernetesAgentRuntime
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

func (c *TaskController) SetDebugLogger(logger *log.Logger) {
	c.debugLogger = logger
}

func (c *TaskController) debugf(format string, args ...any) {
	if c != nil && c.debugLogger != nil {
		c.debugLogger.Printf(format, args...)
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

func (c *TaskController) SetWasmToolRuntime(runtime agentruntime.ToolRuntime) {
	c.wasmTools = runtime
}

func (c *TaskController) SetKubernetesToolRuntime(runtime agentruntime.ToolRuntime) {
	c.kubernetesTools = runtime
}

func (c *TaskController) SetA2AToolRuntime(runtime agentruntime.ToolRuntime) {
	c.a2aTools = runtime
}

func (c *TaskController) SetAgentKubernetesRuntime(rt *agentruntime.KubernetesAgentRuntime) {
	c.agentK8sRuntime = rt
}

func (c *TaskController) SetGovernanceStores(roleStore *store.AgentRoleStore, toolPermStore *store.ToolPermissionStore) {
	c.roleStore = roleStore
	c.toolPermStore = toolPermStore
}

func (c *TaskController) SetToolApprovalStore(s *store.ToolApprovalStore) {
	c.toolApprovalStore = s
}

func (c *TaskController) SetTaskApprovalStore(s *store.TaskApprovalStore) {
	c.taskApprovalStore = s
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

func (c *TaskController) SetContextAdapterStore(s *store.ContextAdapterStore) {
	c.contextAdapterStore = s
}

func (c *TaskController) adaptRuntimeInput(ctx context.Context, task *resources.Task, system resources.AgentSystem, input map[string]string) (map[string]string, error) {
	ref := strings.TrimSpace(system.Spec.ContextAdapter)
	if ref == "" || c.contextAdapterStore == nil {
		return input, nil
	}
	key := store.ScopedName(task.Metadata.Namespace, ref)
	deps := agentruntime.ContextAdapterDeps{
		Tools:          c.toolStore,
		Isolated:       c.isolatedTools,
		Wasm:           c.wasmTools,
		SecretResolver: c.cliSecretResolver,
		Cli:            c.cliToolConfig,
		McpMgr:         c.mcpSessionMgr,
		McpStore:       c.mcpServerStore,
	}
	out, err := agentruntime.AdaptTaskInputViaContextAdapter(ctx, task.Metadata.Namespace, key, c.contextAdapterStore, deps, input)
	if err != nil {
		c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("context adapter failed: %s error=%v", ref, err))
	}
	return out, err
}

func (c *TaskController) SetMcpRuntime(sessionMgr *agentruntime.McpSessionManager, mcpServerStore *store.McpServerStore) {
	c.mcpSessionMgr = sessionMgr
	c.mcpServerStore = mcpServerStore
}

func (c *TaskController) SetCliToolRuntime(config agentruntime.CLIToolRuntimeConfig, secrets agentruntime.SecretResolver) {
	c.cliToolConfig = config
	c.cliSecretResolver = secrets
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
	if err := c.reconcileWaitingApprovalSweep(ctx); err != nil {
		return err
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		slotAcquired := false
		if c.workerStore != nil {
			acquired, err := c.tryAcquireWorkerSlot(ctx)
			if err != nil {
				return err
			}
			if !acquired {
				c.debugf("task scheduler idle worker=%s reason=worker_slot_unavailable", c.workerID)
				return nil
			}
			slotAcquired = true
		}
		task, claimed, err := c.taskStore.ClaimNextDue(ctx, c.workerID, c.leaseDuration, c.workerClaimHints(), c.taskMatchesWorker)
		if err != nil {
			if slotAcquired {
				_ = c.workerStore.ReleaseSlot(ctx, c.workerID)
			}
			return err
		}
		if !claimed {
			if slotAcquired {
				_ = c.workerStore.ReleaseSlot(ctx, c.workerID)
				c.debugf("worker=%s released empty claim slot", c.workerID)
			}
			c.debugf("task scheduler idle worker=%s reason=no_due_matching_task", c.workerID)
			return nil
		}

		taskKey := taskScopedName(task)
		c.debugf("task claimed task=%s worker=%s phase=%s attempt=%d lease=%s execution_mode=%s", taskKey, c.workerID, task.Status.Phase, task.Status.Attempts, c.leaseDuration.String(), c.executionMode)
		stopHeartbeat := c.startHeartbeat(ctx, taskKey)
		c.appendTaskLog(taskKey, fmt.Sprintf("task claimed by worker=%s lease=%s", c.workerID, c.leaseDuration))
		c.appendTaskHistory(&task, "claim", fmt.Sprintf("task claimed by worker=%s lease=%s", c.workerID, c.leaseDuration))
		c.publishTaskEvent(task, "task.claimed", "task claimed by worker")
		c.emitMetering(ctx, agentruntime.MeteringEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Component: "task-controller",
			Type:      "task.attempt_started",
			Namespace: resources.NormalizeNamespace(task.Metadata.Namespace),
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
			Namespace:    resources.NormalizeNamespace(task.Metadata.Namespace),
			ResourceKind: "Task",
			ResourceName: task.Metadata.Name,
			Principal:    c.workerID,
			Message:      fmt.Sprintf("task %s claimed for attempt %d", task.Metadata.Name, task.Status.Attempts),
		})
		reconcileErr := c.reconcileTask(ctx, task)
		stopHeartbeat()
		if slotAcquired {
			if err := c.workerStore.ReleaseSlot(ctx, c.workerID); err != nil {
				if c.logger != nil {
					c.logger.Printf("worker=%s release slot failed: %v", c.workerID, err)
				}
			} else {
				c.debugf("worker=%s released task slot task=%s", c.workerID, taskKey)
			}
		}
		if reconcileErr != nil {
			if c.logger != nil {
				c.logger.Printf("task=%s reconcile failed: %v", task.Metadata.Name, reconcileErr)
			}
		}
	}
}

func (c *TaskController) reconcileTask(ctx context.Context, task resources.Task) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	phase := strings.ToLower(strings.TrimSpace(task.Status.Phase))
	switch phase {
	case "", "pending":
		if !isAttemptDue(task) {
			return nil
		}
		if err := c.reconcilePending(ctx, task); err != nil {
			return err
		}
		updated, ok, err := c.taskStore.Get(ctx, taskScopedName(task))
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if strings.EqualFold(updated.Status.Phase, "running") && strings.EqualFold(updated.Status.ClaimedBy, c.workerID) {
			return c.reconcileRunning(ctx, updated)
		}
		return nil
	case "running":
		return c.reconcileRunning(ctx, task)
	case "waitingapproval":
		return c.reconcileWaitingApproval(ctx, task)
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

func (c *TaskController) reconcilePending(ctx context.Context, task resources.Task) error {
	system, errs, pendingMCP := c.validateTask(ctx, task)
	if len(errs) > 0 {
		return c.markFailed(task, strings.Join(errs, "; "))
	}
	if len(pendingMCP) > 0 {
		// MCP server(s) are still starting; keep the task Pending and retry.
		if c.logger != nil {
			c.logger.Printf("task=%s mcp tools not yet available, will retry: %s", task.Metadata.Name, strings.Join(pendingMCP, "; "))
		}
		return fmt.Errorf("mcp tools not yet available (will retry): %s", strings.Join(pendingMCP, "; "))
	}

	task.Status.Phase = "Running"
	task.Status.LastError = ""
	task.Status.CompletedAt = ""
	task.Status.NextAttemptAt = ""
	task.Status.Output = nil
	task.Status.Messages = nil
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

func (c *TaskController) reconcileRunning(ctx context.Context, task resources.Task) error {
	system, errs, pendingMCP := c.validateTask(ctx, task)
	if len(pendingMCP) > 0 {
		errs = append(errs, pendingMCP...)
	}
	if len(errs) > 0 {
		return c.handleExecutionFailure(task, strings.Join(errs, "; "))
	}
	if strings.EqualFold(strings.TrimSpace(c.executionMode), "message-driven") {
		return c.reconcileRunningMessageDriven(ctx, task, system)
	}
	c.appendTaskLog(taskScopedName(task), fmt.Sprintf("task execution started system=%s", system.Metadata.Name))

	output, err := c.executeTask(ctx, &task, system)
	if err != nil {
		var reviewPauseErr *taskApprovalPauseError
		if errors.As(err, &reviewPauseErr) {
			return c.transitionToWaitingApproval(task, reviewPauseErr.reason, &resources.TaskBlockedOn{
				Kind:   "TaskApproval",
				Name:   reviewPauseErr.approvalName,
				Reason: "output_review",
			})
		}
		if agentruntime.IsApprovalRequiredError(err) {
			approvalName := c.createSyncToolApproval(ctx, task, err)
			return c.transitionToWaitingApproval(task, err.Error(), &resources.TaskBlockedOn{
				Kind:   "ToolApproval",
				Name:   approvalName,
				Reason: "tool_approval",
			})
		}
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
		telemetry.RecordTaskCompletion(resources.NormalizeNamespace(task.Metadata.Namespace), task.Spec.System, "succeeded", time.Since(startT).Seconds())
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
		Namespace:       resources.NormalizeNamespace(task.Metadata.Namespace),
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
		Namespace:    resources.NormalizeNamespace(task.Metadata.Namespace),
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

// reconcileWaitingApprovalSweep resumes tasks stuck in WaitingApproval after POST /approve updates
// the ToolApproval in the store. ClaimNextDue never selects WaitingApproval (isTaskClaimable is false),
// so reconcileWaitingApproval would otherwise never run.
func (c *TaskController) reconcileWaitingApprovalSweep(ctx context.Context) error {
	if c == nil || c.taskStore == nil || (c.toolApprovalStore == nil && c.taskApprovalStore == nil) {
		return nil
	}
	_taskList, err := c.taskStore.List(ctx)
	if err != nil {
		return err
	}
	for _, task := range _taskList {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !strings.EqualFold(strings.TrimSpace(task.Status.Phase), "waitingapproval") {
			continue
		}
		if !c.taskMatchesWorker(task) {
			continue
		}
		key := taskScopedName(task)
		fresh, ok, err := c.taskStore.Get(ctx, key)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(fresh.Status.Phase), "waitingapproval") {
			continue
		}
		if err := c.reconcileWaitingApproval(ctx, fresh); err != nil {
			return err
		}
	}
	return nil
}

func (c *TaskController) createSyncToolApproval(ctx context.Context, task resources.Task, execErr error) string {
	if c.toolApprovalStore == nil {
		return ""
	}
	toolName := agentruntime.ParseToolFromApprovalError(execErr)
	if toolName == "" {
		return ""
	}
	taskKey := taskScopedName(task)
	syncMsgID := fmt.Sprintf("sync/a%d", task.Status.Attempts)
	approvalName := agentruntime.ToolApprovalResourceName(taskKey, syncMsgID)
	ns := resources.NormalizeNamespace(task.Metadata.Namespace)

	reason := strings.TrimSpace(execErr.Error())
	if len(reason) > 500 {
		reason = reason[:500]
	}

	approval := resources.ToolApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "ToolApproval",
		Metadata: resources.ObjectMeta{
			Namespace: ns,
			Name:      approvalName,
		},
		Spec: resources.ToolApprovalSpec{
			TaskRef:        taskKey,
			Tool:           toolName,
			Agent:          "",
			Reason:         reason,
			OperationClass: "write",
		},
		Status: resources.ToolApprovalStatus{
			Phase: "Pending",
		},
	}
	if _, err := c.toolApprovalStore.Upsert(ctx, approval); err != nil {
		if c.logger != nil {
			c.logger.Printf("task=%s failed to create tool approval: %v", task.Metadata.Name, err)
		}
	}
	return approvalName
}

func (c *TaskController) transitionToWaitingApproval(task resources.Task, reason string, blockedOn *resources.TaskBlockedOn) error {
	task.Status.Phase = "WaitingApproval"
	task.Status.LastError = agentruntime.RedactSensitive(reason)
	task.Status.BlockedOn = blockedOn
	task.Status.ObservedGeneration = task.Metadata.Generation
	c.appendTaskHistory(&task, "waiting_approval", fmt.Sprintf("task paused pending approval: %s", reason))
	if _, err := c.upsertTask(task); err != nil {
		return err
	}
	if c.logger != nil {
		c.logger.Printf("task=%s transitioned Running->WaitingApproval", task.Metadata.Name)
	}
	c.publishTaskEvent(task, "task.waiting_approval", "task paused pending tool approval")
	c.appendTaskLog(taskScopedName(task), fmt.Sprintf("task transitioned Running->WaitingApproval reason=%s", reason))
	return nil
}

func (c *TaskController) reconcileWaitingApproval(ctx context.Context, task resources.Task) error {
	if task.Status.BlockedOn != nil {
		switch strings.ToLower(strings.TrimSpace(task.Status.BlockedOn.Kind)) {
		case "toolapproval":
			return c.reconcileWaitingToolApproval(ctx, task, strings.TrimSpace(task.Status.BlockedOn.Name))
		case "taskapproval":
			return c.reconcileWaitingTaskApproval(ctx, task, strings.TrimSpace(task.Status.BlockedOn.Name))
		}
	}
	return c.reconcileWaitingToolApprovalLegacy(ctx, task)
}

func (c *TaskController) reconcileWaitingToolApprovalLegacy(ctx context.Context, task resources.Task) error {
	if c.toolApprovalStore == nil {
		return nil
	}
	taskKey := taskScopedName(task)
	_aList, err := c.toolApprovalStore.List(ctx)
	if err != nil {
		return err
	}
	for _, a := range _aList {
		if strings.TrimSpace(a.Spec.TaskRef) == taskKey || strings.TrimSpace(a.Spec.TaskRef) == task.Metadata.Name {
			return c.applyWaitingToolApproval(ctx, task, a)
		}
	}
	return nil
}

func (c *TaskController) reconcileWaitingToolApproval(ctx context.Context, task resources.Task, approvalName string) error {
	if c.toolApprovalStore == nil || strings.TrimSpace(approvalName) == "" {
		return nil
	}
	approval, ok, err := c.toolApprovalStore.Get(ctx, store.ScopedName(task.Metadata.Namespace, approvalName))
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return c.applyWaitingToolApproval(ctx, task, approval)
}

func (c *TaskController) applyWaitingToolApproval(ctx context.Context, task resources.Task, approval resources.ToolApproval) error {
	taskKey := taskScopedName(task)
	if approval.Status.ExpiresAt != "" {
		if expiresAt, err := time.Parse(time.RFC3339, approval.Status.ExpiresAt); err == nil {
			if time.Now().UTC().After(expiresAt) && approval.Status.Phase == "Pending" {
				approval.Status.Phase = "Expired"
				_, _ = c.toolApprovalStore.Upsert(ctx, approval)
			}
		}
	}

	switch approval.Status.Phase {
	case "Approved":
		task.Status.Phase = "Running"
		task.Status.LastError = ""
		task.Status.BlockedOn = nil
		task.Status.AssignedWorker = c.workerID
		task.Status.ClaimedBy = c.workerID
		task.Status.LeaseUntil = time.Now().UTC().Add(c.leaseDuration).Format(time.RFC3339Nano)
		task.Status.LastHeartbeat = time.Now().UTC().Format(time.RFC3339Nano)
		c.appendTaskTrace(&task, resources.TaskTraceEvent{
			Type:    "tool_approval_approved",
			Agent:   strings.TrimSpace(approval.Spec.Agent),
			Message: fmt.Sprintf("tool=%s decided_by=%s", strings.TrimSpace(approval.Spec.Tool), strings.TrimSpace(approval.Status.DecidedBy)),
		})
		c.appendTaskHistory(&task, "running", "tool approval granted, task resumed")
		if _, err := c.upsertTask(task); err != nil {
			return err
		}
		if c.logger != nil {
			c.logger.Printf("task=%s transitioned WaitingApproval->Running (approved)", task.Metadata.Name)
		}
		c.publishTaskEvent(task, "task.running", "tool approval granted, task resumed")
		c.appendTaskLog(taskKey, "task transitioned WaitingApproval->Running (approved)")
	case "Denied":
		c.appendTaskTrace(&task, resources.TaskTraceEvent{
			Type:    "tool_approval_denied",
			Agent:   strings.TrimSpace(approval.Spec.Agent),
			Message: fmt.Sprintf("tool=%s decided_by=%s comment=%s", strings.TrimSpace(approval.Spec.Tool), strings.TrimSpace(approval.Status.DecidedBy), strings.TrimSpace(approval.Status.Comment)),
		})
		return c.markFailed(task, "tool approval denied")
	case "Expired":
		c.appendTaskTrace(&task, resources.TaskTraceEvent{
			Type:    "tool_approval_expired",
			Agent:   strings.TrimSpace(approval.Spec.Agent),
			Message: fmt.Sprintf("tool=%s", strings.TrimSpace(approval.Spec.Tool)),
		})
		return c.markFailed(task, "tool approval expired")
	}
	return nil
}

func (c *TaskController) reconcileWaitingTaskApproval(ctx context.Context, task resources.Task, approvalName string) error {
	if c.taskApprovalStore == nil || strings.TrimSpace(approvalName) == "" {
		return nil
	}
	approval, ok, err := c.taskApprovalStore.Get(ctx, store.ScopedName(task.Metadata.Namespace, approvalName))
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if approval.Status.ExpiresAt != "" {
		if expiresAt, err := time.Parse(time.RFC3339, approval.Status.ExpiresAt); err == nil {
			if time.Now().UTC().After(expiresAt) && approval.Status.Phase == "Pending" {
				approval.Status.Phase = "Expired"
				_, _ = c.taskApprovalStore.Upsert(ctx, approval)
			}
		}
	}
	switch approval.Status.Phase {
	case "Approved":
		c.appendTaskTrace(&task, resources.TaskTraceEvent{
			Type:    "task_approval_approved",
			Agent:   strings.TrimSpace(approval.Spec.Agent),
			Message: fmt.Sprintf("checkpoint=%s decided_by=%s", strings.TrimSpace(approval.Spec.CheckpointID), strings.TrimSpace(approval.Status.DecidedBy)),
		})
		return c.resumeApprovedTaskApproval(ctx, task, approval)
	case "Denied":
		c.appendTaskTrace(&task, resources.TaskTraceEvent{
			Type:    "task_approval_denied",
			Agent:   strings.TrimSpace(approval.Spec.Agent),
			Message: fmt.Sprintf("checkpoint=%s decided_by=%s comment=%s", strings.TrimSpace(approval.Spec.CheckpointID), strings.TrimSpace(approval.Status.DecidedBy), strings.TrimSpace(approval.Status.Comment)),
		})
		return c.markFailed(task, "task approval denied")
	case "Expired":
		c.appendTaskTrace(&task, resources.TaskTraceEvent{
			Type:    "task_approval_expired",
			Agent:   strings.TrimSpace(approval.Spec.Agent),
			Message: fmt.Sprintf("checkpoint=%s", strings.TrimSpace(approval.Spec.CheckpointID)),
		})
		return c.markFailed(task, "task approval expired")
	case "ChangesRequested":
		c.appendTaskTrace(&task, resources.TaskTraceEvent{
			Type:    "task_approval_changes_requested",
			Agent:   strings.TrimSpace(approval.Spec.Agent),
			Message: fmt.Sprintf("checkpoint=%s decided_by=%s cycle=%d comment=%s", strings.TrimSpace(approval.Spec.CheckpointID), strings.TrimSpace(approval.Status.DecidedBy), approval.Spec.ReviewCycle, strings.TrimSpace(approval.Status.Comment)),
		})
		return c.resumeRequestedChangesTaskApproval(ctx, task, approval)
	default:
		return nil
	}
}

type taskApprovalPauseError struct {
	approvalName string
	reason       string
}

func (e *taskApprovalPauseError) Error() string {
	if e == nil {
		return "task approval pending"
	}
	return e.reason
}

func (c *TaskController) resumeApprovedTaskApproval(ctx context.Context, task resources.Task, approval resources.TaskApproval) error {
	resume, err := resources.DecodeTaskApprovalResumeContext(approval.Spec.ResumeContext)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(resume.Mode)) {
	case "message-driven":
		return c.resumeMessageDrivenTaskApproval(ctx, task, approval, resume, false)
	case "sequential":
		return c.resumeSequentialTaskApproval(ctx, task, approval, resume, false)
	default:
		return fmt.Errorf("unsupported task approval resume mode %q", resume.Mode)
	}
}

func (c *TaskController) resumeRequestedChangesTaskApproval(ctx context.Context, task resources.Task, approval resources.TaskApproval) error {
	if !resources.TaskApprovalAllowsRequestChanges(approval) {
		return c.markFailed(task, "task approval request_changes is disabled for this checkpoint")
	}
	if approval.Spec.ReviewCycle >= resources.TaskApprovalMaxReviewCycles(approval) {
		return c.markFailed(task, "task approval max review cycles reached")
	}
	resume, err := resources.DecodeTaskApprovalResumeContext(approval.Spec.ResumeContext)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(resume.Mode)) {
	case "message-driven":
		return c.resumeMessageDrivenTaskApproval(ctx, task, approval, resume, true)
	case "sequential":
		return c.resumeSequentialTaskApproval(ctx, task, approval, resume, true)
	default:
		return fmt.Errorf("unsupported task approval resume mode %q", resume.Mode)
	}
}

func resumeMessageToTaskMessage(msg resources.TaskApprovalResumeMessage) resources.TaskMessage {
	return resources.TaskMessage{
		Timestamp:      strings.TrimSpace(msg.Timestamp),
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
		DelegateOf:     strings.TrimSpace(msg.DelegateOf),
	}
}

func ensureTaskMessageRecordController(task *resources.Task, message resources.TaskMessage) int {
	if task == nil {
		return -1
	}
	for idx, existing := range task.Status.Messages {
		if strings.EqualFold(strings.TrimSpace(existing.MessageID), strings.TrimSpace(message.MessageID)) {
			if strings.TrimSpace(message.IdempotencyKey) == "" {
				message.IdempotencyKey = strings.TrimSpace(existing.IdempotencyKey)
			}
			task.Status.Messages[idx] = message
			return idx
		}
	}
	task.Status.Messages = append(task.Status.Messages, message)
	return len(task.Status.Messages) - 1
}

func allTaskMessagesTerminalController(messages []resources.TaskMessage) bool {
	if len(messages) == 0 {
		return true
	}
	for _, message := range messages {
		switch strings.ToLower(strings.TrimSpace(message.Phase)) {
		case "succeeded", "deadletter":
		default:
			return false
		}
	}
	return true
}

func stringifyTaskApprovalOutput(output any) string {
	switch value := output.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]string:
		raw, _ := json.Marshal(value)
		return string(raw)
	default:
		raw, _ := json.Marshal(output)
		return string(raw)
	}
}

func (c *TaskController) maybeCreateCompletionReviewFromTaskApproval(ctx context.Context, task resources.Task, approval resources.TaskApproval, resume resources.TaskApprovalResumeContext) (bool, error) {
	if c.taskApprovalStore == nil || c.agentSystemStore == nil || strings.EqualFold(strings.TrimSpace(approval.Spec.CheckpointType), "task_output") {
		return false, nil
	}
	system, ok, err := c.agentSystemStore.Get(ctx, store.ScopedName(task.Metadata.Namespace, task.Spec.System))
	if err != nil {
		return false, err
	}
	if !ok || system.Spec.CompletionReview == nil {
		return false, nil
	}
	if strings.EqualFold(strings.TrimSpace(approval.Spec.CheckpointID), strings.TrimSpace(system.Spec.CompletionReview.CheckpointID)) {
		return false, nil
	}
	taskKey := taskScopedName(task)
	approvalName := agentruntime.TaskApprovalResourceName(taskKey, system.Spec.CompletionReview.CheckpointID, 1)
	nextResume := resources.TaskApprovalResumeContext{
		Mode:           strings.TrimSpace(resume.Mode),
		Action:         "message_complete",
		System:         strings.TrimSpace(task.Spec.System),
		ProducingAgent: strings.TrimSpace(resume.ProducingAgent),
		CurrentMessage: resume.CurrentMessage,
		Output:         copyStringMap(task.Status.Output),
	}
	created := resources.TaskApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskApproval",
		Metadata: resources.ObjectMeta{
			Namespace: task.Metadata.Namespace,
			Name:      approvalName,
		},
		Spec: resources.TaskApprovalSpec{
			TaskRef:             taskKey,
			CheckpointID:        system.Spec.CompletionReview.CheckpointID,
			CheckpointType:      "task_output",
			Agent:               strings.TrimSpace(resume.ProducingAgent),
			Reason:              strings.TrimSpace(system.Spec.CompletionReview.Reason),
			TTL:                 strings.TrimSpace(system.Spec.CompletionReview.TTL),
			AllowRequestChanges: system.Spec.CompletionReview.AllowRequestChanges,
			MaxReviewCycles:     system.Spec.CompletionReview.MaxReviewCycles,
			ReviewCycle:         1,
			Output:              copyStringMap(task.Status.Output),
			OutputFormat:        "json",
			ResumeContext:       mustEncodeTaskApprovalResumeContext(nextResume),
		},
		Status: resources.TaskApprovalStatus{Phase: "Pending"},
	}
	if _, err := c.taskApprovalStore.Upsert(ctx, created); err != nil {
		return false, err
	}
	task.Status.Phase = "WaitingApproval"
	task.Status.BlockedOn = &resources.TaskBlockedOn{
		Kind:   "TaskApproval",
		Name:   approvalName,
		Reason: "output_review",
	}
	task.Status.LastError = ""
	task.Status.ObservedGeneration = task.Metadata.Generation
	if _, err := c.upsertTask(task); err != nil {
		return false, err
	}
	c.appendTaskLog(taskKey, fmt.Sprintf("task paused for completion review: approval=%s", approvalName))
	return true, nil
}

func (c *TaskController) resumeMessageDrivenTaskApproval(ctx context.Context, task resources.Task, approval resources.TaskApproval, resume resources.TaskApprovalResumeContext, requestChanges bool) error {
	if resume.CurrentMessage == nil {
		return fmt.Errorf("task approval %q is missing resume current_message", approval.Metadata.Name)
	}
	currentMessage := resumeMessageToTaskMessage(*resume.CurrentMessage)
	currentIndex := ensureTaskMessageRecordController(&task, currentMessage)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	current := task.Status.Messages[currentIndex]
	current.Phase = "Succeeded"
	current.Worker = c.workerID
	current.ProcessedAt = now
	current.NextAttemptAt = ""
	current.LastError = ""
	task.Status.Messages[currentIndex] = current

	if requestChanges {
		if c.agentMessageBus == nil {
			return fmt.Errorf("agent message bus is required to resume task approval request_changes")
		}
		rerun := agentruntime.AgentMessageFromResumeMessage(*resume.CurrentMessage)
		rerun.MessageID = fmt.Sprintf("%s/review-%d", strings.TrimSpace(resume.CurrentMessage.MessageID), approval.Spec.ReviewCycle+1)
		rerun.IdempotencyKey = rerun.MessageID
		rerun.Type = "review_request_changes"
		rerun.ParentID = strings.TrimSpace(resume.CurrentMessage.MessageID)
		rerun.Timestamp = now
		rerun.Payload = agentruntime.EncodeReviewRequestPayload(agentruntime.ReviewRequestPayload{
			Content:        strings.TrimSpace(resume.CurrentMessage.Payload),
			Feedback:       strings.TrimSpace(approval.Status.Comment),
			PreviousOutput: stringifyTaskApprovalOutput(approval.Spec.Output),
			CheckpointID:   strings.TrimSpace(approval.Spec.CheckpointID),
			Cycle:          approval.Spec.ReviewCycle + 1,
			RequestedBy:    strings.TrimSpace(approval.Status.DecidedBy),
			Supersedes:     strings.TrimSpace(approval.Metadata.Name),
		})
		if _, err := c.agentMessageBus.Publish(ctx, rerun); err != nil {
			return err
		}
		rerunRecord := resumeMessageToTaskMessage(agentruntime.ResumeMessageFromAgentMessage(rerun))
		rerunRecord.Phase = "Queued"
		rerunRecord.Worker = ""
		rerunRecord.ProcessedAt = ""
		rerunRecord.NextAttemptAt = ""
		rerunRecord.LastError = ""
		ensureTaskMessageRecordController(&task, rerunRecord)
		task.Status.Phase = "Running"
		task.Status.BlockedOn = nil
		task.Status.LastError = ""
		task.Status.AssignedWorker = c.workerID
		task.Status.ClaimedBy = c.workerID
		task.Status.LeaseUntil = time.Now().UTC().Add(c.leaseDuration).Format(time.RFC3339Nano)
		task.Status.LastHeartbeat = now
		task.Status.ObservedGeneration = task.Metadata.Generation
		if _, err := c.upsertTask(task); err != nil {
			return err
		}
		c.appendTaskLog(taskScopedName(task), fmt.Sprintf("task approval requested changes: rerun_message=%s", rerun.MessageID))
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(resume.Action)) {
	case "message_forward":
		if c.agentMessageBus == nil {
			return fmt.Errorf("agent message bus is required to resume task approval")
		}
		for _, stored := range resume.NextMessages {
			envelope := agentruntime.AgentMessageFromResumeMessage(stored)
			if _, err := c.agentMessageBus.Publish(ctx, envelope); err != nil {
				return err
			}
			record := resumeMessageToTaskMessage(stored)
			record.Phase = "Queued"
			record.Worker = ""
			record.ProcessedAt = ""
			record.NextAttemptAt = ""
			record.LastError = ""
			ensureTaskMessageRecordController(&task, record)
		}
		task.Status.Phase = "Running"
		task.Status.BlockedOn = nil
		task.Status.LastError = ""
		task.Status.AssignedWorker = c.workerID
		task.Status.ClaimedBy = c.workerID
		task.Status.LeaseUntil = time.Now().UTC().Add(c.leaseDuration).Format(time.RFC3339Nano)
		task.Status.LastHeartbeat = now
		task.Status.ObservedGeneration = task.Metadata.Generation
		if _, err := c.upsertTask(task); err != nil {
			return err
		}
		c.appendTaskLog(taskScopedName(task), fmt.Sprintf("task approval granted: forwarded %d downstream message(s)", len(resume.NextMessages)))
		return nil
	default:
		if len(resume.Output) > 0 {
			task.Status.Output = copyStringMap(resume.Output)
		}
		if created, err := c.maybeCreateCompletionReviewFromTaskApproval(ctx, task, approval, resume); err != nil {
			return err
		} else if created {
			return nil
		}
		if allTaskMessagesTerminalController(task.Status.Messages) {
			task.Status.Phase = "Succeeded"
			task.Status.CompletedAt = now
			task.Status.AssignedWorker = ""
			task.Status.ClaimedBy = ""
			task.Status.LeaseUntil = ""
			task.Status.LastHeartbeat = ""
		} else {
			task.Status.Phase = "Running"
			task.Status.AssignedWorker = c.workerID
			task.Status.ClaimedBy = c.workerID
			task.Status.LeaseUntil = time.Now().UTC().Add(c.leaseDuration).Format(time.RFC3339Nano)
			task.Status.LastHeartbeat = now
		}
		task.Status.BlockedOn = nil
		task.Status.LastError = ""
		task.Status.ObservedGeneration = task.Metadata.Generation
		if _, err := c.upsertTask(task); err != nil {
			return err
		}
		c.appendTaskLog(taskScopedName(task), "task approval granted: task resumed/completed")
		return nil
	}
}

func (c *TaskController) resumeSequentialTaskApproval(ctx context.Context, task resources.Task, approval resources.TaskApproval, resume resources.TaskApprovalResumeContext, requestChanges bool) error {
	system, ok, err := c.agentSystemStore.Get(ctx, store.ScopedName(task.Metadata.Namespace, task.Spec.System))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("agentsystem %q not found", task.Spec.System)
	}
	order := resources.ExecutionAgentOrder(system)
	if len(order) == 0 {
		return fmt.Errorf("cannot derive execution order from agentsystem %q", system.Metadata.Name)
	}

	startIndex := resume.NextAgentIndex
	runtimeInput := copyStringMap(resume.NextRuntimeInput)
	if requestChanges {
		startIndex = resume.CurrentAgentIndex
		runtimeInput = copyStringMap(resume.RuntimeInput)
		runtimeInput["review.feedback"] = strings.TrimSpace(approval.Status.Comment)
		runtimeInput["review.previous_output"] = stringifyTaskApprovalOutput(approval.Spec.Output)
		runtimeInput["review.checkpoint_id"] = strings.TrimSpace(approval.Spec.CheckpointID)
		runtimeInput["review.cycle"] = strconv.Itoa(approval.Spec.ReviewCycle + 1)
		runtimeInput["review.requested_by"] = strings.TrimSpace(approval.Status.DecidedBy)
		runtimeInput["review.supersedes"] = strings.TrimSpace(approval.Metadata.Name)
	}
	if !requestChanges && startIndex >= len(order) {
		if created, err := c.maybeCreateCompletionReviewFromTaskApproval(ctx, task, approval, resume); err != nil {
			return err
		} else if created {
			return nil
		}
		task.Status.Phase = "Succeeded"
		task.Status.LastError = ""
		task.Status.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
		task.Status.Output = copyStringMap(resume.Output)
		task.Status.AssignedWorker = ""
		task.Status.ClaimedBy = ""
		task.Status.LeaseUntil = ""
		task.Status.LastHeartbeat = ""
		task.Status.BlockedOn = nil
		task.Status.ObservedGeneration = task.Metadata.Generation
		if _, err := c.upsertTask(task); err != nil {
			return err
		}
		return nil
	}

	task.Status.Phase = "Running"
	task.Status.BlockedOn = nil
	task.Status.LastError = ""
	task.Status.AssignedWorker = c.workerID
	task.Status.ClaimedBy = c.workerID
	task.Status.LeaseUntil = time.Now().UTC().Add(c.leaseDuration).Format(time.RFC3339Nano)
	task.Status.LastHeartbeat = time.Now().UTC().Format(time.RFC3339Nano)
	task.Status.ObservedGeneration = task.Metadata.Generation
	if _, err := c.upsertTask(task); err != nil {
		return err
	}

	output, err := c.executeTaskFromResume(ctx, &task, system, order, startIndex, runtimeInput, copyStringMap(resume.Output))
	if err != nil {
		var pauseErr *taskApprovalPauseError
		if errors.As(err, &pauseErr) {
			return c.transitionToWaitingApproval(task, pauseErr.reason, &resources.TaskBlockedOn{
				Kind:   "TaskApproval",
				Name:   pauseErr.approvalName,
				Reason: "output_review",
			})
		}
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
	task.Status.BlockedOn = nil
	c.appendTaskHistory(&task, "succeeded", "task execution completed successfully")
	_, err = c.upsertTask(task)
	if err != nil {
		return err
	}
	c.appendTaskLog(taskScopedName(task), "task approval resume completed successfully")
	return nil
}

func (c *TaskController) reconcileRunningMessageDriven(ctx context.Context, task resources.Task, system resources.AgentSystem) error {
	if c.agentMessageBus == nil {
		return c.handleExecutionFailure(task, "task execution mode message-driven requires configured agent message bus")
	}
	order := resources.ExecutionAgentOrder(system)
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
	allPolicies := []resources.AgentPolicy{}
	if c.policyStore != nil {
		if listed, err := c.policyStore.List(ctx); err == nil {
			allPolicies = listed
		}
	}
	policies := agentruntime.MatchedPolicies(task, system, allPolicies)
	tokenBudget := agentruntime.MinimumTokenBudget(policies)
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
		kickoff := resources.TaskMessage{
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
		c.appendTaskTrace(&task, resources.TaskTraceEvent{
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

func (c *TaskController) markFailed(task resources.Task, reason string) error {
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
		telemetry.RecordTaskCompletion(resources.NormalizeNamespace(task.Metadata.Namespace), task.Spec.System, "failed", time.Since(startT).Seconds())
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
		Namespace: resources.NormalizeNamespace(task.Metadata.Namespace),
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
		Namespace:    resources.NormalizeNamespace(task.Metadata.Namespace),
		ResourceKind: "Task",
		ResourceName: task.Metadata.Name,
		Principal:    c.workerID,
		Message:      reason,
	})
	return nil
}

func (c *TaskController) handleExecutionFailure(task resources.Task, reason string) error {
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
			Namespace: resources.NormalizeNamespace(task.Metadata.Namespace),
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
			Namespace:    resources.NormalizeNamespace(task.Metadata.Namespace),
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

func (c *TaskController) markDeadLetter(task resources.Task, reason string) error {
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
		Namespace: resources.NormalizeNamespace(task.Metadata.Namespace),
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
		Namespace:    resources.NormalizeNamespace(task.Metadata.Namespace),
		ResourceKind: "Task",
		ResourceName: task.Metadata.Name,
		Principal:    c.workerID,
		Message:      reason,
	})
	return nil
}

// validateTask checks that all resources referenced by a task exist and are
// consistent. It returns (system, permanentErrors, pendingMCPHints). When
// pendingMCPHints is non-empty the caller should requeue rather than fail
// permanently: the referenced tools are expected to appear once an MCP server
// finishes connecting.
func (c *TaskController) validateTask(ctx context.Context, task resources.Task) (resources.AgentSystem, []string, []string) {
	errs := make([]string, 0)
	var pendingMCP []string
	if strings.TrimSpace(task.Spec.System) == "" {
		errs = append(errs, "spec.system is required")
		return resources.AgentSystem{}, errs, nil
	}

	system, ok, err := c.agentSystemStore.Get(ctx, store.ScopedName(task.Metadata.Namespace, task.Spec.System))
	if err != nil {
		return resources.AgentSystem{}, append(errs, fmt.Sprintf("agentsystem lookup failed: %v", err)), nil
	}
	if !ok {
		errs = append(errs, fmt.Sprintf("agentsystem %q not found", task.Spec.System))
		return resources.AgentSystem{}, errs, nil
	}

	if len(system.Spec.Agents) == 0 {
		errs = append(errs, fmt.Sprintf("agentsystem %q has no spec.agents", system.Metadata.Name))
		return system, errs, nil
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
		agent, ok, err := c.agentStore.Get(ctx, store.ScopedName(task.Metadata.Namespace, name))
		if err != nil {
			errs = append(errs, fmt.Sprintf("agent %q lookup failed: %v", name, err))
			continue
		}
		if !ok {
			errs = append(errs, fmt.Sprintf("agent %q not found", name))
			continue
		}

		for _, toolName := range agent.Spec.Tools {
			if _, ok, _ := c.toolStore.Get(ctx, store.ScopedName(task.Metadata.Namespace, toolName)); !ok {
				// If the tool name looks like an MCP-generated name (contains "--"),
				// check whether the originating MCP server is still starting up. If
				// so, treat this as a transient condition and requeue rather than
				// permanently failing the task.
				if c.mcpServerStore != nil && strings.Contains(toolName, "--") {
					serverName := strings.SplitN(toolName, "--", 2)[0]
					if server, found, _ := c.mcpServerStore.Get(ctx, store.ScopedName(task.Metadata.Namespace, serverName)); found {
						phase := strings.ToLower(strings.TrimSpace(server.Status.Phase))
						if phase != "ready" && phase != "error" {
							pendingMCP = append(pendingMCP, fmt.Sprintf("agent %q references mcp tool %q (server %q not ready: phase=%q)", name, toolName, serverName, server.Status.Phase))
							continue
						}
					}
				}
				errs = append(errs, fmt.Sprintf("agent %q references missing tool %q", name, toolName))
			}
		}
		if c.roleStore != nil {
			for _, roleName := range agent.Spec.Roles {
				if strings.TrimSpace(roleName) == "" {
					continue
				}
				roleKey := store.ScopedName(task.Metadata.Namespace, roleName)
				if _, ok, _ := c.roleStore.Get(ctx, roleKey); !ok {
					if strings.Contains(roleName, "/") {
						if _, ok, _ := c.roleStore.Get(ctx, roleName); !ok {
							errs = append(errs, fmt.Sprintf("agent %q references missing role %q", name, roleName))
						}
					} else {
						errs = append(errs, fmt.Sprintf("agent %q references missing role %q", name, roleName))
					}
				}
			}
		}
		if strings.TrimSpace(agent.Spec.ModelRef) == "" {
			errs = append(errs, fmt.Sprintf("agent %q must set spec.model_ref", name))
		} else if _, _, err := resolveAgentEffectiveModel(ctx, task.Metadata.Namespace, agent, c.modelEPStore); err != nil {
			errs = append(errs, fmt.Sprintf("agent %q model resolution failed: %s", name, err.Error()))
		}

		if strings.TrimSpace(agent.Spec.Memory.Ref) != "" {
			if _, ok, _ := c.memoryStore.Get(ctx, store.ScopedName(task.Metadata.Namespace, agent.Spec.Memory.Ref)); !ok {
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
	return system, errs, pendingMCP
}

func validateGraph(system resources.AgentSystem, agentSet map[string]struct{}) []string {
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
		for _, to := range resources.GraphOutgoingAgents(edge) {
			if _, ok := agentSet[to]; !ok {
				errs = append(errs, fmt.Sprintf("graph edge %q -> %q points to unknown agent", node, to))
			}
		}
	}

	return errs
}

func hasGraphCycle(graph map[string]resources.GraphEdge) bool {
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
		for _, next := range resources.GraphOutgoingAgents(graph[node]) {
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

func hasGraphEntrypoint(system resources.AgentSystem, agentSet map[string]struct{}) bool {
	if len(system.Spec.Graph) == 0 {
		return len(system.Spec.Agents) > 0
	}
	indegree := make(map[string]int, len(agentSet))
	for name := range agentSet {
		indegree[name] = 0
	}
	for _, edge := range system.Spec.Graph {
		for _, to := range resources.GraphOutgoingAgents(edge) {
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

func resolveAgentEffectiveModel(ctx context.Context, defaultNamespace string, agent resources.Agent, modelEPStore *store.ModelEndpointStore) (resources.ModelEndpoint, string, error) {
	if modelEPStore == nil {
		return resources.ModelEndpoint{}, "", fmt.Errorf("no model endpoint store configured")
	}
	return resources.ResolveAgentModelRef(ctx, defaultNamespace, agent.Spec.ModelRef, modelEPStore)
}

func entryAgentsFromSystem(system resources.AgentSystem) []string {
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
		for _, to := range resources.GraphOutgoingAgents(edge) {
			if _, ok := indegree[to]; ok {
				indegree[to]++
			}
		}
		// Delegates are reachable from this node and must not be treated as
		// entry points, even though they don't appear in Edges.
		for _, d := range edge.Delegates {
			to := strings.TrimSpace(d.To)
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

func reviewCheckpointForNode(system resources.AgentSystem, agentName string) *resources.ReviewCheckpointSpec {
	node, ok := system.Spec.Graph[strings.TrimSpace(agentName)]
	if !ok {
		return nil
	}
	return node.Review
}

func reviewCycleFromInput(input map[string]string) (int, string) {
	cycle := 1
	if input != nil {
		if raw := strings.TrimSpace(input["review.cycle"]); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				cycle = parsed
			}
		}
	}
	supersedes := ""
	if input != nil {
		supersedes = strings.TrimSpace(input["review.supersedes"])
	}
	return cycle, supersedes
}

func (c *TaskController) createTaskApprovalForCheckpoint(
	ctx context.Context,
	task *resources.Task,
	checkpoint resources.ReviewCheckpointSpec,
	checkpointType string,
	agentName string,
	outputSnapshot any,
	outputFormat string,
	resume resources.TaskApprovalResumeContext,
	reviewCycle int,
	supersedes string,
) (resources.TaskApproval, error) {
	if c.taskApprovalStore == nil || task == nil {
		return resources.TaskApproval{}, fmt.Errorf("task approval store not configured")
	}
	taskKey := taskScopedName(*task)
	if reviewCycle <= 0 {
		reviewCycle = 1
	}
	approvalName := agentruntime.TaskApprovalResourceName(taskKey, checkpoint.CheckpointID, reviewCycle)
	reason := strings.TrimSpace(checkpoint.Reason)
	if reason == "" {
		reason = fmt.Sprintf("checkpoint %s requires review", checkpoint.CheckpointID)
	}
	approval := resources.TaskApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskApproval",
		Metadata: resources.ObjectMeta{
			Namespace: task.Metadata.Namespace,
			Name:      approvalName,
		},
		Spec: resources.TaskApprovalSpec{
			TaskRef:             taskKey,
			CheckpointID:        strings.TrimSpace(checkpoint.CheckpointID),
			CheckpointType:      strings.TrimSpace(checkpointType),
			Agent:               strings.TrimSpace(agentName),
			Reason:              reason,
			TTL:                 strings.TrimSpace(checkpoint.TTL),
			AllowRequestChanges: checkpoint.AllowRequestChanges,
			MaxReviewCycles:     checkpoint.MaxReviewCycles,
			ReviewCycle:         reviewCycle,
			Supersedes:          strings.TrimSpace(supersedes),
			Output:              outputSnapshot,
			OutputFormat:        strings.TrimSpace(outputFormat),
			ResumeContext:       mustEncodeTaskApprovalResumeContext(resume),
		},
		Status: resources.TaskApprovalStatus{Phase: "Pending"},
	}
	return c.taskApprovalStore.Upsert(ctx, approval)
}

func (c *TaskController) executeTask(ctx context.Context, task *resources.Task, system resources.AgentSystem) (map[string]string, error) {
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}
	order := resources.ExecutionAgentOrder(system)
	if len(order) == 0 {
		return nil, fmt.Errorf("cannot derive execution order from agentsystem %q", system.Metadata.Name)
	}

	// Restore the W3C trace context from the task annotations so this span is
	// linked as a continuation of the HTTP request that created the task.
	ctx = telemetry.ExtractTraceContext(ctx, task.Metadata.Annotations)
	ctx, taskSpan := telemetry.StartTaskSpan(ctx, task.Metadata.Name, system.Metadata.Name,
		resources.NormalizeNamespace(task.Metadata.Namespace), task.Status.Attempts)
	defer taskSpan.End()

	c.appendTaskTrace(task, resources.TaskTraceEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Type:      "task_start",
		Message:   fmt.Sprintf("system=%s agents=%d", system.Metadata.Name, len(order)),
	})

	var _allPolicies []resources.AgentPolicy
	if c.policyStore != nil {
		if listed, err := c.policyStore.List(ctx); err == nil {
			_allPolicies = listed
		}
	}
	policies := agentruntime.MatchedPolicies(*task, system, _allPolicies)
	tokenBudget := agentruntime.MinimumTokenBudget(policies)
	orlojMaxDepth := agentruntime.MinimumChildDepth(policies)
	orlojMaxChildren := agentruntime.MinimumChildTasks(policies)
	orlojCurrentDepth := parseOrlojDepthLabel(task.Metadata.Labels)
	totalEstimatedTokens := 0
	totalUsedTokens := 0
	var lastAgentInputBefore map[string]string

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
	runtimeInput, adaptErr := c.adaptRuntimeInput(ctx, task, system, runtimeInput)
	if adaptErr != nil {
		return nil, fmt.Errorf("context adapter: %w", adaptErr)
	}
	if c.eventBus != nil {
		c.executor.OnStepEvent = func(evt agentruntime.AgentStepEvent) {
			c.eventBus.Publish(eventbus.Event{
				Source:    "task-controller",
				Type:      "task.trace",
				Kind:      "Task",
				Name:      task.Metadata.Name,
				Namespace: resources.NormalizeNamespace(task.Metadata.Namespace),
				Data:      evt,
			})
		}
	}
	defer func() { c.executor.OnStepEvent = nil }()
	for idx, agentName := range order {
		agentInputBefore := copyStringMap(runtimeInput)
		lastAgentInputBefore = copyStringMap(agentInputBefore)
		agent, ok, err := c.agentStore.Get(ctx, store.ScopedName(task.Metadata.Namespace, agentName))
		if err != nil {
			return nil, err
		}
		if !ok {
			c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent missing before execution: %s", agentName))
			c.appendTaskTrace(task, resources.TaskTraceEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Type:      "agent_missing",
				Agent:     agentName,
				Message:   "agent not found",
			})
			return nil, fmt.Errorf("agent %q not found", agentName)
		}
		_, effectiveModel, err := resolveAgentEffectiveModel(ctx, task.Metadata.Namespace, agent, c.modelEPStore)
		if err != nil {
			c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent model resolution failed: %s error=%s", agent.Metadata.Name, err))
			c.appendTaskTrace(task, resources.TaskTraceEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Type:      "model_resolution_error",
				Agent:     agent.Metadata.Name,
				Message:   err.Error(),
			})
			return nil, fmt.Errorf("agent %q model resolution failed: %w", agentName, err)
		}
		agent.Spec.Model = effectiveModel
		c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent start: %s model=%s model_ref=%s tools=%d", agent.Metadata.Name, effectiveModel, agent.Spec.ModelRef, len(agent.Spec.Tools)))
		c.appendTaskTrace(task, resources.TaskTraceEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Type:      "agent_start",
			Agent:     agent.Metadata.Name,
			Message:   fmt.Sprintf("model=%s model_ref=%s tools=%d", effectiveModel, agent.Spec.ModelRef, len(agent.Spec.Tools)),
		})

		agentCtx, agentSpan := telemetry.StartAgentSpan(ctx, agent.Metadata.Name,
			fmt.Sprintf("a%d.s%d", task.Status.Attempts, idx+1), task.Status.Attempts)

		if err := agentruntime.EnforcePoliciesForAgent(agent, effectiveModel, policies); err != nil {
			c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent policy violation: %s error=%s", agent.Metadata.Name, err))
			c.appendTaskTrace(task, resources.TaskTraceEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Type:      "policy_violation",
				Agent:     agent.Metadata.Name,
				Message:   err.Error(),
			})
			telemetry.EndSpanError(agentSpan, err)
			return nil, err
		}

		var approvalCtx *agentruntime.GovernedToolApprovalContext
		if c.toolApprovalStore != nil {
			taskKey := taskScopedName(*task)
			syncMsgID := fmt.Sprintf("sync/a%d/%s", task.Status.Attempts, agentName)
			store := c.toolApprovalStore
			approvalCtx = &agentruntime.GovernedToolApprovalContext{
				Getter: func(key string) (resources.ToolApproval, bool, error) {
					return store.Get(ctx, key)
				},
				TaskKey:   taskKey,
				MessageID: syncMsgID,
			}
		}
		toolRuntime := agentruntime.BuildGovernedToolRuntimeForAgentWithGovernance(
			agentCtx,
			nil,
			c.isolatedTools,
			c.toolStore,
			c.roleStore,
			c.toolPermStore,
			task.Metadata.Namespace,
			agent,
			approvalCtx,
		)
		if c.mcpSessionMgr != nil && c.mcpServerStore != nil {
			agentruntime.ConfigureMcpRuntime(toolRuntime, c.mcpSessionMgr, c.mcpServerStore, task.Metadata.Namespace)
		}
		agentruntime.ConfigureHttpRuntime(toolRuntime, c.cliSecretResolver, task.Metadata.Namespace)
		agentruntime.ConfigureCliRuntime(toolRuntime, c.cliSecretResolver, nil, c.cliToolConfig, task.Metadata.Namespace)
		agentruntime.ConfigureExternalRuntime(toolRuntime, c.cliSecretResolver, task.Metadata.Namespace)
		agentruntime.ConfigureGRPCRuntime(toolRuntime, c.cliSecretResolver, task.Metadata.Namespace)
		agentruntime.ConfigureWebhookCallbackRuntime(toolRuntime, c.cliSecretResolver, task.Metadata.Namespace)
		agentruntime.ConfigureWasmRuntime(toolRuntime, c.wasmTools, task.Metadata.Namespace)
		if c.kubernetesTools != nil {
			agentruntime.ConfigureKubernetesRuntime(toolRuntime, c.kubernetesTools, task.Metadata.Namespace)
		}
		if c.a2aTools != nil {
			agentruntime.ConfigureA2ARuntime(toolRuntime, c.a2aTools, task.Metadata.Namespace)
		}
		var finalRT agentruntime.ToolRuntime = toolRuntime
		if agentruntime.AgentHasOrlojTools(agent) {
			finalRT = agentruntime.NewOrlojToolRuntime(toolRuntime, c.taskStore, agentruntime.OrlojToolConfig{
				ParentNamespace: task.Metadata.Namespace,
				ParentTaskName:  task.Metadata.Name,
				CurrentDepth:    orlojCurrentDepth,
				MaxDepth:        orlojMaxDepth,
				MaxChildren:     orlojMaxChildren,
			})
			agent.Spec.Tools = append(agent.Spec.Tools, agentruntime.BuiltinOrlojToolNames()...)
			agent.Spec.Tools = dedupeStringsController(agent.Spec.Tools)
		}

		var result agentruntime.AgentExecutionResult
		if c.agentK8sRuntime != nil && c.canRunAgentAsJob(agentCtx, agent) {
			syncMsgID := fmt.Sprintf("sync/a%d/%s", task.Status.Attempts, agentName)
			jobResult, jobErr := c.agentK8sRuntime.ExecuteAgent(agentCtx, *task, agent, runtimeInput, task.Status.Attempts, syncMsgID)
			if jobErr != nil {
				c.logger.Printf("agent k8s job failed for %s (falling back to in-process): %v", agentName, jobErr)
				result, err = c.executor.ExecuteAgentWithRuntime(agentCtx, agent, runtimeInput, finalRT)
			} else if jobResult.Error != "" {
				result = agentruntime.AgentJobResultToExecution(jobResult, agentName)
				err = fmt.Errorf("%s", jobResult.Error)
			} else {
				result = agentruntime.AgentJobResultToExecution(jobResult, agentName)
			}
		} else {
			result, err = c.executor.ExecuteAgentWithRuntime(agentCtx, agent, runtimeInput, finalRT)
		}
		if err != nil {
			category := "failure"
			if strings.Contains(strings.ToLower(err.Error()), "timed out") {
				category = "timeout"
			}
			c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent %s: %s error=%s", agentName, category, err))
			c.appendTaskTrace(task, resources.TaskTraceEvent{
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
		c.appendTaskTrace(task, resources.TaskTraceEvent{
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
			Namespace:       resources.NormalizeNamespace(task.Metadata.Namespace),
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
			Namespace:    resources.NormalizeNamespace(task.Metadata.Namespace),
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
		telemetry.RecordAgentExecution(result.Agent, effectiveModel, result.Duration.Seconds(), result.TokensUsed, result.EstimatedTokens)

		totalEstimatedTokens += result.EstimatedTokens
		totalUsedTokens += result.TokensUsed
		if agentBudget := agentruntime.AgentTokenBudget(policies, agentName); agentBudget > 0 && result.TokensUsed > agentBudget {
			c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("per-agent token budget exceeded for %s: used=%d source=%s budget=%d", agentName, result.TokensUsed, result.TokenSource, agentBudget))
			c.appendTaskTrace(task, resources.TaskTraceEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Type:      "token_budget_exceeded",
				Agent:     agentName,
				Message:   fmt.Sprintf("per-agent used=%d source=%s budget=%d", result.TokensUsed, result.TokenSource, agentBudget),
			})
			return nil, fmt.Errorf("per-agent token budget exceeded for %q: used=%d source=%s budget=%d", agentName, result.TokensUsed, result.TokenSource, agentBudget)
		}
		if tokenBudget > 0 && totalUsedTokens > tokenBudget {
			c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("token budget exceeded after %s: used=%d source=%s budget=%d estimated=%d", agentName, totalUsedTokens, result.TokenSource, tokenBudget, totalEstimatedTokens))
			c.appendTaskTrace(task, resources.TaskTraceEvent{
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
		task.Status.Output = copyStringMap(output)

		nextRuntimeInput := copyStringMap(runtimeInput)
		delete(nextRuntimeInput, "review.feedback")
		delete(nextRuntimeInput, "review.previous_output")
		delete(nextRuntimeInput, "review.checkpoint_id")
		delete(nextRuntimeInput, "review.cycle")
		delete(nextRuntimeInput, "review.requested_by")
		delete(nextRuntimeInput, "review.supersedes")
		nextRuntimeInput["previous_agent"] = result.Agent
		nextRuntimeInput["previous_agent_last_event"] = result.LastEvent
		var plannedMessage *resources.TaskMessage

		if idx+1 < len(order) {
			nextAgent := order[idx+1]
			content := strings.TrimSpace(result.Output)
			if content == "" {
				content = strings.TrimSpace(result.LastEvent)
			}
			if content == "" {
				content = fmt.Sprintf("steps=%d tool_calls=%d tokens=%d usage_source=%s", result.Steps, result.ToolCalls, result.TokensUsed, strings.TrimSpace(result.TokenSource))
			}
			message := resources.TaskMessage{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				FromAgent: result.Agent,
				ToAgent:   nextAgent,
				Type:      "task_handoff",
				Content:   content,
			}
			c.populateTaskMessageMetadata(task, &message, idx)
			output[prefix+".message_to"] = nextAgent
			output[prefix+".message_content"] = content
			nextRuntimeInput["inbox.from"] = result.Agent
			nextRuntimeInput["inbox.to"] = nextAgent
			nextRuntimeInput["inbox.content"] = content
			nextRuntimeInput["inbox.message_id"] = message.MessageID
			nextRuntimeInput["inbox.trace_id"] = message.TraceID
			nextRuntimeInput["inbox.branch_id"] = message.BranchID
			nextRuntimeInput["inbox.parent_branch_id"] = message.ParentBranchID
			plannedMessage = &message
		}

		if checkpoint := reviewCheckpointForNode(system, agentName); checkpoint != nil && c.taskApprovalStore != nil {
			reviewCycle, supersedes := reviewCycleFromInput(runtimeInput)
			approvalOutput := strings.TrimSpace(result.Output)
			if approvalOutput == "" {
				approvalOutput = strings.TrimSpace(result.LastEvent)
			}
			approval, err := c.createTaskApprovalForCheckpoint(
				ctx,
				task,
				*checkpoint,
				"agent_output",
				result.Agent,
				approvalOutput,
				"text",
				resources.TaskApprovalResumeContext{
					Mode:              "sequential",
					Action:            "sequential_continue",
					System:            strings.TrimSpace(task.Spec.System),
					ProducingAgent:    result.Agent,
					RuntimeInput:      agentInputBefore,
					NextRuntimeInput:  nextRuntimeInput,
					Output:            copyStringMap(output),
					CurrentAgentIndex: idx,
					NextAgentIndex:    idx + 1,
				},
				reviewCycle,
				supersedes,
			)
			if err != nil {
				return nil, err
			}
			task.Status.Output = copyStringMap(output)
			return nil, &taskApprovalPauseError{
				approvalName: approval.Metadata.Name,
				reason:       fmt.Sprintf("task output review pending: %s", checkpoint.CheckpointID),
			}
		}

		if plannedMessage != nil {
			c.appendTaskMessage(task, *plannedMessage)
			if err := c.publishAgentMessage(ctx, task, *plannedMessage); err != nil {
				c.appendTaskTrace(task, resources.TaskTraceEvent{
					Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
					Type:      "agent_message_error",
					Agent:     result.Agent,
					Message:   err.Error(),
				})
				return nil, err
			}
			c.appendTaskTrace(task, resources.TaskTraceEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Type:      "agent_message",
				Agent:     result.Agent,
				BranchID:  plannedMessage.BranchID,
				Message:   fmt.Sprintf("to=%s content=%s branch_id=%s", plannedMessage.ToAgent, plannedMessage.Content, plannedMessage.BranchID),
			})
			c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("agent message: %s -> %s content=%s", result.Agent, plannedMessage.ToAgent, plannedMessage.Content))
		}

		if idx+1 < len(order) {
			if _, uErr := c.upsertTask(*task); uErr != nil {
				c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("intermediate upsert after agent %s failed: %v", agentName, uErr))
			}
		}
		runtimeInput = nextRuntimeInput
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
	c.appendTaskTrace(task, resources.TaskTraceEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Type:      "task_summary",
		Message:   fmt.Sprintf("agents=%s tokens_used_total=%d tokens_estimated_total=%d", output["agents_executed"], totalUsedTokens, totalEstimatedTokens),
		Tokens:    totalUsedTokens,
	})
	task.Status.Output = copyStringMap(output)
	if system.Spec.CompletionReview != nil && c.taskApprovalStore != nil {
		lastAgent := ""
		if len(order) > 0 {
			lastAgent = order[len(order)-1]
		}
		completionInput := copyStringMap(runtimeInput)
		if len(lastAgentInputBefore) > 0 {
			completionInput = copyStringMap(lastAgentInputBefore)
		}
		reviewCycle, supersedes := reviewCycleFromInput(completionInput)
		approval, err := c.createTaskApprovalForCheckpoint(
			ctx,
			task,
			*system.Spec.CompletionReview,
			"task_output",
			lastAgent,
			copyStringMap(output),
			"json",
			resources.TaskApprovalResumeContext{
				Mode:              "sequential",
				Action:            "sequential_finalize",
				System:            strings.TrimSpace(task.Spec.System),
				ProducingAgent:    lastAgent,
				RuntimeInput:      copyStringMap(completionInput),
				NextRuntimeInput:  copyStringMap(completionInput),
				Output:            copyStringMap(output),
				CurrentAgentIndex: len(order) - 1,
				NextAgentIndex:    len(order),
			},
			reviewCycle,
			supersedes,
		)
		if err != nil {
			return nil, err
		}
		return nil, &taskApprovalPauseError{
			approvalName: approval.Metadata.Name,
			reason:       fmt.Sprintf("task output review pending: %s", system.Spec.CompletionReview.CheckpointID),
		}
	}
	return output, nil
}

func (c *TaskController) executeTaskFromResume(
	ctx context.Context,
	task *resources.Task,
	system resources.AgentSystem,
	order []string,
	startIndex int,
	runtimeInput map[string]string,
	output map[string]string,
) (map[string]string, error) {
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}
	if len(order) == 0 {
		order = resources.ExecutionAgentOrder(system)
	}
	if len(order) == 0 {
		return nil, fmt.Errorf("cannot derive execution order from agentsystem %q", system.Metadata.Name)
	}
	if startIndex < 0 {
		startIndex = 0
	}
	if startIndex >= len(order) {
		return copyStringMap(output), nil
	}
	if runtimeInput == nil {
		runtimeInput = copyStringMap(task.Spec.Input)
	}
	if output == nil {
		output = map[string]string{}
	}
	if output["system"] == "" {
		output["system"] = system.Metadata.Name
	}
	if output["priority"] == "" {
		output["priority"] = task.Spec.Priority
	}
	if output["execution_order"] == "" {
		output["execution_order"] = strings.Join(order, " -> ")
	}
	if output["result"] == "" {
		output["result"] = "executed"
	}
	if output["topic"] == "" {
		if topic, ok := task.Spec.Input["topic"]; ok {
			output["topic"] = topic
		}
	}

	var allPolicies []resources.AgentPolicy
	if c.policyStore != nil {
		if listed, err := c.policyStore.List(ctx); err == nil {
			allPolicies = listed
		}
	}
	policies := agentruntime.MatchedPolicies(*task, system, allPolicies)
	tokenBudget := agentruntime.MinimumTokenBudget(policies)
	orlojMaxDepth := agentruntime.MinimumChildDepth(policies)
	orlojMaxChildren := agentruntime.MinimumChildTasks(policies)
	orlojCurrentDepth := parseOrlojDepthLabel(task.Metadata.Labels)
	totalEstimatedTokens, _ := strconv.Atoi(strings.TrimSpace(output["tokens_estimated_total"]))
	totalUsedTokens, _ := strconv.Atoi(strings.TrimSpace(output["tokens_used_total"]))
	var lastAgentInputBefore map[string]string

	c.appendTaskTrace(task, resources.TaskTraceEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Type:      "task_resume",
		Message:   fmt.Sprintf("system=%s start_index=%d", system.Metadata.Name, startIndex),
	})

	if startIndex == 0 {
		var adaptErr error
		runtimeInput, adaptErr = c.adaptRuntimeInput(ctx, task, system, runtimeInput)
		if adaptErr != nil {
			return nil, fmt.Errorf("context adapter: %w", adaptErr)
		}
	}
	if c.eventBus != nil {
		c.executor.OnStepEvent = func(evt agentruntime.AgentStepEvent) {
			c.eventBus.Publish(eventbus.Event{
				Source:    "task-controller",
				Type:      "task.trace",
				Kind:      "Task",
				Name:      task.Metadata.Name,
				Namespace: resources.NormalizeNamespace(task.Metadata.Namespace),
				Data:      evt,
			})
		}
	}
	defer func() { c.executor.OnStepEvent = nil }()
	for idx := startIndex; idx < len(order); idx++ {
		agentName := order[idx]
		agentInputBefore := copyStringMap(runtimeInput)
		lastAgentInputBefore = copyStringMap(agentInputBefore)
		agent, ok, err := c.agentStore.Get(ctx, store.ScopedName(task.Metadata.Namespace, agentName))
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("agent %q not found", agentName)
		}
		_, effectiveModel, err := resolveAgentEffectiveModel(ctx, task.Metadata.Namespace, agent, c.modelEPStore)
		if err != nil {
			return nil, fmt.Errorf("agent %q model resolution failed: %w", agentName, err)
		}
		agent.Spec.Model = effectiveModel
		if err := agentruntime.EnforcePoliciesForAgent(agent, effectiveModel, policies); err != nil {
			return nil, err
		}

		taskKey := taskScopedName(*task)
		syncMsgID := fmt.Sprintf("sync/a%d/%s", task.Status.Attempts, agentName)
		var approvalCtx *agentruntime.GovernedToolApprovalContext
		if c.toolApprovalStore != nil {
			store := c.toolApprovalStore
			approvalCtx = &agentruntime.GovernedToolApprovalContext{
				Getter: func(key string) (resources.ToolApproval, bool, error) {
					return store.Get(ctx, key)
				},
				TaskKey:   taskKey,
				MessageID: syncMsgID,
			}
		}
		toolRuntime := agentruntime.BuildGovernedToolRuntimeForAgentWithGovernance(
			ctx,
			nil,
			c.isolatedTools,
			c.toolStore,
			c.roleStore,
			c.toolPermStore,
			task.Metadata.Namespace,
			agent,
			approvalCtx,
		)
		if c.mcpSessionMgr != nil && c.mcpServerStore != nil {
			agentruntime.ConfigureMcpRuntime(toolRuntime, c.mcpSessionMgr, c.mcpServerStore, task.Metadata.Namespace)
		}
		agentruntime.ConfigureHttpRuntime(toolRuntime, c.cliSecretResolver, task.Metadata.Namespace)
		agentruntime.ConfigureCliRuntime(toolRuntime, c.cliSecretResolver, nil, c.cliToolConfig, task.Metadata.Namespace)
		agentruntime.ConfigureExternalRuntime(toolRuntime, c.cliSecretResolver, task.Metadata.Namespace)
		agentruntime.ConfigureGRPCRuntime(toolRuntime, c.cliSecretResolver, task.Metadata.Namespace)
		agentruntime.ConfigureWebhookCallbackRuntime(toolRuntime, c.cliSecretResolver, task.Metadata.Namespace)
		agentruntime.ConfigureWasmRuntime(toolRuntime, c.wasmTools, task.Metadata.Namespace)
		if c.kubernetesTools != nil {
			agentruntime.ConfigureKubernetesRuntime(toolRuntime, c.kubernetesTools, task.Metadata.Namespace)
		}
		if c.a2aTools != nil {
			agentruntime.ConfigureA2ARuntime(toolRuntime, c.a2aTools, task.Metadata.Namespace)
		}
		var finalRT agentruntime.ToolRuntime = toolRuntime
		if agentruntime.AgentHasOrlojTools(agent) {
			finalRT = agentruntime.NewOrlojToolRuntime(toolRuntime, c.taskStore, agentruntime.OrlojToolConfig{
				ParentNamespace: task.Metadata.Namespace,
				ParentTaskName:  task.Metadata.Name,
				CurrentDepth:    orlojCurrentDepth,
				MaxDepth:        orlojMaxDepth,
				MaxChildren:     orlojMaxChildren,
			})
			agent.Spec.Tools = append(agent.Spec.Tools, agentruntime.BuiltinOrlojToolNames()...)
			agent.Spec.Tools = dedupeStringsController(agent.Spec.Tools)
		}

		var result agentruntime.AgentExecutionResult
		if c.agentK8sRuntime != nil && c.canRunAgentAsJob(ctx, agent) {
			jobResult, jobErr := c.agentK8sRuntime.ExecuteAgent(ctx, *task, agent, runtimeInput, task.Status.Attempts, syncMsgID)
			if jobErr != nil {
				c.logger.Printf("agent k8s job failed for %s (falling back to in-process): %v", agentName, jobErr)
				result, err = c.executor.ExecuteAgentWithRuntime(ctx, agent, runtimeInput, finalRT)
			} else if jobResult.Error != "" {
				result = agentruntime.AgentJobResultToExecution(jobResult, agentName)
				err = fmt.Errorf("%s", jobResult.Error)
			} else {
				result = agentruntime.AgentJobResultToExecution(jobResult, agentName)
			}
		} else {
			result, err = c.executor.ExecuteAgentWithRuntime(ctx, agent, runtimeInput, finalRT)
		}
		if err != nil {
			return nil, fmt.Errorf("agent %q execution failed: %w", agentName, err)
		}

		totalEstimatedTokens += result.EstimatedTokens
		totalUsedTokens += result.TokensUsed
		if agentBudget := agentruntime.AgentTokenBudget(policies, agentName); agentBudget > 0 && result.TokensUsed > agentBudget {
			return nil, fmt.Errorf("per-agent token budget exceeded for %q: used=%d budget=%d", agentName, result.TokensUsed, agentBudget)
		}
		if tokenBudget > 0 && totalUsedTokens > tokenBudget {
			return nil, fmt.Errorf("token budget exceeded after agent %q: used=%d budget=%d", agentName, totalUsedTokens, tokenBudget)
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
		output["tokens_used_total"] = strconv.Itoa(totalUsedTokens)
		output["tokens_estimated_total"] = strconv.Itoa(totalEstimatedTokens)
		task.Status.Output = copyStringMap(output)

		nextRuntimeInput := copyStringMap(runtimeInput)
		delete(nextRuntimeInput, "review.feedback")
		delete(nextRuntimeInput, "review.previous_output")
		delete(nextRuntimeInput, "review.checkpoint_id")
		delete(nextRuntimeInput, "review.cycle")
		delete(nextRuntimeInput, "review.requested_by")
		delete(nextRuntimeInput, "review.supersedes")
		nextRuntimeInput["previous_agent"] = result.Agent
		nextRuntimeInput["previous_agent_last_event"] = result.LastEvent

		if idx+1 < len(order) {
			nextAgent := order[idx+1]
			content := strings.TrimSpace(result.Output)
			if content == "" {
				content = strings.TrimSpace(result.LastEvent)
			}
			if content == "" {
				content = fmt.Sprintf("steps=%d tool_calls=%d tokens=%d usage_source=%s", result.Steps, result.ToolCalls, result.TokensUsed, strings.TrimSpace(result.TokenSource))
			}
			message := resources.TaskMessage{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				FromAgent: result.Agent,
				ToAgent:   nextAgent,
				Type:      "task_handoff",
				Content:   content,
			}
			c.populateTaskMessageMetadata(task, &message, idx)
			output[prefix+".message_to"] = nextAgent
			output[prefix+".message_content"] = content
			nextRuntimeInput["inbox.from"] = result.Agent
			nextRuntimeInput["inbox.to"] = nextAgent
			nextRuntimeInput["inbox.content"] = content
			nextRuntimeInput["inbox.message_id"] = message.MessageID
			nextRuntimeInput["inbox.trace_id"] = message.TraceID
			nextRuntimeInput["inbox.branch_id"] = message.BranchID
			nextRuntimeInput["inbox.parent_branch_id"] = message.ParentBranchID
			if checkpoint := reviewCheckpointForNode(system, agentName); checkpoint != nil && c.taskApprovalStore != nil {
				reviewCycle, supersedes := reviewCycleFromInput(runtimeInput)
				approvalOutput := strings.TrimSpace(result.Output)
				if approvalOutput == "" {
					approvalOutput = strings.TrimSpace(result.LastEvent)
				}
				approval, err := c.createTaskApprovalForCheckpoint(
					ctx,
					task,
					*checkpoint,
					"agent_output",
					result.Agent,
					approvalOutput,
					"text",
					resources.TaskApprovalResumeContext{
						Mode:              "sequential",
						Action:            "sequential_continue",
						System:            strings.TrimSpace(task.Spec.System),
						ProducingAgent:    result.Agent,
						RuntimeInput:      agentInputBefore,
						NextRuntimeInput:  nextRuntimeInput,
						Output:            copyStringMap(output),
						CurrentAgentIndex: idx,
						NextAgentIndex:    idx + 1,
					},
					reviewCycle,
					supersedes,
				)
				if err != nil {
					return nil, err
				}
				return nil, &taskApprovalPauseError{
					approvalName: approval.Metadata.Name,
					reason:       fmt.Sprintf("task output review pending: %s", checkpoint.CheckpointID),
				}
			}
			c.appendTaskMessage(task, message)
			if err := c.publishAgentMessage(ctx, task, message); err != nil {
				return nil, err
			}
		} else if checkpoint := reviewCheckpointForNode(system, agentName); checkpoint != nil && c.taskApprovalStore != nil {
			reviewCycle, supersedes := reviewCycleFromInput(runtimeInput)
			approvalOutput := strings.TrimSpace(result.Output)
			if approvalOutput == "" {
				approvalOutput = strings.TrimSpace(result.LastEvent)
			}
			approval, err := c.createTaskApprovalForCheckpoint(
				ctx,
				task,
				*checkpoint,
				"agent_output",
				result.Agent,
				approvalOutput,
				"text",
				resources.TaskApprovalResumeContext{
					Mode:              "sequential",
					Action:            "sequential_continue",
					System:            strings.TrimSpace(task.Spec.System),
					ProducingAgent:    result.Agent,
					RuntimeInput:      agentInputBefore,
					NextRuntimeInput:  nextRuntimeInput,
					Output:            copyStringMap(output),
					CurrentAgentIndex: idx,
					NextAgentIndex:    idx + 1,
				},
				reviewCycle,
				supersedes,
			)
			if err != nil {
				return nil, err
			}
			return nil, &taskApprovalPauseError{
				approvalName: approval.Metadata.Name,
				reason:       fmt.Sprintf("task output review pending: %s", checkpoint.CheckpointID),
			}
		}

		if idx+1 < len(order) {
			if _, uErr := c.upsertTask(*task); uErr != nil {
				c.appendTaskLog(taskScopedName(*task), fmt.Sprintf("intermediate upsert after agent %s failed: %v", agentName, uErr))
			}
		}
		runtimeInput = nextRuntimeInput
	}

	output["agents_executed"] = strconv.Itoa(len(order))
	if tokenBudget > 0 {
		remainingUsed := max(0, tokenBudget-totalUsedTokens)
		remainingEstimated := max(0, tokenBudget-totalEstimatedTokens)
		output["tokens_used_remaining"] = strconv.Itoa(remainingUsed)
		output["tokens_estimated_remaining"] = strconv.Itoa(remainingEstimated)
	}
	task.Status.Output = copyStringMap(output)
	if system.Spec.CompletionReview != nil && c.taskApprovalStore != nil {
		lastAgent := order[len(order)-1]
		completionInput := copyStringMap(runtimeInput)
		if len(lastAgentInputBefore) > 0 {
			completionInput = copyStringMap(lastAgentInputBefore)
		}
		reviewCycle, supersedes := reviewCycleFromInput(completionInput)
		approval, err := c.createTaskApprovalForCheckpoint(
			ctx,
			task,
			*system.Spec.CompletionReview,
			"task_output",
			lastAgent,
			copyStringMap(output),
			"json",
			resources.TaskApprovalResumeContext{
				Mode:              "sequential",
				Action:            "sequential_finalize",
				System:            strings.TrimSpace(task.Spec.System),
				ProducingAgent:    lastAgent,
				RuntimeInput:      copyStringMap(completionInput),
				NextRuntimeInput:  copyStringMap(completionInput),
				Output:            copyStringMap(output),
				CurrentAgentIndex: len(order) - 1,
				NextAgentIndex:    len(order),
			},
			reviewCycle,
			supersedes,
		)
		if err != nil {
			return nil, err
		}
		return nil, &taskApprovalPauseError{
			approvalName: approval.Metadata.Name,
			reason:       fmt.Sprintf("task output review pending: %s", system.Spec.CompletionReview.CheckpointID),
		}
	}
	return output, nil
}

func containsFold(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}

func isAttemptDue(task resources.Task) bool {
	if strings.TrimSpace(task.Status.NextAttemptAt) == "" {
		return true
	}
	next, err := parseControllerTimestamp(task.Status.NextAttemptAt)
	if err != nil {
		return true
	}
	return !time.Now().UTC().Before(next)
}

func shouldRetryTask(task resources.Task, reason string) bool {
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

func retryDelay(task resources.Task) (time.Duration, error) {
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

func mustEncodeTaskApprovalResumeContext(ctx resources.TaskApprovalResumeContext) map[string]any {
	out, err := resources.EncodeTaskApprovalResumeContext(ctx)
	if err != nil {
		return map[string]any{}
	}
	return out
}

func taskScopedName(task resources.Task) string {
	return store.ScopedName(task.Metadata.Namespace, task.Metadata.Name)
}

func (c *TaskController) taskMatchesWorker(task resources.Task) bool {
	taskKey := taskScopedName(task)
	if strings.EqualFold(strings.TrimSpace(task.Spec.Mode), "template") {
		c.debugf("task scheduling skipped task=%s worker=%s reason=template_mode", taskKey, c.workerID)
		return false
	}
	phase := strings.ToLower(strings.TrimSpace(task.Status.Phase))
	assigned := strings.TrimSpace(task.Status.AssignedWorker)
	// Assignment is a pending-task placement hint. Running tasks may fail over on lease expiry.
	if phase != "running" && assigned != "" && !strings.EqualFold(assigned, c.workerID) {
		c.debugf("task scheduling skipped task=%s worker=%s reason=assigned_to_other assigned_worker=%s phase=%s", taskKey, c.workerID, assigned, phase)
		return false
	}
	if c.workerStore == nil {
		c.debugf("task scheduling matched task=%s worker=%s reason=no_worker_store", taskKey, c.workerID)
		return true
	}
	worker, ok, err := c.workerStore.Get(context.Background(), c.workerID)
	if err != nil {
		c.debugf("task scheduling skipped task=%s worker=%s reason=worker_lookup_error error=%v", taskKey, c.workerID, err)
		return false
	}
	if !ok {
		// Allow scheduling when worker registration is not present (for embedded/single-process use).
		c.debugf("task scheduling matched task=%s worker=%s reason=worker_registration_missing", taskKey, c.workerID)
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(worker.Status.Phase), "ready") &&
		!strings.EqualFold(strings.TrimSpace(worker.Status.Phase), "pending") {
		c.debugf("task scheduling skipped task=%s worker=%s reason=worker_not_ready worker_phase=%s", taskKey, c.workerID, worker.Status.Phase)
		return false
	}

	req := task.Spec.Requirements
	if strings.TrimSpace(req.Region) != "" && !strings.EqualFold(strings.TrimSpace(req.Region), strings.TrimSpace(worker.Spec.Region)) {
		c.debugf("task scheduling skipped task=%s worker=%s reason=region_mismatch required_region=%s worker_region=%s", taskKey, c.workerID, req.Region, worker.Spec.Region)
		return false
	}
	if req.GPU && !worker.Spec.Capabilities.GPU {
		c.debugf("task scheduling skipped task=%s worker=%s reason=gpu_required", taskKey, c.workerID)
		return false
	}
	if strings.TrimSpace(req.Model) != "" && len(worker.Spec.Capabilities.SupportedModels) > 0 &&
		!containsFold(worker.Spec.Capabilities.SupportedModels, req.Model) {
		c.debugf("task scheduling skipped task=%s worker=%s reason=model_unsupported required_model=%s supported_models=%s", taskKey, c.workerID, req.Model, strings.Join(worker.Spec.Capabilities.SupportedModels, ","))
		return false
	}
	c.debugf("task scheduling matched task=%s worker=%s worker_region=%s worker_phase=%s", taskKey, c.workerID, worker.Spec.Region, worker.Status.Phase)
	return true
}

func (c *TaskController) workerClaimHints() store.WorkerClaimHints {
	if c.workerStore == nil {
		return store.WorkerClaimHints{}
	}
	worker, ok, err := c.workerStore.Get(context.Background(), c.workerID)
	if err != nil {
		c.debugf("worker claim hints unavailable worker=%s reason=lookup_error error=%v", c.workerID, err)
		return store.WorkerClaimHints{}
	}
	if !ok {
		c.debugf("worker claim hints unavailable worker=%s reason=worker_registration_missing", c.workerID)
		return store.WorkerClaimHints{}
	}
	c.debugf("worker claim hints worker=%s region=%s supported_models=%s", c.workerID, worker.Spec.Region, strings.Join(worker.Spec.Capabilities.SupportedModels, ","))
	return store.WorkerClaimHints{
		AssignedWorker:  c.workerID,
		Region:          strings.TrimSpace(worker.Spec.Region),
		SupportedModels: append([]string(nil), worker.Spec.Capabilities.SupportedModels...),
	}
}

func (c *TaskController) tryAcquireWorkerSlot(ctx context.Context) (bool, error) {
	worker, acquired, err := c.workerStore.TryAcquireSlot(ctx, c.workerID)
	if err != nil {
		return false, err
	}
	if !acquired && worker.Metadata.Name == "" {
		// Embedded/single-process worker can run without explicit Worker registration.
		c.debugf("worker=%s slot acquired reason=worker_registration_missing", c.workerID)
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
		c.debugf("worker=%s slot unavailable current=%d max=%d worker_phase=%s", c.workerID, worker.Status.CurrentTasks, worker.Spec.MaxConcurrentTasks, worker.Status.Phase)
		return false, nil
	}
	c.debugf("worker=%s slot acquired current=%d max=%d worker_phase=%s", c.workerID, worker.Status.CurrentTasks, worker.Spec.MaxConcurrentTasks, worker.Status.Phase)
	return true, nil
}

func (c *TaskController) appendTaskHistory(task *resources.Task, eventType string, message string) {
	if task == nil {
		return
	}
	task.Status.History = append(task.Status.History, resources.TaskHistoryEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Type:      eventType,
		Worker:    c.workerID,
		Message:   message,
	})
	if len(task.Status.History) > 200 {
		task.Status.History = task.Status.History[len(task.Status.History)-200:]
	}
}

func (c *TaskController) appendTaskMessage(task *resources.Task, message resources.TaskMessage) {
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

func (c *TaskController) populateTaskMessageMetadata(task *resources.Task, message *resources.TaskMessage, hopIndex int) {
	if task == nil || message == nil {
		return
	}
	namespace := resources.NormalizeNamespace(task.Metadata.Namespace)
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
		resources.NormalizeNamespace(namespace),
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

func (c *TaskController) publishAgentMessage(ctx context.Context, task *resources.Task, message resources.TaskMessage) error {
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

func (c *TaskController) appendRuntimeStepTrace(task *resources.Task, agentName string, events []agentruntime.AgentStepEvent) {
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

func (c *TaskController) appendTaskTrace(task *resources.Task, event resources.TaskTraceEvent) {
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

func countTraceEventsForType(trace []resources.TaskTraceEvent, eventType string, attempt int) int {
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

func hasKickoffMessage(messages []resources.TaskMessage, attempt int, agent string) bool {
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

func nextTraceStepID(trace []resources.TaskTraceEvent, attempt int) string {
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

func (c *TaskController) upsertTask(task resources.Task) (resources.Task, error) {
	ctx := context.Background()
	var lastErr error
	for i := 0; i < 5; i++ {
		updated, err := c.taskStore.Upsert(ctx, task)
		if err == nil {
			return updated, nil
		}
		if !store.IsConflict(err) {
			return resources.Task{}, err
		}
		lastErr = err
		current, ok, err := c.taskStore.Get(ctx, taskScopedName(task))
		if err != nil {
			return resources.Task{}, err
		}
		if !ok {
			return resources.Task{}, lastErr
		}
		task.Metadata.ResourceVersion = current.Metadata.ResourceVersion
		task.Metadata.Generation = current.Metadata.Generation
		task.Metadata.CreatedAt = current.Metadata.CreatedAt
		task.Spec = current.Spec
	}
	if lastErr != nil {
		return resources.Task{}, lastErr
	}
	return c.taskStore.Upsert(ctx, task)
}

func (c *TaskController) appendTaskLog(taskName, message string) {
	if strings.TrimSpace(taskName) == "" || strings.TrimSpace(message) == "" {
		return
	}
	if err := c.taskStore.AppendLog(context.Background(), taskName, message); err != nil && c.logger != nil {
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
				if err := c.taskStore.RenewLease(hbCtx, taskName, c.workerID, c.leaseDuration); err != nil {
					if c.logger != nil {
						c.logger.Printf("task=%s lease heartbeat failed: %v", taskName, err)
					}
					return
				}
				c.debugf("task=%s lease heartbeat renewed worker=%s lease=%s", taskName, c.workerID, c.leaseDuration.String())
			}
		}
	}()
	return cancel
}

func (c *TaskController) publishTaskEvent(task resources.Task, eventType string, message string) {
	if c.eventBus == nil {
		return
	}
	c.eventBus.Publish(eventbus.Event{
		Source:    "task-controller",
		Type:      strings.TrimSpace(eventType),
		Kind:      "Task",
		Name:      task.Metadata.Name,
		Namespace: resources.NormalizeNamespace(task.Metadata.Namespace),
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

func parseOrlojDepthLabel(labels map[string]string) int {
	v, ok := labels["orloj.dev/depth"]
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func dedupeStringsController(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// canRunAgentAsJob checks whether an agent's tools are compatible with K8s Job
// execution (no Docker-dependent tools).
func (c *TaskController) canRunAgentAsJob(ctx context.Context, agent resources.Agent) bool {
	var tools []resources.Tool
	mcpServers := make(map[string]resources.McpServer)
	for _, toolName := range agent.Spec.Tools {
		toolName = strings.TrimSpace(toolName)
		if toolName == "" {
			continue
		}
		tool, ok, err := c.toolStore.Get(ctx, toolName)
		if err != nil || !ok {
			continue
		}
		tools = append(tools, tool)
		if strings.ToLower(strings.TrimSpace(tool.Spec.Type)) == "mcp" {
			ref := strings.TrimSpace(tool.Spec.McpServerRef)
			if ref != "" && c.mcpServerStore != nil {
				if _, loaded := mcpServers[ref]; !loaded {
					srv, ok, err := c.mcpServerStore.Get(ctx, ref)
					if err == nil && ok {
						mcpServers[ref] = srv
					}
				}
			}
		}
	}
	return agentruntime.CanRunAsJob(agent, tools, mcpServers)
}
