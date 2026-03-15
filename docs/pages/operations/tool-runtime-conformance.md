# Tool Runtime Conformance

Status: release-candidate specification for Gate 0 contract stabilization.

## Purpose

Define pass/fail criteria each tool runtime backend must satisfy before production use.

Backends in scope:

- `native`
- `container`
- `wasm`
- `external`

## Required Test Groups

### 1. Contract Compliance

- accepts `tool_contract_version=v1`
- rejects unknown major versions with deterministic non-retryable error
- preserves `request_id` in response
- returns required fields for `ok`, `error`, and `denied`
- emits canonical failure metadata: `tool_status`, `tool_code`, `tool_reason`, `retryable`

### 2. Timeout and Cancellation

- enforces `timeout_ms`
- timeout maps to `code=timeout`, `reason=tool_execution_timeout`, `retryable=true`
- honors cancellation signals
- cancellation maps to `code=canceled`, `reason=tool_execution_canceled`

### 3. Retry Semantics

- deterministic retryable/non-retryable mapping
- preserves `usage.attempt`
- policy/permission denials are non-retryable by default
- preserves explicit `retryable` metadata through wrappers/controllers

### 4. Auth and Secret Handling

- resolves `auth.secret_ref` from namespace scope
- never logs raw secret values
- emits deterministic auth error code/reason for missing/invalid secrets
- supports auth profiles (`api_key_header`, `bearer_token`, and others)

### 5. Policy and Permission Hooks

- capability and risk metadata are available for policy checks
- denied calls return `status=denied` with normalized reason
- permission denials map to `permission_denied`, `tool_permission_denied`, `retryable=false`

### 6. Isolation and Resource Boundaries

- honors configured memory/cpu/pids/network constraints where applicable
- blocks runtime escape attempts with deterministic classification
- documents and tests backend-specific isolation behavior

### 7. Observability

- emits start/end lifecycle records
- includes trace correlation fields
- records duration and terminal status
- records normalized failure reason for failed/denied calls

### 8. Determinism and Replay Safety

- idempotent behavior for repeated request delivery with same idempotency key
- consistent failure mapping across repeated runs
- no hidden mutable global state affecting contract semantics

## Exit Criteria

A backend is conformance-ready when:

- all required test groups pass
- known limitations are documented
- policy/auth/observability hooks are verified in integration tests

## Current Harness Implementation

- shared harness: `runtime/conformance/harness.go`
- canonical case catalog: `runtime/conformance/cases/catalog.go`
- base tests: `runtime/conformance/harness_test.go`
- backend registration hooks: `runtime/tool_isolation_backend_registry.go`
- backend suites:
  - `TestGovernedToolRuntimeConformanceSuite`
  - `TestContainerToolRuntimeConformanceSuite`
  - `TestWASMStubRuntimeFailsClosed`
  - `TestWASMRuntimeScaffoldConformanceSuite`
  - `runtime/tool_runtime_wasm_command_executor_test.go`

Run:

```bash
GOCACHE=/tmp/go-build go test ./runtime ./runtime/conformance/...
```
