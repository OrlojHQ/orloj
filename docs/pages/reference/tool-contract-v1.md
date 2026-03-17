# Tool Contract v1

Status: release-candidate contract targeted for Gate 0 stabilization.

## Purpose

Define a consistent execution contract across runtime backends (`http`, `container`, `wasm`, `external`, `grpc`, `webhook-callback`) so policy, retries, auth, and observability behave deterministically.

## Scope

This document defines:

- request/response envelope
- canonical error taxonomy
- capability and risk labels
- auth and redaction requirements
- timeout/cancel semantics
- compatibility expectations

Implemented contract structs are in `runtime/tool_contract.go`.

## Contract Version

- `tool_contract_version`: `v1`
- runtimes must report contract version in execution telemetry
- unknown major versions must be rejected as deterministic non-retryable errors

## Execution Request

```json
{
  "tool_contract_version": "v1",
  "request_id": "req-123",
  "task_id": "default/weekly-report",
  "namespace": "default",
  "agent": "research-agent",
  "tool": {
    "name": "web_search",
    "operation": "invoke",
    "capabilities": ["network.read", "data.read"],
    "risk_level": "medium"
  },
  "input": {
    "query": "latest market analysis"
  },
  "input_raw": "",
  "runtime": {
    "mode": "container",
    "timeout_ms": 15000,
    "max_attempts": 3,
    "backoff": "exponential",
    "max_backoff_ms": 30000,
    "jitter": true
  },
  "auth": {
    "profile": "api_key_header",
    "secret_ref": "search-api-key",
    "scopes": ["search.read"]
  },
  "trace": {
    "trace_id": "trace-abc",
    "span_id": "span-xyz"
  }
}
```

Runtime-required request fields:

- `tool_contract_version` (defaults to `v1` when omitted)
- `request_id`
- `tool.name`

## Execution Response

```json
{
  "request_id": "req-123",
  "status": "ok",
  "output": {
    "summary": "..."
  },
  "usage": {
    "duration_ms": 182,
    "attempt": 1
  },
  "trace": {
    "trace_id": "trace-abc",
    "span_id": "span-xyz"
  }
}
```

`status` values:

- `ok`
- `error`
- `denied`

## Error Model

All failures must include canonical fields:

```json
{
  "status": "error",
  "error": {
    "code": "timeout",
    "reason": "tool_execution_timeout",
    "retryable": true,
    "message": "execution exceeded timeout_ms",
    "details": {
      "timeout_ms": 15000
    }
  }
}
```

Required error fields:

- `code`
- `reason`
- `retryable`
- `message`
- `details`

Canonical `code` values:

- `invalid_input`
- `unsupported_tool`
- `runtime_policy_invalid`
- `isolation_unavailable`
- `permission_denied`
- `secret_resolution_failed`
- `timeout`
- `canceled`
- `execution_failed`

Canonical `reason` values:

- `tool_invalid_input`
- `tool_unsupported`
- `tool_runtime_policy_invalid`
- `tool_isolation_unavailable`
- `tool_permission_denied`
- `tool_secret_resolution_failed`
- `tool_execution_timeout`
- `tool_execution_canceled`
- `tool_backend_failure`

Runtime failures must emit deterministic metadata fields:

- `tool_status`
- `tool_code`
- `tool_reason`
- `retryable`

## Denial Semantics

Policy/permission denials must:

- return `status=denied`
- include normalized reason
- include policy/permission reference when available
- be non-retryable unless explicitly overridden by policy

## Capability Taxonomy

Capability labels are lowercase and dot-delimited:

- `data.read`
- `data.write`
- `network.read`
- `network.write`
- `filesystem.read`
- `filesystem.write`
- `exec.command`
- `external.side_effect`

## Risk Levels

- `low`
- `medium`
- `high`
- `critical`

## Auth Binding

Auth is declarative and secret-referenced via `Tool.spec.auth`:

- `auth.profile`: `bearer` (default), `api_key_header`, `basic`, `oauth2_client_credentials`
- `auth.secretRef`: reference to a `Secret` resource (required when profile is set)
- `auth.headerName`: custom header name (required for `api_key_header`)
- `auth.tokenURL`: OAuth2 token endpoint (required for `oauth2_client_credentials`)
- `auth.scopes[]`: OAuth2 scopes

