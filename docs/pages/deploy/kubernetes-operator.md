# Kubernetes CRD Operator

## Purpose

The CRD sync operator is an **optional** component that makes Orloj resources (Agents, AgentSystems, Tools, etc.) real Kubernetes Custom Resource Definitions. When deployed, you can manage Orloj configuration with `kubectl apply`, store manifests in Git, and let Argo CD or Flux reconcile them — the operator watches CRD objects and syncs them into the Orloj Postgres store automatically.

Without the operator, Orloj works exactly as before: you create resources via `orlojctl apply`, the REST API, or the web console. The operator adds an alternative input path; it does not replace anything.

## When Do You Need It?

| Scenario | Operator needed? |
|---|---|
| Getting started / local dev | No |
| Small team, `orlojctl apply` in CI | No |
| GitOps (Argo CD, Flux, Crossplane) | **Yes** |
| Multi-team platform with RBAC on manifests | **Yes** |
| Unified `kubectl` workflow for all cluster resources | **Yes** |
| You only use the web console | No |

## How It Works

```
┌──────────────┐   watch    ┌───────────────────┐   upsert    ┌──────────────┐
│  K8s CRDs    │──────────► │  orloj-operator   │────────────►│  Postgres    │
│  (etcd)      │◄────────── │  (controller-     │             │  (orlojd     │
│              │  status    │   runtime)        │             │   store)     │
└──────────────┘            └───────────────────┘             └──────────────┘
       ▲                                                            │
       │ kubectl apply                                              │ serves
       │                                                            ▼
  Git repo / CI                                              orlojd API + UI
```

1. You `kubectl apply` an Orloj CRD (e.g. `Agent`).
2. The operator's reconciler converts the CRD spec to an Orloj resource and upserts it into Postgres.
3. The resource is now visible in the REST API, web console, and `orlojctl get`.
4. On delete, the operator's finalizer (`orloj.dev/sync`) removes the resource from the store.
5. A periodic status writer syncs `status.phase`, `observedGeneration`, and `lastSyncedAt` back to the CRD subresource.

