# Configuration

This page is the canonical reference for runtime environment variables and flag-to-env precedence for `orlojd`, `orlojworker`, and `orlojctl`.

See also [CLI reference](../reference/cli.md) for exhaustive flag definitions.

## Precedence

1. CLI flags
2. Environment variable fallback
3. Code defaults

Example:

- `--task-execution-mode` overrides `ORLOJ_TASK_EXECUTION_MODE`.
- If neither is set, code defaults apply.

## Runtime Environment Matrix

| Variable | Used By | Flag Overrides | Purpose / Conditions |
|---|---|---|---|
| `ORLOJ_POSTGRES_DSN` | `orlojd`, `orlojworker` | `--postgres-dsn` | Postgres DSN when `--storage-backend=postgres`. |
| `ORLOJ_TASK_EXECUTION_MODE` | `orlojd`, `orlojworker` | `--task-execution-mode` | Task execution mode: `sequential` or `message-driven`. |
| `ORLOJ_EMBEDDED_WORKER_MAX_CONCURRENT_TASKS` | `orlojd` | `--embedded-worker-max-concurrent-tasks` | Embedded worker default concurrency. |
| `ORLOJ_TASK_WORKER_REGION` | `orlojd` | `--task-worker-region` | Region for embedded worker registration. |
| `ORLOJ_WORKER_HEALTHZ_ADDR` | `orlojworker` | `--healthz-addr` | Optional worker liveness endpoint bind address. |
| `ORLOJ_MODEL_SECRET_ENV_PREFIX` | `orlojd`, `orlojworker` | `--model-secret-env-prefix` | Env prefix for model endpoint `secretRef` lookups. |
| `ORLOJ_TOOL_ISOLATION_BACKEND` | `orlojd`, `orlojworker` | `--tool-isolation-backend` | Container isolation backend: `none` or `container`. WASM tools run independently. |
| `ORLOJ_TOOL_CONTAINER_RUNTIME` | `orlojd`, `orlojworker` | `--tool-container-runtime` | Container runtime binary for tool isolation. |
| `ORLOJ_TOOL_CONTAINER_IMAGE` | `orlojd`, `orlojworker` | `--tool-container-image` | Container image used by isolated tool execution. |
| `ORLOJ_TOOL_CONTAINER_NETWORK` | `orlojd`, `orlojworker` | `--tool-container-network` | Container network mode for isolated tools. |
| `ORLOJ_TOOL_CONTAINER_MEMORY` | `orlojd`, `orlojworker` | `--tool-container-memory` | Container memory limit for isolated tools. |
| `ORLOJ_TOOL_CONTAINER_CPUS` | `orlojd`, `orlojworker` | `--tool-container-cpus` | Container CPU limit for isolated tools. |
| `ORLOJ_TOOL_CONTAINER_PIDS_LIMIT` | `orlojworker` | `--tool-container-pids-limit` | Container PID limit for isolated tools. |
| `ORLOJ_TOOL_CONTAINER_USER` | `orlojd`, `orlojworker` | `--tool-container-user` | Container user/group for isolated tools. |
| `ORLOJ_TOOL_SECRET_ENV_PREFIX` | `orlojd`, `orlojworker` | `--tool-secret-env-prefix` | Env prefix for tool `secretRef` lookups. |
| `ORLOJ_TOOL_WASM_MODULE` | `orlojd`, `orlojworker` | `--tool-wasm-module` | Default WASM module path (per-tool `spec.wasm.module` takes precedence). |
| `ORLOJ_TOOL_WASM_ENTRYPOINT` | `orlojd`, `orlojworker` | `--tool-wasm-entrypoint` | Default WASM entrypoint function name. |
| `ORLOJ_TOOL_WASM_MEMORY_BYTES` | `orlojd`, `orlojworker` | `--tool-wasm-memory-bytes` | Default max memory bytes for WASM runtime. |
| `ORLOJ_TOOL_WASM_FUEL` | `orlojd`, `orlojworker` | `--tool-wasm-fuel` | Default WASM execution fuel limit. |
| `ORLOJ_TOOL_WASM_WASI` | `orlojd`, `orlojworker` | `--tool-wasm-wasi` | Default: enable WASI host functions for WASM tools. |
| `ORLOJ_TOOL_WASM_CACHE_DIR` | `orlojd`, `orlojworker` | `--tool-wasm-cache-dir` | Disk cache directory for remote WASM modules (HTTPS/OCI). Default: `~/.orloj/wasm-cache`. |
| `ORLOJ_TOOL_K8S_ENABLED` | `orlojd`, `orlojworker` | `--tool-k8s-enabled` | Enable Kubernetes tool isolation runtime. Default: `false`. |
| `ORLOJ_TOOL_K8S_NAMESPACE` | `orlojd`, `orlojworker` | `--tool-k8s-namespace` | Namespace for tool Jobs. Default: current pod namespace or `default`. |
| `ORLOJ_TOOL_K8S_SERVICE_ACCOUNT` | `orlojd`, `orlojworker` | `--tool-k8s-service-account` | Service account for tool Pods. |
| `ORLOJ_TOOL_K8S_JOB_TTL` | `orlojd`, `orlojworker` | `--tool-k8s-job-ttl` | TTL seconds after Job finishes. Default: `300`. |
| `ORLOJ_TOOL_K8S_DEFAULT_IMAGE` | `orlojd`, `orlojworker` | `--tool-k8s-default-image` | Fallback image for HTTP tools without an explicit image. Default: `curlimages/curl:8.8.0`. |
| `ORLOJ_EVENT_BUS_BACKEND` | `orlojd` | `--event-bus-backend` | Control-plane event bus backend: `memory` or `nats`. |
| `ORLOJ_NATS_URL` | `orlojd`, `orlojworker` | `--nats-url` (server), `--agent-message-nats-url` (runtime bus) | Base NATS URL; also fallback for runtime message bus URL. |
| `ORLOJ_NATS_SUBJECT_PREFIX` | `orlojd` | `--nats-subject-prefix` | Subject prefix used for control-plane NATS event bus. |
| `ORLOJ_AGENT_MESSAGE_BUS_BACKEND` | `orlojd`, `orlojworker` | `--agent-message-bus-backend` | Runtime message bus backend: `none`, `memory`, `nats-jetstream`. |
| `ORLOJ_AGENT_MESSAGE_NATS_URL` | `orlojd`, `orlojworker` | `--agent-message-nats-url` | NATS URL used when runtime bus backend is `nats-jetstream`. |
| `ORLOJ_AGENT_MESSAGE_SUBJECT_PREFIX` | `orlojd`, `orlojworker` | `--agent-message-subject-prefix` | Subject prefix for runtime agent messages. |
| `ORLOJ_AGENT_MESSAGE_STREAM` | `orlojd`, `orlojworker` | `--agent-message-stream-name` | JetStream stream name for runtime agent messages. |
| `ORLOJ_AGENT_MESSAGE_CONSUME` | `orlojworker` | `--agent-message-consume` | Enables worker-side runtime inbox consumers. |
| `ORLOJ_AGENT_MESSAGE_CONSUMER_NAMESPACE` | `orlojworker` | `--agent-message-consumer-namespace` | Optional namespace filter for runtime inbox consumers. |
| `ORLOJ_API_TOKEN` | `orlojd`, `orlojctl`, `orloj-alertcheck` | `--api-key` (server), `--api-token` (client/checker) | Bearer token fallback for API auth. |
| `ORLOJ_API_TOKENS` | `orlojd` | none | Multi-token auth map (`name:token:role` entries; legacy `token:role` supported). |
| `ORLOJ_UI_PATH` | `orlojd` | `--ui-path` | Base URL path for the web console (default `/`). |
| `ORLOJ_CORS_ALLOWED_ORIGINS` | `orlojd` | `--cors-allowed-origins` | Comma-separated CORS allowed origins. Empty means same-origin only. |
| `ORLOJ_TLS_CERT_FILE` | `orlojd` | `--tls-cert-file` | TLS certificate file for native HTTPS. Requires `ORLOJ_TLS_KEY_FILE`. |
| `ORLOJ_TLS_KEY_FILE` | `orlojd` | `--tls-key-file` | TLS private key file for native HTTPS. Requires `ORLOJ_TLS_CERT_FILE`. |
| `ORLOJ_AUTH_MODE` | `orlojd` | `--auth-mode` | API auth mode (`off`, `native`, `sso`; `sso` unavailable in this distribution). |
| `ORLOJ_AUTH_SESSION_TTL` | `orlojd` | `--auth-session-ttl` | Session TTL for native auth mode. |
| `ORLOJ_AUTH_RESET_ADMIN_USERNAME` | `orlojd` | `--auth-reset-admin-username` | One-shot local admin reset username. |
| `ORLOJ_AUTH_RESET_ADMIN_PASSWORD` | `orlojd` | `--auth-reset-admin-password` | One-shot local admin reset password and exit. |
| `ORLOJ_SETUP_TOKEN` | `orlojd` | none | Protects `/v1/auth/setup`; required request value for initial setup when set. |
| `ORLOJ_SECRET_ENCRYPTION_KEY` | `orlojd`, `orlojworker` | `--secret-encryption-key` | AES key for encrypting Secret resource data at rest. On `orlojd`, it also wraps the stored `SealedSecret` private key. |
| `ORLOJ_SECRET_<name>` | `orlojd`, `orlojworker` | `--model-secret-env-prefix`, `--tool-secret-env-prefix` | Dynamic secret lookup fallback for `secretRef` resolution. |
| `ORLOJ_SERVER` | `orlojctl` | `--server` | Default API base URL after `ORLOJCTL_SERVER`. |
| `ORLOJCTL_SERVER` | `orlojctl` | `--server` | Highest-precedence env default API base URL. |
| `ORLOJCTL_API_TOKEN` | `orlojctl` | `--api-token` | Bearer token for CLI API calls. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `orlojd`, `orlojworker` | none | OTLP gRPC endpoint for OpenTelemetry traces. Empty disables export. |
| `OTEL_EXPORTER_OTLP_INSECURE` | `orlojd`, `orlojworker` | none | Set `true` for non-TLS OTLP in development. |
| `ORLOJ_LOG_LEVEL` | `orlojd`, `orlojworker` | `--log-level`, `--debug` | Minimum log level: `debug`, `info` (default), `warn`, or `error`. `--debug` is equivalent to `--log-level=debug` and takes precedence. |
| `ORLOJ_LOG_FORMAT` | `orlojd`, `orlojworker` | none | Log format: `json` (default) or `text`. |

