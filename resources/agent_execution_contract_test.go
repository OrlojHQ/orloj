package resources

import (
	"strings"
	"testing"
)

func TestAgentNormalizeExecutionDefaults(t *testing.T) {
	agent := Agent{
		Kind:     "Agent",
		Metadata: ObjectMeta{Name: "runner"},
		Spec: AgentSpec{
			Prompt:   "test",
			ModelRef: "openai-default",
		},
	}
	if err := agent.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if agent.Spec.Execution.Profile != AgentExecutionProfileDynamic {
		t.Fatalf("expected default profile %q, got %q", AgentExecutionProfileDynamic, agent.Spec.Execution.Profile)
	}
	if agent.Spec.Execution.DuplicateToolCallPolicy != AgentDuplicateToolCallPolicyShortCircuit {
		t.Fatalf("expected default duplicate policy %q, got %q", AgentDuplicateToolCallPolicyShortCircuit, agent.Spec.Execution.DuplicateToolCallPolicy)
	}
	if agent.Spec.Execution.OnContractViolation != AgentContractViolationPolicyNonRetryableError {
		t.Fatalf("expected default contract violation policy %q, got %q", AgentContractViolationPolicyNonRetryableError, agent.Spec.Execution.OnContractViolation)
	}
}

func TestParseAgentManifestExecutionContractYAML(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: contract-agent
spec:
  model_ref: openai-default
  prompt: execute in contract mode
  execution:
    profile: contract
    tool_sequence:
      - tool.alpha
      - tool.beta
      - tool.alpha
    required_output_markers:
      - CONTRACT_OK
      - CONTRACT_OK
      - FINALIZED
    duplicate_tool_call_policy: deny
    on_contract_violation: non_retryable_error
`)
	agent, err := ParseAgentManifest(raw)
	if err != nil {
		t.Fatalf("parse agent failed: %v", err)
	}
	if agent.Spec.Execution.Profile != AgentExecutionProfileContract {
		t.Fatalf("expected profile %q, got %q", AgentExecutionProfileContract, agent.Spec.Execution.Profile)
	}
	seq := strings.Join(agent.Spec.Execution.ToolSequence, ",")
	if seq != "tool.alpha,tool.beta" {
		t.Fatalf("unexpected tool_sequence normalization %q", seq)
	}
	markers := strings.Join(agent.Spec.Execution.RequiredOutputMarkers, ",")
	if markers != "CONTRACT_OK,FINALIZED" {
		t.Fatalf("unexpected marker normalization %q", markers)
	}
	if agent.Spec.Execution.DuplicateToolCallPolicy != AgentDuplicateToolCallPolicyDeny {
		t.Fatalf("expected duplicate policy %q, got %q", AgentDuplicateToolCallPolicyDeny, agent.Spec.Execution.DuplicateToolCallPolicy)
	}
	if agent.Spec.Execution.OnContractViolation != AgentContractViolationPolicyNonRetryableError {
		t.Fatalf("expected on_contract_violation %q, got %q", AgentContractViolationPolicyNonRetryableError, agent.Spec.Execution.OnContractViolation)
	}
}

func TestAgentNormalizeRejectsContractWithoutToolSequence(t *testing.T) {
	agent := Agent{
		Kind:     "Agent",
		Metadata: ObjectMeta{Name: "contract-agent"},
		Spec: AgentSpec{
			Prompt:   "test",
			ModelRef: "openai-default",
			Execution: AgentExecutionSpec{
				Profile: AgentExecutionProfileContract,
			},
		},
	}
	err := agent.Normalize()
	if err == nil {
		t.Fatal("expected error for contract profile without tool_sequence")
	}
	if !strings.Contains(err.Error(), "tool_sequence is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentNormalizeAcceptsObserveViolationPolicy(t *testing.T) {
	agent := Agent{
		Kind:     "Agent",
		Metadata: ObjectMeta{Name: "contract-agent"},
		Spec: AgentSpec{
			Prompt:   "test",
			ModelRef: "openai-default",
			Execution: AgentExecutionSpec{
				Profile:             AgentExecutionProfileContract,
				ToolSequence:        []string{"tool.alpha"},
				OnContractViolation: AgentContractViolationPolicyObserve,
			},
		},
	}
	if err := agent.Normalize(); err != nil {
		t.Fatalf("expected observe policy to be accepted, got %v", err)
	}
	if agent.Spec.Execution.OnContractViolation != AgentContractViolationPolicyObserve {
		t.Fatalf("expected on_contract_violation %q, got %q", AgentContractViolationPolicyObserve, agent.Spec.Execution.OnContractViolation)
	}
}

func TestAgentNormalizeRejectsInvalidViolationPolicy(t *testing.T) {
	agent := Agent{
		Kind:     "Agent",
		Metadata: ObjectMeta{Name: "contract-agent"},
		Spec: AgentSpec{
			Prompt:   "test",
			ModelRef: "openai-default",
			Execution: AgentExecutionSpec{
				Profile:             AgentExecutionProfileContract,
				ToolSequence:        []string{"tool.alpha"},
				OnContractViolation: "panic",
			},
		},
	}
	err := agent.Normalize()
	if err == nil {
		t.Fatal("expected error for invalid on_contract_violation")
	}
	if !strings.Contains(err.Error(), "invalid spec.execution.on_contract_violation") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentNormalizeRejectsInvalidExecutionProfile(t *testing.T) {
	agent := Agent{
		Kind:     "Agent",
		Metadata: ObjectMeta{Name: "contract-agent"},
		Spec: AgentSpec{
			Prompt:   "test",
			ModelRef: "openai-default",
			Execution: AgentExecutionSpec{
				Profile: "static",
			},
		},
	}
	err := agent.Normalize()
	if err == nil {
		t.Fatal("expected error for invalid execution profile")
	}
	if !strings.Contains(err.Error(), "invalid spec.execution.profile") {
		t.Fatalf("unexpected error: %v", err)
	}
}
