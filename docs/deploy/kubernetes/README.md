# Kubernetes Deployment Assets

This directory contains Kubernetes deployment assets for Orloj.

## Files

- `orloj-stack.yaml`: baseline raw manifests (namespace, config, secrets, Postgres, NATS, `orlojd`, `orlojworker`).

## Helm Chart

- Primary Helm chart: `charts/orloj`

Use the operator guide at `docs/pages/deployment/kubernetes.md` for Helm install/upgrade, manifest fallback flow, verification, and operations.
