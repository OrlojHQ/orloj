package agentruntime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBoundedHelpersDoNotLeakWhenRuntimeHonorsContext(t *testing.T) {
	testCases := []string{
		"governed",
		"container",
		"wasm",
	}
	for _, mode := range testCases {
		t.Run(mode, func(t *testing.T) {
			if os.Getenv("ORLOJ_BOUNDED_LEAK_CHILD") == mode {
				baseline := runtime.NumGoroutine()
				for i := 0; i < 4; i++ {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
					switch mode {
					case "governed":
						_, _ = callToolRuntimeBounded(ctx, ctxAwareBlockingToolRuntime{}, "tool", "input")
					case "container":
						_, _, _ = runContainerCommandBounded(ctx, ctxAwareBlockingContainerRunner{}, "docker", []string{"run"}, "", nil)
					case "wasm":
						_, _ = executeWASMToolBounded(ctx, ctxAwareBlockingWASMExecutor{}, WASMToolExecuteRequest{Tool: "tool"})
					}
					cancel()
				}
				time.Sleep(50 * time.Millisecond)
				fmt.Printf("%d\n", runtime.NumGoroutine()-baseline)
				return
			}

			cmd := exec.Command(os.Args[0], "-test.run=^TestBoundedHelpersDoNotLeakWhenRuntimeHonorsContext$/^"+mode+"$")
			cmd.Env = append(os.Environ(), "ORLOJ_BOUNDED_LEAK_CHILD="+mode)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("bounded helper subprocess failed: %v\n%s", err, output)
			}
			fields := strings.Fields(string(output))
			if len(fields) == 0 {
				t.Fatalf("expected goroutine delta output, got %q", output)
			}
			delta, err := strconv.Atoi(fields[0])
			if err != nil {
				t.Fatalf("parse goroutine delta failed: %v\n%s", err, output)
			}
			if delta > 0 {
				t.Fatalf("expected no leaked goroutines for %s helper, got delta %d", mode, delta)
			}
		})
	}
}

type ctxAwareBlockingToolRuntime struct{}

func (ctxAwareBlockingToolRuntime) Call(ctx context.Context, _ string, _ string) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

type ctxAwareBlockingContainerRunner struct{}

func (ctxAwareBlockingContainerRunner) Run(ctx context.Context, _ string, _ []string, _ string, _ map[string]string) (string, string, error) {
	<-ctx.Done()
	return "", "", ctx.Err()
}

type ctxAwareBlockingWASMExecutor struct{}

func (ctxAwareBlockingWASMExecutor) Execute(ctx context.Context, _ WASMToolExecuteRequest) (WASMToolExecuteResponse, error) {
	<-ctx.Done()
	return WASMToolExecuteResponse{}, ctx.Err()
}
