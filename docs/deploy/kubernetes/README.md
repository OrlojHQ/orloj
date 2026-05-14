# Kubernetes Deployment Assets

This directory contains Kubernetes deployment assets for Orloj.

## Files

- `orloj-stack.yaml`: baseline raw manifests (namespace, config, secrets, Postgres, NATS, `orlojd`, `orlojworker`). Use this when Helm is not available.

## Helm Chart

The primary deployment path is the Helm chart at `charts/orloj`, published to GHCR as an OCI artifact on every `v*` release:

```
oci://ghcr.io/orlojhq/charts/orloj
```

See [`docs/pages/deploy/kubernetes.md`](../../pages/deploy/kubernetes.md) for install/upgrade, manifest fallback flow, verification, and operations.
