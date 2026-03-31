# Changelog

All notable changes to Orloj are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Native CLI tool type (`spec.type: cli`) for invoking local binaries with execve-style argv, Go `text/template` argument mapping, and container-sandboxed execution by default. Credentials are injected via `spec.cli.env_from` (no `spec.auth` for CLI tools). Worker flags `--cli-tool-allowed-commands` and `--cli-tool-max-argv-length` provide additional safety controls.

### Fixed

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

[Unreleased]: https://github.com/OrlojHQ/orloj/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/OrlojHQ/orloj/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/OrlojHQ/orloj/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/OrlojHQ/orloj/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/OrlojHQ/orloj/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/OrlojHQ/orloj/releases/tag/v0.1.0
