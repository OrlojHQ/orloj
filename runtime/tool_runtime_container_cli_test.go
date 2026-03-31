package agentruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestContainerToolRuntimeCallCLI(t *testing.T) {
	runner := &captureContainerRunner{stdout: "pod1\npod2"}
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"kubectl-pods": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "kubectl",
				Args:    []string{"get", "pods", "-n", "{{ .namespace }}"},
				Image:   "bitnami/kubectl:1.30",
				Output:  "stdout",
			},
		},
	})
	rt := NewContainerToolRuntimeWithRunnerAndSecrets(registry, DefaultContainerToolRuntimeConfig(), runner, NewEnvSecretResolver("TEST_SECRET_"))
	result, err := rt.Call(context.Background(), "kubectl-pods", `{"namespace": "default"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(result) != "pod1\npod2" {
		t.Fatalf("unexpected result: %q", result)
	}
	if !containsArg(runner.args, "--entrypoint") {
		t.Fatalf("expected --entrypoint in docker args, got %v", runner.args)
	}
	if !containsArg(runner.args, "kubectl") {
		t.Fatalf("expected kubectl in docker args, got %v", runner.args)
	}
	if !containsArg(runner.args, "bitnami/kubectl:1.30") {
		t.Fatalf("expected image in docker args, got %v", runner.args)
	}
}

func TestContainerToolRuntimeCallCLIDefaultsBridgeNetwork(t *testing.T) {
	runner := &captureContainerRunner{stdout: "ok"}
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"net-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "curl",
				Image:   "curlimages/curl:8.8.0",
				Output:  "stdout",
			},
		},
	})
	rt := NewContainerToolRuntimeWithRunnerAndSecrets(registry, DefaultContainerToolRuntimeConfig(), runner, NewEnvSecretResolver("TEST_SECRET_"))
	_, err := rt.Call(context.Background(), "net-tool", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for i, arg := range runner.args {
		if arg == "--network" && i+1 < len(runner.args) && runner.args[i+1] == "bridge" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --network bridge in docker args, got %v", runner.args)
	}
}

func TestContainerToolRuntimeCallCLICustomNetwork(t *testing.T) {
	runner := &captureContainerRunner{stdout: "ok"}
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"custom-net-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "cmd",
				Image:   "alpine:3.19",
				Network: "none",
				Output:  "stdout",
			},
		},
	})
	rt := NewContainerToolRuntimeWithRunnerAndSecrets(registry, DefaultContainerToolRuntimeConfig(), runner, NewEnvSecretResolver("TEST_SECRET_"))
	_, err := rt.Call(context.Background(), "custom-net-tool", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for i, arg := range runner.args {
		if arg == "--network" && i+1 < len(runner.args) && runner.args[i+1] == "none" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --network none in docker args, got %v", runner.args)
	}
}

func TestContainerToolRuntimeCallCLIMissingImage(t *testing.T) {
	runner := &captureContainerRunner{}
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"no-image": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "echo",
				Output:  "stdout",
			},
		},
	})
	rt := NewContainerToolRuntimeWithRunnerAndSecrets(registry, DefaultContainerToolRuntimeConfig(), runner, NewEnvSecretResolver("TEST_SECRET_"))
	_, err := rt.Call(context.Background(), "no-image", "")
	if err == nil {
		t.Fatal("expected error for missing cli.image")
	}
}

func TestContainerToolRuntimeCallCLIWithEnv(t *testing.T) {
	runner := &captureContainerRunner{stdout: "ok"}
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"env-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command: "cmd",
				Image:   "alpine:3.19",
				Output:  "stdout",
				Env:     map[string]string{"MY_VAR": "my_value"},
			},
		},
	})
	rt := NewContainerToolRuntimeWithRunnerAndSecrets(registry, DefaultContainerToolRuntimeConfig(), runner, NewEnvSecretResolver("TEST_SECRET_"))
	_, err := rt.Call(context.Background(), "env-tool", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for i, arg := range runner.args {
		if arg == "-e" && i+1 < len(runner.args) && runner.args[i+1] == "MY_VAR=my_value" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected -e MY_VAR=my_value in docker args, got %v", runner.args)
	}
}

func containsArg(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}
