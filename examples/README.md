# Examples

This directory is organized by resource kind.

## Layout

- `agents/`
- `agent-systems/`
- `model-endpoints/`
- `tools/`
- `memories/`
- `secrets/`
- `agent-policies/`
- `agent-roles/`
- `tool-permissions/`
- `tasks/`
- `task-schedules/`
- `task-webhooks/`
- `workers/`

## Quick Start (Base Flow)

```bash
go run ./cmd/orlojctl apply -f examples/memories/research_memory.yaml
go run ./cmd/orlojctl apply -f examples/model-endpoints/openai_default.yaml
go run ./cmd/orlojctl apply -f examples/tools/web_search_tool.yaml
go run ./cmd/orlojctl apply -f examples/tools/vector_db_tool.yaml
go run ./cmd/orlojctl apply -f examples/secrets/search_api_key.yaml
go run ./cmd/orlojctl apply -f examples/secrets/openai_api_key.yaml
go run ./cmd/orlojctl apply -f examples/agents/planner_agent.yaml
go run ./cmd/orlojctl apply -f examples/agents/research_agent_model_ref.yaml
go run ./cmd/orlojctl apply -f examples/agents/writer_agent.yaml
go run ./cmd/orlojctl apply -f examples/agent-systems/report_system.yaml
go run ./cmd/orlojctl apply -f examples/agent-policies/cost_policy.yaml
go run ./cmd/orlojctl apply -f examples/tasks/weekly_report_template_task.yaml
go run ./cmd/orlojctl apply -f examples/task-schedules/weekly_report_schedule.yaml
go run ./cmd/orlojctl apply -f examples/secrets/webhook_shared_secret.yaml
go run ./cmd/orlojctl apply -f examples/task-webhooks/generic_webhook.yaml
```

## Starter Blueprints

For reusable architecture templates (pipeline, hierarchical, swarm+loop), see:

- `examples/blueprints/README.md`

For personal runtime verification scenarios (including retry/deadletter and governance deny paths), see:

- `testing/scenarios/README.md`

For live-provider runtime scenarios (real model credentials required), see:

- `testing/scenarios-real/README.md`

If you want to keep model routing worker-global, use `examples/agents/research_agent.yaml` (with explicit `spec.model`) instead of `research_agent_model_ref.yaml`.

If you want Anthropic routing instead of OpenAI routing, apply:

```bash
go run ./cmd/orlojctl apply -f examples/model-endpoints/anthropic_default.yaml
go run ./cmd/orlojctl apply -f examples/secrets/anthropic_api_key.yaml
```

If you want Azure OpenAI routing, apply:

```bash
go run ./cmd/orlojctl apply -f examples/model-endpoints/azure_openai_default.yaml
go run ./cmd/orlojctl apply -f examples/secrets/azure_openai_api_key.yaml
```

If you want local Ollama routing, apply:

```bash
go run ./cmd/orlojctl apply -f examples/model-endpoints/ollama_default.yaml
```

## Cyclical Manager/Research Loop (A <-> B)

This scenario shows explicit bidirectional handoffs (`manager-agent -> research-agent -> manager-agent`) with a bounded turn count (`Task.spec.max_turns`).

```bash
go run ./cmd/orlojctl apply -f examples/agents/manager_agent.yaml
go run ./cmd/orlojctl apply -f examples/agents/research_agent.yaml
go run ./cmd/orlojctl apply -f examples/agent-systems/manager_research_loop_system.yaml
go run ./cmd/orlojctl apply -f examples/tasks/manager_research_loop_task.yaml
```

Run workers/controller in `task-execution-mode=message-driven` with runtime inbox consumers enabled (`--agent-message-consume`) so inter-agent handoff messages are processed.

## Governance UI Scenario (Denied)

This scenario intentionally denies one tool call so governance chips appear in runtime timelines.

```bash
go run ./cmd/orlojctl apply -f examples/agent-roles/analyst_role.yaml
go run ./cmd/orlojctl apply -f examples/tool-permissions/web_search_invoke_permission.yaml
go run ./cmd/orlojctl apply -f examples/tool-permissions/vector_db_invoke_permission.yaml
go run ./cmd/orlojctl apply -f examples/agents/research_agent_governed.yaml
go run ./cmd/orlojctl apply -f examples/agent-systems/report_system_governed.yaml
go run ./cmd/orlojctl apply -f examples/tasks/weekly_report_governed_task.yaml
```

## Governance UI Scenario (Allowed)

Adds the missing role permission for vector DB so the run can proceed.

```bash
go run ./cmd/orlojctl apply -f examples/agent-roles/vector_reader_role.yaml
go run ./cmd/orlojctl apply -f examples/agents/research_agent_governed_allow.yaml
go run ./cmd/orlojctl apply -f examples/agent-systems/report_system_governed_allow.yaml
go run ./cmd/orlojctl apply -f examples/tasks/weekly_report_governed_allow_task.yaml
```

## Load Test Retry-Stress Scenario Resources

These manifests back the `orloj-loadtest` retry-stress injection mode.

```bash
go run ./cmd/orlojctl apply -f examples/agents/loadtest_timeout_agent.yaml
go run ./cmd/orlojctl apply -f examples/agent-systems/loadtest_timeout_system.yaml
```

## WASM Tool Reference Module

Apply the wasm tool resource:

```bash
go run ./cmd/orlojctl apply -f examples/tools/wasm-reference/wasm_echo_tool.yaml
```

Run the reference guest module directly:

```bash
wasmtime run --invoke run examples/tools/wasm-reference/echo_guest.wat
```

Use wasm isolation mode in worker/control-plane binaries and point module path at this file:

```bash
go run ./cmd/orlojworker \
  --tool-isolation-backend=wasm \
  --tool-wasm-module="$(pwd)/examples/tools/wasm-reference/echo_guest.wat" \
  --tool-wasm-entrypoint=run
```
