# Kubernetes Deployment (Helm + Manifest Fallback)

## Purpose

Deploy Orloj on Kubernetes with a Helm chart (recommended) or with raw manifests (fallback).

## Prerequisites

- Kubernetes cluster access (`kubectl` context configured)
- Helm 3 (`helm`)
- `curl`, `jq` for verification (and `go` if running `orlojctl` from source)

The release workflow publishes `orlojd` and `orlojworker` container images plus the Helm chart to GHCR — you do not need to build anything yourself unless you're deploying from a local checkout.

## Install

### 1. Install with Helm (Recommended)

The chart is published as an OCI artifact on every `v*` release:

```bash
helm upgrade --install orloj oci://ghcr.io/orlojhq/charts/orloj \
  --version 0.14.2 \
  --namespace orloj \
  --create-namespace \
  --set postgresql.auth.password='<strong-password>' \
  --set secretEncryptionKey="$(openssl rand -hex 32)" \
  --set auth.mode=native \
  --set auth.setupToken="$(openssl rand -hex 32)"
```

Notes:

- The chart defaults `image.registry`, `image.server.repository`, and `image.worker.repository` at the published GHCR images, so you do not need to set them.
- `secretEncryptionKey` is a 256-bit AES key used to encrypt provider API keys at rest in Postgres. Generate with `openssl rand -hex 32` and store it as you would any other root secret.
- `auth.mode=native` requires `auth.setupToken` for first-user bootstrap. See [Operations &gt; Security](../operations/security.md).
- Model provider API keys (Anthropic, OpenAI, Bedrock, etc.) are **not** chart values — they are encrypted `Secret` resources you create via `orlojctl` after the control plane is up, and `ModelEndpoint` resources reference them by name. See [ModelEndpoint](../concepts/tools/model-endpoint.md).

To inspect effective values:

```bash
helm get values orloj --namespace orloj
```

#### Install from a source checkout

If you've cloned the repo and want to deploy a development build, you can install from the chart directory directly. Subchart deps must be resolved first:

```bash
helm dependency update charts/orloj
helm upgrade --install orloj ./charts/orloj \
  --namespace orloj \
  --create-namespace \
  --set postgresql.auth.password='<strong-password>' \
  --set secretEncryptionKey="$(openssl rand -hex 32)" \
  --set auth.mode=native \
  --set auth.setupToken="$(openssl rand -hex 32)"
```

To pin custom image tags (for example, a locally-built image pushed to your own registry):

```bash
  --set image.registry=ghcr.io/<your-org> \
  --set image.server.repository=<your-org>/orloj-orlojd \
  --set image.server.tag=<your-tag> \
  --set image.worker.repository=<your-org>/orloj-orlojworker \
  --set image.worker.tag=<your-tag>
```

#### GitOps (ArgoCD, Flux)

ArgoCD `Application` example pointing at the OCI chart:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: orloj
  namespace: argocd
spec:
  project: default
  source:
    repoURL: ghcr.io/orlojhq/charts
    chart: orloj
    targetRevision: 0.14.2
    helm:
      valueFiles:
        - values.yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: orloj
  syncPolicy:
    automated: { prune: true, selfHeal: true }
    syncOptions: [ CreateNamespace=true ]
```

### 2. Manifest Fallback (No Helm)

If you cannot use Helm, apply the baseline manifest set:

1. Edit `docs/deploy/kubernetes/orloj-stack.yaml` image references and rotate the baseline secrets (Postgres password, secret encryption key, setup token).
2. Apply manifests:

```bash
kubectl apply -f docs/deploy/kubernetes/orloj-stack.yaml
```

## Verify

Wait for rollouts. The Helm release names follow the `<release>-<component>` convention; with `helm install orloj ...` you get:

```bash
kubectl -n orloj rollout status statefulset/orloj-postgresql
kubectl -n orloj rollout status statefulset/orloj-nats
kubectl -n orloj rollout status deploy/orloj-server
kubectl -n orloj rollout status deploy/orloj-worker
```

If you used the manifest fallback, the names are unprefixed:

```bash
kubectl -n orloj rollout status deploy/postgres
kubectl -n orloj rollout status deploy/nats
kubectl -n orloj rollout status deploy/orlojd
kubectl -n orloj rollout status deploy/orlojworker
```

Port-forward the API service:

```bash
# Helm install
kubectl -n orloj port-forward svc/orloj-server 8080:8080

