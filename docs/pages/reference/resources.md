# Resource Reference

> **Stability: beta** -- All resource kinds under `orloj.dev/v1` are suitable for production use, but their schemas may evolve with migration guidance in future minor releases. See [Versioning and Deprecation](../project/versioning-and-deprecation.md).

This document describes the current resource schemas in `orloj.dev/v1`, based on the runtime types and normalization logic in:

- `resources/agent.go`
- `resources/model_endpoint.go`
- `resources/resource_types.go`
- `resources/graph.go`

## Common Conventions

- Every resource uses standard top-level fields: `apiVersion`, `kind`, `metadata`, `spec`, `status`.
- `metadata.name` is required for all resources.
- `metadata.namespace` defaults to `default` when omitted.
- Most resources default `status.phase` to `Pending` during normalization.

## Resource Kinds

- `Agent`
- `AgentSystem`
- `ModelEndpoint`
- `Tool`
- `Secret`
- `Memory`
- `AgentPolicy`
- `AgentRole`
- `ToolPermission`
- `ToolApproval`
- `Task`
- `TaskSchedule`
- `TaskWebhook`
- `Worker`

## Agent

### `spec`

- `model` (string): direct model id.
- `model_ref` (string): reference to a `ModelEndpoint` (`name` or `namespace/name`).
- `prompt` (string): agent instruction prompt.
- `tools` ([]string): tool names available to the agent.
- `allowed_tools` ([]string): tools pre-authorized without RBAC. Bypasses AgentRole/ToolPermission checks for listed tools.
- `roles` ([]string): bound `AgentRole` names.
- `memory` (object):
  - `ref` (string): `Memory` resource name.
  - `type` (string)
  - `provider` (string)
- `limits` (object):
  - `max_steps` (int)
  - `timeout` (string duration)

### Defaults and Validation

- If both `model` and `model_ref` are empty, `model` defaults to `gpt-4o-mini`.
- `roles` are trimmed and deduplicated (case-insensitive).
- `limits.max_steps` defaults to `10` when `<= 0`.

### `status`

- `phase`, `lastError`, `observedGeneration`

Example: `examples/agents/*.yaml`

## AgentSystem

### `spec`

- `agents` ([]string): participating agent names.
- `graph` (map[string]GraphEdge): per-node routing.

`GraphEdge` fields:

- `next` (string): legacy single-hop route.
- `edges` ([]GraphRoute): fan-out routes.
  - `to` (string)
  - `labels` (map[string]string)
  - `policy` (map[string]string)
- `join` (GraphJoin): fan-in behavior.
  - `mode`: `wait_for_all` or `quorum`
  - `quorum_count` (int, >= 0)
  - `quorum_percent` (int, 0-100)
  - `on_failure`: `deadletter`, `skip`, `continue_partial`

### Defaults and Validation

- `graph[*].next` and `graph[*].edges[].to` are trimmed.
- Route targets are normalized/deduplicated for execution.
- `join` normalization defaults:
  - `mode` -> `wait_for_all`
  - `on_failure` -> `deadletter`
  - `quorum_percent` clamped to `0..100`
  - invalid values are coerced to safe defaults in graph normalization.
- Runtime task validation additionally checks:
  - graph nodes/edges must reference agents in `spec.agents`
  - cyclic graphs require `Task.spec.max_turns > 0`
  - non-cyclic graphs require at least one entrypoint (zero indegree node)

### `status`

- `phase`, `lastError`, `observedGeneration`

Example: `examples/agent-systems/*.yaml`

## ModelEndpoint

### `spec`

- `provider` (string): provider id (`openai`, `anthropic`, `azure-openai`, `ollama`, `mock`, or registry-added providers).
- `base_url` (string)
- `default_model` (string)
- `options` (map[string]string): provider-specific options.
- `auth.secretRef` (string): namespaced reference to a `Secret`.

### Defaults and Validation

- `provider` defaults to `openai` and is normalized to lowercase.
- `base_url` defaults by provider:
  - `openai` -> `https://api.openai.com/v1`
  - `anthropic` -> `https://api.anthropic.com/v1`
  - `ollama` -> `http://127.0.0.1:11434`
- `options` keys are normalized to lowercase; keys/values are trimmed.

### `status`

- `phase`, `lastError`, `observedGeneration`

Example: `examples/model-endpoints/*.yaml`

## Tool

### `spec`

