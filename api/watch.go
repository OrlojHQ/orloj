package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/eventbus"
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
	s.watchResourceStream(w, r, func() []watchRecord {
		items, err := s.stores.Agents.List()
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
	s.watchResourceStream(w, r, func() []watchRecord {
		items, err := s.stores.Tasks.List()
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
	s.watchResourceStream(w, r, func() []watchRecord {
		items, err := s.stores.TaskSchedules.List()
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
	s.watchResourceStream(w, r, func() []watchRecord {
		items, err := s.stores.TaskWebhooks.List()
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

func (s *Server) watchResourceStream(w http.ResponseWriter, r *http.Request, snapshot func() []watchRecord) {
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
	poll := time.NewTicker(1 * time.Second)
	defer poll.Stop()

	sendSnapshot := func() error {
		records := snapshot()
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

		seen = nextSeen
		return nil
	}

	if err := sendSnapshot(); err != nil {
		return
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
			if err := sendSnapshot(); err != nil {
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
