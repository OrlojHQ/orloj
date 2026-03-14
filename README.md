# orloj (control-plane scaffold)

Kubernetes-style control plane scaffold for Agents-as-Code.
Kubernetes for Cognitive Systems

## Documentation

- Docs home: [`docs/index.md`](docs/index.md)
- Authoring guide: [`docs/README.md`](docs/README.md)
- OSS boundary contract: [`BOUNDARY.md`](BOUNDARY.md)
- Extension contracts: [`docs/reference/extensions.md`](docs/reference/extensions.md)
- Phase history: [`docs/phases/phase-log.md`](docs/phases/phase-log.md)
- Starter blueprint manifests: [`examples/blueprints/README.md`](examples/blueprints/README.md)
- Runtime test scenarios: [`testing/scenarios/README.md`](testing/scenarios/README.md)
- Real-model runtime scenarios: [`testing/scenarios-real/README.md`](testing/scenarios-real/README.md)

## Project Policies

- License: [`LICENSE`](LICENSE) (Apache License 2.0)
- Notices: [`NOTICE`](NOTICE)
- Contributing: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Trademarks: [`TRADEMARKS.md`](TRADEMARKS.md)

## Core CRDs

- `Agent`
- `AgentSystem`
- `ModelEndpoint`
- `Tool`
- `Secret`
- `Memory`
- `AgentPolicy`
- `AgentRole`
- `ToolPermission`
- `Task`
- `TaskSchedule`
- `TaskWebhook`
- `Worker`

## Included in current scaffold

- API server CRUD for all thirteen CRDs
- Namespace-aware resources (`metadata.namespace`, default `default`)
- Kubernetes-style resource versioning fields:
  - `metadata.resourceVersion`
  - `metadata.generation`
  - `status.observedGeneration`
- Optimistic concurrency on `PUT` updates (requires `metadata.resourceVersion` or `If-Match`)
- `Agent` runtime + reconciliation controller
- Runtime execution contracts:
  - `ExecutionEngine` (agent loop orchestrator)
  - `ModelGateway` (model inference adapter)
  - `ToolRuntime` (tool invocation adapter)
  - `MemoryStore` (runtime working memory adapter)
  - provider-backed model gateway wiring for task execution (`mock` default, `openai` supported)
  - `ModelRouter` for per-agent endpoint routing via `Agent.spec.model_ref` -> namespaced `ModelEndpoint`
- Tool runtime hardening (Phase 6 foundation):
  - `Tool.spec` now supports capability + runtime policy metadata:
    - `capabilities[]`
    - `risk_level` (`low|medium|high|critical`)
    - `runtime.timeout`
    - `runtime.retry` (`max_attempts`, `backoff`, `max_backoff`, `jitter`)
    - `runtime.isolation_mode` (`none|sandboxed|container|wasm`)
  - runtime enforces per-tool timeout/retry with capped exponential delay + jitter
  - timeout/cancel semantics are explicit and deterministic across governed/container paths:
    - timeout -> `tool_code=timeout`, `tool_reason=tool_execution_timeout`, `retryable=true`
    - canceled -> `tool_code=canceled`, `tool_reason=tool_execution_canceled`, `retryable=false`
    - bounded shutdown behavior is enforced in runtime wrappers so misbehaving backends cannot block call completion indefinitely
  - strict tool runtime envelope is now implemented in code (`runtime/tool_contract.go`):
    - `ToolExecutionRequest` / `ToolExecutionResponse` / `ToolExecutionFailure`
    - `ToolContractExecutor` + `ExecuteToolContract(...)` bridge for legacy `ToolRuntime.Call(...)` backends
    - required request fields: `tool_contract_version` (defaults to `v1`), `request_id`, `tool.name`
  - agent worker tool calls now execute through the contract adapter (`tool_contract_version=v1`)
  - agent runtime executes model-selected tool calls (authorized subset) instead of auto-invoking every configured tool each step
  - unauthorized model-requested tools are denied fail-closed (`tool_permission_denied`)
  - OpenAI/Azure gateways support native chat `tool_calls` response parsing for model-directed tool selection
  - Anthropic/Ollama gateways support native tool-call parsing (`tool_use` / `tool_calls`) for model-directed tool selection
  - canonical tool error taxonomy is enforced across runtime paths:
    - stable metadata fields in errors/traces: `tool_status`, `tool_code`, `tool_reason`, `retryable`
    - canonical codes: `invalid_input`, `unsupported_tool`, `runtime_policy_invalid`, `isolation_unavailable`, `permission_denied`, `secret_resolution_failed`, `timeout`, `canceled`, `execution_failed`
    - canonical reasons: `tool_invalid_input`, `tool_unsupported`, `tool_runtime_policy_invalid`, `tool_isolation_unavailable`, `tool_permission_denied`, `tool_secret_resolution_failed`, `tool_execution_timeout`, `tool_execution_canceled`, `tool_backend_failure`
  - tool capability registry is loaded from declared `Tool` CRDs for each agent execution
  - strict unsupported-tool failures are surfaced when an execution references a tool with no resolved runtime policy
  - initial isolated executor path is wired with concrete backends:
    - high/critical-risk tools default to `runtime.isolation_mode: sandboxed`
    - real container-backed isolated runtime is available via `--tool-isolation-backend=container`
    - if no isolated executor backend is configured, runtime returns explicit isolation errors
    - `--tool-isolation-backend=wasm` supports a command-backed runtime path (default binary `wasmtime`)
    - wasm executor interfaces are defined in `runtime/tool_runtime_wasm_runtime.go` (`WASMToolExecutor`, `WASMToolRuntime`) with pluggable factories
    - default wasm command executor is defined in `runtime/tool_runtime_wasm_command_executor.go`
    - wasm guest modules now use a strict host<->guest JSON contract (`contract_version=v1`, `status=ok|error|denied`)
    - reference guest module and tool manifest are provided in `examples/tools/wasm-reference/`
    - current container backend supports `Tool.spec.type: http` (POST body = agent tool input)
    - `Tool.spec.auth.secretRef` is supported in container mode via namespaced Secret CRD resolution with env fallback:
      - secrets are namespace-scoped (`metadata.namespace`)
      - secret values are stored in `Secret.spec.data` as base64 (Kubernetes-style), not hex
      - resolver checks `<secretRef>`, normalized `<SECRET_REF>`, and `<prefix><SECRET_REF>`
      - default prefix is `ORLOJ_SECRET_` (configurable via `--tool-secret-env-prefix`)
    - unsupported tool types still fail with explicit errors
  - shared tool runtime conformance harness is available at `runtime/conformance/harness.go`
  - shared conformance case catalog helpers are available at `runtime/conformance/cases/catalog.go`
  - backend conformance suites are currently covered for governed and container runtimes in `runtime/tool_runtime_conformance_test.go`
  - isolated runtime backend registration hooks are available in code:
    - `runtime.RegisterToolIsolationBackend(...)`
    - `runtime.BuildToolIsolationRuntime(...)`
  - wasm runtime scaffold supports module/runtime options and executor factory boundary:
    - `WASMToolRuntimeConfig` (`module_path`, `entrypoint`, memory/fuel/WASI)
    - `WASMToolExecutorFactory` for pluggable executor wiring
