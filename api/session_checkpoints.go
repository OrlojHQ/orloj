package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

type sessionCheckpointRewindRequest struct {
	Interrupt bool `json:"interrupt,omitempty"`
	Resume    bool `json:"resume,omitempty"`
}

type sessionCheckpointForkRequest struct {
	Name   string `json:"name"`
	Resume bool   `json:"resume,omitempty"`
}

func (s *Server) handleSessionCheckpoints(
	w http.ResponseWriter,
	r *http.Request,
	sessionName, checkpointPath string,
) {
	if strings.TrimSpace(sessionName) == "" {
		http.Error(w, "session name is required", http.StatusBadRequest)
		return
	}
	key := scopedNameForRequest(r, sessionName)
	parts := splitCheckpointPath(checkpointPath)
	if len(parts) == 0 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		checkpoints, err := s.stores.Sessions.ListCheckpoints(r.Context(), key)
		if err != nil {
			writeSessionCheckpointError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resources.SessionCheckpointList{Items: checkpoints})
		return
	}

	checkpointID := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		checkpoint, found, err := s.stores.Sessions.GetCheckpoint(r.Context(), key, checkpointID)
		if err != nil {
			writeSessionCheckpointError(w, err)
			return
		}
		if !found {
			http.Error(w, fmt.Sprintf("checkpoint %q not found", checkpointID), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, checkpoint)
		return
	}
	if len(parts) != 2 {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	switch parts[1] {
	case "replay":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		result, err := s.stores.Sessions.ReplayCheckpoint(r.Context(), key, checkpointID)
		if err != nil {
			writeSessionCheckpointError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case "rewind":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req sessionCheckpointRewindRequest
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
				http.Error(w, "invalid rewind body", http.StatusBadRequest)
				return
			}
		}
		checkpoint, found, err := s.stores.Sessions.GetCheckpoint(r.Context(), key, checkpointID)
		if err != nil {
			writeSessionCheckpointError(w, err)
			return
		}
		if !found {
			http.Error(w, fmt.Sprintf("checkpoint %q not found", checkpointID), http.StatusNotFound)
			return
		}
		session, event, err := s.stores.Sessions.RewindSession(r.Context(), key, checkpointID, req.Interrupt)
		if err != nil {
			writeSessionCheckpointError(w, err)
			return
		}
		s.publishSessionEvents([]resources.SessionEvent{event})
		if err := s.deleteRewoundSessionTasks(r, session, checkpoint.TurnID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if req.Resume {
			resumeEvents, resumed, resumeErr := s.stores.Sessions.ApplyControl(r.Context(), key, "resume", "resume from checkpoint")
			if resumeErr != nil {
				writeSessionCheckpointError(w, resumeErr)
				return
			}
			s.publishSessionEvents(resumeEvents)
			session = resumed
		}
		s.publishResourceEvent("Session", session.Metadata.Name, "updated", session)
		writeJSON(w, http.StatusOK, map[string]any{
			"session":       session,
			"checkpoint_id": checkpointID,
			"resumed":       req.Resume,
		})
	case "fork":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req sessionCheckpointForkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid fork body", http.StatusBadRequest)
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			http.Error(w, "fork name is required", http.StatusBadRequest)
			return
		}
		session, checkpoint, events, err := s.stores.Sessions.ForkSession(
			r.Context(),
			key,
			checkpointID,
			req.Name,
		)
		if err != nil {
			writeSessionCheckpointError(w, err)
			return
		}
		s.publishSessionEvents(events)
		if req.Resume {
			resumeEvents, resumed, resumeErr := s.stores.Sessions.ApplyControl(
				r.Context(),
				store.ScopedName(session.Metadata.Namespace, session.Metadata.Name),
				"resume",
				"resume forked session",
			)
			if resumeErr != nil {
				writeSessionCheckpointError(w, resumeErr)
				return
			}
			s.publishSessionEvents(resumeEvents)
			session = resumed
		}
		s.publishResourceEvent("Session", session.Metadata.Name, "created", session)
		writeJSON(w, http.StatusCreated, map[string]any{
			"session":    session,
			"checkpoint": checkpoint,
			"resumed":    req.Resume,
		})
	default:
		http.Error(w, "checkpoint action not found", http.StatusNotFound)
	}
}

func (s *Server) deleteRewoundSessionTasks(
	r *http.Request,
	session resources.Session,
	checkpointTurnID string,
) error {
	sessionKey := store.ScopedName(session.Metadata.Namespace, session.Metadata.Name)
	turns, err := s.stores.Sessions.ListTurns(r.Context(), sessionKey)
	if err != nil {
		return err
	}
	affectedTurns := map[string]struct{}{checkpointTurnID: {}}
	for _, turn := range turns {
		if turn.Phase == resources.SessionTurnPhaseCancelled {
			affectedTurns[turn.ID] = struct{}{}
		}
	}
	tasks, err := s.stores.Tasks.List(r.Context())
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.Metadata.Labels["orloj.dev/session"] != session.Metadata.Name ||
			resources.NormalizeNamespace(task.Metadata.Namespace) != resources.NormalizeNamespace(session.Metadata.Namespace) {
			continue
		}
		if _, affected := affectedTurns[task.Metadata.Labels["orloj.dev/turn"]]; !affected {
			continue
		}
		taskKey := store.ScopedName(task.Metadata.Namespace, task.Metadata.Name)
		if err := s.stores.Tasks.Delete(r.Context(), taskKey); err != nil {
			return err
		}
	}
	return nil
}

func splitCheckpointPath(path string) []string {
	raw := strings.Split(strings.Trim(path, "/"), "/")
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		if value := strings.TrimSpace(part); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func writeSessionCheckpointError(w http.ResponseWriter, err error) {
	lower := strings.ToLower(err.Error())
	status := http.StatusInternalServerError
	switch {
	case strings.Contains(lower, "not found"):
		status = http.StatusNotFound
	case strings.Contains(lower, "required"), strings.Contains(lower, "invalid"), strings.Contains(lower, "unsupported"):
		status = http.StatusBadRequest
	case strings.Contains(lower, "active turn"), strings.Contains(lower, "already exists"), strings.Contains(lower, "fence"):
		status = http.StatusConflict
	case strings.Contains(lower, "hash mismatch"), strings.Contains(lower, "lineage"):
		status = http.StatusConflict
	}
	http.Error(w, err.Error(), status)
}
