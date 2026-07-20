package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

var sessionTaskNameSanitizer = regexp.MustCompile(`[^a-z0-9-]+`)

// SessionController turns queued conversational turns into bounded AgentSystem
// Tasks. The Session owns durable conversation state; Tasks continue to own
// orchestration, governance, retries, and tool execution.
type SessionController struct {
	sessions       *store.SessionStore
	tasks          *store.TaskStore
	systems        *store.AgentSystemStore
	eventBus       eventbus.Bus
	logger         *log.Logger
	workerID       string
	reconcileEvery time.Duration
	leaseDuration  time.Duration
	heartbeatEvery time.Duration
}

func NewSessionController(
	sessions *store.SessionStore,
	tasks *store.TaskStore,
	systems *store.AgentSystemStore,
	logger *log.Logger,
	reconcileEvery time.Duration,
) *SessionController {
	if reconcileEvery <= 0 {
		reconcileEvery = time.Second
	}
	return &SessionController{
		sessions:       sessions,
		tasks:          tasks,
		systems:        systems,
		logger:         logger,
		workerID:       defaultWorkerID() + "-sessions",
		reconcileEvery: reconcileEvery,
		leaseDuration:  30 * time.Second,
		heartbeatEvery: 10 * time.Second,
	}
}

func (c *SessionController) ConfigureWorker(workerID string, leaseDuration, heartbeatEvery time.Duration) {
	if value := strings.TrimSpace(workerID); value != "" {
		c.workerID = value + "-sessions"
	}
	if leaseDuration > 0 {
		c.leaseDuration = leaseDuration
	}
	if heartbeatEvery > 0 {
		c.heartbeatEvery = heartbeatEvery
	}
}

func (c *SessionController) SetEventBus(bus eventbus.Bus) {
	c.eventBus = bus
}