# Manifest fallback
kubectl -n orloj port-forward svc/orlojd 8080:8080
```

In another terminal:

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
orlojctl --server http://127.0.0.1:8080 get workers
orlojctl --server http://127.0.0.1:8080 apply -f examples/blueprints/pipeline/ --run
orlojctl --server http://127.0.0.1:8080 get task bp-pipeline-task
```

Done means:

- all rollouts are successful.
- API service is reachable through port-forward.
- at least one worker is `Ready`.
- sample task reaches `Succeeded`.

## Operate

Scale workers (Helm install):

```bash
kubectl -n orloj scale deploy/orloj-worker --replicas=3
kubectl -n orloj rollout status deploy/orloj-worker
```

For long-term scaling, prefer the HPA values:

```yaml
worker:
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70
```

Restart control plane:

```bash
kubectl -n orloj rollout restart deploy/orloj-server
kubectl -n orloj rollout status deploy/orloj-server
```

View logs:

```bash
kubectl -n orloj logs deploy/orloj-server --tail=200
kubectl -n orloj logs deploy/orloj-worker --tail=200
```

Upgrade chart release:

```bash
helm upgrade orloj oci://ghcr.io/orlojhq/charts/orloj \
  --version <new-version> --namespace orloj --reuse-values
```

Rollback:

```bash
helm rollback orloj <revision> --namespace orloj
```

## Troubleshoot

- pods in `ImagePullBackOff`: verify image names/tags and registry access.
- workers not processing: verify `ORLOJ_AGENT_MESSAGE_CONSUME=true` and message-bus env values.
- tasks not created: verify the API endpoint is reachable from `orlojctl`.

## Tool Isolation: Kubernetes Backend

When `toolIsolation.kubernetes.enabled=true`, Orloj runs tool invocations with `isolation_mode: kubernetes` as ephemeral Kubernetes Jobs in the cluster. This eliminates the need for a Docker socket on worker nodes.

### RBAC Requirements

The Helm chart automatically creates a Role (and RoleBinding) for the worker ServiceAccount with the following permissions:

| API Group | Resource | Verbs |
|---|---|---|
| `batch` | `jobs` | `create`, `get`, `list`, `watch`, `delete` |
| (core) | `pods` | `get`, `list` |
| (core) | `pods/log` | `get` |
| (core) | `secrets` | `get` |

The Role is scoped to the namespace configured by `toolIsolation.kubernetes.namespace` (defaults to the release namespace).

### Helm Values

Configure the Kubernetes tool isolation backend under `toolIsolation.kubernetes`:

```yaml
toolIsolation:
  kubernetes:
    enabled: false               # Set to true to enable
    namespace: ""                # Namespace for tool Jobs (default: release namespace)
    serviceAccount: ""           # Service account for tool Pods (default: worker SA)
    jobTTLSeconds: 300           # TTL seconds after Job finishes (automatic cleanup)
    defaultImage: "curlimages/curl:8.8.0"  # Fallback image for HTTP tools
```

When enabled, the chart sets `ORLOJ_TOOL_K8S_ENABLED=true` plus related env vars on both the `orlojd` server and `orlojworker` deployments.

### Coexistence with Container Backend

Both `container` and `kubernetes` isolation backends can be active simultaneously. Each tool's `spec.runtime.isolation_mode` selects which backend handles that tool:

- `isolation_mode: container` — runs via `docker run` on the worker host
- `isolation_mode: kubernetes` — runs as a Kubernetes Job in the cluster

This allows gradual migration from Docker-based isolation to Kubernetes-native execution.

## Agent Execution: Kubernetes Backend

When `agentExecution.kubernetes.enabled=true`, Orloj runs each agent in a multi-agent task as an ephemeral Kubernetes Job instead of executing it in-process on the worker. This isolates agent execution at the pod level and allows independent scaling of agent workloads.

Agents whose tools require Docker (container isolation mode or stdio MCP servers with a container image) automatically fall back to in-process execution.

### RBAC Requirements

The Helm chart creates a Role (and RoleBindings for both the worker and server ServiceAccounts) with the following permissions:

| API Group | Resource | Verbs |
|---|---|---|
| `batch` | `jobs` | `create`, `get`, `list`, `watch`, `delete` |
| (core) | `pods` | `get`, `list` |
| (core) | `pods/log` | `get` |

The Role is scoped to the namespace configured by `agentExecution.kubernetes.namespace` (defaults to the release namespace).

### Helm Values

```yaml
agentExecution:
  kubernetes:
    enabled: false               # Set to true to enable
    namespace: ""                # Namespace for agent Jobs (default: release namespace)
    serviceAccount: ""           # Service account for agent Pods
    image: ""                    # Container image (default: worker image)
    jobTTLSeconds: 600           # TTL seconds after Job finishes
    defaultMemory: "512Mi"       # Default memory limit for agent Pods
    defaultCPU: "500m"           # Default CPU limit for agent Pods
```

### How It Works

1. The orchestrator (worker or server) checks whether the agent can run as a K8s Job (`CanRunAsJob`).
2. If eligible, it writes agent input to the task's status in Postgres and creates a K8s Job running the worker image with `--single-agent` mode.
3. The agent pod reads its input from Postgres, executes the agent, and writes the result back.
4. The orchestrator watches the Job for completion and reads the result.
5. If the orchestrator crashes and restarts, it detects the existing Job by its deterministic name and resumes watching.

### Crash Recovery

Agent Jobs use deterministic names based on the task, agent, and attempt number. If the orchestrator pod restarts mid-execution, it detects the existing Job and either reads its result (if complete) or resumes watching (if still running).

## A2A Protocol

To configure public A2A Agent Card URLs in a Helm deployment, set the A2A public base URL. Individual AgentSystems are exposed with `spec.a2a.enabled: true`.

```bash
helm upgrade orloj ./charts/orloj --namespace orloj --reuse-values \
  --set a2a.publicBaseURL=https://orloj.example.com
```

See the [Chart README](../../../charts/orloj/README.md#a2a-protocol) for the full list of `a2a.*` values and their defaults.

## CRD Sync Operator (Optional)

The Orloj CRD operator makes Orloj resources (Agents, Tools, AgentSystems, etc.) real Kubernetes Custom Resource Definitions. When enabled, you can manage configuration with `kubectl apply` and integrate with GitOps tools like Argo CD and Flux.

```bash
helm upgrade --install orloj oci://ghcr.io/orlojhq/charts/orloj \
  --namespace orloj --reuse-values \
  --set operator.enabled=true \
  --set operator.installCRDs=true
```

The operator is independent of tool isolation and agent execution backends — it manages the *configuration* plane (resource definitions), not the *execution* plane (how tools and agents run). You can use any combination.

See [Kubernetes CRD Operator](./kubernetes-operator.md) for full documentation, values reference, GitOps examples, and migration guide.

## Security Defaults

- This baseline is not HA — `server.replicaCount` defaults to 1. Multi-replica `orlojd` requires leader election (see roadmap).
- Rotate secrets before non-test use:
  - `postgresql.auth.password` (or `postgresql.auth.existingSecret` for a pre-sealed value).
  - `secretEncryptionKey` — losing this makes every encrypted Orloj `Secret` unrecoverable.
  - `auth.setupToken` — single-use bootstrap; rotate after the first admin account is created.
  - `auth.apiToken` — set this only if you also need a static bearer for CLI/automation; otherwise rely on user-issued tokens minted through the native auth flow.
- `ORLOJ_AUTH_MODE` defaults to `native` (the chart's `auth.mode` value). `auth.mode=off` disables authentication entirely and is intended only for local development.
- Restrict namespace and service exposure based on cluster policy. The chart's `server.ingress` is opt-in and emits a `networking.k8s.io/v1 Ingress`; for Gateway API environments, leave `server.ingress.enabled=false` and ship an `HTTPRoute` alongside the release.

## Related Docs

- [Deployment Assets (`docs/deploy/kubernetes`)](../../deploy/kubernetes/README.md)
- [Configuration](../operations/configuration.md)
- [Operations Runbook](../operations/runbook.md)
- [ModelEndpoint](../concepts/tools/model-endpoint.md)
- [Security](../operations/security.md)
