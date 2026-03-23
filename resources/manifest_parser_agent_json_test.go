package resources

import "testing"

func TestParseAgentManifest_MinimalJSON(t *testing.T) {
	raw := []byte(`{
  "apiVersion": "orloj.dev/v1",
  "kind": "Agent",
  "metadata": { "name": "researcher" },
  "spec": {
    "model_ref": "openai-default",
    "prompt": "You are helpful."
  }
}`)
	a, err := ParseAgentManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if a.Metadata.Name != "researcher" {
		t.Fatalf("name: %q", a.Metadata.Name)
	}
	if a.Spec.ModelRef != "openai-default" || a.Spec.Prompt != "You are helpful." {
		t.Fatalf("spec: ref=%q prompt=%q", a.Spec.ModelRef, a.Spec.Prompt)
	}
}

func TestParseAgentManifest_JSONMissingModelRef(t *testing.T) {
	raw := []byte(`{"apiVersion":"orloj.dev/v1","kind":"Agent","metadata":{"name":"x"},"spec":{"prompt":"p"}}`)
	_, err := ParseAgentManifest(raw)
	if err == nil {
		t.Fatal("expected error without model_ref")
	}
}

func TestParseAgentManifest_InvalidJSON(t *testing.T) {
	_, err := ParseAgentManifest([]byte(`{"kind":`))
	if err == nil {
		t.Fatal("expected error for broken JSON")
	}
}
