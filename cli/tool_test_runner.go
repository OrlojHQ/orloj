package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/tetratelabs/wazero"
)

// TestFixture is a single test case read from a JSON fixture file.
type TestFixture struct {
	Name           string `json:"name"`
	Input          string `json:"input"`
	ExpectedStatus string `json:"expected_status"`
	ExpectedOutput string `json:"expected_output"`
	Timeout        string `json:"timeout"`
}

// TestResult holds the outcome of running a single fixture.
type TestResult struct {
	Fixture  TestFixture
	Passed   bool
	Output   string
	Status   string
	Duration time.Duration
	Error    string
}

// WASMTestRunner executes WASM tool modules against test fixtures.
type WASMTestRunner struct {
	modulePath  string
	fuelBudget  uint64
	memBudget   int64
}

// NewWASMTestRunner creates a test runner for the given module.
func NewWASMTestRunner(modulePath string, fuelBudget uint64, memBudget int64) *WASMTestRunner {
	return &WASMTestRunner{
		modulePath: modulePath,
		fuelBudget: fuelBudget,
		memBudget:  memBudget,
	}
}

// LoadFixtures reads all JSON fixture files from a directory.
func LoadFixtures(dir string) ([]TestFixture, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read fixtures dir %q: %w", dir, err)
	}
	var fixtures []TestFixture
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read fixture %q: %w", path, err)
		}
		var f TestFixture
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("parse fixture %q: %w", path, err)
		}
		if f.Name == "" {
			f.Name = strings.TrimSuffix(e.Name(), ".json")
		}
		fixtures = append(fixtures, f)
	}
	return fixtures, nil
}

// RunAll executes each fixture against the WASM module and returns results.
func (r *WASMTestRunner) RunAll(ctx context.Context, fixtures []TestFixture) []TestResult {
	results := make([]TestResult, 0, len(fixtures))
	for _, f := range fixtures {
		result := r.runOne(ctx, f)
		results = append(results, result)
	}
	return results
}

func (r *WASMTestRunner) runOne(ctx context.Context, fixture TestFixture) TestResult {
	timeout := 30 * time.Second
	if fixture.Timeout != "" {
		if parsed, err := time.ParseDuration(fixture.Timeout); err == nil {
			timeout = parsed
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	engine := wazero.NewRuntimeWithConfig(runCtx, wazero.NewRuntimeConfigInterpreter())
	defer engine.Close(runCtx)

	executor := agentruntime.NewWazeroExecutor(engine)
	start := time.Now()

	resp, err := executor.Execute(runCtx, agentruntime.WASMToolExecuteRequest{
		Tool:  "test",
		Input: fixture.Input,
		Runtime: agentruntime.WASMToolRuntimeConfig{
			ModulePath:     r.modulePath,
			Entrypoint:     "run",
			MaxMemoryBytes: r.memBudget,
			Fuel:           r.fuelBudget,
			EnableWASI:     true,
		},
	})
	duration := time.Since(start)

	result := TestResult{
		Fixture:  fixture,
		Duration: duration,
	}

	if err != nil {
		result.Error = err.Error()
		result.Passed = false
		return result
	}

	result.Output = resp.Output
	result.Status = "ok"

	passed := true
	var failures []string

	if fixture.ExpectedStatus != "" && result.Status != fixture.ExpectedStatus {
		passed = false
		failures = append(failures, fmt.Sprintf("status: got %q, want %q", result.Status, fixture.ExpectedStatus))
	}

	if fixture.ExpectedOutput != "" && strings.TrimSpace(resp.Output) != strings.TrimSpace(fixture.ExpectedOutput) {
		passed = false
		failures = append(failures, fmt.Sprintf("output: got %q, want %q", strings.TrimSpace(resp.Output), strings.TrimSpace(fixture.ExpectedOutput)))
	}

	if !passed {
		result.Passed = false
		result.Error = strings.Join(failures, "; ")
	} else {
		result.Passed = true
	}

	return result
}
