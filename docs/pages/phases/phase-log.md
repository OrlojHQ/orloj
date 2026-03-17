# Phase Log

## Phase 1-2 Foundations

- server scaffolding and resource CRUD/status contracts
- scheduler + worker leasing model
- event/watch infrastructure and web console baseline

## Phase 3

- graph routing expansion (`edges[]`) for richer per-edge behavior
- branch-aware task trace/message routing metadata

## Phase 4

- strict message execution ownership tied to task lease holder
- durable idempotency tracking for crash-safe replay
- hardened message retry policy (capped exponential + jitter + non-retryable classification)

## Phase 5

- task message lifecycle APIs and filtered metrics
- per-agent/per-edge runtime metrics (retry/deadletter/latency)
- UI runtime graph diagnostics and timeline panel improvements

## Phase 6

- tool capability/risk/runtime policy enforcement
- isolated tool execution backend path (container runtime)
- namespaced Secret resource + `secretRef` resolution for tool auth

## Phase 7

- `AgentRole` + `ToolPermission` resources for permission-as-code governance
- `Agent.spec.roles[]` binding model for agent identity/authorization
- runtime tool call authorization hooks with fail-closed denials
- permission denial classification as policy/non-retryable errors
- UI governance diagnostics (role/tool-permission visibility and denial chips in timeline)
- permission-denied tool calls now hard-fail task/message execution paths

## Phase 8

- provider-configurable task model gateway (`mock` or `openai`)
- real OpenAI-compatible model calls for sequential and message-driven execution paths
- startup flags/env support for gateway provider, key, base URL, timeout, and default model

## Phase 8.1

- `ModelEndpoint` resource introduced for namespaced provider routing
- `Agent.spec.model_ref` binding for per-agent model provider selection
- runtime `ModelRouter` added (`model_ref` -> endpoint -> provider gateway)
- endpoint auth via namespaced `Secret` (`auth.secretRef`) with env-prefix fallback

## Phase 8.2

- provider architecture expanded to support `anthropic` alongside `openai` and `mock`
- `ModelEndpoint.spec.options` introduced for provider-specific runtime settings
- `ModelRouter` switched to shared factory path for scalable provider onboarding
- provider aliases (`openai-compatible`) and option validation (`anthropic max_tokens`) added

## Phase 8.3

- provider plugin registry added (`RegisterModelProvider`) to decouple provider onboarding from core runtime edits
- built-in providers (`mock`, `openai`, `anthropic`) moved into registry-backed plugins
- `ModelEndpoint.spec.provider` now accepts custom provider ids for registered plugins
- router/provider key requirements now resolve from plugin metadata rather than hardcoded switches

## Phase 8.4

- added enterprise/provider plugins: `azure-openai` and `ollama`
- added runtime gateways/tests for Azure OpenAI and Ollama chat APIs
- worker/server provider flags and env key fallback updated for Azure OpenAI
- added examples/secrets for `azure-openai` and `ollama` model endpoints

## Forward Roadmap

- migrate model provider plugins from in-process registration to external runtime provider plugins (for independent deploy/version/isolation)
- detailed phase sequencing (including dedicated Tool Platform stream) is tracked in `docs/pages/phases/roadmap.md`

## Phase 9.1

- added initial load harness command: `cmd/orloj-loadtest`
- baseline scenario applies reporting manifests, verifies ready workers, and runs concurrent task load
- added deterministic failure-injection mode (`inject-invalid-system-rate`) for deadletter-path validation
- added load testing operations doc: `docs/pages/operations/load-testing.md`

## Phase 9.2

- added retry/deadletter alert check command: `cmd/orloj-alertcheck`
- added default alert profile: `monitoring/alerts/retry-deadletter-default.json`
- added dashboard contract artifact: `monitoring/dashboards/retry-deadletter-overview.json`
- added monitoring operations doc: `docs/pages/operations/monitoring-alerts.md`

## Phase 9.3

- expanded `cmd/orloj-loadtest` with failure-injection scenarios:
  - invalid-system deadletter injection
  - retry-stress timeout-system injection
  - simulated worker-crash/expired-lease takeover injection
