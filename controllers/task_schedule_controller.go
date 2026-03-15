package controllers

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/crds"
	"github.com/OrlojHQ/orloj/cronexpr"
	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/store"
)

const (
	taskScheduleNameLabel      = "orloj.dev/task-schedule"
	taskScheduleNamespaceLabel = "orloj.dev/task-schedule-namespace"
	taskScheduleSlotLabel      = "orloj.dev/task-schedule-slot"
)

// TaskScheduleController reconciles recurring task schedules into run tasks.
type TaskScheduleController struct {
	taskScheduleStore *store.TaskScheduleStore
	taskStore         *store.TaskStore
	reconcileEvery    time.Duration
	logger            *log.Logger
	eventBus          eventbus.Bus
}

func NewTaskScheduleController(
	taskScheduleStore *store.TaskScheduleStore,
	taskStore *store.TaskStore,
	logger *log.Logger,
	reconcileEvery time.Duration,
) *TaskScheduleController {
	if reconcileEvery <= 0 {
		reconcileEvery = 2 * time.Second
	}
	return &TaskScheduleController{
		taskScheduleStore: taskScheduleStore,
		taskStore:         taskStore,
		reconcileEvery:    reconcileEvery,
		logger:            logger,
	}
}

func (c *TaskScheduleController) SetEventBus(bus eventbus.Bus) {
	c.eventBus = bus
}

