# Troubleshooting

Use this page for deterministic diagnosis and remediation of common failures.

## First Checks

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl get workers
go run ./cmd/orlojctl get tasks
```

If these checks fail, inspect `orlojd` and `orlojworker` logs first.

## Common Issues

### `postgres backend selected but --postgres-dsn is empty`

Cause:

- `--storage-backend=postgres` is set without DSN.

Fix:

```bash
export ORLOJ_POSTGRES_DSN='postgres://orloj:orloj@127.0.0.1:5432/orloj?sslmode=disable'
```

### Unsupported backend values

Cause:

- invalid value for storage/event/message/tool-isolation backend flags.

Fix:

- storage: `memory|postgres`
- event bus (`orlojd`): `memory|nats`
- runtime message bus: `none|memory|nats-jetstream`
- tool isolation: `none|container|wasm`

### Workers never claim tasks

Checks:

- worker is `Ready` and heartbeating
- execution mode matches deployment mode
- model provider/auth is valid
- task requirements (`region`, `gpu`, `model`) match worker capabilities

Commands:

```bash
go run ./cmd/orlojctl get workers
go run ./cmd/orlojctl get tasks
go run ./cmd/orlojctl trace task <task-name>
```

### Message-driven flow not progressing

Cause:

- worker consumer is not enabled.

Fix:

- set `--agent-message-consume`
- set non-`none` `--agent-message-bus-backend`

### Tool calls fail with permission denials

Cause:

- governance policy denies requested action.

Fix:

- validate `Agent.spec.roles`, `AgentRole`, and `ToolPermission`.
- inspect `tool_code`, `tool_reason`, and `retryable` in trace metadata.

### Model provider auth failures

Cause:

- missing or invalid provider key.

Fix:

- set `--model-gateway-api-key` or provider env var (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `AZURE_OPENAI_API_KEY`).

### Wasm/container runtime errors

Cause:

- missing runtime binary/module path or invalid runtime configuration.

Fix:

- verify backend-specific settings (container runtime settings or wasm module/runtime configuration).

## Escalation Workflow

1. Capture failing command and exact error text.
2. Capture task trace:

```bash
go run ./cmd/orlojctl trace task <task-name>
```

3. Capture recent events:

```bash
go run ./cmd/orlojctl events --once --timeout=30s --raw
```

4. File an issue with logs, trace, and manifest snippets.
