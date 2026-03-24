# Deploy and Operate

This section covers deploying Orloj to any environment and running it in production.

## Deployment Targets

Choose a deployment path based on your environment:

| Target | Best For | Persistence | Process Management | Scope |
| ---------- | ----------------------------------------------- | ------------------------------- | -------------------------- | ----------------------------- |
| Local | Development and rapid iteration | Optional (`memory` or Postgres) | terminal or Docker Compose | single operator machine |
| VPS | Single-node production-style self-hosting | Postgres volume | systemd + Docker Compose | small internal workloads |
| Kubernetes | Cluster-based operations and lifecycle controls | PVC-backed Postgres | Kubernetes deployments | platform-managed environments |

**Deployment runbooks:**

1. [Local Deployment](./local.md)
2. [VPS Deployment (Compose + systemd)](./vps.md)
3. [Kubernetes Deployment (Helm + Manifest Fallback)](./kubernetes.md)
4. [Remote CLI and API access](./remote-cli-access.md) -- tokens, `orlojctl` profiles, and `config.json` after you expose the control plane

## Hosted Stack, Local CLI

When the control plane runs in Compose, Kubernetes, or GHCR images, install **`orlojctl` alone** on the machine you use to operate the cluster: download the `orlojctl_*` archive for your OS and arch from [GitHub Releases](https://github.com/OrlojHQ/orloj/releases) (see [Install: CLI only for hosted deployments](../getting-started/install.md#cli-only-for-hosted-deployments)). Then follow [Remote CLI and API access](./remote-cli-access.md) for `--server`, tokens, and optional profiles.

## Day-to-Day Operations

- [Configuration](../operations/configuration.md) -- flags, environment variables, and runtime tuning
- [Runbook](../operations/runbook.md) -- startup, verification, and incident response
- [Security and Isolation](../operations/security.md) -- secrets, auth, and network hardening
- [Upgrades and Rollbacks](../operations/upgrades.md) -- version migration procedures
- [Troubleshooting](../operations/troubleshooting.md) -- common issues and debugging

## Observability

- [Observability](../operations/observability.md) -- OpenTelemetry tracing, Prometheus metrics, structured logging
- [Monitoring and Alerts](../operations/monitoring-alerts.md) -- alerting rules and dashboards
- [Backup and Restore](../operations/backup-restore.md) -- data protection procedures

## Security Defaults

- Rotate default secrets before non-local use.
- Restrict network exposure to required interfaces.
- Keep API auth strategy explicit for each target.
- After deployment, configure [remote CLI access](./remote-cli-access.md) (API tokens, env vars, optional `orlojctl config` profiles).
