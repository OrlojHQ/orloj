package resources

import "testing"

func TestParseAgentManifestMetadataVersionFields(t *testing.T) {
	raw := []byte(`
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research
  namespace: team-a
  labels:
    orloj.dev/usecase: research
  resourceVersion: "7"
  generation: 3
spec:
  model_ref: openai-default
  prompt: hello
`)

	agent, err := ParseAgentManifest(raw)
	if err != nil {
		t.Fatalf("parse agent manifest failed: %v", err)
	}
	if agent.Metadata.Name != "research" {
		t.Fatalf("expected metadata.name=research, got %q", agent.Metadata.Name)
	}
	if agent.Metadata.Namespace != "team-a" {
		t.Fatalf("expected metadata.namespace=team-a, got %q", agent.Metadata.Namespace)
	}
	if got := agent.Metadata.Labels["orloj.dev/usecase"]; got != "research" {
		t.Fatalf("expected metadata.labels[orloj.dev/usecase]=research, got %q", got)
	}
	if agent.Metadata.ResourceVersion != "7" {
		t.Fatalf("expected metadata.resourceVersion=7, got %q", agent.Metadata.ResourceVersion)
	}
	if agent.Metadata.Generation != 3 {
		t.Fatalf("expected metadata.generation=3, got %d", agent.Metadata.Generation)
	}
}