- added machine-readable run reports (`--json`) and quality-gate exit behavior (`exit 2` on gate violations)
- added profile-backed quality gate support (`--quality-profile`) plus default profile artifact:
  - `monitoring/loadtest/quality-default.json`
- added timeout scenario examples:
  - `examples/agents/loadtest_timeout_agent.yaml`
  - `examples/agent-systems/loadtest_timeout_system.yaml`
- updated load-testing operations docs with new scenarios and commands

## Tool Platform 1.1

- added canonical tool runtime error envelope in code (`runtime/tool_error.go`):
  - normalized fields: `tool_status`, `tool_code`, `tool_reason`, `retryable`
  - deterministic status/code/reason taxonomy for tooling, policy, and UI parsing
- governance/tool runtime paths now emit typed denial/error metadata instead of free-form strings
- task/message runtime step traces now propagate tool error metadata:
  - `error_code`, `error_reason`, `retryable`
- message retry classification now consumes typed tool error metadata directly
- UI denial chips now recognize taxonomy markers (`tool_code`, `tool_reason`) in trace/message errors
- updated tool contract and conformance docs to lock taxonomy and observability expectations

## Tool Platform 1.2

- implemented strict tool contract runtime schema in code (`runtime/tool_contract.go`):
  - `ToolExecutionRequest` / `ToolExecutionResponse` / `ToolExecutionFailure`
  - `ToolContractExecutor` adapter for legacy `ToolRuntime.Call(...)` backends
  - request validation (`tool_contract_version`, `request_id`, `tool.name`) and deterministic error mapping
- wired agent execution loop through the contract adapter so tool calls always execute with `tool_contract_version=v1`
- added `runtime/conformance/harness.go` for reusable contract checks (`status/code/reason/retryable` envelope validation)
- added contract + conformance tests:
  - `runtime/tool_contract_test.go`
  - `runtime/conformance/harness_test.go`

## Tool Platform 1.3

- added backend conformance suites using the shared harness:
  - governed runtime contract suite (`TestGovernedToolRuntimeConformanceSuite`)
  - container runtime contract suite (`TestContainerToolRuntimeConformanceSuite`)
- propagated tool contract correlation metadata through runtime step parsing and task trace:
  - `tool_contract_version`
  - `tool_request_id`
  - `tool_attempt`
- agent worker now emits these correlation fields on tool success/error events for both sequential and message-driven traces

## Tool Platform 2.1

- hardened timeout/cancel semantics across runtime backends:
  - governed runtime now uses bounded call wrappers so timeout/cancel return promptly even when a backend ignores context cancellation
  - container runtime now maps context deadline/cancel to canonical tool taxonomy (`timeout` / `canceled`) instead of generic execution failures
- added bounded-shutdown conformance coverage:
  - harness supports per-case timeout, immediate cancel, and max-latency assertions
  - backend suites include timeout/cancel latency cases for governed and container runtimes
- added explicit wasm isolation backend stub:
  - `--tool-isolation-backend=wasm` now resolves to a fail-closed runtime (`isolation_unavailable`) until real wasm execution is implemented

## Tool Platform 2.2

- added conformance case catalog helpers at `runtime/conformance/cases/catalog.go`:
  - shared base request builder for contract tests
  - reusable unknown-version, immediate-cancel, and bounded-timeout case constructors
- updated backend conformance suites to consume case-catalog helpers and include unknown-version checks for governed/container/wasm coverage
- added wasm runtime scaffold interfaces in `runtime/tool_runtime_wasm_runtime.go`:
  - `WASMToolExecutor` contract (`Execute(ctx, req)`)
  - `WASMToolRuntime` wrapper with registry + namespace scoping support
  - deterministic timeout/cancel/backend-failure mapping using canonical tool error taxonomy
- wired `--tool-isolation-backend=wasm` to the scaffold runtime path in `orlojd`/`orlojworker` (still fail-closed until a real executor is configured)

## Tool Platform 2.3

- added isolated runtime backend registration hooks:
  - `runtime.RegisterToolIsolationBackend(mode, factory)`
  - `runtime.BuildToolIsolationRuntime(options)`
  - default registry now resolves `none`, `container`, and `wasm` without core switch logic in binaries