- Queue-based controller reconciliation (per-resource keyed workqueues)
- Status reconciliation for `AgentSystem`, `ModelEndpoint`, `Tool`, `Memory`, `AgentPolicy`
- `Task` worker state machine with leasing/claiming (`Pending -> Running -> Succeeded|Failed|DeadLetter`)
- Central task scheduler loop in `orlojd`:
  - assigns pending tasks to workers via `Task.status.assignedWorker`
  - clears/reassigns when workers are stale/unavailable
  - respects `Task.spec.requirements` and worker capabilities
- Atomic task claiming and worker leases:
  - `Task.status.assignedWorker`
  - `Task.status.claimedBy`
  - `Task.status.leaseUntil`
  - `Task.status.lastHeartbeat`
  - `Task.status.history` (claim/retry/takeover/deadletter timeline)
  - `Task.status.trace` (structured agent execution events)
  - Postgres claim path uses `FOR UPDATE SKIP LOCKED`
- Worker lifecycle and readiness:
  - `Worker` heartbeat registration from `orlojworker`
  - `WorkerController` marks stale workers `NotReady`
  - atomic worker slot accounting (`Worker.status.currentTasks`) enforces `spec.max_concurrent_tasks`
- Task requirements-aware placement:
  - `Task.spec.requirements.region`
  - `Task.spec.requirements.gpu`
  - `Task.spec.requirements.model`
  - workers only claim tasks matching their declared capabilities
- Cross-resource validation for task execution:
  - `Task.spec.system` must reference an existing `AgentSystem`
  - `AgentSystem` graph must be valid (known agents, no cycles, entrypoint)
  - `AgentSystem.spec.graph` supports both legacy `next` and rich `edges[]` routes per node
  - graph node join config is accepted at `AgentSystem.spec.graph.<node>.join` (`mode`, `quorum_count`, `quorum_percent`, `on_failure`)
  - Referenced `Agent`, `ModelEndpoint` (`Agent.spec.model_ref`), `Tool`, and `Memory` resources must exist
- Task execution now runs the agent runtime in graph order and records per-agent execution output in `Task.status.output`
- Task trace now includes deterministic per-attempt step metadata:
  - `step_id` (stable per attempt, e.g. `a001.s0007`)
  - `attempt`
  - `step`
  - `tool` (when applicable)
  - `tool_contract_version`, `tool_request_id`, `tool_attempt` (when available)
  - `error_code`, `error_reason`, `retryable` (when available)
  - step-level runtime events (`model_call`, `tool_call`, `tool_error`, etc.)
