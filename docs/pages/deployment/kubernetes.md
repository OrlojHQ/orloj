# Kubernetes Deployment (Generic Manifests)

## Purpose

Deploy Orloj on Kubernetes using plain manifests with no Helm or Kustomize dependency.

## Prerequisites

- Kubernetes cluster access (`kubectl` context configured)
- container registry you can push to
- Docker (or compatible image builder)
- `curl`, `jq`, and `go` for CLI verification from operator workstation

## Install

### 1. Build and Push Images

```bash
export REGISTRY=ghcr.io/<your-org-or-user>
export TAG=v0.1.0

docker build -t "${REGISTRY}/orloj-orlojd:${TAG}" --build-arg BINARY=orlojd .
docker build -t "${REGISTRY}/orloj-orlojworker:${TAG}" --build-arg BINARY=orlojworker .
docker push "${REGISTRY}/orloj-orlojd:${TAG}"
docker push "${REGISTRY}/orloj-orlojworker:${TAG}"
```

### 2. Update Manifest Image References

Edit `docs/deploy/kubernetes/orloj-stack.yaml` and replace:

- `ghcr.io/example/orloj-orlojd:latest`
- `ghcr.io/example/orloj-orlojworker:latest`

with your pushed images.

### 3. Rotate Baseline Secrets

In `docs/deploy/kubernetes/orloj-stack.yaml`, update at minimum:

- `postgres-password`
- `postgres-dsn` password value
- `model-gateway-api-key` (if using real model provider)

### 4. Apply Manifests

```bash
kubectl apply -f docs/deploy/kubernetes/orloj-stack.yaml
```

## Verify

Wait for rollouts:

```bash
kubectl -n orloj rollout status deploy/postgres
kubectl -n orloj rollout status deploy/nats
kubectl -n orloj rollout status deploy/orlojd
kubectl -n orloj rollout status deploy/orlojworker
```

Port-forward API service:

```bash
kubectl -n orloj port-forward svc/orlojd 8080:8080
```

In another terminal:

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl --server http://127.0.0.1:8080 get workers
go run ./cmd/orlojctl --server http://127.0.0.1:8080 apply -f examples/blueprints/pipeline/
go run ./cmd/orlojctl --server http://127.0.0.1:8080 get task bp-pipeline-task
```

Done means:

- all deployments are successfully rolled out.
- API service is reachable through port-forward.
- at least one worker is `Ready`.
- sample task reaches `Succeeded`.

## Operate

Scale workers:

```bash
kubectl -n orloj scale deploy/orlojworker --replicas=3
kubectl -n orloj rollout status deploy/orlojworker
```

Restart control plane:

```bash
kubectl -n orloj rollout restart deploy/orlojd
kubectl -n orloj rollout status deploy/orlojd
```

View logs:

```bash
kubectl -n orloj logs deploy/orlojd --tail=200
kubectl -n orloj logs deploy/orlojworker --tail=200
```

## Troubleshoot

- pods in `ImagePullBackOff`: verify image names/tags and registry access.
- workers not processing: verify `ORLOJ_AGENT_MESSAGE_CONSUME=true` and message-bus env values.
- tasks not created: verify port-forward is active and API endpoint is reachable.

## Security Defaults

- This manifest set is a baseline, not HA.
- Rotate secrets before non-test use.
- Restrict namespace and service exposure based on cluster policy.

## Related Docs

- [Deployment Assets (`docs/deploy/kubernetes`)](../../deploy/kubernetes/README.md)
- [Configuration](../operations/configuration.md)
- [Operations Runbook](../operations/runbook.md)
