package api_test

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/crds"
)

type webhookDeliveryPayload struct {
	Accepted  bool   `json:"accepted"`
	Duplicate bool   `json:"duplicate"`
	EventID   string `json:"event_id"`
	Task      string `json:"task"`
	Message   string `json:"message"`
}

func TestTaskWebhookCRUDAndStatusPreconditions(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/task-webhooks", crds.TaskWebhook{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskWebhook",
		Metadata:   crds.ObjectMeta{Name: "build-events"},
		Spec: crds.TaskWebhookSpec{
			TaskRef: "weekly-report-template",
			Auth: crds.TaskWebhookAuthSpec{
				SecretRef: "build-webhook-secret",
			},
		},
	})

	hook := getTaskWebhook(t, server.URL, "build-events", "default")
	if hook.Spec.Auth.Profile != "generic" {
		t.Fatalf("expected generic profile default, got %q", hook.Spec.Auth.Profile)
	}
	if hook.Status.EndpointID == "" || hook.Status.EndpointPath == "" {
		t.Fatalf("expected endpoint id/path to be set in status, got id=%q path=%q", hook.Status.EndpointID, hook.Status.EndpointPath)
	}

	stalePatch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": "0",
		},
		"status": map[string]any{
			"phase": "Ready",
		},
	}
	body, err := json.Marshal(stalePatch)
	if err != nil {
		t.Fatalf("marshal patch failed: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/task-webhooks/build-events/status", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build status request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 409 conflict, got %d body=%s", resp.StatusCode, string(payload))
	}

	okPatch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": hook.Metadata.ResourceVersion,
		},
		"status": map[string]any{
			"phase":             "Ready",
			"lastDeliveryTime":  "2026-03-13T10:00:00Z",
			"lastTriggeredTask": "default/build-events-a1b2c3d4",
		},
	}
	body, err = json.Marshal(okPatch)
	if err != nil {
		t.Fatalf("marshal patch failed: %v", err)
	}
	req, err = http.NewRequest(http.MethodPut, server.URL+"/v1/task-webhooks/build-events/status", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build status request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 status update, got %d body=%s", resp.StatusCode, string(payload))
	}

	deleteReq, err := http.NewRequest(http.MethodDelete, server.URL+"/v1/task-webhooks/build-events", nil)
	if err != nil {
		t.Fatalf("build delete request failed: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete request failed: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusNoContent {
		payload, _ := io.ReadAll(deleteResp.Body)
		t.Fatalf("expected 204 on delete, got %d body=%s", deleteResp.StatusCode, string(payload))
	}
}

func TestTaskWebhookWatchEndpoint(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/task-webhooks", crds.TaskWebhook{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskWebhook",
		Metadata:   crds.ObjectMeta{Name: "watch-build-events"},
		Spec: crds.TaskWebhookSpec{
			TaskRef: "weekly-report-template",
			Auth: crds.TaskWebhookAuthSpec{
				SecretRef: "watch-secret",
			},
		},
	})

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(server.URL + "/v1/task-webhooks/watch?name=watch-build-events")
	if err != nil {
		t.Fatalf("watch request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("watch status=%d body=%s", resp.StatusCode, string(payload))
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", ct)
	}

	reader := bufio.NewReader(resp.Body)
	foundData := false
	for i := 0; i < 10; i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read watch stream failed: %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			foundData = true
			if !strings.Contains(line, "\"type\":\"added\"") {
				t.Fatalf("expected added event, got line: %s", line)
			}
			break
		}
	}
	if !foundData {
		t.Fatal("expected at least one data event from watch stream")
	}
}