func (c *TaskScheduleController) Start(ctx context.Context) {
	ticker := time.NewTicker(c.reconcileEvery)
	defer ticker.Stop()

	var eventCh <-chan eventbus.Event
	if c.eventBus != nil {
		eventCh = c.eventBus.Subscribe(ctx, eventbus.Filter{Source: "apiserver"})
	}

	for {
		if err := c.ReconcileOnce(); err != nil && c.logger != nil {
			c.logger.Printf("task schedule controller reconcile error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case evt := <-eventCh:
			kind := strings.TrimSpace(evt.Kind)
			if !strings.EqualFold(kind, "TaskSchedule") && !strings.EqualFold(kind, "Task") {
				continue
			}
		}
	}
}

func (c *TaskScheduleController) ReconcileOnce() error {
	if c.taskScheduleStore == nil || c.taskStore == nil {
		return nil
	}
	items := c.taskScheduleStore.List()
	sort.Slice(items, func(i, j int) bool {
		left := store.ScopedName(items[i].Metadata.Namespace, items[i].Metadata.Name)
		right := store.ScopedName(items[j].Metadata.Namespace, items[j].Metadata.Name)
		return left < right
	})

	for _, item := range items {
		if err := c.reconcileSchedule(item); err != nil {
			return err
		}
	}
	return nil
}

func (c *TaskScheduleController) reconcileSchedule(item crds.TaskSchedule) error {
	now := time.Now().UTC()
	expr, loc, err := parseScheduleSpec(item)
	if err != nil {
		item.Status.LastError = err.Error()
		item.Status.Phase = "Error"
		item.Status.ObservedGeneration = item.Metadata.Generation
		return c.upsertTaskSchedule(item)
	}
	item.Status.LastError = ""

	if item.Spec.Suspend {
		item.Status.Phase = "Suspended"
		item.Status.LastError = ""
		next, ok := expr.Next(now.In(loc))
		if ok {
			item.Status.NextScheduleTime = next.UTC().Format(time.RFC3339Nano)
		} else {
			item.Status.NextScheduleTime = ""
		}
		item.Status.ObservedGeneration = item.Metadata.Generation
		return c.refreshScheduleStatus(item)
	}

	generated := c.generatedTasks(item)
	activeRuns := listActiveRuns(generated)

	latestSlot, hasDue, withinDeadline, dueErr := evaluateDueSlot(item, expr, loc, now)
	if dueErr != nil {
		item.Status.LastError = dueErr.Error()
		item.Status.Phase = "Error"
		item.Status.ObservedGeneration = item.Metadata.Generation
		return c.upsertTaskSchedule(item)
	}

	if hasDue {
		item.Status.LastScheduleTime = latestSlot.Format(time.RFC3339Nano)
		switch {
		case !withinDeadline:
			c.publishScheduleEvent(item, "taskschedule.missed_deadline", "missed run window; skipped", map[string]any{
				"slot": latestSlot.Format(time.RFC3339Nano),
			})
		case strings.EqualFold(item.Spec.ConcurrencyPolicy, "forbid") && len(activeRuns) > 0:
			c.publishScheduleEvent(item, "taskschedule.skipped_forbid", "active run present; skipped due to concurrency policy", map[string]any{
				"slot":      latestSlot.Format(time.RFC3339Nano),
				"activeRun": activeRuns[0],
			})
		default:
			runScopedName, runErr := c.ensureRunTask(item, latestSlot)
			if runErr != nil {
				item.Status.LastError = runErr.Error()
				item.Status.Phase = "Error"
			} else {
				item.Status.LastError = ""
				item.Status.LastTriggeredTask = runScopedName
				c.publishScheduleEvent(item, "taskschedule.triggered", "scheduled run task created", map[string]any{
					"slot": latestSlot.Format(time.RFC3339Nano),
					"task": runScopedName,
				})
			}
		}
	}

	deleted, cleanupErr := c.cleanupHistory(item)
	if cleanupErr != nil {
		item.Status.LastError = cleanupErr.Error()
		item.Status.Phase = "Error"
	} else if deleted > 0 {
		c.publishScheduleEvent(item, "taskschedule.retention_pruned", "retention policy pruned historical runs", map[string]any{
			"deleted": deleted,
		})
	}

	next, ok := expr.Next(now.In(loc))
	if ok {
		item.Status.NextScheduleTime = next.UTC().Format(time.RFC3339Nano)
	} else {
		item.Status.NextScheduleTime = ""
	}

	return c.refreshScheduleStatus(item)
}

func parseScheduleSpec(item crds.TaskSchedule) (cronexpr.Expression, *time.Location, error) {
	expr, err := cronexpr.Parse(item.Spec.Schedule)
	if err != nil {
		return cronexpr.Expression{}, nil, fmt.Errorf("invalid schedule: %w", err)
	}
	loc, err := time.LoadLocation(item.Spec.TimeZone)
	if err != nil {
		return cronexpr.Expression{}, nil, fmt.Errorf("invalid time zone: %w", err)
	}
	return expr, loc, nil
}

func evaluateDueSlot(item crds.TaskSchedule, expr cronexpr.Expression, loc *time.Location, now time.Time) (time.Time, bool, bool, error) {
	latestLocal, ok := expr.Prev(now.In(loc))
	if !ok {
		return time.Time{}, false, false, nil
	}
	latestUTC := latestLocal.UTC()
	if strings.TrimSpace(item.Status.LastScheduleTime) != "" {
		last, err := parseControllerTimestamp(item.Status.LastScheduleTime)
		if err != nil {
			return time.Time{}, false, false, fmt.Errorf("invalid status.lastScheduleTime %q: %w", item.Status.LastScheduleTime, err)
		}
		if !latestUTC.After(last.UTC()) {
			return time.Time{}, false, false, nil
		}
	}

	deadline := time.Duration(item.Spec.StartingDeadlineSeconds) * time.Second
	if deadline <= 0 {
		deadline = 300 * time.Second
	}
	withinDeadline := now.Sub(latestUTC) <= deadline
	return latestUTC, true, withinDeadline, nil
}

func (c *TaskScheduleController) ensureRunTask(item crds.TaskSchedule, slot time.Time) (string, error) {
	templateNS, templateName, err := resolveTaskRef(item.Metadata.Namespace, item.Spec.TaskRef)
	if err != nil {
		return "", err
	}
	templateKey := store.ScopedName(templateNS, templateName)
	template, ok := c.taskStore.Get(templateKey)
	if !ok {
		return "", fmt.Errorf("task template %q not found", item.Spec.TaskRef)
	}
	if !strings.EqualFold(strings.TrimSpace(template.Spec.Mode), "template") {
		return "", fmt.Errorf("task template %q must set spec.mode=template", item.Spec.TaskRef)
	}

	runName := scheduledTaskName(item.Metadata.Name, slot)
	runNamespace := template.Metadata.Namespace
	runKey := store.ScopedName(runNamespace, runName)

	if existing, ok := c.taskStore.Get(runKey); ok {
		labels := existing.Metadata.Labels
		if labels != nil &&
			strings.EqualFold(strings.TrimSpace(labels[taskScheduleNameLabel]), strings.TrimSpace(item.Metadata.Name)) &&
			strings.EqualFold(strings.TrimSpace(labels[taskScheduleNamespaceLabel]), strings.TrimSpace(item.Metadata.Namespace)) &&
			strings.TrimSpace(labels[taskScheduleSlotLabel]) == slot.UTC().Format(time.RFC3339Nano) {
			return runKey, nil
		}
		return "", fmt.Errorf("scheduled run task name conflict for %q", runKey)
	}

	labels := copyStringMap(template.Metadata.Labels)
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[taskScheduleNameLabel] = item.Metadata.Name
	labels[taskScheduleNamespaceLabel] = crds.NormalizeNamespace(item.Metadata.Namespace)
	labels[taskScheduleSlotLabel] = slot.UTC().Format(time.RFC3339Nano)

	spec := cloneTaskSpec(template.Spec)
	spec.Mode = "run"
	runTask := crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: crds.ObjectMeta{
			Name:      runName,
			Namespace: runNamespace,
			Labels:    labels,
		},
		Spec: spec,
	}
	if _, err := c.taskStore.Upsert(runTask); err != nil {
		return "", err
	}
	return runKey, nil
}

