package resources

import (
	"strings"
	"testing"
)

func TestParseMemoryBytes(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"128m", 128 * 1024 * 1024, false},
		{"1g", 1024 * 1024 * 1024, false},
		{"512k", 512 * 1024, false},
		{"1024b", 1024, false},
		{"1024", 1024, false},
		{"2G", 2 * 1024 * 1024 * 1024, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-1m", 0, true},
		{"0m", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMemoryBytes(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %d", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseMemoryBytes(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseMemoryBytes_Overflow(t *testing.T) {
	overflowCases := []string{
		"9999999999g",
		"9999999999999m",
		"9223372036854775807k",
	}
	for _, input := range overflowCases {
		t.Run(input, func(t *testing.T) {
			_, err := ParseMemoryBytes(input)
			if err == nil {
				t.Fatalf("expected overflow error for %q", input)
			}
			if !strings.Contains(err.Error(), "overflow") {
				t.Fatalf("expected overflow in error message, got: %v", err)
			}
		})
	}
}

func TestParseMemoryBytes_KubernetesStyle(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1Gi", 1024 * 1024 * 1024},
		{"128Mi", 128 * 1024 * 1024},
		{"512Ki", 512 * 1024},
		{"2gi", 2 * 1024 * 1024 * 1024},
		{"256mi", 256 * 1024 * 1024},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMemoryBytes(tt.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseMemoryBytes(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateContainerResources(t *testing.T) {
	tests := []struct {
		name    string
		res     ContainerResources
		wantErr string
	}{
		{"empty is valid", ContainerResources{}, ""},
		{"valid memory", ContainerResources{Memory: "1g"}, ""},
		{"valid cpus", ContainerResources{CPUs: "0.5"}, ""},
		{"valid pids", ContainerResources{PidsLimit: 128}, ""},
		{"all valid", ContainerResources{Memory: "256m", CPUs: "2.0", PidsLimit: 64}, ""},
		{"bad memory", ContainerResources{Memory: "abc"}, "memory"},
		{"bad cpus", ContainerResources{CPUs: "not-a-number"}, "cpus"},
		{"zero cpus", ContainerResources{CPUs: "0"}, "cpus"},
		{"negative cpus", ContainerResources{CPUs: "-1"}, "cpus"},
		{"negative pids", ContainerResources{PidsLimit: -1}, "pids_limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContainerResources(tt.res, "test")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestEnforceContainerResourceCeiling(t *testing.T) {
	tests := []struct {
		name    string
		res     ContainerResources
		ceiling ContainerResourceCeiling
		wantErr bool
	}{
		{"empty ceiling allows anything", ContainerResources{Memory: "64g"}, ContainerResourceCeiling{}, false},
		{"within ceiling", ContainerResources{Memory: "512m"}, ContainerResourceCeiling{MaxMemory: "1g"}, false},
		{"at ceiling", ContainerResources{Memory: "1g"}, ContainerResourceCeiling{MaxMemory: "1g"}, false},
		{"exceeds memory", ContainerResources{Memory: "2g"}, ContainerResourceCeiling{MaxMemory: "1g"}, true},
		{"exceeds cpus", ContainerResources{CPUs: "4.0"}, ContainerResourceCeiling{MaxCPUs: "2.0"}, true},
		{"within cpus", ContainerResources{CPUs: "1.0"}, ContainerResourceCeiling{MaxCPUs: "2.0"}, false},
		{"exceeds pids", ContainerResources{PidsLimit: 1024}, ContainerResourceCeiling{MaxPidsLimit: 512}, true},
		{"within pids", ContainerResources{PidsLimit: 256}, ContainerResourceCeiling{MaxPidsLimit: 512}, false},
		{"no manifest value, ceiling set", ContainerResources{}, ContainerResourceCeiling{MaxMemory: "1g", MaxCPUs: "2.0", MaxPidsLimit: 512}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EnforceContainerResourceCeiling(tt.res, tt.ceiling, "Tool", "test-tool")
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseToolManifestCLIResources(t *testing.T) {
	yaml := `apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: browser-screenshot
spec:
  type: cli
  description: Take a screenshot
  cli:
    command: screenshot
    image: my-chromium:latest
    network: bridge
    resources:
      memory: 1g
      cpus: "1.0"
      pids_limit: 256
`
	tool, err := ParseToolManifest([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if tool.Spec.Cli.Resources.Memory != "1g" {
		t.Errorf("memory = %q, want %q", tool.Spec.Cli.Resources.Memory, "1g")
	}
	if tool.Spec.Cli.Resources.CPUs != "1.0" {
		t.Errorf("cpus = %q, want %q", tool.Spec.Cli.Resources.CPUs, "1.0")
	}
	if tool.Spec.Cli.Resources.PidsLimit != 256 {
		t.Errorf("pids_limit = %d, want %d", tool.Spec.Cli.Resources.PidsLimit, 256)
	}
}

func TestParseToolManifestCLIResourcesJSON(t *testing.T) {
	js := `{
		"apiVersion": "orloj.dev/v1",
		"kind": "Tool",
		"metadata": {"name": "browser-screenshot"},
		"spec": {
			"type": "cli",
			"cli": {
				"command": "screenshot",
				"image": "my-chromium:latest",
				"resources": {
					"memory": "2g",
					"cpus": "0.75",
					"pids_limit": 128
				}
			}
		}
	}`
	tool, err := ParseToolManifest([]byte(js))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if tool.Spec.Cli.Resources.Memory != "2g" {
		t.Errorf("memory = %q, want %q", tool.Spec.Cli.Resources.Memory, "2g")
	}
	if tool.Spec.Cli.Resources.CPUs != "0.75" {
		t.Errorf("cpus = %q, want %q", tool.Spec.Cli.Resources.CPUs, "0.75")
	}
	if tool.Spec.Cli.Resources.PidsLimit != 128 {
		t.Errorf("pids_limit = %d, want %d", tool.Spec.Cli.Resources.PidsLimit, 128)
	}
}

func TestParseMcpServerManifestResources(t *testing.T) {
	yaml := `apiVersion: orloj.dev/v1
kind: McpServer
metadata:
  name: playwright-mcp
spec:
  transport: stdio
  image: playwright-mcp:latest
  resources:
    memory: 1g
    cpus: "2.0"
    pids_limit: 512
`
	server, err := ParseMcpServerManifest([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if server.Spec.Resources.Memory != "1g" {
		t.Errorf("memory = %q, want %q", server.Spec.Resources.Memory, "1g")
	}
	if server.Spec.Resources.CPUs != "2.0" {
		t.Errorf("cpus = %q, want %q", server.Spec.Resources.CPUs, "2.0")
	}
	if server.Spec.Resources.PidsLimit != 512 {
		t.Errorf("pids_limit = %d, want %d", server.Spec.Resources.PidsLimit, 512)
	}
}

func TestParseMcpServerManifestResourcesJSON(t *testing.T) {
	js := `{
		"apiVersion": "orloj.dev/v1",
		"kind": "McpServer",
		"metadata": {"name": "playwright-mcp"},
		"spec": {
			"transport": "stdio",
			"image": "playwright-mcp:latest",
			"resources": {
				"memory": "512m",
				"pids_limit": 256
			}
		}
	}`
	server, err := ParseMcpServerManifest([]byte(js))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if server.Spec.Resources.Memory != "512m" {
		t.Errorf("memory = %q, want %q", server.Spec.Resources.Memory, "512m")
	}
	if server.Spec.Resources.PidsLimit != 256 {
		t.Errorf("pids_limit = %d, want %d", server.Spec.Resources.PidsLimit, 256)
	}
}
