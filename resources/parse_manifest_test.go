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

func TestParseAgentManifest_MultiDocument(t *testing.T) {
	raw := []byte(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: first
spec:
  model_ref: mep
  prompt: hello
---
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: second
spec:
  model_ref: mep
  prompt: world
`)
	_, err := ParseAgentManifest(raw)
	if err == nil {
		t.Fatal("expected error for multi-document YAML")
	}
	if !strings.Contains(err.Error(), "multi-document") {
		t.Fatalf("expected multi-document error, got: %v", err)
	}
}

func TestAgentNormalize_OutputSchemaDepthLimit(t *testing.T) {
	schema := map[string]any{"type": "object"}
	current := schema
	for i := 0; i < 20; i++ {
		child := map[string]any{"type": "object"}
		current["properties"] = child
		current = child
	}
	agent := Agent{
		Metadata: ObjectMeta{Name: "deep-schema"},
		Spec: AgentSpec{
			ModelRef:  "mep",
			Prompt:    "test",
			Execution: AgentExecutionSpec{OutputSchema: schema},
		},
	}
	err := agent.Normalize()
	if err == nil {
		t.Fatal("expected error for deeply nested output schema")
	}
	if !strings.Contains(err.Error(), "nesting depth") {
		t.Fatalf("expected nesting depth error, got: %v", err)
	}
}

func TestAgentNormalize_OutputSchemaRequiresType(t *testing.T) {
	agent := Agent{
		Metadata: ObjectMeta{Name: "no-type"},
		Spec: AgentSpec{
			ModelRef:  "mep",
			Prompt:    "test",
			Execution: AgentExecutionSpec{OutputSchema: map[string]any{"description": "foo"}},
		},
	}
	err := agent.Normalize()
	if err == nil {
		t.Fatal("expected error for schema without type key")
	}
}

func TestTaskNormalize_RunModeRequiresSystem(t *testing.T) {
	task := Task{
		Metadata: ObjectMeta{Name: "no-system"},
		Spec: TaskSpec{
			Mode: "run",
		},
	}
	err := task.Normalize()
	if err == nil {
		t.Fatal("expected error when spec.system is empty in run mode")
	}
	if !strings.Contains(err.Error(), "spec.system is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskNormalize_TemplateModeNoSystemOK(t *testing.T) {
	task := Task{
		Metadata: ObjectMeta{Name: "template-task"},
		Spec: TaskSpec{
			Mode: "template",
		},
	}
	err := task.Normalize()
	if err != nil {
		t.Fatalf("template mode should not require system: %v", err)
	}
}

func TestYamlScalarToTyped_QuotedCommas(t *testing.T) {
	result := yamlScalarToTyped(`[a, "hello, world", b]`)
	arr, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", result)
	}
	if len(arr) != 3 {
		t.Fatalf("expected 3 elements, got %d: %v", len(arr), arr)
	}
	if arr[0] != "a" {
		t.Fatalf("arr[0] = %q, want %q", arr[0], "a")
	}
	if arr[1] != "hello, world" {
		t.Fatalf("arr[1] = %q, want %q", arr[1], "hello, world")
	}
	if arr[2] != "b" {
		t.Fatalf("arr[2] = %q, want %q", arr[2], "b")
	}
}

func TestYamlScalarToTyped_SingleQuotedCommas(t *testing.T) {
	result := yamlScalarToTyped(`['one, two', three]`)
	arr, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", result)
	}
	if len(arr) != 2 {
		t.Fatalf("expected 2 elements, got %d: %v", len(arr), arr)
	}
	if arr[0] != "one, two" {
		t.Fatalf("arr[0] = %q, want %q", arr[0], "one, two")
	}
}
