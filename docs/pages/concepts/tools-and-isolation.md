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
| `type` | Tool type. Determines the transport and execution model. See below. |
| `endpoint` | The tool's network endpoint. |
| `capabilities` | Declared operations this tool provides. Used for permission matching. |
| `risk_level` | `low`, `medium`, `high`, or `critical`. Affects default isolation mode. |
| `runtime.isolation_mode` | Execution isolation backend (see below). |
| `runtime.timeout` | Maximum execution time. Defaults to `30s`. |
| `runtime.retry` | Retry policy for failed invocations. |
| `auth.secretRef` | Reference to a Secret resource for tool authentication. |

## Tool Types

`Tool.spec.type` determines how the runtime communicates with the tool. Five types are supported:

| Type | Transport | Contract | Use case |
|---|---|---|---|
| `http` | HTTP POST to `endpoint` | Raw body or `ToolExecutionResponse` | Simple API integrations. Default when omitted. |
| `external` | HTTP POST to `endpoint` | Strict `ToolExecutionRequest` / `ToolExecutionResponse` | Tools running as standalone microservices that need the full execution context. |
| `grpc` | Unary gRPC call to `endpoint` | `ToolExecutionRequest` / `ToolExecutionResponse` as JSON over `orloj.tool.v1.ToolService/Execute` | Teams that prefer gRPC for tool communication. |
| `webhook-callback` | HTTP POST to `endpoint`, then poll `{endpoint}/{request_id}` | `ToolExecutionRequest` / `ToolExecutionResponse` | Long-running tools, batch jobs, or tools that require human-in-the-loop steps. |
| `queue` | Reserved for future use | -- | Planned for message-queue-backed tools. |

All types flow through the same governed runtime pipeline -- policy enforcement, retry, timeout, auth injection, and error taxonomy behave identically regardless of tool type.

Unknown type values are rejected at apply time.

### HTTP (default)

The `http` type sends the agent's tool input as an HTTP POST body to `spec.endpoint`. The runtime accepts both raw text responses and structured `ToolExecutionResponse` JSON envelopes. Auth is injected as an `Authorization: Bearer` header when `auth.secretRef` is configured.

### External

The `external` type sends the full `ToolExecutionRequest` contract envelope as JSON to `spec.endpoint` and expects a `ToolExecutionResponse` back. This gives the external service access to the full execution context (task ID, agent, namespace, trace IDs, attempt number). Use this when your tool needs to be aware of the Orloj execution context.

### gRPC

The `grpc` type calls `orloj.tool.v1.ToolService/Execute` as a unary gRPC method on `spec.endpoint`, using a JSON codec. The request and response payloads are the same `ToolExecutionRequest` / `ToolExecutionResponse` envelopes as `external`. Use this when your tool infrastructure is gRPC-native.

### Webhook-Callback

The `webhook-callback` type supports asynchronous tool execution:

1. The runtime POSTs a `ToolExecutionRequest` to `spec.endpoint`.
2. The tool returns `202 Accepted` (or `200 OK` with an immediate result).
3. If `202`: the runtime polls `{endpoint}/{request_id}` at regular intervals until a `ToolExecutionResponse` with a terminal status arrives, or the configured timeout expires.

This is useful for tools that take minutes to complete (e.g., batch processing, code review, CI pipeline triggers) or that require external approval before returning a result.

## Isolation Modes

Isolation modes control the execution boundary of a tool, independent of tool type.

| Mode | Description | Default for |
|---|---|---|
| `none` | Direct execution in the worker process. The `http` type makes real HTTP calls; other types use their respective transports. | `low` and `medium` risk tools |
| `sandboxed` | Restricted container execution with secure defaults: read-only filesystem, no capabilities, no privilege escalation, no network, non-root user, memory/CPU/pids limits. | `high` and `critical` risk tools |
| `container` | Each tool invocation runs in an isolated container. Full filesystem and network isolation. | Explicitly configured |
| `wasm` | Tool runs as a WebAssembly module with a host-guest stdin/stdout contract. Memory-safe and deterministic. | Explicitly configured |

The isolation mode defaults are based on `risk_level`:
- `low` or `medium` risk: defaults to `none`
- `high` or `critical` risk: defaults to `sandboxed`

You can always override the default by setting `runtime.isolation_mode` explicitly.

### Sandboxed Defaults

When `isolation_mode` is `sandboxed`, the container backend enforces these secure defaults:

- `--read-only` filesystem
- `--cap-drop=ALL` (no Linux capabilities)
- `--security-opt no-new-privileges`
- `--network none` (no network access)
- `--user 65532:65532` (non-root)
- `--memory 128m`
- `--cpus 0.50`
- `--pids-limit 64`

These defaults can be overridden with `--tool-container-*` flags on `orlojd` and `orlojworker`, but the default posture is restrictive.

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

## Error Taxonomy

Tool failures use a canonical error taxonomy with three fields:

| Field | Purpose |
|---|---|
| `tool_code` | Machine-readable error code (e.g. `rate_limited`, `unsupported_tool`, `secret_resolution_failed`) |
| `tool_reason` | Human-readable explanation |
| `retryable` | Whether the runtime should retry the invocation |

HTTP status codes are mapped automatically: `429` and `5xx` are retryable, `4xx` are not. HTTP `401` maps to `auth_invalid` and `403` maps to `auth_forbidden` -- both non-retryable. All tool types share the same taxonomy, so policy and observability behave consistently.

## Auth Profiles

Tools support four authentication profiles via `spec.auth.profile`:

| Profile | Secret format | Injection |
|---------|--------------|-----------|
| `bearer` (default) | Single token value | `Authorization: Bearer <token>` |
| `api_key_header` | Single key value | Custom header via `spec.auth.headerName` |
| `basic` | `username:password` | `Authorization: Basic <base64>` |
| `oauth2_client_credentials` | Multi-key secret with `client_id` and `client_secret` | Token exchange at `spec.auth.tokenURL`, then bearer injection |

When `spec.auth.secretRef` is set without an explicit profile, the default is `bearer` for backward compatibility. See the [Tool Contract v1](../reference/tool-contract-v1.md) for full auth binding details.

### Secret Rotation

Secret resolution is performed fresh per tool invocation -- there is no caching of raw secret values. If a secret is rotated between invocations, the new value takes effect on the next call without requiring a restart.

For `oauth2_client_credentials`, access tokens are cached with a TTL derived from the token endpoint's `expires_in` response. Tokens are evicted on expiry or when the tool endpoint returns HTTP 401, triggering a fresh token exchange.

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
