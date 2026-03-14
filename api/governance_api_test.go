package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/AnonJon/orloj/crds"
)

func TestAgentRoleCRUDAndNamespaceScoping(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	postJSON(t, server.URL+"/v1/agent-roles", crds.AgentRole{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentRole",
		Metadata: crds.ObjectMeta{
			Name:      "analyst",
			Namespace: "team-a",
		},
		Spec: crds.AgentRoleSpec{Permissions: []string{"tool:web_search:invoke"}},
	})
	postJSON(t, server.URL+"/v1/agent-roles", crds.AgentRole{
		APIVersion: "orloj.dev/v1",
		Kind:       "AgentRole",
		Metadata: crds.ObjectMeta{
			Name:      "analyst",
			Namespace: "team-b",
		},
		Spec: crds.AgentRoleSpec{Permissions: []string{"tool:db_read:invoke"}},
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
	var role crds.AgentRole
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

	payload := crds.ToolPermission{
		APIVersion: "orloj.dev/v1",
		Kind:       "ToolPermission",
		Metadata: crds.ObjectMeta{
			Name:      "db-write",
			Namespace: "team-a",
		},
		Spec: crds.ToolPermissionSpec{
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
