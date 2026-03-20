# Install Orloj

This guide covers how to install Orloj for local evaluation and production-like use: from source (clone and run or build), from **release binaries** (GitHub Releases), or from **container images** (GitHub Container Registry). Use release artifacts when you want a tagged, published build instead of building from source.

## Before You Begin

- **From source:** Go `1.24+`, optionally Bun `1.3+` for docs/frontend
- **Containers:** Docker
- **API checks:** `curl` and `jq`

---

## From source

Clone the repo, then either run in place or build binaries.

```bash
git clone https://github.com/OrlojHQ/orloj.git && cd orloj
```

### Run from source (no build)

Single process with embedded worker:

```bash
go run ./cmd/orlojd \
  --storage-backend=memory \
  --task-execution-mode=sequential \
  --embedded-worker \
  --model-gateway-provider=mock
```

### Build local binaries

```bash
go build -o ./bin/orlojd ./cmd/orlojd
go build -o ./bin/orlojworker ./cmd/orlojworker
go build -o ./bin/orlojctl ./cmd/orlojctl
```

Run the server:

```bash
./bin/orlojd --storage-backend=memory --task-execution-mode=sequential --embedded-worker --model-gateway-provider=mock
```

---

## From release binaries (GitHub Releases)

Download the server, worker, and CLI for your platform from [GitHub Releases](https://github.com/OrlojHQ/orloj/releases). Artifacts are named by binary, git tag, OS, and arch (e.g. `orlojd_v0.1.0_linux_amd64.tar.gz`, `orlojctl_v0.1.0_darwin_arm64.tar.gz`). Verify with `checksums.txt` on the same release. Extract and run:

```bash
# Example: after downloading and extracting orlojd, orlojworker, orlojctl for your OS/arch
./orlojd --storage-backend=memory --task-execution-mode=sequential --embedded-worker --model-gateway-provider=mock
```

Use a specific version tag (e.g. `v0.1.0`) for production; see [Release Process](../project/release-process.md) for versioning and artifact details.

---

## From container images (GHCR)

Published releases are pushed to GitHub Container Registry. Pull and run the server and worker without building from source:

```bash
docker pull ghcr.io/orlojhq/orloj-orlojd:latest
docker pull ghcr.io/orlojhq/orloj-orlojworker:latest
```

Use a version tag instead of `latest` for production (e.g. `ghcr.io/orlojhq/orloj-orlojd:v0.1.0`). You still need Postgres and optionally NATS for persistence and message-driven mode; see [Deployment](../deployment/index.md) for full-stack options. Example, server only with in-memory storage:

```bash
docker run --rm -p 8080:8080 ghcr.io/orlojhq/orloj-orlojd:latest \
  --addr=:8080 \
  --storage-backend=memory \
  --task-execution-mode=sequential \
  --embedded-worker \
  --model-gateway-provider=mock
```

For a full stack (Postgres, NATS, server, workers), use the [VPS](../deployment/vps.md) or [Kubernetes](../deployment/kubernetes.md) deployment guides with `image: ghcr.io/orlojhq/orloj-orlojd:<tag>` (and the worker image) instead of building from the repo.

---

## Docker Compose (from source)

To run the full stack from the repo (Postgres, NATS, `orlojd`, two workers) with a local build:

```bash
git clone https://github.com/OrlojHQ/orloj.git && cd orloj
docker compose up --build
```

This builds the server and worker images from the Dockerfile. To use release images instead, override the service images to `ghcr.io/orlojhq/orloj-orlojd:<tag>` and `ghcr.io/orlojhq/orloj-orlojworker:<tag>` (see [Deployment](../deployment/index.md)).

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
- [Configuration](../operations/configuration.md)
