package startup

import (
	"reflect"
	"testing"
)

func TestNewMcpSessionManagerAppliesSharedRuntimeConfig(t *testing.T) {
	manager := NewMcpSessionManager(McpRuntimeConfig{
		ContainerRuntime: "podman",
		ContainerMemory:  "256m",
		ContainerCPUs:    "1.5",
		ContainerPids:    64,
	})
	if manager == nil {
		t.Fatal("expected session manager")
	}

	managerValue := reflect.ValueOf(manager).Elem()
	if managerValue.FieldByName("secretResolver").IsNil() {
		t.Fatal("expected secret resolver to be configured")
	}
	containerConfig := managerValue.FieldByName("containerConfig")
	if containerConfig.IsNil() {
		t.Fatal("expected container config to be configured")
	}
	config := containerConfig.Elem()
	if got := config.FieldByName("RuntimeBinary").String(); got != "podman" {
		t.Fatalf("expected runtime binary podman, got %q", got)
	}
	if got := config.FieldByName("Network").String(); got != "bridge" {
		t.Fatalf("expected default network bridge, got %q", got)
	}
	if got := config.FieldByName("Memory").String(); got != "256m" {
		t.Fatalf("expected memory 256m, got %q", got)
	}
	if got := config.FieldByName("CPUs").String(); got != "1.5" {
		t.Fatalf("expected CPUs 1.5, got %q", got)
	}
	if got := int(config.FieldByName("PidsLimit").Int()); got != 64 {
		t.Fatalf("expected pids limit 64, got %d", got)
	}
}