func (c *SessionController) Start(ctx context.Context) {
	ticker := time.NewTicker(c.reconcileEvery)
	defer ticker.Stop()
	for {
		if err := c.ReconcileOnce(ctx); err != nil && !errors.Is(err, context.Canceled) && c.logger != nil {
			c.logger.Printf("session controller reconcile error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *SessionController) ReconcileOnce(ctx context.Context) error {
	if c.sessions == nil || c.tasks == nil || c.systems == nil {
		return nil
	}
	expired, err := c.sessions.ExpireIdleSessions(ctx)
	if err != nil {
		return err
	}
	c.publishSessionEvents(expired)
	if err := c.pruneCheckpoints(ctx); err != nil {
		return err
	}
	claim, claimed, events, err := c.sessions.ClaimNextTurn(ctx, c.workerID, c.leaseDuration)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	c.publishSessionEvents(events)
	return c.executeTurn(ctx, claim)
}

func (c *SessionController) pruneCheckpoints(ctx context.Context) error {
	sessions, err := c.sessions.List(ctx)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		key := store.ScopedName(session.Metadata.Namespace, session.Metadata.Name)
		pruned, err := c.sessions.PruneCheckpoints(ctx, key)
		if err != nil {
			return err
		}
		if len(pruned) == 0 {
			continue
		}
		events, err := c.sessions.ListEvents(ctx, key, session.Status.LastEventSequence, 10)
		if err != nil {
			return err
		}
		c.publishSessionEvents(events)
	}
	return nil
}

func (c *SessionController) executeTurn(ctx context.Context, claim store.SessionClaim) error {
	systemKey := store.ScopedName(claim.Session.Metadata.Namespace, claim.Session.Spec.System)
	system, ok, err := c.systems.Get(ctx, systemKey)
	if err != nil {
		return c.failClaim(ctx, claim, fmt.Errorf("load AgentSystem: %w", err))
	}
	if !ok {
		return c.failClaim(ctx, claim, fmt.Errorf("AgentSystem %q not found", claim.Session.Spec.System))
	}
	if claim.Checkpoint != nil &&
		claim.Checkpoint.SystemGeneration > 0 &&
		system.Metadata.Generation != claim.Checkpoint.SystemGeneration {
		events, _, pauseErr := c.sessions.ApplyControl(
			ctx,
			store.ScopedName(claim.Session.Metadata.Namespace, claim.Session.Metadata.Name),
			"pause",
			fmt.Sprintf(
				"checkpoint generation %d is incompatible with AgentSystem generation %d",
				claim.Checkpoint.SystemGeneration,
				system.Metadata.Generation,
			),
		)
		if pauseErr != nil {
			return pauseErr
		}
		c.publishSessionEvents(events)
		return nil
	}

	taskName := sessionTurnTaskName(claim.Session.Metadata.Name, claim.Turn.ID)
	taskKey := store.ScopedName(claim.Session.Metadata.Namespace, taskName)
	task, exists, err := c.tasks.Get(ctx, taskKey)
	if err != nil {
		return c.failClaim(ctx, claim, fmt.Errorf("load turn task: %w", err))
	}
	if !exists {
		transcript, transcriptErr := c.sessionTranscript(ctx, claim.Session, claim.Turn.ID)
		if transcriptErr != nil {
			return c.failClaim(ctx, claim, transcriptErr)
		}
		streamAgent := ""
		if order := resources.ExecutionAgentOrder(system); len(order) > 0 {
			streamAgent = order[len(order)-1]
		}
		task = resources.Task{
			APIVersion: "orloj.dev/v1",
			Kind:       "Task",
			Metadata: resources.ObjectMeta{
				Name:      taskName,
				Namespace: resources.NormalizeNamespace(claim.Session.Metadata.Namespace),
				Labels: map[string]string{
					"orloj.dev/created-by": "session",
					"orloj.dev/session":    claim.Session.Metadata.Name,
					"orloj.dev/turn":       claim.Turn.ID,
				},
				Annotations: map[string]string{
					"orloj.dev/session-turn":   claim.Turn.ID,
					"orloj.dev/session-worker": claim.Turn.ClaimedBy,
					"orloj.dev/session-fence":  strconv.FormatInt(claim.Turn.Fence, 10),
				},
			},
			Spec: resources.TaskSpec{
				System: claim.Session.Spec.System,
				Mode:   "run",
				Input: mergeSessionTaskInput(claim.Session.Spec.Input, map[string]string{
					"topic":                     claim.Turn.Content,
					"session.id":                claim.Session.Metadata.Name,
					"session.stream_agent":      streamAgent,
					"session.turn_id":           claim.Turn.ID,
					"session.system_generation": strconv.FormatInt(system.Metadata.Generation, 10),
					"session.transcript":        transcript,
				}),
			},
			Status: resources.TaskStatus{Phase: "Pending"},
		}
		task, err = c.tasks.Upsert(ctx, task)
		if err != nil {
			return c.failClaim(ctx, claim, fmt.Errorf("create turn task: %w", err))
		}
		c.publishTaskCreated(task)
	} else {
		if task.Metadata.Annotations == nil {
			task.Metadata.Annotations = map[string]string{}
		}
		expectedFence := strconv.FormatInt(claim.Turn.Fence, 10)
		if task.Metadata.Annotations["orloj.dev/session-worker"] != claim.Turn.ClaimedBy ||
			task.Metadata.Annotations["orloj.dev/session-fence"] != expectedFence {
			if err := c.tasks.Delete(ctx, taskKey); err != nil {
				return c.failClaim(ctx, claim, fmt.Errorf("replace stale turn task: %w", err))
			}
			return c.executeTurn(ctx, claim)
		}
	}

	turnCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	heartbeatDone := make(chan struct{})
	go c.heartbeat(turnCtx, claim, heartbeatDone)
	defer func() {
		cancel()
		<-heartbeatDone
	}()

	mapped := c.mappedTaskTraceCount(turnCtx, claim.Session, taskName)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		currentSession, sessionExists, sessionErr := c.sessions.Get(turnCtx, store.ScopedName(claim.Session.Metadata.Namespace, claim.Session.Metadata.Name))
		if sessionErr != nil {
			return sessionErr
		}
		if !sessionExists ||
			currentSession.Status.Fence != claim.Turn.Fence ||
			!strings.EqualFold(currentSession.Status.ClaimedBy, claim.Turn.ClaimedBy) {
			return nil
		}

		task, exists, err = c.tasks.Get(turnCtx, taskKey)
		if err != nil {
			return c.failClaim(turnCtx, claim, fmt.Errorf("load turn task: %w", err))
		}
		if !exists {
			return c.failClaim(turnCtx, claim, fmt.Errorf("turn task %q disappeared", taskName))
		}
		mapped = c.mapTaskTrace(turnCtx, claim, task, mapped)

		taskPhase := strings.ToLower(strings.TrimSpace(task.Status.Phase))
		if taskPhase == "waitingapproval" {
			if !strings.EqualFold(currentSession.Status.Phase, resources.SessionPhaseWaitingApproval) {
				events, _, approvalErr := c.sessions.SetApprovalState(turnCtx, claim, true, task.Status.BlockedOn)
				if approvalErr != nil {
					return c.failClaim(turnCtx, claim, fmt.Errorf("mark session waiting for approval: %w", approvalErr))
				}
				c.publishSessionEvents(events)
			}
		} else if strings.EqualFold(currentSession.Status.Phase, resources.SessionPhaseWaitingApproval) {
			events, _, approvalErr := c.sessions.SetApprovalState(turnCtx, claim, false, nil)
			if approvalErr != nil {
				return c.failClaim(turnCtx, claim, fmt.Errorf("resume session after approval: %w", approvalErr))
			}
			c.publishSessionEvents(events)
		}

		switch taskPhase {
		case "succeeded":
			output := sessionTaskOutput(task.Status.Output)
			if output == "" {
				return c.failClaim(turnCtx, claim, fmt.Errorf("turn task %q produced no assistant output", taskName))
			}
			if checkpointErr := c.checkpointCompletedTurn(turnCtx, claim, task); checkpointErr != nil {
				return c.failClaim(turnCtx, claim, checkpointErr)
			}
			events, _, err := c.sessions.CompleteTurn(turnCtx, claim, output, sessionTaskUsage(task))
			if err != nil {
				return err
			}
			c.publishSessionEvents(events)
			return nil
		case "failed", "deadletter":
			message := strings.TrimSpace(task.Status.LastError)
			if message == "" {
				message = fmt.Sprintf("turn task ended in phase %s", task.Status.Phase)
			}
			return c.failClaim(turnCtx, claim, fmt.Errorf("%s", message))
		}

		select {
		case <-turnCtx.Done():
			return turnCtx.Err()
		case <-ticker.C:
		}
	}
}

func (c *SessionController) checkpointCompletedTurn(
	ctx context.Context,
	claim store.SessionClaim,
	task resources.Task,
) error {
	state, err := json.Marshal(map[string]any{
		"version":   resources.SessionCheckpointStateVersion,
		"completed": true,
		"output":    task.Status.Output,
		"trace":     task.Status.Trace,
	})
	if err != nil {
		return err
	}
	systemGeneration, _ := strconv.ParseInt(
		strings.TrimSpace(task.Spec.Input["session.system_generation"]),
		10,
		64,
	)
	_, event, err := c.sessions.CreateCheckpoint(
		ctx,
		store.ScopedName(claim.Session.Metadata.Namespace, claim.Session.Metadata.Name),
		claim.Turn.ID,
		claim.Turn.ClaimedBy,
		claim.Turn.Fence,
		resources.SessionCheckpoint{
			TurnID:           claim.Turn.ID,
			TaskName:         task.Metadata.Name,
			Attempt:          claim.Turn.Attempt,
			SafePoint:        resources.SessionCheckpointSafePointTurn,
			StateVersion:     resources.SessionCheckpointStateVersion,
			SystemGeneration: systemGeneration,
			State:            state,
		},
	)
	if err != nil {
		return fmt.Errorf("checkpoint completed turn: %w", err)
	}
	c.publishSessionEvents([]resources.SessionEvent{event})
	return nil
}

func (c *SessionController) heartbeat(ctx context.Context, claim store.SessionClaim, done chan<- struct{}) {
	defer close(done)
	interval := c.heartbeatEvery
	if interval <= 0 || interval >= c.leaseDuration {
		interval = c.leaseDuration / 3
	}
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.sessions.RenewLease(
				ctx,
				store.ScopedName(claim.Session.Metadata.Namespace, claim.Session.Metadata.Name),
				claim.Turn.ID,
				claim.Turn.ClaimedBy,
				claim.Turn.Fence,
				c.leaseDuration,
			); err != nil {
				return
			}
		}
	}
}