- `type` (string): tool type. Allowed values: `http`, `external`, `grpc`, `webhook-callback`, `queue`. Unknown values are rejected at apply time.
- `endpoint` (string): tool endpoint URL (or `host:port` for gRPC).
- `capabilities` ([]string): declared operations.
- `operation_classes` ([]string): operation class annotations. Allowed values: `read`, `write`, `delete`, `admin`. Used by `ToolPermission.operation_rules` for per-class policy verdicts.
- `risk_level` (string): `low`, `medium`, `high`, `critical`.
- `runtime` (object):
  - `timeout` (duration string)
  - `isolation_mode`: `none`, `sandboxed`, `container`, `wasm`
  - `retry.max_attempts` (int)
  - `retry.backoff` (duration string)
  - `retry.max_backoff` (duration string)
  - `retry.jitter`: `none`, `full`, `equal`
- `auth` (object):
  - `profile` (string): auth profile. Allowed values: `bearer`, `api_key_header`, `basic`, `oauth2_client_credentials`. Defaults to `bearer` when `secretRef` is set.
  - `secretRef` (string): namespaced secret reference. Required when `profile` is set.
  - `headerName` (string): custom header name. Required when `profile=api_key_header`.
  - `tokenURL` (string): OAuth2 token endpoint. Required when `profile=oauth2_client_credentials`.
  - `scopes` ([]string): OAuth2 scopes.

### Defaults and Validation

- `type` defaults to `http`. Unknown types are rejected with a validation error.
- `auth.profile` defaults to `bearer` when `secretRef` is set. Unknown profiles are rejected.
- `auth.headerName` is required when `profile=api_key_header`.
- `auth.tokenURL` is required when `profile=oauth2_client_credentials`.
- `capabilities` are trimmed and deduplicated (case-insensitive).
- `operation_classes` are trimmed, lowercased, and deduplicated. Invalid values are rejected. Defaults to `["read"]` for `low`/`medium` risk, `["write"]` for `high`/`critical` risk.
- `risk_level` defaults to `low`.
- `runtime.timeout` defaults to `30s` and must parse as duration.
- `runtime.isolation_mode` defaults to:
  - `sandboxed` for `high`/`critical` risk
  - `none` for `low`/`medium` risk
- `runtime.retry` defaults:
  - `max_attempts` -> `1`
  - `backoff` -> `0s`
  - `max_backoff` -> `30s`
  - `jitter` -> `none`

### `status`

- `phase`, `lastError`, `observedGeneration`

Examples:

- `examples/tools/*.yaml`
- `examples/tools/wasm-reference/wasm_echo_tool.yaml`

## Secret

### `spec`

- `data` (map[string]string): base64-encoded values.
- `stringData` (map[string]string): write-only plaintext convenience input.

### Defaults and Validation

- `stringData` entries are merged into `data` as base64 during normalization.
- Every `data` value must be non-empty valid base64.
- `stringData` is cleared after normalization (write-only behavior).

### `status`

- `phase`, `lastError`, `observedGeneration`

Examples: `examples/secrets/*.yaml`

## Memory

### `spec`

- `type` (string)
- `provider` (string)
- `embedding_model` (string)

### Defaults and Validation

- No field-level defaults in `spec`; only common metadata/status defaults apply.

### `status`

- `phase`, `lastError`, `observedGeneration`

Example: `examples/memories/research_memory.yaml`

## AgentPolicy

### `spec`

- `max_tokens_per_run` (int)
- `allowed_models` ([]string)
- `blocked_tools` ([]string)
- `apply_mode` (string): `scoped` or `global`
- `target_systems` ([]string)
- `target_tasks` ([]string)

### Defaults and Validation

- `apply_mode` defaults to `scoped`.
- `apply_mode` must be `scoped` or `global`.

### `status`

- `phase`, `lastError`, `observedGeneration`

Example: `examples/agent-policies/cost_policy.yaml`

## AgentRole

### `spec`

- `description` (string)
- `permissions` ([]string): normalized permission strings.

### Defaults and Validation

- `permissions` are trimmed and deduplicated (case-insensitive).

### `status`

- `phase`, `lastError`, `observedGeneration`

Examples: `examples/agent-roles/*.yaml`

## ToolPermission

### `spec`

- `tool_ref` (string): tool name reference.
- `action` (string): action name (commonly `invoke`).
- `required_permissions` ([]string)
- `match_mode` (string): `all` or `any`
- `apply_mode` (string): `global` or `scoped`
- `target_agents` ([]string): required when `apply_mode=scoped`
- `operation_rules` ([]object): per-operation-class policy verdicts.
  - `operation_class` (string): `read`, `write`, `delete`, `admin`, or `*` (wildcard). Defaults to `*`.
  - `verdict` (string): `allow`, `deny`, or `approval_required`. Defaults to `allow`.