- Inter-agent handoff messaging is recorded in:
  - `Task.status.messages` (`message_id`, `idempotency_key`, `task_id`, `attempt`, `system`, `from_agent`, `to_agent`, `branch_id`, `parent_branch_id`, `type`, `content`, `timestamp`, `trace_id`, `parent_id`)
  - per-message lifecycle fields: `phase`, `attempts`, `max_attempts`, `last_error`, `worker`, `processed_at`, `next_attempt_at`
  - `Task.status.trace` with `agent_message` events and `branch_id`
  - persistent replay guard state in `Task.status.message_idempotency`
  - join aggregation state in `Task.status.join_states`
- Runtime agent message bus (data-plane) is configurable for inter-agent handoffs:
  - backends: `none` (default), `memory`, `nats-jetstream`
  - worker inbox consumers are opt-in (`orlojworker --agent-message-consume`)
  - in `task-execution-mode=message-driven`, controller sends kickoff (`system -> first agent`) and workers execute/forward agent hops via runtime messages
  - cyclical handoffs are supported when `Task.spec.max_turns > 0`; branch forwarding stops at that turn limit
  - strict message execution ownership:
    - only the worker currently holding `Task.status.claimedBy` may start message attempts
    - non-owner workers requeue until lease expiry; expired leases are takeover-safe with `Task.status.history` `takeover` events
    - message-attempt processing renews `Task.status.leaseUntil` / `Task.status.lastHeartbeat`
  - consumer lifecycle emits `agent_message_received`, `agent_message_processed`, `agent_message_retry_scheduled`, `agent_message_deadletter`, and `agent_message_failed` trace events
  - retries are delayed via message bus requeue timing (`RetryAfter`) and use message-level policy `Task.spec.message_retry`:
    - `max_attempts`, `backoff`, `max_backoff`, `jitter` (`none|full|equal`), and `non_retryable` markers
    - capped exponential retry delay with jitter at message scope
    - non-retryable classification dead-letters immediately for policy errors and invalid system/agent/graph refs (plus configured markers)
    - compatibility fallback keeps older tasks/messages working when only `Task.spec.retry` is present
  - retries use message-level attempt counters; on exhaustion, message and task move to `DeadLetter`
  - supports fan-out forwarding to multiple downstream edges in one hop (`spec.graph.<node>.edges[]`)
  - supports fan-in gating with downstream join semantics (`wait_for_all` or `quorum`) before agent execution
  - kickoff supports multi-entry graphs (all zero-indegree roots are enqueued)
  - publish failures fail task execution (and then follow task retry/backoff policy)
  - envelope fields: `message_id`, `idempotency_key`, `task_id`, `attempt`, `system`, `namespace`, `from_agent`, `to_agent`, `branch_id`, `parent_branch_id`, `type`, `payload`, `timestamp`, `trace_id`, `parent_id`
  - routing compatibility supports both graph edge styles (`next` and `edges[].to`)
  - observability APIs:
    - `GET /v1/tasks/{name}/messages`
      - filters: `phase` (comma list: `queued,running,retrypending,succeeded,deadletter`), `from_agent`, `to_agent`, `branch_id`, `trace_id`, `limit`
      - response includes filtered message list plus lifecycle counters
    - `GET /v1/tasks/{name}/metrics`
      - supports the same filters as `/messages`
      - exposes totals and rollups for `per_agent` and `per_edge`:
      - `retry_count`, `deadletters`, lifecycle counts, in-flight counts, `latency_ms_avg`, `latency_ms_p95`
  - runtime graph diagnostics in `/ui/` topology execution mode:
    - edge overlays now show live per-edge stats: `in_flight`, `retries`, `deadletters`, `p95 latency`
    - topology edges are selectable (not just nodes) for runtime drill-down
    - timeline panel follows selected edge/node and renders ordered message/trace activity
    - governance visibility includes:
      - namespace inventory cards for `AgentRole` and `ToolPermission`
      - topology node governance detail (agent role bindings + tool permission rules)
      - denial reason chips in task/timeline views for permission/policy failures
- AgentPolicy enforcement during task execution:
  - `allowed_models` enforced per agent
  - `blocked_tools` enforced per agent
  - `max_tokens_per_run` enforced as run-level estimated token budget (minimum budget across active policies)
  - policy scoping supported via `apply_mode` + `target_systems`/`target_tasks`
    - `apply_mode: scoped` (default): policy applies only to matched targets
    - `apply_mode: global`: policy applies to all tasks
- Governance permissions layer (Phase 7):
  - `Agent.spec.roles[]` binds agents to namespaced `AgentRole` resources
  - `AgentRole.spec.permissions[]` grants tool/capability permissions
  - `ToolPermission` declares required permissions for tool action execution
  - tool calls fail closed when role refs are missing or required permissions are not granted
  - permission-denied tool execution hard-fails the task/message run (no silent partial success)
  - permission denials are surfaced as policy errors and treated as non-retryable
- Task retry/backoff supported via `Task.spec.retry`:
  - `max_attempts`: total run attempts before terminal failure
  - `backoff`: exponential backoff base duration (for retryable errors)
  - retryable examples: timeouts/transient connection failures
  - non-retryable examples: policy violations, token budget violations