func (c *TaskScheduleController) cleanupHistory(item crds.TaskSchedule) (int, error) {
	generated := c.generatedTasks(item)
	successes := make([]crds.Task, 0, len(generated))
	failures := make([]crds.Task, 0, len(generated))
	for _, task := range generated {
		switch strings.ToLower(strings.TrimSpace(task.Status.Phase)) {
		case "succeeded":
			successes = append(successes, task)
		case "failed", "deadletter":
			failures = append(failures, task)
		}
	}
	sort.Slice(successes, func(i, j int) bool {
		return taskTerminalTime(successes[i]).After(taskTerminalTime(successes[j]))
	})
	sort.Slice(failures, func(i, j int) bool {
		return taskTerminalTime(failures[i]).After(taskTerminalTime(failures[j]))
	})

	deleted := 0
	for i := item.Spec.SuccessfulHistoryLimit; i < len(successes); i++ {
		if err := c.taskStore.Delete(store.ScopedName(successes[i].Metadata.Namespace, successes[i].Metadata.Name)); err != nil {
			return deleted, err
		}
		deleted++
	}
	for i := item.Spec.FailedHistoryLimit; i < len(failures); i++ {
		if err := c.taskStore.Delete(store.ScopedName(failures[i].Metadata.Namespace, failures[i].Metadata.Name)); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func (c *TaskScheduleController) generatedTasks(item crds.TaskSchedule) []crds.Task {
	all := c.taskStore.List()
	out := make([]crds.Task, 0, len(all))
	for _, task := range all {
		labels := task.Metadata.Labels
		if labels == nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(labels[taskScheduleNameLabel]), strings.TrimSpace(item.Metadata.Name)) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(labels[taskScheduleNamespaceLabel]), crds.NormalizeNamespace(item.Metadata.Namespace)) {
			continue
		}
		out = append(out, task)
	}
	return out
}

func listActiveRuns(tasks []crds.Task) []string {
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		switch strings.ToLower(strings.TrimSpace(task.Status.Phase)) {
		case "", "pending", "running":
			out = append(out, store.ScopedName(task.Metadata.Namespace, task.Metadata.Name))
		}
	}
	sort.Strings(out)
	return out
}