func (c *SessionController) failClaim(ctx context.Context, claim store.SessionClaim, cause error) error {
	events, _, err := c.sessions.FailTurn(ctx, claim, cause)
	if err != nil {
		// A pause/cancel changes the fence; the superseded execution should stop
		// quietly instead of overwriting the newer Session state.
		if strings.Contains(strings.ToLower(err.Error()), "fence changed") ||
			strings.Contains(strings.ToLower(err.Error()), "not running") {
			return nil
		}
		return err
	}
	c.publishSessionEvents(events)
	return cause
}

func (c *SessionController) sessionTranscript(ctx context.Context, session resources.Session, currentTurnID string) (string, error) {
	events, err := c.sessions.ListEvents(ctx, store.ScopedName(session.Metadata.Namespace, session.Metadata.Name), 0, 1000)
	if err != nil {
		return "", fmt.Errorf("load session transcript: %w", err)
	}
	var rewindEventSequence uint64
	var rewindCheckpointSequence uint64
	for _, evt := range events {
		if evt.Type != resources.SessionEventSessionRewound {
			continue
		}
		rewindEventSequence = evt.Sequence
		switch value := evt.Payload["checkpoint_sequence"].(type) {
		case float64:
			rewindCheckpointSequence = uint64(value)
		case uint64:
			rewindCheckpointSequence = value
		case int:
			if value > 0 {
				rewindCheckpointSequence = uint64(value)
			}
		}
	}
	lines := make([]string, 0)
	for _, evt := range events {
		if rewindEventSequence > 0 &&
			evt.Sequence > rewindCheckpointSequence &&
			evt.Sequence < rewindEventSequence {
			continue
		}
		if evt.TurnID == currentTurnID {
			continue
		}
		if evt.Type != resources.SessionEventMessageCreated && evt.Type != resources.SessionEventMessageCompleted {
			continue
		}
		role, _ := evt.Payload["role"].(string)
		content, _ := evt.Payload["content"].(string)
		role = strings.TrimSpace(role)
		content = strings.TrimSpace(content)
		if role == "" || content == "" {
			continue
		}
		lines = append(lines, role+": "+content)
	}
	return strings.Join(lines, "\n"), nil
}

