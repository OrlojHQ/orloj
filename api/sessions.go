package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

const sessionStreamHeartbeat = 15 * time.Second

type sessionTurnRequest struct {
	Content   string `json:"content"`
	Interrupt bool   `json:"interrupt,omitempty"`
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.Sessions.ListCursor, func(item resources.Session) resources.ObjectMeta {
			return item.Metadata
		})
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.SessionList{
			ListMeta: resources.ListMeta{Continue: cont},
			Items:    items,
		})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseSessionManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		systemKey := store.ScopedName(obj.Metadata.Namespace, obj.Spec.System)
		system, ok, err := s.stores.AgentSystems.Get(r.Context(), systemKey)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("AgentSystem %q not found", obj.Spec.System), http.StatusBadRequest)
			return
		}
		current, exists, err := s.stores.Sessions.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if exists {
			obj.Status = current.Status
		} else {
			obj.Status.SystemGeneration = system.Metadata.Generation
		}
		obj, err = s.stores.Sessions.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		action := "created"
		if exists {
			action = "updated"
		}
		s.publishResourceEvent("Session", obj.Metadata.Name, action, obj)
		if !exists {
			if events, listErr := s.stores.Sessions.ListEvents(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name), 0, 1); listErr == nil {
				s.publishSessionEvents(events)
			}
		}
		s.logApply("Session", obj.Metadata.Name)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSessionByName(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/sessions/"), "/")
	if path == "" {
		http.Error(w, "session name is required", http.StatusBadRequest)
		return
	}
	if path == "watch" {
		s.watchSessions(w, r)
		return
	}
	if marker := strings.Index(path, "/checkpoints"); marker >= 0 {
		name := strings.Trim(path[:marker], "/")
		checkpointPath := strings.TrimPrefix(path[marker:], "/checkpoints")
		s.handleSessionCheckpoints(w, r, name, strings.Trim(checkpointPath, "/"))
		return
	}

	for _, suffix := range []string{"/turns", "/events", "/stream", "/pause", "/resume", "/cancel", "/complete"} {
		if !strings.HasSuffix(path, suffix) {
			continue
		}
		name := strings.TrimSuffix(path, suffix)
		switch suffix {
		case "/turns":
			s.handleSessionTurns(w, r, name)
		case "/events":
			s.handleSessionEvents(w, r, name)
		case "/stream":
			s.handleSessionStream(w, r, name)
		default:
			s.handleSessionControl(w, r, name, strings.TrimPrefix(suffix, "/"))
		}
		return
	}

	name := path
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.Sessions.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("session %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseSessionManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.Sessions.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("session %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if obj.Metadata.Name != name {
			http.Error(w, "Session rename is not supported", http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.Sessions.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("Session", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.Sessions.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("Session", name, "deleted", map[string]any{
			"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)},
		})
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSessionTurns(w http.ResponseWriter, r *http.Request, name string) {
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		turns, err := s.stores.Sessions.ListTurns(r.Context(), key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": turns})
	case http.MethodPost:
		var req sessionTurnRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid turn body", http.StatusBadRequest)
			return
		}
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if idempotencyKey == "" {
			http.Error(w, "Idempotency-Key header is required", http.StatusBadRequest)
			return
		}
		turn, events, created, err := s.stores.Sessions.EnqueueTurn(r.Context(), key, resources.SessionTurn{
			Content:        req.Content,
			Interrupt:      req.Interrupt,
			IdempotencyKey: idempotencyKey,
		})
		if err != nil {
			status := http.StatusConflict
			if strings.Contains(strings.ToLower(err.Error()), "required") {
				status = http.StatusBadRequest
			}
			http.Error(w, err.Error(), status)
			return
		}
		s.publishSessionEvents(events)
		if session, ok, getErr := s.stores.Sessions.Get(r.Context(), key); getErr == nil && ok {
			s.publishResourceEvent("Session", session.Metadata.Name, "updated", session)
		}
		status := http.StatusAccepted
		if !created {
			status = http.StatusOK
		}
		writeJSON(w, status, map[string]any{"turn": turn, "created": created})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSessionEvents(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	after, err := parseSessionEventCursor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 {
			http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
			return
		}
	}
	events, err := s.stores.Sessions.ListEvents(r.Context(), scopedNameForRequest(r, name), after, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": events})
}

func (s *Server) handleSessionControl(w http.ResponseWriter, r *http.Request, name, action string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	events, session, err := s.stores.Sessions.ApplyControl(r.Context(), scopedNameForRequest(r, name), action, body.Reason)
	if err != nil {
		status := http.StatusConflict
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	s.publishSessionEvents(events)
	s.publishResourceEvent("Session", session.Metadata.Name, "updated", session)
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleSessionStream(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	after, err := parseSessionEventCursor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ctx, cancel, ok := s.acquireWatchStream(w, r)
	if !ok {
		return
	}
	defer cancel()

	key := scopedNameForRequest(r, name)
	if _, exists, err := s.stores.Sessions.Get(ctx, key); err != nil {
		writeStoreError(w, err)
		return
	} else if !exists {
		http.Error(w, fmt.Sprintf("session %q not found", name), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	namespace := requestNamespace(r)
	var wake <-chan eventbus.Event
	if s.bus != nil {
		wake = s.bus.Subscribe(ctx, eventbus.Filter{
			SinceID:   s.bus.LatestID(),
			Kind:      "Session",
			Name:      name,
			Namespace: namespace,
		})
	}
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	heartbeat := time.NewTicker(sessionStreamHeartbeat)
	defer heartbeat.Stop()

	for {
		events, listErr := s.stores.Sessions.ListEvents(ctx, key, after, 500)
		if listErr != nil {
			writeSessionSSE(w, flusher, resources.SessionEvent{
				Type:      resources.SessionEventError,
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Payload:   map[string]any{"message": listErr.Error()},
			})
			return
		}
		for _, evt := range events {
			if err := writeSessionSSE(w, flusher, evt); err != nil {
				return
			}
			after = evt.Sequence
		}
		session, exists, getErr := s.stores.Sessions.Get(ctx, key)
		if getErr != nil || !exists {
			return
		}
		if resources.IsTerminalSessionPhase(session.Status.Phase) && after >= session.Status.LastEventSequence {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-poll.C:
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func parseSessionEventCursor(r *http.Request) (uint64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("after"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("since"))
	}
	if raw == "" {
		raw = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid event cursor %q", raw)
	}
	return value, nil
}

func writeSessionSSE(w http.ResponseWriter, flusher http.Flusher, evt resources.SessionEvent) error {
	raw, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	if evt.Sequence > 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", evt.Sequence); err != nil {
			return err
		}
	}
	eventType := strings.TrimSpace(evt.Type)
	if eventType == "" {
		eventType = "session.event"
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, raw); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func (s *Server) publishSessionEvents(events []resources.SessionEvent) {
	if s == nil || s.bus == nil {
		return
	}
	for _, evt := range events {
		s.bus.Publish(eventbus.Event{
			Source:    "session-runtime",
			Type:      evt.Type,
			Kind:      "Session",
			Name:      evt.SessionName,
			Namespace: resources.NormalizeNamespace(evt.Namespace),
			Action:    evt.Type,
			Data:      evt,
		})
	}
}
