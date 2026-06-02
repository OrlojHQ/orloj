package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

func TestAgentRoleCRUDAndNamespaceScoping(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/agent-roles?namespace=team-a", resources.AgentRole{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentRole",
		Metadata: resources.ObjectMeta{
			Name:      "analyst",
			Namespace: "team-a",
		},
		Spec: resources.AgentRoleSpec{Permissions: []string{"tool:web_search:invoke"}},
	})
	postJSON(t, server.URL+"/v1/agent-roles?namespace=team-b", resources.AgentRole{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentRole",
		Metadata: resources.ObjectMeta{
			Name:      "analyst",
			Namespace: "team-b",
		},
		Spec: resources.AgentRoleSpec{Permissions: []string{"tool:db_read:invoke"}},
	})

	resp, err := http.Get(server.URL + "/v1/agent-roles/analyst?namespace=team-b")
	if err != nil {
		t.Fatalf("get namespaced role failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
	}
	var role resources.AgentRole
	if err := json.NewDecoder(resp.Body).Decode(&role); err != nil {
		t.Fatalf("decode role failed: %v", err)
	}
	if role.Metadata.Namespace != "team-b" {
		t.Fatalf("expected team-b role, got %q", role.Metadata.Namespace)
	}
	if len(role.Spec.Permissions) != 1 || role.Spec.Permissions[0] != "tool:db_read:invoke" {
		t.Fatalf("unexpected role permissions: %+v", role.Spec.Permissions)
	}
}

func TestToolPermissionCreateRejectsInvalidScopedSpec(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	payload := resources.ToolPermission{
		APIVersion: "orloj.dev/v1",
		Kind:       "ToolPermission",
		Metadata: resources.ObjectMeta{
			Name:      "db-write",
			Namespace: "team-a",
		},
		Spec: resources.ToolPermissionSpec{
			ToolRef:   "db_write",
			ApplyMode: "scoped",
			Action:    "invoke",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	resp, err := http.Post(server.URL+"/v1/tool-permissions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post tool permission failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, string(respBody))
	}
}

func TestTaskApprovalRequestChangesRequiresComment(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/task-approvals", resources.TaskApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskApproval",
		Metadata: resources.ObjectMeta{
			Name:      "review-1",
			Namespace: "default",
		},
		Spec: resources.TaskApprovalSpec{
			TaskRef:        "default/report-task",
			CheckpointID:   "writer-review",
			CheckpointType: "agent_output",
		},
		Status: resources.TaskApprovalStatus{
			Phase: "Pending",
		},
	})

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/task-approvals/review-1/request-changes?namespace=default", bytes.NewReader([]byte(`{"decided_by":"reviewer"}`)))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestTaskApprovalRequestChangesRejectedWhenDisabled(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	allow := false
	postJSON(t, server.URL+"/v1/task-approvals", resources.TaskApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskApproval",
		Metadata: resources.ObjectMeta{
			Name:      "review-disabled",
			Namespace: "default",
		},
		Spec: resources.TaskApprovalSpec{
			TaskRef:             "default/report-task",
			CheckpointID:        "writer-review",
			CheckpointType:      "agent_output",
			AllowRequestChanges: &allow,
			MaxReviewCycles:     3,
			ReviewCycle:         1,
		},
		Status: resources.TaskApprovalStatus{
			Phase: "Pending",
		},
	})

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/task-approvals/review-disabled/request-changes?namespace=default", bytes.NewReader([]byte(`{"decided_by":"reviewer","comment":"revise it"}`)))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 409, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestTaskApprovalRequestChangesRejectedAtMaxReviewCycles(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	allow := true
	postJSON(t, server.URL+"/v1/task-approvals", resources.TaskApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskApproval",
		Metadata: resources.ObjectMeta{
			Name:      "review-maxed",
			Namespace: "default",
		},
		Spec: resources.TaskApprovalSpec{
			TaskRef:             "default/report-task",
			CheckpointID:        "writer-review",
			CheckpointType:      "agent_output",
			AllowRequestChanges: &allow,
			MaxReviewCycles:     2,
			ReviewCycle:         2,
		},
		Status: resources.TaskApprovalStatus{
			Phase: "Pending",
		},
	})

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/task-approvals/review-maxed/request-changes?namespace=default", bytes.NewReader([]byte(`{"decided_by":"reviewer","comment":"revise it"}`)))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 409, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestToolApprovalDecisionPersistsComment(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/tool-approvals", resources.ToolApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "ToolApproval",
		Metadata: resources.ObjectMeta{
			Name:      "tool-review-1",
			Namespace: "default",
		},
		Spec: resources.ToolApprovalSpec{
			TaskRef:        "default/report-task",
			Tool:           "web_search",
			OperationClass: "invoke",
			Agent:          "research-agent",
		},
		Status: resources.ToolApprovalStatus{
			Phase:     "Pending",
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339),
		},
	})

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/tool-approvals/tool-review-1/approve?namespace=default", bytes.NewReader([]byte(`{"decided_by":"reviewer","comment":"approved for this run"}`)))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
	}
	var approval resources.ToolApproval
	if err := json.NewDecoder(resp.Body).Decode(&approval); err != nil {
		t.Fatalf("decode approval failed: %v", err)
	}
	if approval.Status.Comment != "approved for this run" {
		t.Fatalf("expected comment to persist, got %+v", approval.Status)
	}
}

func TestTaskApprovalConcurrentDecisionsOnlyOneSucceeds(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/task-approvals", resources.TaskApproval{
		APIVersion: "orloj.dev/v1",
		Kind:       "TaskApproval",
		Metadata: resources.ObjectMeta{
			Name:      "race-review",
			Namespace: "default",
		},
		Spec: resources.TaskApprovalSpec{
			TaskRef:        "default/report-task",
			CheckpointID:   "writer-review",
			CheckpointType: "agent_output",
		},
		Status: resources.TaskApprovalStatus{
			Phase: "Pending",
		},
	})

	type result struct {
		status int
		body   string
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, action := range []string{"approve", "deny"} {
		action := action
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			req, err := http.NewRequest(
				http.MethodPost,
				server.URL+"/v1/task-approvals/race-review/"+action+"?namespace=default",
				bytes.NewReader([]byte(`{"decided_by":"reviewer","comment":"decision"}`)),
			)
			if err != nil {
				results <- result{status: 0, body: err.Error()}
				return
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				results <- result{status: 0, body: err.Error()}
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			results <- result{status: resp.StatusCode, body: string(body)}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	okCount := 0
	conflictCount := 0
	for res := range results {
		switch res.status {
		case http.StatusOK:
			okCount++
		case http.StatusConflict:
			conflictCount++
		default:
			t.Fatalf("expected 200/409 from concurrent decisions, got %d body=%s", res.status, res.body)
		}
	}
	if okCount != 1 || conflictCount != 1 {
		t.Fatalf("expected one success and one conflict, got success=%d conflict=%d", okCount, conflictCount)
	}

	resp, err := http.Get(server.URL + "/v1/task-approvals/race-review?namespace=default")
	if err != nil {
		t.Fatalf("get approval failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
	}
	var approval resources.TaskApproval
	if err := json.NewDecoder(resp.Body).Decode(&approval); err != nil {
		t.Fatalf("decode approval failed: %v", err)
	}
	if approval.Status.Phase != "Approved" && approval.Status.Phase != "Denied" {
		t.Fatalf("expected terminal approval phase after race, got %+v", approval.Status)
	}
	if approval.Status.Decision != "approved" && approval.Status.Decision != "denied" {
		t.Fatalf("expected terminal decision after race, got %+v", approval.Status)
	}
}
