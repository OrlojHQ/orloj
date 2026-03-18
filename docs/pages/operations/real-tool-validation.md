# Real Tool Validation (Model Decision Gate)

Use this runbook to validate model-selected tool usage in an Anthropic-backed A/B scenario.

## Goal

- Task A should use a tool.
- Task B should not use a tool.

Scenario path:

- `testing/scenarios-real/05-tool-decision`

## Before You Begin

1. Add a valid Anthropic key to:
  - `testing/scenarios-real/05-tool-decision/secret.yaml`
2. Ensure Docker is available for containerized tools.
3. Ensure API server is reachable at `http://localhost:8080` (or override `API_BASE`).
4. Start the local deterministic stub tool service:

```bash
make real-tool-stub
```

## Runtime Startup

Terminal 1 (server):

```bash
go run ./cmd/orlojd --task-execution-mode=message-driven --agent-message-bus-backend=memory
```

Terminal 2 (worker with Anthropic + containerized tools):

```bash
go run ./cmd/orlojworker \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=memory \
  --agent-message-consume \
  --model-gateway-provider=anthropic \
  --tool-isolation-backend=container \
  --tool-container-network=bridge
```

## Apply Scenario

```bash
make real-apply-tool-decision
```

This applies:

- `ModelEndpoint` pinned to `claude-sonnet-4-20250514`
- one HTTP tool with `spec.runtime.isolation_mode=container`
- one decision agent
- two tasks:
  - `rr-tool-use-task`
  - `rr-tool-no-use-task`

The tool points at the local stub service via `http://host.docker.internal:18080/tool/decision`.

## Run Gate

```bash
make real-gate-tool-decision
```

## Pass/Fail Criteria

### `rr-tool-use-task`

Pass requires all:

- task phase is `Succeeded`
- `status.output["agent.1.tool_calls"] >= 1`
- at least one `tool_call` event in `status.trace[]`
- `status.output["agent.1.last_event"]` contains `TOOL_USED: yes` and `EVIDENCE:`

### `rr-tool-no-use-task`

Pass requires all:

- task phase is `Succeeded`
- `status.output["agent.1.tool_calls"] == 0`
- zero `tool_call` events in `status.trace[]`
- `status.output["agent.1.last_event"]` contains `TOOL_USED: no` and `EVIDENCE: self-contained-input`

## Reliability Target

Pass `make real-gate-tool-decision` five consecutive times.

## Manual Inspection

```bash
make real-check NS=rr-real-tool-decision TASK=rr-tool-use-task
make real-check NS=rr-real-tool-decision TASK=rr-tool-no-use-task
```
