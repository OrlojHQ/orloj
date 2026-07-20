package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"github.com/OrlojHQ/orloj/telemetry"
)

const (
	chatCompletionsCreatedBy    = "chat-completions"
	chatCompletionsLabel        = "orloj.dev/created-by"
	chatCompletionMaxDuration   = 30 * time.Minute
	chatCompletionMaxConcurrent = 1000
	chatCompletionHeartbeat     = 15 * time.Second
)

type chatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []chatCompletionMessage `json:"messages"`
	Stream   bool                    `json:"stream"`
}

type chatCompletionMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   *chatCompletionUsage   `json:"usage,omitempty"`
}

type chatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      *chatCompletionMsgOut `json:"message,omitempty"`
	Delta        *chatCompletionMsgOut `json:"delta,omitempty"`
	FinishReason *string               `json:"finish_reason"`
}

type chatCompletionMsgOut struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type chatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatCompletionErrorBody struct {
	Error chatCompletionError `json:"error"`
}

type chatCompletionError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeChatCompletionError(w, http.StatusBadRequest, "failed to read request body", "invalid_request_error")
		return
	}

	var req chatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeChatCompletionError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error")
		return
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		writeChatCompletionError(w, http.StatusBadRequest, "model is required", "invalid_request_error")
		return
	}

	prompt, err := flattenChatMessages(req.Messages)
	if err != nil {
		writeChatCompletionError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	if strings.TrimSpace(prompt) == "" {
		writeChatCompletionError(w, http.StatusBadRequest, "messages must include non-empty text content", "invalid_request_error")
		return
	}

	namespace := requestNamespace(r)
	systemKey := store.ScopedName(namespace, model)
	system, ok, err := s.stores.AgentSystems.Get(r.Context(), systemKey)
	if err != nil {
		writeChatCompletionError(w, http.StatusServiceUnavailable, "failed to look up AgentSystem", "server_error")
		return
	}
	if !ok {
		writeChatCompletionError(w, http.StatusNotFound, fmt.Sprintf("AgentSystem %q not found", model), "invalid_request_error")
		return
	}

	ctx, cancel, ok := s.acquireChatCompletion(w, r)
	if !ok {
		return
	}
	defer cancel()
	r = r.WithContext(ctx)

	completionID := "chatcmpl-" + randomHexID(12)
	sessionName := "chatcmpl-" + randomHexID(8)
	now := time.Now().UTC()

	session := resources.Session{
		APIVersion: "orloj.dev/v1",
		Kind:       "Session",
		Metadata: resources.ObjectMeta{
			Name:      sessionName,
			Namespace: resources.NormalizeNamespace(system.Metadata.Namespace),
			Labels: map[string]string{
				chatCompletionsLabel: chatCompletionsCreatedBy,
			},
			Annotations: map[string]string{
				"orloj.dev/created-by":         chatCompletionsCreatedBy,
				"orloj.dev/chat-completion-id": completionID,
			},
		},
		Spec: resources.SessionSpec{
			System:   system.Metadata.Name,
			IdleTTL:  chatCompletionMaxDuration.String(),
			MaxTurns: 1,
		},
		Status: resources.SessionStatus{SystemGeneration: system.Metadata.Generation},
	}
	telemetry.InjectTraceContext(r.Context(), session.Metadata.Annotations)

	created, err := s.stores.Sessions.Upsert(r.Context(), session)
	if err != nil {
		writeChatCompletionError(w, http.StatusServiceUnavailable, "failed to create session", "server_error")
		return
	}
	s.publishResourceEvent("Session", created.Metadata.Name, "created", created)
	if initial, listErr := s.stores.Sessions.ListEvents(r.Context(), store.ScopedName(created.Metadata.Namespace, created.Metadata.Name), 0, 1); listErr == nil {
		s.publishSessionEvents(initial)
	}
	turn, turnEvents, _, err := s.stores.Sessions.EnqueueTurn(
		r.Context(),
		store.ScopedName(created.Metadata.Namespace, created.Metadata.Name),
		resources.SessionTurn{
			Content:        prompt,
			IdempotencyKey: completionID,
		},
	)
	if err != nil {
		writeChatCompletionError(w, http.StatusServiceUnavailable, "failed to create session turn", "server_error")
		return
	}
	s.publishSessionEvents(turnEvents)

	if req.Stream {
		s.streamChatCompletionSession(w, r, created, turn, completionID, model, now.Unix())
		return
	}

	_, content, usage, err := s.waitForChatCompletionSession(r.Context(), created, turn, nil, nil)
	if err != nil {
		if r.Context().Err() != nil {
			writeChatCompletionError(w, http.StatusGatewayTimeout, "request cancelled or timed out while waiting for session", "server_error")
			return
		}
		status := http.StatusInternalServerError
		errType := "server_error"
		if strings.Contains(strings.ToLower(err.Error()), "approval") {
			status = http.StatusConflict
			errType = "invalid_request_error"
		}
		writeChatCompletionError(w, status, err.Error(), errType)
		return
	}

	resp := chatCompletionResponse{
		ID:      completionID,
		Object:  "chat.completion",
		Created: now.Unix(),
		Model:   model,
		Choices: []chatCompletionChoice{{
			Index: 0,
			Message: &chatCompletionMsgOut{
				Role:    "assistant",
				Content: content,
			},
			FinishReason: stringPtr("stop"),
		}},
		Usage: usage,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) acquireChatCompletion(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, bool) {
	if s != nil && chatCompletionMaxConcurrent > 0 {
		current := s.chatCompletionCount.Add(1)
		if int(current) > chatCompletionMaxConcurrent {
			s.chatCompletionCount.Add(-1)
			writeChatCompletionError(w, http.StatusServiceUnavailable, "too many concurrent chat completions", "server_error")
			return nil, nil, false
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), chatCompletionMaxDuration)
	if s != nil && chatCompletionMaxConcurrent > 0 {
		originalCancel := cancel
		cancel = func() {
			originalCancel()
			s.chatCompletionCount.Add(-1)
		}
	}
	return ctx, cancel, true
}

func (s *Server) waitForChatCompletionSession(
	ctx context.Context,
	session resources.Session,
	turn resources.SessionTurn,
	heartbeat func(),
	onEvent func(resources.SessionEvent) error,
) (resources.Session, string, *chatCompletionUsage, error) {
	key := store.ScopedName(session.Metadata.Namespace, session.Metadata.Name)
	namespace := resources.NormalizeNamespace(session.Metadata.Namespace)
	var wake <-chan eventbus.Event
	if s.bus != nil {
		wake = s.bus.Subscribe(ctx, eventbus.Filter{
			SinceID:   s.bus.LatestID(),
			Kind:      "Session",
			Name:      session.Metadata.Name,
			Namespace: namespace,
		})
	}
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	var heartbeatC <-chan time.Time
	var heartbeatTicker *time.Ticker
	if heartbeat != nil {
		heartbeatTicker = time.NewTicker(chatCompletionHeartbeat)
		heartbeatC = heartbeatTicker.C
		defer heartbeatTicker.Stop()
	}

	var cursor uint64
	var content string
	var usage *chatCompletionUsage
	for {
		events, err := s.stores.Sessions.ListEvents(ctx, key, cursor, 500)
		if err != nil {
			return resources.Session{}, "", nil, fmt.Errorf("failed to load session events: %w", err)
		}
		for _, evt := range events {
			cursor = evt.Sequence
			if evt.TurnID != "" && evt.TurnID != turn.ID {
				continue
			}
			if evt.Type == resources.SessionEventMessageCompleted && evt.MessageID == turn.AssistantMessageID {
				if value, ok := evt.Payload["content"].(string); ok {
					content = strings.TrimSpace(value)
				}
				usage = chatCompletionUsageFromSessionEvent(evt)
			}
			if onEvent != nil {
				if err := onEvent(evt); err != nil {
					return resources.Session{}, "", nil, err
				}
			}
		}

		current, ok, err := s.stores.Sessions.Get(ctx, key)
		if err != nil {
			return resources.Session{}, "", nil, fmt.Errorf("failed to load session: %w", err)
		}
		if !ok {
			return resources.Session{}, "", nil, fmt.Errorf("session %q not found", session.Metadata.Name)
		}
		if current.Status.CompletedTurns > 0 || strings.EqualFold(current.Status.Phase, resources.SessionPhaseCompleted) {
			if content == "" {
				return current, "", usage, fmt.Errorf("session %q completed but produced no assistant content", session.Metadata.Name)
			}
			return current, content, usage, nil
		}
		if strings.EqualFold(current.Status.Phase, resources.SessionPhaseWaitingApproval) {
			return current, "", usage, fmt.Errorf("session %q is waiting for approval", session.Metadata.Name)
		}
		if resources.IsTerminalSessionPhase(current.Status.Phase) {
			message := strings.TrimSpace(current.Status.LastError)
			if message == "" {
				message = fmt.Sprintf("session %q ended in phase %s", session.Metadata.Name, current.Status.Phase)
			}
			return current, "", usage, fmt.Errorf("%s", message)
		}

		select {
		case <-ctx.Done():
			return resources.Session{}, "", usage, ctx.Err()
		case <-wake:
		case <-poll.C:
		case <-heartbeatC:
			heartbeat()
		}
	}
}

func chatCompletionUsageFromSessionEvent(evt resources.SessionEvent) *chatCompletionUsage {
	raw, ok := evt.Payload["usage"].(map[string]any)
	if !ok || raw == nil {
		return nil
	}
	asInt := func(key string) int {
		switch value := raw[key].(type) {
		case int:
			return value
		case int64:
			return int(value)
		case float64:
			return int(value)
		case json.Number:
			parsed, _ := strconv.Atoi(value.String())
			return parsed
		default:
			return 0
		}
	}
	usage := &chatCompletionUsage{
		PromptTokens:     asInt("input_tokens"),
		CompletionTokens: asInt("output_tokens"),
		TotalTokens:      asInt("total_tokens"),
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.TotalTokens == 0 {
		return nil
	}
	return usage
}

func flattenChatMessages(messages []chatCompletionMessage) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("messages is required")
	}
	var parts []string
	for i, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "system", "user", "assistant":
		case "":
			return "", fmt.Errorf("messages[%d].role is required", i)
		default:
			return "", fmt.Errorf("messages[%d].role %q is not supported", i, msg.Role)
		}
		text, err := chatMessageText(msg.Content)
		if err != nil {
			return "", fmt.Errorf("messages[%d].content: %w", i, err)
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", role, text))
	}
	return strings.Join(parts, "\n"), nil
}

