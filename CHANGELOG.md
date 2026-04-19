# Changelog

All notable changes to Orloj are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Human review checkpoints with `TaskApproval`**: Orloj can now pause a workflow for human review of agent output or final task output. `AgentSystem` graph nodes support `spec.graph.<node>.review`, and systems support `spec.completion_review`, each with `checkpoint_id`, `reason`, `ttl`, `allow_request_changes`, and `max_review_cycles`. Reviewers can `approve`, `deny`, or `request_changes`; change requests rerun the producing agent with `review.*` context and create a new approval cycle linked through `spec.supersedes`.
- **Task approval API, CLI, UI, and OpenAPI support**: added `POST/GET /v1/task-approvals`, `GET/DELETE /v1/task-approvals/{name}`, and decision endpoints for `approve`, `deny`, and `request-changes`. The web console now has first-class `TaskApproval` detail pages and a unified approvals queue, `orlojctl` supports `get task-approvals` plus approval actions, and the generated OpenAPI schema documents the full resource and decision surface.
- **Human review docs and examples**: added `TaskApproval` concept/reference pages, a `Human Review Checkpoints` guide, sidebar navigation entries, and sequential/message-driven example manifests for review-gated systems and tasks.

### Changed

- **Explicit task approval blocker tracking**: tasks in `WaitingApproval` now persist `status.blocked_on` with the exact blocking approval kind, name, and reason. This makes approval resume deterministic for both `ToolApproval` and `TaskApproval` flows and exposes the active blocker in the UI/API.
- **Approval decision comments are consistent across approval types**: tool approval approve/deny endpoints and CLI commands now accept optional reviewer `comment` values, matching the audit/comment model used by task approvals.
- **Web UI reskin (modernized visual theme)**: retuned the design-token palette for a contemporary look while preserving the green brand and existing information architecture. Backgrounds are now neutral near-black (`#09090b`/`#111113`) instead of blue-tinted GitHub gray; the accent is a softened mint (`#4ade80`) reserved for CTAs and active states, with the saturated `#22c55e` green kept only for `Ready`/`Succeeded` status. Cards use hover-reveal borders over subtle elevation instead of persistent hard borders; page titles and stat values are larger (1.75rem / 2rem) with tightened letter-spacing; zero-value metric cards are dimmed to de-emphasize empty state. The sidebar active item is a neutral pill with a 2px accent left-bar (replacing the solid green-tinted highlight), and category labels are tighter with increased letter-spacing. The topbar gained a glassmorphism backdrop (14px blur with saturation) and a prominent `⌘K` search chip trigger (previously icon-only). Graph nodes now have a type-specific 2px colored top stripe (system/agent/model/tool/secret/memory/role/task/schedule/webhook/worker), soft elevation on hover, and a green `--accent-glow` when running. Dependency edges are solid 1.5px with animated flow only on active routes. The minimap is now opt-in via a toolbar toggle button alongside the legend toggle. Trace event type badges switched from saturated full-color chips to neutral monospace pills with a colored dot, reducing the "rainbow" appearance of long traces. All changes are token- and CSS-driven; no layout, IA, or backend changes.
- **Web UI**: agent role resources use the `UserCog` icon in the system graph, sidebar, and search (replacing `KeyRound`, which was easy to mistake for the tools wrench at small sizes)
- **Web UI system topology: edge polish and honest flow animation**: dependency edges (the dashed gray lines between agents and their model/tools/secrets/roles/memory) are now legible at rest — `--edge-stroke` in the dark theme moved from `#3a3a3f` to `#71717a` and the two stacked opacity damps (inline `0.5` plus CSS `stroke-opacity: 0.55`) were collapsed to a single `stroke-opacity: 0.6` in CSS. Hovering a node now also shifts its related dependency edges to an accent-tinted stroke (`color-mix(--accent 60%, --edge-stroke)`) so the highlighted sub-graph reads as a cohesive group, not just "same line, thicker." Routing edges no longer paint solid green at rest — they share the neutral `--edge-stroke` with dependency edges (solid instead of dashed, slightly thicker and more opaque, so the pipeline path still reads as the primary flow). They only turn green when flow is actually happening, and even then, on the system detail graph only the specific hops landing on a currently-running agent churn (edge target ∈ `runningAgents`), so a single in-flight pipeline shows motion on one edge at a time instead of every routing edge moving in lockstep; on the task detail graph animation is gated on `task.status.phase === "Running"`. Hovering a node also paints its related routing edges full accent green to signal the relevant pipeline path. This keeps systems with many idle agents visually quiet while still giving live activity a clear green signal. The flow animation itself was slowed from `0.8s linear` to `1.6s ease-in-out` so it reads as agentic flow rather than a network spinner.
- **Web UI system topology: hover highlighting now follows semantic dependencies, not just direct edges**: previously, hovering an agent only lit up nodes connected by a direct graph edge, so a shared model endpoint (edged to the system node to keep the graph readable) or a transitive dependency like a tool's secret never highlighted. The hover logic now uses a precomputed "semantic neighbour" map that records every agent's `model_ref` (and that model's `auth.secretRef`), its `spec.tools` (and each tool's `auth.secretRef`), `spec.roles`, and `memory.ref`, regardless of whether the rendered edge landed on the agent or got re-routed to the system. All of an agent's declared dependencies now light up on hover.
- **Web UI system topology: rerun instances collapse to a single node per task lineage**: reruns of the same task (tracked via `orloj.dev/source-task` label) no longer stack up alongside the original in the topology graph. Each lineage renders exactly one node: an active instance (`Pending`/`Running`/`WaitingApproval`) when one exists, otherwise the most recent terminal instance (`Succeeded`/`Failed`/`DeadLetter`) by `createdAt`. Failed or dead-lettered runs stay visible until a new run of the same lineage starts, so breakages aren't hidden. Templates (`spec.mode: template`) are always preserved so schedule/webhook wiring remains intact. Full execution history is unchanged on the Tasks page.

