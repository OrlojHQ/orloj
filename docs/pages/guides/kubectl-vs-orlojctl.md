# kubectl vs orlojctl — Which Tool Do I Use?

When the [CRD operator](../deploy/kubernetes-operator.md) is deployed, Orloj resources live as real Kubernetes CRDs. This means `kubectl` can handle basic CRUD — but `orlojctl` remains the primary tool for operations, runtime control, and admin tasks.

## Resource Lifecycle (CRUD)

| Action | `kubectl` (with operator) | `orlojctl` (always works) |
|---|---|---|
| Create / Update | `kubectl apply -f agent.yaml` | `orlojctl apply -f agent.yaml` |
| List | `kubectl get agents` | `orlojctl get agents` |
| Describe | `kubectl describe agent my-agent` | `orlojctl describe agent my-agent` |
| Delete | `kubectl delete agent my-agent` | `orlojctl delete agent my-agent` |
| Diff | `kubectl diff -f agent.yaml` | `orlojctl diff -f agent.yaml` |
| Edit | `kubectl edit agent my-agent` | `orlojctl edit agent my-agent` |
| Validate | `kubectl apply --dry-run=server -f agent.yaml` | `orlojctl validate -f agent.yaml` |
| Watch | `kubectl get agents -w` | `orlojctl get agents -w` |

Both tools work for CRUD when the operator is running. Choose based on your workflow:

- **GitOps / CI pipeline** → `kubectl apply` (or let Argo CD / Flux do it)
- **Interactive / ad-hoc** → `orlojctl apply` (talks directly to the Orloj API, no operator required)

## Operations (orlojctl only)

These commands interact with the Orloj runtime and have no `kubectl` equivalent:

| Command | Purpose |
|---|---|
| `orlojctl run --system <name>` | Create and execute a task |
| `orlojctl cancel task <name>` | Cancel a running task |
| `orlojctl retry task <name>` | Retry a terminal task |
| `orlojctl approve / deny` | Approve or deny tool/task approvals |
| `orlojctl logs <agent>` | Stream agent execution logs |
| `orlojctl trace task <name>` | Inspect task execution trace |
| `orlojctl graph system <name>` | Render agent system topology |
| `orlojctl events` | Stream control-plane events |
| `orlojctl get tasks -w` | Watch task lifecycle changes |
| `orlojctl messages task/<name>` | Inspect inter-agent messages |
| `orlojctl metrics task/<name>` | View task message metrics |
| `orlojctl memory-entries <name>` | Query memory store entries |
| `orlojctl top workers` | Worker utilization overview |
| `orlojctl top tasks` | Task status overview |
| `orlojctl wait task/<name>` | Block until a task condition is met |

## Admin (orlojctl only)

| Command | Purpose |
|---|---|
| `orlojctl admin create-user` | Create a local user account |
| `orlojctl admin list-users` | List all user accounts |
| `orlojctl admin delete-user` | Remove a user account |
| `orlojctl admin reset-password` | Reset a user's password |
| `orlojctl auth whoami` | Show current identity |
| `orlojctl create token` | Create an API bearer token |
| `orlojctl get tokens` | List API tokens |
| `orlojctl config set-profile` | Configure CLI connection profiles |
| `orlojctl seal secret` | Encrypt secrets for git-safe storage |
| `orlojctl validate -f` | Offline manifest validation (no server) |
| `orlojctl eval run` | Run agent evaluations |
| `orlojctl tool test` | Test WASM tool modules |
| `orlojctl tool scaffold` | Scaffold a new WASM tool project |
| `orlojctl init` | Scaffold a new agent system |

## Bottom Line

- **`kubectl`** handles resource CRUD when the operator is deployed — ideal for GitOps and teams that already use `kubectl` for everything.
- **`orlojctl`** is required for runtime operations (run, cancel, approve, logs, trace) and admin tasks (users, tokens, secrets, eval). It also works for CRUD without the operator.
- You don't have to choose one exclusively. Most teams use `kubectl apply` in CI for configuration and `orlojctl` interactively for operations.

## Related Docs

- [Kubernetes CRD Operator](../deploy/kubernetes-operator.md)
- [CLI Reference](../reference/cli.md)
