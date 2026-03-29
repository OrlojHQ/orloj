package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalAgentYAML = `apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: test-agent
spec:
  model_ref: openai-default
  prompt: hello
`

func TestRunValidate_SkipsConfigLoad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	f := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(f, []byte(minimalAgentYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"validate", "-f", f}); err != nil {
		t.Fatal(err)
	}
}

func TestRunValidate_MissingFlag(t *testing.T) {
	err := Run([]string{"validate"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "-f is required") {
		t.Fatalf("got %v", err)
	}
}

func TestRunValidate_NonexistentPath(t *testing.T) {
	err := Run([]string{"validate", "-f", filepath.Join(t.TempDir(), "nope.yaml")})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cannot access") {
		t.Fatalf("got %v", err)
	}
}

func TestRunValidate_EmptyDirectory(t *testing.T) {
	err := Run([]string{"validate", "-f", t.TempDir()})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no manifest files found") {
		t.Fatalf("got %v", err)
	}
}

func TestRunValidate_ValidFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := filepath.Join(t.TempDir(), "a.yaml")
	if err := os.WriteFile(f, []byte(minimalAgentYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"validate", "-f", f}); err != nil {
		t.Fatal(err)
	}
}

func TestRunValidate_InvalidYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(f, []byte("{not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{"validate", "-f", f})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("got %v", err)
	}
}

func TestRunValidate_MissingMetadataName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	raw := `apiVersion: orloj.dev/v1
kind: Agent
metadata:
  namespace: default
spec:
  prompt: x
`
	f := filepath.Join(t.TempDir(), "noname.yaml")
	if err := os.WriteFile(f, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{"validate", "-f", f})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("got %v", err)
	}
}

func TestRunValidate_UnrecognizedKind(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	raw := `apiVersion: orloj.dev/v1
kind: NotAnOrlojKind
metadata:
  name: x
spec: {}
`
	f := filepath.Join(t.TempDir(), "unknown.yaml")
	if err := os.WriteFile(f, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{"validate", "-f", f})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("got %v", err)
	}
	// Per-file detail (unsupported kind) is printed to stdout; see resources.TestParseManifest_UnsupportedKind.
}

func TestRunValidate_NestedDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	sub := filepath.Join(root, "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "inner.yaml"), []byte(minimalAgentYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"validate", "-f", root}); err != nil {
		t.Fatal(err)
	}
}

func TestRunValidate_MixedValidInvalid(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "good.yaml"), []byte(minimalAgentYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.yaml"), []byte("kind: Agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{"validate", "-f", root})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("got %v", err)
	}
}

func TestManifestPaths_SingleFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "one.yaml")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := manifestPaths(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != f {
		t.Fatalf("got %v", paths)
	}
}