- refactored `orlojd` and `orlojworker` isolated-runtime wiring to use registry-based backend resolution
- expanded wasm runtime adapter boundary:
  - `WASMToolRuntimeConfig` with module/runtime options (`module_path`, `entrypoint`, memory/fuel/WASI)
  - `WASMToolExecutorFactory` for pluggable executor construction
  - lazy factory-based executor initialization with deterministic runtime-policy errors when misconfigured
- added conformance/runtime tests for registry hooks and wasm factory/config behavior

## Tool Platform 2.4

- added concrete command-backed wasm executor implementation:
  - `runtime/tool_runtime_wasm_command_executor.go`
  - `WASMCommandExecutorFactory` + `WASMCommandRunner` boundary
  - default runtime binary `wasmtime` with configurable args/entrypoint/module
- wired `orlojd` and `orlojworker` wasm backend flags for command execution tuning:
  - `--tool-wasm-runtime-binary`
  - `--tool-wasm-runtime-args`
  - existing wasm flags (`module`, `entrypoint`, `memory`, `fuel`, `wasi`) now flow through runtime config
- integrated wasm backend creation through registry options with command executor factory injection
- added runtime tests for command executor behavior and deterministic error mapping:
  - runtime policy invalid when module path/runtime binary are missing
  - missing runtime binary on host maps to non-retryable runtime policy invalid
  - request payload/args/env propagation validated through runner capture tests

## Tool Platform 2.5

- introduced strict wasm host/guest contract layer:
  - `runtime/tool_runtime_wasm_contract.go`
  - request envelope: `contract_version`, tool context, runtime limits
  - response envelope: `contract_version`, `status`, `output`, `error`
- command-backed wasm executor now requires/validates contract responses instead of loose stdout parsing
- contract violations now classify as deterministic non-retryable runtime policy errors
- module-declared denied/error responses now map directly into canonical tool error taxonomy
- added wasm contract tests:
  - `runtime/tool_runtime_wasm_contract_test.go`
  - expanded `runtime/tool_runtime_wasm_command_executor_test.go`
- added runnable reference wasm module artifacts:
  - `examples/tools/wasm-reference/echo_guest.wat`
  - `examples/tools/wasm-reference/wasm_echo_tool.yaml`
  - `examples/tools/wasm-reference/README.md`
- added wasm contract reference doc:
  - `docs/pages/reference/wasm-tool-module-contract-v1.md`

## Tool Platform 3.1

- switched agent execution from "run every configured tool each step" to model-selected tool invocation:
  - runtime now executes only requested tool calls from model responses
  - unrequested tools are not auto-invoked
- added authorized tool-call selection/validation layer:
  - `runtime/model_tool_calls.go`
  - validates model-requested tools against `Agent.spec.tools[]`
  - unauthorized selections fail closed as permission denials
- model request context now includes task/message runtime input fields for better tool-choice grounding
- OpenAI-compatible gateways now support native tool-call paths:
  - request includes chat `tools` definitions (`tool_choice=auto`) when tools are available
  - response parses `message.tool_calls` into runtime `ModelResponse.ToolCalls`
  - implemented for both `openai` and `azure-openai` gateways
- mock gateway now emits deterministic model-selected tool calls (step/context aware) for local testing
- added tests for model-directed tool selection and gateway tool-call parsing:
  - `runtime/model_tool_calls_test.go`
  - expanded `runtime/model_gateway_openai_test.go`
  - expanded `runtime/model_gateway_azure_openai_test.go`

## Tool Platform 3.2

- added native structured tool-call support for Anthropic and Ollama gateways:
  - Anthropic request now includes `tools[]` when candidate tools exist
  - Anthropic response `content[].type=tool_use` is parsed into runtime `ModelResponse.ToolCalls`
  - Ollama request now includes `tools[]` when candidate tools exist
  - Ollama response `message.tool_calls[]` is parsed into runtime `ModelResponse.ToolCalls`
- runtime model-selected tool execution path now has native structured coverage across:
  - `openai`
  - `azure-openai`
  - `anthropic`
  - `ollama`
