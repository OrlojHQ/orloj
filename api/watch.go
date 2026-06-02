package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/resources"
)

type watchEvent struct {
	Type     string `json:"type"`
	Resource any    `json:"resource"`
}

type watchRecord struct {
	Name            string
	Namespace       string
	ResourceVersion int64
	Resource        any
}

func (s *Server) watchAgents(w http.ResponseWriter, r *http.Request) {
	s.watchResourceStream(w, r, "Agent", func() []watchRecord {
		items, err := s.stores.Agents.List(r.Context())
		if err != nil {
			writeStoreFetchError(w, err)
			return nil
		}
		records := make([]watchRecord, 0, len(items))
		for _, item := range items {
			item = s.withRuntimeStatus(item)
			records = append(records, watchRecord{
				Name:            item.Metadata.Name,
				Namespace:       resources.NormalizeNamespace(item.Metadata.Namespace),
				ResourceVersion: parseResourceVersion(item.Metadata.ResourceVersion),
				Resource:        item,
			})
		}
		return records
	})
}

func (s *Server) watchTasks(w http.ResponseWriter, r *http.Request) {
	s.watchResourceStream(w, r, "Task", func() []watchRecord {
		items, err := s.stores.Tasks.List(r.Context())
		if err != nil {
			writeStoreFetchError(w, err)
			return nil
		}
		records := make([]watchRecord, 0, len(items))
		for _, item := range items {
			records = append(records, watchRecord{
				Name:            item.Metadata.Name,
				Namespace:       resources.NormalizeNamespace(item.Metadata.Namespace),
				ResourceVersion: parseResourceVersion(item.Metadata.ResourceVersion),
				Resource:        item,
			})
		}
		return records
	})
}

func (s *Server) watchTaskSchedules(w http.ResponseWriter, r *http.Request) {
	s.watchResourceStream(w, r, "TaskSchedule", func() []watchRecord {
		items, err := s.stores.TaskSchedules.List(r.Context())
		if err != nil {
			writeStoreFetchError(w, err)
			return nil
		}
		records := make([]watchRecord, 0, len(items))
		for _, item := range items {
			records = append(records, watchRecord{
				Name:            item.Metadata.Name,
				Namespace:       resources.NormalizeNamespace(item.Metadata.Namespace),
				ResourceVersion: parseResourceVersion(item.Metadata.ResourceVersion),
				Resource:        item,
			})
		}
		return records
	})
}

func (s *Server) watchTaskWebhooks(w http.ResponseWriter, r *http.Request) {
	s.watchResourceStream(w, r, "TaskWebhook", func() []watchRecord {
		items, err := s.stores.TaskWebhooks.List(r.Context())
		if err != nil {
			writeStoreFetchError(w, err)
			return nil
		}
		records := make([]watchRecord, 0, len(items))
		for _, item := range items {
			records = append(records, watchRecord{
				Name:            item.Metadata.Name,
				Namespace:       resources.NormalizeNamespace(item.Metadata.Namespace),
				ResourceVersion: parseResourceVersion(item.Metadata.ResourceVersion),
				Resource:        item,
			})
		}
		return records
	})
}

