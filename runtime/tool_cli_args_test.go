package agentruntime

import (
	"strings"
	"testing"
)

func TestEvaluateCLIArgsStaticOnly(t *testing.T) {
	args, err := evaluateCLIArgs([]string{"get", "pods", "-o", "json"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d", len(args))
	}
	if args[0] != "get" || args[3] != "json" {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestEvaluateCLIArgsTemplated(t *testing.T) {
	args, err := evaluateCLIArgs(
		[]string{"get", "pods", "-n", "{{ .namespace }}", "-o", "{{ .format }}"},
		`{"namespace": "production", "format": "json"}`,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 6 {
		t.Fatalf("expected 6 args, got %d: %v", len(args), args)
	}
	if args[3] != "production" || args[5] != "json" {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestEvaluateCLIArgsDropsEmpty(t *testing.T) {
	args, err := evaluateCLIArgs(
		[]string{"cmd", "{{ .optional }}"},
		`{"optional": ""}`,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 1 || args[0] != "cmd" {
		t.Fatalf("expected empty template result to be dropped, got %v", args)
	}
}

func TestEvaluateCLIArgsMissingKeyError(t *testing.T) {
	_, err := evaluateCLIArgs(
		[]string{"cmd", "{{ .missing }}"},
		`{"other": "value"}`,
	)
	if err == nil {
		t.Fatal("expected error for missing template key")
	}
}

func TestEvaluateCLIArgsInvalidTemplate(t *testing.T) {
	_, err := evaluateCLIArgs(
		[]string{"cmd", "{{ .broken"},
		`{"broken": "value"}`,
	)
	if err == nil {
		t.Fatal("expected error for invalid template syntax")
	}
}

func TestEvaluateCLIArgsInvalidJSON(t *testing.T) {
	_, err := evaluateCLIArgs(
		[]string{"cmd", "{{ .key }}"},
		`not-json`,
	)
	if err == nil {
		t.Fatal("expected error for invalid JSON input")
	}
}

func TestEvaluateCLIArgsEmptyInput(t *testing.T) {
	args, err := evaluateCLIArgs([]string{"echo", "hello"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 2 || args[1] != "hello" {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestEvaluateCLIArgsNoShellSplitting(t *testing.T) {
	args, err := evaluateCLIArgs(
		[]string{"cmd", "{{ .value }}"},
		`{"value": "hello world ; rm -rf /"}`,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args (no shell splitting), got %d: %v", len(args), args)
	}
	if !strings.Contains(args[1], "rm -rf") {
		t.Fatalf("expected value preserved as single arg, got %q", args[1])
	}
}

func TestTotalArgvLength(t *testing.T) {
	args := []string{"cmd", "--flag", "value"}
	total := totalArgvLength(args)
	if total != 14 {
		t.Fatalf("expected total 14, got %d", total)
	}
}
