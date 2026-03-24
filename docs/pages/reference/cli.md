# CLI Reference

This page is the canonical reference for CLI flags across all Orloj binaries.

## Binaries

- `orlojctl`: resource CRUD, observability views, event stream tooling
- `orlojd`: API server, controllers, scheduler, optional embedded worker
- `orlojworker`: task worker and optional runtime inbox consumer
- `orloj-loadtest`: reliability/load harness
- `orloj-alertcheck`: alert profile evaluator

## `orlojctl`

Usage patterns:

```text
orlojctl apply -f <resource.yaml>
orlojctl create secret <name> --from-literal key=value [...]
orlojctl get [-w] <resource>
orlojctl delete <resource> <name>
orlojctl run --system <name> [key=value ...]
orlojctl init --blueprint pipeline|hierarchical|swarm-loop [--name <prefix>] [--dir <path>]
orlojctl logs <agent-name>|task/<task-name>
orlojctl trace task <task-name>
orlojctl graph system|task <name>
orlojctl events [filters...]
orlojctl admin reset-password --new-password <value> [--username <name>]
orlojctl config path|get|use <name>|set-profile <name> [--server URL] [--token value] [--token-env NAME]
```

### Global Auth and Server Resolution

- Global auth flag: `--api-token <token>`
- Version command: `orlojctl version` (also `-version`, `--version`)
- Token precedence:
  1. `--api-token`
  2. `ORLOJCTL_API_TOKEN`
  3. `ORLOJ_API_TOKEN`
  4. Active profile `token`, then `token_env`
- Default server precedence when `--server` is omitted:
  1. `ORLOJCTL_SERVER`
  2. `ORLOJ_SERVER`
  3. Active profile `server`
  4. `http://127.0.0.1:8080`

### `orlojctl apply`

| Flag | Default | Description |
|---|---|---|
| `-f` | none | Path to resource manifest (required). |
| `--server` | resolved server | API server URL. |

### `orlojctl create secret`

| Flag | Default | Description |
|---|---|---|
| `--from-literal` | none | Repeatable `key=value` pair; at least one required. |
| `--namespace` | `default` | Secret namespace. |
| `-n` | `default` | Shorthand for `--namespace`. |
| `--server` | resolved server | API server URL. |

### `orlojctl get`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `-w` | `false` | Watch mode (currently only supported for `tasks`). |

Supported resources:

- `agents`
- `agent-systems`
- `model-endpoints`
- `tools`
- `secrets`
- `memories`
- `agent-policies`
- `agent-roles`
- `tool-permissions`
- `tasks`
- `task-schedules`
- `task-webhooks`
- `workers`
- `mcp-servers`

### `orlojctl delete`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--namespace` | empty | Optional namespace override for namespaced resources. |
| `-n` | empty | Shorthand for `--namespace`. |

### `orlojctl run`

| Flag | Default | Description |
|---|---|---|
| `--system` | none | Target `AgentSystem` (required). |
| `--server` | resolved server | API server URL. |
| `--namespace` | `default` | Task namespace. |
| `-n` | `default` | Shorthand for `--namespace`. |
| `--poll` | `2s` | Poll interval while waiting for task completion. |
| `--timeout` | `5m` | Max wait time for task completion. |

Positional args after flags are parsed as `key=value` task input.

### `orlojctl events`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--since` | `0` | Resume stream from event id. |
| `--source` | empty | Filter by event source. |
| `--type` | empty | Filter by event type. |
| `--kind` | empty | Filter by resource kind. |
| `--name` | empty | Filter by resource name. |
| `--namespace` | empty | Filter by resource namespace. |
| `-n` | empty | Shorthand for `--namespace`. |
| `--once` | `false` | Exit after first matching event. |
| `--timeout` | `0` | Max stream time (`0` means no timeout). |
| `--raw` | `false` | Print raw event JSON payload. |

### `orlojctl logs`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |

### `orlojctl trace`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |

### `orlojctl graph`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |

### `orlojctl admin reset-password`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--username` | empty | Optional admin username. |
| `--new-password` | none | New password (required). |

### `orlojctl config set-profile`

| Flag | Default | Description |
|---|---|---|
| `--server` | empty | Profile API server URL. |
| `--token` | empty | Profile bearer token (prefer `--token-env` for secrets). |
| `--token-env` | empty | Env var name read at runtime for token value. |

