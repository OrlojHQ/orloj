# Live Validation Matrix

Use this runbook to exercise Orloj with real model providers and a deterministic local tool stub before open source release.

## Purpose

The automated Go test suite proves core correctness, but the live-validation matrix is where we check:

- real provider behavior
- message-driven execution
- tool isolation with real HTTP calls
- memory-backed agent workflows
- governance deny paths
- trigger paths through webhooks and schedules

## Before You Start

1. Run the automated baseline:

```bash
go test ./...
```

2. Start `orlojd`:

```bash
go run ./cmd/orlojd --task-execution-mode=message-driven --agent-message-bus-backend=memory
```

3. Start the worker for your lane:

Anthropic, model-only:

```bash
go run ./cmd/orlojworker \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=memory \
  --agent-message-consume \
  --model-gateway-provider=anthropic
```

Anthropic, tool-backed:

```bash
go run ./cmd/orlojworker \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=memory \
  --agent-message-consume \
  --model-gateway-provider=anthropic \
  --tool-isolation-backend=container \
  --tool-container-network=bridge
```

4. Start the deterministic stub service:

```bash
make real-tool-stub
```

5. Replace all `replace-me` provider secrets in `testing/scenarios-real/`.

## Matrix Overview

### Wave 0

- `make real-gate-pipeline`
- `make real-gate-hier`
- `make real-gate-loop`
- `make real-gate-tool`
- `make real-gate-tool-decision`

### Wave 1

- `make real-gate-memory-shared`
- `make real-gate-memory-reuse`

### Wave 2

- `make real-gate-tool-auth`
- `make real-gate-governance-deny`
- `make real-gate-tool-retry`

### Wave 3

- `make real-gate-webhook`
- `make real-gate-schedule`

## Acceptance Targets

- Run every Wave 0 and Wave 1 scenario 3 times:

```bash
make real-repeat TARGET=real-gate-pipeline COUNT=3
make real-repeat TARGET=real-gate-memory-shared COUNT=3
```

- Run governance and tool-decision scenarios 5 times:

```bash
make real-repeat TARGET=real-gate-tool-decision COUNT=5
make real-repeat TARGET=real-gate-governance-deny COUNT=5
```

## Deterministic Tool Stub

The local stub service lives at:

- host: `http://127.0.0.1:18080`
- container-accessible: `http://host.docker.internal:18080`

Supported paths:

- `/tool/smoke`
- `/tool/decision`
- `/tool/auth`
- `/tool/retry-once`

This avoids public echo services and gives stable auth/retry assertions.

## Artifact Convention

Every gate captures artifacts under:

```text
testing/artifacts/real/<namespace>/<task>/<timestamp>/
```

Files:

- `task.json`
- `messages.json`
- `metrics.json`
- `memory-<name>.json` for memory-backed scenarios
- `verdict.txt`

## UI Validation Checklist

After a gate passes, inspect `/ui/` and confirm:

- task trace is readable and includes the expected step sequence
- tool calls are visible for tool-backed scenarios
- memory entries are visible on the Memory detail page
- deny/failure paths are understandable without reading source code

## Troubleshooting

- `secret placeholder detected`: replace `replace-me` in the scenario secret.
- `tool container cannot reach stub`: start the worker with `--tool-container-network=bridge` and keep the stub on port `18080`.
- `webhook has not created a task yet`: check the signing secret and confirm the delivery returned HTTP `202`.
- `schedule has not created a task yet`: give the minute-level schedule up to 120 seconds and confirm `orlojd` is reconciling schedules.

## Related

- [Real-Model Scenario README](../../../testing/scenarios-real/README.md)
- [Webhook Triggers](./webhooks.md)
- [Task Scheduling (Cron)](./task-scheduling.md)
- [Real Tool Validation](./real-tool-validation.md)
