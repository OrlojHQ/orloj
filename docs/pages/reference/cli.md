# CLI Reference

`orlojctl` is the command-line interface for managing Orloj resources, running tasks, and inspecting system state.

For server and worker daemon flags, see [Server Flags](./server-flags.md). For load-test and alert-check tools, see [Internal Tools](./internal-tools.md).

> **Note:** When the [CRD operator](../deploy/kubernetes-operator.md) is deployed, `kubectl` can replace `orlojctl` for basic resource CRUD (`apply`, `get`, `delete`, `edit`, `diff`). Runtime operations (`run`, `cancel`, `approve`, `logs`, `trace`, `events`, etc.) and admin commands (`admin`, `auth`, `config`, `seal`, `eval`) remain `orlojctl`-only. See [kubectl vs orlojctl](../guides/kubectl-vs-orlojctl.md) for a detailed comparison.

## Usage patterns

```text
orlojctl apply -f <file-or-directory> [--run] [--dry-run] [--namespace <ns>]
orlojctl validate -f <file|dir>
orlojctl create secret <name> --from-literal key=value [...]
orlojctl create token <name> --role <role>
orlojctl seal public-key
orlojctl seal secret -f <secret-manifest> [--out <file>] [--stdout]
orlojctl seal secret <name> --from-literal key=value [...] [--out <file>] [--stdout]
orlojctl approve tool-approval|task-approval <name> [--decided-by <id>] [--comment <text>]
orlojctl deny tool-approval|task-approval <name> [--decided-by <id>] [--comment <text>]
orlojctl request-changes task-approval <name> --decided-by <id> --comment <text>
orlojctl get [-w] <resource> [name] [-o table|json|yaml]
orlojctl get tokens
orlojctl get memory-entries <memory-name> [--query <q>] [--prefix <p>] [--limit <n>]
orlojctl memory-entries <memory-name> [--query <q>] [--prefix <p>] [--limit <n>]
orlojctl delete <resource> <name>
orlojctl delete token <name>
orlojctl describe <resource> <name>
orlojctl edit <resource> <name>
orlojctl diff -f <file-or-directory> [--namespace <ns>]
orlojctl wait <resource>/<name> --for condition=<value> [--timeout <duration>]
orlojctl cancel task <name> [--reason <text>]
orlojctl retry task <name> [--with-overrides key=value ...]
orlojctl top workers|tasks
orlojctl run --system <name> [key=value ...]
orlojctl init <name> [--blueprint pipeline|hierarchical|swarm-loop]
orlojctl logs <agent-name>|task/<task-name>
orlojctl trace task <task-name>
orlojctl graph system|task <name>
orlojctl events [filters...]
orlojctl messages task/<task-name> [--agent <name>] [-o table|json|yaml]
orlojctl metrics task/<task-name> [-o table|json|yaml]
orlojctl health [-o table|json|yaml]
orlojctl status [-o table|json|yaml]
orlojctl completion bash|zsh|fish
orlojctl auth whoami [--server URL]
orlojctl auth login [-u <username>] [-p <password>] [--server URL]
orlojctl admin create-user <username> --role <role>
orlojctl admin list-users
orlojctl admin delete-user <username>
orlojctl admin reset-password --username <name> --new-password <value>
orlojctl config path|get|use <name>|set-profile <name> [--server URL] [--token value] [--token-env NAME]
orlojctl eval run --dataset <name> --system <name> [--scoring <strategy>] [--concurrency <n>]
orlojctl eval list [-o table|json|yaml]
orlojctl eval get <name> [-o table|json|yaml]
orlojctl eval compare <run1> <run2> [run3 ...]
orlojctl eval datasets [-o table|json|yaml]
orlojctl eval export <name> [--format csv|json]
orlojctl eval annotate <name> --sample <sample> --score <n> [--pass] [--comment <text>]
orlojctl eval import <name> -f <csv-file>
orlojctl eval finalize <name>
```

## Global Auth and Server Resolution

- Global auth flag: `--api-token <token>`
- Global namespace flag: `--namespace <ns>` or `-n <ns>` (sets the request namespace for API calls; must match `metadata.namespace` in manifest bodies on create/update)
- Version command: `orlojctl version` (also `-version`, `--version`)
- Token precedence:
  1. `--api-token`
  2. `ORLOJCTL_API_TOKEN`
  3. `ORLOJ_API_TOKEN`
  4. Active profile `token`, then `token_env`