## A2A Protocol

| Variable | Used By | Flag Overrides | Purpose / Conditions |
|---|---|---|---|
| `ORLOJ_A2A_PUBLIC_BASE_URL` | `orlojd` | `--a2a-public-base-url` | Public base URL used in Agent Card `url` fields. Required for externally-reachable cards. |
| `ORLOJ_A2A_PROTOCOL_VERSION` | `orlojd` | `--a2a-protocol-version` | A2A protocol version string to advertise in Agent Cards. |
| `ORLOJ_A2A_CARD_CACHE_TTL` | `orlojd`, `orlojworker` | `--a2a-card-cache-ttl` | Cache TTL for fetched remote Agent Cards. Default: `5m`. |
| `ORLOJ_A2A_ALLOW_PRIVATE_ENDPOINTS` | `orlojd`, `orlojworker` | `--a2a-allow-private-endpoints` | Allow outbound A2A requests to private/loopback IPs. Default: `false`. |
| `ORLOJ_A2A_REMOTE_AGENTS` | `orlojd` | `--a2a-remote-agents` | JSON-encoded list of static remote A2A agents to register. |
| `ORLOJ_A2A_RATE_LIMIT_ENABLED` | `orlojd` | `--a2a-rate-limit-enabled` | Enable per-IP rate limiting on A2A endpoints. Default: `true`. |
| `ORLOJ_A2A_RATE_LIMIT_RPM` | `orlojd` | `--a2a-rate-limit-rpm` | Max JSON-RPC requests per minute per IP when rate limiting is enabled. Default: `30`. |
| `ORLOJ_A2A_RATE_LIMIT_MAX_SUBSCRIBE` | `orlojd` | `--a2a-rate-limit-max-subscribe` | Max concurrent SSE subscribe connections globally. Default: `10`. |