- Message retry/backoff supported via `Task.spec.message_retry`:
  - `max_attempts`: per-message retry budget before dead-letter
  - `backoff` + `max_backoff`: capped exponential delay base and cap
  - `jitter`: `none`, `full`, or `equal`
  - `non_retryable`: configurable classification markers for immediate dead-letter
  - defaults/fallbacks inherit from `Task.spec.retry` for backward compatibility
- Graph join semantics:
  - configure on downstream node: `AgentSystem.spec.graph.<node>.join`
  - `mode`: `wait_for_all` (default) or `quorum`
  - `quorum_count` and `quorum_percent` are both supported for quorum thresholds
  - fan-in activation is persisted per-attempt in `Task.status.join_states`
- Pluggable state backend: in-memory (default) or Postgres persistence
- Watch stream endpoints (SSE) for incremental updates:
  - `/v1/agents/watch`
  - `/v1/tasks/watch`
- Event bus and live event stream:
  - in-memory event bus in `orlojd` (bounded replay buffer)
  - API emits `resource.created|updated|deleted|status` events
  - controllers emit lifecycle events (task assignment/claim/retry/succeeded/deadletter, worker ready/not_ready)
  - `/v1/events/watch` SSE endpoint with filters (`since`, `source`, `type`, `kind`, `name`, `namespace`)
- Standalone worker binary: `cmd/orlojworker`
- `orlojctl` CLI: `apply`, `get`, `logs`, `trace`, `graph`, `events`
- Built-in web console at `/ui/` for:
  - namespace-scoped resource inventory
  - system-centric operations view (search/filter AgentSystems, select a system, inspect related runs)
  - task detail drill-down (graph, output, trace, messages, history, logs)
  - message lifecycle visibility in run detail (`phase`, `attempts/max_attempts`, `worker`, `next_attempt_at`, `last_error`)
  - live control-plane event stream
  - live topology updates from `/v1/events/watch`, `/v1/tasks/watch`, and `/v1/agents/watch`
  - click-to-inspect topology object detail (`metadata`, `spec`, `status`, warning context)
  - topology mode switch: `Design` (declared wiring) vs `Execution` (runtime flow lane)
  - runtime message overlay on topology edges (from `Task.status.messages`)
  - per-edge runtime message metrics (`count`, approximate payload size, last-message time)
  - topology and DAG rendering support both legacy `next` and fan-out `edges[]` graph declarations
  - warning states for missing references (agent/tool/memory/worker)
- System topology view contract (provisional):
  - `AgentSystem` is the root node.
  - Root fans out to an `agents` group node.
  - `agents` group fans out to each `Agent`.
  - Each `Agent` fans out only to its own referenced `Tool` and `Memory`.
  - `AgentPolicy` attaches to `AgentSystem` as governance metadata (not runtime flow).
  - Agent flow edges are derived from `AgentSystem.spec.graph`.
  - Runtime lane (`Task`/`Worker`) is rendered separately from design-time topology.
- Phase-1 validation coverage:
  - end-to-end task lifecycle tests (`apply -> schedule -> claim/run -> retry/deadletter -> trace`)
  - failure injection tests (stale heartbeat reassignment, worker crash lease takeover)
  - API status contract tests for `Task.status` and `Worker.status` field names
  - Postgres-backed shared-state integration tests (enabled with `ORLOJ_POSTGRES_DSN`)
- Phase-9 load harness foundation:
  - `cmd/orloj-loadtest` for repeatable message-driven load scenarios
  - baseline scenario applies reporting manifests, checks worker readiness, and drives concurrent task creation
  - deterministic deadletter injection via invalid system routing (`--inject-invalid-system-rate`)
- Phase-9 monitoring/alerting foundation:
  - `cmd/orloj-alertcheck` evaluates retry/deadletter/latency/in-flight thresholds from live task metrics
  - default alert profile artifact: `monitoring/alerts/retry-deadletter-default.json`
  - dashboard contract artifact: `monitoring/dashboards/retry-deadletter-overview.json`
- Phase-9 failure-injection suite + quality gates:
  - `cmd/orloj-loadtest` now supports retry-stress and simulated expired-lease takeover injections
  - machine-readable JSON reports (`--json`) with quality-gate non-zero exit behavior
  - default load-test quality profile artifact: `monitoring/loadtest/quality-default.json`

## Run

Start the API/control plane:

```bash
go run ./cmd/orlojd
```

Open the web console:

```bash
open http://127.0.0.1:8080/ui/
```

Or in a browser: `http://127.0.0.1:8080/ui/`

React UI development scaffold (optional, incremental migration path):

```bash
cd frontend
npm install
npm run dev
```

- Vite dev server runs at `http://127.0.0.1:5173` and proxies `/v1/*` + `/healthz` to `http://127.0.0.1:8080`.
- Production build output is written to `frontend/dist`.

Build React assets for Go embedding:

```bash
cd frontend
npm run build
```

After rebuilding `orlojd`, `/ui/` serves assets from `frontend/dist`.
If `frontend/dist/index.html` is missing, `/ui/` returns `503` with a build-required message.

Start API/control plane with NATS-backed event bus:

```bash
go run ./cmd/orlojd \
  --event-bus-backend=nats \
  --nats-url=nats://127.0.0.1:4222
```

