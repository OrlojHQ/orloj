# Real Tool Validation (Anthropic Decision Gate)

This runbook validates model-selected tool usage with Anthropic in a paired A/B scenario:

- task A should use a tool
- task B should not use a tool

Scenario path:

- `testing/scenarios-real/05-anthropic-tool-decision`

## Prerequisites

1. Insert a valid Anthropic key in:
   - `testing/scenarios-real/05-anthropic-tool-decision/secret.yaml`
2. Ensure Docker is available for containerized tool execution.
3. Ensure API server is reachable at `http://localhost:8080` (or override `API_BASE`).

## Runtime Startup

Terminal 1 (control plane):

```bash
go run ./cmd/orlojd --task-execution-mode=message-driven --agent-message-bus-backend=memory
```

Terminal 2 (worker; Anthropic + containerized tools):

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

The gate runs:

- `make real-check-anthropic-tool-use`
- `make real-check-anthropic-tool-no-use`

and fails fast on any unmet condition.

## Pass/Fail Criteria

### `rr-anthropic-tool-use-task`

Pass requires all:

- task phase is `Succeeded`
- `status.output["agent.1.tool_calls"] >= 1`
- at least one `tool_call` event exists in `status.trace[]`
- `status.output["agent.1.last_event"]` includes:
  - `TOOL_USED: yes`
  - `EVIDENCE:`

### `rr-anthropic-tool-no-use-task`

Pass requires all:

- task phase is `Succeeded`
- `status.output["agent.1.tool_calls"] == 0`
- zero `tool_call` events in `status.trace[]`
- `status.output["agent.1.last_event"]` includes:
  - `TOOL_USED: no`
  - `EVIDENCE: self-contained-input`

## Reliability Target

Run and pass `make real-gate-anthropic-tool-decision` 5 consecutive times.

## Manual Inspection Commands

```bash
make real-check NS=rr-real-anthropic-tool-decision TASK=rr-anthropic-tool-use-task
make real-check NS=rr-real-anthropic-tool-decision TASK=rr-anthropic-tool-no-use-task
```
