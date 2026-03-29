# Changelog

All notable changes to Orloj are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.2.0]: https://github.com/OrlojHQ/orloj/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/OrlojHQ/orloj/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/OrlojHQ/orloj/releases/tag/v0.1.0
