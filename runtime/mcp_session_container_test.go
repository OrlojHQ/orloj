package agentruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestBuildContainerStdioTransportUsesEnvFileAndCidFile(t *testing.T) {
	manager := NewMcpSessionManager(nil)
	transport, err := manager.buildContainerStdioTransport(
		resources.McpServer{
			Metadata: resources.ObjectMeta{Name: "codex-github-official"},
			Spec: resources.McpServerSpec{
				Image:   "agentflow-github-mcp-server:test",
				Command: "server",
				Args:    []string{"stdio"},
			},
		},
		"server",
		&resolvedMcpEnv{EnvVars: []string{"GITHUB_PERSONAL_ACCESS_TOKEN=secret-token", "GITHUB_READ_ONLY=1"}},
		nil,
	)
	if err != nil {
		t.Fatalf("build container transport: %v", err)
	}
	stdio, ok := transport.(*StdioMcpTransport)
	if !ok {
		t.Fatalf("expected *StdioMcpTransport, got %T", transport)
	}

	args := strings.Join(stdio.args, " ")
	if strings.Contains(args, "secret-token") {
		t.Fatalf("docker args leaked secret: %s", args)
	}
	if mcpSessionContainsArg(stdio.args, "-e") {
		t.Fatalf("docker args should use --env-file, got %v", stdio.args)
	}

	envFile := argAfter(stdio.args, "--env-file")
	if envFile == "" {
		t.Fatalf("expected --env-file in docker args: %v", stdio.args)
	}
	cidFile := argAfter(stdio.args, "--cidfile")
	if cidFile == "" {
		t.Fatalf("expected --cidfile in docker args: %v", stdio.args)
	}
	if filepath.Dir(envFile) != filepath.Dir(cidFile) {
		t.Fatalf("expected env and cid files in same cleanup dir, got %q and %q", envFile, cidFile)
	}

	content, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	if got := string(content); !strings.Contains(got, "GITHUB_PERSONAL_ACCESS_TOKEN=secret-token\n") || !strings.Contains(got, "GITHUB_READ_ONLY=1\n") {
		t.Fatalf("env file missing expected values: %q", got)
	}

	cleanupDir := filepath.Dir(envFile)
	if err := stdio.Close(); err != nil {
		t.Fatalf("close transport: %v", err)
	}
	if _, err := os.Stat(cleanupDir); !os.IsNotExist(err) {
		t.Fatalf("expected cleanup dir removed, stat err=%v", err)
	}
}

func TestStdioTransportCleanupRunsOnceAfterStartFailureAndClose(t *testing.T) {
	called := 0
	transport := NewStdioMcpTransport(StdioMcpTransportConfig{
		Command: "definitely-not-a-real-mcp-command",
		OnClose: func() { called++ },
	})
	if _, err := transport.Initialize(context.Background()); err == nil {
		t.Fatal("expected initialize failure")
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("close transport: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected cleanup once, got %d", called)
	}
}

func argAfter(args []string, key string) string {
	for i, arg := range args {
		if arg == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func mcpSessionContainsArg(args []string, key string) bool {
	for _, arg := range args {
		if arg == key {
			return true
		}
	}
	return false
}
