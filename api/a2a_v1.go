package api

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"sort"
	"strings"
	"time"

	lf "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/a2asrv/push"
	"google.golang.org/grpc/metadata"
	grpcpeer "google.golang.org/grpc/peer"

	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
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
	call, err := h.callFromContext(ctx)
	if err != nil || req == nil || req.ID == "" {
		return nil, lf.ErrInvalidParams
	}
	call = v1CallForTenant(call, req.Tenant)
	task, err := h.server.findTaskByA2AID(call.request, string(req.ID), call.systemName)
	if err != nil {
		return nil, lf.ErrTaskNotFound
	}
	result := orloja2a.OrlojTaskToV1(task)
	applyV1HistoryLength(result, req.HistoryLength)
	return result, nil
}

func (h *a2aV1Handler) ListTasks(ctx context.Context, req *lf.ListTasksRequest) (*lf.ListTasksResponse, error) {
	call, err := h.callFromContext(ctx)
	if err != nil {
		return nil, lf.ErrInvalidRequest
	}
	if req == nil {
		req = &lf.ListTasksRequest{}
	}
	call = v1CallForTenant(call, req.Tenant)
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
	call, err := h.callFromContext(ctx)
	if err != nil || req == nil || req.ID == "" {
		return nil, lf.ErrInvalidParams
	}
	call = v1CallForTenant(call, req.Tenant)
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
	if req == nil || req.Message == nil {
		return nil, lf.ErrInvalidParams
	}
	if req != nil && req.Config != nil && req.Config.ReturnImmediately {
		task, _, err := h.createTask(ctx, req)
		if err != nil {
			return nil, err
		}
		return orloja2a.OrlojTaskToV1(task), nil
	}
	waitCtx, release, ok := h.server.acquireA2AWaiter(ctx)
	if !ok {
		return nil, lf.NewError(lf.ErrInternalError, "too many concurrent A2A wait connections")
	}
	defer release()
	task, call, err := h.createTask(waitCtx, req)
	if err != nil {
		return nil, err
	}
	return h.waitForTask(waitCtx, call, task, nil)
}

func (h *a2aV1Handler) SendStreamingMessage(ctx context.Context, req *lf.SendMessageRequest) iter.Seq2[lf.Event, error] {
	return func(yield func(lf.Event, error) bool) {
		if req == nil || req.Message == nil {
			yield(nil, lf.ErrInvalidParams)
			return
		}
		if h.server.a2aConfig != nil && !h.server.a2aConfig.StreamingEnabled {
			yield(nil, lf.ErrUnsupportedOperation)
			return
		}
		waitCtx, release, ok := h.server.acquireA2AWaiter(ctx)
		if !ok {
			yield(nil, lf.NewError(lf.ErrInternalError, "too many concurrent A2A wait connections"))
			return
		}
		defer release()
		task, call, err := h.createTask(waitCtx, req)
		if err != nil {
			yield(nil, err)
			return
		}
		initial := orloja2a.OrlojTaskToV1(task)
		if !yield(initial, nil) || initial.Status.State.Terminal() {
			return
		}
		if _, err := h.waitForTask(waitCtx, call, task, yield); errors.Is(err, context.DeadlineExceeded) {
			yield(nil, lf.NewError(lf.ErrInternalError, "maximum A2A wait duration exceeded"))
		}
	}
}

func (h *a2aV1Handler) SubscribeToTask(ctx context.Context, req *lf.SubscribeToTaskRequest) iter.Seq2[lf.Event, error] {
	return func(yield func(lf.Event, error) bool) {
		call, err := h.callFromContext(ctx)
		if err != nil || req == nil || req.ID == "" {
			yield(nil, lf.ErrInvalidParams)
			return
		}
		call = v1CallForTenant(call, req.Tenant)
		if h.server.a2aConfig != nil && !h.server.a2aConfig.StreamingEnabled {
			yield(nil, lf.ErrUnsupportedOperation)
			return
		}
		waitCtx, release, ok := h.server.acquireA2AWaiter(ctx)
		if !ok {
			yield(nil, lf.NewError(lf.ErrInternalError, "too many concurrent A2A wait connections"))
			return
		}
		defer release()
		task, err := h.server.findTaskByA2AID(call.request.WithContext(waitCtx), string(req.ID), call.systemName)
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
		if _, err := h.waitForTask(waitCtx, call, task, yield); errors.Is(err, context.DeadlineExceeded) {
			yield(nil, lf.NewError(lf.ErrInternalError, "maximum A2A wait duration exceeded"))
		}
	}
}

