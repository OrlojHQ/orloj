package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"iter"
	"net/http"
	"sort"
	"strings"
	"time"

	lf "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/resources"
	orloja2a "github.com/OrlojHQ/orloj/runtime/a2a"
	"github.com/OrlojHQ/orloj/store"
)

type a2aV1CallContextKey struct{}

type a2aV1CallContext struct {
	request    *http.Request
	systemName string
}

type a2aV1Handler struct {
	server *Server
}

var _ a2asrv.RequestHandler = (*a2aV1Handler)(nil)

func (s *Server) handleA2AV1JSONRPC(w http.ResponseWriter, r *http.Request, systemName string) {
	call := a2aV1CallContext{request: r, systemName: strings.TrimSpace(systemName)}
	ctx := context.WithValue(r.Context(), a2aV1CallContextKey{}, call)
	a2asrv.NewJSONRPCHandler(&a2aV1Handler{server: s}, a2asrv.WithTransportKeepAlive(15*time.Second)).
		ServeHTTP(w, r.WithContext(ctx))
}

func (h *a2aV1Handler) GetTask(ctx context.Context, req *lf.GetTaskRequest) (*lf.Task, error) {
	call, err := v1CallFromContext(ctx)
	if err != nil || req == nil || req.ID == "" {
		return nil, lf.ErrInvalidParams
	}
	task, err := h.server.findTaskByA2AID(call.request, string(req.ID), call.systemName)
	if err != nil {
		return nil, lf.ErrTaskNotFound
	}
	result := orloja2a.OrlojTaskToV1(task)
	applyV1HistoryLength(result, req.HistoryLength)
	return result, nil
}

func (h *a2aV1Handler) ListTasks(ctx context.Context, req *lf.ListTasksRequest) (*lf.ListTasksResponse, error) {
	call, err := v1CallFromContext(ctx)
	if err != nil {
		return nil, lf.ErrInvalidRequest
	}
	if req == nil {
		req = &lf.ListTasksRequest{}
	}
	pageSize := req.PageSize
	if pageSize == 0 {
		pageSize = 50
	}
	if pageSize < 1 || pageSize > 100 {
		return nil, lf.ErrInvalidParams
	}

	tasks, err := h.server.stores.Tasks.List(ctx)
	if err != nil {
		return nil, lf.ErrInternalError
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		return taskSortTimestamp(tasks[i]).After(taskSortTimestamp(tasks[j]))
	})

	filtered := make([]resources.Task, 0, len(tasks))
	for _, task := range tasks {
		if !isA2ATask(task) || !h.server.a2aIdentityAllowsTask(call.request, task) {
			continue
		}
		if call.systemName != "" && task.Spec.System != call.systemName {
			continue
		}
		if req.ContextID != "" && task.Metadata.Labels[orloja2a.LabelA2AContextID] != req.ContextID {
			continue
		}
		if req.Status != "" && orloja2a.OrlojPhaseToV1State(task) != req.Status {
			continue
		}
		if req.StatusTimestampAfter != nil && taskSortTimestamp(task).Before(*req.StatusTimestampAfter) {
			continue
		}
		filtered = append(filtered, task)
	}

	start, err := v1PageStart(filtered, req.PageToken)
	if err != nil {
		return nil, lf.ErrInvalidParams
	}
	end := min(start+pageSize, len(filtered))
	result := &lf.ListTasksResponse{
		Tasks:     make([]*lf.Task, 0, end-start),
		PageSize:  pageSize,
		TotalSize: len(filtered),
	}
	for _, task := range filtered[start:end] {
		item := orloja2a.OrlojTaskToV1(task)
		applyV1HistoryLength(item, req.HistoryLength)
		if !req.IncludeArtifacts {
			item.Artifacts = nil
		}
		result.Tasks = append(result.Tasks, item)
	}
	if end < len(filtered) {
		result.NextPageToken = base64.RawURLEncoding.EncodeToString([]byte(store.ScopedName(
			filtered[end-1].Metadata.Namespace,
			filtered[end-1].Metadata.Name,
		)))
	}
	return result, nil
}

func (h *a2aV1Handler) CancelTask(ctx context.Context, req *lf.CancelTaskRequest) (*lf.Task, error) {
	call, err := v1CallFromContext(ctx)
	if err != nil || req == nil || req.ID == "" {
		return nil, lf.ErrInvalidParams
	}
	task, err := h.server.findTaskByA2AID(call.request, string(req.ID), call.systemName)
	if err != nil {
		return nil, lf.ErrTaskNotFound
	}
	current := orloja2a.OrlojTaskToV1(task)
	if current.Status.State.Terminal() {
		return nil, lf.ErrTaskNotCancelable
	}

	if task.Metadata.Labels == nil {
		task.Metadata.Labels = make(map[string]string)
	}
	task.Metadata.Labels[orloja2a.LabelA2ACancelled] = "true"
	task.Status.Phase = "Failed"
	task.Status.LastError = "a2a:cancelled:cancelled via A2A v1"
	task.Status.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	updated, err := h.server.stores.Tasks.Upsert(ctx, task)
	if err != nil {
		return nil, lf.ErrInternalError
	}
	h.server.publishResourceEvent("Task", updated.Metadata.Name, "status", updated)
	return orloja2a.OrlojTaskToV1(updated), nil
}

