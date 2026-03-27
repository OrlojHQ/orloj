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
  --embedded-worker
```

### Build local binaries

```bash
go build -o ./bin/orlojd ./cmd/orlojd
go build -o ./bin/orlojworker ./cmd/orlojworker
go build -o ./bin/orlojctl ./cmd/orlojctl
```

Run the server:

```bash
./bin/orlojd --storage-backend=memory --task-execution-mode=sequential --embedded-worker
```

---

## From release binaries (GitHub Releases)

The install script detects your OS and architecture, downloads the matching binaries, verifies checksums, and installs to `/usr/local/bin` (or `~/.local/bin` if no sudo):

```bash
curl -sSfL https://raw.githubusercontent.com/OrlojHQ/orloj/main/scripts/install.sh | sh
```

Install a specific version or a subset of binaries:

```bash
# Specific version
curl -sSfL https://raw.githubusercontent.com/OrlojHQ/orloj/main/scripts/install.sh | ORLOJ_VERSION=v0.1.1 sh

# CLI only (for remote management of a hosted deployment)
curl -sSfL https://raw.githubusercontent.com/OrlojHQ/orloj/main/scripts/install.sh | ORLOJ_BINARIES="orlojctl" sh
```

Or download manually from [GitHub Releases](https://github.com/OrlojHQ/orloj/releases). Artifacts are named by binary, git tag, OS, and arch (e.g. `orlojd_v0.1.0_linux_amd64.tar.gz`, `orlojctl_v0.1.0_darwin_arm64.tar.gz`). Verify with `checksums.txt` on the same release.

Run the server:

```bash
orlojd --storage-backend=memory --task-execution-mode=sequential --embedded-worker
```

### CLI only for hosted deployments

If `orlojd` and workers run elsewhere—Docker Compose on a VPS, Kubernetes, GHCR images, or a managed host—you **do not** need the full repo on your laptop. Install just the CLI:

```bash
curl -sSfL https://raw.githubusercontent.com/OrlojHQ/orloj/main/scripts/install.sh | ORLOJ_BINARIES="orlojctl" sh
```

Or download only the **`orlojctl_*_<os>_<arch>`** archive for your platform from [GitHub Releases](https://github.com/OrlojHQ/orloj/releases), verify it with `checksums.txt`, extract the binary, and put it on your `PATH`.

Point `orlojctl` at your API with `--server` and authenticate with a bearer token; see [Remote CLI and API access](../deploy/remote-cli-access.md). Prefer a **CLI version that matches your server’s release tag** when possible.

---

## From container images (GHCR)

Published releases are pushed to GitHub Container Registry. Pull and run the server and worker without building from source:

```bash
docker pull ghcr.io/orlojhq/orloj-orlojd:latest
docker pull ghcr.io/orlojhq/orloj-orlojworker:latest
```

Use a version tag instead of `latest` for production (e.g. `ghcr.io/orlojhq/orloj-orlojd:v0.1.0`). You still need Postgres and optionally NATS for persistence and message-driven mode; see [Deployment](../deploy/) for full-stack options. Example, server only with in-memory storage:

```bash
docker run --rm -p 8080:8080 ghcr.io/orlojhq/orloj-orlojd:latest \
  --addr=:8080 \
  --storage-backend=memory \
  --task-execution-mode=sequential \
  --embedded-worker
```

For a full stack (Postgres, NATS, server, workers), use the [VPS](../deploy/vps.md) or [Kubernetes](../deploy/kubernetes.md) deployment guides with `image: ghcr.io/orlojhq/orloj-orlojd:<tag>` (and the worker image) instead of building from the repo.

---

## Docker Compose (from source)

To run the full stack from the repo (Postgres, NATS, `orlojd`, two workers) with a local build:

```bash
git clone https://github.com/OrlojHQ/orloj.git && cd orloj
docker compose up --build
```

This builds the server and worker images from the Dockerfile. To use release images instead, override the service images to `ghcr.io/orlojhq/orloj-orlojd:<tag>` and `ghcr.io/orlojhq/orloj-orlojworker:<tag>` (see [Deployment](../deploy/)).

## Verify Installation

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl get workers
```

Expected result:

- `healthz` returns healthy status.
- At least one worker is `Ready`.

## Next Steps

- [Deployment Overview](../deploy/)
- [Local Deployment](../deploy/local.md)
- [VPS Deployment](../deploy/vps.md)
- [Kubernetes Deployment](../deploy/kubernetes.md)
- [Quickstart](./quickstart.md)
- [Configuration](../operations/configuration.md)