- Default server precedence when `--server` is omitted:
  1. `ORLOJCTL_SERVER`
  2. `ORLOJ_SERVER`
  3. Active profile `server`
  4. `http://127.0.0.1:8080`

## `orlojctl apply`

| Flag | Default | Description |
|---|---|---|
| `-f` | none | Path to a manifest file or directory (required). |
| `--run` | `false` | Include runnable `Task` manifests when `-f` points to a directory. |
| `--dry-run` | `false` | Preview create/update/no-op actions without persisting. |
| `--namespace` | global namespace (if set) | Request namespace for applied manifests. Must match `metadata.namespace` in each manifest body. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |
| `--server` | resolved server | API server URL. |

When applying to a non-default namespace, pass `--namespace <ns>` and ensure each manifest's `metadata.namespace` matches (or omit it and let the server apply the request namespace). A body/query mismatch returns `400 Bad Request` from the API.

- **File:** applies that manifest.
- **Directory:** walks recursively (skips `.git` dirs) and evaluates every `.yaml`, `.yml`, and `.json` file in sorted path order.
  - By default, runnable `Task` manifests (`spec.mode: run` or omitted mode) are skipped for safety.
  - `Task` manifests with `spec.mode: template` are always applied.
  - Pass `--run` to include runnable tasks during directory apply.
  - Failures are collected; the command exits with an error if any file failed.

Behavior matrix:

| Command | Runnable Task (`spec.mode: run` or omitted mode) | Template Task (`spec.mode: template`) | Other Kinds |
|---|---|---|---|
| `orlojctl apply -f task.yaml` | Applied | Applied | Applied |
| `orlojctl apply -f <dir>` | Skipped | Applied | Applied |
| `orlojctl apply -f <dir> --run` | Applied | Applied | Applied |

## `orlojctl validate`

Parse and normalize manifests **offline** (no API server, no `orlojctl` config file required). Use in CI or before `apply` to catch schema and normalization errors early.

| Flag | Default | Description |
|---|---|---|
| `-f` | none | Path to a manifest file or a directory (required). |

- **File:** validates that one manifest.
- **Directory:** walks recursively (skips `.git` dirs) and validates every `.yaml`, `.yml`, and `.json` file.
- **Exit code:** `0` if every file is valid; `1` if any file fails. Failed files are listed with path and error on stdout.

Examples:

```bash
orlojctl validate -f agent.yaml
orlojctl validate -f ./manifests/
```

## `orlojctl create secret`

| Flag | Default | Description |
|---|---|---|
| `--from-literal` | none | Repeatable `key=value` pair; at least one required. |
| `--namespace` | `default` | Secret namespace. |
| `-n` | `default` | Shorthand for `--namespace`. |
| `--server` | resolved server | API server URL. |

## `orlojctl create token`

| Flag | Default | Description |
|---|---|---|
| `--role` | none | Token role (`admin`, `writer`, `reader`, `controller`). Required. |
| `--server` | resolved server | API server URL. |

## `orlojctl seal`

Git-safe secret workflow commands:

- `orlojctl seal public-key` -- fetch the active control-plane sealing public key from `GET /v1/sealing-key/public`
- `orlojctl seal secret -f <secret-manifest>` -- read a normal `Secret` manifest, fetch the active public key, and write `<name>.sealed.yaml` by default
- `orlojctl seal secret <name> --from-literal key=value [...]` -- build a transient `Secret` locally, seal it, and write `<name>.sealed.yaml` without creating an intermediate plaintext manifest

`seal secret` does not talk to workers. The generated `SealedSecret` is later applied through the normal resource API.

By default, `seal secret` writes YAML to a file:

- with `-f secret.yaml`, the default output is `secret.sealed.yaml` next to the source file
- with inline `--from-literal`, the default output is `<name>.sealed.yaml` in the current directory

Useful flags:

| Flag | Default | Description |
|---|---|---|
| `-f` | none | Path to an existing `Secret` manifest. |
| `--from-literal` | none | Repeatable `key=value` pair used to build a transient `Secret` locally. |
| `-o` / `--out` | auto-generated path | Explicit output path for the generated `SealedSecret` manifest. |
| `--stdout` | `false` | Print the generated manifest to stdout instead of writing a file. |
| `--format` | `yaml` | Output format: `yaml` or `json`. |
| `--namespace` / `-n` | global namespace or `default` | Namespace override for sealed secrets generated from literals or manifests. |

Examples:

