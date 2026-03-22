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

- `model_ref` (string): reference to a `ModelEndpoint` (`name` or `namespace/name`).
- `prompt` (string): agent instruction prompt.
- `tools` ([]string): tool names available to the agent.
- `allowed_tools` ([]string): tools pre-authorized without RBAC. Bypasses AgentRole/ToolPermission checks for listed tools.
- `roles` ([]string): bound `AgentRole` names.
- `memory` (object):
  - `ref` (string): reference to a `Memory` resource. This attaches the memory backend to the agent. See [Memory](../concepts/memory/index.md).
  - `allow` ([]string): explicit built-in memory operations allowed for the agent: `read`, `write`, `search`, `list`, `ingest`.
  - `type` (string)
  - `provider` (string)
- `limits` (object):
  - `max_steps` (int)
  - `timeout` (string duration)
- `execution` (object): optional per-agent execution contract.
  - `profile` (string): `dynamic` (default) or `contract`.
  - `tool_sequence` ([]string): required tool names when `profile=contract`. Tracked as a set (order-independent).
  - `required_output_markers` ([]string): strings that should appear in final model output when `profile=contract`. Treated as best-effort: missing markers at `max_steps` produce a warning, not a hard failure, when all tools completed.
  - `duplicate_tool_call_policy` (string): `short_circuit` (default) or `deny`. In `short_circuit` mode, duplicate tool calls reuse cached results and inject a completion hint. This applies to **all profiles**, not just `contract`.
  - `on_contract_violation` (string): `observe` or `non_retryable_error` (default). In `observe` mode, violations are logged as telemetry events but do not stop execution or deadletter the task.
  - `tool_use_behavior` (string): Controls what happens after a tool call succeeds. See [Tool Use Behavior](#tool-use-behavior) below.

#### Tool Use Behavior

The `tool_use_behavior` field controls whether the model gets another turn after a successful tool call. This is the primary lever for optimizing token usage in tool-calling agents.

| Value | Model calls | When to use |
|-------|------------|-------------|
| `run_llm_again` (default) | Tool call + follow-up model call to process the result | The agent needs to **interpret, format, or synthesize** the tool output before handing off. Most agents need this. |
| `stop_on_first_tool` | Tool call only -- tool output becomes the agent's final output directly | The agent is a **relay** that calls a tool and passes raw data to the next agent in the pipeline. No interpretation needed. |

**Example: `run_llm_again` (default)**

An analyst agent calls an API tool, then needs to produce labeled output from the raw response:

```yaml
kind: Agent
metadata:
  name: analyst-agent
spec:
  prompt: "Call the API, then return SUMMARY: and EVIDENCE: labels."
  tools:
    - external-api-tool
  # tool_use_behavior defaults to run_llm_again -- agent gets a
  # second model call to read the tool result and produce labels.
```

Step 1: model calls `external-api-tool` → Step 2: model reads tool result, produces labeled output → done (2 model calls).

**Example: `stop_on_first_tool`**

A fetcher agent's only job is to call a tool and pass the raw result downstream:

```yaml
kind: Agent
metadata:
  name: fetcher-agent
spec:
  prompt: "Fetch the latest data from the API."
  tools:
    - external-api-tool
  execution:
    tool_use_behavior: stop_on_first_tool
  # Agent exits immediately after the tool returns.
  # Raw tool output becomes the agent's output -- no extra model call.
```

Step 1: model calls `external-api-tool` → done (1 model call). The next agent in the pipeline receives the raw tool response as context.

**When NOT to use `stop_on_first_tool`:**
- The agent needs to produce structured/labeled output from the tool result.
- The agent has multiple tools and may need to call more than one.
- The agent needs to reason about the tool result before responding.

### Defaults and Validation

- `model_ref` is required.
- `roles` are trimmed and deduplicated (case-insensitive).
- `memory.allow` is trimmed, normalized, and deduplicated. It requires `memory.ref`.
- `limits.max_steps` defaults to `10` when `<= 0`.
- `execution.profile` defaults to `dynamic`.
- `execution.duplicate_tool_call_policy` defaults to `short_circuit`. Applies to all profiles.
- `execution.on_contract_violation` defaults to `non_retryable_error`. Set to `observe` for safe production rollout.
- `execution.tool_use_behavior` defaults to `run_llm_again`.
- `execution.tool_sequence` and `execution.required_output_markers` are trimmed and deduplicated.
- When `execution.profile=contract`, `execution.tool_sequence` is required.
- Tool sequence is tracked as a set: tools may be called in any order.
- When all tools in `tool_sequence` complete but `required_output_markers` are not satisfied at `max_steps`, the task completes with a `contract_warning` event instead of deadlettering.

**Structured tool protocol:** Tool results are sent to the model using the provider's native structured tool calling protocol (OpenAI `role: "tool"` with `tool_call_id`, Anthropic `tool_result` content blocks). This gives the model structured evidence that a tool was already called, preventing unnecessary repeat calls.

**Scaling ladder for cost control:**

1. `profile: dynamic` (default): structured tool protocol prevents repeat calls. Succeeded tools are filtered from the available tools list. No YAML changes needed.
2. `tool_use_behavior: stop_on_first_tool`: for pipeline stages that pass raw data, eliminates all extra model calls (1 model call + 1 tool call total).
3. `profile: contract` + `on_contract_violation: observe`: adds guaranteed early completion when all tools succeed plus telemetry for contract deviations.
4. `profile: contract` + `on_contract_violation: non_retryable_error`: hard enforcement for critical pipeline stages. Violations deadletter the task.

### `status`

- `phase`, `lastError`, `observedGeneration`

Example: `examples/resources/agents/*.yaml`

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

Example: `examples/resources/agent-systems/*.yaml`

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

Example: `examples/resources/model-endpoints/*.yaml`

## Tool

### `spec`

- `type` (string): tool type. Allowed values: `http`, `external`, `grpc`, `webhook-callback`, `queue`, `mcp`. Unknown values are rejected at apply time.
- `endpoint` (string): tool endpoint URL (or `host:port` for gRPC).
- `description` (string): human-readable description of the tool. Passed to model gateways for richer tool definitions. Auto-populated for MCP-generated tools.
- `input_schema` (object): JSON Schema for tool parameters. Passed to model gateways for structured parameter definitions. Auto-populated for MCP-generated tools.
- `mcp_server_ref` (string): name of the McpServer that provides this tool. Required when `type=mcp`.
- `mcp_tool_name` (string): the tool name as reported by the MCP server's `tools/list`. Required when `type=mcp`.
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

- `type` defaults to `http`. Unknown types are rejected with a validation error. `mcp` type tools are typically auto-generated by the McpServer controller; see [Connect an MCP Server](../guides/connect-mcp-server.md).
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

- `examples/resources/tools/*.yaml`
- `examples/resources/tools/wasm-reference/wasm_echo_tool.yaml`

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

Examples: `examples/resources/secrets/*.yaml`

## Memory

A Memory resource configures a persistent memory backend that agents can read from and write to using built-in memory tools. See [Memory Concepts](../concepts/memory/index.md) for a full overview.

### `spec`

- `type` (string): categorization of the memory use case (e.g. `vector`, `kv`). Informational in v1.
- `provider` (string): backend implementation. Built-in values:
  - `in-memory` (default): in-process key-value store. No endpoint needed. Data is lost on restart.
  - `pgvector`: PostgreSQL with the pgvector extension. Full vector-similarity search. Requires `endpoint` (Postgres DSN) and `embedding_model` (ModelEndpoint reference). See [pgvector](../concepts/memory/providers.md#pgvector).
  - `http`: delegates to an external HTTP service. Requires `endpoint`. See [HTTP Adapter](../concepts/memory/providers.md#http-adapter).
  - **Coming soon:** Qdrant, Pinecone, Weaviate, Chroma, Milvus. Custom providers can also be registered via the Go provider registry.
- `embedding_model` (string): reference to a ModelEndpoint resource that provides an OpenAI-compatible `/embeddings` API. Required for vector providers like `pgvector`. The endpoint's `base_url`, `auth`, and `default_model` are used to generate embeddings. Resolved in the same namespace by default; use `namespace/name` for cross-namespace references.
- `endpoint` (string): connection string or URL. For `pgvector`, a Postgres DSN (e.g. `postgres://user@host:5432/db`). For `http`, the adapter service URL. Not needed for `in-memory`.
- `auth` (object):
  - `secretRef` (string): reference to a Secret resource containing credentials. For `http`, used as a bearer token. For `pgvector`, injected as the Postgres password into the DSN.

### Defaults and Validation

- `provider` defaults to `in-memory` when omitted or empty.
- `endpoint` is required when `provider` is `pgvector`, `http`, or any cloud-hosted built-in provider.
- `embedding_model` is required when `provider` is `pgvector`. It must reference a valid ModelEndpoint.
- When `auth.secretRef` is set, the controller resolves the Secret and passes the token to the provider.
- The Memory controller validates the provider, resolves auth, and performs a connectivity check (`Ping`). Unsupported providers, missing secrets, or failed connectivity moves the resource to `Error` phase.

### Built-in Memory Tools

When an Agent references a Memory resource via `spec.memory.ref` and explicitly grants operations with `spec.memory.allow`, the runtime exposes the following built-in tools:

| Tool | Description |
|---|---|
| `memory.read` | Retrieve a value by key. |
| `memory.write` | Store a key-value pair. |
| `memory.search` | Search entries by keyword (or vector similarity). |
| `memory.list` | List entries, optionally filtered by key prefix. |
| `memory.ingest` | Chunk a document into overlapping segments and store them. |

These tools do not need to be listed in the agent's `spec.tools` -- they are injected automatically.

### `status`

- `phase`: `Pending`, `Ready`, or `Error`.
- `lastError`: description of the most recent error (e.g. unsupported provider, connectivity failure).
- `observedGeneration`

Example: `examples/resources/memories/research_memory.yaml`

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

Example: `examples/resources/agent-policies/cost_policy.yaml`

## AgentRole

### `spec`

- `description` (string)
- `permissions` ([]string): normalized permission strings.

### Defaults and Validation

- `permissions` are trimmed and deduplicated (case-insensitive).

### `status`

- `phase`, `lastError`, `observedGeneration`

Examples: `examples/resources/agent-roles/*.yaml`

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

Examples: `examples/resources/tool-permissions/*.yaml`

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

Example: `examples/resources/tasks/*.yaml`

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

Example: `examples/resources/task-schedules/*.yaml`

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
- `idempotency.dedupe_window_seconds` must be `>= 0`. Defaults to `259200` (72 hours) for `github` profile or `86400` (24 hours) for `generic` profile.
- `payload.mode` defaults to `raw` and only `raw` is allowed in v1.
- `payload.input_key` defaults to `webhook_payload`.

### `status`

- `phase`, `lastError`, `observedGeneration`
- `endpointID`, `endpointPath`
- `lastDeliveryTime`, `lastEventID`, `lastTriggeredTask`
- `acceptedCount`, `duplicateCount`, `rejectedCount`

Examples: `examples/resources/task-webhooks/*.yaml`

## McpServer

Represents a connection to an external MCP (Model Context Protocol) server. The McpServer controller discovers tools via `tools/list` and auto-generates `Tool` resources (type=mcp) for each.

### `spec`

- `transport` (string): **required**. `stdio` or `http`.
- `command` (string): stdio transport: command to spawn the MCP server process.
- `args` ([]string): stdio transport: command arguments.
- `env` ([]object): stdio transport: environment variables for the child process. Each entry has:
  - `name` (string): environment variable name.
  - `value` (string): literal value.
  - `secretRef` (string): resolve value from a Secret resource. Mutually exclusive with `value`.
- `endpoint` (string): http transport: the MCP server URL.
- `auth` (object): http transport: authentication configuration.
  - `secretRef` (string): secret reference for auth.
  - `profile` (string): `bearer` or `api_key_header`. Defaults to `bearer`.
- `tool_filter` (object): optional tool import filtering.
  - `include` ([]string): allowlist of MCP tool names. When set, only listed tools are generated. When empty, all discovered tools are generated.
- `reconnect` (object): reconnection policy.
  - `max_attempts` (int): max reconnection attempts. Defaults to 3.
  - `backoff` (duration string): backoff between attempts. Defaults to `2s`.

### Defaults and Validation

- `transport` is required. Must be `stdio` or `http`.
- `command` is required when `transport=stdio`.
- `endpoint` is required when `transport=http`.
- `env[].secretRef` and `env[].value` are mutually exclusive.
- `reconnect.max_attempts` defaults to `3`.
- `reconnect.backoff` defaults to `2s`.

### `status`

- `phase`: `Pending`, `Connecting`, `Ready`, `Error`.
- `discoveredTools` ([]string): all tool names from the MCP server's `tools/list` response.
- `generatedTools` ([]string): names of the `Tool` resources actually created.
- `lastSyncedAt` (timestamp): last successful tool sync.
- `lastError` (string): last error message.

Guide: [Connect an MCP Server](../guides/connect-mcp-server.md)

Examples: [`examples/resources/mcp-servers/mcp_server_everything_stdio.yaml`](../../../examples/resources/mcp-servers/mcp_server_everything_stdio.yaml), [`examples/resources/mcp-servers/README.md`](../../../examples/resources/mcp-servers/README.md)

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

Example: `examples/resources/workers/worker_a.yaml`

## Related References

- [API Reference](./api.md)
- [Task Scheduling (Cron)](../operations/task-scheduling.md)
- [Webhook Triggers](../operations/webhooks.md)
- [Tool Contract v1](./tool-contract-v1.md)
- [WASM Tool Module Contract v1](./wasm-tool-module-contract-v1.md)
- [Tool Runtime Conformance](../operations/tool-runtime-conformance.md)
- [Versioning and Deprecation](../project/versioning-and-deprecation.md)
