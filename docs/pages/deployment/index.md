# Deployment Overview

This section provides setup runbooks by deployment target.

## Purpose

Choose a deployment path based on your environment and required operational behavior.

## Deployment Targets

| Target     | Best For                                        | Persistence                     | Process Management         | Scope                         |
| ---------- | ----------------------------------------------- | ------------------------------- | -------------------------- | ----------------------------- |
| Local      | Development and rapid iteration                 | Optional (`memory` or Postgres) | terminal or Docker Compose | single operator machine       |
| VPS        | Single-node production-style self-hosting       | Postgres volume                 | systemd + Docker Compose   | small internal workloads      |
| Kubernetes | Cluster-based operations and lifecycle controls | PVC-backed Postgres             | Kubernetes deployments     | platform-managed environments |

## Runbooks

1. [Local Deployment](./local.md)
2. [VPS Deployment (Compose + systemd)](./vps.md)
3. [Kubernetes Deployment (Helm + Manifest Fallback)](./kubernetes.md)
4. [Remote CLI and API access](./remote-cli-access.md) — tokens, `orlojctl` profiles, and `config.json` after you expose the control plane

## Hosted stack, local CLI

When the control plane runs in Compose, Kubernetes, or GHCR images, install **`orlojctl` alone** on the machine you use to operate the cluster (laptop, bastion, or CI): download the `orlojctl_*` archive for your OS and arch from [GitHub Releases](https://github.com/OrlojHQ/orloj/releases) (see [Install: CLI only for hosted deployments](../getting-started/install.md#cli-only-for-hosted-deployments)). Then follow [Remote CLI and API access](./remote-cli-access.md) for `--server`, tokens, and optional profiles.

## Publishing the documentation site

- **[GitHub Pages](./github-pages.md)** — manual workflow for maintainers (custom domain or `github.io`)

## Security Defaults

- Rotate default secrets before non-local use.
- Restrict network exposure to required interfaces.
- Keep API auth strategy explicit for each target.
- After deployment, configure [remote CLI access](./remote-cli-access.md) (API tokens, env vars, optional `orlojctl config` profiles).

## Related Docs

- [Install](../getting-started/install.md)
- [Operations Runbook](../operations/runbook.md)