func TestWebhookDeliveryGenericAcceptedAndDuplicate(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	secret := "very-secret"
	createWebhookFixtures(t, server.URL, "template-weekly", "incoming-builds", "generic", false, secret)

	hook := getTaskWebhook(t, server.URL, "incoming-builds", "default")
	body := []byte(`{"event":"build.completed","repo":"orloj"}`)
	eventID := "evt-001"
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	signature := signGeneric(secret, timestamp, body)

	status, payload, raw := deliverWebhook(t, server.URL, hook.Status.EndpointPath, body, map[string]string{
		"X-Signature": signature,
		"X-Timestamp": timestamp,
		"X-Event-Id":  eventID,
	})
	if status != http.StatusAccepted {
		t.Fatalf("expected 202 accepted, got %d body=%s", status, raw)
	}
	if !payload.Accepted || payload.Duplicate {
		t.Fatalf("expected accepted=true duplicate=false, got accepted=%t duplicate=%t", payload.Accepted, payload.Duplicate)
	}
	if payload.Task == "" {
		t.Fatalf("expected task in delivery response, body=%s", raw)
	}

	runNS, runName := splitScopedTask(payload.Task)
	runTask := getTask(t, server.URL, runName, runNS)
	if runTask.Spec.Mode != "run" {
		t.Fatalf("expected generated run mode=run, got %q", runTask.Spec.Mode)
	}
	if got := runTask.Spec.Input["webhook_payload"]; got != string(body) {
		t.Fatalf("expected payload input to equal raw body, got %q", got)
	}
	if got := runTask.Spec.Input["webhook_event_id"]; got != eventID {
		t.Fatalf("expected webhook_event_id=%q, got %q", eventID, got)
	}
	if got := runTask.Spec.Input["webhook_source"]; got != "generic" {
		t.Fatalf("expected webhook_source=generic, got %q", got)
	}
	if runTask.Metadata.Labels["orloj.dev/task-webhook"] != "incoming-builds" {
		t.Fatalf("expected task webhook label to be incoming-builds, got labels=%v", runTask.Metadata.Labels)
	}
	if runTask.Metadata.Labels["orloj.dev/task-webhook-namespace"] != "default" {
		t.Fatalf("expected webhook namespace label default, got labels=%v", runTask.Metadata.Labels)
	}
	if runTask.Metadata.Labels["orloj.dev/webhook-event-id"] == "" {
		t.Fatalf("expected webhook event hash label to be set, labels=%v", runTask.Metadata.Labels)
	}

	status, payload, raw = deliverWebhook(t, server.URL, hook.Status.EndpointPath, body, map[string]string{
		"X-Signature": signature,
		"X-Timestamp": timestamp,
		"X-Event-Id":  eventID,
	})
	if status != http.StatusAccepted {
		t.Fatalf("expected duplicate to return 202, got %d body=%s", status, raw)
	}
	if !payload.Accepted || !payload.Duplicate {
		t.Fatalf("expected duplicate delivery response, got accepted=%t duplicate=%t", payload.Accepted, payload.Duplicate)
	}
	if payload.Task != runNS+"/"+runName {
		t.Fatalf("expected duplicate to return same task %s/%s, got %q", runNS, runName, payload.Task)
	}

	hook = getTaskWebhook(t, server.URL, "incoming-builds", "default")
	if hook.Status.AcceptedCount != 1 {
		t.Fatalf("expected acceptedCount=1, got %d", hook.Status.AcceptedCount)
	}
	if hook.Status.DuplicateCount != 1 {
		t.Fatalf("expected duplicateCount=1, got %d", hook.Status.DuplicateCount)
	}
}

