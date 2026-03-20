# Configuration

This page defines runtime configuration for `orlojd`, `orlojworker`, and client-side defaults for `orlojctl` (see also [CLI reference](../reference/cli.md)).

## Precedence

1. CLI flags
2. Environment variable fallback
3. Code defaults

Example:

- `--model-gateway-provider` overrides `ORLOJ_MODEL_GATEWAY_PROVIDER`.
- If neither is set, default is `mock`.

## Core Environment Variables

| Variable | Used By | Purpose |
|---|---|---|
| `ORLOJ_POSTGRES_DSN` | `orlojd`, `orlojworker` | Postgres DSN when `--storage-backend=postgres`. |
| `ORLOJ_TASK_EXECUTION_MODE` | `orlojd`, `orlojworker` | `sequential` or `message-driven`. |
| `ORLOJ_MODEL_GATEWAY_PROVIDER` | `orlojd`, `orlojworker` | `mock`, `openai`, `anthropic`, `azure-openai`, `ollama`. |
| `ORLOJ_MODEL_GATEWAY_API_KEY` | `orlojd`, `orlojworker` | Explicit model API key. |
| `OPENAI_API_KEY` | `orlojd`, `orlojworker` | Fallback key for OpenAI. |
| `ANTHROPIC_API_KEY` | `orlojd`, `orlojworker` | Fallback key for Anthropic. |
| `AZURE_OPENAI_API_KEY` | `orlojd`, `orlojworker` | Fallback key for Azure OpenAI. |
| `ORLOJ_EVENT_BUS_BACKEND` | `orlojd` | Server event bus (`memory|nats`). |
| `ORLOJ_NATS_URL` | `orlojd`, `orlojworker` | NATS URL and fallback for runtime message bus URL. |
| `ORLOJ_AGENT_MESSAGE_BUS_BACKEND` | `orlojd`, `orlojworker` | Runtime message bus (`none|memory|nats-jetstream`). |
| `ORLOJ_AUTH_MODE` | `orlojd` | API auth mode (`off|local|sso`). OSS supports `off` and `local`; `sso` requires enterprise adapter. |
| `ORLOJ_AUTH_SESSION_TTL` | `orlojd` | Session TTL for local auth mode (example: `24h`). |
| `ORLOJ_SECRET_ENCRYPTION_KEY` | `orlojd`, `orlojworker` | 256-bit AES key (hex or base64) for encrypting Secret resource data at rest. |
| `ORLOJ_TOOL_ISOLATION_BACKEND` | `orlojd`, `orlojworker` | Tool isolation (`none|container|wasm`). |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `orlojd`, `orlojworker` | OTLP gRPC endpoint for OpenTelemetry trace export. Empty disables export. |
| `OTEL_EXPORTER_OTLP_INSECURE` | `orlojd`, `orlojworker` | Set to `true` for non-TLS OTLP connections (development). |
| `ORLOJ_LOG_FORMAT` | `orlojd`, `orlojworker` | Log output format: `json` (default) or `text`. |
| `ORLOJ_SERVER` | `orlojctl` | Default API base URL when `--server` is omitted (after `ORLOJCTL_SERVER`). |
| `ORLOJCTL_SERVER` | `orlojctl` | Default API base URL when `--server` is omitted (highest precedence among env defaults). |
| `ORLOJCTL_API_TOKEN` | `orlojctl` | Bearer token for API calls (same semantics as `ORLOJ_API_TOKEN` for the client). |

## Server Flags

Print full options:

```bash
go run ./cmd/orlojd -h
```

High-impact groups:

- API/server: `--addr`
- storage: `--storage-backend`, `--postgres-dsn`, pool sizing flags
- execution: `--task-execution-mode`, embedded worker/lease controls
- model gateway: provider, API key, timeout, base URL, default model
- tool runtime: isolation mode, container and wasm controls
- buses: server event bus and runtime message bus flags

## Worker Flags

Print full options:

```bash
go run ./cmd/orlojworker -h
```

High-impact groups:

- identity/capacity: `--worker-id`, `--region`, `--gpu`, `--supported-models`, `--max-concurrent-tasks`
- storage: same postgres flags as server
- execution: `--task-execution-mode`, `--agent-message-consume`, runtime consumer controls
- model/tool runtime: provider and isolation flags

## Secret Resolution

Model endpoints and tools reference secrets via `secretRef` fields. The runtime resolves secrets using a chain of resolvers:

1. **Resource store** -- looks up a `Secret` resource by name.
2. **Environment variables** -- looks up `ORLOJ_SECRET_<name>` (prefix is configurable with `--model-secret-env-prefix` and `--tool-secret-env-prefix`).

### Encryption at Rest

Pass `--secret-encryption-key` (or set `ORLOJ_SECRET_ENCRYPTION_KEY`) on both `orlojd` and `orlojworker` to encrypt `Secret.spec.data` values in the database using AES-256-GCM. The same key must be used by all processes sharing the database. See [Security and Isolation -- Encryption at Rest](./security.md#encryption-at-rest) for key generation and usage.

## Recommended Production Baseline

- `orlojd`: `--storage-backend=postgres`, `--task-execution-mode=message-driven`, `--agent-message-bus-backend=nats-jetstream`
- `orlojworker`: `--storage-backend=postgres`, `--task-execution-mode=message-driven`, `--agent-message-consume`
- Enable `--secret-encryption-key` on all processes if using `Secret` resources
- Set provider keys via `ORLOJ_SECRET_*` environment variables or an external secret manager
- Set `OTEL_EXPORTER_OTLP_ENDPOINT` to your tracing backend (Jaeger, Tempo, etc.) for distributed trace collection
- See [Observability](./observability.md) for the full tracing, metrics, and logging setup

## Verification

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl get workers
go run ./cmd/orlojctl get tasks
```
