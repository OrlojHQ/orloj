# Configuration

This page defines runtime configuration for `orlojd` and `orlojworker`.

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
| `ORLOJ_EVENT_BUS_BACKEND` | `orlojd` | Control-plane event bus (`memory|nats`). |
| `ORLOJ_NATS_URL` | `orlojd`, `orlojworker` | NATS URL and fallback for runtime message bus URL. |
| `ORLOJ_AGENT_MESSAGE_BUS_BACKEND` | `orlojd`, `orlojworker` | Runtime message bus (`none|memory|nats-jetstream`). |
| `ORLOJ_TOOL_ISOLATION_BACKEND` | `orlojd`, `orlojworker` | Tool isolation (`none|container|wasm`). |

## Control Plane Flags

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
- buses: control-plane event bus and runtime message bus flags

## Worker Flags

Print full options:

```bash
go run ./cmd/orlojworker -h
```

High-impact groups:

- identity/capacity: `--worker-id`, `--region`, `--gpu`, `--supported-models`, `--max-concurrent-tasks`
- storage: same postgres flags as control plane
- execution: `--task-execution-mode`, `--agent-message-consume`, runtime consumer controls
- model/tool runtime: provider and isolation flags

## Recommended Production Baseline

- `orlojd`: `--storage-backend=postgres`, `--task-execution-mode=message-driven`, `--agent-message-bus-backend=nats-jetstream`
- `orlojworker`: `--storage-backend=postgres`, `--task-execution-mode=message-driven`, `--agent-message-consume`
- set provider keys via env or secret-backed configuration

## Verification

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl get workers
go run ./cmd/orlojctl get tasks
```
