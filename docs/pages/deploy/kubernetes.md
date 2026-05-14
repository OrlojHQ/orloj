# Kubernetes Deployment (Helm + Manifest Fallback)

## Purpose

Deploy Orloj on Kubernetes with a Helm chart (recommended) or with raw manifests (fallback).

## Prerequisites

- Kubernetes cluster access (`kubectl` context configured)
- container registry you can push to
- Docker (or compatible image builder)
- Helm 3 (`helm`)
- `curl`, `jq`, and `go` for CLI verification from operator workstation

## Install

### 1. Build and Push Images

```bash
export REGISTRY=ghcr.io/<your-org-or-user>
export TAG=v0.1.0

docker build -t "${REGISTRY}/orloj-orlojd:${TAG}" --target orlojd \
  --build-arg "VERSION=${TAG}" --build-arg "COMMIT=$(git rev-parse HEAD)" --build-arg "DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)" .
docker build -t "${REGISTRY}/orloj-orlojworker:${TAG}" --target orlojworker \
  --build-arg "VERSION=${TAG}" --build-arg "COMMIT=$(git rev-parse HEAD)" --build-arg "DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)" .
docker push "${REGISTRY}/orloj-orlojd:${TAG}"
docker push "${REGISTRY}/orloj-orlojworker:${TAG}"
```

### 2. Install with Helm (Recommended)

```bash
helm upgrade --install orloj ./charts/orloj \
  --namespace orloj \
  --create-namespace \
  --set orlojd.image.repository="${REGISTRY}/orloj-orlojd" \
  --set orlojd.image.tag="${TAG}" \
  --set orlojworker.image.repository="${REGISTRY}/orloj-orlojworker" \
  --set orlojworker.image.tag="${TAG}" \
  --set postgres.auth.password='<strong-password>' \
  --set runtimeSecret.modelGatewayApiKey='<model-provider-api-key>'
```

To inspect effective values:

```bash
helm get values orloj --namespace orloj
```

### 3. Manifest Fallback (No Helm)

If you cannot use Helm, apply the baseline manifest set:

1. Edit `docs/deploy/kubernetes/orloj-stack.yaml` image references.
2. Rotate baseline secrets (`postgres-password`, DSN password, model API key).
3. Apply manifests:

```bash
kubectl apply -f docs/deploy/kubernetes/orloj-stack.yaml
```

## Verify

Wait for rollouts:

```bash
kubectl -n orloj rollout status deploy/orloj-postgres
kubectl -n orloj rollout status deploy/orloj-nats
kubectl -n orloj rollout status deploy/orloj-orlojd
kubectl -n orloj rollout status deploy/orloj-orlojworker
```

If you used manifest fallback instead of Helm, use:

```bash
kubectl -n orloj rollout status deploy/postgres
kubectl -n orloj rollout status deploy/nats
kubectl -n orloj rollout status deploy/orlojd
kubectl -n orloj rollout status deploy/orlojworker
```

Port-forward API service:

```bash
kubectl -n orloj port-forward svc/orloj-orlojd 8080:8080
```

For manifest fallback, port-forward `svc/orlojd` instead.

In another terminal:

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl --server http://127.0.0.1:8080 get workers
go run ./cmd/orlojctl --server http://127.0.0.1:8080 apply -f examples/blueprints/pipeline/ --run
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
kubectl -n orloj scale deploy/orloj-orlojworker --replicas=3
kubectl -n orloj rollout status deploy/orloj-orlojworker
```

Restart control plane:

```bash
kubectl -n orloj rollout restart deploy/orloj-orlojd
kubectl -n orloj rollout status deploy/orloj-orlojd
```

View logs:

```bash
kubectl -n orloj logs deploy/orloj-orlojd --tail=200
kubectl -n orloj logs deploy/orloj-orlojworker --tail=200
```

Upgrade chart release:

```bash
helm upgrade orloj ./charts/orloj --namespace orloj
```

## Troubleshoot

- pods in `ImagePullBackOff`: verify image names/tags and registry access.
- workers not processing: verify `ORLOJ_AGENT_MESSAGE_CONSUME=true` and message-bus env values.
- tasks not created: verify port-forward is active and API endpoint is reachable.
- Helm rollback: `helm rollback orloj <revision> --namespace orloj`.

## Tool Isolation: Kubernetes Backend

When `toolIsolation.kubernetes.enabled=true`, Orloj runs tool invocations with `isolation_mode: kubernetes` as ephemeral Kubernetes Jobs in the cluster. This eliminates the need for a Docker socket on worker nodes.

### RBAC Requirements

The Helm chart automatically creates a Role (and RoleBinding) for the worker ServiceAccount with the following permissions:

| API Group | Resource | Verbs |
|---|---|---|
| `batch/v1` | `jobs` | `create`, `get`, `list`, `watch`, `delete` |
| `v1` | `pods` | `get`, `list` |
| `v1` | `pods/log` | `get` |
| `v1` | `secrets` | `get` |

The Role is scoped to the namespace configured by `toolIsolation.kubernetes.namespace` (defaults to the release namespace).

### Helm Values

Configure the Kubernetes tool isolation backend under `toolIsolation.kubernetes`:

```yaml
toolIsolation:
  kubernetes:
    enabled: false          # Set to true to enable
    namespace: ""           # Namespace for tool Jobs (default: release namespace)
    serviceAccount: ""      # Service account for tool Pods (default: worker SA)
    jobTTL: 300             # TTL seconds after Job finishes
    defaultImage: "curlimages/curl:8.8.0"  # Fallback image for HTTP tools
```

When enabled, the chart passes `--tool-k8s-enabled=true` and related flags to both `orlojd` (embedded worker) and `orlojworker` deployments.

### Coexistence with Container Backend

Both `container` and `kubernetes` isolation backends can be active simultaneously. Each tool's `spec.runtime.isolation_mode` selects which backend handles that tool:

- `isolation_mode: container` -- runs via `docker run` on the worker host
- `isolation_mode: kubernetes` -- runs as a Kubernetes Job in the cluster

This allows gradual migration from Docker-based isolation to Kubernetes-native execution.

## Security Defaults

- This baseline is not HA.
- Rotate secrets before non-test use.
- `ORLOJ_AUTH_MODE` defaults to `native` in chart runtime config.
- Set and rotate `runtimeSecret.apiToken` for CLI/automation bearer auth.
- Restrict namespace and service exposure based on cluster policy.

## Related Docs

- [Deployment Assets (`docs/deploy/kubernetes`)](../../deploy/kubernetes/README.md)
- [Configuration](../operations/configuration.md)
- [Operations Runbook](../operations/runbook.md)
