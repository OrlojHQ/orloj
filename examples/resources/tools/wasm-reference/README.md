# WASM Reference Tool Module

This directory provides a reference WASI guest and matching `Tool` manifest
that demonstrates the Orloj WASM tool contract v1.

## Files

- `echo_guest.go`: Go source for a WASI guest that reads the JSON contract from stdin and echoes the input back
- `echo_guest.wat`: original WAT reference guest (writes a hardcoded response; kept for reference)
- `echo_guest.wasm`: compiled WASI binary (build with the command below)
- `wasm_echo_tool.yaml`: `Tool` CRD with per-tool `spec.wasm` configuration

## Building the Guest

```bash
GOOS=wasip1 GOARCH=wasm go build -o examples/resources/tools/wasm-reference/echo_guest.wasm \
  ./examples/resources/tools/wasm-reference/echo_guest.go
```

## Quick Check

```bash
echo '{"contract_version":"v1","tool":"wasm-echo","input":"hello"}' | \
  go run ./cmd/orlojd --tool-wasm-module=examples/resources/tools/wasm-reference/echo_guest.wasm
```

Or test in isolation using `wasmtime` (if installed):

```bash
echo '{"contract_version":"v1","tool":"wasm-echo","input":"hello"}' | \
  wasmtime run examples/resources/tools/wasm-reference/echo_guest.wasm
```

Expected stdout:

```json
{"contract_version":"v1","status":"ok","output":"hello"}
```

## Per-Tool Configuration

The embedded wazero runtime reads WASM settings from each tool's `spec.wasm` block.
See `wasm_echo_tool.yaml` for the full example:

```yaml
spec:
  type: wasm
  wasm:
    module: examples/resources/tools/wasm-reference/echo_guest.wasm
    entrypoint: run
    max_memory_bytes: 16777216  # 16 MB
    fuel: 100000
    enable_wasi: true
```

The embedded runtime (wazero) is always available -- no external binary required
and no `--tool-isolation-backend` configuration needed for WASM tools.