Start API/control plane with durable runtime inter-agent messaging (JetStream):

```bash
go run ./cmd/orlojd \
  --event-bus-backend=nats \
  --nats-url=nats://127.0.0.1:4222 \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=nats-jetstream \
  --agent-message-nats-url=nats://127.0.0.1:4222 \
  --agent-message-subject-prefix=orloj.agentmsg \
  --agent-message-stream-name=ORLOJ_AGENT_MESSAGES
```

Start API/control plane with Postgres-backed state:

```bash
export ORLOJ_POSTGRES_DSN='postgres://user:pass@127.0.0.1:5432/orloj?sslmode=disable'
go run ./cmd/orlojd \
  --storage-backend=postgres \
  --sql-driver=pgx
```

Start one or more task workers (recommended for distributed execution):

```bash
go run ./cmd/orlojworker \
  --storage-backend=postgres \
  --sql-driver=pgx \
  --task-execution-mode=message-driven \
  --worker-id=worker-a \
  --agent-message-bus-backend=nats-jetstream \
  --agent-message-consume

go run ./cmd/orlojworker \
  --storage-backend=postgres \
  --sql-driver=pgx \
  --task-execution-mode=message-driven \
  --worker-id=worker-b \
  --agent-message-bus-backend=nats-jetstream \
  --agent-message-consume
```

Enable real model inference for task execution (instead of the mock gateway):

```bash
export OPENAI_API_KEY='sk-...'
go run ./cmd/orlojworker \
  --storage-backend=postgres \
  --sql-driver=pgx \
  --task-execution-mode=message-driven \
  --worker-id=worker-a \
  --agent-message-bus-backend=nats-jetstream \
  --agent-message-consume \
  --model-gateway-provider=openai
```

Enable per-agent provider routing with `ModelEndpoint` + `Agent.spec.model_ref`:

```bash
go run ./cmd/orlojctl apply -f examples/model-endpoints/openai_default.yaml
go run ./cmd/orlojctl apply -f examples/agents/research_agent_model_ref.yaml
```

Anthropic `ModelEndpoint` + secret example:

```bash
go run ./cmd/orlojctl apply -f examples/model-endpoints/anthropic_default.yaml
go run ./cmd/orlojctl apply -f examples/secrets/anthropic_api_key.yaml
```

Azure OpenAI `ModelEndpoint` + secret example:

```bash
go run ./cmd/orlojctl apply -f examples/model-endpoints/azure_openai_default.yaml
go run ./cmd/orlojctl apply -f examples/secrets/azure_openai_api_key.yaml
```

Ollama `ModelEndpoint` example:

```bash
go run ./cmd/orlojctl apply -f examples/model-endpoints/ollama_default.yaml
```

In this mode:
- agents without `model_ref` use worker fallback gateway config (`--model-gateway-*`)
- agents with `model_ref` resolve the referenced namespaced `ModelEndpoint`
- `ModelEndpoint.spec.auth.secretRef` is resolved from namespaced `Secret` (with env fallback)
- additional providers can be added via runtime provider plugin registration (`runtime.RegisterModelProvider`)

Enable container-isolated tool execution (example):

```bash
go run ./cmd/orlojworker \
  --storage-backend=postgres \
  --sql-driver=pgx \
  --worker-id=worker-a \
  --tool-isolation-backend=container \
  --tool-container-runtime=docker \
  --tool-container-image=curlimages/curl:8.8.0 \
  --tool-container-network=none \
  --tool-secret-env-prefix=ORLOJ_SECRET_
```

Example secret env for `Tool.spec.auth.secretRef: search-api-key`:

```bash
export ORLOJ_SECRET_SEARCH_API_KEY='your-token'
```

The scheduler in `orlojd` assigns due pending tasks to ready workers. Workers enforce assignment and capacity before claiming.
For `task-execution-mode=message-driven`, use a non-`none` agent message bus backend and enable worker consumers (`--agent-message-consume`).

Start full stack with Docker Compose (`postgres` + `orlojd` + 2 workers):

```bash
docker compose up --build -d
docker compose ps
```

Stop the stack:

```bash
docker compose down
```

Optional: run an embedded worker inside `orlojd` (single-process mode):

```bash
go run ./cmd/orlojd \
  --run-task-worker \
  --task-worker-id=embedded-worker
```

`postgres` mode flags:

- `--storage-backend`: `memory` (default) or `postgres`
- `--postgres-dsn`: Postgres DSN (or set `ORLOJ_POSTGRES_DSN`)
- `--sql-driver`: `database/sql` driver name (default `pgx`)
- `--postgres-max-open-conns`
- `--postgres-max-idle-conns`
- `--postgres-conn-max-lifetime`
- `orlojd` worker flags:
  - `--run-task-worker`
  - `--task-worker-id`
  - `--task-lease-duration`
  - `--task-heartbeat-interval`
  - `--task-execution-mode` (`sequential` or `message-driven`)
  - model gateway flags:
    - `--model-gateway-provider` (`mock`, `openai`, `anthropic`, `azure-openai`, or `ollama`)
    - `--model-gateway-api-key`
    - `--model-gateway-base-url`
    - `--model-gateway-timeout`
    - `--model-gateway-default-model`
    - `--model-secret-env-prefix`
  - isolated tool runtime flags:
    - `--tool-isolation-backend` (`none`, `container`, or `wasm`)
    - `--tool-container-runtime`
    - `--tool-container-image`
    - `--tool-container-network`
    - `--tool-container-memory`
    - `--tool-container-cpus`
    - `--tool-container-pids-limit`
    - `--tool-container-user`
    - `--tool-secret-env-prefix`
    - `--tool-wasm-module`
    - `--tool-wasm-entrypoint`
    - `--tool-wasm-runtime-binary`
    - `--tool-wasm-runtime-args`
    - `--tool-wasm-memory-bytes`
    - `--tool-wasm-fuel`
    - `--tool-wasm-wasi`
- `orlojd` event bus flags:
  - `--event-bus-backend` (`memory` or `nats`)
  - `--nats-url`
  - `--nats-subject-prefix`
- `orlojd` runtime agent message bus flags:
  - `--agent-message-bus-backend` (`none`, `memory`, or `nats-jetstream`)
  - `--agent-message-nats-url`
  - `--agent-message-subject-prefix`
  - `--agent-message-stream-name`
  - `--agent-message-history-max`
  - `--agent-message-dedupe-window`
- `orlojworker` worker flags:
  - `--task-execution-mode` (`sequential` or `message-driven`)
  - model gateway flags:
    - `--model-gateway-provider` (`mock`, `openai`, `anthropic`, `azure-openai`, or `ollama`)
    - `--model-gateway-api-key`
    - `--model-gateway-base-url`
    - `--model-gateway-timeout`
    - `--model-gateway-default-model`
    - `--model-secret-env-prefix`
  - `--worker-id`
  - `--lease-duration`
  - `--heartbeat-interval`
  - `--reconcile-interval`
  - `--region`
  - `--gpu`
  - `--supported-models`
  - `--max-concurrent-tasks`
  - isolated tool runtime flags:
    - `--tool-isolation-backend` (`none`, `container`, or `wasm`)
    - `--tool-container-runtime`
    - `--tool-container-image`
    - `--tool-container-network`
    - `--tool-container-memory`
    - `--tool-container-cpus`
    - `--tool-container-pids-limit`
    - `--tool-container-user`
    - `--tool-secret-env-prefix`
    - `--tool-wasm-module`
    - `--tool-wasm-entrypoint`
    - `--tool-wasm-runtime-binary`
    - `--tool-wasm-runtime-args`
    - `--tool-wasm-memory-bytes`
    - `--tool-wasm-fuel`
    - `--tool-wasm-wasi`
- `orlojworker` runtime agent message bus flags:
  - `--agent-message-bus-backend` (`none`, `memory`, or `nats-jetstream`)
  - `--agent-message-nats-url`
  - `--agent-message-subject-prefix`
  - `--agent-message-stream-name`
  - `--agent-message-history-max`
  - `--agent-message-dedupe-window`
  - `--agent-message-consume` (opt-in inbox consumers)
  - `--agent-message-consumer-namespace` (optional namespace filter)
  - `--agent-message-consumer-refresh`
  - `--agent-message-consumer-dedupe-window`

Apply resources:

```bash
go run ./cmd/orlojctl apply -f examples/memories/research_memory.yaml
go run ./cmd/orlojctl apply -f examples/model-endpoints/openai_default.yaml
go run ./cmd/orlojctl apply -f examples/tools/web_search_tool.yaml
go run ./cmd/orlojctl apply -f examples/tools/vector_db_tool.yaml
go run ./cmd/orlojctl apply -f examples/secrets/search_api_key.yaml
go run ./cmd/orlojctl apply -f examples/secrets/openai_api_key.yaml
go run ./cmd/orlojctl apply -f examples/agents/planner_agent.yaml
go run ./cmd/orlojctl apply -f examples/agents/research_agent.yaml
go run ./cmd/orlojctl apply -f examples/agents/writer_agent.yaml
go run ./cmd/orlojctl apply -f examples/agent-systems/report_system.yaml
go run ./cmd/orlojctl apply -f examples/agent-policies/cost_policy.yaml
go run ./cmd/orlojctl apply -f examples/tasks/weekly_report_template_task.yaml
go run ./cmd/orlojctl apply -f examples/task-schedules/weekly_report_schedule.yaml
go run ./cmd/orlojctl apply -f examples/secrets/webhook_shared_secret.yaml
go run ./cmd/orlojctl apply -f examples/task-webhooks/github_push_webhook.yaml
```

For additional governance-focused scenarios (denied/allowed runs), see [`examples/README.md`](examples/README.md).

List resources:

```bash
go run ./cmd/orlojctl get agents
go run ./cmd/orlojctl get agent-systems
go run ./cmd/orlojctl get model-endpoints
go run ./cmd/orlojctl get tools
go run ./cmd/orlojctl get memories
go run ./cmd/orlojctl get agent-policies
go run ./cmd/orlojctl get agent-roles
go run ./cmd/orlojctl get tool-permissions
go run ./cmd/orlojctl get tasks
go run ./cmd/orlojctl get task-schedules
go run ./cmd/orlojctl get task-webhooks
go run ./cmd/orlojctl get workers
go run ./cmd/orlojctl get -w tasks
```

Read agent runtime logs:

```bash
go run ./cmd/orlojctl logs research-agent
go run ./cmd/orlojctl logs task/weekly-report
go run ./cmd/orlojctl trace task weekly-report
go run ./cmd/orlojctl graph system report-system
go run ./cmd/orlojctl graph task weekly-report
go run ./cmd/orlojctl events
go run ./cmd/orlojctl events --kind=Task --name=weekly-report --source=controller --type=task.succeeded
go run ./cmd/orlojctl events --kind=Task --namespace=default --type=task.succeeded --once --timeout=30s
```

Run the load harness:

```bash
go run ./cmd/orloj-loadtest \
  --base-url=http://127.0.0.1:8080 \
  --namespace=default \
  --tasks=200 \
  --create-concurrency=25 \
  --poll-concurrency=50 \
  --run-timeout=10m \
  --quality-profile=monitoring/loadtest/quality-default.json
```

Run retry-stress + takeover failure injection:

```bash
go run ./cmd/orloj-loadtest \
  --tasks=200 \
  --inject-timeout-system-rate=0.20 \
  --inject-expired-lease-rate=0.15 \
  --min-retry-total=50 \
  --min-takeover-events=20 \
  --json=true
```

Run alert checks for retry storm/deadletter growth:

```bash
go run ./cmd/orloj-alertcheck \
  --base-url=http://127.0.0.1:8080 \
  --namespace=default \
  --profile=monitoring/alerts/retry-deadletter-default.json \
  --json=true
```

Run Postgres integration tests (optional):

```bash
export ORLOJ_POSTGRES_DSN='postgres://user:pass@127.0.0.1:5432/orloj?sslmode=disable'
go test ./api ./controllers -run Postgres
```

Run NATS integration tests (optional):

```bash
export ORLOJ_NATS_URL='nats://127.0.0.1:4222'
go test ./eventbus -run NATS
```

Watch live control-plane events:

```bash
curl -N "http://127.0.0.1:8080/v1/events/watch"
curl -N "http://127.0.0.1:8080/v1/events/watch?source=apiserver&kind=Task"
curl -N "http://127.0.0.1:8080/v1/tasks?namespace=team-a"
curl -N "http://127.0.0.1:8080/v1/tasks/weekly-report?namespace=team-a"
go run ./cmd/orlojctl events --source=apiserver --kind=Task --namespace=team-a
```

## API Endpoints

- `GET /healthz`
- `GET /v1/capabilities`
- `GET /ui` (redirects to `/ui/`)
- `GET /ui/*` (static web console assets)
- `GET|POST /v1/agents`
- `GET|PUT|DELETE /v1/agents/{name}`
- `GET /v1/agents/{name}/logs`
- `GET|PUT /v1/agents/{name}/status`
- `GET /v1/agents/watch`
- `GET|POST /v1/agent-systems`
- `GET|PUT|DELETE /v1/agent-systems/{name}`
- `GET|PUT /v1/agent-systems/{name}/status`
- `GET|POST /v1/model-endpoints`
- `GET|PUT|DELETE /v1/model-endpoints/{name}`
- `GET|PUT /v1/model-endpoints/{name}/status`
- `GET|POST /v1/tools`
- `GET|PUT|DELETE /v1/tools/{name}`
- `GET|PUT /v1/tools/{name}/status`
- `GET|POST /v1/secrets`
- `GET|PUT|DELETE /v1/secrets/{name}`
- `GET|POST /v1/memories`
- `GET|PUT|DELETE /v1/memories/{name}`
- `GET|PUT /v1/memories/{name}/status`
- `GET|POST /v1/agent-policies`
- `GET|PUT|DELETE /v1/agent-policies/{name}`
- `GET|PUT /v1/agent-policies/{name}/status`
- `GET|POST /v1/agent-roles`
- `GET|PUT|DELETE /v1/agent-roles/{name}`
- `GET|POST /v1/tool-permissions`
- `GET|PUT|DELETE /v1/tool-permissions/{name}`
- `GET|POST /v1/tasks`
- `GET|PUT|DELETE /v1/tasks/{name}`
- `GET /v1/tasks/{name}/logs`
- `GET|PUT /v1/tasks/{name}/status`
- `GET /v1/tasks/watch`
- `GET|POST /v1/task-schedules`
- `GET|PUT|DELETE /v1/task-schedules/{name}`
- `GET|PUT /v1/task-schedules/{name}/status`
- `GET /v1/task-schedules/watch`
- `GET|POST /v1/task-webhooks`
- `GET|PUT|DELETE /v1/task-webhooks/{name}`
- `GET|PUT /v1/task-webhooks/{name}/status`
- `GET /v1/task-webhooks/watch`
- `POST /v1/webhook-deliveries/{endpoint_id}` (signature-authenticated webhook ingress)
- `GET /v1/events/watch`
- `GET|POST /v1/workers`
- `GET|PUT|DELETE /v1/workers/{name}`
- `GET|PUT /v1/workers/{name}/status`

