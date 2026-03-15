package controllers

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/crds"
	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/store"
)

// TaskSchedulerController assigns pending tasks to eligible workers.
type TaskSchedulerController struct {
	taskStore      *store.TaskStore
	workerStore    *store.WorkerStore
	reconcileEvery time.Duration
	staleAfter     time.Duration
	logger         *log.Logger
	eventBus       eventbus.Bus
}

func NewTaskSchedulerController(
	taskStore *store.TaskStore,
	workerStore *store.WorkerStore,
	logger *log.Logger,
	reconcileEvery time.Duration,
	staleAfter time.Duration,
) *TaskSchedulerController {
	if reconcileEvery <= 0 {
		reconcileEvery = 2 * time.Second
	}
	if staleAfter <= 0 {
		staleAfter = 20 * time.Second
	}
	return &TaskSchedulerController{
		taskStore:      taskStore,
		workerStore:    workerStore,
		reconcileEvery: reconcileEvery,
		staleAfter:     staleAfter,
		logger:         logger,
	}
}

func (c *TaskSchedulerController) Start(ctx context.Context) {
	ticker := time.NewTicker(c.reconcileEvery)
	defer ticker.Stop()
	var eventCh <-chan eventbus.Event
	if c.eventBus != nil {
		eventCh = c.eventBus.Subscribe(ctx, eventbus.Filter{Source: "apiserver"})
	}

	for {
		if err := c.ReconcileOnce(); err != nil && c.logger != nil {
			c.logger.Printf("task scheduler reconcile error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case evt := <-eventCh:
			if !strings.EqualFold(evt.Kind, "Task") && !strings.EqualFold(evt.Kind, "Worker") {
				continue
			}
		}
	}
}

func (c *TaskSchedulerController) SetEventBus(bus eventbus.Bus) {
	c.eventBus = bus
}

func (c *TaskSchedulerController) ReconcileOnce() error {
	if c.taskStore == nil || c.workerStore == nil {
		return nil
	}

	workers := c.workerStore.List()
	if len(workers) == 0 {
		return nil
	}

	eligible := make(map[string]crds.Worker, len(workers))
	for _, worker := range workers {
		if c.workerEligible(worker) {
			eligible[worker.Metadata.Name] = worker
		}
	}
	if len(eligible) == 0 {
		return nil
	}

	tasks := c.taskStore.List()
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Metadata.Name < tasks[j].Metadata.Name
	})

	newAssignments := make(map[string]int, len(eligible))
	for _, task := range tasks {
		if !taskPendingForScheduling(task) {
			continue
		}

		changed, err := c.reconcileTaskAssignment(task, eligible, newAssignments)
		if err != nil {
			return err
		}
		if changed {
			continue
		}
	}

	return nil
}

func (c *TaskSchedulerController) reconcileTaskAssignment(task crds.Task, eligible map[string]crds.Worker, newAssignments map[string]int) (bool, error) {
	current := strings.TrimSpace(task.Status.AssignedWorker)
	if current != "" {
		worker, ok := eligible[current]
		if ok && taskFitsWorkerRequirements(task, worker) {
			return false, nil
		}

		task.Status.AssignedWorker = ""
		appendHistoryWithWorker(&task, "assignment_cleared", "", fmt.Sprintf("cleared assignment from worker=%s", current))
		c.publishAssignmentEvent(task, "task.assignment_cleared", fmt.Sprintf("cleared assignment from worker=%s", current))
	}

	name, ok := c.selectWorker(task, eligible, newAssignments)
	if !ok {
		if current != "" {
			_, err := c.upsertTask(task)
			return true, err
		}
		return false, nil
	}

	task.Status.AssignedWorker = name
	appendHistoryWithWorker(&task, "assigned", name, fmt.Sprintf("task assigned to worker=%s", name))
	if _, err := c.upsertTask(task); err != nil {
		return false, err
	}
	c.publishAssignmentEvent(task, "task.assigned", fmt.Sprintf("task assigned to worker=%s", name))
	newAssignments[name]++
	return true, nil
}