func (h *a2aV1Handler) SendMessage(ctx context.Context, req *lf.SendMessageRequest) (lf.SendMessageResult, error) {
	task, call, err := h.createTask(ctx, req)
	if err != nil {
		return nil, err
	}
	result := orloja2a.OrlojTaskToV1(task)
	if req.Config != nil && req.Config.ReturnImmediately {
		return result, nil
	}
	return h.waitForTask(ctx, call, task, nil)
}

func (h *a2aV1Handler) SendStreamingMessage(ctx context.Context, req *lf.SendMessageRequest) iter.Seq2[lf.Event, error] {
	return func(yield func(lf.Event, error) bool) {
		if h.server.a2aConfig != nil && !h.server.a2aConfig.StreamingEnabled {
			yield(nil, lf.ErrUnsupportedOperation)
			return
		}
		task, call, err := h.createTask(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}
		initial := orloja2a.OrlojTaskToV1(task)
		if !yield(initial, nil) || initial.Status.State.Terminal() {
			return
		}
		_, _ = h.waitForTask(ctx, call, task, yield)
	}
}

func (h *a2aV1Handler) SubscribeToTask(ctx context.Context, req *lf.SubscribeToTaskRequest) iter.Seq2[lf.Event, error] {
	return func(yield func(lf.Event, error) bool) {
		call, err := v1CallFromContext(ctx)
		if err != nil || req == nil || req.ID == "" {
			yield(nil, lf.ErrInvalidParams)
			return
		}
		if h.server.a2aConfig != nil && !h.server.a2aConfig.StreamingEnabled {
			yield(nil, lf.ErrUnsupportedOperation)
			return
		}
		task, err := h.server.findTaskByA2AID(call.request, string(req.ID), call.systemName)
		if err != nil {
			yield(nil, lf.ErrTaskNotFound)
			return
		}
		initial := orloja2a.OrlojTaskToV1(task)
		if initial.Status.State.Terminal() {
			yield(nil, lf.ErrUnsupportedOperation)
			return
		}
		if !yield(initial, nil) {
			return
		}
		_, _ = h.waitForTask(ctx, call, task, yield)
	}
}

func (h *a2aV1Handler) GetTaskPushConfig(context.Context, *lf.GetTaskPushConfigRequest) (*lf.PushConfig, error) {
	return nil, lf.ErrPushNotificationNotSupported
}

func (h *a2aV1Handler) ListTaskPushConfigs(context.Context, *lf.ListTaskPushConfigRequest) (*lf.ListTaskPushConfigResponse, error) {
	return nil, lf.ErrPushNotificationNotSupported
}

func (h *a2aV1Handler) CreateTaskPushConfig(context.Context, *lf.PushConfig) (*lf.PushConfig, error) {
	return nil, lf.ErrPushNotificationNotSupported
}

func (h *a2aV1Handler) DeleteTaskPushConfig(context.Context, *lf.DeleteTaskPushConfigRequest) error {
	return lf.ErrPushNotificationNotSupported
}

func (h *a2aV1Handler) GetExtendedAgentCard(context.Context, *lf.GetExtendedAgentCardRequest) (*lf.AgentCard, error) {
	return nil, lf.ErrExtendedCardNotConfigured
}

func (h *a2aV1Handler) createTask(ctx context.Context, req *lf.SendMessageRequest) (resources.Task, a2aV1CallContext, error) {
	call, err := v1CallFromContext(ctx)
	if err != nil {
		return resources.Task{}, call, lf.ErrInvalidRequest
	}
	systemName := call.systemName
	if systemName == "" && req != nil {
		systemName = v1TargetFromMetadata(req.Metadata)
	}
	var target resources.AgentSystem
	if systemName == "" {
		var ok bool
		target, ok, err = h.server.defaultA2ASystem(call.request)
		if err != nil {
			return resources.Task{}, call, lf.ErrInternalError
		}
		if !ok {
			return resources.Task{}, call, lf.ErrInvalidParams
		}
		systemName = target.Metadata.Name
	} else {
		var ok bool
		target, ok, err = h.server.a2aEnabledSystemByName(call.request, systemName)
		if err != nil {
			return resources.Task{}, call, lf.ErrInternalError
		}
		if !ok {
			return resources.Task{}, call, lf.ErrTaskNotFound
		}
	}
	if !a2aIdentityAllowsSystem(call.request, target) && target.Spec.A2A.Auth != resources.A2AAuthPublic {
		return resources.Task{}, call, lf.ErrTaskNotFound
	}

	if req != nil && req.Message != nil && req.Message.TaskID != "" {
		return resources.Task{}, call, lf.ErrUnsupportedOperation
	}
	task, err := orloja2a.CreateOrlojTaskFromV1(req, target.Metadata.Name, target.Metadata.Namespace)
	if err != nil {
		return resources.Task{}, call, err
	}
	task.Metadata.Name = a2aInternalTaskName(target, task.Metadata.Labels[orloja2a.LabelA2ATaskID])
	created, err := h.server.stores.Tasks.Upsert(ctx, task)
	if err != nil {
		return resources.Task{}, call, lf.ErrInternalError
	}
	h.server.publishResourceEvent("Task", created.Metadata.Name, "created", created)
	call.systemName = target.Metadata.Name
	return created, call, nil
}