## Server and Worker Flags

Use [CLI reference](../reference/cli.md) as the exhaustive list for all flags and defaults.

Quick grouping:

- Server (`orlojd`): auth, storage, embedded worker, control-plane event bus, runtime message bus, model secret resolution, tool isolation.
- Worker (`orlojworker`): identity/capacity, storage, runtime inbox consumers, model secret resolution, tool isolation.

## Web Console Path

By default, `orlojd` serves the built-in web console at the root path (`/`). The REST API lives under `/v1/...`, `/healthz`, and `/metrics`, so there is no collision.

To mount the console at a subpath instead (useful when multiple services share a single reverse proxy hostname):

```bash
# Serve the console at https://tools.example.com/orloj/
orlojd --ui-path=/orloj/
# or
ORLOJ_UI_PATH=/orloj/ orlojd
```

| Setting | Console URL | API URL |
|---|---|---|
| `--ui-path=/` (default) | `https://example.com/` | `https://example.com/v1/...` |
| `--ui-path=/console/` | `https://example.com/console/` | `https://example.com/v1/...` |
| `--ui-path=/orloj/` | `https://tools.example.com/orloj/` | `https://tools.example.com/v1/...` |

The value is normalized to always have a leading and trailing `/`. Client-side routes (e.g. `/tasks/my-task`) are served via SPA fallback so browser refreshes work at any depth.

