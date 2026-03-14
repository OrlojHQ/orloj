package crds

import "testing"

func TestParseAgentManifestRolesYAML(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: researcher
spec:
  model: gpt-4o
  prompt: test
  roles:
    - analyst
    - analyst
`)
	agent, err := ParseAgentManifest(raw)
	if err != nil {
		t.Fatalf("parse agent failed: %v", err)
	}
	if len(agent.Spec.Roles) != 1 || agent.Spec.Roles[0] != "analyst" {
		t.Fatalf("unexpected roles: %+v", agent.Spec.Roles)
	}
}

func TestParseAgentRoleManifestYAML(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: AgentRole
metadata:
  name: analyst
spec:
  permissions:
    - tool:web_search:invoke
    - capability:web.read
`)
	role, err := ParseAgentRoleManifest(raw)
	if err != nil {
		t.Fatalf("parse role failed: %v", err)
	}
	if role.Metadata.Name != "analyst" {
		t.Fatalf("unexpected role name %q", role.Metadata.Name)
	}
	if len(role.Spec.Permissions) != 2 {
		t.Fatalf("unexpected permissions: %+v", role.Spec.Permissions)
	}
}

func TestParseToolPermissionManifestYAML(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: ToolPermission
metadata:
  name: web-search
spec:
  tool_ref: web_search
  action: invoke
  match_mode: all
  required_permissions:
    - tool:web_search:invoke
`)
	perm, err := ParseToolPermissionManifest(raw)
	if err != nil {
		t.Fatalf("parse tool permission failed: %v", err)
	}
	if perm.Spec.ToolRef != "web_search" {
		t.Fatalf("unexpected tool_ref %q", perm.Spec.ToolRef)
	}
	if perm.Spec.MatchMode != "all" {
		t.Fatalf("unexpected match_mode %q", perm.Spec.MatchMode)
	}
}