- added gateway tool-call parsing tests:
  - expanded `runtime/model_gateway_anthropic_test.go`
  - expanded `runtime/model_gateway_ollama_test.go`

## OSS Launch Readiness

- added CI workflow (`.github/workflows/ci.yml`): Go build, test, vet, frontend build on push/PR
- implemented secret/auth redaction in log and trace paths (`runtime/redact.go`, `runtime/redact_test.go`)
- wired redaction into task controller and container tool runtime
- added `Stability: beta` lifecycle labels to API reference, resource reference, and extensions docs
- added `CODE_OF_CONDUCT.md` (Contributor Covenant v2.1)
- linked `CONTRIBUTING.md` and `CODE_OF_CONDUCT.md` from `docs/pages/project/index.md`
- updated `docs/pages/operations/security.md` to mark approval hooks as post-launch (Phase 12)
- added `build` target to Makefile (`go build ./cmd/...`)

## OSS Rebrand and Documentation

- replaced Kubernetes-specific vocabulary across all docs (CRDs -> resources, control plane -> server, controllers -> services)
- rewrote README for OSS launch (concise value prop, quickstart, architecture)
- restructured onboarding: sequential mode first, blueprint-first quickstart
- documented secrets story (dev vs production, env-var resolver chain)
- added two-tier execution mode docs (sequential for dev, message-driven for prod)
- restructured docs to professional standard with Vocs site publishing
- added deployment docs and assets for local, VPS, and Kubernetes targets
- added VPS docker-compose (`docs/deploy/vps/docker-compose.vps.yml`) and systemd service

## Platform Improvements

- added `--api-key` flag and `--embedded-worker` alias to `orlojd`
- added database migration system (`store/migrate.go` + embedded SQL files)
- added secret encryption at rest (`store/secret_crypto.go`): AES-256-GCM encryption for `Secret.spec.data` values, enabled via `--secret-encryption-key` flag on both `orlojd` and `orlojworker`
- added `spec.allowed_tools` on Agent for simplified governance without full RBAC
- added `orlojctl run` command for imperative task execution
- added `orlojctl init` command for project scaffolding
- extracted shared startup package (`startup/`) to deduplicate orlojd and orlojworker initialization
- updated `.gitignore` for current binary names, switched to bun lockfile

## Observability

- added trace waterfall/timeline UI component (`frontend/src/components/TraceView.tsx`) wired into TaskDetail page
- integrated OpenTelemetry SDK with OTLP gRPC exporter (`telemetry/otel.go`)
- added span helpers for task execution, agent steps, and message processing (`telemetry/spans.go`)
- wired OTel initialization into `orlojd` and `orlojworker` startup
- instrumented task controller and agent message consumer with OTel spans
- added Prometheus client with `/metrics` endpoint (`telemetry/metrics.go`), auth-exempt
- instrumented task/agent/message paths with Prometheus counters and histograms
- added structured logging setup (`telemetry/logging.go`)
- added observability operations doc (`docs/pages/operations/observability.md`)
- added enterprise observability boundary doc (`docs/pages/boundaries/observability-enterprise.BOUNDARY.md`)

## Tool Platform 2.6

- replaced `MockToolClient` with `HTTPToolClient` as the base runtime for `isolation_mode=none`:
  - `runtime/tool_runtime_http.go`
  - real HTTP POST to `Tool.spec.endpoint` with `ToolExecutionResponse` parsing
  - auth injection via `Authorization: Bearer` header from secret-resolved `Tool.spec.auth.secretRef`
  - HTTP status code mapping to canonical tool error taxonomy (429/5xx retryable, 4xx non-retryable)
  - contract-aware response parsing: detects `ToolExecutionResponse` envelopes vs raw output
- added `Tool.spec.type` validation at apply time in `resources/resource_types.go`:
  - allowed values: `http`, `external`, `grpc`, `queue`, `webhook-callback`
  - unknown types rejected with deterministic validation error
