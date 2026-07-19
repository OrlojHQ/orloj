package api

import (
	"context"
	"time"

	lf "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv/push"

	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/resources"
	orloja2a "github.com/OrlojHQ/orloj/runtime/a2a"
	"github.com/OrlojHQ/orloj/store"
)

// StartA2APushDispatcher subscribes to authoritative Task events and delivers
// the latest v1 task state to every registered callback. It blocks until ctx
// is canceled and is intended to run as one server background component.
func (s *Server) StartA2APushDispatcher(ctx context.Context) {
	if s == nil || s.bus == nil || s.stores.A2APushConfigs == nil {
		return
	}
	allowPrivate := s.a2aConfig != nil && s.a2aConfig.AllowPrivateEndpoints
	sender := orloja2a.NewSafePushSender(allowPrivate, 10*time.Second)
	events := s.bus.Subscribe(ctx, eventbus.Filter{Kind: "Task"})
	sem := make(chan struct{}, 8)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			select {
			case sem <- struct{}{}:
				go func() {
					defer func() { <-sem }()
					s.dispatchA2APushEvent(ctx, sender, event)
				}()
			case <-ctx.Done():
				return
			}
		}
	}
}

func (s *Server) dispatchA2APushEvent(ctx context.Context, sender push.Sender, event eventbus.Event) {
	key := store.ScopedName(resources.NormalizeNamespace(event.Namespace), event.Name)
	task, ok, err := s.stores.Tasks.Get(ctx, key)
	if err != nil || !ok || task.Metadata.Labels == nil {
		return
	}
	taskID := lf.TaskID(task.Metadata.Labels[orloja2a.LabelA2ATaskID])
	if taskID == "" {
		return
	}
	configs, err := s.stores.A2APushConfigs.List(ctx, taskID)
	if err != nil || len(configs) == 0 {
		return
	}
	v1Task := orloja2a.OrlojTaskToV1(task)
	for _, config := range configs {
		var lastErr error
		for attempt, delay := range []time.Duration{0, 250 * time.Millisecond, time.Second} {
			if delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			lastErr = sender.SendPush(ctx, config, v1Task)
			if lastErr == nil {
				break
			}
			if attempt == 2 && s.logger != nil {
				s.logger.Printf(
					"A2A push delivery failed task_id=%s config_id=%s attempts=3 error=%v",
					taskID,
					config.ID,
					lastErr,
				)
			}
		}
	}
}
