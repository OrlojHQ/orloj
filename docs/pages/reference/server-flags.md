# Server Flags

Flag reference for the `orlojd` (API server) and `orlojworker` (task worker) daemon binaries.

Both binaries share the same flag groups for **tool isolation**, **model secret resolution**, and **message bus** configuration. Flags that differ between the two are noted in the Condition / Notes column.

See [Configuration](../operations/configuration.md) for the full environment-variable matrix and precedence rules.

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
| `--ui-path` | `/` | Base URL path for the web console. | Env fallback: `ORLOJ_UI_PATH`. Set to a subpath (e.g. `/console/`) when sharing a hostname via reverse proxy. |
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

### Model secret resolution

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--model-secret-env-prefix` | `ORLOJ_SECRET_` | Env prefix for model `secretRef` resolution. | Env fallback: `ORLOJ_MODEL_SECRET_ENV_PREFIX`. |

Model routing (provider, base URL, default model, API key, timeout) is configured exclusively via **ModelEndpoint** resources. Agents reference endpoints through `spec.model_ref`. See [Configure Model Routing](../guides/configure-model-routing.md).

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

---

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

### Model secret resolution

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--model-secret-env-prefix` | `ORLOJ_SECRET_` | Env prefix for model `secretRef` resolution. | Env fallback: `ORLOJ_MODEL_SECRET_ENV_PREFIX`. |

Model routing (provider, base URL, default model, API key, timeout) is configured exclusively via **ModelEndpoint** resources. Agents reference endpoints through `spec.model_ref`. See [Configure Model Routing](../guides/configure-model-routing.md).

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

## Command Discovery

Use help output as the authoritative source for your current build:

```bash
go run ./cmd/orlojd -h
go run ./cmd/orlojworker -h
```

## Related

- [Configuration](../operations/configuration.md) — full env-variable matrix and precedence rules
- [CLI (orlojctl)](./cli.md) — user-facing CLI reference
- [Deployment](../deploy/) — deployment guides for all targets
