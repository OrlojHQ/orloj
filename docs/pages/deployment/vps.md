# VPS Deployment (Compose + systemd)

## Purpose

Run Orloj on a single VPS with Docker Compose managed by systemd for automatic restart and reboot recovery.

## Prerequisites

- Linux VPS with systemd (for example Ubuntu 22.04+)
- Docker Engine with Compose plugin
- `git`, `curl`, and `jq`
- sudo access

## Install

### 1. Place Repository on Host

```bash
sudo mkdir -p /opt/orloj
sudo chown "$USER":"$USER" /opt/orloj
git clone https://github.com/OrlojHQ/orloj.git /opt/orloj
cd /opt/orloj
```

### 2. Configure Runtime Variables

```bash
cp docs/deploy/vps/.env.vps.example docs/deploy/vps/.env.vps
```

Edit `docs/deploy/vps/.env.vps` and rotate at minimum:

- `POSTGRES_PASSWORD`
- `ORLOJ_POSTGRES_DSN` password component
- `ORLOJ_MODEL_GATEWAY_PROVIDER` and key if not using mock

### 3. Validate Compose Config

```bash
docker compose --env-file docs/deploy/vps/.env.vps -f docs/deploy/vps/docker-compose.vps.yml config
```

### 4. Install systemd Unit

```bash
sudo cp docs/deploy/vps/orloj-compose.service /etc/systemd/system/orloj.service
sudo systemctl daemon-reload
sudo systemctl enable --now orloj
```

## Verify

Service status:

```bash
sudo systemctl status orloj --no-pager
```

Stack and health checks:

```bash
docker compose --env-file docs/deploy/vps/.env.vps -f docs/deploy/vps/docker-compose.vps.yml ps
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl get workers
```

Sample task execution:

```bash
go run ./cmd/orlojctl apply -f examples/blueprints/pipeline/
go run ./cmd/orlojctl get task bp-pipeline-task
```

Done means:

- `orloj` systemd unit is active.
- stack survives restart (`sudo systemctl restart orloj`).
- health and worker checks pass.
- sample task reaches `Succeeded`.

## Operate

Restart stack:

```bash
sudo systemctl restart orloj
```

Tail service logs:

```bash
sudo journalctl -u orloj -f
```

Tail compose logs:

```bash
docker compose --env-file docs/deploy/vps/.env.vps -f docs/deploy/vps/docker-compose.vps.yml logs -f
```

Upgrade flow:

1. `git pull` in `/opt/orloj`.
2. `sudo systemctl reload orloj`.
3. rerun verification checks.

## Troubleshoot

- `docker compose ... config` fails: fix missing/invalid `.env.vps` values.
- systemd start fails: verify docker binary path and service logs (`journalctl -u orloj`).
- workers absent: verify `ORLOJ_AGENT_MESSAGE_CONSUME=true` and message-bus settings.

## Security Defaults

- This is a single-node baseline, not HA.
- Bind or firewall `8080` to trusted networks only.
- API auth is disabled by default in `.env.vps.example`; enable deliberately only after planning client auth flow.

## Related Docs

- [Deployment Assets (`docs/deploy/vps`)](../../deploy/vps/README.md)
- [Operations Runbook](../operations/runbook.md)
- [Production Checklist](../getting-started/production-checklist.md)
