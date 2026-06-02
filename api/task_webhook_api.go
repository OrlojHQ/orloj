package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
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
		items, cont, err := fetchListPage(r.Context(), r, s.stores.TaskWebhooks.ListCursor, func(item resources.TaskWebhook) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.TaskWebhookList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
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
		existing, ok, err := s.stores.TaskWebhooks.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) { return }
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.TaskWebhooks.Upsert(r.Context(), obj)
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
		obj, ok, err := s.stores.TaskWebhooks.Get(r.Context(), key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("taskwebhook %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.TaskWebhooks.Delete(r.Context(), key); err != nil {
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
		current, ok, err := s.stores.TaskWebhooks.Get(r.Context(), key)
		if writeStoreFetchError(w, err) { return }
		if !ok {
			http.Error(w, fmt.Sprintf("taskwebhook %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		if bodyKey != key {
			obj, err = s.stores.TaskWebhooks.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.TaskWebhooks.Upsert(r.Context(), obj)
		}
		if err != nil {
			if errors.Is(err, store.ErrResourceAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
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

	hook, ok := s.findTaskWebhookByEndpointID(r.Context(), endpointID)
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

	secretValue, err := s.resolveWebhookSecret(r.Context(), hook)
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

	var eventID string
	if h := strings.TrimSpace(hook.Spec.Idempotency.EventIDHeader); h != "" {
		eventID = strings.TrimSpace(r.Header.Get(h))
	}
	if eventID == "" && hook.Spec.Idempotency.EventIDFromBody != "" {
		eventID = extractEventIDFromBody(body, hook.Spec.Idempotency.EventIDFromBody)
	}
	if eventID == "" {
		s.recordTaskWebhookDeliveryResult(hook, "", "", "missing_event_id", true, false, "missing event id")
		http.Error(w, "missing event id", http.StatusBadRequest)
		return
	}

	// Pre-check: fast path for duplicates without creating a task.
	if taskName, duplicate, err := s.stores.WebhookDedupe.Get(r.Context(), endpointID, eventID, now); err != nil {
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

	runTask, runErr := s.createTaskFromWebhook(r.Context(), hook, eventID, body, now)
	if runErr != nil {
		s.recordTaskWebhookDeliveryResult(hook, eventID, "", "task_create_error", true, false, runErr.Error())
		http.Error(w, "webhook task creation failed", http.StatusBadRequest)
		return
	}
	// Atomic dedup insert: protects against concurrent requests that both
	// pass the pre-check above. If another request inserted first, this
	// returns the existing task name and we treat it as a duplicate.
	window := time.Duration(hook.Spec.Idempotency.DedupeWindowSeconds) * time.Second
	if existingTask, isDup, err := s.stores.WebhookDedupe.TryInsert(r.Context(), endpointID, eventID, runTask, now.Add(window), now); err != nil {
		s.recordTaskWebhookDeliveryResult(hook, eventID, runTask, "dedupe_store_error", true, false, err.Error())
		http.Error(w, "failed to process webhook", http.StatusInternalServerError)
		return
	} else if isDup {
		s.recordTaskWebhookDeliveryResult(hook, eventID, existingTask, "duplicate", false, true, "")
		writeJSON(w, http.StatusAccepted, webhookDeliveryResponse{
			Accepted:  true,
			Duplicate: true,
			EventID:   eventID,
			Task:      existingTask,
			Message:   "duplicate delivery",
		})
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

func (s *Server) findTaskWebhookByEndpointID(ctx context.Context, endpointID string) (resources.TaskWebhook, bool) {
	item, ok, err := s.stores.TaskWebhooks.GetByEndpointID(ctx, endpointID)
	if err != nil || !ok {
		return resources.TaskWebhook{}, false
	}
	return item, true
}

func (s *Server) resolveWebhookSecret(ctx context.Context, hook resources.TaskWebhook) ([]byte, error) {
	secretNS, secretName, err := resolveRef(hook.Metadata.Namespace, hook.Spec.Auth.SecretRef)
	if err != nil {
		return nil, err
	}
	secret, ok, err := s.stores.Secrets.Get(ctx, store.ScopedName(secretNS, secretName))
	if err != nil {
		return nil, fmt.Errorf("secret %q lookup failed: %w", hook.Spec.Auth.SecretRef, err)
	}
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
	profile := strings.ToLower(strings.TrimSpace(hook.Spec.Auth.Profile))

	if profile == "shared_token" {
		return verifySharedToken(hook, r, secret)
	}

	sigHeaderName := strings.TrimSpace(hook.Spec.Auth.SignatureHeader)
	if sigHeaderName == "" {
		return fmt.Errorf("signature header is empty")
	}
	rawHeaderValue := strings.TrimSpace(r.Header.Get(sigHeaderName))
	if rawHeaderValue == "" {
		return fmt.Errorf("missing signature header %s", sigHeaderName)
	}

	received, timestamp, err := extractSignatureAndTimestamp(hook.Spec.Auth, rawHeaderValue, r)
	if err != nil {
		return err
	}

	payload, err := buildHMACPayload(hook.Spec.Auth, profile, body, timestamp, now)
	if err != nil {
		return err
	}

	hashFunc := resolveHMACAlgorithm(hook.Spec.Auth, profile)
	mac := hmac.New(hashFunc, secret)
	_, _ = mac.Write(payload)
	expected := mac.Sum(nil)

	provided, err := decodeWebhookSignature(received, resolveSignatureEncoding(hook.Spec.Auth, profile))
	if err != nil {
		return err
	}
	if !hmac.Equal(expected, provided) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

func verifySharedToken(hook resources.TaskWebhook, r *http.Request, secret []byte) error {
	headerName := strings.TrimSpace(hook.Spec.Auth.SignatureHeader)
	if headerName == "" {
		return fmt.Errorf("signature header is empty")
	}
	received := r.Header.Get(headerName)
	if received == "" {
		return fmt.Errorf("missing token header %s", headerName)
	}
	if subtle.ConstantTimeCompare([]byte(received), secret) != 1 {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

// extractEventIDFromBody extracts an event ID from a JSON request body using a
// dot-separated field path (e.g. "update_id" or "data.id"). Numeric values are
// converted to their string representation.
func extractEventIDFromBody(body []byte, fieldPath string) string {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}
	parts := strings.Split(fieldPath, ".")
	var current any = obj
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = m[part]
		if !ok {
			return ""
		}
	}
	switch v := current.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func extractSignatureAndTimestamp(auth resources.TaskWebhookAuthSpec, rawHeaderValue string, r *http.Request) (signature string, timestamp string, err error) {
	headerFormat := strings.ToLower(strings.TrimSpace(auth.HeaderFormat))

	if headerFormat == "kv_pairs" {
		parsed := parseKVPairsHeader(rawHeaderValue)
		sigKey := strings.TrimSpace(auth.SignatureKey)
		if sigKey == "" {
			return "", "", fmt.Errorf("signature_key is required for kv_pairs header_format")
		}
		signature = strings.TrimSpace(parsed[sigKey])
		if signature == "" {
			return "", "", fmt.Errorf("signature key %q not found in header", sigKey)
		}
		tsKey := strings.TrimSpace(auth.TimestampKey)
		if tsKey != "" {
			timestamp = strings.TrimSpace(parsed[tsKey])
		}
		return signature, timestamp, nil
	}

	// plain header format (default)
	received := rawHeaderValue
	prefix := strings.TrimSpace(auth.SignaturePrefix)
	if prefix != "" {
		trimmed, ok := trimCaseInsensitivePrefix(received, prefix)
		if !ok {
			return "", "", fmt.Errorf("invalid signature prefix")
		}
		received = trimmed
	}
	received = strings.TrimSpace(received)
	if received == "" {
		return "", "", fmt.Errorf("signature is empty")
	}

	tsHeader := strings.TrimSpace(auth.TimestampHeader)
	if tsHeader != "" {
		timestamp = strings.TrimSpace(r.Header.Get(tsHeader))
		if timestamp == "" {
			return "", "", fmt.Errorf("missing timestamp header %s", tsHeader)
		}
	}
	return received, timestamp, nil
}

func parseKVPairsHeader(value string) map[string]string {
	result := make(map[string]string)
	for _, pair := range strings.Split(value, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result
}

func buildHMACPayload(auth resources.TaskWebhookAuthSpec, profile string, body []byte, timestamp string, now time.Time) ([]byte, error) {
	format := strings.ToLower(strings.TrimSpace(auth.PayloadFormat))

	// Legacy profiles map to specific payload formats.
	if format == "" {
		switch profile {
		case "github":
			format = "body"
		case "generic":
			format = "timestamp_dot_body"
		default:
			format = "body"
		}
	}

	switch format {
	case "body":
		return body, nil

	case "timestamp_dot_body":
		if timestamp == "" {
			return nil, fmt.Errorf("missing timestamp for payload construction")
		}
		if err := validateTimestampSkew(timestamp, auth.MaxSkewSeconds, now); err != nil {
			return nil, err
		}
		return []byte(timestamp + "." + string(body)), nil

	case "prefix_timestamp_body":
		if timestamp == "" {
			return nil, fmt.Errorf("missing timestamp for payload construction")
		}
		if err := validateTimestampSkew(timestamp, auth.MaxSkewSeconds, now); err != nil {
			return nil, err
		}
		sep := auth.PayloadSeparator
		if sep == "" {
			sep = "."
		}
		pfx := auth.PayloadPrefix
		if pfx != "" {
			return []byte(pfx + sep + timestamp + sep + string(body)), nil
		}
		return []byte(timestamp + sep + string(body)), nil

	default:
		return nil, fmt.Errorf("unsupported payload_format %q", format)
	}
}

func resolveHMACAlgorithm(auth resources.TaskWebhookAuthSpec, profile string) func() hash.Hash {
	algo := strings.ToLower(strings.TrimSpace(auth.Algorithm))
	if algo == "" {
		// Legacy profiles always use sha256.
		return sha256.New
	}
	switch algo {
	case "sha1":
		return sha1.New
	case "sha512":
		return sha512.New
	default:
		return sha256.New
	}
}

func resolveSignatureEncoding(auth resources.TaskWebhookAuthSpec, profile string) string {
	enc := strings.ToLower(strings.TrimSpace(auth.SignatureEncoding))
	if enc == "" {
		return "hex"
	}
	return enc
}

func decodeWebhookSignature(encoded string, encoding string) ([]byte, error) {
	switch encoding {
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			// Try URL-safe and raw variants.
			decoded, err = base64.RawStdEncoding.DecodeString(encoded)
			if err != nil {
				return nil, fmt.Errorf("signature must be valid base64")
			}
		}
		return decoded, nil
	default:
		decoded, err := hex.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("signature must be hex")
		}
		return decoded, nil
	}
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

func (s *Server) createTaskFromWebhook(ctx context.Context, hook resources.TaskWebhook, eventID string, body []byte, now time.Time) (string, error) {
	var templateSpec resources.TaskSpec
	var templateLabels map[string]string
	var runNamespace string

	if hook.Spec.TaskTemplate != nil {
		templateSpec = *hook.Spec.TaskTemplate
		runNamespace = resources.NormalizeNamespace(hook.Metadata.Namespace)
	} else {
		templateNS, templateName, err := resolveRef(hook.Metadata.Namespace, hook.Spec.TaskRef)
		if err != nil {
			return "", err
		}
		templateKey := store.ScopedName(templateNS, templateName)
		template, ok, err := s.stores.Tasks.Get(ctx, templateKey)
		if err != nil {
			return "", fmt.Errorf("task template %q lookup failed: %w", hook.Spec.TaskRef, err)
		}
		if !ok {
			return "", fmt.Errorf("task template %q not found", hook.Spec.TaskRef)
		}
		if !strings.EqualFold(strings.TrimSpace(template.Spec.Mode), "template") {
			return "", fmt.Errorf("task template %q must set spec.mode=template", hook.Spec.TaskRef)
		}
		templateSpec = template.Spec
		templateLabels = template.Metadata.Labels
		runNamespace = template.Metadata.Namespace
	}

	eventIDHash := shortHex(eventID)
	runName := webhookTaskName(hook.Metadata.Name, eventID)
	runKey := store.ScopedName(runNamespace, runName)
	existing, ok, err := s.stores.Tasks.Get(ctx, runKey)
	if err != nil {
		return "", fmt.Errorf("task run %q lookup failed: %w", runKey, err)
	}
	if ok {
		labels := existing.Metadata.Labels
		if labels != nil &&
			strings.EqualFold(strings.TrimSpace(labels[taskWebhookNameLabel]), strings.TrimSpace(hook.Metadata.Name)) &&
			strings.EqualFold(strings.TrimSpace(labels[taskWebhookNamespaceLabel]), resources.NormalizeNamespace(hook.Metadata.Namespace)) {
			return runKey, nil
		}
		return "", fmt.Errorf("webhook run task name conflict for %q", runKey)
	}

	labels := copyStringMap(templateLabels)
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[taskWebhookNameLabel] = hook.Metadata.Name
	labels[taskWebhookNamespaceLabel] = resources.NormalizeNamespace(hook.Metadata.Namespace)
	labels[taskWebhookEventIDLabel] = eventIDHash

	spec := cloneTaskSpecForWebhook(templateSpec)
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
	if _, err := s.stores.Tasks.Upsert(ctx, runTask); err != nil {
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
	ctx := context.Background()
	key := store.ScopedName(namespace, name)
	for i := 0; i < 3; i++ {
		item, ok, err := s.stores.TaskWebhooks.Get(ctx, key)
		if err != nil {
			return resources.TaskWebhook{}, fmt.Errorf("taskwebhook %q lookup failed: %w", name, err)
		}
		if !ok {
			return resources.TaskWebhook{}, fmt.Errorf("taskwebhook %q not found", name)
		}
		mutate(&item.Status)
		if item.Status.ObservedGeneration == 0 {
			item.Status.ObservedGeneration = item.Metadata.Generation
		}
		updated, err := s.stores.TaskWebhooks.Upsert(ctx, item)
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