```bash
# Seal an existing Secret manifest into secret.sealed.yaml
orlojctl seal secret -f secret.yaml

# Seal literals directly into payment-gateway.sealed.yaml
orlojctl seal secret payment-gateway \
  --from-literal api_key=sk-prod-123 \
  --from-literal org=acme

# Keep stdout for scripting
orlojctl seal secret -f secret.yaml --stdout
```

## `orlojctl approve` / `orlojctl deny`

Approves or denies a pending `ToolApproval` or `TaskApproval`:

- `orlojctl approve tool-approval <name> ...`
- `orlojctl deny tool-approval <name> ...`
- `orlojctl approve task-approval <name> ...`
- `orlojctl deny task-approval <name> ...`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--namespace` | global namespace (if set) | Optional namespace override. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |
| `--decided-by` | empty | Decision actor identity. |
| `--comment` | empty | Optional reviewer comment. |
| `--reason` | empty | Legacy alias for `--comment`. |

## `orlojctl request-changes`

Requests changes on a pending `TaskApproval` and reruns the producing agent with injected `review.*` context:

- `orlojctl request-changes task-approval <name> --decided-by reviewer@example.com --comment "Revise the disclaimer"`

The command fails if the checkpoint disables `request_changes` or if the approval has already reached `max_review_cycles`.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--namespace` | global namespace (if set) | Optional namespace override. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |
| `--decided-by` | empty | Decision actor identity. |
| `--comment` | empty | Required reviewer feedback unless you use the legacy `--reason` alias. |
| `--reason` | empty | Legacy alias for `--comment`. |

## `orlojctl get`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `-w` | `false` | Watch mode (currently only supported for `tasks`). |
| `-o` | `table` | Output format: `table`, `json`, `yaml`. |
| `--namespace` | global namespace (if set) | Optional namespace override/filter. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

Supported resources:

- `agents`
- `agent-systems`
- `model-endpoints`
- `tools`
- `secrets`
- `sealed-secrets`
- `memories`
- `agent-policies`
- `agent-roles`
- `tool-permissions`
- `tool-approvals`
- `task-approvals`
- `tasks`
- `task-schedules`
- `task-webhooks`
- `workers`
- `mcp-servers`
- `eval-datasets`
- `eval-runs`
- `tokens`

Notes:

- `orlojctl get <resource> [name]` supports both list and single-resource fetch.
- `orlojctl get memory-entries <memory-name> ...` delegates to memory entry inspection.

Examples (MCP servers):

```bash
# Apply an MCP server manifest
orlojctl apply -f mcp-server.yaml

# List all MCP servers
orlojctl get mcp-servers

# Get a specific MCP server
orlojctl get mcp-server my-server

# Delete an MCP server
orlojctl delete mcp-server my-server
```

See the [Connect an MCP Server](../guides/connect-mcp-server.md) guide for full setup instructions.

## `orlojctl delete`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--namespace` | global namespace (if set) | Optional namespace override for namespaced resources. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

## `orlojctl run`

| Flag | Default | Description |
|---|---|---|
| `--system` | none | Target `AgentSystem` (required). |
| `--server` | resolved server | API server URL. |
| `--namespace` | global namespace (if set), else `default` | Task namespace. |
| `-n` | global namespace (if set), else `default` | Shorthand for `--namespace`. |
| `--poll` | `2s` | Poll interval while waiting for task completion. |
| `--timeout` | `5m` | Max wait time for task completion. |

Positional args after flags are parsed as `key=value` task input.

## `orlojctl events`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--since` | `0` | Resume stream from event id. |
| `--source` | empty | Filter by event source. |
| `--type` | empty | Filter by event type. |
| `--kind` | empty | Filter by resource kind. |
| `--name` | empty | Filter by resource name. |
| `--namespace` | global namespace (if set) | Filter by resource namespace. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |
| `--once` | `false` | Exit after first matching event. |
| `--timeout` | `0` | Max stream time (`0` means no timeout). |
| `--raw` | `false` | Print raw event JSON payload. |

## `orlojctl memory-entries`

Inspect stored entries for a `Memory` resource.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--query` | empty | Semantic query (`q` parameter). |
| `--prefix` | empty | Key prefix filter (`prefix` parameter). |
| `--limit` | `100` | Max entries returned. |
| `-o` | `table` | Output format: `table`, `json`, `yaml`. |
| `--namespace` | global namespace (if set) | Optional namespace override. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

## `orlojctl describe`

Fetches a single resource and prints a human-readable summary plus YAML payload.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--namespace` | global namespace (if set) | Optional namespace override. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |
| `-o` | `table` | Output format: `table`, `json`, `yaml`. |