func (h *a2aV1Handler) GetTaskPushConfig(ctx context.Context, req *lf.GetTaskPushConfigRequest) (*lf.PushConfig, error) {
	call, err := h.callFromContext(ctx)
	if err != nil || req == nil || req.TaskID == "" || req.ID == "" {
		return nil, lf.ErrInvalidParams
	}
	call = v1CallForTenant(call, req.Tenant)
	task, err := h.server.findTaskByA2AID(call.request, string(req.TaskID), call.systemName)
	if err != nil {
		return nil, lf.ErrTaskNotFound
	}
	taskKey := store.ScopedName(task.Metadata.Namespace, task.Metadata.Name)
	config, err := h.server.stores.A2APushConfigs.GetForTask(ctx, taskKey, req.TaskID, req.ID)
	if err != nil {
		if errors.Is(err, push.ErrPushConfigNotFound) {
			return nil, lf.ErrTaskNotFound
		}
		return nil, lf.ErrInternalError
	}
	return config, nil
}

func (h *a2aV1Handler) ListTaskPushConfigs(ctx context.Context, req *lf.ListTaskPushConfigRequest) (*lf.ListTaskPushConfigResponse, error) {
	call, err := h.callFromContext(ctx)
	if err != nil || req == nil || req.TaskID == "" {
		return nil, lf.ErrInvalidParams
	}
	call = v1CallForTenant(call, req.Tenant)
	task, err := h.server.findTaskByA2AID(call.request, string(req.TaskID), call.systemName)
	if err != nil {
		return nil, lf.ErrTaskNotFound
	}
	taskKey := store.ScopedName(task.Metadata.Namespace, task.Metadata.Name)
	configs, err := h.server.stores.A2APushConfigs.ListForTask(ctx, taskKey, req.TaskID)
	if err != nil {
		return nil, lf.ErrInternalError
	}
	pageSize := req.PageSize
	if pageSize == 0 {
		pageSize = 50
	}
	if pageSize < 1 || pageSize > 100 {
		return nil, lf.ErrInvalidParams
	}
	start := 0
	if req.PageToken != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(req.PageToken)
		if err != nil {
			return nil, lf.ErrInvalidParams
		}
		found := false
		for index, config := range configs {
			if config.ID == string(decoded) {
				start = index + 1
				found = true
				break
			}
		}
		if !found {
			return nil, lf.ErrInvalidParams
		}
	}
	end := min(start+pageSize, len(configs))
	response := &lf.ListTaskPushConfigResponse{Configs: configs[start:end]}
	if end < len(configs) {
		response.NextPageToken = base64.RawURLEncoding.EncodeToString([]byte(configs[end-1].ID))
	}
	return response, nil
}

func (h *a2aV1Handler) CreateTaskPushConfig(ctx context.Context, req *lf.PushConfig) (*lf.PushConfig, error) {
	call, err := h.callFromContext(ctx)
	if err != nil || req == nil || req.TaskID == "" {
		return nil, lf.ErrInvalidParams
	}
	call = v1CallForTenant(call, req.Tenant)
	task, err := h.server.findTaskByA2AID(call.request, string(req.TaskID), call.systemName)
	if err != nil {
		return nil, lf.ErrTaskNotFound
	}
	if err := h.validatePushConfig(req); err != nil {
		return nil, err
	}
	taskKey := store.ScopedName(task.Metadata.Namespace, task.Metadata.Name)
	saved, err := h.server.stores.A2APushConfigs.SaveForTask(ctx, taskKey, req.TaskID, req)
	if err != nil {
		return nil, lf.ErrInternalError
	}
	return saved, nil
}

func (h *a2aV1Handler) DeleteTaskPushConfig(ctx context.Context, req *lf.DeleteTaskPushConfigRequest) error {
	call, err := h.callFromContext(ctx)
	if err != nil || req == nil || req.TaskID == "" || req.ID == "" {
		return lf.ErrInvalidParams
	}
	call = v1CallForTenant(call, req.Tenant)
	task, err := h.server.findTaskByA2AID(call.request, string(req.TaskID), call.systemName)
	if err != nil {
		return lf.ErrTaskNotFound
	}
	taskKey := store.ScopedName(task.Metadata.Namespace, task.Metadata.Name)
	if err := h.server.stores.A2APushConfigs.DeleteForTask(ctx, taskKey, req.TaskID, req.ID); err != nil {
		return lf.ErrInternalError
	}
	return nil
}

func (h *a2aV1Handler) GetExtendedAgentCard(ctx context.Context, _ *lf.GetExtendedAgentCardRequest) (*lf.AgentCard, error) {
	if _, err := h.callFromContext(ctx); err != nil {
		return nil, err
	}
	return nil, lf.ErrExtendedCardNotConfigured
}

