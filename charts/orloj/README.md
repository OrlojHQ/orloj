# Orloj Helm Chart

Deploy the full Orloj stack on Kubernetes: API server (`orlojd`), distributed
task workers (`orlojworker`), PostgreSQL, and NATS with JetStream.

## Prerequisites

- Helm 3.10+
- Kubernetes 1.26+

## Quick start

```bash
# Fetch sub-chart dependencies (PostgreSQL + NATS)
helm dependency update charts/orloj/

helm upgrade --install orloj ./charts/orloj \
  --namespace orloj \
  --create-namespace \
  --set auth.mode=native \
  --set auth.setupToken="$(openssl rand -hex 16)" \
  --set secretEncryptionKey="$(openssl rand -hex 32)"
```

The chart uses Bitnami PostgreSQL and NATS sub-charts by default. Both can be
disabled in favour of external services — see **External services** below.

## Uninstall

```bash
helm uninstall orloj --namespace orloj
```

## Key values

| Value | Default | Description |
|---|---|---|
| `image.registry` | `ghcr.io` | Container registry |
| `image.server.repository` | `orlojhq/orlojd` | Server image |
| `image.worker.repository` | `orlojhq/orlojworker` | Worker image |
| `server.replicaCount` | `1` | Server replicas (HA requires leader election) |
| `worker.replicaCount` | `2` | Worker replicas |
| `worker.maxConcurrentTasks` | `1` | Max tasks per worker pod |
| `auth.mode` | `off` | Auth mode: `off` or `native` |
| `auth.apiToken` | `""` | Bearer token (simple auth) |
| `auth.setupToken` | `""` | First-user bootstrap token (`native` mode) |
| `secretEncryptionKey` | `""` | 256-bit AES key for Secret resources at rest |
| `existingSecret` | `""` | Use an existing K8s Secret instead of chart-managed |
| `server.ingress.enabled` | `false` | Enable Ingress for the web console |

See [`values.yaml`](values.yaml) for the full reference.

## External services

### Bring-your-own PostgreSQL

```yaml
postgresql:
  enabled: false
externalPostgres:
  dsn: "postgres://orloj:secret@db.example.com:5432/orloj?sslmode=require"
  # or reference an existing K8s Secret:
  # existingSecret: my-pg-secret
  # existingSecretKey: postgres-dsn
```

### Bring-your-own NATS

```yaml
nats:
  enabled: false
externalNats:
  url: "nats://nats.example.com:4222"
```

## Debug logging

```bash
helm upgrade orloj ./charts/orloj \
  --namespace orloj \
  --reuse-values \
  --set server.logLevel=debug \
  --set worker.logLevel=debug
```

## A2A Protocol

| Value | Default | Description |
|---|---|---|
| `a2a.publicBaseURL` | `""` | Public URL for Agent Card `url` fields |
| `a2a.protocolVersion` | `""` | A2A protocol version to advertise |
| `a2a.cardCacheTTL` | `"5m"` | TTL for cached remote Agent Cards |
| `a2a.allowPrivateEndpoints` | `false` | Allow outbound requests to private IPs |
| `a2a.remoteAgents` | `[]` | Static list of remote A2A agents |
| `a2a.rateLimit.enabled` | `true` | Enable rate limiting for A2A endpoints |
| `a2a.rateLimit.requestsPerMinute` | `30` | Max requests per minute per IP |
| `a2a.rateLimit.maxConcurrentSubscriptions` | `10` | Max concurrent SSE connections |

See [`values.yaml`](values.yaml) for the full A2A configuration reference.

## Production checklist

- Set `auth.mode=native` and provide `auth.setupToken`
- Generate `secretEncryptionKey` with `openssl rand -hex 32`
- Enable Ingress with TLS or use a LoadBalancer service
- Tune `worker.replicaCount` and `worker.maxConcurrentTasks` for your workload
- Consider enabling `worker.autoscaling.enabled` with appropriate HPA thresholds
- Externalize PostgreSQL and NATS for durability and independent scaling
- Enable `server.serviceMonitor.enabled` if running Prometheus Operator