func (c *TaskSchedulerController) selectWorker(task crds.Task, eligible map[string]crds.Worker, newAssignments map[string]int) (string, bool) {
	names := make([]string, 0, len(eligible))
	for name := range eligible {
		names = append(names, name)
	}
	sort.Strings(names)

	selected := ""
	selectedLoad := 0
	for _, name := range names {
		worker := eligible[name]
		if !taskFitsWorkerRequirements(task, worker) {
			continue
		}
		maxConcurrent := worker.Spec.MaxConcurrentTasks
		if maxConcurrent <= 0 {
			maxConcurrent = 1
		}
		load := worker.Status.CurrentTasks + newAssignments[name]
		if load >= maxConcurrent {
			continue
		}
		if selected == "" || load < selectedLoad {
			selected = name
			selectedLoad = load
		}
	}
	if selected == "" {
		return "", false
	}
	return selected, true
}

func (c *TaskSchedulerController) workerEligible(worker crds.Worker) bool {
	phase := strings.ToLower(strings.TrimSpace(worker.Status.Phase))
	if phase != "ready" && phase != "pending" {
		return false
	}
	if strings.TrimSpace(worker.Status.LastHeartbeat) == "" {
		return false
	}
	ts, err := parseControllerTimestamp(worker.Status.LastHeartbeat)
	if err != nil {
		return false
	}
	if time.Since(ts.UTC()) > c.staleAfter {
		return false
	}
	return true
}

func taskFitsWorkerRequirements(task crds.Task, worker crds.Worker) bool {
	req := task.Spec.Requirements
	if strings.TrimSpace(req.Region) != "" && !strings.EqualFold(strings.TrimSpace(req.Region), strings.TrimSpace(worker.Spec.Region)) {
		return false
	}
	if req.GPU && !worker.Spec.Capabilities.GPU {
		return false
	}
	if strings.TrimSpace(req.Model) != "" && len(worker.Spec.Capabilities.SupportedModels) > 0 && !containsFold(worker.Spec.Capabilities.SupportedModels, req.Model) {
		return false
	}
	return true
}

func taskPendingForScheduling(task crds.Task) bool {
	if strings.EqualFold(strings.TrimSpace(task.Spec.Mode), "template") {
		return false
	}
	phase := strings.ToLower(strings.TrimSpace(task.Status.Phase))
	if phase != "" && phase != "pending" {
		return false
	}
	if strings.TrimSpace(task.Status.NextAttemptAt) == "" {
		return true
	}
	next, err := parseControllerTimestamp(task.Status.NextAttemptAt)
	if err != nil {
		return true
	}
	return !time.Now().UTC().Before(next)
}

func appendHistoryWithWorker(task *crds.Task, eventType, worker, message string) {
	if task == nil {
		return
	}
	task.Status.History = append(task.Status.History, crds.TaskHistoryEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Type:      eventType,
		Worker:    worker,
		Message:   message,
	})
	if len(task.Status.History) > 200 {
		task.Status.History = task.Status.History[len(task.Status.History)-200:]
	}
}

func (c *TaskSchedulerController) upsertTask(task crds.Task) (crds.Task, error) {
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
		current, ok := c.taskStore.Get(store.ScopedName(task.Metadata.Namespace, task.Metadata.Name))
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

func (c *TaskSchedulerController) publishAssignmentEvent(task crds.Task, eventType string, message string) {
	if c.eventBus == nil {
		return
	}
	c.eventBus.Publish(eventbus.Event{
		Source:    "task-scheduler",
		Type:      strings.TrimSpace(eventType),
		Kind:      "Task",
		Name:      task.Metadata.Name,
		Namespace: crds.NormalizeNamespace(task.Metadata.Namespace),
		Action:    "assignment",
		Message:   strings.TrimSpace(message),
		Data: map[string]any{
			"assignedWorker": task.Status.AssignedWorker,
			"phase":          task.Status.Phase,
		},
	})
}
