package resources

import (
	"strings"
	"testing"
)

func TestAgentNormalizeWithModelRefDoesNotForceDefaultModel(t *testing.T) {
	agent := Agent{
		Kind:     "Agent",
		Metadata: ObjectMeta{Name: "researcher"},
		Spec: AgentSpec{
			Prompt:   "test",
			ModelRef: "openai-prod",
		},
	}
	if err := agent.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if agent.Spec.Model != "" {
		t.Fatalf("expected empty explicit model when model_ref is set, got %q", agent.Spec.Model)
	}
	if agent.Spec.ModelRef != "openai-prod" {
		t.Fatalf("unexpected model_ref %q", agent.Spec.ModelRef)
	}
}

func TestParseAgentManifestWithModelRefYAML(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: researcher
spec:
  model_ref: openai-team-a
  prompt: test
`)
	agent, err := ParseAgentManifest(raw)
	if err != nil {
		t.Fatalf("parse agent failed: %v", err)
	}
	if agent.Spec.ModelRef != "openai-team-a" {
		t.Fatalf("expected model_ref openai-team-a, got %q", agent.Spec.ModelRef)
	}
	if agent.Spec.Model != "" {
		t.Fatalf("expected model to remain empty when model_ref is set, got %q", agent.Spec.Model)
	}
}

func TestParseAgentManifestWithMemoryAllowYAML(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: memory-agent
spec:
  model_ref: anthropic-prod
  prompt: test
  memory:
    ref: shared-store
    allow:
      - read
      - memory.write
      - search
`)
	agent, err := ParseAgentManifest(raw)
	if err != nil {
		t.Fatalf("parse agent failed: %v", err)
	}
	got := strings.Join(agent.Spec.Memory.Allow, ",")
	if got != "read,write,search" {
		t.Fatalf("unexpected memory allow set %q", got)
	}
}

func TestAgentNormalizeRejectsMemoryAllowWithoutRef(t *testing.T) {
	agent := Agent{
		Kind:     "Agent",
		Metadata: ObjectMeta{Name: "memory-agent"},
		Spec: AgentSpec{
			Prompt: "test",
			Memory: MemorySpec{
				Allow: []string{"read"},
			},
		},
	}
	err := agent.Normalize()
	if err == nil {
		t.Fatal("expected error for memory.allow without memory.ref")
	}
	if !strings.Contains(err.Error(), "spec.memory.ref is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
