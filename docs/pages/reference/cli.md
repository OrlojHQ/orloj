# CLI Reference

`orlojctl` is the command-line interface for managing Orloj resources, running tasks, and inspecting system state.

For server and worker daemon flags, see [Server Flags](./server-flags.md). For load-test and alert-check tools, see [Internal Tools](./internal-tools.md).

## Usage patterns

```text
orlojctl apply -f <file-or-directory> [--run] [--dry-run] [--namespace <ns>]
orlojctl validate -f <file|dir>
orlojctl create secret <name> --from-literal key=value [...]
orlojctl create token <name> --role <role>
orlojctl approve tool-approval <name> [--decided-by <id>] [--reason <text>]
orlojctl deny tool-approval <name> [--decided-by <id>] [--reason <text>]
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
orlojctl admin create-user <username> --role <role>
orlojctl admin list-users
orlojctl admin delete-user <username>
orlojctl admin reset-password --username <name> --new-password <value>
orlojctl config path|get|use <name>|set-profile <name> [--server URL] [--token value] [--token-env NAME]
```

## Global Auth and Server Resolution

- Global auth flag: `--api-token <token>`
- Global namespace flag: `--namespace <ns>` or `-n <ns>` (applies default namespace to namespace-aware commands)
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
| `--namespace` | global namespace (if set) | Optional namespace override for manifests lacking `metadata.namespace`. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |
| `--server` | resolved server | API server URL. |

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

## `orlojctl approve` / `orlojctl deny`

Approves or denies a pending `ToolApproval`:

- `orlojctl approve tool-approval <name> ...`
- `orlojctl deny tool-approval <name> ...`

| Flag | Default | Description |
|---|---|---|
| `--server` | resolved server | API server URL. |
| `--namespace` | global namespace (if set) | Optional namespace override. |
| `-n` | global namespace (if set) | Shorthand for `--namespace`. |
| `--decided-by` | empty | Decision actor identity. |
| `--reason` | empty | Optional decision rationale. |

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
- `memories`
- `agent-policies`
- `agent-roles`
- `tool-permissions`
- `tool-approvals`
- `tasks`
- `task-schedules`
- `task-webhooks`
- `workers`
- `mcp-servers`
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
| `--namespace` | global namespace (if set) | Optional namespace override for manifests lacking `metadata.namespace`. |
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
- `orlojctl config get`: print current config/profile data
- `orlojctl config use <name>`: switch active profile

## `orlojctl init`

Positional argument `<name>` is required. It sets both the output directory and the resource name prefix.

| Flag | Default | Description |
|---|---|---|
| `--blueprint` | `pipeline` | Blueprint topology: `pipeline`, `hierarchical`, `swarm-loop`. |

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
