# Tools and Isolation

A **Tool** is an external capability that agents can invoke during execution. Orloj provides a standardized tool contract, multiple isolation backends, and runtime controls for timeout, retry, and risk classification.

## Defining a Tool

Tools are declared as resources that describe the tool's endpoint, auth requirements, risk level, and runtime configuration.

```yaml
apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: web_search
spec:
  type: http
  endpoint: https://api.search.com
  auth:
    secretRef: search-api-key
```

For tools that require isolation and runtime controls:

```yaml
apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: wasm_echo
spec:
  type: wasm
  capabilities:
    - wasm.echo.invoke
  risk_level: high
  runtime:
    isolation_mode: wasm
    timeout: 5s
    retry:
      max_attempts: 1
      backoff: 0s
      max_backoff: 1s
      jitter: none
```

### Key Fields

| Field | Description |
|---|---|
| `type` | Runtime type. Defaults to `http`. |
| `endpoint` | The tool's network endpoint. |
| `capabilities` | Declared operations this tool provides. Used for permission matching. |
| `risk_level` | `low`, `medium`, `high`, or `critical`. Affects default isolation mode. |
| `runtime.isolation_mode` | Execution isolation backend (see below). |
| `runtime.timeout` | Maximum execution time. Defaults to `30s`. |
| `runtime.retry` | Retry policy for failed invocations. |
| `auth.secretRef` | Reference to a Secret resource for tool authentication. |

## Isolation Modes

Orloj supports four isolation backends for tool execution, providing defense-in-depth for untrusted or high-risk tools.

| Mode | Description | Default for |
|---|---|---|
| `none` | Direct execution in the worker process. No isolation boundary. | `low` and `medium` risk tools |
| `sandboxed` | Restricted execution environment with limited syscall access. | `high` and `critical` risk tools |
| `container` | Each tool invocation runs in an isolated container. Full filesystem and network isolation. | Explicitly configured |
| `wasm` | Tool runs as a WebAssembly module with a host-guest stdin/stdout contract. Memory-safe and deterministic. | Explicitly configured |

The isolation mode defaults are based on `risk_level`:
- `low` or `medium` risk: defaults to `none`
- `high` or `critical` risk: defaults to `sandboxed`

You can always override the default by setting `runtime.isolation_mode` explicitly.

## Tool Contract v1

Every tool interaction follows a standardized request/response envelope. This contract ensures tools are portable, testable, and observable regardless of the isolation backend.

**Request envelope** (sent to the tool):
```json
{
  "request_id": "req-abc-123",
  "tool": "web_search",
  "action": "invoke",
  "parameters": {
    "query": "enterprise AI adoption trends"
  },
  "auth": {
    "type": "bearer",
    "token": "sk-..."
  },
  "context": {
    "task": "weekly-report",
    "agent": "research-agent",
    "attempt": 1
  }
}
```

**Response envelope** (returned from the tool):
```json
{
  "request_id": "req-abc-123",
  "status": "success",
  "result": {
    "data": "..."
  }
}
```

**Error response:**
```json
{
  "request_id": "req-abc-123",
  "status": "error",
  "error": {
    "tool_code": "rate_limited",
    "tool_reason": "API rate limit exceeded",
    "retryable": true
  }
}
```

The tool contract defines a canonical error taxonomy with `tool_code`, `tool_reason`, and `retryable` fields, enabling the runtime to make intelligent retry decisions.

## WASM Tools

WASM tools communicate over stdin/stdout using the same JSON envelope contract. The host writes the request to the module's stdin and reads the response from stdout. This provides memory-safe, deterministic execution with no filesystem or network access unless explicitly granted.

See the [WASM Tool Module Contract v1](../reference/wasm-tool-module-contract-v1.md) for the full specification.

## Retry and Timeout

Each tool can configure its own retry policy independently of the task-level retry:

```yaml
runtime:
  timeout: 5s
  retry:
    max_attempts: 3
    backoff: 1s
    max_backoff: 30s
    jitter: full
```

Retry uses capped exponential backoff. The `jitter` field controls randomization: `none` (deterministic), `full` (random between 0 and backoff), or `equal` (half deterministic, half random).

## Governance Integration

Tool invocations are gated by the [governance layer](./governance.md). An agent must have the required permissions (via AgentRole) to invoke a tool, and the tool must not be blocked by any applicable AgentPolicy. Unauthorized calls fail closed with a `tool_permission_denied` error.

## Related Resources

- [Resource Reference: Tool](../reference/crds.md)
- [Tool Contract v1](../reference/tool-contract-v1.md)
- [WASM Tool Module Contract v1](../reference/wasm-tool-module-contract-v1.md)
- [Tool Runtime Conformance](../operations/tool-runtime-conformance.md)
- [Guide: Build a Custom Tool](../guides/build-custom-tool.md)
