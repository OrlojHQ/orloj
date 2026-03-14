package crds

import "testing"

func TestAgentRoleNormalizeDedupesPermissions(t *testing.T) {
	role := AgentRole{
		Kind:     "AgentRole",
		Metadata: ObjectMeta{Name: "analyst"},
		Spec: AgentRoleSpec{
			Permissions: []string{"tool:web_search:invoke", " TOOL:WEB_SEARCH:INVOKE ", "capability:web.read"},
		},
	}
	if err := role.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if len(role.Spec.Permissions) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(role.Spec.Permissions))
	}
}

func TestToolPermissionNormalizeDefaultsAndScopedValidation(t *testing.T) {
	perm := ToolPermission{
		Kind:     "ToolPermission",
		Metadata: ObjectMeta{Name: "web_search"},
	}
	if err := perm.Normalize(); err != nil {
		t.Fatalf("normalize defaults failed: %v", err)
	}
	if perm.Spec.ToolRef != "web_search" {
		t.Fatalf("expected default tool_ref=web_search, got %q", perm.Spec.ToolRef)
	}
	if perm.Spec.Action != "invoke" {
		t.Fatalf("expected default action invoke, got %q", perm.Spec.Action)
	}
	if perm.Spec.MatchMode != "all" {
		t.Fatalf("expected default match_mode all, got %q", perm.Spec.MatchMode)
	}
	if perm.Spec.ApplyMode != "global" {
		t.Fatalf("expected default apply_mode global, got %q", perm.Spec.ApplyMode)
	}

	scoped := ToolPermission{
		Kind:     "ToolPermission",
		Metadata: ObjectMeta{Name: "db_write"},
		Spec: ToolPermissionSpec{
			ApplyMode: "scoped",
		},
	}
	if err := scoped.Normalize(); err == nil {
		t.Fatal("expected scoped tool permission to require target_agents")
	}
}