## `orlojctl edit`

Fetches a resource, opens it in `$VISUAL`/`$EDITOR` (`vi` fallback), and applies the edited manifest on save.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--namespace` | global namespace (if set) | Optional namespace override. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

## `orlojctl diff`

Shows a unified diff between live state and the provided manifest(s), using normalized resource payloads (runtime status fields excluded).

| Flag | Default | Description |
|---|---|---|
| `-f` | none | Path to manifest file or directory (required). |
| `--run` | `false` | Include runnable tasks when diffing directories. |
| `--namespace` | global namespace (if set) | Request namespace for diffed manifests. Must match `metadata.namespace` in each manifest body. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |
| `--server` | resolved server | API server URL. |

## `orlojctl wait`

Polls a resource until a condition is met or timeout is reached.

| Flag | Default | Description |
|---|---|---|
| `--for` | `condition=Complete` | Wait condition expression (`condition=<phase-or-alias>`). |
| `--timeout` | `5m` | Maximum wait time. |
| `--interval` | `2s` | Poll interval. |
| `--server` | resolved server | API server URL. |
| `--namespace` | global namespace (if set) | Optional namespace override. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

Exit behavior:

- Success when condition is satisfied.
- Timeout exits with code `1`.
- Invalid usage/request errors exit with code `2`.

## `orlojctl cancel task`

Marks a non-terminal task as `Failed` through task status update.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--reason` | `task canceled via orlojctl` | Failure reason recorded on task status. |
| `--namespace` | global namespace (if set) | Optional namespace override. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

## `orlojctl retry task`

Creates a new task from an existing terminal task spec.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--with-overrides` | none | Repeatable `key=value` input overrides. |
| `--namespace` | global namespace (if set) | Optional namespace override. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

## `orlojctl top`

Quick operational summaries for task and worker state.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `-o` | `table` | Output format: `table`, `json`, `yaml`. |
| `--namespace` | global namespace (if set) | Optional namespace override/filter. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

Targets:

- `orlojctl top workers`
- `orlojctl top tasks`

## `orlojctl messages`

Inspect inter-agent task messages.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--agent` | empty | Filter where `from_agent` or `to_agent` matches. |
| `--phase` | empty | Lifecycle phase filter. |
| `--limit` | `0` | Max messages returned (`0` = no limit). |
| `-o` | `table` | Output format: `table`, `json`, `yaml`. |
| `--namespace` | global namespace (if set) | Optional namespace override. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

Target forms:

- `orlojctl messages task/<task-name>`
- `orlojctl messages task <task-name>`

## `orlojctl metrics`

Inspect task message observability metrics.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--phase` | empty | Lifecycle phase filter. |
| `--limit` | `0` | Max message samples used (`0` = no limit). |
| `-o` | `table` | Output format: `table`, `json`, `yaml`. |
| `--namespace` | global namespace (if set) | Optional namespace override. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

Target forms:

- `orlojctl metrics task/<task-name>`
- `orlojctl metrics task <task-name>`

## `orlojctl health`

Checks `/healthz`.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `-o` | `table` | Output format: `table`, `json`, `yaml`. |

## `orlojctl status`

Composite status view using `/healthz`, `/v1/auth/config`, `/v1/capabilities`, `/v1/workers`, and `/v1/namespaces`.

Table output includes `auth_mode`, `setup_required`, and `setup_token_required` (from auth config; the last is true when `ORLOJ_SETUP_TOKEN` is set on the server). JSON/YAML snapshots include `auth_setup_token_required`.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `-o` | `table` | Output format: `table`, `json`, `yaml`. |

## `orlojctl completion`

Emits shell completion scripts.

Usage:

- `orlojctl completion bash`
- `orlojctl completion zsh`
- `orlojctl completion fish`

## `orlojctl logs`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |

## `orlojctl trace`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |

## `orlojctl graph`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |

## `orlojctl auth whoami`

Returns the currently authenticated identity from `/v1/auth/me`.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |

## `orlojctl auth login`

Authenticate against a native-mode server and save a bearer token to the active profile. Prompts interactively for username and password if not provided via flags.

| Flag | Default | Description |
|---|---|---|
| `-u`, `--username` | (prompted) | Username to authenticate with. |
| `-p`, `--password` | (prompted) | Password (prefer interactive prompt over flags for security). |
| `--server` | resolved server | API server URL. |

On success the minted token is written into the active profile's `token` field in `config.json`. Subsequent commands automatically use it.

## `orlojctl admin create-user`

| Flag | Default | Description |
|---|---|---|
| `--role` | `reader` | User role (`admin`, `writer`, `reader`, `controller`). |
| `--server` | resolved server | API server URL. |

## `orlojctl admin list-users`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |

## `orlojctl admin delete-user`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |

## `orlojctl admin reset-password`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--username` | none | Target username (required). |
| `--new-password` | none | New password (required). |

