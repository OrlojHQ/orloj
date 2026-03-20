# WASM Tool Module Contract v1

Status: release-candidate contract targeted for Gate 0 stabilization.

This contract defines host-to-guest payloads for wasm-isolated tool execution.

## Scope

- Host runtime: `runtime/tool_runtime_wasm_command_executor.go`
- Guest module: WASI-compatible module that reads request JSON from stdin and writes response JSON to stdout
- Contract version: `v1`

## Request Envelope (stdin)

```json
{
  "contract_version": "v1",
  "namespace": "default",
  "tool": "wasm_echo",
  "input": "{\"query\":\"hello\"}",
  "capabilities": ["wasm.echo.invoke"],
  "risk_level": "high",
  "runtime": {
    "entrypoint": "run",
    "max_memory_bytes": 67108864,
    "fuel": 0,
    "enable_wasi": true
  }
}
```

## Response Envelope (stdout)

### Success

```json
{
  "contract_version": "v1",
  "status": "ok",
  "output": "result payload"
}
```

### Error

```json
{
  "contract_version": "v1",
  "status": "error",
  "error": {
    "code": "execution_failed",
    "reason": "tool_backend_failure",
    "retryable": true,
    "message": "guest execution failed",
    "details": {
      "guest_error": "example"
    }
  }
}
```

### Denied

```json
{
  "contract_version": "v1",
  "status": "denied",
  "error": {
    "code": "permission_denied",
    "reason": "tool_permission_denied",
    "retryable": false,
    "message": "blocked by policy"
  }
}
```

## Validation Rules

- `contract_version` is required and must be `v1`
- `status` must be one of `ok`, `error`, `denied`
- invalid/missing fields are classified as deterministic non-retryable runtime policy errors
- `error` fields map directly into canonical tool error metadata (`tool_code`, `tool_reason`, `retryable`)

## Reference Guest Module

- `examples/resources/tools/wasm-reference/echo_guest.wat`
- `examples/resources/tools/wasm-reference/README.md`

## Related Docs

- [Tool Contract v1](./tool-contract-v1.md)
- [Tool Runtime Conformance](../operations/tool-runtime-conformance.md)