func taskTerminalTime(task crds.Task) time.Time {
	candidates := []string{
		task.Status.CompletedAt,
		task.Status.StartedAt,
		task.Metadata.CreatedAt,
	}
	for _, value := range candidates {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if ts, err := parseControllerTimestamp(value); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}

func (c *TaskScheduleController) refreshScheduleStatus(item crds.TaskSchedule) error {
	generated := c.generatedTasks(item)
	item.Status.ActiveRuns = listActiveRuns(generated)

	lastSuccess := time.Time{}
	for _, task := range generated {
		if !strings.EqualFold(strings.TrimSpace(task.Status.Phase), "succeeded") {
			continue
		}
		when := taskTerminalTime(task)
		if when.After(lastSuccess) {
			lastSuccess = when
		}
	}
	if lastSuccess.IsZero() {
		item.Status.LastSuccessfulTime = ""
	} else {
		item.Status.LastSuccessfulTime = lastSuccess.Format(time.RFC3339Nano)
	}

	if strings.TrimSpace(item.Status.LastError) == "" {
		item.Status.Phase = "Ready"
	}
	item.Status.ObservedGeneration = item.Metadata.Generation
	return c.upsertTaskSchedule(item)
}

func (c *TaskScheduleController) upsertTaskSchedule(item crds.TaskSchedule) error {
	var lastErr error
	for i := 0; i < 5; i++ {
		if _, err := c.taskScheduleStore.Upsert(item); err == nil {
			return nil
		} else if !store.IsConflict(err) {
			return err
		} else {
			lastErr = err
		}

		current, ok := c.taskScheduleStore.Get(store.ScopedName(item.Metadata.Namespace, item.Metadata.Name))
		if !ok {
			return lastErr
		}
		item.Metadata.ResourceVersion = current.Metadata.ResourceVersion
		item.Metadata.Generation = current.Metadata.Generation
		item.Metadata.CreatedAt = current.Metadata.CreatedAt
		item.Metadata.Labels = copyStringMap(current.Metadata.Labels)
		item.Spec = current.Spec
	}
	if lastErr != nil {
		return lastErr
	}
	_, err := c.taskScheduleStore.Upsert(item)
	return err
}

func (c *TaskScheduleController) publishScheduleEvent(item crds.TaskSchedule, eventType string, message string, data map[string]any) {
	if c.eventBus == nil {
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	data["phase"] = item.Status.Phase
	data["lastError"] = item.Status.LastError
	c.eventBus.Publish(eventbus.Event{
		Source:    "task-schedule-controller",
		Type:      strings.TrimSpace(eventType),
		Kind:      "TaskSchedule",
		Name:      item.Metadata.Name,
		Namespace: crds.NormalizeNamespace(item.Metadata.Namespace),
		Action:    strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		Message:   strings.TrimSpace(message),
		Data:      data,
	})
}

func resolveTaskRef(scheduleNamespace, taskRef string) (string, string, error) {
	ref := strings.TrimSpace(taskRef)
	if ref == "" {
		return "", "", fmt.Errorf("spec.task_ref is required")
	}
	if !strings.Contains(ref, "/") {
		return crds.NormalizeNamespace(scheduleNamespace), ref, nil
	}
	parts := strings.SplitN(ref, "/", 2)
	ns := crds.NormalizeNamespace(parts[0])
	name := strings.TrimSpace(parts[1])
	if name == "" {
		return "", "", fmt.Errorf("invalid spec.task_ref %q", taskRef)
	}
	return ns, name, nil
}

func scheduledTaskName(scheduleName string, slot time.Time) string {
	base := sanitizeName(strings.ToLower(strings.TrimSpace(scheduleName)))
	if base == "" {
		base = "schedule"
	}
	if len(base) > 40 {
		base = base[:40]
		base = strings.Trim(base, "-")
	}
	return fmt.Sprintf("%s-%s", base, slot.UTC().Format("20060102-1504"))
}

func sanitizeName(value string) string {
	if value == "" {
		return ""
	}
	builder := strings.Builder{}
	builder.Grow(len(value))
	lastDash := false
	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func cloneTaskSpec(spec crds.TaskSpec) crds.TaskSpec {
	out := spec
	out.Input = copyStringMap(spec.Input)
	out.MessageRetry.NonRetryable = append([]string(nil), spec.MessageRetry.NonRetryable...)
	return out
}
