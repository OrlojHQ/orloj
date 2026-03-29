package resources

import (
	"strings"
	"testing"
)

func TestParseManifest_UnsupportedKind(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: NotAnOrlojKind
metadata:
  name: x
spec: {}
`)
	_, _, _, err := ParseManifest("NotAnOrlojKind", raw)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("expected unsupported kind in error, got %v", err)
	}
}

func TestParseToolApprovalManifest_JSONMinimal(t *testing.T) {
	raw := []byte(`{
  "apiVersion": "orloj.dev/v1",
  "kind": "ToolApproval",
  "metadata": { "name": "approval-one" },
  "spec": { "task_ref": "task/default/t1", "tool": "deploy" }
}`)
	a, err := ParseToolApprovalManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if a.Metadata.Name != "approval-one" {
		t.Fatalf("name %q", a.Metadata.Name)
	}
	if a.Spec.TaskRef != "task/default/t1" || a.Spec.Tool != "deploy" {
		t.Fatalf("spec %+v", a.Spec)
	}
}

func TestParseManifest_NormalizesKindCasing(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: casing-test
spec:
  model_ref: mep
  prompt: hi
`)
	norm, name, _, err := ParseManifest("AgEnT", raw)
	if err != nil {
		t.Fatal(err)
	}
	if norm != "agent" {
		t.Fatalf("norm kind: got %q", norm)
	}
	if name != "casing-test" {
		t.Fatalf("name: got %q", name)
	}
}