Other config subcommands:

- `orlojctl config path`: print config file path
- `orlojctl config get`: print current config/profile data
- `orlojctl config use <name>`: switch active profile

### `orlojctl init`

| Flag | Default | Description |
|---|---|---|
| `--blueprint` | none | Required blueprint: `pipeline`, `hierarchical`, `swarm-loop`. |
| `--name` | blueprint name | Prefix for generated resource names. |
| `--dir` | `.` | Output directory. |

## `orlojd`

Print full flags:

```bash
go run ./cmd/orlojd -h
```

### Core, auth, and storage

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--version` | `false` | Print version and exit. | n/a |
| `--addr` | `:8080` | Server listen address. | n/a |
| `--api-key` | empty | Bearer token auth key. | Env fallback: `ORLOJ_API_TOKEN`; see also `ORLOJ_API_TOKENS`. |
| `--auth-mode` | `off` | API auth mode. | `off|native|sso` (`sso` unavailable in this distribution). |
| `--auth-session-ttl` | `24h` | Session TTL for local auth mode. | Env fallback: `ORLOJ_AUTH_SESSION_TTL`. |
| `--auth-reset-admin-username` | empty | One-shot admin reset username. | Env fallback: `ORLOJ_AUTH_RESET_ADMIN_USERNAME`. |
| `--auth-reset-admin-password` | empty | One-shot admin reset password and exit. | Env fallback: `ORLOJ_AUTH_RESET_ADMIN_PASSWORD`. |
| `--secret-encryption-key` | empty | AES-256-GCM key for Secret encryption at rest. | Env fallback: `ORLOJ_SECRET_ENCRYPTION_KEY`. |
| `--storage-backend` | `memory` | State backend. | `memory|postgres`. |
| `--postgres-dsn` | empty | Postgres DSN. | Required when `--storage-backend=postgres`; env `ORLOJ_POSTGRES_DSN`. |
| `--sql-driver` | `pgx` | `database/sql` driver for Postgres backend. | Postgres backend only. |
| `--postgres-max-open-conns` | `20` | Max open Postgres connections. | Postgres backend only. |
| `--postgres-max-idle-conns` | `10` | Max idle Postgres connections. | Postgres backend only. |
| `--postgres-conn-max-lifetime` | `30m` | Max Postgres connection lifetime. | Postgres backend only. |

### Task execution and embedded worker

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--reconcile-interval` | `2s` | Agent reconcile interval. | n/a |
| `--task-execution-mode` | `sequential` | Task execution mode. | `sequential|message-driven`; env `ORLOJ_TASK_EXECUTION_MODE`. |
| `--run-task-worker` | `false` | Run embedded task worker in `orlojd`. | Alias exists: `--embedded-worker`. |
| `--embedded-worker` | `false` | Alias for `--run-task-worker`. | n/a |
| `--task-worker-id` | `embedded-worker` | Embedded worker identity. | n/a |
| `--task-worker-region` | `default` | Embedded worker region. | Env fallback: `ORLOJ_TASK_WORKER_REGION`. |
| `--embedded-worker-max-concurrent-tasks` | `1` | Embedded worker max concurrent tasks. | Env fallback: `ORLOJ_EMBEDDED_WORKER_MAX_CONCURRENT_TASKS`. |
| `--task-lease-duration` | `30s` | Embedded worker task lease duration. | Embedded worker only. |
| `--task-heartbeat-interval` | `10s` | Embedded worker lease heartbeat interval. | Embedded worker only. |