func TestWebhookDeliveryGithubPresetAndSignatureRejection(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	secret := "github-secret"
	createWebhookFixtures(t, server.URL, "template-pr", "github-events", "github", false, secret)

	hook := getTaskWebhook(t, server.URL, "github-events", "default")
	body := []byte(`{"action":"opened","pull_request":{"number":12}}`)
	goodSig := signGitHub(secret, body)

	status, payload, raw := deliverWebhook(t, server.URL, hook.Status.EndpointPath, body, map[string]string{
		"X-Hub-Signature-256": goodSig,
		"X-GitHub-Delivery":   "gh-delivery-001",
	})
	if status != http.StatusAccepted {
		t.Fatalf("expected github delivery 202, got %d body=%s", status, raw)
	}
	if !payload.Accepted || payload.Duplicate {
		t.Fatalf("expected github accepted=true duplicate=false, got accepted=%t duplicate=%t", payload.Accepted, payload.Duplicate)
	}

	status, _, raw = deliverWebhook(t, server.URL, hook.Status.EndpointPath, body, map[string]string{
		"X-Hub-Signature-256": "sha256=bad",
		"X-GitHub-Delivery":   "gh-delivery-002",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("expected invalid github signature 401, got %d body=%s", status, raw)
	}

	hook = getTaskWebhook(t, server.URL, "github-events", "default")
	if hook.Status.AcceptedCount != 1 {
		t.Fatalf("expected acceptedCount=1, got %d", hook.Status.AcceptedCount)
	}
	if hook.Status.RejectedCount != 1 {
		t.Fatalf("expected rejectedCount=1, got %d", hook.Status.RejectedCount)
	}
}

func TestWebhookDeliveryTimestampSkewAndSuspended(t *testing.T) {
	t.Run("timestamp skew rejected", func(t *testing.T) {
		server := newTestServer(t)
		defer server.Close()

		secret := "skew-secret"
		createWebhookFixtures(t, server.URL, "template-skew", "incoming-skew", "generic", false, secret)
		hook := getTaskWebhook(t, server.URL, "incoming-skew", "default")

		body := []byte(`{"event":"stale"}`)
		oldTimestamp := strconv.FormatInt(time.Now().UTC().Add(-10*time.Minute).Unix(), 10)
		signature := signGeneric(secret, oldTimestamp, body)

		status, _, raw := deliverWebhook(t, server.URL, hook.Status.EndpointPath, body, map[string]string{
			"X-Signature": signature,
			"X-Timestamp": oldTimestamp,
			"X-Event-Id":  "evt-skew-1",
		})
		if status != http.StatusUnauthorized {
			t.Fatalf("expected 401 for stale timestamp, got %d body=%s", status, raw)
		}

		hook = getTaskWebhook(t, server.URL, "incoming-skew", "default")
		if hook.Status.RejectedCount != 1 {
			t.Fatalf("expected rejectedCount=1, got %d", hook.Status.RejectedCount)
		}
		if !strings.Contains(strings.ToLower(hook.Status.LastError), "timestamp") {
			t.Fatalf("expected lastError to mention timestamp, got %q", hook.Status.LastError)
		}
	})

	t.Run("suspended webhook rejected", func(t *testing.T) {
		server := newTestServer(t)
		defer server.Close()

		secret := "suspended-secret"
		createWebhookFixtures(t, server.URL, "template-suspended", "incoming-suspended", "generic", true, secret)
		hook := getTaskWebhook(t, server.URL, "incoming-suspended", "default")

		body := []byte(`{"event":"blocked"}`)
		timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
		signature := signGeneric(secret, timestamp, body)

		status, _, raw := deliverWebhook(t, server.URL, hook.Status.EndpointPath, body, map[string]string{
			"X-Signature": signature,
			"X-Timestamp": timestamp,
			"X-Event-Id":  "evt-suspended-1",
		})
		if status != http.StatusConflict {
			t.Fatalf("expected 409 for suspended webhook, got %d body=%s", status, raw)
		}

		hook = getTaskWebhook(t, server.URL, "incoming-suspended", "default")
		if hook.Status.RejectedCount != 1 {
			t.Fatalf("expected rejectedCount=1, got %d", hook.Status.RejectedCount)
		}
		if !strings.Contains(strings.ToLower(hook.Status.LastError), "suspend") {
			t.Fatalf("expected lastError to mention suspended, got %q", hook.Status.LastError)
		}
	})
}

func TestWebhookDeliveryRequiresTemplateTask(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	secret := "template-check-secret"
	postJSON(t, server.URL+"/v1/secrets", crds.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata:   crds.ObjectMeta{Name: "template-check-secret"},
		Spec: crds.SecretSpec{
			StringData: map[string]string{"value": secret},
		},
	})
	postJSON(t, server.URL+"/v1/tasks", crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: "not-a-template"},
		Spec: crds.TaskSpec{
			Mode:   "run",
			System: "report-system",
			Input:  map[string]string{"topic": "x"},
		},
	})
	postJSON(t, server.URL+"/v1/task-webhooks", crds.TaskWebhook{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskWebhook",
		Metadata:   crds.ObjectMeta{Name: "enforce-template"},
		Spec: crds.TaskWebhookSpec{
			TaskRef: "not-a-template",
			Auth: crds.TaskWebhookAuthSpec{
				SecretRef: "template-check-secret",
			},
		},
	})

	hook := getTaskWebhook(t, server.URL, "enforce-template", "default")
	body := []byte(`{"event":"run"}`)
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	signature := signGeneric(secret, timestamp, body)

	status, _, raw := deliverWebhook(t, server.URL, hook.Status.EndpointPath, body, map[string]string{
		"X-Signature": signature,
		"X-Timestamp": timestamp,
		"X-Event-Id":  "evt-enforce-1",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-template ref, got %d body=%s", status, raw)
	}
	if !strings.Contains(raw, "spec.mode=template") {
		t.Fatalf("expected template mode error, got body=%s", raw)
	}
}

