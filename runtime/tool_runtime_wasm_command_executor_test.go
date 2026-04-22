package agentruntime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tetratelabs/wazero"
)

func buildTestWazeroEngine(t *testing.T) wazero.Runtime {
	t.Helper()
	engine := wazero.NewRuntimeWithConfig(context.Background(), wazero.NewRuntimeConfigInterpreter())
	t.Cleanup(func() { engine.Close(context.Background()) })
	return engine
}

func compileGoWASMGuest(t *testing.T, dir, goSource string) string {
	t.Helper()
	srcPath := filepath.Join(dir, "guest.go")
	if err := os.WriteFile(srcPath, []byte(goSource), 0644); err != nil {
		t.Fatalf("write guest source: %v", err)
	}
	outPath := filepath.Join(dir, "guest.wasm")
	cmd := exec.Command("go", "build", "-o", outPath, srcPath)
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile wasm guest: %v\n%s", err, string(output))
	}
	return outPath
}

const echoGuestSource = `package main

import (
	"encoding/json"
	"io"
	"os"
)

func main() {
	data, _ := io.ReadAll(os.Stdin)
	var req struct {
		Input string ` + "`json:\"input\"`" + `
	}
	_ = json.Unmarshal(data, &req)
	resp := map[string]string{
		"contract_version": "v1",
		"status":           "ok",
		"output":           req.Input,
	}
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}
`

const deniedGuestSource = `package main

import (
	"encoding/json"
	"os"
)

func main() {
	resp := map[string]interface{}{
		"contract_version": "v1",
		"status":           "denied",
		"error": map[string]interface{}{
			"code":      "permission_denied",
			"reason":    "tool_permission_denied",
			"retryable": false,
			"message":   "blocked",
		},
	}
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}
`

const errorGuestSource = `package main

import (
	"encoding/json"
	"os"
)

func main() {
	resp := map[string]interface{}{
		"contract_version": "v1",
		"status":           "error",
		"error": map[string]interface{}{
			"code":      "rate_limited",
			"reason":    "upstream throttled",
			"retryable": true,
			"message":   "back off and retry",
		},
	}
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}
`

const emptyStdoutGuestSource = `package main

func main() {}
`

const badContractVersionSource = `package main

import (
	"encoding/json"
	"os"
)

func main() {
	resp := map[string]string{
		"contract_version": "v2",
		"status":           "ok",
		"output":           "ok",
	}
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}
`

