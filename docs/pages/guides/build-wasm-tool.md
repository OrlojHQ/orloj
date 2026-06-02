# Build a WASM Tool

This guide walks through building and deploying a WebAssembly tool that agents can invoke through Orloj's embedded wazero runtime. WASM tools execute in-process with host-enforced resource limits -- no external runtime binary required.

## Prerequisites

- Orloj server (`orlojd`) and at least one worker running
- `orlojctl` available
- A language toolchain that can compile to WebAssembly (Go, Rust, C/C++, Zig, AssemblyScript, etc.)
- Familiarity with [Tools and Isolation](../concepts/tools/tool.md) concepts

## How WASM Tools Work

WASM tools communicate with the host over **stdin/stdout** using a JSON contract (v1):

1. Orloj writes a JSON request to the module's **stdin**.
2. The module reads the request, does its work, and writes a JSON response to **stdout**.
3. The host reads the response and passes the result back to the agent.

The module runs inside an embedded [wazero](https://wazero.io) runtime (pure Go, zero CGO). Memory, CPU fuel, and I/O access are enforced by the host, not by the guest. The module cannot escape its sandbox.

## The WASM Tool Contract v1

### Request (host writes to stdin)

The host sends a single JSON object:

```json
{
  "contract_version": "v1",
  "namespace": "production",
  "tool": "my-wasm-tool",
  "input": "{\"query\": \"search term\"}",
  "capabilities": ["wasm.my-tool.invoke"],
  "risk_level": "low",
  "runtime": {
    "entrypoint": "run",
    "max_memory_bytes": 67108864,
    "fuel": 1000000,
    "enable_wasi": true
  },
  "auth": {
    "profile": "bearer",
    "headers": {
      "Authorization": "Bearer sk-..."
    }
  }
}
```

| Field | Type | Description |
|---|---|---|
| `contract_version` | string | Always `"v1"`. |
| `namespace` | string | The Orloj namespace the tool is running in. |
| `tool` | string | The tool resource name. |
| `input` | string | The agent's tool input, serialized as a JSON string. |
| `capabilities` | string[] | Declared capabilities from the Tool manifest. |
| `risk_level` | string | `low`, `medium`, `high`, or `critical`. |
| `runtime` | object | Resource limits and entrypoint (informational; enforced by host). |
| `auth` | object | Auth profile and resolved headers. Only present if `spec.auth` is configured on the Tool. |

The `input` field is a **string** containing serialized JSON from the agent. Your module should parse it to extract parameters.

### Response (module writes to stdout)

The module must write exactly one JSON object to stdout.

**Success:**

```json
{
  "contract_version": "v1",
  "status": "ok",
  "output": "The result of the tool invocation."
}
```

**Error (retryable):**

```json
{
  "contract_version": "v1",
  "status": "error",
  "error": {
    "code": "rate_limited",
    "reason": "upstream API throttled",
    "message": "try again in 5s",
    "retryable": true
  }
}
```

**Denied:**

```json
{
  "contract_version": "v1",
  "status": "denied",
  "error": {
    "code": "permission_denied",
    "reason": "insufficient scope",
    "message": "tool requires admin access",
    "retryable": false
  }
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `contract_version` | string | yes | Must be `"v1"`. |
| `status` | string | yes | `"ok"`, `"error"`, or `"denied"`. |
| `output` | string | on success | The tool result returned to the agent. |
| `error` | object | on error/denied | Structured error with `code`, `reason`, `message`, `retryable`, and optional `details` (map of string to string). |

The `error.code` and `error.retryable` fields drive the runtime's retry and dead-letter behavior. Use the [error taxonomy](../concepts/tools/tool.md#error-taxonomy) codes when applicable.

### WASI and `proc_exit`

When WASI is enabled (`enable_wasi: true`), the guest has access to stdin, stdout, and stderr. Go and Rust WASI guests typically call `proc_exit(0)` on normal termination. The host treats `proc_exit(0)` as success -- only non-zero exit codes are treated as failures.

## Step 1: Write a Guest Module

Any language that compiles to WASM can produce a guest. The simplest path is Go targeting `wasip1`.

### Go

```go
package main

import (
    "encoding/json"
    "io"
    "os"
)

type request struct {
    ContractVersion string `json:"contract_version"`
    Tool            string `json:"tool"`
    Input           string `json:"input"`
}

type response struct {
    ContractVersion string `json:"contract_version"`
    Status          string `json:"status"`
    Output          string `json:"output"`
}

type errorResponse struct {
    ContractVersion string      `json:"contract_version"`
    Status          string      `json:"status"`
    Error           errorDetail `json:"error"`
}

type errorDetail struct {
    Code    string `json:"code"`
    Message string `json:"message"`
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
```

Build:

```bash
GOOS=wasip1 GOARCH=wasm go build -o my-tool.wasm my_tool.go
```

### Rust

```rust
use std::io::{self, Read};

fn main() {
    let mut input = String::new();
    io::stdin().read_to_string(&mut input).unwrap();

    // Parse JSON (use serde_json in real code)
    // ... your tool logic ...

    println!(r#"{{"contract_version":"v1","status":"ok","output":"result here"}}"#);
}
```

Build with the WASI target:

```bash
cargo build --target wasm32-wasip1 --release
cp target/wasm32-wasip1/release/my_tool.wasm .
```

### Other Languages

Any language with a WASI compilation target works: C/C++ (via wasi-sdk or Emscripten), Zig (`-target wasm32-wasi`), AssemblyScript, etc. The only requirement is reading JSON from stdin and writing JSON to stdout.

## Step 2: Register the Tool

Create a Tool manifest with `type: wasm` and a `spec.wasm` block:

```yaml
apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: my-wasm-tool
spec:
  type: wasm
  wasm:
    module: my-tool.wasm
    entrypoint: run
    max_memory_bytes: 67108864   # 64 MB (default)
    fuel: 1000000                # Execution step limit (default: 1M)
    enable_wasi: true            # Required for stdin/stdout
  capabilities:
    - wasm.my-tool.invoke
  risk_level: low
  runtime:
    isolation_mode: wasm
    timeout: 5s
```

Apply:

```bash
orlojctl apply -f my-wasm-tool.yaml
```

### `spec.wasm` Fields

| Field | Default | Description |
|---|---|---|
| `module` | *(required)* | Relative path under `--tool-wasm-cache-dir`, HTTPS URL, or OCI artifact reference (`oci://...`) to the `.wasm` module. |
| `entrypoint` | `run` | Exported function name to invoke. |
| `max_memory_bytes` | `67108864` (64 MB) | Maximum WASM linear memory. Host-enforced. |
| `fuel` | `1000000` (1M) | Execution fuel limit. Prevents runaway modules. Host-enforced. |
| `enable_wasi` | `false` | Enable WASI (stdin/stdout/stderr). Most tools need this set to `true`. |
| `image_pull_secret` | *(optional)* | Name of a Secret containing registry credentials for pulling OCI-referenced modules. The Secret must have `username` and `password` keys. |

#### Module reference formats

The `module` field accepts three formats:

- **Local path** (relative): `my-tool.wasm` or `tools/echo.wasm` — resolved under `--tool-wasm-cache-dir` (default `~/.orloj/wasm-cache`). Absolute paths, `..` segments, and paths outside the cache directory are rejected.
- **HTTPS URL**: `https://artifacts.example.com/tools/echo-v1.2.wasm` — plain `http://` URLs are not supported.
- **OCI reference**: `oci://ghcr.io/orloj-tools/echo:v1.2`

Remote modules (HTTPS and OCI) are fetched once and cached on disk in `--tool-wasm-cache-dir`, keyed by SHA-256 of the reference. Subsequent invocations use the cached copy.

For local paths, copy or mount the `.wasm` file into `--tool-wasm-cache-dir` (or a subdirectory). In a containerized deployment, mount the cache directory or place modules under it at startup.

#### Private OCI registries

For private OCI registries, set `image_pull_secret` to reference a Secret with `username` and `password` keys:

```yaml
spec:
  wasm:
    module: oci://ghcr.io/my-org/private-tool:v1
    image_pull_secret: ghcr-creds
```

## Step 3: Grant Agent Access

Add the tool to an agent's `tools` list:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research-agent
spec:
  model_ref: openai-default
  tools:
    - web_search
    - my-wasm-tool
  limits:
    max_steps: 10
    timeout: 60s
```

If governance is enabled, you also need a `ToolPermission` and an `AgentRole`. See the [governance guide](./setup-governance.md).

## Step 4: Test Locally

You can test the guest contract without running Orloj by piping JSON to the WASM binary.

Using Go's WASI runner:

```bash
echo '{"contract_version":"v1","tool":"my-wasm-tool","input":"hello"}' | \
  GOOS=wasip1 GOARCH=wasm go run my_tool.go
```

Using wasmtime (if installed):

```bash
echo '{"contract_version":"v1","tool":"my-wasm-tool","input":"hello"}' | \
  wasmtime run my-tool.wasm
```

Expected output:

```json
{"contract_version":"v1","status":"ok","output":"processed: hello"}
```

## Resource Limits

All limits are enforced by the **host**, not the guest module. A malicious or buggy guest cannot override them.

| Limit | What it controls | Default |
|---|---|---|
| `max_memory_bytes` | WASM linear memory ceiling | 64 MB |
| `fuel` | Execution step budget (prevents infinite loops) | 1,000,000 |
| `runtime.timeout` | Wall-clock timeout for the entire invocation | 30s |
| `enable_wasi` | When `false`, the module has no access to stdin/stdout/stderr | `false` |

If fuel is exhausted before the module completes, the host terminates execution and returns an error to the agent.

## Coexistence with Container Tools

WASM tools run on a dedicated runtime slot, independent of the `--tool-isolation-backend` flag. You can mix WASM tools and container-isolated tools (including MCP servers, CLI tools, etc.) in the same agent system:

```yaml
spec:
  tools:
    - my-wasm-tool          # Runs in wazero (always available)
    - kubectl-get            # Runs in container (requires --tool-isolation-backend=container)
    - web_search             # Runs as HTTP (no isolation)
```

## Error Handling Best Practices

1. **Always write a response.** If your module exits without writing to stdout, the host treats it as a contract violation.
2. **Use `contract_version: "v1"` and a valid `status`.** Missing or unsupported values cause a contract error.
3. **Prefer structured errors over panics.** Write an error response JSON instead of crashing. Panics produce an opaque host-level error with no retry information.
4. **Set `retryable` accurately.** The runtime uses this field to decide whether to retry or dead-letter the invocation.

## Scaffold a New Tool

Use `orlojctl tool scaffold` to generate a ready-to-build project:

```bash
orlojctl tool scaffold my-echo --lang go
```

This creates a `my-echo/` directory with a contract-compliant guest module, Makefile, tool manifest, test fixtures, and a README. Supported languages: `go`, `rust`.

## Test a Tool

Use `orlojctl tool test` to validate a WASM module against fixture files:

```bash
orlojctl tool test my-echo.wasm --fixtures fixtures/
```

Each fixture is a JSON file specifying input, expected status, and expected output:

```json
{
  "name": "echo hello",
  "input": "{\"query\": \"hello\"}",
  "expected_status": "ok",
  "expected_output": "processed: {\"query\": \"hello\"}",
  "timeout": "5s"
}
```

The test runner validates the contract (v1, valid status), asserts expected output, and reports pass/fail with timing.

Options:

| Flag | Default | Description |
|---|---|---|
| `--fixtures` | `fixtures/` | Directory containing JSON fixture files. |
| `--fuel-budget` | `1000000` | Maximum fuel per fixture run. |
| `--memory-budget` | `67108864` | Maximum memory bytes per fixture run. |

## Observability

WASM tool execution emits Prometheus metrics automatically:

| Metric | Type | Labels | Description |
|---|---|---|---|
| `orloj_tool_execution_duration_seconds` | Histogram | `tool`, `type`, `status` | Duration of tool execution (all types). |
| `orloj_wasm_fuel_consumed` | Counter | `tool` | Total fuel consumed. |
| `orloj_wasm_compilation_cache_hits_total` | Counter | `tool` | Module compilation cache hits. |
| `orloj_wasm_compilation_cache_misses_total` | Counter | `tool` | Module compilation cache misses. |
| `orloj_wasm_module_fetch_duration_seconds` | Histogram | `source` | Remote module fetch duration. |

## Reference Example

A complete working example lives in the repository:

- **Guest source**: `examples/resources/tools/wasm-reference/echo_guest.go`
- **Tool manifest**: `examples/resources/tools/wasm-reference/wasm_echo_tool.yaml`
- **README with build instructions**: `examples/resources/tools/wasm-reference/README.md`

## Next Steps

- [Tool Concepts](../concepts/tools/tool.md) -- tool types, isolation modes, and the error taxonomy
- [Build a Custom Tool](./build-custom-tool.md) -- HTTP, gRPC, external, and webhook-callback tool types
- [Connect an MCP Server](./connect-mcp-server.md) -- auto-discover tools from MCP servers