When using a custom DNS (e.g. `console.example.com`), you typically do **not** need to set `ORLOJ_UI_PATH` — the default `/` means the console is at `https://console.example.com/`. Point your DNS and reverse proxy at `orlojd` and everything works.

## Secret Resolution

Model endpoints and tools resolve `secretRef` values in this order:

1. Secret resources in the control-plane store.
2. Environment variables with configurable prefixes (`ORLOJ_SECRET_<name>` by default).

### Encryption at Rest

Set `--secret-encryption-key` (or `ORLOJ_SECRET_ENCRYPTION_KEY`) on every process sharing the same backing store.

- Use one consistent key for all `orlojd`/`orlojworker` processes against the same database.
- On `orlojd`, the same key also protects the persisted `SealedSecret` private key.
- Rotating keys requires a migration procedure (see security/upgrade runbooks).

## Postgres Tuning

### Connection Pool (main store)

The main Postgres pool is configured via CLI flags:

| Flag | Default | Description |
|---|---|---|
| `--postgres-max-open-conns` | 20 | Maximum open connections |
| `--postgres-max-idle-conns` | 10 | Maximum idle connections kept warm |
| `--postgres-conn-max-lifetime` | 30m | Maximum lifetime of a connection before recycling |

Idle connections are evicted after 5 minutes to reduce stale TCP connection risk behind firewalls/load balancers.

### Connection Pool (pgvector memory backend)

The pgvector backend uses a separate `pgxpool` created from the Memory resource `spec.endpoint` DSN. Tune it with DSN params:

```text
postgres://user:pass@host:5432/db?pool_max_conns=10&pool_min_conns=2&pool_max_conn_idle_time=5m&pool_health_check_period=1m
```

| Parameter | Default | Description |
|---|---|---|
| `pool_max_conns` | max(4, NumCPU) | Maximum pool size |
| `pool_min_conns` | 0 | Minimum warm connections |
| `pool_max_conn_lifetime` | 1h | Recycle connections after this duration |
| `pool_max_conn_idle_time` | 30m | Close idle connections after this duration |
| `pool_health_check_period` | 1m | How often to ping idle connections |

### Statement Timeout

Neither the main store nor pgvector backend sets `statement_timeout` by default. Add it via DSN `options`:

```bash
# Main store (30-second statement timeout)
--postgres-dsn="postgres://user:pass@host:5432/db?options=-c%20statement_timeout%3D30000"

# pgvector memory endpoint
postgres://user:pass@host:5432/db?options=-c%20statement_timeout%3D30000&pool_max_conns=10
```

## Recommended Production Baseline

- `orlojd`: `--storage-backend=postgres`, `--task-execution-mode=message-driven`, `--agent-message-bus-backend=nats-jetstream`
- `orlojworker`: `--storage-backend=postgres`, `--task-execution-mode=message-driven`, `--agent-message-consume`
- Enable `--secret-encryption-key` on all processes when using Secret resources
- Configure model/tool credentials via `ORLOJ_SECRET_<name>` or external secret management
- Set `OTEL_EXPORTER_OTLP_ENDPOINT` for distributed tracing
- See [Observability](./observability.md) for tracing, metrics, and logs setup

## Verification

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl get workers
go run ./cmd/orlojctl get tasks
```