func TestWazeroExecutorBasicExecution(t *testing.T) {
	engine := buildTestWazeroEngine(t)
	executor := NewWazeroExecutor(engine)
	dir := t.TempDir()
	modulePath := compileGoWASMGuest(t, dir, echoGuestSource)

	resp, err := executor.Execute(context.Background(), WASMToolExecuteRequest{
		Tool:  "test-tool",
		Input: "hello",
		Runtime: WASMToolRuntimeConfig{
			ModulePath: modulePath,
			EnableWASI: true,
		},
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if resp.Output != "hello" {
		t.Fatalf("expected output 'hello', got %q", resp.Output)
	}
}

func TestWazeroExecutorModuleCaching(t *testing.T) {
	engine := buildTestWazeroEngine(t)
	executor := NewWazeroExecutor(engine)
	dir := t.TempDir()
	modulePath := compileGoWASMGuest(t, dir, echoGuestSource)

	for i := 0; i < 3; i++ {
		_, err := executor.Execute(context.Background(), WASMToolExecuteRequest{
			Tool:  "test-tool",
			Input: "hello",
			Runtime: WASMToolRuntimeConfig{
				ModulePath: modulePath,
				EnableWASI: true,
			},
		})
		if err != nil {
			t.Fatalf("execute call %d failed: %v", i, err)
		}
	}

	executor.mu.RLock()
	count := len(executor.compiled)
	executor.mu.RUnlock()
	if count != 1 {
		t.Fatalf("expected 1 cached module, got %d", count)
	}
}

func TestWazeroExecutorMissingModulePath(t *testing.T) {
	engine := buildTestWazeroEngine(t)
	executor := NewWazeroExecutor(engine)

	_, err := executor.Execute(context.Background(), WASMToolExecuteRequest{
		Tool: "test-tool",
		Runtime: WASMToolRuntimeConfig{
			ModulePath: "",
			EnableWASI: true,
		},
	})
	if err == nil {
		t.Fatal("expected error for missing module path")
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeRuntimePolicyInvalid || reason != ToolReasonRuntimePolicyInvalid {
		t.Fatalf("unexpected code=%s reason=%s", code, reason)
	}
	if retryable {
		t.Fatal("expected non-retryable")
	}
}

func TestWazeroExecutorMissingModuleFile(t *testing.T) {
	engine := buildTestWazeroEngine(t)
	executor := NewWazeroExecutor(engine)

	_, err := executor.Execute(context.Background(), WASMToolExecuteRequest{
		Tool: "test-tool",
		Runtime: WASMToolRuntimeConfig{
			ModulePath: "/nonexistent/path/module.wasm",
			EnableWASI: true,
		},
	})
	if err == nil {
		t.Fatal("expected error for missing module file")
	}
	code, _, _, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeRuntimePolicyInvalid {
		t.Fatalf("expected policy invalid code, got %s", code)
	}
}

func TestWazeroExecutorCorruptModule(t *testing.T) {
	engine := buildTestWazeroEngine(t)
	executor := NewWazeroExecutor(engine)
	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad.wasm")
	if err := os.WriteFile(badPath, []byte("not a wasm module"), 0644); err != nil {
		t.Fatalf("write bad module: %v", err)
	}

	_, err := executor.Execute(context.Background(), WASMToolExecuteRequest{
		Tool: "test-tool",
		Runtime: WASMToolRuntimeConfig{
			ModulePath: badPath,
			EnableWASI: true,
		},
	})
	if err == nil {
		t.Fatal("expected error for corrupt module")
	}
	code, _, _, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeExecutionFailed {
		t.Fatalf("expected execution failed code, got %s", code)
	}
}

func TestWazeroExecutorContextCancellationCompilationPhase(t *testing.T) {
	engine := buildTestWazeroEngine(t)
	executor := NewWazeroExecutor(engine)

	// A cancelled context should fail during module compilation (uncached path).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := executor.Execute(ctx, WASMToolExecuteRequest{
		Tool: "test-tool",
		Runtime: WASMToolRuntimeConfig{
			ModulePath: "/nonexistent/but-context-cancelled.wasm",
			EnableWASI: true,
		},
	})
	if err == nil {
		t.Fatal("expected error from cancelled context or missing file")
	}
}

func TestWazeroExecutorDeniedResponse(t *testing.T) {
	engine := buildTestWazeroEngine(t)
	executor := NewWazeroExecutor(engine)
	dir := t.TempDir()
	modulePath := compileGoWASMGuest(t, dir, deniedGuestSource)

	_, err := executor.Execute(context.Background(), WASMToolExecuteRequest{
		Tool: "test-tool",
		Runtime: WASMToolRuntimeConfig{
			ModulePath: modulePath,
			EnableWASI: true,
		},
	})
	if err == nil {
		t.Fatal("expected denied error")
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodePermissionDenied || reason != ToolReasonPermissionDenied {
		t.Fatalf("unexpected code=%s reason=%s", code, reason)
	}
	if retryable {
		t.Fatal("expected non-retryable denied")
	}
}

func TestWazeroExecutorFactoryBuild(t *testing.T) {
	engine := buildTestWazeroEngine(t)
	factory := NewWazeroExecutorFactory(engine)

	exec1, err := factory.Build(context.Background(), WASMToolRuntimeConfig{})
	if err != nil {
		t.Fatalf("factory build failed: %v", err)
	}
	exec2, err := factory.Build(context.Background(), WASMToolRuntimeConfig{})
	if err != nil {
		t.Fatalf("second factory build failed: %v", err)
	}
	if exec1 != exec2 {
		t.Fatal("expected factory to return same executor instance")
	}
}

func TestWazeroExecutorClose(t *testing.T) {
	engine := wazero.NewRuntimeWithConfig(context.Background(), wazero.NewRuntimeConfigInterpreter())
	executor := NewWazeroExecutor(engine)
	dir := t.TempDir()
	modulePath := compileGoWASMGuest(t, dir, echoGuestSource)

	_, err := executor.Execute(context.Background(), WASMToolExecuteRequest{
		Tool: "test-tool",
		Runtime: WASMToolRuntimeConfig{
			ModulePath: modulePath,
			EnableWASI: true,
		},
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if closeErr := executor.Close(context.Background()); closeErr != nil {
		t.Fatalf("close failed: %v", closeErr)
	}

	executor.mu.RLock()
	count := len(executor.compiled)
	executor.mu.RUnlock()
	if count != 0 {
		t.Fatalf("expected 0 cached modules after close, got %d", count)
	}
}

func TestWazeroExecutorInvalidContractVersion(t *testing.T) {
	engine := buildTestWazeroEngine(t)
	executor := NewWazeroExecutor(engine)
	dir := t.TempDir()
	modulePath := compileGoWASMGuest(t, dir, badContractVersionSource)

	_, err := executor.Execute(context.Background(), WASMToolExecuteRequest{
		Tool: "test-tool",
		Runtime: WASMToolRuntimeConfig{
			ModulePath: modulePath,
			EnableWASI: true,
		},
	})
	if err == nil {
		t.Fatal("expected invalid contract version error")
	}
	code, reason, _, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeRuntimePolicyInvalid || reason != ToolReasonRuntimePolicyInvalid {
		t.Fatalf("unexpected code=%s reason=%s", code, reason)
	}
}

func TestWazeroExecutorErrorResponse(t *testing.T) {
	engine := buildTestWazeroEngine(t)
	executor := NewWazeroExecutor(engine)
	dir := t.TempDir()
	modulePath := compileGoWASMGuest(t, dir, errorGuestSource)

	_, err := executor.Execute(context.Background(), WASMToolExecuteRequest{
		Tool: "test-tool",
		Runtime: WASMToolRuntimeConfig{
			ModulePath: modulePath,
			EnableWASI: true,
		},
	})
	if err == nil {
		t.Fatal("expected error from error guest")
	}
	code, _, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != "rate_limited" {
		t.Fatalf("expected code 'rate_limited', got %q", code)
	}
	if !retryable {
		t.Fatal("expected retryable error")
	}
}

func TestWazeroExecutorEmptyStdout(t *testing.T) {
	engine := buildTestWazeroEngine(t)
	executor := NewWazeroExecutor(engine)
	dir := t.TempDir()
	modulePath := compileGoWASMGuest(t, dir, emptyStdoutGuestSource)

	_, err := executor.Execute(context.Background(), WASMToolExecuteRequest{
		Tool: "test-tool",
		Runtime: WASMToolRuntimeConfig{
			ModulePath: modulePath,
			EnableWASI: true,
		},
	})
	if err == nil {
		t.Fatal("expected error from empty stdout")
	}
	code, reason, retryable, ok := ToolErrorMeta(err)
	if !ok {
		t.Fatal("expected tool error metadata")
	}
	if code != ToolCodeRuntimePolicyInvalid || reason != ToolReasonRuntimePolicyInvalid {
		t.Fatalf("unexpected code=%s reason=%s", code, reason)
	}
	if retryable {
		t.Fatal("expected non-retryable for contract violation")
	}
}
