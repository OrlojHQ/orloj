# Tool Contract V1 (Draft)

## Purpose

Define a stable contract for tool execution across runtime backends (`native`, `container`, `wasm`, `external`) so policy enforcement, retries, auth, and observability behave consistently.

## Scope

This document defines:

- request/response contract
- error model and reason codes
- capability and risk taxonomy
- auth binding and redaction requirements
- cancellation and timeout behavior
- versioning expectations

## Contract Version

- `tool_contract_version`: `v1`
- all runtimes must report the contract version in execution telemetry
- implemented runtime contract structs live in `runtime/tool_contract.go`:
  - `ToolExecutionRequest`
  - `ToolExecutionResponse`
  - `ToolExecutionFailure`
  - `ToolContractExecutor` / `ExecuteToolContract(...)`

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

Current required request fields (runtime-enforced):

- `tool_contract_version` (defaults to `v1` when omitted)
- `request_id`
- `tool.name`

Input encoding notes:

- `input_raw` is used as-is when present.
- otherwise `input` is JSON-encoded and sent to legacy `ToolRuntime.Call(...)` paths.

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

Response `status`:

- `ok`
- `error`
- `denied`

## Error Model

All failures must return:

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

### Required error fields

- `code`: stable short code (`timeout`, `auth_failed`, `policy_denied`, `invalid_input`, `dependency_unavailable`)
- `reason`: normalized taxonomy key used by policy/UI/analytics
- `retryable`: deterministic boolean used by retry engine
- `message`: human-readable summary
- `details`: machine-parseable map

### Canonical Runtime Taxonomy (V1)

Status values:

- `error`
- `denied`

Canonical error `code` values:

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

Runtime requirement:

- runtime errors must carry deterministic metadata in logs/trace strings using:
  - `tool_status=<error|denied>`
  - `tool_code=<...>`
  - `tool_reason=<...>`
  - `retryable=<true|false>`

### Required denial behavior

For policy/permission denials:

- return `status=denied`
- include `reason` and the policy/permission id when available
- classify as non-retryable unless explicitly overridden by policy

## Capability Taxonomy (V1)

Capability labels must be normalized lowercase and dot-delimited:

- `data.read`
- `data.write`
- `network.read`
- `network.write`
- `filesystem.read`
- `filesystem.write`
- `exec.command`
- `external.side_effect`

These labels are the policy and approval hook boundary.

## Risk Levels

- `low`
- `medium`
- `high`
- `critical`

Policy may enforce additional controls by risk tier (deny/approval/allow).

## Auth Binding

Tool auth must be declarative and reference secrets by name:

- `auth.profile`: predefined binding strategy (`api_key_header`, `bearer_token`, `basic`, `custom_headers`)
- `auth.secret_ref`: namespace-scoped secret reference
- `auth.scopes[]`: optional required scopes for policy checks

Rules:

- do not persist resolved secret values in task/message status or logs
- redact auth values in all logs and traces
- auth resolution failures must produce deterministic error codes

## Runtime Semantics

All runtimes must honor:

- `timeout_ms` hard deadline
- cancellation propagation from task/message cancellation
- attempt index (`usage.attempt`) for retries
- deterministic mapping of runtime errors to contract error codes
- bounded completion on timeout/cancel paths (calls must return promptly when context is done)
- response envelope usage fields:
  - `usage.duration_ms`
  - `usage.attempt`
- error responses must roundtrip through `ToolExecutionResponse.ToError()` without losing `code`, `reason`, or `retryable`

## Observability Requirements

Every execution must emit:

- start and end timestamps
- status (`ok/error/denied`)
- `error.reason` when failed/denied
- duration and attempt count
- trace correlation fields (`trace_id`, `span_id`)

When available, task/message trace events should include:

- `tool_contract_version`
- `tool_request_id`
- `tool_attempt`
- `error_code`
- `error_reason`
- `retryable`

## Versioning and Migration

- contract evolution is versioned as `v1` and updated in place
- breaking changes require explicit project-level approval before introducing a new major
- additive optional fields should be preferred to preserve `v1` compatibility
- runtimes must reject unknown major versions with a deterministic non-retryable error