## Update Semantics

- `POST` upserts desired `spec` and preserves existing `status`.
- `PUT /v1/{resource}/{name}` updates desired `spec` only and requires:
  - `metadata.resourceVersion` in body, or
  - `If-Match: <resourceVersion>` header
- `PUT /status` updates only `status` and also requires resourceVersion preconditions.
- On stale version updates, the API returns `409 Conflict`.
- By-name reads/writes support namespace scoping via `?namespace=<ns>` (defaults to `default`).
- CRD metadata supports labels via `metadata.labels` for grouping and UI filtering.
- List endpoints support exact-match label filtering via `?labelSelector=key=value[,key2=value2]`.

## Notes

- Default state backend is in-memory.
- Postgres persistence is supported via `database/sql` and auto-creates schema on startup.
- `cmd/orlojd` links the `pgx` SQL driver by default (`--sql-driver=pgx`).
- Web console source and embed handler are in `/frontend` and served by the API at `/ui/`.
- React UI lives in `/frontend` (Vite + TypeScript); `npm run build` outputs to `/frontend/dist`.
- Web UI assets are embedded in the `orlojd` binary at build time; restart/rebuild after UI code changes.
- `/ui/` serves `/frontend/dist` only; if unbuilt, `/ui/` returns `503` with a build-required message.
- Event bus env vars:
  - `ORLOJ_EVENT_BUS_BACKEND` (`memory` or `nats`)
  - `ORLOJ_NATS_URL`
  - `ORLOJ_NATS_SUBJECT_PREFIX`
- Runtime agent message bus env vars:
  - `ORLOJ_AGENT_MESSAGE_BUS_BACKEND` (`none`, `memory`, or `nats-jetstream`)
  - `ORLOJ_AGENT_MESSAGE_NATS_URL`
  - `ORLOJ_AGENT_MESSAGE_SUBJECT_PREFIX`
  - `ORLOJ_AGENT_MESSAGE_STREAM`
  - `ORLOJ_AGENT_MESSAGE_CONSUME` (`true|false`)
  - `ORLOJ_AGENT_MESSAGE_CONSUMER_NAMESPACE`
- Task runtime mode env vars:
  - `ORLOJ_TASK_EXECUTION_MODE` (`sequential` or `message-driven`)
- Model gateway env vars:
  - `ORLOJ_MODEL_GATEWAY_PROVIDER` (`mock`, `openai`, `anthropic`, `azure-openai`, or `ollama`)
  - `ORLOJ_MODEL_GATEWAY_API_KEY`
  - `ORLOJ_MODEL_GATEWAY_BASE_URL`
  - `ORLOJ_MODEL_GATEWAY_TIMEOUT`
  - `ORLOJ_MODEL_GATEWAY_DEFAULT_MODEL`
  - `ORLOJ_MODEL_SECRET_ENV_PREFIX`
  - if `ORLOJ_MODEL_GATEWAY_API_KEY` is empty, provider-specific defaults apply (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `AZURE_OPENAI_API_KEY`)
  - if `ORLOJ_MODEL_GATEWAY_BASE_URL` or `ORLOJ_MODEL_GATEWAY_DEFAULT_MODEL` are empty, provider defaults are used
- API auth/RBAC can be enabled with:
  - `ORLOJ_API_TOKEN=<token>` (single admin token), or
  - `ORLOJ_API_TOKENS=<token:role,...>` where roles are `reader`, `writer`, `controller`, `admin`
- When auth is enabled:
  - read endpoints require at least `reader`
  - spec write endpoints require `writer`
  - `/status` write endpoints require `controller`
  - `/v1/webhook-deliveries/{endpoint_id}` remains bearer-exempt and is authenticated by webhook signature headers
- Multi-process worker execution requires a shared backend (Postgres recommended).
- Task execution assignment model:
  - scheduler writes `Task.status.assignedWorker`
  - workers claim compatible assigned pending tasks (or unassigned tasks in single-process fallback)
  - running tasks can fail over on lease expiry even if previously assigned to another worker
  - worker slot capacity is enforced via `Worker.spec.max_concurrent_tasks` and `Worker.status.currentTasks`
- Phase-2 event-driven progress:
  - task, scheduler, and worker controllers now wake early on API task/worker events (polling loop remains as fallback)
  - pluggable event bus backend (`memory` default, `nats` optional) is available in `orlojd`
- Resource watch endpoints (`/v1/agents/watch`, `/v1/tasks/watch`, `/v1/task-schedules/watch`, `/v1/task-webhooks/watch`) are polling-backed SSE streams suitable for MVP usage.
- Container files:
  - `Dockerfile` supports building `orlojd` or `orlojworker` via `BINARY` build arg.
  - `docker-compose.yml` runs Postgres, API server, and two workers by default.