- added external tool executor runtime (`runtime/tool_runtime_external.go`):
  - `Tool.spec.type=external` delegates `ToolExecutionRequest` JSON to `Tool.spec.endpoint` via HTTP POST
  - strict contract response parsing: rejects non-JSON responses
  - registered in default isolation backend registry
- added gRPC tool adapter (`runtime/tool_runtime_grpc.go`):
  - `Tool.spec.type=grpc` sends `ToolExecutionRequest` via generic unary gRPC call
  - uses JSON codec over `orloj.tool.v1.ToolService/Execute` method path
  - registered in default isolation backend registry
- added webhook-callback async adapter (`runtime/tool_runtime_webhook_callback.go`):
  - `Tool.spec.type=webhook-callback` fires request, then polls `{endpoint}/{request_id}` for result
  - supports push-based callback delivery via `DeliverCallback` API
  - 202 Accepted triggers async polling; 200 returns immediate result
  - registered in default isolation backend registry
- documented and enforced sandbox defaults:
  - `SandboxedContainerDefaults()` function codifies secure-by-default settings
  - container runtime enforces `--read-only`, `--cap-drop=ALL`, `--security-opt no-new-privileges`, `--network none`, non-root user, memory/CPU/pids limits
  - added conformance test for sandboxed default assertions
- added conformance suites for new backends:
  - `TestHTTPToolRuntimeConformanceSuite`
  - `TestExternalToolRuntimeConformanceSuite`
- all new adapters registered via `runtime.RegisterToolIsolationBackend` in default registry

### Tool Platform 3: Tool Auth and Secret Binding

- expanded `ToolAuth` resource to support four auth profiles:
  - `bearer` (default, backward-compatible)
  - `api_key_header` with custom header name
  - `basic` with base64-encoded `username:password`
  - `oauth2_client_credentials` with token exchange, caching, and auto-refresh
- added `Tool.Normalize()` validation for auth fields (profile, headerName, tokenURL, scopes)
- updated manifest parser to handle new auth fields (`profile`, `headerName`/`header_name`, `tokenURL`/`token_url`, `scopes`)
- created centralized `AuthInjector` (`runtime/tool_auth_injector.go`):
  - `Resolve(ctx, toolName, auth)` returns `AuthResult{Headers, EnvVars, Profile}`
  - all four profiles implemented with proper error mapping
- implemented OAuth2 client credentials flow (`runtime/tool_auth_oauth2.go`):
  - token exchange via `grant_type=client_credentials`
  - in-memory cache keyed by `tokenURL+clientID` with TTL from `expires_in`
  - `Evict()` for cache invalidation on 401 responses
- added auth failure classification to error taxonomy:
  - `auth_invalid` / `tool_auth_invalid` for HTTP 401 and gRPC `Unauthenticated`
  - `auth_forbidden` / `tool_auth_forbidden` for HTTP 403 and gRPC `PermissionDenied`
  - `auth_expired` / `tool_auth_expired` for expired OAuth2 tokens
- added auth audit fields to traces:
  - `tool_auth_profile` and `tool_auth_secret_ref` on `TaskTraceEvent` and `AgentStepEvent`
  - propagation through `agent_message_consumer.go` and `task_controller.go`
- updated all six backends to use `AuthInjector`:
  - `HTTPToolClient`: replaced inline bearer logic with `AuthInjector.Resolve`
  - `ExternalToolRuntime`: same pattern
  - `GRPCToolRuntime`: resolves auth, propagates profile to contract request
  - `WebhookCallbackToolRuntime`: refactored from string auth header to map-based headers
  - `ContainerToolRuntime`: generalized env var injection for all profiles
  - `WASMToolRuntime`: added `WASMToolModuleReqAuth` struct with profile and headers
- documented rotation semantics:
  - secret resolution is fresh per invocation (no caching of raw secrets)
  - OAuth2 access tokens cached with TTL, evicted on expiry or 401
- comprehensive test coverage:
  - `TestAuthInjector*`: all four profiles, error cases, edge cases (14 tests)
  - `TestOAuth2TokenCache*`: caching, eviction, expiry, errors (5 tests)
  - `TestHTTPToolClientMaps401ToAuthInvalid`, `TestHTTPToolClientMaps403ToAuthForbidden`
  - `TestToolNormalizeAuth*`: resource validation for profiles, constraints, scopes (7 tests)