### Event bus and runtime message bus

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--event-bus-backend` | `memory` | Control-plane event bus backend. | `memory|nats`; env `ORLOJ_EVENT_BUS_BACKEND`. |
| `--nats-url` | `nats://127.0.0.1:4222` | NATS URL for control-plane event bus. | Used when `--event-bus-backend=nats`; env `ORLOJ_NATS_URL`. |
| `--nats-subject-prefix` | `orloj.controlplane` | NATS subject prefix for control-plane events. | NATS event bus only; env `ORLOJ_NATS_SUBJECT_PREFIX`. |
| `--agent-message-bus-backend` | `none` | Runtime agent message bus backend. | `none|memory|nats-jetstream`; env `ORLOJ_AGENT_MESSAGE_BUS_BACKEND`. |
| `--agent-message-nats-url` | `nats://127.0.0.1:4222` | NATS URL for runtime agent messages. | Used when `nats-jetstream`; env `ORLOJ_AGENT_MESSAGE_NATS_URL` (falls back to `ORLOJ_NATS_URL`). |
| `--agent-message-subject-prefix` | `orloj.agentmsg` | Subject prefix for runtime agent messages. | Env `ORLOJ_AGENT_MESSAGE_SUBJECT_PREFIX`. |
| `--agent-message-stream-name` | `ORLOJ_AGENT_MESSAGES` | JetStream stream name for runtime messages. | Env `ORLOJ_AGENT_MESSAGE_STREAM`. |
| `--agent-message-history-max` | `2048` | In-memory runtime message history capacity. | In-memory runtime message backend behavior. |
| `--agent-message-dedupe-window` | `2m` | In-memory runtime message dedupe window. | In-memory runtime message backend behavior. |

### Model gateway

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--model-gateway-provider` | `mock` | Task model gateway provider. | `mock|openai|anthropic|azure-openai|ollama`; env `ORLOJ_MODEL_GATEWAY_PROVIDER`. |
| `--model-gateway-api-key` | empty | Explicit provider API key. | Env fallback: `ORLOJ_MODEL_GATEWAY_API_KEY`. |
| `--model-gateway-base-url` | empty | Provider base URL override. | Env fallback: `ORLOJ_MODEL_GATEWAY_BASE_URL`. |
| `--model-gateway-timeout` | `30s` | HTTP timeout for model gateway calls. | Env fallback: `ORLOJ_MODEL_GATEWAY_TIMEOUT`. |
| `--model-gateway-default-model` | empty | Fallback default model. | Env fallback: `ORLOJ_MODEL_GATEWAY_DEFAULT_MODEL`. |
| `--model-secret-env-prefix` | `ORLOJ_SECRET_` | Env prefix for model `secretRef` resolution. | Env fallback: `ORLOJ_MODEL_SECRET_ENV_PREFIX`. |

### Tool isolation runtime

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--tool-isolation-backend` | `none` | Tool isolation executor backend. | `none|container|wasm`; env `ORLOJ_TOOL_ISOLATION_BACKEND`. |
| `--tool-container-runtime` | `docker` | Container runtime binary. | Container backend; env `ORLOJ_TOOL_CONTAINER_RUNTIME`. |
| `--tool-container-image` | `curlimages/curl:8.8.0` | Container image for isolated tool calls. | Container backend; env `ORLOJ_TOOL_CONTAINER_IMAGE`. |
| `--tool-container-network` | `none` | Container network mode. | Container backend; env `ORLOJ_TOOL_CONTAINER_NETWORK`. |
| `--tool-container-memory` | `128m` | Container memory limit. | Container backend; env `ORLOJ_TOOL_CONTAINER_MEMORY`. |
| `--tool-container-cpus` | `0.50` | Container CPU limit. | Container backend; env `ORLOJ_TOOL_CONTAINER_CPUS`. |
| `--tool-container-pids-limit` | `64` | Container PID limit. | Container backend. |
| `--tool-container-user` | `65532:65532` | Container user. | Container backend; env `ORLOJ_TOOL_CONTAINER_USER`. |
| `--tool-secret-env-prefix` | `ORLOJ_SECRET_` | Env prefix for tool `secretRef` resolution. | Env fallback: `ORLOJ_TOOL_SECRET_ENV_PREFIX`. |
| `--tool-wasm-module` | empty | WASM module path or identifier. | WASM backend; env `ORLOJ_TOOL_WASM_MODULE`. |
| `--tool-wasm-entrypoint` | `run` | WASM entrypoint function. | WASM backend; env `ORLOJ_TOOL_WASM_ENTRYPOINT`. |
| `--tool-wasm-runtime-binary` | `wasmtime` | WASM runtime binary. | WASM backend; env `ORLOJ_TOOL_WASM_RUNTIME_BINARY`. |
| `--tool-wasm-runtime-args` | empty | Comma-separated args passed to WASM runtime binary. | WASM backend; env `ORLOJ_TOOL_WASM_RUNTIME_ARGS`. |
| `--tool-wasm-memory-bytes` | `67108864` | Max WASM memory bytes. | WASM backend; env `ORLOJ_TOOL_WASM_MEMORY_BYTES`. |
| `--tool-wasm-fuel` | `0` | WASM execution fuel limit. | `0` disables; env `ORLOJ_TOOL_WASM_FUEL`. |
| `--tool-wasm-wasi` | `true` | Enable WASI host functions. | WASM backend; env `ORLOJ_TOOL_WASM_WASI`. |

