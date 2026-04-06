# Changelog

All notable changes to Orloj are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.6.1] - 2026-04-05

### Fixed

- **Webhook delivery auth bypass**: `POST /v1/webhook-deliveries/*` is now exempt from global API token authentication, allowing external senders (Telegram, GitHub, etc.) to deliver webhooks without an Orloj Bearer token. Authentication for these endpoints is handled by the TaskWebhook resource's own auth profile (HMAC signature or shared token).

## [0.6.0] - 2026-04-05

### Added

- **Built-in orloj tools**: `orloj.task.create` and `orloj.task.list` built-in tools for cross-task orchestration. Agents with these in `spec.allowed_tools` can create tasks from templates (fire-and-forget) and list tasks by label. Child tasks are linked via `orloj.dev/parent-task` and `orloj.dev/depth` labels. Governed like any other tool via ToolPermission, AgentPolicy `blocked_tools`, and ToolApproval.
- **AgentPolicy child task limits**: `max_child_depth` and `max_child_tasks` fields on AgentPolicy to prevent runaway task creation chains. Defaults: depth 5, children 20.
- **TaskWebhook auth profiles**: `hmac` and `shared_token` profiles for TaskWebhook, supporting configurable HMAC algorithm (`sha256`, `sha1`, `sha512`), payload format (`body`, `timestamp_dot_body`, `prefix_timestamp_body`), signature encoding (`hex`, `base64`), and structured header parsing (`kv_pairs` for Stripe-style combined headers). The `shared_token` profile enables constant-time token comparison for services like Telegram. Existing `generic` and `github` profiles are unchanged.
- **README**: Document official [Python](https://pypi.org/project/orloj-sdk/) and [TypeScript](https://www.npmjs.com/package/orloj) HTTP API SDKs ([orloj-python-sdk](https://github.com/OrlojHQ/orloj-python-sdk), [orloj-js-sdk](https://github.com/OrlojHQ/orloj-js-sdk)), with PyPI and npm badges.

### Changed

- **Tool runtime docs**: Core concepts, tool reference, and tool concept pages now list seven transport types (HTTP, external, gRPC, webhook-callback, MCP, CLI, WASM) and four isolation modes (none, sandboxed, container, WASM), correcting previous counts and removing references to the unimplemented `queue` type.

### Removed

- **`queue` tool type**: Removed from validation and documentation. The type was accepted by `spec.type` validation and documented, but no queue runtime existed — tools with `type: queue` silently fell through to the HTTP client at runtime. A future queue transport can be re-introduced when a `QueueToolRuntime` implementation is available.

### Fixed

- **AgentPolicy enforcement in message-driven mode**: `blocked_tools` and `allowed_models` checks (`EnforcePoliciesForAgent`) are now enforced in message-driven execution mode. Previously these AgentPolicy fields were only checked in synchronous mode, allowing agents in message-driven (production) deployments to use blocked tools or disallowed models without error.
- **ToolApproval in synchronous mode**: The synchronous execution path now passes a `GovernedToolApprovalContext` to the governed tool runtime and creates a `ToolApproval` resource when approval is required. Previously, sync mode passed `nil`, causing approved tools to re-trigger approval on every re-execution after human approval.
- **Tool type dispatch**: `GovernedToolRuntime` now explicitly routes every validated `spec.type` (`http`, `external`, `grpc`, `webhook-callback`, `mcp`, `cli`, `wasm`) to its correct transport runtime. Previously only `mcp` and `cli` were explicitly dispatched; all other types — including `external`, `grpc`, and `webhook-callback` — fell through to the base HTTP client regardless of their declared type. Unknown types now fail closed with an explicit error instead of silently executing as HTTP.
- **HTTP tool registry propagation**: The default `HTTPToolClient` created when callers pass `nil` as the base runtime now receives the tool capability registry. Previously, both production call sites (`task_controller` and `agent_message_consumer`) passed `nil`, which created an `HTTPToolClient` without a registry — causing every low/medium-risk HTTP tool to fail with "unsupported tool" instead of executing.
- **Task controller**: `reconcilePending` no longer increments `status.attempts`; attempts are counted when a task is claimed in the store (`applyTaskClaim`), avoiding duplicate increments if a pending task is reconciled after claim.

## [0.5.1] - 2026-04-02

### Added

- **`GET /v1/auth/config`**: `setup_token_required` indicates when `ORLOJ_SETUP_TOKEN` is set so clients can require `setup_token` on initial setup; the web console setup page shows a setup-token field when applicable. `orlojctl status` includes `setup_token_required` in table/JSON output.

### Fixed

- **Sequential agent handoffs**: Downstream agents in sequential task execution now receive the upstream agent's actual output (`result.Output`) before falling back to the last event message, instead of being handed generic values such as `worker completed`.
- **Postgres task claiming**: Worker claim SQL used placeholder indices `$2`/`$3` while the driver bound arguments as `$1`/`$2`, causing `could not determine data type of parameter $1` (SQLSTATE 42P18) during embedded worker reconcile when both region and assigned-worker hints were set.

## [0.5.0] - 2026-04-01

### Added

- Native CLI tool type (`spec.type: cli`) for invoking local binaries with execve-style argv, Go `text/template` argument mapping, and container-sandboxed execution by default. Credentials are injected via `spec.cli.env_from` (no `spec.auth` for CLI tools). Worker flags `--cli-tool-allowed-commands` and `--cli-tool-max-argv-length` provide additional safety controls.

### Changed

- **OpenAPI**: Regenerated `openapi/openapi.yaml` from `openapi/build_openapi.py` with concise `info.description`, **secrets** tag documentation for redaction/`***` merge, a supported replacement-style namespaced PUT rename note in `info`, model-endpoint/secret operation summaries, and `openapi/schemas/secret.yaml` field/resource descriptions. [CONTRIBUTING.md](CONTRIBUTING.md#openapi) documents the generator workflow.
- **Model routing (`openai-compatible`)**: Split `openai-compatible` into a dedicated provider plugin (with `openai_compatible` alias) so it no longer inherits strict `openai` API-key requirements by alias. `openai-compatible` now supports both authenticated and unauthenticated endpoints, while `openai`, `anthropic`, and `azure-openai` remain auth-required.
- **Model endpoint create UX (web console)**: The create dialog now shows pre-create warnings for common local-model misconfiguration: using `/v1` with native `provider: ollama`, cloud-style model IDs on local/self-hosted endpoints, and missing `auth.secretRef` on providers that require it.
- **Docs alignment (model auth + local Ollama)**: Updated model endpoint reference/concepts/guides/troubleshooting/configuration docs to reflect runtime behavior: `openai-compatible` auth is optional, native Ollama uses root base URL (not `/v1`), and model secret env fallback is `ORLOJ_SECRET_<name>` (prefix-configurable).

### Fixed

- **API PUT rename (namespaced resources)**: For agents, agent systems, tools, secrets, memories, agent policies, agent roles, tool permissions, tasks (including **task log** rows in Postgres), task schedules, task webhooks, workers, model endpoints, and MCP servers, PUT keeps `metadata.name` from the body when it differs from the URL path and **moves** the stored object to the new scoped key (409 if the target name already exists). Previously many handlers overwrote the body name from the path, so YAML renames appeared to save but reverted.
- **MCP server detail save**: Added `PUT /v1/mcp-servers/{name}` support in the API handler/store path (including `If-Match` preconditions and rename conflict handling), so MCP server edits from the UI YAML tab now persist instead of returning method-not-allowed.
- **Secret PUT / YAML tab**: Bodies that still contain the API redaction placeholder `***` in `spec.data` / `spec.stringData` (as returned by GET) are merged with the stored secret before validation, so renaming or editing metadata without re-entering secret material no longer fails with invalid base64.
- **Resource YAML detail tabs (frontend, all kinds)** — agents, agent systems, tools, secrets, memories, MCP servers, policies, roles, tool permissions, tool approvals, tasks, task schedules, task webhooks, workers, model endpoints: YAML saves use the **route** name for PUT/DELETE, **re-fetch** the resource immediately before PUT to merge a current `resourceVersion` (avoids 404/409 from stale cache or editor JSON), **update** the detail query cache from the PUT response, **navigate** when `metadata.name` changes after save, and show a **load error** instead of stuck loading when GET fails. Workers use the existing cluster-wide list lookup and a dedicated `["Worker","detail",name]` query key.
- **Issue #8 (local/self-hosted OpenAI-compatible endpoints)**: Model endpoints using `provider: openai-compatible` no longer fail with `requires auth.secretRef` when no auth is needed (for example local Ollama `/v1`). If `auth.secretRef` is provided, the secret is still resolved and forwarded as bearer auth.
- **OpenAI-compatible request auth header behavior**: `Authorization` is now omitted when no API key is configured, preventing invalid empty-bearer requests to unauthenticated self-hosted providers.
- Token and user name path segments containing encoded slashes (`%2F`) are now rejected. Previously the slash check ran before URL-decoding, so `%2F` bypassed validation.
- Token CRUD audit events now explicitly attach caller identity in the handler, matching the pattern used by user CRUD handlers for consistency.
- In-memory `UpsertUser` logs a warning when an existing user's `CreatedAt` timestamp is unparseable instead of silently resetting it.
- YAML `Tool` manifest parsing: nested JSON Schema `type` keys under `spec.input_schema` no longer overwrite `spec.type` (fixes `orlojctl validate` for CLI tool examples with schemas).

## [0.4.0] - 2026-03-30

### Added

- **Native multi-user authentication** (`--auth-mode native`): local username/password auth with bcrypt-hashed passwords, session cookies, and role-based access control.
- **API token management**: named bearer tokens stored with SHA-256 hashes. CRUD via `POST /v1/tokens`, `GET /v1/tokens`, `DELETE /v1/tokens/{name}` (admin-only).
- **Multi-user admin API**: `POST /v1/auth/users`, `GET /v1/auth/users`, `DELETE /v1/auth/users/{username}` for managing local accounts (admin-only). Server-generated passwords returned once on creation.
- **Named bearer token format**: `name:token:role` alongside the existing legacy `token:role` format; env tokens (`ORLOJ_API_TOKENS`) checked first, then store-managed tokens.
- **First-user bootstrap**: `/v1/auth/setup` creates the initial admin account; optionally protected by `ORLOJ_SETUP_TOKEN` env var.
- **Auth identity propagation**: bearer and session callers carry `AuthIdentity` (name, role, method) through request context for audit logging. Bearer principals logged as `token-name` or `bearer:<role>`.
- **Audit logging for admin operations**: token and user create/delete events emitted via the audit extension with principal, resource kind, and action.
- `orlojctl`: `create token`, `get tokens`, `delete token` commands for API token lifecycle.
- `orlojctl`: `admin create-user`, `admin list-users`, `admin delete-user` commands for local user management.
- `orlojctl`: `auth whoami` queries `/v1/auth/me` and prints current identity.
- `orlojctl`: `admin reset-password --username ... --new-password ...` for targeted password resets (invalidates target sessions).
- `orlojctl`: global `--namespace` / `-n` default for namespace-aware commands; `apply` supports `--dry-run` and optional namespace override on manifest payloads.
- `orlojctl`: `approve` / `deny` for pending tool approvals (`tool-approval`), with optional `--decided-by`, `--reason`, and namespace flags.
- `orlojctl`: richer `get` (fetch by resource name, `-o table|json|yaml`, `tool-approvals` list view, memory entry listing, namespace filter for task watch).
- `orlojctl`: `describe`, `edit`, `diff`, `wait`, `cancel task`, `retry task`, `top`, `messages`, `metrics`, `health`, `status`, and shell `completion` (bash/zsh/fish).
- OpenAPI: optional `reason` on the tool approval decision request body (`openapi/schemas/common.yaml`).
- OpenAPI: full schema and endpoint documentation for all OSS auth and token endpoints.
- PostgreSQL migration `009_auth_users_and_tokens` creating `auth_local_users` and `auth_api_tokens` tables with backfill from env-configured admin credentials.
- Startup warning when `--auth-mode native` is active but `ORLOJ_SETUP_TOKEN` is not set.

### Changed

- `/v1/auth/me` now returns identity fields (`method`, `name`, `role`, compat `username`) for UI/CLI bootstrap.
- `/v1/auth/admin/reset-password` requires an explicit `username` field and invalidates the target user's sessions.
- Native session authorization now uses the actual logged-in user's role instead of a hardcoded default.
- `orlojctl`: `main` exits with `cli.ExitCode(err)` so coded CLI errors can use non-default exit statuses.

### Fixed

- API: SSE watch endpoints (`/v1/events/watch`, resource `…/watch` URLs) work again when requests use bearer authentication (the auth middleware response wrapper now forwards `Flush` to the underlying connection).
- API: session deletion failures after password change, password reset, and user deletion are now logged instead of silently ignored.
- OpenAPI: `/v1/auth/change-password` spec now correctly declares authentication as required (matching the actual server behavior).

## [0.3.0] - 2026-03-29

### Changed

- `orlojctl apply -f <dir>` now skips runnable `Task` manifests by default (`spec.mode: run` or omitted mode). Use `--run` to include runnable tasks in directory applies. Single-file apply behavior is unchanged.
- Internal CLI file naming was aligned from `agentctl*` to `orlojctl*` and the `Makefile` now uses `ORLOJCTL` as the canonical CLI variable with backward-compatible `AGENTCTL` alias support.

## [0.2.0] - 2026-03-29

### Added

- OpenAPI 3.1 specification for the full v1 API surface
- `orlojctl validate` command for offline manifest validation
- Guided "first agent system in 5 minutes" tutorial ([docs](https://docs.orloj.dev/guides/five-minute-tutorial)), linked from the docs home page, guides overview, README, and quickstart

### Changed

- `orlojctl init` now takes a positional `<name>` argument that sets both the output directory and resource prefix; `--blueprint` defaults to `pipeline`; `--name` and `--dir` flags removed
- `orlojctl apply -f` accepts a manifest **file or directory** (same recursive discovery as `validate` for `.yaml`, `.yml`, and `.json`); applies each file and aggregates errors

## [0.1.1] - 2026-03-27

### Fixed

- GoReleaser now produces per-binary archives (orlojd, orlojworker, orlojctl
  are separate downloads instead of a single combined archive)

### Added

- `scripts/install.sh` for curl-based binary installation

## [0.1.0] - 2026-03-26

### Added

- Initial public release
- 15 resource kinds: Agent, AgentSystem, ModelEndpoint, Tool, Secret, Memory,
  AgentPolicy, AgentRole, ToolPermission, ToolApproval, Task, TaskSchedule,
  TaskWebhook, Worker, McpServer
- Server (`orlojd`) with embedded web console, REST API, PostgreSQL and
  in-memory storage backends
- Distributed task execution (`orlojworker`) with lease-based claiming,
  message-driven mode via NATS JetStream, and configurable tool isolation
- CLI (`orlojctl`) with apply, get, delete, run, init, logs, trace, graph,
  events, config subcommands
- Model routing for OpenAI, Anthropic, Azure OpenAI, and Ollama providers
- DAG-based orchestration: pipeline, hierarchical, and swarm-loop topologies
  with fan-out/fan-in and configurable join semantics
- Governance enforcement: policies, roles, tool permissions, and gated tool
  approval workflows
- MCP server integration with automatic tool discovery and sync
- Memory resources with vector-backed retrieval (pgvector)
- Task scheduling (cron) and webhook-triggered task creation
- OpenTelemetry tracing, Prometheus metrics, and structured logging
- Docker Compose stack for local multi-worker deployment
- Homebrew tap distribution (`OrlojHQ/orloj`)
- Blueprint scaffolding via `orlojctl init`

[Unreleased]: https://github.com/OrlojHQ/orloj/compare/v0.6.1...HEAD
[0.6.1]: https://github.com/OrlojHQ/orloj/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/OrlojHQ/orloj/compare/v0.5.1...v0.6.0
[0.5.1]: https://github.com/OrlojHQ/orloj/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/OrlojHQ/orloj/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/OrlojHQ/orloj/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/OrlojHQ/orloj/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/OrlojHQ/orloj/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/OrlojHQ/orloj/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/OrlojHQ/orloj/releases/tag/v0.1.0
