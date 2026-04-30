package agentruntime

import (
	"fmt"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

// EnforcePoliciesForAgent checks that the agent's effective model and declared
// tools comply with all matched AgentPolicy resources. Returns an error on the
// first violation. This must be called in both synchronous and message-driven
// execution paths before the agent runs.
func EnforcePoliciesForAgent(agent resources.Agent, effectiveModel string, policies []resources.AgentPolicy) error {
	for _, policy := range policies {
		if !policyAppliesToAgent(policy, agent.Metadata.Name) {
			continue
		}
		if len(policy.Spec.AllowedModels) > 0 && !containsFoldSlice(policy.Spec.AllowedModels, effectiveModel) {
			return fmt.Errorf("policy %q disallows model %q (model_ref=%q) for agent %q", policy.Metadata.Name, effectiveModel, agent.Spec.ModelRef, agent.Metadata.Name)
		}
		for _, tool := range agent.Spec.Tools {
			if containsFoldSlice(policy.Spec.BlockedTools, tool) {
				return fmt.Errorf("policy %q blocks tool %q for agent %q", policy.Metadata.Name, tool, agent.Metadata.Name)
			}
		}
	}
	return nil
}

// MatchedPolicies returns the subset of policies that apply to the given
// task/system combination, respecting apply_mode (global vs scoped).
func MatchedPolicies(task resources.Task, system resources.AgentSystem, all []resources.AgentPolicy) []resources.AgentPolicy {
	out := make([]resources.AgentPolicy, 0, len(all))
	for _, policy := range all {
		if policyAppliesTo(policy, task, system) {
			out = append(out, policy)
		}
	}
	return out
}

// MinimumTokenBudget returns the smallest positive MaxTokensPerRun across
// system-wide policies (those without target_agents), or 0 when no budget is
// configured. Policies with target_agents are per-agent budgets and are handled
// by AgentTokenBudget instead.
func MinimumTokenBudget(policies []resources.AgentPolicy) int {
	min := 0
	for _, policy := range policies {
		if policy.Spec.MaxTokensPerRun <= 0 {
			continue
		}
		if len(policy.Spec.TargetAgents) > 0 {
			continue
		}
		if min == 0 || policy.Spec.MaxTokensPerRun < min {
			min = policy.Spec.MaxTokensPerRun
		}
	}
	return min
}

func policyAppliesTo(policy resources.AgentPolicy, task resources.Task, system resources.AgentSystem) bool {
	mode := strings.ToLower(strings.TrimSpace(policy.Spec.ApplyMode))
	if mode == "" {
		mode = "scoped"
	}
	if mode == "global" {
		return true
	}
	if len(policy.Spec.TargetTasks) > 0 && containsFoldSlice(policy.Spec.TargetTasks, task.Metadata.Name) {
		return true
	}
	if len(policy.Spec.TargetSystems) > 0 && containsFoldSlice(policy.Spec.TargetSystems, system.Metadata.Name) {
		return true
	}
	return false
}

// MinimumChildDepth returns the smallest positive MaxChildDepth across the
// given policies, or 0 when no limit is configured.
func MinimumChildDepth(policies []resources.AgentPolicy) int {
	min := 0
	for _, policy := range policies {
		if policy.Spec.MaxChildDepth <= 0 {
			continue
		}
		if min == 0 || policy.Spec.MaxChildDepth < min {
			min = policy.Spec.MaxChildDepth
		}
	}
	return min
}

// MinimumChildTasks returns the smallest positive MaxChildTasks across the
// given policies, or 0 when no limit is configured.
func MinimumChildTasks(policies []resources.AgentPolicy) int {
	min := 0
	for _, policy := range policies {
		if policy.Spec.MaxChildTasks <= 0 {
			continue
		}
		if min == 0 || policy.Spec.MaxChildTasks < min {
			min = policy.Spec.MaxChildTasks
		}
	}
	return min
}

// policyAppliesToAgent returns true when a policy should be enforced for the
// named agent. Policies without target_agents apply to all agents. Policies
// with target_agents only apply when the agent name is in the list.
func policyAppliesToAgent(policy resources.AgentPolicy, agentName string) bool {
	if len(policy.Spec.TargetAgents) == 0 {
		return true
	}
	return containsFoldSlice(policy.Spec.TargetAgents, agentName)
}

// AgentTokenBudget returns the smallest positive MaxTokensPerRun across
// policies that apply to the named agent, or 0 when no budget is configured.
// Policies with target_agents only match the listed agents; policies without
// target_agents apply to all agents.
func AgentTokenBudget(policies []resources.AgentPolicy, agentName string) int {
	min := 0
	for _, policy := range policies {
		if policy.Spec.MaxTokensPerRun <= 0 {
			continue
		}
		if !policyAppliesToAgent(policy, agentName) {
			continue
		}
		if min == 0 || policy.Spec.MaxTokensPerRun < min {
			min = policy.Spec.MaxTokensPerRun
		}
	}
	return min
}

func containsFoldSlice(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}