### Defaults and Validation

- `tool_ref` defaults to `metadata.name` when omitted.
- `action` defaults to `invoke`.
- `match_mode` defaults to `all`.
- `apply_mode` defaults to `global`.
- `required_permissions` and `target_agents` are trimmed and deduplicated.
- `target_agents` must be non-empty when `apply_mode=scoped`.
- `operation_rules` values are trimmed and lowercased. Invalid `operation_class` or `verdict` values are rejected.
- When `operation_rules` is present, the authorizer evaluates the tool's `operation_classes` against the rules. The most restrictive matching verdict wins (`deny` > `approval_required` > `allow`).
- When `operation_rules` is empty, behavior is unchanged (backward-compatible binary allow/deny).

### `status`

- `phase`, `lastError`, `observedGeneration`

Examples: `examples/tool-permissions/*.yaml`

## ToolApproval

Captures a pending human/system approval request for a tool invocation that was flagged by a `ToolPermission` `operation_rules` verdict of `approval_required`.

### `spec`

- `task_ref` (string, required): name of the Task resource waiting for approval.
- `tool` (string, required): tool name that triggered the approval request.
- `operation_class` (string): the operation class that requires approval.
- `agent` (string): agent that attempted the tool call.
- `input` (string): tool input payload (for audit context).
- `reason` (string): human-readable reason for the approval request.
- `ttl` (duration string): time-to-live before auto-expiry. Defaults to `10m`.

### `status`

- `phase` (string): `Pending`, `Approved`, `Denied`, `Expired`. Defaults to `Pending`.
- `decision` (string): `approved` or `denied`.
- `decided_by` (string): identity of the approver/denier.
- `decided_at` (string): RFC3339 timestamp of the decision.
- `expires_at` (string): RFC3339 timestamp when the approval expires.

### API Endpoints

- `POST /v1/tool-approvals` -- create an approval request.
- `GET /v1/tool-approvals` -- list approval requests (supports namespace and label filters).
- `GET /v1/tool-approvals/{name}` -- get a specific approval.
- `DELETE /v1/tool-approvals/{name}` -- delete an approval.
- `POST /v1/tool-approvals/{name}/approve` -- approve a pending request. Body: `{"decided_by": "..."}`.
- `POST /v1/tool-approvals/{name}/deny` -- deny a pending request. Body: `{"decided_by": "..."}`.

## Task

### `spec`

- `system` (string): target `AgentSystem` name.
- `mode` (string): `run` (default) or `template`.
- `input` (map[string]string): task payload.
- `priority` (string)
- `max_turns` (int, >= 0): required for cyclic graph traversal.
- `retry` (object):
  - `max_attempts` (int)
  - `backoff` (duration string)
- `message_retry` (object):
  - `max_attempts` (int)
  - `backoff` (duration string)
  - `max_backoff` (duration string)
  - `jitter`: `none`, `full`, `equal`
  - `non_retryable` ([]string)
- `requirements` (object):
  - `region` (string)
  - `gpu` (bool)
  - `model` (string)

### Defaults and Validation

- `input` defaults to `{}`.
- `priority` defaults to `normal`.
- `mode` defaults to `run`.
- `mode=template` marks a task as non-executable template for schedules.
- `max_turns` must be `>= 0`.
- `retry` defaults:
  - `max_attempts` -> `1`
  - `backoff` -> `0s`
- `message_retry` defaults:
  - `max_attempts` -> `retry.max_attempts`
  - `backoff` -> `retry.backoff`
  - `max_backoff` -> `24h`
  - `jitter` -> `full`
- `retry.backoff`, `message_retry.backoff`, and `message_retry.max_backoff` must parse as durations.

### `status`

Primary fields:

- `phase`: `Pending`, `Running`, `WaitingApproval`, `Succeeded`, `Failed`, `DeadLetter`.
- `lastError`, `startedAt`, `completedAt`, `nextAttemptAt`, `attempts`
- `output`, `assignedWorker`, `claimedBy`, `leaseUntil`, `lastHeartbeat`
- `observedGeneration`

The `WaitingApproval` phase indicates the task is paused pending a `ToolApproval` decision. When the linked `ToolApproval` is approved, the task transitions back to `Running`. When denied or expired, the task transitions to `Failed` with an `approval_denied` or `approval_timeout` reason.

