# Real Tool Validation (Anthropic Decision Gate)

Use this runbook to validate model-selected tool usage with Anthropic in an A/B scenario.

## Goal

- Task A should use a tool.
- Task B should not use a tool.

Scenario path:

- `testing/scenarios-real/05-anthropic-tool-decision`

## Before You Begin

1. Add a valid Anthropic key to:
  - `testing/scenarios-real/05-anthropic-tool-decision/secret.yaml`
2. Ensure Docker is available for containerized tools.
3. Ensure API server is reachable at `http://localhost:8080` (or override `API_BASE`).

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
make real-apply-anthropic-tool-decision
```

This applies:

- Anthropic `ModelEndpoint` pinned to `claude-3-5-sonnet-latest`
- one HTTP tool with `spec.runtime.isolation_mode=container`
- one decision agent
- two tasks:
  - `rr-anthropic-tool-use-task`
  - `rr-anthropic-tool-no-use-task`

## Run Gate

```bash
make real-gate-anthropic-tool-decision
```

## Pass/Fail Criteria

### `rr-anthropic-tool-use-task`

Pass requires all:

- task phase is `Succeeded`
- `status.output["agent.1.tool_calls"] >= 1`
- at least one `tool_call` event in `status.trace[]`
- `status.output["agent.1.last_event"]` contains `TOOL_USED: yes` and `EVIDENCE:`

### `rr-anthropic-tool-no-use-task`

Pass requires all:

- task phase is `Succeeded`
- `status.output["agent.1.tool_calls"] == 0`
- zero `tool_call` events in `status.trace[]`
- `status.output["agent.1.last_event"]` contains `TOOL_USED: no` and `EVIDENCE: self-contained-input`

## Reliability Target

Pass `make real-gate-anthropic-tool-decision` five consecutive times.

## Manual Inspection

```bash
make real-check NS=rr-real-anthropic-tool-decision TASK=rr-anthropic-tool-use-task
make real-check NS=rr-real-anthropic-tool-decision TASK=rr-anthropic-tool-no-use-task
```
