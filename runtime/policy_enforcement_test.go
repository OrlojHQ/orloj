package agentruntime

import (
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestEnforcePoliciesForAgent_AllowedModels(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "writer"},
		Spec:     resources.AgentSpec{ModelRef: "gpt-4o", Tools: []string{"web-search"}},
	}
	policy := resources.AgentPolicy{
		Metadata: resources.ObjectMeta{Name: "restrict-models"},
		Spec: resources.AgentPolicySpec{
			AllowedModels: []string{"gpt-4o", "claude-3"},
		},
	}

	if err := EnforcePoliciesForAgent(agent, "gpt-4o", []resources.AgentPolicy{policy}); err != nil {
		t.Fatalf("expected allowed model to pass: %v", err)
	}

	if err := EnforcePoliciesForAgent(agent, "gpt-3.5-turbo", []resources.AgentPolicy{policy}); err == nil {
		t.Fatal("expected disallowed model to fail")
	}
}

func TestEnforcePoliciesForAgent_BlockedTools(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "researcher"},
		Spec:     resources.AgentSpec{ModelRef: "gpt-4o", Tools: []string{"web-search", "dangerous-tool"}},
	}
	policy := resources.AgentPolicy{
		Metadata: resources.ObjectMeta{Name: "block-dangerous"},
		Spec: resources.AgentPolicySpec{
			BlockedTools: []string{"dangerous-tool"},
		},
	}

	if err := EnforcePoliciesForAgent(agent, "gpt-4o", []resources.AgentPolicy{policy}); err == nil {
		t.Fatal("expected blocked tool to fail")
	}

	agent.Spec.Tools = []string{"web-search", "calculator"}
	if err := EnforcePoliciesForAgent(agent, "gpt-4o", []resources.AgentPolicy{policy}); err != nil {
		t.Fatalf("expected no blocked tools to pass: %v", err)
	}
}

func TestEnforcePoliciesForAgent_NoPolicies(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "any"},
		Spec:     resources.AgentSpec{ModelRef: "any-model", Tools: []string{"any-tool"}},
	}
	if err := EnforcePoliciesForAgent(agent, "any-model", nil); err != nil {
		t.Fatalf("expected no policies to pass: %v", err)
	}
}

func TestMatchedPolicies_Global(t *testing.T) {
	task := resources.Task{Metadata: resources.ObjectMeta{Name: "task-1"}}
	system := resources.AgentSystem{Metadata: resources.ObjectMeta{Name: "system-1"}}
	global := resources.AgentPolicy{
		Metadata: resources.ObjectMeta{Name: "global-policy"},
		Spec:     resources.AgentPolicySpec{ApplyMode: "global"},
	}
	scoped := resources.AgentPolicy{
		Metadata: resources.ObjectMeta{Name: "scoped-policy"},
		Spec:     resources.AgentPolicySpec{ApplyMode: "scoped", TargetSystems: []string{"other-system"}},
	}

	matched := MatchedPolicies(task, system, []resources.AgentPolicy{global, scoped})
	if len(matched) != 1 || matched[0].Metadata.Name != "global-policy" {
		t.Fatalf("expected only global policy, got %d policies", len(matched))
	}
}

func TestMatchedPolicies_ScopedBySystem(t *testing.T) {
	task := resources.Task{Metadata: resources.ObjectMeta{Name: "task-1"}}
	system := resources.AgentSystem{Metadata: resources.ObjectMeta{Name: "research-system"}}
	policy := resources.AgentPolicy{
		Metadata: resources.ObjectMeta{Name: "research-only"},
		Spec:     resources.AgentPolicySpec{ApplyMode: "scoped", TargetSystems: []string{"research-system"}},
	}

	matched := MatchedPolicies(task, system, []resources.AgentPolicy{policy})
	if len(matched) != 1 {
		t.Fatalf("expected scoped policy to match, got %d", len(matched))
	}
}

func TestMatchedPolicies_ScopedByTask(t *testing.T) {
	task := resources.Task{Metadata: resources.ObjectMeta{Name: "special-task"}}
	system := resources.AgentSystem{Metadata: resources.ObjectMeta{Name: "any-system"}}
	policy := resources.AgentPolicy{
		Metadata: resources.ObjectMeta{Name: "task-specific"},
		Spec:     resources.AgentPolicySpec{ApplyMode: "scoped", TargetTasks: []string{"special-task"}},
	}

	matched := MatchedPolicies(task, system, []resources.AgentPolicy{policy})
	if len(matched) != 1 {
		t.Fatalf("expected task-scoped policy to match, got %d", len(matched))
	}

	task.Metadata.Name = "other-task"
	matched = MatchedPolicies(task, system, []resources.AgentPolicy{policy})
	if len(matched) != 0 {
		t.Fatalf("expected no match for other-task, got %d", len(matched))
	}
}

func TestMatchedPolicies_DefaultIsScoped(t *testing.T) {
	task := resources.Task{Metadata: resources.ObjectMeta{Name: "task-1"}}
	system := resources.AgentSystem{Metadata: resources.ObjectMeta{Name: "system-1"}}
	policy := resources.AgentPolicy{
		Metadata: resources.ObjectMeta{Name: "no-targets"},
		Spec:     resources.AgentPolicySpec{},
	}

	matched := MatchedPolicies(task, system, []resources.AgentPolicy{policy})
	if len(matched) != 0 {
		t.Fatalf("expected scoped policy with no targets to match nothing, got %d", len(matched))
	}
}

func TestMinimumTokenBudget(t *testing.T) {
	policies := []resources.AgentPolicy{
		{Spec: resources.AgentPolicySpec{MaxTokensPerRun: 5000}},
		{Spec: resources.AgentPolicySpec{MaxTokensPerRun: 2000}},
		{Spec: resources.AgentPolicySpec{MaxTokensPerRun: 0}},
	}
	if got := MinimumTokenBudget(policies); got != 2000 {
		t.Fatalf("expected 2000, got %d", got)
	}
	if got := MinimumTokenBudget(nil); got != 0 {
		t.Fatalf("expected 0 for nil policies, got %d", got)
	}
}
