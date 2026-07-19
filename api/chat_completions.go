package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
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

var (
	chatModelOutputPrefixRegex = regexp.MustCompile(`^step=\d+\s+model_output=`)
	chatAgentMessageContentKey = regexp.MustCompile(`^agent\.(\d+)\.message_content$`)
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
	taskName := "chatcmpl-" + randomHexID(8)
	now := time.Now().UTC()

	task := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Name:      taskName,
			Namespace: resources.NormalizeNamespace(system.Metadata.Namespace),
			Labels: map[string]string{
				chatCompletionsLabel: chatCompletionsCreatedBy,
			},
			Annotations: map[string]string{
				"orloj.dev/created-by":         chatCompletionsCreatedBy,
				"orloj.dev/chat-completion-id": completionID,
			},
		},
		Spec: resources.TaskSpec{
			System: system.Metadata.Name,
			Mode:   "run",
			Input: map[string]string{
				"topic": prompt,
			},
		},
		Status: resources.TaskStatus{
			Phase:     "Pending",
			StartedAt: now.Format(time.RFC3339Nano),
		},
	}
	telemetry.InjectTraceContext(r.Context(), task.Metadata.Annotations)

	created, err := s.stores.Tasks.Upsert(r.Context(), task)
	if err != nil {
		writeChatCompletionError(w, http.StatusServiceUnavailable, "failed to create task", "server_error")
		return
	}
	s.publishResourceEvent("Task", created.Metadata.Name, "created", created)

	if req.Stream {
		s.streamChatCompletionTask(w, r, created, completionID, model, now.Unix())
		return
	}

	finished, err := s.waitForChatCompletionTask(r.Context(), created, nil)
	if err != nil {
		if r.Context().Err() != nil {
			writeChatCompletionError(w, http.StatusGatewayTimeout, "request cancelled or timed out while waiting for task", "server_error")
			return
		}
		writeChatCompletionError(w, http.StatusInternalServerError, err.Error(), "server_error")
		return
	}

	content, mapErr := chatCompletionContentFromTask(finished)
	if mapErr != nil {
		status := http.StatusInternalServerError
		errType := "server_error"
		phase := strings.TrimSpace(finished.Status.Phase)
		if phase == "WaitingApproval" {
			status = http.StatusConflict
			errType = "invalid_request_error"
		}
		writeChatCompletionError(w, status, mapErr.Error(), errType)
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

func (s *Server) waitForChatCompletionTask(
	ctx context.Context,
	task resources.Task,
	heartbeat func(),
) (resources.Task, error) {
	key := store.ScopedName(task.Metadata.Namespace, task.Metadata.Name)
	namespace := resources.NormalizeNamespace(task.Metadata.Namespace)

	var events <-chan eventbus.Event
	if s.bus != nil {
		since := s.bus.LatestID()
		events = s.bus.Subscribe(ctx, eventbus.Filter{
			SinceID:   since,
			Kind:      "Task",
			Name:      task.Metadata.Name,
			Namespace: namespace,
		})
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var heartbeatC <-chan time.Time
	var heartbeatTicker *time.Ticker
	if heartbeat != nil {
		heartbeatTicker = time.NewTicker(chatCompletionHeartbeat)
		heartbeatC = heartbeatTicker.C
		defer heartbeatTicker.Stop()
	}

	for {
		current, ok, err := s.stores.Tasks.Get(ctx, key)
		if err != nil {
			return resources.Task{}, fmt.Errorf("failed to load task: %w", err)
		}
		if !ok {
			return resources.Task{}, fmt.Errorf("task %q not found", task.Metadata.Name)
		}
		if isChatCompletionTerminal(current.Status.Phase) {
			return current, nil
		}

		if events == nil {
			select {
			case <-ctx.Done():
				return resources.Task{}, ctx.Err()
			case <-ticker.C:
			case <-heartbeatC:
				heartbeat()
			}
			continue
		}

		select {
		case <-ctx.Done():
			return resources.Task{}, ctx.Err()
		case _, ok := <-events:
			if !ok {
				if ctx.Err() != nil {
					return resources.Task{}, ctx.Err()
				}
				events = nil
			}
		case <-ticker.C:
			// periodic re-check in case an update was missed between subscribe and first get
		case <-heartbeatC:
			heartbeat()
		}
	}
}

func isChatCompletionTerminal(phase string) bool {
	switch strings.TrimSpace(phase) {
	case "Succeeded", "Failed", "DeadLetter", "WaitingApproval":
		return true
	default:
		return false
	}
}

func chatCompletionContentFromTask(task resources.Task) (string, error) {
	phase := strings.TrimSpace(task.Status.Phase)
	switch phase {
	case "Succeeded":
		content := flattenChatCompletionOutput(task.Status.Output)
		if strings.TrimSpace(content) == "" {
			return "", fmt.Errorf("task %q succeeded but produced no assistant content", task.Metadata.Name)
		}
		return content, nil
	case "WaitingApproval":
		return "", fmt.Errorf("task %q is waiting for approval and cannot complete a chat completion", task.Metadata.Name)
	case "Failed", "DeadLetter":
		msg := strings.TrimSpace(task.Status.LastError)
		if msg == "" {
			msg = fmt.Sprintf("task %q ended in phase %s", task.Metadata.Name, phase)
		}
		return "", fmt.Errorf("%s", msg)
	default:
		return "", fmt.Errorf("task %q ended in unexpected phase %q", task.Metadata.Name, phase)
	}
}

func flattenChatCompletionOutput(output map[string]string) string {
	if len(output) == 0 {
		return ""
	}
	if v, ok := output["last_output"]; ok && strings.TrimSpace(v) != "" {
		v = chatModelOutputPrefixRegex.ReplaceAllString(v, "")
		return resources.UnwrapFencedCodeBlock(strings.TrimSpace(v))
	}
	if v, ok := output["response"]; ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v, ok := output["result"]; ok && strings.TrimSpace(v) != "" && v != "executed" {
		return strings.TrimSpace(v)
	}

	bestIdx := -1
	bestContent := ""
	for key, value := range output {
		match := chatAgentMessageContentKey.FindStringSubmatch(key)
		if match == nil {
			continue
		}
		idx, err := strconv.Atoi(match[1])
		if err != nil || strings.TrimSpace(value) == "" {
			continue
		}
		if idx >= bestIdx {
			bestIdx = idx
			bestContent = strings.TrimSpace(value)
		}
	}
	return bestContent
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

func (s *Server) streamChatCompletionTask(
	w http.ResponseWriter,
	r *http.Request,
	task resources.Task,
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

	finished, err := s.waitForChatCompletionTask(r.Context(), task, func() {
		_, _ = fmt.Fprint(w, ": keep-alive\n\n")
		flusher.Flush()
	})
	if err != nil {
		message := err.Error()
		if r.Context().Err() != nil {
			message = "request cancelled or timed out while waiting for task"
		}
		writeChatCompletionSSE(w, flusher, chatCompletionErrorBody{
			Error: chatCompletionError{Message: message, Type: "server_error"},
		})
		writeChatCompletionDone(w, flusher)
		return
	}

	content, err := chatCompletionContentFromTask(finished)
	if err != nil {
		writeChatCompletionSSE(w, flusher, chatCompletionErrorBody{
			Error: chatCompletionError{Message: err.Error(), Type: "server_error"},
		})
		writeChatCompletionDone(w, flusher)
		return
	}

	writeChatCompletionChunk(w, flusher, chatCompletionResponse{
		ID:      completionID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []chatCompletionChoice{{
			Index: 0,
			Delta: &chatCompletionMsgOut{Content: content},
		}},
	})
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