func (h *a2aV1Handler) waitForTask(
	ctx context.Context,
	call a2aV1CallContext,
	task resources.Task,
	yield func(lf.Event, error) bool,
) (*lf.Task, error) {
	since := h.server.bus.LatestID()
	events := h.server.bus.Subscribe(ctx, eventbus.Filter{
		SinceID:   since,
		Kind:      "Task",
		Name:      task.Metadata.Name,
		Namespace: resources.NormalizeNamespace(task.Metadata.Namespace),
	})
	key := store.ScopedName(task.Metadata.Namespace, task.Metadata.Name)

	for {
		current, ok, err := h.server.stores.Tasks.Get(ctx, key)
		if err != nil {
			return nil, lf.ErrInternalError
		}
		if !ok || !h.server.a2aIdentityAllowsTask(call.request, current) {
			return nil, lf.ErrTaskNotFound
		}
		result := orloja2a.OrlojTaskToV1(current)
		if result.Status.State.Terminal() || result.Status.State == lf.TaskStateInputRequired || result.Status.State == lf.TaskStateAuthRequired {
			if yield != nil {
				yield(&lf.TaskStatusUpdateEvent{
					TaskID:    result.ID,
					ContextID: result.ContextID,
					Status:    result.Status,
				}, nil)
			}
			return result, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case _, ok := <-events:
			if !ok {
				return nil, ctx.Err()
			}
			if yield != nil {
				updated, exists, getErr := h.server.stores.Tasks.Get(ctx, key)
				if getErr != nil || !exists {
					return nil, lf.ErrTaskNotFound
				}
				v1 := orloja2a.OrlojTaskToV1(updated)
				if !yield(&lf.TaskStatusUpdateEvent{
					TaskID:    v1.ID,
					ContextID: v1.ContextID,
					Status:    v1.Status,
				}, nil) {
					return v1, nil
				}
			}
		}
	}
}

func v1CallFromContext(ctx context.Context) (a2aV1CallContext, error) {
	call, ok := ctx.Value(a2aV1CallContextKey{}).(a2aV1CallContext)
	if !ok || call.request == nil {
		return a2aV1CallContext{}, fmt.Errorf("missing A2A request context")
	}
	return call, nil
}

func v1TargetFromMetadata(metadata map[string]any) string {
	for _, key := range []string{"agent", "target"} {
		if value, ok := metadata[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isA2ATask(task resources.Task) bool {
	return task.Metadata.Labels != nil && strings.TrimSpace(task.Metadata.Labels[orloja2a.LabelA2ATaskID]) != ""
}

func taskSortTimestamp(task resources.Task) time.Time {
	for _, value := range []string{task.Status.CompletedAt, task.Status.LastHeartbeat, task.Status.StartedAt, task.Metadata.CreatedAt} {
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func v1PageStart(tasks []resources.Task, token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, err
	}
	for index, task := range tasks {
		if store.ScopedName(task.Metadata.Namespace, task.Metadata.Name) == string(decoded) {
			return index + 1, nil
		}
	}
	return 0, fmt.Errorf("page token no longer exists")
}

func applyV1HistoryLength(task *lf.Task, historyLength *int) {
	if task == nil || historyLength == nil {
		return
	}
	if *historyLength <= 0 {
		task.History = nil
		return
	}
	if len(task.History) > *historyLength {
		task.History = task.History[len(task.History)-*historyLength:]
	}
}

func isA2AV1Method(method string) bool {
	switch method {
	case "SendMessage",
		"SendStreamingMessage",
		"GetTask",
		"ListTasks",
		"CancelTask",
		"SubscribeToTask",
		"CreateTaskPushNotificationConfig",
		"GetTaskPushNotificationConfig",
		"ListTaskPushNotificationConfigs",
		"DeleteTaskPushNotificationConfig",
		"GetExtendedAgentCard":
		return true
	default:
		return false
	}
}