Every synced resource gets the annotation `orloj.dev/managed-by: crd-sync`, which `orlojd` uses for conflict detection (see [CRD conflict policy](#crd-conflict-policy)).

## Install

### Enable via Helm

```bash
helm upgrade --install orloj oci://ghcr.io/orlojhq/charts/orloj \
  --namespace orloj \
  --reuse-values \
  --set operator.enabled=true \
  --set operator.installCRDs=true
```

This deploys:

- CRD manifests for all supported resource kinds
- The `orloj-operator` Deployment (connects to the same Postgres as `orlojd`)
- A ServiceAccount with RBAC for CRD watch/list/get/update/patch and status updates
- A PodDisruptionBudget (`minAvailable: 1`)
- Optional ServiceMonitor for Prometheus scraping

### Verify

```bash
# CRDs registered
kubectl get crd agents.orloj.dev

# Operator running
kubectl -n orloj rollout status deploy/orloj-operator

# List Orloj resources (empty initially)
kubectl get agents,agentsystems,tools,mcpservers,modelendpoints,memories,agentpolicies,secrets.orloj.dev
```

## Helm Values Reference

All operator values live under the `operator.*` key:

| Value | Default | Description |
|---|---|---|
| `operator.enabled` | `false` | Deploy the CRD sync operator. |
| `operator.installCRDs` | `true` | Install CRD manifests with the chart. |
| `operator.replicaCount` | `1` | Operator replicas. Leader election ensures only one is active. |
| `operator.image.repository` | `orlojhq/orloj-operator` | Operator container image. |
| `operator.image.tag` | `""` (appVersion) | Image tag override. |
| `operator.resources.requests.cpu` | `100m` | CPU request. |
| `operator.resources.requests.memory` | `128Mi` | Memory request. |
| `operator.resources.limits.cpu` | `500m` | CPU limit. |
| `operator.resources.limits.memory` | `256Mi` | Memory limit. |
| `operator.statusSyncInterval` | `5s` | Interval for syncing CRD status back to Kubernetes. |
| `operator.leaderElection` | `true` | Enable leader election for HA. |
| `operator.healthzPort` | `8081` | Health probe port. |
| `operator.metricsPort` | `8080` | Prometheus metrics port. |
| `operator.serviceMonitor.enabled` | `false` | Create a Prometheus ServiceMonitor. |
| `operator.serviceMonitor.interval` | `30s` | Scrape interval. |
| `operator.serviceMonitor.labels` | `{}` | Extra labels on the ServiceMonitor. |
| `operator.pdb.enabled` | `true` | Create a PodDisruptionBudget. |
| `operator.pdb.minAvailable` | `1` | Minimum available replicas. |

The operator also requires access to the same Postgres database as `orlojd`. The chart wires `--postgres-dsn` and `--secret-encryption-key` from the shared release secret automatically.

## CRD Conflict Policy

When the operator is running, resources it creates are annotated `orloj.dev/managed-by: crd-sync`. If someone then edits that same resource through the REST API, the change will be overwritten on the next operator reconcile. The `--crd-conflict-policy` flag on `orlojd` controls how the API server handles this:

| Mode | Behavior |
|---|---|
| `off` | No conflict detection. REST writes silently proceed. |
| `warn` (default) | REST writes succeed but `orlojd` logs a warning and sets the `X-Orloj-CRD-Managed` response header. |
| `reject` | REST writes to CRD-managed resources return `409 Conflict`. |

Set the policy via Helm:

```yaml
crdConflictPolicy: reject
```

Or via the environment variable `ORLOJ_CRD_CONFLICT_POLICY`.

## Namespace Mapping

By default, the operator maps the Kubernetes namespace of a CRD object directly to the Orloj namespace in the store. A resource in K8s namespace `team-a` is stored in Orloj namespace `team-a`.

For setups where K8s namespaces don't align with Orloj namespaces (e.g., a single `gitops` namespace holds all manifests but they target different Orloj namespaces), use the `orloj.dev/target-namespace` annotation:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: summarizer
  namespace: gitops-manifests           # K8s governance namespace
  annotations:
    orloj.dev/target-namespace: production  # Orloj store namespace
spec:
  model_ref: gpt-4o
  prompt: "You are a concise summarizer."
```

| Annotation set? | Orloj namespace used |
|---|---|
| No | `metadata.namespace` (K8s namespace) |
| Yes | `orloj.dev/target-namespace` value |

This is useful when:

- Your K8s namespace structure is governed by platform policy (e.g., one namespace per ArgoCD Application)
- You have many Orloj namespaces but don't want to create a matching K8s namespace for each
- You want to decouple K8s RBAC boundaries from Orloj resource organization

If you don't set the annotation, behavior is unchanged — K8s namespace = Orloj namespace.

## First Use Walkthrough

### 1. Enable the operator

```bash
helm upgrade --install orloj oci://ghcr.io/orlojhq/charts/orloj \
  --namespace orloj --reuse-values \
  --set operator.enabled=true \
  --set operator.installCRDs=true
```

### 2. Apply an Agent CRD

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: summarizer
  namespace: default
spec:
  prompt: You are a concise summarizer.
  model_ref: gpt-4o
  limits:
    max_steps: 5
```

```bash
kubectl apply -f summarizer-agent.yaml
```

### 3. Verify sync

```bash
# CRD status shows "Synced"
kubectl get agent summarizer -o jsonpath='{.status.phase}'

# Resource visible via orlojctl
orlojctl get agent summarizer
```

### 4. See in the web console

Open the Orloj web console — the agent appears in the Agents list with a "CRD Managed" badge.

## GitOps Setup

### Argo CD

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: orloj-resources
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/your-org/orloj-config
    targetRevision: main
    path: manifests/orloj
  destination:
    server: https://kubernetes.default.svc
    namespace: default
  syncPolicy:
    automated: { prune: true, selfHeal: true }
```

Place Orloj CRD manifests (Agents, Tools, AgentSystems, etc.) under `manifests/orloj/` in your config repo. Argo CD applies them; the operator syncs them into the store.

### Flux

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: orloj-config
  namespace: flux-system
spec:
  url: https://github.com/your-org/orloj-config
  ref:
    branch: main
  interval: 1m
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: orloj-resources
  namespace: flux-system
spec:
  sourceRef:
    kind: GitRepository
    name: orloj-config
  path: ./manifests/orloj
  prune: true
  interval: 5m
```

## Migrating Existing Resources to CRDs

If you already have resources created via `orlojctl apply` or the REST API, you can adopt them as CRDs:

1. Export the resource:

```bash
orlojctl get agent my-agent -o yaml > my-agent.yaml
```

2. Add the CRD `apiVersion` and strip runtime status fields:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: my-agent
  namespace: default
spec:
  # ... your existing spec
```

3. Apply:

```bash
kubectl apply -f my-agent.yaml
```

The operator upserts the resource. Because the name matches, the existing store entry is updated and adopted — it gains the `orloj.dev/managed-by: crd-sync` annotation. From this point, manage it via `kubectl` and Git.

## Secrets and GitOps

For secrets that tools and MCP servers need at runtime, you have two options:

**Option A: Orloj Secret CRD** — store secrets directly in the Orloj store via the `Secret` CRD. Simple but means plaintext values in your manifests (not safe to commit to Git without additional encryption).

**Option B: K8s-native Secrets + `secretRef`** (recommended for GitOps) — use an external secrets operator (Bitnami Sealed Secrets, External Secrets Operator, HashiCorp Vault) to manage K8s-native Secrets, then reference them from Orloj resources:

```yaml
apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: my-api-tool
spec:
  type: http
  endpoint: "https://api.example.com"
  auth:
    secretRef: my-k8s-secret   # K8s-native Secret created by Sealed Secrets
```

The `KubernetesSecretResolver` reads K8s-native Secrets at runtime. This keeps secrets out of Git, leverages K8s RBAC and audit logging, and integrates with existing secret management tooling.

## Phase 1 Scope

The operator currently syncs these 8 resource kinds:

| CRD | API Group | Store Kind |
|---|---|---|
| `agents.orloj.dev` | `orloj.dev/v1` | Agent |
| `agentsystems.orloj.dev` | `orloj.dev/v1` | AgentSystem |
| `tools.orloj.dev` | `orloj.dev/v1` | Tool |
| `mcpservers.orloj.dev` | `orloj.dev/v1` | McpServer |
| `modelendpoints.orloj.dev` | `orloj.dev/v1` | ModelEndpoint |
| `memories.orloj.dev` | `orloj.dev/v1` | Memory |
| `agentpolicies.orloj.dev` | `orloj.dev/v1` | AgentPolicy |
| `secrets.orloj.dev` | `orloj.dev/v1` | Secret |

Runtime resources (Tasks, TaskSchedules, TaskWebhooks, Workers) are not CRDs — they are created through the API at runtime or via `orlojctl`.

## Relationship to Agent K8s Execution

The CRD operator and the Kubernetes agent/tool execution backends are **orthogonal features** that stack:

| Feature | What it does | Helm values |
|---|---|---|
| **CRD Operator** | Syncs resource definitions (Agents, Tools, ...) from K8s CRDs into the Orloj store. Manages the *configuration* plane. | `operator.enabled` |
| **Tool K8s Isolation** | Runs individual tool invocations as ephemeral K8s Jobs. Manages the *execution* plane for tools. | `toolIsolation.kubernetes.enabled` |
| **Agent K8s Execution** | Runs entire agent steps as ephemeral K8s Jobs. Manages the *execution* plane for agents. | `agentExecution.kubernetes.enabled` |

You can use any combination: CRDs without K8s execution, K8s execution without CRDs, or all three together.

## Related Docs

- [Kubernetes Deployment (Helm)](./kubernetes.md)
- [kubectl vs orlojctl](../guides/kubectl-vs-orlojctl.md)
- [Architecture](../concepts/architecture.md)
- [Server Flags — `--crd-conflict-policy`](../reference/server-flags.md)
- [Troubleshooting — Operator](../operations/troubleshooting.md#operator)
