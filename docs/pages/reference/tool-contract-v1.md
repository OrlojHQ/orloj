# Tool Contract v1

Status: release-candidate contract targeted for Gate 0 stabilization.

## Purpose

Define a consistent execution contract across runtime backends (`native`, `container`, `wasm`, `external`) so policy, retries, auth, and observability behave deterministically.

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

Auth must be declarative and secret-referenced:

- `auth.profile` (`api_key_header`, `bearer_token`, `basic`, `custom_headers`)
- `auth.secret_ref`
- `auth.scopes[]`

Rules:

- do not persist resolved secrets in status/logs
- redact auth values in logs and traces
- auth resolution failures must map to canonical error fields

## Runtime Semantics

All runtimes must honor:

- `timeout_ms`
- cancellation propagation
- retry attempt index (`usage.attempt`)
- deterministic error mapping
- bounded return on timeout/cancel

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
