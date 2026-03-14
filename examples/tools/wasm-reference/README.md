# WASM Reference Tool Module

This directory provides a minimal runnable WASI guest and matching `Tool` manifest.

## Files

- `echo_guest.wat`: reference guest module with exported `run` function
- `wasm_echo_tool.yaml`: `Tool` CRD using `runtime.isolation_mode: wasm`

## Quick Check (module only)

```bash
wasmtime run --invoke run examples/tools/wasm-reference/echo_guest.wat
```

Expected stdout:

```json
{"contract_version":"v1","status":"ok","output":"reference wasm module"}
```

## Runtime Wiring

Use the wasm backend and point module path at this guest:

```bash
go run ./cmd/orlojworker \
  --tool-isolation-backend=wasm \
  --tool-wasm-module="$(pwd)/examples/tools/wasm-reference/echo_guest.wat" \
  --tool-wasm-entrypoint=run
```