func (c *SessionController) mappedTaskTraceCount(ctx context.Context, session resources.Session, taskName string) int {
	events, err := c.sessions.ListEvents(ctx, store.ScopedName(session.Metadata.Namespace, session.Metadata.Name), 0, 1000)
	if err != nil {
		return 0
	}
	maxIndex := -1
	for _, evt := range events {
		task, _ := evt.Payload["task"].(string)
		if task != taskName {
			continue
		}
		switch value := evt.Payload["task_trace_index"].(type) {
		case float64:
			if int(value) > maxIndex {
				maxIndex = int(value)
			}
		case int:
			if value > maxIndex {
				maxIndex = value
			}
		}
	}
	return maxIndex + 1
}

func (c *SessionController) mapTaskTrace(ctx context.Context, claim store.SessionClaim, task resources.Task, start int) int {
	if start < 0 {
		start = 0
	}
	for index := start; index < len(task.Status.Trace); index++ {
		trace := task.Status.Trace[index]
		eventType := ""
		payload := map[string]any{
			"task":             task.Metadata.Name,
			"task_trace_index": index,
			"agent":            trace.Agent,
			"tool":             trace.Tool,
			"message":          trace.Message,
			"step":             trace.Step,
		}
		switch strings.ToLower(strings.TrimSpace(trace.Type)) {
		case "tool_call":
			eventType = resources.SessionEventToolCompleted
		default:
			continue
		}
		evt, err := c.sessions.AppendEvent(
			ctx,
			store.ScopedName(claim.Session.Metadata.Namespace, claim.Session.Metadata.Name),
			claim.Turn.ID,
			claim.Turn.ClaimedBy,
			claim.Turn.Fence,
			resources.SessionEvent{
				Type:      eventType,
				TurnID:    claim.Turn.ID,
				MessageID: claim.Turn.AssistantMessageID,
				Attempt:   claim.Turn.Attempt,
				Payload:   payload,
			},
		)
		if err != nil {
			return index
		}
		c.publishSessionEvents([]resources.SessionEvent{evt})
	}
	return len(task.Status.Trace)
}

