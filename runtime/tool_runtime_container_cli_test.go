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

func TestContainerToolRuntimeCallCLIDefaultsOperatorNetwork(t *testing.T) {
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
		if arg == "--network" && i+1 < len(runner.args) && runner.args[i+1] == "none" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --network none in docker args, got %v", runner.args)
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

type recordingContainerRunner struct {
	calls []recordedCall
}

type recordedCall struct {
	binary string
	args   []string
	stdin  string
	env    map[string]string
}

func (r *recordingContainerRunner) Run(_ context.Context, binary string, args []string, stdin string, env map[string]string) (string, string, error) {
	r.calls = append(r.calls, recordedCall{
		binary: binary,
		args:   append([]string(nil), args...),
		stdin:  stdin,
		env:    copyStringMap(env),
	})
	return "ok", "", nil
}

func TestContainerToolRuntimeCallCLIWithImagePullSecret(t *testing.T) {
	runner := &recordingContainerRunner{}
	secrets := staticSecretResolver{values: map[string]string{
		"ghcr-creds:registry": "ghcr.io",
		"ghcr-creds:username": "bot",
		"ghcr-creds:password": "ghp_token123",
	}}
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"private-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command:         "myapp",
				Image:           "ghcr.io/org/private-tool:v1",
				ImagePullSecret: "ghcr-creds",
				Output:          "stdout",
			},
		},
	})
	rt := NewContainerToolRuntimeWithRunnerAndSecrets(registry, DefaultContainerToolRuntimeConfig(), runner, secrets)
	_, err := rt.Call(context.Background(), "private-tool", "{}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 calls (pull + run), got %d", len(runner.calls))
	}

	pullCall := runner.calls[0]
	if !containsArg(pullCall.args, "pull") {
		t.Fatalf("first call should be pull, got args: %v", pullCall.args)
	}
	if !containsArg(pullCall.args, "ghcr.io/org/private-tool:v1") {
		t.Fatalf("pull should reference the image, got args: %v", pullCall.args)
	}
	if pullCall.env["DOCKER_CONFIG"] == "" {
		t.Fatal("pull call should have DOCKER_CONFIG env set")
	}

	runCall := runner.calls[1]
	if !containsArg(runCall.args, "run") {
		t.Fatalf("second call should be run, got args: %v", runCall.args)
	}
	if runCall.env["DOCKER_CONFIG"] == "" {
		t.Fatal("run call should have DOCKER_CONFIG env set")
	}
	if pullCall.env["DOCKER_CONFIG"] != runCall.env["DOCKER_CONFIG"] {
		t.Fatal("pull and run should use the same DOCKER_CONFIG")
	}
}

func TestContainerToolRuntimeCallCLIWithImagePullSecretMissing(t *testing.T) {
	runner := &recordingContainerRunner{}
	secrets := staticSecretResolver{values: map[string]string{}}
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"private-tool": {
			Type: "cli",
			Cli: resources.ToolCliSpec{
				Command:         "myapp",
				Image:           "ghcr.io/org/private-tool:v1",
				ImagePullSecret: "missing-creds",
				Output:          "stdout",
			},
		},
	})
	rt := NewContainerToolRuntimeWithRunnerAndSecrets(registry, DefaultContainerToolRuntimeConfig(), runner, secrets)
	_, err := rt.Call(context.Background(), "private-tool", "{}")
	if err == nil {
		t.Fatal("expected error for missing image pull secret")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected no runner calls on secret resolution failure, got %d", len(runner.calls))
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