### Fixed

- **Rate limiter goroutine leak**: `authRateLimiter` no longer spawns a background goroutine per instance; cleanup runs inline during `allow()`, throttled to every 5 minutes
- **Watch streams killed by request timeout**: SSE watch endpoints (`/watch` suffix) now bypass the global read timeout middleware
- **Watch polling with event bus present**: `watchResourceStream` now subscribes to the event bus reactively instead of polling snapshots every 1s when a bus is available
- **Task controller claim scoping bug**: fixed incorrect indentation/brace alignment in `ReconcileOnce` that caused slot release and early-return paths to execute at the wrong scope
- **Slice aliasing in worker claim hints**: `workerClaimHints()` now copies `SupportedModels` to prevent mutations from corrupting the worker spec
- **Work queue blocking on full channel**: replaced fixed-size channel with a growable slice + notify channel so `Enqueue` never blocks
- **Event bus subscriber double-close panic**: `Subscribe` goroutine now uses a single deferred cleanup, preventing races on concurrent context cancellation
- **Agent message bus close/send panic**: `Close()` now signals via a broadcast `done` channel instead of closing individual subscriber channels, eliminating send-on-closed-channel races
- **Bounded tool helpers leaking goroutines**: removed redundant wrapper goroutines in `callToolRuntimeBounded`, `runContainerCommandBounded`, and `executeWASMToolBounded`; the underlying runtimes already honor context cancellation
- **Embedding provider data race**: `OpenAIEmbeddingProvider.dimensions` is now an `atomic.Int64`
- **MCP HTTP transport session ID race**: added `RWMutex` for session reads/writes and a serialization mutex for session establishment
- **MCP image inspect swallowing errors**: `ensureImagePulled` now distinguishes "image not found" from actual Docker daemon errors instead of treating all failures as "not present"
- **Webhook poll errors silently dropped**: non-context HTTP errors from poll requests now return a proper `ToolError` instead of `nil`
- **Task store returning shared references**: all `TaskStore` read/write paths now deep-copy tasks to prevent callers from corrupting store state
- **Worker heartbeat shutdown using cancelled context**: the final "NotReady" write now uses a fresh 5s context instead of the already-cancelled parent
- **SQL claim missing GPU/model filters**: `claimNextDueTaskSQL` now filters on `RequiresGPU` and `SupportedModels` hints, preventing workers from claiming incompatible tasks