func (c *SessionController) publishSessionEvents(events []resources.SessionEvent) {
	if c.eventBus == nil {
		return
	}
	for _, evt := range events {
		c.eventBus.Publish(eventbus.Event{
			Source:    "session-controller",
			Type:      evt.Type,
			Kind:      "Session",
			Name:      evt.SessionName,
			Namespace: evt.Namespace,
			Action:    evt.Type,
			Data:      evt,
		})
	}
}

func (c *SessionController) publishTaskCreated(task resources.Task) {
	if c.eventBus == nil {
		return
	}
	c.eventBus.Publish(eventbus.Event{
		Source:    "apiserver",
		Type:      "resource.created",
		Kind:      "Task",
		Name:      task.Metadata.Name,
		Namespace: resources.NormalizeNamespace(task.Metadata.Namespace),
		Action:    "created",
		Data:      task,
	})
}

func sessionTurnTaskName(sessionName, turnID string) string {
	base := strings.ToLower(strings.TrimSpace(sessionName))
	base = sessionTaskNameSanitizer.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if len(base) > 30 {
		base = strings.TrimRight(base[:30], "-")
	}
	id := strings.ReplaceAll(strings.TrimSpace(turnID), "-", "")
	if len(id) > 16 {
		id = id[:16]
	}
	return fmt.Sprintf("session-%s-%s", base, id)
}

func mergeSessionTaskInput(base, additions map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(additions))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range additions {
		out[key] = value
	}
	return out
}

func sessionTaskOutput(output map[string]string) string {
	for _, key := range []string{"last_output", "response", "result"} {
		value := strings.TrimSpace(output[key])
		if value == "" || (key == "result" && value == "executed") {
			continue
		}
		if strings.HasPrefix(value, "step=") {
			if index := strings.Index(value, " model_output="); index >= 0 {
				value = value[index+len(" model_output="):]
			}
		}
		return resources.UnwrapFencedCodeBlock(strings.TrimSpace(value))
	}
	bestIndex := -1
	best := ""
	for key, value := range output {
		if !strings.HasPrefix(key, "agent.") || !strings.HasSuffix(key, ".message_content") {
			continue
		}
		middle := strings.TrimSuffix(strings.TrimPrefix(key, "agent."), ".message_content")
		index, err := strconv.Atoi(middle)
		if err == nil && index >= bestIndex && strings.TrimSpace(value) != "" {
			bestIndex = index
			best = strings.TrimSpace(value)
		}
	}
	return best
}

func sessionTaskUsage(task resources.Task) map[string]any {
	input := 0
	output := 0
	total := 0
	for _, evt := range task.Status.Trace {
		input += evt.InputTokens
		output += evt.OutputTokens
		total += evt.Tokens
	}
	if total == 0 {
		total = input + output
	}
	return map[string]any{
		"input_tokens":  input,
		"output_tokens": output,
		"total_tokens":  total,
	}
}