## `orlojworker`

Print full flags:

```bash
go run ./cmd/orlojworker -h
```

### Core, storage, and identity

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--version` | `false` | Print version and exit. | n/a |
| `--worker-id` | `worker-1` | Worker identity. | n/a |
| `--healthz-addr` | empty | Optional `/healthz` listener address. | Empty disables; env `ORLOJ_WORKER_HEALTHZ_ADDR`. |
| `--region` | `default` | Worker region. | n/a |
| `--gpu` | `false` | Declare GPU capability. | n/a |
| `--supported-models` | empty | Comma-separated supported model IDs. | n/a |
| `--max-concurrent-tasks` | `1` | Worker concurrency capacity. | n/a |
| `--storage-backend` | `postgres` | State backend. | `postgres|memory`. |
| `--postgres-dsn` | empty | Postgres DSN. | Required when `--storage-backend=postgres`; env `ORLOJ_POSTGRES_DSN`. |
| `--sql-driver` | `pgx` | `database/sql` driver for Postgres backend. | Postgres backend only. |
| `--postgres-max-open-conns` | `20` | Max open Postgres connections. | Postgres backend only. |
| `--postgres-max-idle-conns` | `10` | Max idle Postgres connections. | Postgres backend only. |
| `--postgres-conn-max-lifetime` | `30m` | Max Postgres connection lifetime. | Postgres backend only. |
| `--secret-encryption-key` | empty | AES-256-GCM key for Secret encryption at rest. | Env fallback: `ORLOJ_SECRET_ENCRYPTION_KEY`. |

### Task execution and runtime inbox consumers

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--reconcile-interval` | `1s` | Claim/reconcile interval. | n/a |
| `--lease-duration` | `30s` | Task lease duration. | n/a |
| `--heartbeat-interval` | `10s` | Lease heartbeat interval. | n/a |
| `--task-execution-mode` | `sequential` | Task execution mode. | `sequential|message-driven`; env `ORLOJ_TASK_EXECUTION_MODE`. |
| `--agent-message-bus-backend` | `none` | Runtime agent message bus backend. | `none|memory|nats-jetstream`; env `ORLOJ_AGENT_MESSAGE_BUS_BACKEND`. |
| `--agent-message-nats-url` | `nats://127.0.0.1:4222` | NATS URL for runtime agent messages. | Used when `nats-jetstream`; env `ORLOJ_AGENT_MESSAGE_NATS_URL` (fallback `ORLOJ_NATS_URL`). |
| `--agent-message-subject-prefix` | `orloj.agentmsg` | Subject prefix for runtime messages. | Env `ORLOJ_AGENT_MESSAGE_SUBJECT_PREFIX`. |
| `--agent-message-stream-name` | `ORLOJ_AGENT_MESSAGES` | JetStream stream name for runtime messages. | Env `ORLOJ_AGENT_MESSAGE_STREAM`. |
| `--agent-message-history-max` | `2048` | In-memory runtime message history capacity. | In-memory runtime message backend behavior. |
| `--agent-message-dedupe-window` | `2m` | In-memory runtime message dedupe window. | In-memory runtime message backend behavior. |
| `--agent-message-consume` | `false` | Enable runtime inbox consumers in worker. | Env fallback: `ORLOJ_AGENT_MESSAGE_CONSUME`. |
| `--agent-message-consumer-namespace` | empty | Namespace filter for runtime inbox consumers. | Env fallback: `ORLOJ_AGENT_MESSAGE_CONSUMER_NAMESPACE`. |
| `--agent-message-consumer-refresh` | `10s` | Consumer reconciliation interval. | n/a |
| `--agent-message-consumer-dedupe-window` | `10m` | Inbox processing dedupe window. | n/a |

