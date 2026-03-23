package agentruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

type authTestRoleLookup struct {
	roles map[string]resources.AgentRole
}

func (s *authTestRoleLookup) Get(_ context.Context, name string) (resources.AgentRole, bool, error) {
	r, ok := s.roles[name]
	return r, ok, nil
}

type authTestPermLookup struct {
	items []resources.ToolPermission
}

func (s *authTestPermLookup) List(_ context.Context) ([]resources.ToolPermission, error) {
	return s.items, nil
}

func TestAuthorizerAllowWhenNoRolesNoRules(t *testing.T) {
	agent := resources.Agent{Metadata: resources.ObjectMeta{Name: "a"}}
	auth := NewAgentToolAuthorizer(context.Background(), "default", agent, nil, nil)
	result, err := auth.Authorize("web_search", resources.ToolSpec{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != AuthorizeVerdictAllow {
		t.Errorf("expected allow, got %s", result.Verdict)
	}
}

func TestAuthorizerAllowWhenToolInAllowedList(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "a"},
		Spec:     resources.AgentSpec{AllowedTools: []string{"web_search"}, Roles: []string{"missing-role"}},
	}
	auth := NewAgentToolAuthorizer(context.Background(), "default", agent, nil, nil)
	result, err := auth.Authorize("web_search", resources.ToolSpec{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != AuthorizeVerdictAllow {
		t.Errorf("expected allow, got %s", result.Verdict)
	}
}

func TestAuthorizerDenyWhenMissingRoles(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "a"},
		Spec:     resources.AgentSpec{Roles: []string{"admin"}},
	}
	auth := NewAgentToolAuthorizer(context.Background(), "default", agent, nil, nil)
	_, err := auth.Authorize("web_search", resources.ToolSpec{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrToolPermissionDenied) {
		t.Errorf("expected ErrToolPermissionDenied, got %v", err)
	}
}

func TestAuthorizerTriStateApprovalRequired(t *testing.T) {
	agent := resources.Agent{Metadata: resources.ObjectMeta{Name: "a"}}
	perms := &authTestPermLookup{items: []resources.ToolPermission{
		{
			Metadata: resources.ObjectMeta{Name: "perm1", Namespace: "default"},
			Spec: resources.ToolPermissionSpec{
				ToolRef: "deploy-tool",
				Action:  "invoke",
				OperationRules: []resources.OperationRule{
					{OperationClass: "write", Verdict: "approval_required"},
					{OperationClass: "read", Verdict: "allow"},
				},
			},
		},
	}}

	auth := NewAgentToolAuthorizer(context.Background(), "default", agent, nil, perms)
	result, err := auth.Authorize("deploy-tool", resources.ToolSpec{OperationClasses: []string{"write"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != AuthorizeVerdictApprovalRequired {
		t.Errorf("expected approval_required, got %s", result.Verdict)
	}
}

func TestAuthorizerTriStateDenyOverridesApproval(t *testing.T) {
	agent := resources.Agent{Metadata: resources.ObjectMeta{Name: "a"}}
	perms := &authTestPermLookup{items: []resources.ToolPermission{
		{
			Metadata: resources.ObjectMeta{Name: "perm1", Namespace: "default"},
			Spec: resources.ToolPermissionSpec{
				ToolRef: "admin-tool",
				Action:  "invoke",
				OperationRules: []resources.OperationRule{
					{OperationClass: "admin", Verdict: "deny"},
					{OperationClass: "write", Verdict: "approval_required"},
				},
			},
		},
	}}

	auth := NewAgentToolAuthorizer(context.Background(), "default", agent, nil, perms)
	_, err := auth.Authorize("admin-tool", resources.ToolSpec{OperationClasses: []string{"write", "admin"}})
	if err == nil {
		t.Fatal("expected error (deny overrides approval)")
	}
	if !errors.Is(err, ErrToolPermissionDenied) {
		t.Errorf("expected ErrToolPermissionDenied, got %v", err)
	}
}

func TestAuthorizerBackwardCompatibleWhenNoOperationRules(t *testing.T) {
	roles := &authTestRoleLookup{roles: map[string]resources.AgentRole{
		"default/ops": {
			Metadata: resources.ObjectMeta{Name: "ops", Namespace: "default"},
			Spec:     resources.AgentRoleSpec{Permissions: []string{"tool:simple_tool:invoke"}},
		},
	}}
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "a"},
		Spec:     resources.AgentSpec{Roles: []string{"ops"}},
	}
	perms := &authTestPermLookup{items: []resources.ToolPermission{
		{
			Metadata: resources.ObjectMeta{Name: "perm1", Namespace: "default"},
			Spec: resources.ToolPermissionSpec{
				ToolRef: "simple-tool",
				Action:  "invoke",
			},
		},
	}}

	auth := NewAgentToolAuthorizer(context.Background(), "default", agent, roles, perms)
	result, err := auth.Authorize("simple-tool", resources.ToolSpec{OperationClasses: []string{"read"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != AuthorizeVerdictAllow {
		t.Errorf("expected allow (backward compatible, no operation rules), got %s", result.Verdict)
	}
}

func TestAuthorizerWildcardOperationRule(t *testing.T) {
	agent := resources.Agent{Metadata: resources.ObjectMeta{Name: "a"}}
	perms := &authTestPermLookup{items: []resources.ToolPermission{
		{
			Metadata: resources.ObjectMeta{Name: "perm1", Namespace: "default"},
			Spec: resources.ToolPermissionSpec{
				ToolRef: "any-tool",
				Action:  "invoke",
				OperationRules: []resources.OperationRule{
					{OperationClass: "*", Verdict: "approval_required"},
				},
			},
		},
	}}

	auth := NewAgentToolAuthorizer(context.Background(), "default", agent, nil, perms)
	result, err := auth.Authorize("any-tool", resources.ToolSpec{OperationClasses: []string{"delete"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != AuthorizeVerdictApprovalRequired {
		t.Errorf("expected approval_required via wildcard, got %s", result.Verdict)
	}
}

func TestMostRestrictiveVerdict(t *testing.T) {
	tests := []struct {
		a, b, want string
	}{
		{"allow", "allow", "allow"},
		{"allow", "approval_required", "approval_required"},
		{"allow", "deny", "deny"},
		{"approval_required", "allow", "approval_required"},
		{"approval_required", "deny", "deny"},
		{"deny", "allow", "deny"},
		{"deny", "approval_required", "deny"},
	}
	for _, tt := range tests {
		got := mostRestrictiveVerdict(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("mostRestrictiveVerdict(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestAuthorizerNilReturnsAllow(t *testing.T) {
	var auth *AgentToolAuthorizer
	result, err := auth.Authorize("any-tool", resources.ToolSpec{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != AuthorizeVerdictAllow {
		t.Errorf("expected allow for nil authorizer, got %s", result.Verdict)
	}
}
