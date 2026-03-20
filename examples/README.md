# Examples

Top level:

- **`resources/`** — manifests grouped **by resource kind** (agents, tasks, tools, …). See [`resources/README.md`](resources/README.md).
- **`blueprints/`** — minimal pipeline / hierarchical / swarm topology templates.
- **`use-cases/`** — copy-paste scenario bundles.

## Layout

- `resources/agents/`
- `resources/agent-systems/`
- `resources/model-endpoints/`
- `resources/tools/`
- `resources/memories/`
- `resources/secrets/`
- `resources/agent-policies/`
- `resources/agent-roles/`
- `resources/tool-permissions/`
- `resources/tasks/`
- `resources/task-schedules/`
- `resources/task-webhooks/`
- `resources/workers/`
- `resources/mcp-servers/`
- `blueprints/`
- `use-cases/`

## Real-world scenarios

Self-contained **use case** directories (full YAML templates + README per scenario) live under:

- `examples/use-cases/README.md` — index
- `examples/use-cases/weekly-intelligence-brief/`, `cross-functional-pmo/`, `roadmap-synthesis-swarm/`, `event-driven-webhook/`

## Quick Start (Base Flow)

```bash
go run ./cmd/orlojctl apply -f examples/resources/memories/research_memory.yaml
go run ./cmd/orlojctl apply -f examples/resources/model-endpoints/openai_default.yaml
go run ./cmd/orlojctl apply -f examples/resources/tools/web_search_tool.yaml
go run ./cmd/orlojctl apply -f examples/resources/tools/vector_db_tool.yaml
go run ./cmd/orlojctl apply -f examples/resources/secrets/search_api_key.yaml
go run ./cmd/orlojctl apply -f examples/resources/secrets/openai_api_key.yaml
go run ./cmd/orlojctl apply -f examples/resources/agents/planner_agent.yaml
go run ./cmd/orlojctl apply -f examples/resources/agents/research_agent_model_ref.yaml
go run ./cmd/orlojctl apply -f examples/resources/agents/writer_agent.yaml
go run ./cmd/orlojctl apply -f examples/resources/agent-systems/report_system.yaml
go run ./cmd/orlojctl apply -f examples/resources/agent-policies/cost_policy.yaml
go run ./cmd/orlojctl apply -f examples/resources/tasks/weekly_report_template_task.yaml
go run ./cmd/orlojctl apply -f examples/resources/task-schedules/weekly_report_schedule.yaml
go run ./cmd/orlojctl apply -f examples/resources/secrets/webhook_shared_secret.yaml
go run ./cmd/orlojctl apply -f examples/resources/task-webhooks/generic_webhook.yaml
```

## Starter Blueprints

For reusable architecture templates (pipeline, hierarchical, swarm+loop), see:

- `examples/blueprints/README.md`

For personal runtime verification scenarios (including retry/deadletter and governance deny paths), see:

- `testing/scenarios/README.md`

For live-provider runtime scenarios (real model credentials required), see:

- `testing/scenarios-real/README.md`

Model routing is configured per agent via `spec.model_ref`. Ensure the referenced `ModelEndpoint` exists before running tasks.

If you want Anthropic routing instead of OpenAI routing, apply:

```bash
go run ./cmd/orlojctl apply -f examples/resources/model-endpoints/anthropic_default.yaml
go run ./cmd/orlojctl apply -f examples/resources/secrets/anthropic_api_key.yaml
```

If you want Azure OpenAI routing, apply:

```bash
go run ./cmd/orlojctl apply -f examples/resources/model-endpoints/azure_openai_default.yaml
go run ./cmd/orlojctl apply -f examples/resources/secrets/azure_openai_api_key.yaml
```

If you want local Ollama routing, apply:

```bash
go run ./cmd/orlojctl apply -f examples/resources/model-endpoints/ollama_default.yaml
```

## Cyclical Manager/Research Loop (A <-> B)

This scenario shows explicit bidirectional handoffs (`manager-agent -> research-agent -> manager-agent`) with a bounded turn count (`Task.spec.max_turns`).

```bash
go run ./cmd/orlojctl apply -f examples/resources/agents/manager_agent.yaml
go run ./cmd/orlojctl apply -f examples/resources/agents/research_agent.yaml
go run ./cmd/orlojctl apply -f examples/resources/agent-systems/manager_research_loop_system.yaml
go run ./cmd/orlojctl apply -f examples/resources/tasks/manager_research_loop_task.yaml
```

Run workers/controller in `task-execution-mode=message-driven` with runtime inbox consumers enabled (`--agent-message-consume`) so inter-agent handoff messages are processed.

## Governance UI Scenario (Denied)

This scenario intentionally denies one tool call so governance chips appear in runtime timelines.

```bash
go run ./cmd/orlojctl apply -f examples/resources/agent-roles/analyst_role.yaml
go run ./cmd/orlojctl apply -f examples/resources/tool-permissions/web_search_invoke_permission.yaml
go run ./cmd/orlojctl apply -f examples/resources/tool-permissions/vector_db_invoke_permission.yaml
go run ./cmd/orlojctl apply -f examples/resources/agents/research_agent_governed.yaml
go run ./cmd/orlojctl apply -f examples/resources/agent-systems/report_system_governed.yaml
go run ./cmd/orlojctl apply -f examples/resources/tasks/weekly_report_governed_task.yaml
```

## Governance UI Scenario (Allowed)

Adds the missing role permission for vector DB so the run can proceed.

```bash
go run ./cmd/orlojctl apply -f examples/resources/agent-roles/vector_reader_role.yaml
go run ./cmd/orlojctl apply -f examples/resources/agents/research_agent_governed_allow.yaml
go run ./cmd/orlojctl apply -f examples/resources/agent-systems/report_system_governed_allow.yaml
go run ./cmd/orlojctl apply -f examples/resources/tasks/weekly_report_governed_allow_task.yaml
```

## Load Test Retry-Stress Scenario Resources

These manifests back the `orloj-loadtest` retry-stress injection mode.

```bash
go run ./cmd/orlojctl apply -f examples/resources/agents/loadtest_timeout_agent.yaml
go run ./cmd/orlojctl apply -f examples/resources/agent-systems/loadtest_timeout_system.yaml
```

## WASM Tool Reference Module

Apply the wasm tool resource:

```bash
go run ./cmd/orlojctl apply -f examples/resources/tools/wasm-reference/wasm_echo_tool.yaml
```

Run the reference guest module directly:

```bash
wasmtime run --invoke run examples/resources/tools/wasm-reference/echo_guest.wat
```

Use wasm isolation mode in worker/control-plane binaries and point module path at this file:

```bash
go run ./cmd/orlojworker \
  --tool-isolation-backend=wasm \
  --tool-wasm-module="$(pwd)/examples/resources/tools/wasm-reference/echo_guest.wat" \
  --tool-wasm-entrypoint=run
```
