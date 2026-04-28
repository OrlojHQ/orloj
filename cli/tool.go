package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newToolCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tool",
		Short: "WASM tool development utilities",
	}
	cmd.AddCommand(newToolScaffoldCommand())
	cmd.AddCommand(newToolTestCommand())
	return cmd
}

func newToolTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <module.wasm>",
		Short: "Run test fixtures against a WASM tool module",
		Args:  cobra.ExactArgs(1),
		RunE:  runToolTest,
	}
	cmd.Flags().String("fixtures", "fixtures/", "Directory containing JSON test fixture files")
	cmd.Flags().Uint64("fuel-budget", 1_000_000, "Maximum fuel per fixture execution")
	cmd.Flags().Int64("memory-budget", 67108864, "Maximum memory bytes per fixture execution")
	return cmd
}

func runToolTest(cmd *cobra.Command, args []string) error {
	modulePath := strings.TrimSpace(args[0])
	if modulePath == "" {
		return errors.New("module path cannot be empty")
	}
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return fmt.Errorf("module %q not found", modulePath)
	}

	fixturesDir, _ := cmd.Flags().GetString("fixtures")
	fuelBudget, _ := cmd.Flags().GetUint64("fuel-budget")
	memoryBudget, _ := cmd.Flags().GetInt64("memory-budget")

	fixtures, err := LoadFixtures(fixturesDir)
	if err != nil {
		return err
	}
	if len(fixtures) == 0 {
		return fmt.Errorf("no fixtures found in %q", fixturesDir)
	}

	runner := NewWASMTestRunner(modulePath, fuelBudget, memoryBudget)
	results := runner.RunAll(cmd.Context(), fixtures)

	passed, failed := 0, 0
	for _, r := range results {
		if r.Passed {
			passed++
			fmt.Printf("  ✓ %s (%s)\n", r.Fixture.Name, r.Duration.Round(time.Millisecond))
		} else {
			failed++
			fmt.Printf("  ✗ %s (%s)\n", r.Fixture.Name, r.Duration.Round(time.Millisecond))
			if r.Error != "" {
				fmt.Printf("    error: %s\n", r.Error)
			}
		}
	}

	fmt.Printf("\n%d passed, %d failed, %d total\n", passed, failed, len(results))
	if failed > 0 {
		return fmt.Errorf("%d fixture(s) failed", failed)
	}
	return nil
}

func newToolScaffoldCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scaffold <name>",
		Short: "Generate a ready-to-build WASM tool project",
		Args:  cobra.ExactArgs(1),
		RunE:  runToolScaffold,
	}
	cmd.Flags().String("lang", "go", "Language for the guest module: go, rust")
	cmd.Flags().String("dir", ".", "Parent directory for the scaffolded project")
	return cmd
}