### Model gateway

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--model-gateway-provider` | `mock` | Task model gateway provider. | `mock|openai|anthropic|azure-openai|ollama`; env `ORLOJ_MODEL_GATEWAY_PROVIDER`. |
| `--model-gateway-api-key` | empty | Explicit provider API key. | Env fallback: `ORLOJ_MODEL_GATEWAY_API_KEY`. |
| `--model-gateway-base-url` | empty | Provider base URL override. | Env fallback: `ORLOJ_MODEL_GATEWAY_BASE_URL`. |
| `--model-gateway-timeout` | `30s` | HTTP timeout for model gateway calls. | Env fallback: `ORLOJ_MODEL_GATEWAY_TIMEOUT`. |
| `--model-gateway-default-model` | empty | Fallback default model. | Env fallback: `ORLOJ_MODEL_GATEWAY_DEFAULT_MODEL`. |
| `--model-secret-env-prefix` | `ORLOJ_SECRET_` | Env prefix for model `secretRef` resolution. | Env fallback: `ORLOJ_MODEL_SECRET_ENV_PREFIX`. |

### Tool isolation runtime

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--tool-isolation-backend` | `none` | Tool isolation executor backend. | `none|container|wasm`; env `ORLOJ_TOOL_ISOLATION_BACKEND`. |
| `--tool-container-runtime` | `docker` | Container runtime binary. | Container backend; env `ORLOJ_TOOL_CONTAINER_RUNTIME`. |
| `--tool-container-image` | `curlimages/curl:8.8.0` | Container image for isolated tool calls. | Container backend; env `ORLOJ_TOOL_CONTAINER_IMAGE`. |
| `--tool-container-network` | `none` | Container network mode. | Container backend; env `ORLOJ_TOOL_CONTAINER_NETWORK`. |
| `--tool-container-memory` | `128m` | Container memory limit. | Container backend; env `ORLOJ_TOOL_CONTAINER_MEMORY`. |
| `--tool-container-cpus` | `0.50` | Container CPU limit. | Container backend; env `ORLOJ_TOOL_CONTAINER_CPUS`. |
| `--tool-container-pids-limit` | `64` | Container PID limit. | Container backend; env `ORLOJ_TOOL_CONTAINER_PIDS_LIMIT`. |
| `--tool-container-user` | `65532:65532` | Container user. | Container backend; env `ORLOJ_TOOL_CONTAINER_USER`. |
| `--tool-secret-env-prefix` | `ORLOJ_SECRET_` | Env prefix for tool `secretRef` resolution. | Env fallback: `ORLOJ_TOOL_SECRET_ENV_PREFIX`. |
| `--tool-wasm-module` | empty | WASM module path or identifier. | WASM backend; env `ORLOJ_TOOL_WASM_MODULE`. |
| `--tool-wasm-entrypoint` | `run` | WASM entrypoint function. | WASM backend; env `ORLOJ_TOOL_WASM_ENTRYPOINT`. |
| `--tool-wasm-runtime-binary` | `wasmtime` | WASM runtime binary. | WASM backend; env `ORLOJ_TOOL_WASM_RUNTIME_BINARY`. |
| `--tool-wasm-runtime-args` | empty | Comma-separated args passed to WASM runtime binary. | WASM backend; env `ORLOJ_TOOL_WASM_RUNTIME_ARGS`. |
| `--tool-wasm-memory-bytes` | `67108864` | Max WASM memory bytes. | WASM backend; env `ORLOJ_TOOL_WASM_MEMORY_BYTES`. |
| `--tool-wasm-fuel` | `0` | WASM execution fuel limit. | `0` disables; env `ORLOJ_TOOL_WASM_FUEL`. |
| `--tool-wasm-wasi` | `true` | Enable WASI host functions. | WASM backend; env `ORLOJ_TOOL_WASM_WASI`. |

## `orloj-loadtest`

Print full flags:

```bash
go run ./cmd/orloj-loadtest -h
```

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--base-url` | `http://127.0.0.1:8080` | Orloj API base URL. | n/a |
| `--namespace` | `default` | Target namespace. | n/a |
| `--tasks` | `50` | Number of tasks to create. | n/a |
| `--create-concurrency` | `10` | Concurrent task-create workers. | n/a |
| `--poll-concurrency` | `20` | Concurrent status-poll workers. | n/a |
| `--poll-interval` | `500ms` | Poll interval for task status. | n/a |
| `--run-timeout` | `5m` | Global run timeout. | n/a |
| `--task-system` | `report-system` | AgentSystem for generated tasks. | n/a |
| `--topic-prefix` | `loadtest-topic` | Task input topic prefix. | n/a |
| `--task-priority` | `high` | Task priority. | n/a |
| `--task-retry-attempts` | `3` | Generated `Task.spec.retry.max_attempts`. | n/a |
| `--task-retry-backoff` | `2s` | Generated `Task.spec.retry.backoff`. | n/a |
| `--message-retry-attempts` | `4` | Generated `Task.spec.message_retry.max_attempts`. | n/a |
| `--message-retry-backoff` | `200ms` | Generated `Task.spec.message_retry.backoff`. | n/a |
| `--message-retry-max-backoff` | `2s` | Generated `Task.spec.message_retry.max_backoff`. | n/a |
| `--message-retry-jitter` | `full` | Generated `Task.spec.message_retry.jitter`. | `none|full|equal`. |
| `--setup` | `true` | Apply baseline manifests before load run. | n/a |
| `--min-ready-workers` | `2` | Minimum ready workers required before run. | `0` disables check. |
| `--worker-ready-timeout` | `45s` | Max wait for worker readiness check. | n/a |
| `--inject-invalid-system-rate` | `0` | Fraction routed to invalid system. | Injection control. |
| `--invalid-system-name` | `missing-system-loadtest` | Invalid system name used for injection. | Injection control. |
| `--inject-timeout-system-rate` | `0` | Fraction routed to timeout system. | Injection control. |
| `--timeout-system-name` | `loadtest-timeout-system` | Timeout system name for injection. | Injection control. |
| `--timeout-agent-name` | `loadtest-timeout-agent` | Timeout-agent name in injected system. | Injection control. |
| `--timeout-agent-duration` | `1ms` | Timeout used by injected agent limits. | Injection control. |
| `--inject-expired-lease-rate` | `0` | Fraction patched for expired-lease takeover simulation. | Injection control. |
| `--expired-lease-owner` | `worker-crashed-simulated` | Synthetic owner ID used for expired-lease simulation. | Injection control. |
| `--quality-profile` | empty | Optional JSON profile for quality gates. | n/a |
| `--min-success-rate` | `0` | Minimum success-rate gate. | `0` disables. |
| `--max-deadletter-rate` | `-1` | Maximum deadletter-rate gate. | `-1` disables. |
| `--max-failed-rate` | `-1` | Maximum failed-rate gate. | `-1` disables. |
| `--max-timed-out` | `0` | Maximum timed-out task count gate. | `-1` disables. |
| `--min-retry-total` | `-1` | Minimum total retry-count gate. | `-1` disables. |
| `--min-takeover-events` | `-1` | Minimum takeover-history event-count gate. | `-1` disables. |
| `--json` | `false` | Emit machine-readable JSON report. | n/a |
| `--verbose` | `false` | Print periodic progress. | n/a |

## `orloj-alertcheck`

Print full flags:

```bash
go run ./cmd/orloj-alertcheck -h
```

| Flag | Default | Description |
|---|---|---|
| `--base-url` | `http://127.0.0.1:8080` | Orloj API base URL. |
| `--namespace` | `default` | Target namespace. |
| `--api-token` | empty | Optional bearer token for API auth (env fallback: `ORLOJ_API_TOKEN`). |
| `--profile` | `monitoring/alerts/retry-deadletter-default.json` | Alert threshold profile JSON file. |
| `--task-name-prefix` | empty | Optional task metadata.name prefix filter. |
| `--task-system` | empty | Optional `Task.spec.system` filter. |
| `--poll-concurrency` | `20` | Concurrent task metrics fetch workers. |
| `--timeout` | `2m` | Global command timeout. |
| `--json` | `true` | Emit JSON output. |
| `--verbose` | `false` | Emit verbose progress logs. |

## Command Discovery

Use help output as the authoritative source for your current build:

```bash
go run ./cmd/orlojctl help
go run ./cmd/orlojd -h
go run ./cmd/orlojworker -h
go run ./cmd/orloj-loadtest -h
go run ./cmd/orloj-alertcheck -h
```
