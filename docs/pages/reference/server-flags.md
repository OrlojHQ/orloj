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
| `--log-level` | `info` | Minimum log level. | `debug|info|warn|error`; env `ORLOJ_LOG_LEVEL`. |
| `--debug` | `false` | Enable debug logging. | Equivalent to `--log-level=debug`; takes precedence over `--log-level`. |
| `--addr` | `:8080` | Server listen address. | n/a |
| `--ui-path` | `/` | Base URL path for the web console. | Env fallback: `ORLOJ_UI_PATH`. Set to a subpath (e.g. `/console/`) when sharing a hostname via reverse proxy. |
| `--cors-allowed-origins` | empty | Comma-separated CORS allowed origins. | Env fallback: `ORLOJ_CORS_ALLOWED_ORIGINS`. Empty means same-origin only. |
| `--tls-cert-file` | empty | TLS certificate file for HTTPS. | Env fallback: `ORLOJ_TLS_CERT_FILE`. Requires `--tls-key-file`. |
| `--tls-key-file` | empty | TLS private key file for HTTPS. | Env fallback: `ORLOJ_TLS_KEY_FILE`. Requires `--tls-cert-file`. |
| `--api-key` | empty | Bearer token auth key. | Env fallback: `ORLOJ_API_TOKEN`; see also `ORLOJ_API_TOKENS`. Prefer env over flag (flag values are visible in process listings). |
| `--auth-mode` | `off` | API auth mode. | `off|native|sso` (`sso` unavailable in this distribution). |
| `--auth-session-ttl` | `24h` | Session TTL for local auth mode. | Env fallback: `ORLOJ_AUTH_SESSION_TTL`. |
| `--auth-reset-admin-username` | empty | One-shot admin reset username. | Env fallback: `ORLOJ_AUTH_RESET_ADMIN_USERNAME`. |
| `--auth-reset-admin-password` | empty | One-shot admin reset password and exit. | Env fallback: `ORLOJ_AUTH_RESET_ADMIN_PASSWORD`. Prefer env over flag. |
| `--trusted-proxies` | empty | Comma-separated CIDRs of reverse proxies whose `X-Forwarded-For` / `X-Real-IP` headers are trusted for client IP extraction. | Env fallback: `ORLOJ_TRUSTED_PROXIES`. Required for correct per-client auth rate limiting behind a proxy. See [Security — Trusted proxy configuration](../operations/security.md#trusted-proxy-configuration). |
| `--secret-encryption-key` | empty | AES-256-GCM key for Secret encryption at rest. | Env fallback: `ORLOJ_SECRET_ENCRYPTION_KEY`. Prefer env over flag. On `orlojd`, also wraps the DB-stored `SealedSecret` private key. |
| `--storage-backend` | `memory` | State backend. | `memory|postgres`. |
| `--postgres-dsn` | empty | Postgres DSN. | Required when `--storage-backend=postgres`; env `ORLOJ_POSTGRES_DSN`. |
| `--sql-driver` | `pgx` | `database/sql` driver for Postgres backend. | Postgres backend only. |
| `--postgres-max-open-conns` | `20` | Max open Postgres connections. | Postgres backend only. |
| `--postgres-max-idle-conns` | `10` | Max idle Postgres connections. | Postgres backend only. |
| `--postgres-conn-max-lifetime` | `30m` | Max Postgres connection lifetime. | Postgres backend only. |

### A2A protocol

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--a2a-public-base-url` | empty | Public base URL for Agent Card `url` fields. | Env `ORLOJ_A2A_PUBLIC_BASE_URL`. Required for externally-reachable Agent Cards. |
| `--a2a-protocol-version` | empty | A2A protocol version to advertise. | Env `ORLOJ_A2A_PROTOCOL_VERSION`. |
| `--a2a-card-cache-ttl` | `5m` | TTL for cached remote Agent Cards. | Env `ORLOJ_A2A_CARD_CACHE_TTL`. |
| `--a2a-allow-private-endpoints` | `false` | Allow outbound A2A requests to private/loopback IPs. | Env `ORLOJ_A2A_ALLOW_PRIVATE_ENDPOINTS`. See [Security — A2A Security](../operations/security.md#a2a-security). |
| `--a2a-remote-agents` | empty | JSON-encoded list of static remote A2A agents. | Env `ORLOJ_A2A_REMOTE_AGENTS`. |
| `--a2a-rate-limit-enabled` | `true` | Enable per-IP rate limiting for A2A endpoints. | Env `ORLOJ_A2A_RATE_LIMIT_ENABLED`. |
| `--a2a-rate-limit-rpm` | `30` | Max A2A JSON-RPC requests per minute per IP. | Env `ORLOJ_A2A_RATE_LIMIT_RPM`. |
| `--a2a-rate-limit-max-subscribe` | `10` | Max concurrent SSE subscribe connections globally (server-wide). | Env `ORLOJ_A2A_RATE_LIMIT_MAX_SUBSCRIBE`. |

### CRD conflict policy

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--crd-conflict-policy` | `warn` | How `orlojd` handles REST API writes to CRD-managed resources. | `off|warn|reject`; env `ORLOJ_CRD_CONFLICT_POLICY`. Only relevant when the CRD operator is also running. |

Modes:

- **`off`** — No conflict detection. REST writes proceed normally even if the resource is CRD-managed.
- **`warn`** (default) — REST writes succeed, but `orlojd` logs a warning and sets the `X-Orloj-CRD-Managed: true` response header. The operator will overwrite the change on its next reconcile.
- **`reject`** — REST writes to CRD-managed resources return `409 Conflict` with a message directing the user to update via `kubectl apply` or Git.

See [Kubernetes CRD Operator](../deploy/kubernetes-operator.md) for full operator documentation.

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
| `--tool-isolation-backend` | `none` | Container isolation backend for tool sandboxing. | `none|container`; env `ORLOJ_TOOL_ISOLATION_BACKEND`. |
| `--tool-container-runtime` | `docker` | Container runtime binary. | Container backend; env `ORLOJ_TOOL_CONTAINER_RUNTIME`. |
| `--tool-container-image` | `curlimages/curl:8.8.0` | Container image for isolated tool calls. | Container backend; env `ORLOJ_TOOL_CONTAINER_IMAGE`. |
| `--tool-container-network` | `none` | Container network mode. | Container backend; env `ORLOJ_TOOL_CONTAINER_NETWORK`. |
| `--tool-container-memory` | `128m` | Default container memory limit. Per-tool `spec.cli.resources.memory` and per-McpServer `spec.resources.memory` take precedence when set. | Container backend; env `ORLOJ_TOOL_CONTAINER_MEMORY`. |
| `--tool-container-cpus` | `0.50` | Default container CPU limit. Per-tool `spec.cli.resources.cpus` and per-McpServer `spec.resources.cpus` take precedence when set. | Container backend; env `ORLOJ_TOOL_CONTAINER_CPUS`. |
| `--tool-container-pids-limit` | `64` | Default container PID limit. Per-tool `spec.cli.resources.pids_limit` and per-McpServer `spec.resources.pids_limit` take precedence when set. | Container backend; env `ORLOJ_TOOL_CONTAINER_PIDS_LIMIT`. |
| `--tool-container-user` | `65532:65532` | Container user. | Container backend; env `ORLOJ_TOOL_CONTAINER_USER`. |
| `--tool-container-max-memory` | empty | Operator ceiling for per-tool/McpServer `resources.memory`. Empty means unbounded. Manifests exceeding this are rejected at apply time. | `orlojd` only; env `ORLOJ_TOOL_CONTAINER_MAX_MEMORY`. |
| `--tool-container-max-cpus` | empty | Operator ceiling for per-tool/McpServer `resources.cpus`. Empty means unbounded. | `orlojd` only; env `ORLOJ_TOOL_CONTAINER_MAX_CPUS`. |
| `--tool-container-max-pids-limit` | `0` | Operator ceiling for per-tool/McpServer `resources.pids_limit`. 0 means unbounded. | `orlojd` only; env `ORLOJ_TOOL_CONTAINER_MAX_PIDS_LIMIT`. |
| `--tool-secret-env-prefix` | `ORLOJ_SECRET_` | Env prefix for tool `secretRef` resolution. | Env fallback: `ORLOJ_TOOL_SECRET_ENV_PREFIX`. |
| `--tool-wasm-module` | empty | Default WASM module path (per-tool `spec.wasm.module` takes precedence). | Always available; env `ORLOJ_TOOL_WASM_MODULE`. |
| `--tool-wasm-entrypoint` | `run` | Default WASM entrypoint function. | Always available; env `ORLOJ_TOOL_WASM_ENTRYPOINT`. |
| `--tool-wasm-memory-bytes` | `67108864` | Default max WASM memory bytes. | Always available; env `ORLOJ_TOOL_WASM_MEMORY_BYTES`. |
| `--tool-wasm-fuel` | `1000000` | Default WASM execution fuel limit. | Always available; env `ORLOJ_TOOL_WASM_FUEL`. |
| `--tool-wasm-wasi` | `true` | Default: enable WASI host functions. | Always available; env `ORLOJ_TOOL_WASM_WASI`. |
| `--tool-wasm-cache-dir` | `~/.orloj/wasm-cache` | Disk cache directory for remote WASM modules (HTTPS/OCI). | Always available; env `ORLOJ_TOOL_WASM_CACHE_DIR`. |
| `--tool-k8s-enabled` | `false` | Enable Kubernetes tool isolation runtime. | Env `ORLOJ_TOOL_K8S_ENABLED`. When true, tools with `isolation_mode: kubernetes` run as K8s Jobs. |
| `--tool-k8s-namespace` | pod namespace or `default` | Namespace for tool Jobs. | Env `ORLOJ_TOOL_K8S_NAMESPACE`. |
| `--tool-k8s-service-account` | empty | Service account for tool Pods. | Env `ORLOJ_TOOL_K8S_SERVICE_ACCOUNT`. |
| `--tool-k8s-job-ttl` | `300` | TTL seconds after Job finishes (`ttlSecondsAfterFinished`). | Env `ORLOJ_TOOL_K8S_JOB_TTL`. |
| `--tool-k8s-default-image` | `curlimages/curl:8.8.0` | Fallback image for HTTP tools without an explicit image. | Env `ORLOJ_TOOL_K8S_DEFAULT_IMAGE`. |

### Agent Kubernetes execution

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--agent-k8s-enabled` | `false` | Run agents as ephemeral K8s Jobs. | Env `ORLOJ_AGENT_K8S_ENABLED`. Agents with Docker-dependent tools fall back to in-process. |
| `--agent-k8s-namespace` | pod namespace or `default` | Namespace for agent Jobs. | Env `ORLOJ_AGENT_K8S_NAMESPACE`. |
| `--agent-k8s-service-account` | empty | Service account for agent Pods. | Env `ORLOJ_AGENT_K8S_SERVICE_ACCOUNT`. |
| `--agent-k8s-image` | own image | Container image for agent Jobs. | Env `ORLOJ_AGENT_K8S_IMAGE`. Defaults to the running binary's own image. |
| `--agent-k8s-job-ttl` | `600` | TTL seconds after Job finishes (`ttlSecondsAfterFinished`). | Env `ORLOJ_AGENT_K8S_JOB_TTL`. |
| `--agent-k8s-default-memory` | `512Mi` | Default memory limit for agent Pods. | Env `ORLOJ_AGENT_K8S_DEFAULT_MEMORY`. |
| `--agent-k8s-default-cpu` | `500m` | Default CPU limit for agent Pods. | Env `ORLOJ_AGENT_K8S_DEFAULT_CPU`. |

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
| `--log-level` | `info` | Minimum log level. | `debug|info|warn|error`; env `ORLOJ_LOG_LEVEL`. |
| `--debug` | `false` | Enable debug logging. | Equivalent to `--log-level=debug`; takes precedence over `--log-level`. |
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
| `--secret-encryption-key` | empty | AES-256-GCM key for Secret encryption at rest. | Env fallback: `ORLOJ_SECRET_ENCRYPTION_KEY`. Workers do not use the `SealedSecret` private key. |

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
| `--tool-isolation-backend` | `none` | Container isolation backend for tool sandboxing. | `none|container`; env `ORLOJ_TOOL_ISOLATION_BACKEND`. |
| `--tool-container-runtime` | `docker` | Container runtime binary. | Container backend; env `ORLOJ_TOOL_CONTAINER_RUNTIME`. |
| `--tool-container-image` | `curlimages/curl:8.8.0` | Container image for isolated tool calls. | Container backend; env `ORLOJ_TOOL_CONTAINER_IMAGE`. |
| `--tool-container-network` | `none` | Container network mode. | Container backend; env `ORLOJ_TOOL_CONTAINER_NETWORK`. |
| `--tool-container-memory` | `128m` | Default container memory limit. Per-tool `spec.cli.resources.memory` takes precedence when set. | Container backend; env `ORLOJ_TOOL_CONTAINER_MEMORY`. |
| `--tool-container-cpus` | `0.50` | Default container CPU limit. Per-tool `spec.cli.resources.cpus` takes precedence when set. | Container backend; env `ORLOJ_TOOL_CONTAINER_CPUS`. |
| `--tool-container-pids-limit` | `64` | Default container PID limit. Per-tool `spec.cli.resources.pids_limit` takes precedence when set. | Container backend; env `ORLOJ_TOOL_CONTAINER_PIDS_LIMIT`. |
| `--tool-container-user` | `65532:65532` | Container user. | Container backend; env `ORLOJ_TOOL_CONTAINER_USER`. |
| `--tool-secret-env-prefix` | `ORLOJ_SECRET_` | Env prefix for tool `secretRef` resolution. | Env fallback: `ORLOJ_TOOL_SECRET_ENV_PREFIX`. |
| `--tool-wasm-module` | empty | Default WASM module path (per-tool `spec.wasm.module` takes precedence). | Always available; env `ORLOJ_TOOL_WASM_MODULE`. |
| `--tool-wasm-entrypoint` | `run` | Default WASM entrypoint function. | Always available; env `ORLOJ_TOOL_WASM_ENTRYPOINT`. |
| `--tool-wasm-memory-bytes` | `67108864` | Default max WASM memory bytes. | Always available; env `ORLOJ_TOOL_WASM_MEMORY_BYTES`. |
| `--tool-wasm-fuel` | `1000000` | Default WASM execution fuel limit. | Always available; env `ORLOJ_TOOL_WASM_FUEL`. |
| `--tool-wasm-wasi` | `true` | Default: enable WASI host functions. | Always available; env `ORLOJ_TOOL_WASM_WASI`. |
| `--tool-wasm-cache-dir` | `~/.orloj/wasm-cache` | Disk cache directory for remote WASM modules (HTTPS/OCI). | Always available; env `ORLOJ_TOOL_WASM_CACHE_DIR`. |
| `--tool-k8s-enabled` | `false` | Enable Kubernetes tool isolation runtime. | Env `ORLOJ_TOOL_K8S_ENABLED`. When true, tools with `isolation_mode: kubernetes` run as K8s Jobs. |
| `--tool-k8s-namespace` | pod namespace or `default` | Namespace for tool Jobs. | Env `ORLOJ_TOOL_K8S_NAMESPACE`. |
| `--tool-k8s-service-account` | empty | Service account for tool Pods. | Env `ORLOJ_TOOL_K8S_SERVICE_ACCOUNT`. |
| `--tool-k8s-job-ttl` | `300` | TTL seconds after Job finishes (`ttlSecondsAfterFinished`). | Env `ORLOJ_TOOL_K8S_JOB_TTL`. |
| `--tool-k8s-default-image` | `curlimages/curl:8.8.0` | Fallback image for HTTP tools without an explicit image. | Env `ORLOJ_TOOL_K8S_DEFAULT_IMAGE`. |

### Agent Kubernetes execution

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--agent-k8s-enabled` | `false` | Run agents as ephemeral K8s Jobs. | Env `ORLOJ_AGENT_K8S_ENABLED`. Agents with Docker-dependent tools fall back to in-process. |
| `--agent-k8s-namespace` | pod namespace or `default` | Namespace for agent Jobs. | Env `ORLOJ_AGENT_K8S_NAMESPACE`. |
| `--agent-k8s-service-account` | empty | Service account for agent Pods. | Env `ORLOJ_AGENT_K8S_SERVICE_ACCOUNT`. |
| `--agent-k8s-image` | own image | Container image for agent Jobs. | Env `ORLOJ_AGENT_K8S_IMAGE`. Defaults to the running binary's own image. |
| `--agent-k8s-job-ttl` | `600` | TTL seconds after Job finishes (`ttlSecondsAfterFinished`). | Env `ORLOJ_AGENT_K8S_JOB_TTL`. |
| `--agent-k8s-default-memory` | `512Mi` | Default memory limit for agent Pods. | Env `ORLOJ_AGENT_K8S_DEFAULT_MEMORY`. |
| `--agent-k8s-default-cpu` | `500m` | Default CPU limit for agent Pods. | Env `ORLOJ_AGENT_K8S_DEFAULT_CPU`. |
| `--single-agent` | `false` | Run a single agent execution (used by K8s agent Jobs). | Internal flag; not for manual use. |
| `--task-id` | empty | Task ID for single-agent mode. | Used with `--single-agent`. |
| `--agent-name` | empty | Agent name for single-agent mode. | Used with `--single-agent`. |
| `--attempt` | `0` | Attempt number for single-agent mode. | Used with `--single-agent`. |
| `--message-id` | empty | Message ID for single-agent mode. | Used with `--single-agent`. |

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
