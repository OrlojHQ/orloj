# CLI Reference

This page documents command-line interfaces for operating Orloj.

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
orlojctl get [-w] <resource>
orlojctl delete <resource> <name>
orlojctl run --system <name> [key=value ...]
orlojctl logs <agent-name>|task/<task-name>
orlojctl trace task <task-name>
orlojctl graph system|task <name>
orlojctl events [filters...]
```

### `orlojctl run`

Imperative task execution. Creates a task targeting the specified AgentSystem, polls until completion, and prints the result.

```bash
orlojctl run --system report-system topic="AI copilots" depth=detailed
```

Flags:

- `--system` (required): AgentSystem to execute.
- `--namespace` (default `default`): namespace for the task.
- `--poll` (default `2s`): status polling interval.
- `--timeout` (default `5m`): maximum wait time.

Positional arguments after flags are parsed as `key=value` input pairs.

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

Common flags:

- `--server` (default `http://127.0.0.1:8080`)
- `--namespace` on namespaced operations
- `-w` for watch mode on supported `get` commands
- `events` filters: `--source`, `--type`, `--kind`, `--name`, `--namespace`, `--since`, `--once`, `--timeout`, `--raw`

## `orlojd`

Print full flags:

```bash
go run ./cmd/orlojd -h
```

Critical flags:

- `--addr`
- `--api-key` (enable bearer token auth; env fallback: `ORLOJ_API_TOKEN`)
- `--storage-backend` (`memory|postgres`)
- `--postgres-dsn`
- `--task-execution-mode` (`sequential|message-driven`)
- `--embedded-worker` (run a built-in worker in the server process)
- `--event-bus-backend` (`memory|nats`)
- `--agent-message-bus-backend` (`none|memory|nats-jetstream`)
- `--model-gateway-provider` (`mock|openai|anthropic|azure-openai|ollama`)
- `--tool-isolation-backend` (`none|container|wasm`)

## `orlojworker`

Print full flags:

```bash
go run ./cmd/orlojworker -h
```

Critical flags:

- `--worker-id`
- `--storage-backend` (`memory|postgres`)
- `--postgres-dsn`
- `--task-execution-mode` (`sequential|message-driven`)
- `--agent-message-consume`
- `--agent-message-bus-backend` (`none|memory|nats-jetstream`)
- `--model-gateway-provider`
- `--tool-isolation-backend`

## `orloj-loadtest`

Print full flags:

```bash
go run ./cmd/orloj-loadtest -h
```

Primary controls:

- `--tasks`
- `--create-concurrency`
- `--poll-concurrency`
- `--quality-profile`
- `--inject-invalid-system-rate`
- `--inject-timeout-system-rate`
- `--inject-expired-lease-rate`

## `orloj-alertcheck`

Print full flags:

```bash
go run ./cmd/orloj-alertcheck -h
```

Primary controls:

- `--profile`
- `--namespace`
- `--task-system`
- `--task-name-prefix`
- `--api-token`

## Command Discovery

Use binary help output as the authoritative source for your current build:

```bash
go run ./cmd/orlojctl help
go run ./cmd/orlojd -h
go run ./cmd/orlojworker -h
```
