# Runtime Test Scenarios

This directory is a personal test harness with self-contained YAML for runtime validation.

Use these scenarios with `task-execution-mode=message-driven` and worker inbox consumers enabled (`--agent-message-consume`).

For live-provider testing with real model credentials, use `testing/scenarios-real/README.md`.

## How to apply a scenario

From repo root, apply all YAML in a scenario directory:

```bash
find testing/scenarios/01-pipeline -name '*.yaml' -print | sort | xargs -I{} go run ./cmd/orlojctl apply -f {}
```

Replace `01-pipeline` with any scenario folder below.

## Scenarios

1. `01-pipeline`
- Linear `planner -> research -> writer` handoffs.
- Expected: task succeeds; message chain progresses in order.

2. `02-hierarchical`
- Manager delegates to leads; workers feed a join node (`wait_for_all`).
- Expected: editor executes after both worker branches complete.

3. `03-loop-max-turns`
- Cyclical `manager <-> research` graph with `max_turns`.
- Expected: bounded loop terminates and task succeeds.

4. `04-governance-deny`
- Agent has a tool, but role permissions do not satisfy `ToolPermission` requirements.
- Expected: permission denial path triggers (deterministic with mock model gateway).
- Tip: set `ORLOJ_MODEL_GATEWAY_PROVIDER=mock` while testing this scenario.

5. `05-retry-deadletter`
- Single-agent timeout stress with message retry budget.
- Expected: retry scheduling then dead-letter after max attempts.

## Useful checks

```bash
go run ./cmd/orlojctl get task rt-pipeline-task
curl -s "http://localhost:8080/v1/tasks/rt-pipeline-task/messages" | jq .
curl -s "http://localhost:8080/v1/tasks/rt-pipeline-task/metrics" | jq .
```

Swap task name per scenario (`rt-hier-task`, `rt-loop-task`, `rt-gov-deny-task`, `rt-retry-deadletter-task`).