## `orlojctl config set-profile`

| Flag | Default | Description |
|---|---|---|
| `--server` | empty | Profile API server URL. |
| `--token` | empty | Profile bearer token (prefer `--token-env` for secrets). |
| `--token-env` | empty | Env var name read at runtime for token value. |

Other config subcommands:

- `orlojctl config path`: print config file path
- `orlojctl config get`: print current config/profile data with resolution sources (shows whether server/token come from env, profile, or defaults)
- `orlojctl config use <name>`: switch active profile (probes auth status on the target server)

## `orlojctl init`

Positional argument `<name>` is required. It sets both the output directory and the resource name prefix.

| Flag | Default | Description |
|---|---|---|
| `--blueprint` | `pipeline` | Blueprint topology: `pipeline`, `hierarchical`, `swarm-loop`. |

## `orlojctl eval`

Evaluation framework commands for measuring and comparing agent system quality. See the [Agent Evaluation guide](../guides/run-agent-evaluation.md) for a full walkthrough.

### `orlojctl eval run`

Creates an EvalRun and polls until completion.

| Flag | Default | Description |
|---|---|---|
| `--dataset` | none | EvalDataset name (required). |
| `--system` | none | AgentSystem name (required). |
| `--scoring` | `exact_match` | Scoring strategy: `exact_match`, `llm_judge`, `manual`, `custom`. |
| `--model-ref` | empty | Judge model for `llm_judge` scoring. |
| `--rubric` | empty | Evaluation rubric for `llm_judge` scoring. |
| `--concurrency` | `1` | Maximum parallel samples. |
| `--timeout` | empty | Per-sample task timeout (e.g. `60s`, `5m`). |
| `--server` | resolved server | API server URL. |
| `--namespace` | global namespace (if set) | Namespace for the eval run. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

### `orlojctl eval list`

Lists all EvalRuns.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `-o` | `table` | Output format: `table`, `json`, `yaml`. |
| `--namespace` | global namespace (if set) | Namespace filter. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

### `orlojctl eval get`

Gets a specific EvalRun by name.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `-o` | `table` | Output format: `table`, `json`, `yaml`. |
| `--namespace` | global namespace (if set) | Namespace override. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

### `orlojctl eval compare`

Compares two or more completed EvalRuns side-by-side, showing pass rate, mean score, tokens, and latency.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |

### `orlojctl eval datasets`

Lists all EvalDatasets.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `-o` | `table` | Output format: `table`, `json`, `yaml`. |
| `--namespace` | global namespace (if set) | Namespace filter. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |

### `orlojctl eval export`

Exports an EvalRun's results for review.

| Flag | Default | Description |
|---|---|---|
| `--format` | `json` | Export format: `json` or `csv`. |
| `--server` | resolved server | API server URL. |

### `orlojctl eval annotate`

Annotates a single sample result in a PendingReview run.

| Flag | Default | Description |
|---|---|---|
| `--sample` | none | Sample name (required). |
| `--score` | none | Numeric score 0.0–1.0 (required). |
| `--pass` | `false` | Mark the sample as passed. |
| `--comment` | empty | Reviewer comment / reasoning. |
| `--server` | resolved server | API server URL. |

### `orlojctl eval import`

Bulk-imports annotations from a CSV file into a PendingReview run.

| Flag | Default | Description |
|---|---|---|
| `-f` | none | Path to CSV file (required). Columns: `sample_name`, `score`, `pass`, `reasoning`. |
| `--server` | resolved server | API server URL. |

### `orlojctl eval finalize`

Finalizes a PendingReview run, computing aggregate summary metrics and transitioning it to Succeeded.

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |

## Command Discovery

Use help output as the authoritative source for your current build:

```bash
orlojctl help
go run ./cmd/orlojctl help
```

## Related

- [Remote CLI & API Access](../deploy/remote-cli-access.md) — server setup, tokens, and profiles
- [Server Flags](./server-flags.md) — orlojd and orlojworker daemon flags
- [API Reference](./api.md) — REST API reference