func runToolScaffold(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if name == "" {
		return errors.New("name cannot be empty")
	}

	lang, _ := cmd.Flags().GetString("lang")
	lang = strings.ToLower(strings.TrimSpace(lang))

	parentDir, _ := cmd.Flags().GetString("dir")
	parentDir = strings.TrimSpace(parentDir)
	if parentDir == "" {
		parentDir = "."
	}

	var files map[string]string
	switch lang {
	case "go":
		files = scaffoldGoWasmTool(name)
	case "rust":
		files = scaffoldRustWasmTool(name)
	default:
		return fmt.Errorf("unsupported language %q (expected: go, rust)", lang)
	}

	outDir := filepath.Join(parentDir, name)
	fixturesDir := filepath.Join(outDir, "fixtures")
	if err := os.MkdirAll(fixturesDir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", outDir, err)
	}

	if lang == "rust" {
		srcDir := filepath.Join(outDir, "src")
		if err := os.MkdirAll(srcDir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", srcDir, err)
		}
	}

	for path, content := range files {
		fullPath := filepath.Join(outDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Printf("  created %s\n", filepath.Join(outDir, path))
	}

	fmt.Printf("\nScaffolded WASM tool %q (%s) in %s/\n", name, lang, outDir)
	fmt.Printf("\nTo build and deploy:\n")
	switch lang {
	case "go":
		fmt.Printf("  cd %s && make build\n", outDir)
	case "rust":
		fmt.Printf("  cd %s && make build\n", outDir)
	}
	fmt.Printf("  orlojctl apply -f %s/tool.yaml\n", outDir)
	fmt.Printf("\nTo test:\n")
	fmt.Printf("  orlojctl tool test %s/%s.wasm --fixtures %s/fixtures/\n", outDir, name, outDir)
	return nil
}

func scaffoldGoWasmTool(name string) map[string]string {
	modName := strings.ReplaceAll(name, "-", "_")
	return map[string]string{
		"guest.go": fmt.Sprintf(`package main

import (
	"encoding/json"
	"io"
	"os"
)

type request struct {
	ContractVersion string %[1]sjson:"contract_version"%[1]s
	Tool            string %[1]sjson:"tool"%[1]s
	Input           string %[1]sjson:"input"%[1]s
}

type response struct {
	ContractVersion string %[1]sjson:"contract_version"%[1]s
	Status          string %[1]sjson:"status"%[1]s
	Output          string %[1]sjson:"output"%[1]s
}

type errorResponse struct {
	ContractVersion string      %[1]sjson:"contract_version"%[1]s
	Status          string      %[1]sjson:"status"%[1]s
	Error           errorDetail %[1]sjson:"error"%[1]s
}

type errorDetail struct {
	Code    string %[1]sjson:"code"%[1]s
	Message string %[1]sjson:"message"%[1]s
}

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		writeError("guest_error", "failed to read stdin: "+err.Error())
		return
	}

	var req request
	if err := json.Unmarshal(data, &req); err != nil {
		writeError("guest_error", "failed to parse request: "+err.Error())
		return
	}

	// --- Your tool logic here ---
	result := "processed: " + req.Input

	resp := response{
		ContractVersion: "v1",
		Status:          "ok",
		Output:          result,
	}
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}

func writeError(code, msg string) {
	resp := errorResponse{
		ContractVersion: "v1",
		Status:          "error",
		Error:           errorDetail{Code: code, Message: msg},
	}
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}
`, "`"),
		"go.mod": fmt.Sprintf(`module %s

go 1.25.0
`, modName),
		"Makefile": fmt.Sprintf(`.PHONY: build test clean

build:
	GOOS=wasip1 GOARCH=wasm go build -o %s.wasm guest.go

test: build
	orlojctl tool test %s.wasm --fixtures fixtures/

clean:
	rm -f %s.wasm
`, name, name, name),
		"tool.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: %s
spec:
  type: wasm
  wasm:
    module: %s.wasm
    entrypoint: run
    max_memory_bytes: 67108864
    fuel: 1000000
    enable_wasi: true
  capabilities:
    - wasm.%s.invoke
  risk_level: low
  runtime:
    isolation_mode: wasm
    timeout: 5s
`, name, name, name),
		"fixtures/test_echo.json": `{
  "name": "echo hello",
  "input": "{\"query\": \"hello\"}",
  "expected_status": "ok",
  "expected_output": "processed: {\"query\": \"hello\"}",
  "timeout": "5s"
}
`,
		"README.md": fmt.Sprintf(`# %s

A WASM tool for Orloj agents.

## Prerequisites

- Go 1.25+ (with wasip1/wasm support)
- ` + "`orlojctl`" + ` available on your PATH

## Build

` + "```bash" + `
make build
` + "```" + `

This compiles ` + "`guest.go`" + ` to ` + "`%s.wasm`" + ` using GOOS=wasip1 GOARCH=wasm.

## Test

` + "```bash" + `
make test
` + "```" + `

Runs the WASM module against fixture files in ` + "`fixtures/`" + `.

## Deploy

1. Copy ` + "`%s.wasm`" + ` to a location accessible by the Orloj worker.
2. Update the ` + "`module`" + ` path in ` + "`tool.yaml`" + `:

` + "```yaml" + `
spec:
  wasm:
    module: /opt/orloj/tools/%s.wasm
` + "```" + `

3. Apply:

` + "```bash" + `
orlojctl apply -f tool.yaml
` + "```" + `

4. Add ` + "`%s`" + ` to an agent's ` + "`tools`" + ` list.
`, name, name, name, name, name),
	}
}

func scaffoldRustWasmTool(name string) map[string]string {
	crateName := strings.ReplaceAll(name, "-", "_")
	return map[string]string{
		"src/main.rs": `use std::io::{self, Read, Write};

fn main() {
    let mut input = String::new();
    if let Err(e) = io::stdin().read_to_string(&mut input) {
        write_error("guest_error", &format!("failed to read stdin: {}", e));
        return;
    }

    // Parse the JSON request. In production, use serde_json.
    // This minimal implementation extracts the "input" field.
    let result = format!("processed: {}", input.trim());

    let response = format!(
        r#"{{"contract_version":"v1","status":"ok","output":"{}"}}"#,
        result.replace('"', r#"\""#)
    );
    let _ = io::stdout().write_all(response.as_bytes());
    let _ = io::stdout().write_all(b"\n");
}

fn write_error(code: &str, msg: &str) {
    let response = format!(
        r#"{{"contract_version":"v1","status":"error","error":{{"code":"{}","message":"{}"}}}}"#,
        code,
        msg.replace('"', r#"\""#)
    );
    let _ = io::stdout().write_all(response.as_bytes());
    let _ = io::stdout().write_all(b"\n");
}
`,
		"Cargo.toml": fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[[bin]]
name = "%s"
path = "src/main.rs"
`, crateName, crateName),
		"Makefile": fmt.Sprintf(`.PHONY: build test clean

build:
	cargo build --target wasm32-wasip1 --release
	cp target/wasm32-wasip1/release/%s.wasm .

test: build
	orlojctl tool test %s.wasm --fixtures fixtures/

clean:
	cargo clean
	rm -f %s.wasm
`, crateName, crateName, crateName),
		"tool.yaml": fmt.Sprintf(`apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: %s
spec:
  type: wasm
  wasm:
    module: %s.wasm
    entrypoint: run
    max_memory_bytes: 67108864
    fuel: 1000000
    enable_wasi: true
  capabilities:
    - wasm.%s.invoke
  risk_level: low
  runtime:
    isolation_mode: wasm
    timeout: 5s
`, name, crateName, name),
		"fixtures/test_echo.json": `{
  "name": "echo hello",
  "input": "{\"query\": \"hello\"}",
  "expected_status": "ok",
  "timeout": "5s"
}
`,
		"README.md": fmt.Sprintf(`# %s

A WASM tool for Orloj agents, written in Rust.

## Prerequisites

- Rust toolchain with `+"`wasm32-wasip1`"+` target:

`+"```bash"+`
rustup target add wasm32-wasip1
`+"```"+`

- `+"`orlojctl`"+` available on your PATH

## Build

`+"```bash"+`
make build
`+"```"+`

This compiles to `+"`target/wasm32-wasip1/release/%s.wasm`"+` and copies it to the project root.

## Test

`+"```bash"+`
make test
`+"```"+`

Runs the WASM module against fixture files in `+"`fixtures/`"+`.

## Deploy

1. Copy `+"`%s.wasm`"+` to a location accessible by the Orloj worker.
2. Update the `+"`module`"+` path in `+"`tool.yaml`"+`.
3. Apply:

`+"```bash"+`
orlojctl apply -f tool.yaml
`+"```"+`

4. Add `+"`%s`"+` to an agent's `+"`tools`"+` list.
`, name, crateName, crateName, name),
	}
}
