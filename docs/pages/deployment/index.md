# Deployment Overview

This section provides setup runbooks by deployment target.

## Purpose

Choose a deployment path based on your environment and required operational behavior.

## Deployment Targets

| Target | Best For | Persistence | Process Management | Scope |
|---|---|---|---|---|
| Local | Development and rapid iteration | Optional (`memory` or Postgres) | terminal or Docker Compose | single operator machine |
| VPS | Single-node production-style self-hosting | Postgres volume | systemd + Docker Compose | small internal workloads |
| Kubernetes | Cluster-based operations and lifecycle controls | PVC-backed Postgres | Kubernetes deployments | platform-managed environments |

## What This Is and Is Not

- This setup track is the Gate-0 launch-blocker deployment baseline.
- It is not multi-control-plane HA. HA remains deferred to post-Gate-0 reliability hardening.

## Runbooks

1. [Local Deployment](./local.md)
2. [VPS Deployment (Compose + systemd)](./vps.md)
3. [Kubernetes Deployment (Helm + Manifest Fallback)](./kubernetes.md)

## Security Defaults

- Rotate default secrets before non-local use.
- Restrict network exposure to required interfaces.
- Keep API auth strategy explicit for each target.

## Related Docs

- [Install](../getting-started/install.md)
- [Operations Runbook](../operations/runbook.md)
- [Production Checklist](../getting-started/production-checklist.md)