func chatMessageText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("must be a string")
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, nil
	}
	return "", fmt.Errorf("must be a string (multimodal content parts are not supported)")
}

func writeChatCompletionError(w http.ResponseWriter, status int, message, errType string) {
	writeJSON(w, status, chatCompletionErrorBody{
		Error: chatCompletionError{
			Message: message,
			Type:    errType,
		},
	})
}

func (s *Server) streamChatCompletionSession(
	w http.ResponseWriter,
	r *http.Request,
	session resources.Session,
	turn resources.SessionTurn,
	completionID string,
	model string,
	created int64,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeChatCompletionError(w, http.StatusInternalServerError, "streaming is not supported", "server_error")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	writeChatCompletionChunk(w, flusher, chatCompletionResponse{
		ID:      completionID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []chatCompletionChoice{{
			Index: 0,
			Delta: &chatCompletionMsgOut{Role: "assistant"},
		}},
	})

	sentContent := false
	var streamedContent strings.Builder
	_, content, usage, err := s.waitForChatCompletionSession(
		r.Context(),
		session,
		turn,
		func() {
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		},
		func(evt resources.SessionEvent) error {
			if evt.TurnID != "" && evt.TurnID != turn.ID {
				return nil
			}
			switch evt.Type {
			case resources.SessionEventMessageDelta:
				if evt.MessageID != "" && evt.MessageID != turn.AssistantMessageID {
					return nil
				}
				delta, _ := evt.Payload["delta"].(string)
				if delta == "" {
					return nil
				}
				sentContent = true
				streamedContent.WriteString(delta)
				writeChatCompletionChunk(w, flusher, chatCompletionResponse{
					ID:      completionID,
					Object:  "chat.completion.chunk",
					Created: created,
					Model:   model,
					Choices: []chatCompletionChoice{{
						Index: 0,
						Delta: &chatCompletionMsgOut{Content: delta},
					}},
				})
			case resources.SessionEventMessageReset:
				if sentContent {
					return fmt.Errorf("session execution restarted after partial content was streamed")
				}
			}
			return nil
		},
	)
	if err != nil {
		message := err.Error()
		if r.Context().Err() != nil {
			message = "request cancelled or timed out while waiting for session"
		}
		writeChatCompletionSSE(w, flusher, chatCompletionErrorBody{
			Error: chatCompletionError{Message: message, Type: "server_error"},
		})
		writeChatCompletionDone(w, flusher)
		return
	}
	remaining := content
	if sentContent {
		emitted := streamedContent.String()
		if !strings.HasPrefix(content, emitted) {
			writeChatCompletionSSE(w, flusher, chatCompletionErrorBody{
				Error: chatCompletionError{
					Message: "durable completed content diverged from streamed model output",
					Type:    "server_error",
				},
			})
			writeChatCompletionDone(w, flusher)
			return
		}
		remaining = strings.TrimPrefix(content, emitted)
	}
	if remaining != "" {
		writeChatCompletionChunk(w, flusher, chatCompletionResponse{
			ID:      completionID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   model,
			Choices: []chatCompletionChoice{{
				Index: 0,
				Delta: &chatCompletionMsgOut{Content: remaining},
			}},
		})
	}
	writeChatCompletionChunk(w, flusher, chatCompletionResponse{
		ID:      completionID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []chatCompletionChoice{{
			Index:        0,
			Delta:        &chatCompletionMsgOut{},
			FinishReason: stringPtr("stop"),
		}},
		Usage: usage,
	})
	writeChatCompletionDone(w, flusher)
}

func writeChatCompletionChunk(w http.ResponseWriter, flusher http.Flusher, chunk chatCompletionResponse) {
	writeChatCompletionSSE(w, flusher, chunk)
}

func writeChatCompletionSSE(w http.ResponseWriter, flusher http.Flusher, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func writeChatCompletionDone(w http.ResponseWriter, flusher http.Flusher) {
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func stringPtr(value string) *string {
	return &value
}

func randomHexID(nbytes int) string {
	if nbytes <= 0 {
		nbytes = 8
	}
	buf := make([]byte, nbytes)
	if _, err := rand.Read(buf); err != nil {
		// Extremely unlikely; fall back to timestamp-derived id.
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
