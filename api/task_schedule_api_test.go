package api_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/crds"
)

func TestTaskScheduleCRUDAndStatusPreconditions(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/task-schedules", crds.TaskSchedule{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskSchedule",
		Metadata:   crds.ObjectMeta{Name: "hourly"},
		Spec: crds.TaskScheduleSpec{
			TaskRef:  "template-task",
			Schedule: "0 * * * *",
			TimeZone: "UTC",
			Suspend:  false,
		},
	})

	resp, err := http.Get(server.URL + "/v1/task-schedules/hourly")
	if err != nil {
		t.Fatalf("get task schedule failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("get task schedule status=%d body=%s", resp.StatusCode, string(body))
	}
	var schedule crds.TaskSchedule
	if err := json.NewDecoder(resp.Body).Decode(&schedule); err != nil {
		t.Fatalf("decode task schedule failed: %v", err)
	}
	if schedule.Spec.Schedule != "0 * * * *" {
		t.Fatalf("expected schedule 0 * * * *, got %q", schedule.Spec.Schedule)
	}

	patch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": "0",
		},
		"status": map[string]any{
			"phase": "Ready",
		},
	}
	body, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("marshal patch failed: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, server.URL+"/v1/task-schedules/hourly/status", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build status request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	conflictResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	defer conflictResp.Body.Close()
	if conflictResp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(conflictResp.Body)
		t.Fatalf("expected 409 conflict, got %d body=%s", conflictResp.StatusCode, string(b))
	}

	patch = map[string]any{
		"metadata": map[string]any{
			"resourceVersion": schedule.Metadata.ResourceVersion,
		},
		"status": map[string]any{
			"phase":            "Ready",
			"lastScheduleTime": "2026-03-13T10:00:00Z",
			"nextScheduleTime": "2026-03-13T11:00:00Z",
			"activeRuns":       []string{"default/hourly-20260313-1000"},
		},
	}
	body, err = json.Marshal(patch)
	if err != nil {
		t.Fatalf("marshal patch failed: %v", err)
	}
	req, err = http.NewRequest(http.MethodPut, server.URL+"/v1/task-schedules/hourly/status", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build status request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	okResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	defer okResp.Body.Close()
	if okResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(okResp.Body)
		t.Fatalf("status update failed: %d body=%s", okResp.StatusCode, string(b))
	}

	deleteReq, err := http.NewRequest(http.MethodDelete, server.URL+"/v1/task-schedules/hourly", nil)
	if err != nil {
		t.Fatalf("build delete request failed: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete request failed: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(deleteResp.Body)
		t.Fatalf("delete failed: %d body=%s", deleteResp.StatusCode, string(b))
	}
}

func TestTaskScheduleWatchEndpoint(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/task-schedules", crds.TaskSchedule{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskSchedule",
		Metadata:   crds.ObjectMeta{Name: "watch-schedule"},
		Spec: crds.TaskScheduleSpec{
			TaskRef:  "template-task",
			Schedule: "* * * * *",
			TimeZone: "UTC",
		},
	})

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(server.URL + "/v1/task-schedules/watch?name=watch-schedule")
	if err != nil {
		t.Fatalf("watch request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("watch status=%d body=%s", resp.StatusCode, string(body))
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