func createWebhookFixtures(t *testing.T, baseURL, templateName, webhookName, profile string, suspended bool, secretValue string) {
	t.Helper()
	postJSON(t, baseURL+"/v1/secrets", crds.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata:   crds.ObjectMeta{Name: webhookName + "-secret"},
		Spec: crds.SecretSpec{
			StringData: map[string]string{"value": secretValue},
		},
	})
	postJSON(t, baseURL+"/v1/tasks", crds.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   crds.ObjectMeta{Name: templateName},
		Spec: crds.TaskSpec{
			Mode:   "template",
			System: "report-system",
			Input: map[string]string{
				"topic": "webhook-triggered",
			},
		},
	})
	postJSON(t, baseURL+"/v1/task-webhooks", crds.TaskWebhook{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskWebhook",
		Metadata:   crds.ObjectMeta{Name: webhookName},
		Spec: crds.TaskWebhookSpec{
			TaskRef: templateName,
			Suspend: suspended,
			Auth:    crds.TaskWebhookAuthSpec{Profile: profile, SecretRef: webhookName + "-secret"},
		},
	})
}

func getTaskWebhook(t *testing.T, baseURL, name, namespace string) crds.TaskWebhook {
	t.Helper()
	reqURL := fmt.Sprintf("%s/v1/task-webhooks/%s?namespace=%s", baseURL, name, url.QueryEscape(namespace))
	resp, err := http.Get(reqURL)
	if err != nil {
		t.Fatalf("get task webhook failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("get task webhook status=%d body=%s", resp.StatusCode, string(body))
	}
	var hook crds.TaskWebhook
	if err := json.NewDecoder(resp.Body).Decode(&hook); err != nil {
		t.Fatalf("decode task webhook failed: %v", err)
	}
	return hook
}

func deliverWebhook(t *testing.T, baseURL, path string, body []byte, headers map[string]string) (int, webhookDeliveryPayload, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new webhook request failed: %v", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send webhook request failed: %v", err)
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	raw := string(rawBody)
	var out webhookDeliveryPayload
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") && len(rawBody) > 0 {
		_ = json.Unmarshal(rawBody, &out)
	}
	return resp.StatusCode, out, raw
}

func getTask(t *testing.T, baseURL, name, namespace string) crds.Task {
	t.Helper()
	reqURL := fmt.Sprintf("%s/v1/tasks/%s?namespace=%s", baseURL, name, url.QueryEscape(namespace))
	resp, err := http.Get(reqURL)
	if err != nil {
		t.Fatalf("get task failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("get task status=%d body=%s", resp.StatusCode, string(body))
	}
	var task crds.Task
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		t.Fatalf("decode task failed: %v", err)
	}
	return task
}

func splitScopedTask(ref string) (string, string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "default", ""
	}
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
		return parts[0], parts[1]
	}
	return "default", ref
}

func signGeneric(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func signGitHub(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