func (h *a2aV1Handler) createTask(ctx context.Context, req *lf.SendMessageRequest) (resources.Task, a2aV1CallContext, error) {
	call, err := h.callFromContext(ctx)
	if err != nil {
		return resources.Task{}, call, lf.ErrInvalidRequest
	}
	systemName := call.systemName
	if systemName == "" && req != nil {
		call = v1CallForTenant(call, req.Tenant)
		systemName = call.systemName
	}
	if systemName == "" && req != nil {
		call = v1CallForTenant(call, v1TargetFromMetadata(req.Metadata))
		systemName = call.systemName
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
	if req.Config != nil && req.Config.PushConfig != nil {
		req.Config.PushConfig.TaskID = lf.TaskID(task.Metadata.Labels[orloja2a.LabelA2ATaskID])
		if err := h.validatePushConfig(req.Config.PushConfig); err != nil {
			return resources.Task{}, call, err
		}
	}
	task.Metadata.Name = a2aInternalTaskName(target, task.Metadata.Labels[orloja2a.LabelA2ATaskID])
	created, err := h.server.stores.Tasks.Upsert(ctx, task)
	if err != nil {
		return resources.Task{}, call, lf.ErrInternalError
	}
	h.server.publishResourceEvent("Task", created.Metadata.Name, "created", created)
	if req.Config != nil && req.Config.PushConfig != nil {
		config := req.Config.PushConfig
		config.TaskID = lf.TaskID(created.Metadata.Labels[orloja2a.LabelA2ATaskID])
		taskKey := store.ScopedName(created.Metadata.Namespace, created.Metadata.Name)
		if _, err := h.server.stores.A2APushConfigs.SaveForTask(ctx, taskKey, config.TaskID, config); err != nil {
			return resources.Task{}, call, lf.ErrInternalError
		}
	}
	call.systemName = target.Metadata.Name
	return created, call, nil
}

func (h *a2aV1Handler) validatePushConfig(config *lf.PushConfig) error {
	if config == nil || strings.TrimSpace(config.URL) == "" {
		return lf.ErrInvalidParams
	}
	allowPrivate := h.server.a2aConfig != nil && h.server.a2aConfig.AllowPrivateEndpoints
	if err := agentruntime.ValidateEndpointURL(config.URL, allowPrivate); err != nil {
		return fmt.Errorf("%w: invalid push notification URL", lf.ErrInvalidParams)
	}
	if config.Auth != nil {
		switch strings.ToLower(strings.TrimSpace(config.Auth.Scheme)) {
		case "", "bearer", "basic":
		default:
			return fmt.Errorf("%w: unsupported push authentication scheme", lf.ErrInvalidParams)
		}
	}
	return nil
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

func (h *a2aV1Handler) callFromContext(ctx context.Context) (a2aV1CallContext, error) {
	call, ok := ctx.Value(a2aV1CallContextKey{}).(a2aV1CallContext)
	if ok && call.request != nil {
		return call, nil
	}

	// gRPC carries the same bearer credential in incoming metadata. Build a
	// synthetic request so the existing A2A authorization and task visibility
	// rules remain the single source of truth across transports.
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://a2a-grpc.local/a2a", nil)
	if err != nil {
		return a2aV1CallContext{}, err
	}
	if md, present := metadata.FromIncomingContext(ctx); present {
		if values := md.Get("authorization"); len(values) > 0 {
			request.Header.Set("Authorization", values[0])
		}
	}
	if peer, present := grpcpeer.FromContext(ctx); present && peer.Addr != nil {
		request.RemoteAddr = peer.Addr.String()
	}
	if h.server.a2aRateLimiter != nil && !h.server.a2aRateLimiter.Allow(request) {
		return a2aV1CallContext{}, lf.NewError(lf.ErrInternalError, "rate limit exceeded")
	}
	allowed, statusCode, _, identity := authorizeWithIdentity(h.server.authorizer, request, "a2a")
	if allowed {
		request = request.WithContext(withAuthIdentity(request.Context(), identity))
		return a2aV1CallContext{request: request}, nil
	}
	if request.Header.Get("Authorization") == "" {
		// Unauthenticated calls may still access AgentSystems explicitly
		// configured with spec.a2a.auth=public.
		return a2aV1CallContext{request: request}, nil
	}
	if statusCode == http.StatusForbidden {
		return a2aV1CallContext{}, lf.ErrUnauthorized
	}
	return a2aV1CallContext{}, lf.ErrUnauthenticated
}

func v1TargetFromMetadata(metadata map[string]any) string {
	for _, key := range []string{"agent", "target"} {
		if value, ok := metadata[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func v1CallForTenant(call a2aV1CallContext, tenant string) a2aV1CallContext {
	if call.systemName != "" || call.request == nil {
		return call
	}
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		return call
	}
	namespace := resources.DefaultNamespace
	systemName := tenant
	if value, name, ok := strings.Cut(tenant, "/"); ok {
		namespace = resources.NormalizeNamespace(value)
		systemName = strings.TrimSpace(name)
	}
	if systemName == "" {
		return call
	}
	request := call.request.Clone(call.request.Context())
	query := request.URL.Query()
	query.Set("namespace", namespace)
	request.URL.RawQuery = query.Encode()
	call.request = request
	call.systemName = systemName
	return call
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