func (s *Server) watchEvents(w http.ResponseWriter, r *http.Request) {
	if s.bus == nil {
		http.Error(w, "event bus unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel, ok := s.acquireWatchStream(w, r)
	if !ok {
		return
	}
	defer cancel()
	r = r.WithContext(ctx)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	since := parseSinceID(r.URL.Query().Get("since"))
	filter := eventbus.Filter{
		SinceID:   since,
		Source:    strings.TrimSpace(r.URL.Query().Get("source")),
		Type:      strings.TrimSpace(r.URL.Query().Get("type")),
		Kind:      strings.TrimSpace(r.URL.Query().Get("kind")),
		Name:      strings.TrimSpace(r.URL.Query().Get("name")),
		Namespace: strings.TrimSpace(r.URL.Query().Get("namespace")),
	}
	stream := s.bus.Subscribe(r.Context(), filter)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
				_ = writeSSE(w, "close", map[string]string{"reason": "max_watch_duration_reached"})
				flusher.Flush()
			}
			return
		case evt, ok := <-stream:
			if !ok {
				return
			}
			if err := writeSSE(w, "event", evt); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

const (
	watchMaxDuration         = 30 * time.Minute
	watchMaxConcurrentGlobal = 1000
)

func (s *Server) acquireWatchStream(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, bool) {
	if s != nil && s.watchRateLimiter != nil && !s.watchRateLimiter.Allow(r) {
		http.Error(w, "too many watch connections from this client", http.StatusTooManyRequests)
		return nil, nil, false
	}
	if s != nil && watchMaxConcurrentGlobal > 0 {
		cur := s.watchSubscribeCount.Add(1)
		if int(cur) > watchMaxConcurrentGlobal {
			s.watchSubscribeCount.Add(-1)
			http.Error(w, "too many concurrent watch connections", http.StatusServiceUnavailable)
			return nil, nil, false
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), watchMaxDuration)
	if s != nil && watchMaxConcurrentGlobal > 0 {
		origCancel := cancel
		cancel = func() {
			origCancel()
			s.watchSubscribeCount.Add(-1)
		}
	}
	return ctx, cancel, true
}

func (s *Server) watchResourceStream(w http.ResponseWriter, r *http.Request, kind string, snapshot func() []watchRecord) {
	ctx, cancel, ok := s.acquireWatchStream(w, r)
	if !ok {
		return
	}
	defer cancel()
	r = r.WithContext(ctx)
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	sinceRV := parseResourceVersion(r.URL.Query().Get("resourceVersion"))
	nameFilter := strings.TrimSpace(r.URL.Query().Get("name"))
	namespaceFilterValue, hasNamespaceFilter := namespaceFilter(r)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	seen := make(map[string]int64)
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	startEventID := uint64(0)
	if s != nil && s.bus != nil {
		startEventID = s.bus.LatestID()
	}

	if err := sendWatchSnapshot(w, snapshot(), seen, sinceRV, nameFilter, namespaceFilterValue, hasNamespaceFilter); err != nil {
		return
	}
	flusher.Flush()

	if s == nil || s.bus == nil {
		s.watchResourceStreamPolling(w, r, snapshot, seen, sinceRV, nameFilter, namespaceFilterValue, hasNamespaceFilter, flusher, heartbeat)
		return
	}

	filter := eventbus.Filter{
		SinceID:   startEventID,
		Source:    "apiserver",
		Kind:      strings.TrimSpace(kind),
		Name:      nameFilter,
		Namespace: namespaceFilterValue,
	}
	if !hasNamespaceFilter {
		filter.Namespace = ""
	}
	stream := s.bus.Subscribe(r.Context(), filter)

	for {
		select {
		case <-r.Context().Done():
			if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
				_ = writeSSE(w, "close", map[string]string{"reason": "max_watch_duration_reached"})
				flusher.Flush()
			}
			return
		case evt := <-stream:
			record, ok := watchRecordFromResource(evt.Data)
			if !ok {
				if strings.EqualFold(strings.TrimSpace(evt.Action), "deleted") {
					record = watchRecord{
						Name:      strings.TrimSpace(evt.Name),
						Namespace: resources.NormalizeNamespace(strings.TrimSpace(evt.Namespace)),
						Resource: map[string]any{
							"metadata": resources.ObjectMeta{
								Name:      strings.TrimSpace(evt.Name),
								Namespace: resources.NormalizeNamespace(strings.TrimSpace(evt.Namespace)),
							},
						},
					}
				} else {
					continue
				}
			}
			if hasNamespaceFilter && !strings.EqualFold(record.Namespace, namespaceFilterValue) {
				continue
			}
			if nameFilter != "" && !strings.EqualFold(record.Name, nameFilter) {
				continue
			}
			eventType := watchEventTypeForAction(evt.Action)
			if eventType != "deleted" {
				recordKey := record.Namespace + "/" + record.Name
				if record.ResourceVersion <= sinceRV {
					continue
				}
				if previousRV, existed := seen[recordKey]; existed && record.ResourceVersion != 0 && previousRV == record.ResourceVersion {
					continue
				}
				seen[recordKey] = record.ResourceVersion
			} else {
				recordKey := record.Namespace + "/" + record.Name
				if _, existed := seen[recordKey]; !existed {
					continue
				}
				delete(seen, recordKey)
			}
			if err := writeSSE(w, "resource", watchEvent{Type: eventType, Resource: record.Resource}); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) watchResourceStreamPolling(
	w http.ResponseWriter,
	r *http.Request,
	snapshot func() []watchRecord,
	seen map[string]int64,
	sinceRV int64,
	nameFilter string,
	namespaceFilterValue string,
	hasNamespaceFilter bool,
	flusher http.Flusher,
	heartbeat *time.Ticker,
) {
	poll := time.NewTicker(1 * time.Second)
	defer poll.Stop()

	for {
		select {
		case <-r.Context().Done():
			if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
				_ = writeSSE(w, "close", map[string]string{"reason": "max_watch_duration_reached"})
				flusher.Flush()
			}
			return
		case <-poll.C:
			if err := sendWatchSnapshot(w, snapshot(), seen, sinceRV, nameFilter, namespaceFilterValue, hasNamespaceFilter); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func sendWatchSnapshot(
	w http.ResponseWriter,
	records []watchRecord,
	seen map[string]int64,
	sinceRV int64,
	nameFilter string,
	namespaceFilterValue string,
	hasNamespaceFilter bool,
) error {
	sort.Slice(records, func(i, j int) bool {
		left := records[i].Namespace + "/" + records[i].Name
		right := records[j].Namespace + "/" + records[j].Name
		return left < right
	})

	nextSeen := make(map[string]int64, len(records))
	for _, rec := range records {
		rec.Namespace = resources.NormalizeNamespace(rec.Namespace)
		if hasNamespaceFilter && !strings.EqualFold(rec.Namespace, namespaceFilterValue) {
			continue
		}
		if nameFilter != "" && !strings.EqualFold(nameFilter, rec.Name) {
			continue
		}
		recordKey := rec.Namespace + "/" + rec.Name
		nextSeen[recordKey] = rec.ResourceVersion
		previousRV, existed := seen[recordKey]
		if rec.ResourceVersion <= sinceRV {
			continue
		}
		if existed && previousRV == rec.ResourceVersion {
			continue
		}
		eventType := "added"
		if existed {
			eventType = "updated"
		}
		if err := writeSSE(w, "resource", watchEvent{Type: eventType, Resource: rec.Resource}); err != nil {
			return err
		}
	}

	for key, previousRV := range seen {
		if _, ok := nextSeen[key]; ok {
			continue
		}
		namespace := ""
		name := key
		if parts := strings.SplitN(key, "/", 2); len(parts) == 2 {
			namespace = parts[0]
			name = parts[1]
		}
		if hasNamespaceFilter && !strings.EqualFold(namespace, namespaceFilterValue) {
			continue
		}
		if nameFilter != "" && !strings.EqualFold(nameFilter, name) {
			continue
		}
		meta := resources.ObjectMeta{Name: name, Namespace: namespace, ResourceVersion: strconv.FormatInt(previousRV, 10)}
		if err := writeSSE(w, "resource", watchEvent{Type: "deleted", Resource: map[string]any{"metadata": meta}}); err != nil {
			return err
		}
	}

	for key := range seen {
		delete(seen, key)
	}
	for key, value := range nextSeen {
		seen[key] = value
	}
	return nil
}

func watchRecordFromResource(resource any) (watchRecord, bool) {
	switch obj := resource.(type) {
	case resources.Agent:
		return watchRecord{
			Name:            strings.TrimSpace(obj.Metadata.Name),
			Namespace:       resources.NormalizeNamespace(obj.Metadata.Namespace),
			ResourceVersion: parseResourceVersion(obj.Metadata.ResourceVersion),
			Resource:        obj,
		}, true
	case resources.Task:
		return watchRecord{
			Name:            strings.TrimSpace(obj.Metadata.Name),
			Namespace:       resources.NormalizeNamespace(obj.Metadata.Namespace),
			ResourceVersion: parseResourceVersion(obj.Metadata.ResourceVersion),
			Resource:        obj,
		}, true
	case resources.TaskSchedule:
		return watchRecord{
			Name:            strings.TrimSpace(obj.Metadata.Name),
			Namespace:       resources.NormalizeNamespace(obj.Metadata.Namespace),
			ResourceVersion: parseResourceVersion(obj.Metadata.ResourceVersion),
			Resource:        obj,
		}, true
	case resources.TaskWebhook:
		return watchRecord{
			Name:            strings.TrimSpace(obj.Metadata.Name),
			Namespace:       resources.NormalizeNamespace(obj.Metadata.Namespace),
			ResourceVersion: parseResourceVersion(obj.Metadata.ResourceVersion),
			Resource:        obj,
		}, true
	case map[string]any:
		meta, ok := watchObjectMetaFromMap(obj["metadata"])
		if !ok {
			return watchRecord{}, false
		}
		return watchRecord{
			Name:            strings.TrimSpace(meta.Name),
			Namespace:       resources.NormalizeNamespace(meta.Namespace),
			ResourceVersion: parseResourceVersion(meta.ResourceVersion),
			Resource:        obj,
		}, true
	default:
		return watchRecord{}, false
	}
}

func watchObjectMetaFromMap(raw any) (resources.ObjectMeta, bool) {
	switch meta := raw.(type) {
	case resources.ObjectMeta:
		return meta, true
	case map[string]string:
		return resources.ObjectMeta{
			Name:            strings.TrimSpace(meta["name"]),
			Namespace:       resources.NormalizeNamespace(meta["namespace"]),
			ResourceVersion: strings.TrimSpace(meta["resourceVersion"]),
		}, true
	case map[string]any:
		out := resources.ObjectMeta{}
		if name, ok := meta["name"].(string); ok {
			out.Name = strings.TrimSpace(name)
		}
		if namespace, ok := meta["namespace"].(string); ok {
			out.Namespace = resources.NormalizeNamespace(namespace)
		}
		if rv, ok := meta["resourceVersion"].(string); ok {
			out.ResourceVersion = strings.TrimSpace(rv)
		}
		return out, strings.TrimSpace(out.Name) != ""
	default:
		return resources.ObjectMeta{}, false
	}
}

func watchEventTypeForAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "created":
		return "added"
	case "deleted":
		return "deleted"
	default:
		return "updated"
	}
}

func writeSSE(w http.ResponseWriter, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	return nil
}

func parseResourceVersion(v string) int64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func parseSinceID(v string) uint64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