Observability arrays:

- `trace[]`: detailed execution/tool-call events.
- `history[]`: lifecycle transitions.
- `messages[]`: message bus records.
- `message_idempotency[]`: message idempotency state.
- `join_states[]`: fan-in join activation state.

Example: `examples/tasks/*.yaml`

## TaskSchedule

### `spec`

- `task_ref` (string): task template reference (`name` or `namespace/name`).
- `schedule` (string): 5-field cron expression.
- `time_zone` (string): IANA timezone.
- `suspend` (bool): stop triggering when `true`.
- `starting_deadline_seconds` (int): max lateness window for catch-up.
- `concurrency_policy` (string): `forbid` (v1).
- `successful_history_limit` (int): retained successful run count.
- `failed_history_limit` (int): retained failed/deadletter run count.

### Defaults and Validation

- `task_ref` is required and must be `name` or `namespace/name`.
- `schedule` is required and must be a valid 5-field cron.
- `time_zone` defaults to `UTC`.
- `starting_deadline_seconds` defaults to `300`.
- `concurrency_policy` defaults to `forbid`.
- `successful_history_limit` defaults to `10`.
- `failed_history_limit` defaults to `3`.

### `status`

- `phase`, `lastError`, `observedGeneration`
- `lastScheduleTime`, `lastSuccessfulTime`, `nextScheduleTime`
- `lastTriggeredTask`, `activeRuns`

Example: `examples/task-schedules/*.yaml`

## TaskWebhook

### `spec`

- `task_ref` (string): template task reference (`name` or `namespace/name`).
- `suspend` (bool): rejects deliveries when `true`.
- `auth` (object):
  - `profile` (string): `generic` (default) or `github`.
  - `secret_ref` (string): required secret reference (`name` or `namespace/name`).
  - `signature_header` (string)
  - `signature_prefix` (string)
  - `timestamp_header` (string): used by `generic`.
  - `max_skew_seconds` (int): timestamp tolerance for `generic`.
- `idempotency` (object):
  - `event_id_header` (string): header containing unique delivery id.
  - `dedupe_window_seconds` (int): dedupe TTL.
- `payload` (object):
  - `mode` (string): `raw` (v1 only).
  - `input_key` (string): generated task input key for raw payload.

### Defaults and Validation

- `task_ref` is required and must be `name` or `namespace/name`.
- `auth.secret_ref` is required.
- `auth.profile` defaults to `generic`; supported values: `generic`, `github`.
- profile defaults:
  - `generic`:
    - `signature_header` -> `X-Signature`
    - `signature_prefix` -> `sha256=`
    - `timestamp_header` -> `X-Timestamp`
    - `idempotency.event_id_header` -> `X-Event-Id`
  - `github`:
    - `signature_header` -> `X-Hub-Signature-256`
    - `signature_prefix` -> `sha256=`
    - `idempotency.event_id_header` -> `X-GitHub-Delivery`
- `auth.max_skew_seconds` defaults to `300` and must be `>= 0`.
- `idempotency.dedupe_window_seconds` defaults to `86400` and must be `>= 0`.
- `payload.mode` defaults to `raw` and only `raw` is allowed in v1.
- `payload.input_key` defaults to `webhook_payload`.

### `status`

- `phase`, `lastError`, `observedGeneration`
- `endpointID`, `endpointPath`
- `lastDeliveryTime`, `lastEventID`, `lastTriggeredTask`
- `acceptedCount`, `duplicateCount`, `rejectedCount`

Examples: `examples/task-webhooks/*.yaml`

## Worker

### `spec`

- `region` (string)
- `capabilities.gpu` (bool)
- `capabilities.supported_models` ([]string)
- `max_concurrent_tasks` (int)

### Defaults and Validation

- `max_concurrent_tasks` defaults to `1` when `<= 0`.

### `status`

- `phase`, `lastError`, `lastHeartbeat`, `observedGeneration`, `currentTasks`

Example: `examples/workers/worker_a.yaml`

## Related References

- [API Reference](./api.md)
- [Task Scheduling (Cron)](../operations/task-scheduling.md)
- [Webhook Triggers](../operations/webhooks.md)
- [Tool Contract v1](./tool-contract-v1.md)
- [WASM Tool Module Contract v1](./wasm-tool-module-contract-v1.md)
- [Tool Runtime Conformance](../operations/tool-runtime-conformance.md)
- [Versioning and Deprecation](../project/versioning-and-deprecation.md)