- updated user-facing documentation:
  - `build-custom-tool.md`: examples for all four auth profiles
  - `resources.md`: full auth field reference and validation rules
  - `security.md`: auth profile guidance, container auth, error handling, audit trail
  - `tool-contract-v1.md`: expanded auth binding section with rotation semantics
  - `tools-and-isolation.md`: auth profiles section with rotation details

## Tool Platform 4: Policy Hooks and Risk-Tier Routing

- added `OperationClasses` field to `ToolSpec` in `resources/resource_types.go`:
  - allowed values: `read`, `write`, `delete`, `admin`
  - normalization: trim, lowercase, deduplicate
  - validation: rejects unknown values
  - defaults: `["read"]` for low/medium risk, `["write"]` for high/critical risk
  - manifest parser support in `resources/manifest_parser_ext.go`
- added `OperationRule` struct and `OperationRules` to `ToolPermissionSpec`:
  - each rule has `operation_class` (or `*` wildcard) and `verdict` (`allow`/`deny`/`approval_required`)
  - normalization: trim, lowercase, validates both fields
  - manifest parser support for constrained YAML
- refactored `ToolCallAuthorizer` interface to tri-state:
  - `Authorize()` now returns `(*AuthorizeResult, error)` instead of `error`
  - `AuthorizeResult.Verdict`: `allow`, `deny`, `approval_required`
  - `AgentToolAuthorizer` evaluates `OperationRules` when present, computes aggregate verdict
  - most-restrictive-wins: `deny` > `approval_required` > `allow`
  - backward compatible: no operation rules = original binary allow/deny behavior
- added `ToolApproval` resource in `resources/resource_types.go`:
  - `Spec`: `task_ref`, `tool`, `operation_class`, `agent`, `input`, `reason`, `ttl`
  - `Status`: `phase` (Pending/Approved/Denied/Expired), `decision`, `decided_by`, `decided_at`, `expires_at`
  - `Normalize()` with validation, TTL defaulting (10m), and auto-computed `expires_at`
  - `ToolApprovalStore` with in-memory and Postgres backends
  - SQL migration: `store/migrations/002_tool_approvals.up.sql`
  - API endpoints: CRUD + `POST .../approve` and `POST .../deny`
- added `WaitingApproval` task phase:
  - `ErrToolApprovalRequired` sentinel error in governed runtime
  - task controller transitions Running -> WaitingApproval on approval required
  - `reconcileWaitingApproval`: checks ToolApproval status (Approved -> Running, Denied -> Failed, Expired -> Failed)
  - wired `ToolApprovalStore` into task controller via `SetToolApprovalStore`
- added approval error taxonomy:
  - `approval_pending`, `approval_denied`, `approval_timeout` codes and reasons
  - all three are non-retryable in `shouldRetryToolError`
  - trace event classification for `tool_approval_pending`
- comprehensive test coverage:
  - resource normalization: operation classes defaults, validation, deduplication; operation rules normalization, invalid class/verdict rejection; ToolApproval normalization, validation, invalid TTL/phase
  - authorizer: allow/deny/approval_required verdicts, deny-overrides-approval, wildcard rules, backward compatibility, nil authorizer, most-restrictive-verdict unit tests
  - governed runtime: approval required sentinel error, non-retryable verification
  - approval error codes: all three are non-retryable; `IsApprovalRequiredError` helper
- updated user-facing documentation:
  - `resources.md`: `operation_classes` on Tool, `operation_rules` on ToolPermission, new ToolApproval resource with API endpoints, WaitingApproval task phase
  - `tools-and-isolation.md`: operation classes section, approval workflow
  - `security.md`: risk-tier routing guidance, approval workflow operational considerations
  - `build-custom-tool.md`: annotating tools with operation classes
  - `tool-contract-v1.md`: approval error codes table

## Documentation Process

For each new phase:

1. add or update focused docs in `architecture/`, `reference/`, or `operations/`
2. record delta in this phase log
3. link major new docs from `docs/pages/index.md`
