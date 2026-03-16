# Install Orloj

This guide covers supported installation patterns for local evaluation and production-like testing.

## Before You Begin

- Go `1.24+`
- Bun `1.3+` (docs/frontend workflows)
- Docker (recommended for containerized dependencies)
- `curl` and `jq` for API checks

## Option 1: Run from Source

Start the control plane:

```bash
go run ./cmd/orlojd \
  --storage-backend=memory \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=memory
```

Start a worker in a second terminal:

```bash
go run ./cmd/orlojworker \
  --storage-backend=memory \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=memory \
  --agent-message-consume \
  --model-gateway-provider=mock
```

## Option 2: Build Local Binaries

```bash
go build -o ./bin/orlojd ./cmd/orlojd
go build -o ./bin/orlojworker ./cmd/orlojworker
go build -o ./bin/orlojctl ./cmd/orlojctl
```

Run binaries:

```bash
./bin/orlojd --storage-backend=memory
./bin/orlojworker --storage-backend=memory
```

## Option 3: Docker Compose

`docker-compose.yml` includes:

- Postgres
- NATS (JetStream enabled)
- `orlojd`
- two worker instances

Start the stack:

```bash
docker compose up --build
```

## Verify Installation

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl get workers
```

Expected result:

- `healthz` returns healthy status.
- At least one worker is `Ready`.

## Next Steps

- [Deployment Overview](../deployment/index.md)
- [Local Deployment](../deployment/local.md)
- [VPS Deployment](../deployment/vps.md)
- [Kubernetes Deployment](../deployment/kubernetes.md)
- [Quickstart](./quickstart.md)
- [Production Checklist](./production-checklist.md)
- [Configuration](../operations/configuration.md)
