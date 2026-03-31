package resources

import "testing"

func TestToolNormalizeAcceptsCLIType(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "cli-tool"},
		Spec: ToolSpec{
			Type: "cli",
			Cli: ToolCliSpec{
				Command: "kubectl",
				Image:   "bitnami/kubectl:1.30",
			},
		},
	}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("expected cli type to normalize, got %v", err)
	}
	if tool.Spec.Type != "cli" {
		t.Fatalf("expected type=cli, got %q", tool.Spec.Type)
	}
}

func TestToolNormalizeCLIDefaultsIsolationToContainer(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "cli-tool"},
		Spec: ToolSpec{
			Type: "cli",
			Cli: ToolCliSpec{
				Command: "kubectl",
				Image:   "bitnami/kubectl:1.30",
			},
		},
	}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if tool.Spec.Runtime.IsolationMode != "container" {
		t.Fatalf("expected isolation_mode=container for cli, got %q", tool.Spec.Runtime.IsolationMode)
	}
}

func TestToolNormalizeCLIDefaultsNetworkToBridge(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "cli-tool"},
		Spec: ToolSpec{
			Type: "cli",
			Cli: ToolCliSpec{
				Command: "kubectl",
				Image:   "bitnami/kubectl:1.30",
			},
		},
	}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if tool.Spec.Cli.Network != "bridge" {
		t.Fatalf("expected network=bridge, got %q", tool.Spec.Cli.Network)
	}
}

func TestToolNormalizeCLIDefaultsOutputToStdout(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "cli-tool"},
		Spec: ToolSpec{
			Type: "cli",
			Cli: ToolCliSpec{
				Command: "echo",
				Image:   "alpine:3.19",
			},
		},
	}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if tool.Spec.Cli.Output != "stdout" {
		t.Fatalf("expected output=stdout, got %q", tool.Spec.Cli.Output)
	}
}

func TestToolNormalizeCLIRequiresCommand(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "cli-tool"},
		Spec: ToolSpec{
			Type: "cli",
			Cli: ToolCliSpec{
				Image: "alpine:3.19",
			},
		},
	}
	if err := tool.Normalize(); err == nil {
		t.Fatal("expected error when cli.command is missing")
	}
}

func TestToolNormalizeCLIRequiresImageWhenContainerIsolation(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "cli-tool"},
		Spec: ToolSpec{
			Type: "cli",
			Cli: ToolCliSpec{
				Command: "echo",
			},
		},
	}
	if err := tool.Normalize(); err == nil {
		t.Fatal("expected error when cli.image is missing with container isolation")
	}
}

func TestToolNormalizeCLIAllowsNoImageWhenIsolationNone(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "cli-tool"},
		Spec: ToolSpec{
			Type: "cli",
			Cli: ToolCliSpec{
				Command: "/usr/local/bin/echo",
			},
			Runtime: ToolRuntimePolicy{
				IsolationMode: "none",
			},
		},
	}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("expected cli with isolation_mode=none to normalize without image, got %v", err)
	}
}

func TestToolNormalizeCLIRejectsSpecAuth(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "cli-tool"},
		Spec: ToolSpec{
			Type: "cli",
			Cli: ToolCliSpec{
				Command: "echo",
				Image:   "alpine:3.19",
			},
			Auth: ToolAuth{SecretRef: "my-key"},
		},
	}
	if err := tool.Normalize(); err == nil {
		t.Fatal("expected error when spec.auth is set for cli tool")
	}
}

func TestToolNormalizeCLIRejectsInvalidOutput(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "cli-tool"},
		Spec: ToolSpec{
			Type: "cli",
			Cli: ToolCliSpec{
				Command: "echo",
				Image:   "alpine:3.19",
				Output:  "all",
			},
		},
	}
	if err := tool.Normalize(); err == nil {
		t.Fatal("expected error for invalid cli.output value")
	}
}

func TestToolNormalizeCLIValidatesEnvFrom(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "cli-tool"},
		Spec: ToolSpec{
			Type: "cli",
			Cli: ToolCliSpec{
				Command: "echo",
				Image:   "alpine:3.19",
				EnvFrom: []ToolCliEnvRef{
					{Name: "", SecretRef: "my-secret"},
				},
			},
		},
	}
	if err := tool.Normalize(); err == nil {
		t.Fatal("expected error when env_from[0].name is empty")
	}

	tool.Spec.Cli.EnvFrom = []ToolCliEnvRef{
		{Name: "MY_VAR", SecretRef: ""},
	}
	if err := tool.Normalize(); err == nil {
		t.Fatal("expected error when env_from[0].secretRef is empty")
	}
}

