# Local Deployment

## Purpose

Run Orloj locally for development and deterministic feature validation.

## Prerequisites

- Go `1.24+`
- Docker
- `curl` and `jq`
- repository checked out locally

## Install

### Option A: Run from Source

Terminal 1:

```bash
go run ./cmd/orlojd \
  --storage-backend=memory \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=memory
```

Terminal 2:

```bash
go run ./cmd/orlojworker \
  --storage-backend=memory \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=memory \
  --agent-message-consume
```

### Option B: Docker Compose Stack

```bash
docker compose up --build -d
```

Uses [`docker-compose.yml`](../../../docker-compose.yml).

## Verify

Health and worker readiness:

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl get workers
```

Sample task execution:

```bash
go run ./cmd/orlojctl apply -f examples/blueprints/pipeline/
go run ./cmd/orlojctl get task bp-pipeline-task
```

Done means:

- `/healthz` returns healthy status.
- at least one worker is `Ready`.
- `bp-pipeline-task` reaches `Succeeded`.

## Operate

Source-mode stop: terminate both processes.

Compose-mode stop:

```bash
docker compose down
```

Compose logs:

```bash
docker compose logs -f orlojd orlojworker-a orlojworker-b
```

## Troubleshoot

- If workers do not appear, check worker process logs for DSN/backend mismatch.
- If tasks remain pending, verify execution mode and message-consumer flags match.
- If `orlojctl` calls fail, confirm `--server` points to the active API address.

## Security Defaults

For local-only use, API auth may remain disabled. Do not expose local ports publicly.

## Related Docs

- [Quickstart](../getting-started/quickstart.md)
- [Configuration](../operations/configuration.md)
- [Troubleshooting](../operations/troubleshooting.md)
