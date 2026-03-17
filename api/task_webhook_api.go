package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/store"
)

const (
	taskWebhookNameLabel      = "orloj.dev/task-webhook"
	taskWebhookNamespaceLabel = "orloj.dev/task-webhook-namespace"
	taskWebhookEventIDLabel   = "orloj.dev/webhook-event-id"

	maxWebhookPayloadBytes = int64(1 << 20)
)

type webhookDeliveryResponse struct {
	Accepted  bool   `json:"accepted"`
	Duplicate bool   `json:"duplicate"`
	EventID   string `json:"event_id,omitempty"`
	Task      string `json:"task,omitempty"`
	Message   string `json:"message,omitempty"`
}

func (s *Server) handleTaskWebhooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items := s.stores.TaskWebhooks.List()
		ns, hasNS := namespaceFilter(r)
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hasNS || len(selector) > 0 {
			filtered := make([]resources.TaskWebhook, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, ns, hasNS, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.TaskWebhookList{Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseTaskWebhookManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if existing, ok := s.stores.TaskWebhooks.Get(store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name)); ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.TaskWebhooks.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("TaskWebhook", obj.Metadata.Name)
		s.publishResourceEvent("TaskWebhook", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTaskWebhookByName(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/task-webhooks/"), "/")
	if path == "" {
		http.Error(w, "task webhook name is required", http.StatusBadRequest)
		return
	}
	if path == "watch" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.watchTaskWebhooks(w, r)
		return
	}
	if strings.HasSuffix(path, "/status") {
		name := strings.Trim(strings.TrimSuffix(path, "/status"), "/")
		if name == "" {
			http.Error(w, "task webhook name is required", http.StatusBadRequest)
			return
		}
		s.handleTaskWebhookStatusByName(w, r, name)
		return
	}

	name := path
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok := s.stores.TaskWebhooks.Get(key)
		if !ok {
			http.Error(w, fmt.Sprintf("taskwebhook %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.TaskWebhooks.Delete(key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("TaskWebhook", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseTaskWebhookManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok := s.stores.TaskWebhooks.Get(key)
		if !ok {
			http.Error(w, fmt.Sprintf("taskwebhook %q not found", name), http.StatusNotFound)
			return
		}
		obj.Metadata.Name = name
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		obj, err = s.stores.TaskWebhooks.Upsert(obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("TaskWebhook", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	endpointID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/webhook-deliveries/"), "/")
	if endpointID == "" {
		http.Error(w, "endpoint id is required", http.StatusBadRequest)
		return
	}

	hook, ok := s.findTaskWebhookByEndpointID(endpointID)
	if !ok {
		http.Error(w, "webhook endpoint not found", http.StatusNotFound)
		return
	}

	now := time.Now().UTC()
	if hook.Spec.Suspend {
		s.recordTaskWebhookDeliveryResult(hook, "", "", "suspended", true, false, "webhook is suspended")
		http.Error(w, "webhook is suspended", http.StatusConflict)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookPayloadBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.recordTaskWebhookDeliveryResult(hook, "", "", "invalid_body", true, false, "invalid payload")
		http.Error(w, "invalid webhook payload", http.StatusBadRequest)
		return
	}

	secretValue, err := s.resolveWebhookSecret(hook)
	if err != nil {
		s.recordTaskWebhookDeliveryResult(hook, "", "", "secret_error", true, false, err.Error())
		http.Error(w, "webhook secret resolution failed", http.StatusBadRequest)
		return
	}
	if err := verifyWebhookSignature(hook, r, body, secretValue, now); err != nil {
		s.recordTaskWebhookDeliveryResult(hook, "", "", "signature_invalid", true, false, err.Error())
		http.Error(w, "signature verification failed", http.StatusUnauthorized)
		return
	}

	eventID := strings.TrimSpace(r.Header.Get(strings.TrimSpace(hook.Spec.Idempotency.EventIDHeader)))
	if eventID == "" {
		s.recordTaskWebhookDeliveryResult(hook, "", "", "missing_event_id", true, false, "missing event id header")
		http.Error(w, "missing event id", http.StatusBadRequest)
		return
	}

	if taskName, duplicate, err := s.stores.WebhookDedupe.Get(endpointID, eventID, now); err != nil {
		s.recordTaskWebhookDeliveryResult(hook, eventID, "", "dedupe_error", true, false, err.Error())
		http.Error(w, "failed to process webhook", http.StatusInternalServerError)
		return
	} else if duplicate {
		s.recordTaskWebhookDeliveryResult(hook, eventID, taskName, "duplicate", false, true, "")
		writeJSON(w, http.StatusAccepted, webhookDeliveryResponse{
			Accepted:  true,
			Duplicate: true,
			EventID:   eventID,
			Task:      taskName,
			Message:   "duplicate delivery",
		})
		return
	}

	runTask, runErr := s.createTaskFromWebhook(hook, eventID, body, now)
	if runErr != nil {
		s.recordTaskWebhookDeliveryResult(hook, eventID, "", "task_create_error", true, false, runErr.Error())
		http.Error(w, runErr.Error(), http.StatusBadRequest)
		return
	}
	window := time.Duration(hook.Spec.Idempotency.DedupeWindowSeconds) * time.Second
	if err := s.stores.WebhookDedupe.Put(endpointID, eventID, runTask, now.Add(window)); err != nil {
		s.recordTaskWebhookDeliveryResult(hook, eventID, runTask, "dedupe_store_error", true, false, err.Error())
		http.Error(w, "failed to process webhook", http.StatusInternalServerError)
		return
	}

	s.recordTaskWebhookDeliveryResult(hook, eventID, runTask, "accepted", false, false, "")
	writeJSON(w, http.StatusAccepted, webhookDeliveryResponse{
		Accepted:  true,
		Duplicate: false,
		EventID:   eventID,
		Task:      runTask,
		Message:   "delivery accepted",
	})
}

func (s *Server) findTaskWebhookByEndpointID(endpointID string) (resources.TaskWebhook, bool) {
	for _, item := range s.stores.TaskWebhooks.List() {
		if strings.TrimSpace(item.Status.EndpointID) == strings.TrimSpace(endpointID) {
			return item, true
		}
	}
	return resources.TaskWebhook{}, false
}

func (s *Server) resolveWebhookSecret(hook resources.TaskWebhook) ([]byte, error) {
	secretNS, secretName, err := resolveRef(hook.Metadata.Namespace, hook.Spec.Auth.SecretRef)
	if err != nil {
		return nil, err
	}
	secret, ok := s.stores.Secrets.Get(store.ScopedName(secretNS, secretName))
	if !ok {
		return nil, fmt.Errorf("secret %q not found", hook.Spec.Auth.SecretRef)
	}
	if len(secret.Spec.Data) == 0 {
		return nil, fmt.Errorf("secret %q has no data", hook.Spec.Auth.SecretRef)
	}
	keys := make([]string, 0, len(secret.Spec.Data))
	for key := range secret.Spec.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	encoded := strings.TrimSpace(secret.Spec.Data[keys[0]])
	if encoded == "" {
		return nil, fmt.Errorf("secret %q has empty data", hook.Spec.Auth.SecretRef)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("secret %q value must be base64: %w", hook.Spec.Auth.SecretRef, err)
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("secret %q decoded value is empty", hook.Spec.Auth.SecretRef)
	}
	return decoded, nil
}

func verifyWebhookSignature(hook resources.TaskWebhook, r *http.Request, body []byte, secret []byte, now time.Time) error {
	sigHeaderName := strings.TrimSpace(hook.Spec.Auth.SignatureHeader)
	if sigHeaderName == "" {
		return fmt.Errorf("signature header is empty")
	}
	received := strings.TrimSpace(r.Header.Get(sigHeaderName))
	if received == "" {
		return fmt.Errorf("missing signature header %s", sigHeaderName)
	}

	prefix := strings.TrimSpace(hook.Spec.Auth.SignaturePrefix)
	if prefix != "" {
		trimmed, ok := trimCaseInsensitivePrefix(received, prefix)
		if !ok {
			return fmt.Errorf("invalid signature prefix")
		}
		received = trimmed
	}
	received = strings.TrimSpace(received)
	if received == "" {
		return fmt.Errorf("signature is empty")
	}

	var payload []byte
	if strings.EqualFold(strings.TrimSpace(hook.Spec.Auth.Profile), "github") {
		payload = body
	} else {
		timestampHeader := strings.TrimSpace(hook.Spec.Auth.TimestampHeader)
		timestamp := strings.TrimSpace(r.Header.Get(timestampHeader))
		if timestamp == "" {
			return fmt.Errorf("missing timestamp header %s", timestampHeader)
		}
		if err := validateTimestampSkew(timestamp, hook.Spec.Auth.MaxSkewSeconds, now); err != nil {
			return err
		}
		payload = []byte(timestamp + "." + string(body))
	}

	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	expected := mac.Sum(nil)

	provided, err := hex.DecodeString(received)
	if err != nil {
		return fmt.Errorf("signature must be hex")
	}
	if !hmac.Equal(expected, provided) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

func validateTimestampSkew(timestamp string, maxSkewSeconds int, now time.Time) error {
	t, err := parseWebhookTimestamp(timestamp)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	if maxSkewSeconds <= 0 {
		maxSkewSeconds = 300
	}
	skew := now.Sub(t)
	if skew < 0 {
		skew = -skew
	}
	if skew > time.Duration(maxSkewSeconds)*time.Second {
		return fmt.Errorf("timestamp outside allowed skew")
	}
	return nil
}

func parseWebhookTimestamp(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		// Handle unix millis if value appears to be milliseconds.
		if n > 1_000_000_000_000 {
			return time.UnixMilli(n).UTC(), nil
		}
		return time.Unix(n, 0).UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp format")
}

func trimCaseInsensitivePrefix(value, prefix string) (string, bool) {
	value = strings.TrimSpace(value)
	prefix = strings.TrimSpace(prefix)
	if value == "" || prefix == "" {
		return value, true
	}
	if len(value) < len(prefix) {
		return "", false
	}
	if !strings.EqualFold(value[:len(prefix)], prefix) {
		return "", false
	}
	return value[len(prefix):], true
}

func (s *Server) createTaskFromWebhook(hook resources.TaskWebhook, eventID string, body []byte, now time.Time) (string, error) {
	templateNS, templateName, err := resolveRef(hook.Metadata.Namespace, hook.Spec.TaskRef)
	if err != nil {
		return "", err
	}
	templateKey := store.ScopedName(templateNS, templateName)
	template, ok := s.stores.Tasks.Get(templateKey)
	if !ok {
		return "", fmt.Errorf("task template %q not found", hook.Spec.TaskRef)
	}
	if !strings.EqualFold(strings.TrimSpace(template.Spec.Mode), "template") {
		return "", fmt.Errorf("task template %q must set spec.mode=template", hook.Spec.TaskRef)
	}

	eventIDHash := shortHex(eventID)
	runName := webhookTaskName(hook.Metadata.Name, eventID)
	runNamespace := template.Metadata.Namespace
	runKey := store.ScopedName(runNamespace, runName)
	if existing, ok := s.stores.Tasks.Get(runKey); ok {
		labels := existing.Metadata.Labels
		if labels != nil &&
			strings.EqualFold(strings.TrimSpace(labels[taskWebhookNameLabel]), strings.TrimSpace(hook.Metadata.Name)) &&
			strings.EqualFold(strings.TrimSpace(labels[taskWebhookNamespaceLabel]), resources.NormalizeNamespace(hook.Metadata.Namespace)) {
			return runKey, nil
		}
		return "", fmt.Errorf("webhook run task name conflict for %q", runKey)
	}

	labels := copyStringMap(template.Metadata.Labels)
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[taskWebhookNameLabel] = hook.Metadata.Name
	labels[taskWebhookNamespaceLabel] = resources.NormalizeNamespace(hook.Metadata.Namespace)
	labels[taskWebhookEventIDLabel] = eventIDHash

	spec := cloneTaskSpecForWebhook(template.Spec)
	spec.Mode = "run"
	if spec.Input == nil {
		spec.Input = make(map[string]string)
	}
	spec.Input[strings.TrimSpace(hook.Spec.Payload.InputKey)] = string(body)
	spec.Input["webhook_event_id"] = strings.TrimSpace(eventID)
	spec.Input["webhook_received_at"] = now.UTC().Format(time.RFC3339Nano)
	spec.Input["webhook_source"] = strings.TrimSpace(hook.Spec.Auth.Profile)

	runTask := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Name:      runName,
			Namespace: runNamespace,
			Labels:    labels,
		},
		Spec: spec,
	}
	if _, err := s.stores.Tasks.Upsert(runTask); err != nil {
		return "", err
	}
	s.publishResourceEvent("Task", runTask.Metadata.Name, "created", runTask)
	s.publishTaskWebhookEvent("taskwebhook.triggered", hook, "webhook delivery created run task", map[string]any{
		"event_id": eventID,
		"task":     runKey,
	})
	return runKey, nil
}

func cloneTaskSpecForWebhook(spec resources.TaskSpec) resources.TaskSpec {
	cloned := spec
	cloned.Input = copyStringMap(spec.Input)
	if cloned.Input == nil {
		cloned.Input = make(map[string]string)
	}
	cloned.MessageRetry.NonRetryable = append([]string(nil), spec.MessageRetry.NonRetryable...)
	return cloned
}

func webhookTaskName(webhookName, eventID string) string {
	base := strings.TrimSpace(webhookName)
	if base == "" {
		base = "webhook-task"
	}
	h := shortHex(eventID)
	if len(base) > 42 {
		base = base[:42]
	}
	return base + "-" + h
}

func shortHex(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:8])
}

func resolveRef(defaultNamespace, ref string) (string, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", fmt.Errorf("reference is required")
	}
	if strings.Contains(ref, "/") {
		parts := strings.SplitN(ref, "/", 2)
		ns := resources.NormalizeNamespace(parts[0])
		name := strings.TrimSpace(parts[1])
		if ns == "" || name == "" {
			return "", "", fmt.Errorf("invalid reference %q: expected name or namespace/name", ref)
		}
		return ns, name, nil
	}
	return resources.NormalizeNamespace(defaultNamespace), ref, nil
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func (s *Server) recordTaskWebhookDeliveryResult(hook resources.TaskWebhook, eventID, task, eventType string, rejected bool, duplicate bool, lastError string) {
	reason := strings.ToLower(strings.TrimSpace(eventType))
	deliveryType := "accepted"
	if duplicate {
		deliveryType = "duplicate"
	} else if rejected {
		deliveryType = "rejected"
	}

	updated, err := s.updateTaskWebhookStatus(hook.Metadata.Namespace, hook.Metadata.Name, func(status *resources.TaskWebhookStatus) {
		status.LastDeliveryTime = time.Now().UTC().Format(time.RFC3339Nano)
		status.LastEventID = strings.TrimSpace(eventID)
		if task != "" {
			status.LastTriggeredTask = strings.TrimSpace(task)
		}
		if rejected {
			status.RejectedCount++
		} else if duplicate {
			status.DuplicateCount++
		} else {
			status.AcceptedCount++
		}
		status.LastError = strings.TrimSpace(lastError)
		if status.LastError == "" {
			status.Phase = "Ready"
		} else {
			status.Phase = "Error"
		}
	})
	if err != nil {
		if s.logger != nil {
			s.logger.Printf("task webhook status update failed %s/%s: %v", hook.Metadata.Namespace, hook.Metadata.Name, err)
		}
		return
	}
	msg := "webhook delivery processed"
	if strings.TrimSpace(lastError) != "" {
		msg = strings.TrimSpace(lastError)
	} else if reason != "" {
		msg = reason
	}
	payload := map[string]any{
		"event_id": strings.TrimSpace(eventID),
		"task":     strings.TrimSpace(task),
	}
	if reason != "" {
		payload["reason"] = reason
	}
	s.publishTaskWebhookEvent("taskwebhook.delivery."+deliveryType, updated, msg, payload)
}

func (s *Server) updateTaskWebhookStatus(namespace, name string, mutate func(*resources.TaskWebhookStatus)) (resources.TaskWebhook, error) {
	key := store.ScopedName(namespace, name)
	for i := 0; i < 3; i++ {
		item, ok := s.stores.TaskWebhooks.Get(key)
		if !ok {
			return resources.TaskWebhook{}, fmt.Errorf("taskwebhook %q not found", name)
		}
		mutate(&item.Status)
		if item.Status.ObservedGeneration == 0 {
			item.Status.ObservedGeneration = item.Metadata.Generation
		}
		updated, err := s.stores.TaskWebhooks.Upsert(item)
		if err == nil {
			s.publishResourceEvent("TaskWebhook", updated.Metadata.Name, "status", map[string]any{"metadata": updated.Metadata, "status": updated.Status})
			return updated, nil
		}
		if !store.IsConflict(err) {
			return resources.TaskWebhook{}, err
		}
	}
	return resources.TaskWebhook{}, fmt.Errorf("failed to update task webhook status after retries")
}

func (s *Server) publishTaskWebhookEvent(eventType string, hook resources.TaskWebhook, message string, data map[string]any) {
	if s == nil || s.bus == nil {
		return
	}
	s.bus.Publish(eventbus.Event{
		Source:    "apiserver",
		Type:      strings.TrimSpace(eventType),
		Kind:      "TaskWebhook",
		Name:      strings.TrimSpace(hook.Metadata.Name),
		Namespace: resources.NormalizeNamespace(hook.Metadata.Namespace),
		Action:    strings.TrimSpace(eventType),
		Message:   strings.TrimSpace(message),
		Data:      data,
	})
}