## [0.9.0] - 2026-04-14

### Changed

- **CLI migrated from Go `flag` to cobra/pflag**: `orlojctl` now uses [cobra](https://github.com/spf13/cobra) for command and flag parsing, matching the UX conventions of `kubectl`. Flags like `-n`/`--namespace` now work correctly when placed after positional arguments (e.g. `orlojctl delete agent my-agent -n prod`), fixing a long-standing parsing bug where trailing flags were silently ignored. Shell completions (`orlojctl completion bash|zsh|fish|powershell`) now use Cobra's built-in generators.

### Fixed

- **HTTP tool runtime: third-party `"status"` fields no longer trigger a false "missing error envelope" error**: the HTTP tool client previously treated any 2xx response whose JSON body contained a non-empty `status` field as an Orloj tool-contract envelope. APIs like Vapi that return `"status": "queued"` in their success payloads would therefore fail with `tool response missing error envelope` because no `error` object was present. The check is now restricted to the three Orloj-defined status values (`ok`, `error`, `denied`); any other value causes the raw response body to be returned as the tool output.

- **HTTP tools with `auth.secretRef` no longer fail with "no secret resolver is configured"**: `ConfigureHttpRuntime` is now called in both the message-driven worker (`AgentMessageConsumer`) and the synchronous task controller alongside the existing `ConfigureCliRuntime` / `ConfigureExternalRuntime` / `ConfigureGRPCRuntime` / `ConfigureWebhookCallbackRuntime` calls. Previously, the HTTP base runtime (`GovernedToolRuntime.baseRuntime`) was always constructed with a `nil` secret resolver, causing every HTTP tool that declared `spec.auth.secretRef` to fail at call time with a non-retryable `secret_resolution_failed` error regardless of whether the secret existed in the store.

- **Embedded worker secret resolver missing in `orlojd`**: the `AgentMessageConsumerOptions` struct in `cmd/orlojd/main.go` was missing `SecretResolver`, so the embedded message-driven worker in `orlojd` (started with `--embedded-worker`) always ran with a `nil` secret resolver. Any tool using `spec.auth.secretRef` would fail with `secret_resolution_failed` even when the secret existed in the store. The `cliSecretResolver` (store-backed + env-var chain) is now passed as `SecretResolver` in the embedded consumer options, matching the wiring in `cmd/orlojworker/main.go`.

- **Docker-image MCP servers: three fixes for container initialization failures**: (1) The YAML parser for `McpServer` now parses `mountPath` / `mount_path` on env entries, so file-based secrets (e.g. a kubeconfig) are correctly bind-mounted into the container instead of being silently dropped — fixing `No active cluster!` crashes. (2) Container images are pre-pulled (`docker pull`, 5 min timeout) before `docker run`, so the image-download time no longer eats the 30-second initialize handshake timeout. (3) `--tmpfs /tmp:rw,noexec,nosuid` is added to `docker run` so Node.js-based servers that need a writable temp directory work under `--read-only`. (4) The child process is no longer bound to the init-timeout context via `exec.CommandContext`, which previously killed healthy containers the moment the init handshake completed and the timeout context was cancelled.

- **MCP server secret resolver now scoped to server namespace**: `resolveEnv` and `buildHTTPTransport` in `McpSessionManager` now call `WithNamespace` (when the resolver supports it) to scope secret lookups to the `McpServer` resource's own namespace, matching the behaviour of CLI tool secrets. Previously, bare secret names (e.g. `secretRef: my-secret`) resolved against the default namespace and the format `name/value` was misread as `namespace/name`, causing "not found" errors for any secret referenced by env var on a Docker-image MCP server.

- **Task controller: MCP tool references no longer permanently fail on startup race**: when a task is applied before the MCP server has finished connecting and registering its tools, the task controller now detects the transient condition (MCP server phase is neither `Ready` nor `Error`) and requeues the task instead of marking it permanently `Failed`. Once the server becomes `Ready` and the tools appear, the task proceeds normally. Non-MCP tool references that are genuinely missing still fail immediately.

- **Secret YAML parser: literal block scalars (`key: |`) now parsed correctly**: the line-based `parseSecretManifestWithoutNormalize` parser previously did not understand YAML literal block scalars. A multi-line value such as a kubeconfig under `spec.stringData.value: |` was stored as the bare `|` character, and any embedded YAML keys in the content (e.g. `kind: Config` from a kubeconfig) overwrote the Orloj resource's own `kind` field, causing `unsupported kind "Config" for Secret` at apply time. The parser now accumulates literal block content into the correct key and guards `kind`/`apiVersion` assignment to document-root lines only (`indent == 0`).

### Added

- **Task rerun on apply**: `apply --run` now re-executes terminal tasks (DeadLetter, Failed, Succeeded) by creating a new task instance with a unique name (`<name>-run-<timestamp>`); the original task is preserved for history. The `?rerun=true` query parameter is also available on `POST /v1/tasks` for programmatic clients. Returns `409 Conflict` if the task is still active. Both `--run` (apply) and `retry` now set `orloj.dev/source-task` and `orloj.dev/source-task-namespace` labels on new instances for lineage tracking.

- **Real-scenario `18-mcp-k8s-docker`**: new live-validation scenario that registers `ghcr.io/strowk/mcp-k8s-go` as a Docker-image MCP server (`spec.image`), delivers a kubeconfig to the container via file-based secret (`mountPath: /secrets/kubeconfig`), and runs a triage agent that calls `list_namespaces` and `list_pods` against a real cluster. Gate checks tool auto-generation (type=mcp), `tool_filter` enforcement (exactly 2 tools), trace coverage, and required output markers. New `make real-apply-k8s-mcp`, `make real-gate-k8s-mcp`, and `make real-gate-wave6` targets.

- **Real-scenario `19-sales-system`**: new example scenario demonstrating an AI-powered outbound sales pipeline with Vapi voice calling. An AgentSystem handles two phases via conditional `input_type` routing: outreach (lead qualification → Vapi AI call) and follow-up (transcript analysis → CRM update → meeting booking → reporting). The Vapi end-of-call webhook is received by a `TaskWebhook` resource (`shared_token` auth via `x-vapi-secret` header) that creates a fresh task per call, avoiding fixed-name conflicts and providing authenticated delivery. The Vapi payload lands under `vapi_payload` in task input; `input_type: transcript` is injected by the template, so the router routes correctly without a proxy.

## [0.8.0] - 2026-04-13

### Added

- **Conditional edge routing for AgentSystem graphs**: graph edges can now carry an optional `condition` that is evaluated against the completing agent's output. Supported operators: `output_contains` (case-insensitive substring), `output_not_contains`, `output_matches` (regex), and `default` (fallback). When conditions are present, only matched edges fire, enabling data-dependent routing patterns like triage, quality gates, and intelligent hierarchical delegation. Join gates (`wait_for_all`, `quorum`) automatically adjust their expected branch count when conditional routing reduces the set of dispatched upstream agents. Requires message-driven execution mode.
- **JSON path conditions for structured routing**: edge conditions now support `output_json_path` with comparison operators (`equals`, `not_equals`, `contains`, `greater_than`, `less_than`) to route on specific fields within JSON agent output. Paths use dot-notation (e.g. `$.route`, `$.result.confidence`). `contains` works with both arrays (element membership) and strings (substring match). Numeric operators parse string thresholds for type-safe comparison.
- **Structured output for agents**: new `spec.execution.output_schema` field on Agent resources. Defines a JSON Schema that constrains the model's output via provider-native structured output (constrained decoding). Supported across OpenAI (`response_format.json_schema`), Azure OpenAI, OpenAI-compatible endpoints, Anthropic (`output_config.format`), and Ollama (best-effort `format` field). Pairs with JSON path conditions for end-to-end typed routing.
- **Delegation primitive for graph nodes**: new `delegates` and `delegate_join` fields on `GraphEdge` enable two-phase node execution — dispatch to downstream delegates, collect their reports via a delegation gate, re-execute the node with all results in context (`inbox.delegation.*`), then follow normal edges. Supports the same condition operators, join modes (`wait_for_all`, `quorum`), and structured output as regular edges. Delegates automatically route back to the delegator when they reach a terminal point. `delegate_of` metadata propagates through sub-branches for multi-hop delegation trees. Enables hierarchical agent systems (CEO → VPs → Leads) where each manager is a single graph node.

### Security

- **SSRF hardening for outbound runtimes**: all outbound HTTP callers (model gateways, MCP HTTP transport, HTTP/external/webhook-callback tool runtimes, persistent-memory backend, OpenAI embedding provider, OAuth2 token cache) now route through a shared `SafeHTTPClient` whose `net.Dialer.Control` hook enforces SSRF policy against the actual resolved IP at dial time. This closes the previous hostname-bypass gap where `ValidateEndpointURL` only inspected literal IPs and trusted hostnames (including DNS-rebind attacks and names like `metadata.google.internal` that resolve to 169.254.169.254). Loopback, link-local, cloud metadata (AWS/GCP/Azure IMDS, including IPv6 `fd00:ec2::254`), RFC 6598 carrier-grade NAT (100.64.0.0/10), and unspecified addresses are blocked regardless of the allowPrivate flag; RFC 1918 and IPv6 ULA addresses are blocked unless the caller explicitly opts in.
- **`spec.allowPrivate` on `ModelEndpoint`**: new optional boolean that permits a specific model gateway to reach RFC 1918 / ULA / CGNAT destinations. Defaults: `ollama` → `true` (preserves existing local Ollama deployments), every other provider (`openai`, `openai-compatible`, `anthropic`, `azure-openai`, and custom providers) → `false`. **Upgrade note:** if you run an OpenAI-compatible server (vLLM, LM Studio, LocalAI, LiteLLM, TGI, etc.) on `localhost` or a private network under `provider: openai-compatible`, you must set `spec.allowPrivate: true` on those `ModelEndpoint` resources after upgrading or the gateway will fail at dial time with a "private address … is not allowed" error naming the exact field to change.
- **Host CLI tool creation requires admin**: creating or updating a Tool with `spec.type: cli` and `spec.runtime.isolation_mode: none` (host execution) now requires the `admin` role. Writers can still create container-isolated CLI tools and all other tool types. This mirrors the existing admin gate on `/v1/mcp-servers`.
- **CLI runtime fails closed without command allowlist**: the host CLI tool runtime now refuses execution when `ORLOJ_CLI_TOOL_ALLOWED_COMMANDS` is not configured, instead of allowing any command. **Upgrade note:** if you use host CLI tools (`isolation_mode: none`), ensure `ORLOJ_CLI_TOOL_ALLOWED_COMMANDS` is set to the commands you want to permit, or switch to `isolation_mode: container`.
- **Auth rate-limit bypass via spoofed forwarding headers**: `extractClientIP` no longer unconditionally trusts `X-Forwarded-For` / `X-Real-IP` headers. By default, forwarding headers are ignored and the TCP peer address is used for per-client rate limiting. New flag `--trusted-proxies` (env: `ORLOJ_TRUSTED_PROXIES`) accepts comma-separated CIDRs of reverse proxies whose forwarding headers should be trusted; when configured, `X-Forwarded-For` is parsed right-to-left to extract the real client IP. The same trust gate now applies to `X-Forwarded-Proto` for session cookie security. **Upgrade note:** if Orloj runs behind a reverse proxy, set `--trusted-proxies` to your proxy's CIDR(s) to preserve per-client auth rate limiting; without it, all clients behind the proxy share a single rate-limit bucket.

## [0.7.0] - 2026-04-07

### Added

- **Ephemeral MCP sessions with idle timeout**: new `spec.idle_timeout` field on McpServer resources (e.g. `5m`). Sessions are automatically shut down after the configured idle period and transparently recreated on the next `tools/call`. Tool resources persist in the store so agents always know what tools are available. Default `0` preserves the current always-on behavior.
- **Container-backed MCP stdio transport**: new `spec.image` field on McpServer resources. When set, the MCP server runs inside a Docker container (`docker run --rm -i`) with sandboxing (read-only FS, cap-drop=ALL, memory/CPU limits). If `command` is also set it overrides the image entrypoint; if only `image` is set the image's built-in entrypoint is used.
- **File-based secrets for container MCP servers**: new `mountPath` field on `spec.env` entries. When set, the resolved secret value is written to the specified path inside the container as a bind-mounted file, enabling MCP servers that require file-based credentials (e.g. OAuth JSON keys, service account files). The env var is set to the mount path so the server can locate the file.

### Fixed

- **MCP spec-drift detection**: editing an McpServer spec (e.g. changing the image tag or command) now correctly tears down the stale session and rebuilds with the updated spec, instead of silently returning the cached session.

## [0.6.3] - 2026-04-06

### Added

- **Inline task templates for TaskWebhook**: `spec.task_template` can now be used as an alternative to `spec.task_ref`, allowing a webhook to embed its task spec directly without creating a separate Task resource. Exactly one of `task_ref` or `task_template` must be set.

## [0.6.2] - 2026-04-06

### Added

- **`event_id_from_body` for TaskWebhook idempotency**: extract the deduplication event ID from the JSON request body using a dot-separated field path (e.g. `update_id`, `data.event_id`), instead of requiring an HTTP header. Enables direct Telegram-to-Orloj webhook integration without a proxy, since Telegram puts `update_id` in the body, not a header. When `event_id_from_body` is set, `event_id_header` is no longer required.

### Fixed

- **Tool `description` and `input_schema` dropped by YAML parser**: the constrained YAML manifest parser for Tool resources did not populate `spec.description` or `spec.input_schema`, causing the model to receive a generic fallback schema instead of the tool's actual JSON Schema. This led to malformed tool call arguments (e.g. `invalid character 'ð'` errors) when the model wrapped its response in the fallback `{input: string}` envelope.
- **Docker socket access for container tool isolation**: all Docker Compose files now mount `/var/run/docker.sock` and set `group_add: ["0"]` on `orlojd` and `orlojworker` services, and both images include `docker-cli`. Previously, containerized tool execution silently failed because the Docker CLI was missing and the socket was not accessible.

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

[Unreleased]: https://github.com/OrlojHQ/orloj/compare/v0.9.0...HEAD
[0.9.0]: https://github.com/OrlojHQ/orloj/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/OrlojHQ/orloj/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/OrlojHQ/orloj/compare/v0.6.3...v0.7.0
[0.6.3]: https://github.com/OrlojHQ/orloj/compare/v0.6.2...v0.6.3
[0.6.2]: https://github.com/OrlojHQ/orloj/compare/v0.6.1...v0.6.2
[0.6.1]: https://github.com/OrlojHQ/orloj/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/OrlojHQ/orloj/compare/v0.5.1...v0.6.0
[0.5.1]: https://github.com/OrlojHQ/orloj/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/OrlojHQ/orloj/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/OrlojHQ/orloj/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/OrlojHQ/orloj/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/OrlojHQ/orloj/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/OrlojHQ/orloj/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/OrlojHQ/orloj/releases/tag/v0.1.0