### Auth Profiles

| Profile | Secret format | Injection |
|---------|--------------|-----------|
| `bearer` | Single value (token) | `Authorization: Bearer <token>` |
| `api_key_header` | Single value (key) | `<headerName>: <key>` |
| `basic` | `username:password` | `Authorization: Basic <base64>` |
| `oauth2_client_credentials` | Multi-key: `client_id`, `client_secret` | Token exchange, then `Authorization: Bearer <access_token>` |

### Auth Error Codes

| HTTP Status / gRPC Code | `tool_code` | `tool_reason` | Retryable |
|---|---|---|---|
| 401 / `Unauthenticated` | `auth_invalid` | `tool_auth_invalid` | false |
| 403 / `PermissionDenied` | `auth_forbidden` | `tool_auth_forbidden` | false |
| Token expired (OAuth2) | `auth_expired` | `tool_auth_expired` | true (one retry) |
| Secret not found | `secret_resolution_failed` | `tool_secret_resolution_failed` | false |

### Approval Error Codes

| Condition | `tool_code` | `tool_reason` | Retryable |
|---|---|---|---|
| Tool call awaiting approval | `approval_pending` | `tool_approval_pending` | false (pause) |
| Approval denied | `approval_denied` | `tool_approval_denied` | false |
| Approval TTL expired | `approval_timeout` | `tool_approval_timeout` | false |

All approval-related error codes are non-retryable and do not consume retry budget.

### Rules

- Do not persist resolved secrets in status/logs
- Redact auth values in logs and traces
- Auth resolution failures must map to canonical error fields
- Traces record `tool_auth_profile` and `tool_auth_secret_ref` (secret name, not value)

### Secret Rotation Semantics

- Secret resolution is performed fresh per tool invocation (no caching of raw secret values)
- If a secret is rotated between invocations, the new value is used on the next call without restart
- For `oauth2_client_credentials`: access tokens are cached with TTL derived from the token response's `expires_in` field (minus a 30-second safety margin). Cache eviction occurs automatically on expiry or on HTTP 401 from the tool endpoint, triggering a fresh token exchange.
- Long-running tasks get the latest secret value on each step's tool calls

## Runtime Semantics

All runtimes must honor:

- `timeout_ms`
- cancellation propagation
- retry attempt index (`usage.attempt`)
- deterministic error mapping
- bounded return on timeout/cancel

### Transport-Specific Behavior

**`http`** -- Sends raw tool input as the HTTP POST body. Accepts both raw text and `ToolExecutionResponse` JSON in the response. Maps HTTP status codes to canonical errors (429/5xx retryable, 4xx non-retryable).

**`external`** -- Sends the full `ToolExecutionRequest` as the POST body with `Content-Type: application/json` and `X-Tool-Contract-Version: v1`. Requires a `ToolExecutionResponse` JSON response. Non-JSON responses are rejected.

**`grpc`** -- Invokes `orloj.tool.v1.ToolService/Execute` as a unary RPC using a JSON codec. Request and response are `ToolExecutionRequest` and `ToolExecutionResponse` marshaled as JSON. gRPC status codes are mapped to the canonical error taxonomy.

**`webhook-callback`** -- Sends `ToolExecutionRequest` via HTTP POST. A `200` response is treated as immediate completion. A `202` triggers asynchronous polling at `{endpoint}/{request_id}` until a terminal `ToolExecutionResponse` arrives or timeout expires. The runtime also accepts push-based delivery via the callback API.

## Observability Requirements

Each execution must emit:

- start/end timestamps
- terminal status (`ok|error|denied`)
- `error.reason` when failed/denied
- duration and attempt count
- trace correlation (`trace_id`, `span_id`)

Task/message traces should include when available:

- `tool_contract_version`
- `tool_request_id`
- `tool_attempt`
- `error_code`
- `error_reason`
- `retryable`

## Compatibility Policy

- additive fields are preferred
- no unversioned breaking changes on stable contract surfaces
- breaking changes require explicit major versioning and migration guidance
