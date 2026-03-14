# Tool Runtime Conformance (Draft)

## Purpose

Define pass/fail criteria every tool runtime backend must satisfy before production use.

Backends in scope:

- `native`
- `container`
- `wasm`
- `external`

## Required Test Groups

## 1. Contract Compliance

- accepts `tool_contract_version=v1`
- rejects unknown major versions with deterministic non-retryable error
- preserves `request_id` in response
- returns required fields for `ok`, `error`, and `denied`
- emits canonical runtime metadata fields in failures:
  - `tool_status`
  - `tool_code`
  - `tool_reason`
  - `retryable`

## 2. Timeout and Cancellation

- enforces `timeout_ms` hard limit
- maps timeout to `code=timeout`, `reason=tool_execution_timeout`, `retryable=true`
- honors cancellation signal and stops execution promptly
- reports cancellation with deterministic reason code (`code=canceled`, `reason=tool_execution_canceled`)

## 3. Retry Semantics

- deterministic retryable/non-retryable mapping for standard failure classes
- preserves attempt count (`usage.attempt`)
- does not mark policy/permission denials as retryable by default
- preserves explicit `retryable` metadata through wrapper/controller layers

## 4. Auth and Secret Handling

- resolves `auth.secret_ref` from namespace scope
- never logs raw secret values
- emits deterministic auth failure code/reason when secret missing/invalid
- supports auth profile behaviors (`api_key_header`, `bearer_token`, etc.)

## 5. Policy and Permission Hooks

- capability and risk metadata are present for policy checks
- denied calls return `status=denied` with normalized reason
- denial responses include policy/permission reference metadata when available
- permission denials map to:
  - `code=permission_denied`
  - `reason=tool_permission_denied`
  - `retryable=false`

## 6. Isolation and Resource Boundaries

- honors memory/cpu/pids/network constraints where applicable
- runtime escape attempts are blocked and classified deterministically
- backend-specific isolation behavior is documented and tested

## 7. Observability

- emits start/end lifecycle records
- includes `trace_id`/`span_id` correlation
- records duration and terminal status
- records normalized `error.reason` for failed/denied executions
- task/message runtime trace includes `error_code`, `error_reason`, and `retryable` when present

## 8. Determinism and Replay Safety

- idempotent behavior for repeated request delivery with same idempotency key
- consistent error mapping across repeated runs of same failure mode
- no hidden mutable global state affecting contract semantics

## Exit Criteria

A runtime backend is conformance-ready when:

- all required test groups pass
- known limitations are documented in runtime-specific notes
- policy, auth, and observability hooks are verified in integration tests

## Suggested Repo Layout

- `runtime/conformance/` for shared harness
- `runtime/conformance/cases/` for canonical contract case builders/catalog
- backend-specific test entrypoints in runtime package tests

## Current Harness (Implemented)

- shared conformance harness is implemented at `runtime/conformance/harness.go`
- the harness validates envelope invariants:
  - `tool_contract_version=v1`
  - request/trace roundtrip fields
  - required status/error envelope fields
  - expected status/code/reason/retryable assertions
- per-case execution controls are supported for conformance cases:
  - call timeout (`CallTimeout`)
  - immediate cancellation (`CancelImmediately`)
  - bounded latency assertions (`MaxLatency`)
- canonical harness tests live at `runtime/conformance/harness_test.go`
- reusable case catalog helpers live at `runtime/conformance/cases/catalog.go`
- isolated backend registration hooks live at `runtime/tool_isolation_backend_registry.go`
- backend suites currently implemented:
  - governed runtime: `runtime/tool_runtime_conformance_test.go` (`TestGovernedToolRuntimeConformanceSuite`)
  - container runtime: `runtime/tool_runtime_conformance_test.go` (`TestContainerToolRuntimeConformanceSuite`)
  - wasm stub runtime fail-closed contract: `runtime/tool_runtime_conformance_test.go` (`TestWASMStubRuntimeFailsClosed`)
  - wasm runtime scaffold contract suite: `runtime/tool_runtime_conformance_test.go` (`TestWASMRuntimeScaffoldConformanceSuite`)
  - wasm command executor behavior coverage: `runtime/tool_runtime_wasm_command_executor_test.go`

Run:

```bash
GOCACHE=/tmp/go-build go test ./runtime ./runtime/conformance/...
```