func TestParseToolManifestCLI(t *testing.T) {
	yaml := `apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: my-cli-tool
spec:
  type: cli
  risk_level: medium
  cli:
    command: kubectl
    image: bitnami/kubectl:1.30
    network: host
    output: both
    working_dir: /workspace
    stdin_from_input: true
    args:
      - get
      - pods
    env:
      KUBECONFIG: /etc/kube/config
    env_from:
      - name: TOKEN
        secretRef: my-secret
        key: token
`
	tool, err := ParseToolManifest([]byte(yaml))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tool.Spec.Type != "cli" {
		t.Fatalf("expected type=cli, got %q", tool.Spec.Type)
	}
	if tool.Spec.Cli.Command != "kubectl" {
		t.Fatalf("expected command=kubectl, got %q", tool.Spec.Cli.Command)
	}
	if tool.Spec.Cli.Image != "bitnami/kubectl:1.30" {
		t.Fatalf("expected image=bitnami/kubectl:1.30, got %q", tool.Spec.Cli.Image)
	}
	if tool.Spec.Cli.Network != "host" {
		t.Fatalf("expected network=host, got %q", tool.Spec.Cli.Network)
	}
	if tool.Spec.Cli.Output != "both" {
		t.Fatalf("expected output=both, got %q", tool.Spec.Cli.Output)
	}
	if tool.Spec.Cli.WorkingDir != "/workspace" {
		t.Fatalf("expected working_dir=/workspace, got %q", tool.Spec.Cli.WorkingDir)
	}
	if !tool.Spec.Cli.StdinFromInput {
		t.Fatal("expected stdin_from_input=true")
	}
	if len(tool.Spec.Cli.Args) != 2 || tool.Spec.Cli.Args[0] != "get" || tool.Spec.Cli.Args[1] != "pods" {
		t.Fatalf("expected args=[get, pods], got %v", tool.Spec.Cli.Args)
	}
	if tool.Spec.Cli.Env["KUBECONFIG"] != "/etc/kube/config" {
		t.Fatalf("expected env KUBECONFIG=/etc/kube/config, got %v", tool.Spec.Cli.Env)
	}
	if len(tool.Spec.Cli.EnvFrom) != 1 {
		t.Fatalf("expected 1 env_from entry, got %d", len(tool.Spec.Cli.EnvFrom))
	}
	ref := tool.Spec.Cli.EnvFrom[0]
	if ref.Name != "TOKEN" || ref.SecretRef != "my-secret" || ref.Key != "token" {
		t.Fatalf("unexpected env_from entry: %+v", ref)
	}
}

// Regression: JSON Schema uses nested "type" keys; the line-based YAML parser must treat
// input_schema as a subsection so those lines do not overwrite spec.type.
func TestParseToolManifestYAMLSpecTypeNotOverwrittenByInputSchema(t *testing.T) {
	yaml := `apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: gh-pr-list
spec:
  type: cli
  input_schema:
    type: object
    properties:
      repo:
        type: string
      state:
        type: string
        enum: [open, closed]
  cli:
    command: gh
    image: ghcr.io/cli/cli:2.50
    output: stdout
`
	tool, err := ParseToolManifest([]byte(yaml))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tool.Spec.Type != "cli" {
		t.Fatalf("expected spec.type cli, got %q (nested schema type must not overwrite)", tool.Spec.Type)
	}
	if tool.Spec.Cli.Command != "gh" {
		t.Fatalf("expected cli.command gh, got %q", tool.Spec.Cli.Command)
	}
}

func TestToolNormalizeAcceptsCLIInValidTypes(t *testing.T) {
	validTypes := []string{"cli", "CLI", "Cli"}
	for _, toolType := range validTypes {
		tool := Tool{
			Metadata: ObjectMeta{Name: "cli-type-test"},
			Spec: ToolSpec{
				Type: toolType,
				Cli: ToolCliSpec{
					Command: "echo",
					Image:   "alpine:3.19",
				},
			},
		}
		if err := tool.Normalize(); err != nil {
			t.Fatalf("expected type %q to normalize, got %v", toolType, err)
		}
		if tool.Spec.Type != "cli" {
			t.Fatalf("expected normalized type=cli, got %q", tool.Spec.Type)
		}
	}
}
