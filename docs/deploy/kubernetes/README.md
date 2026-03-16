# Kubernetes Deployment Assets

This directory contains generic manifests for a baseline Orloj deployment.

## Files

- `orloj-stack.yaml`: namespace, config, secrets, Postgres, NATS, `orlojd`, and `orlojworker`.

Use the operator guide at `docs/pages/deployment/kubernetes.md` for image build/push, apply, verification, and operations.
