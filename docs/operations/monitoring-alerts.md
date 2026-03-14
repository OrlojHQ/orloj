# Monitoring and Alerts (Phase 9)

## Purpose

Provide repeatable operational checks for retry storms and dead-letter growth in message-driven task execution.

## Included Artifacts

- Alert profile: `monitoring/alerts/retry-deadletter-default.json`
- Dashboard contract: `monitoring/dashboards/retry-deadletter-overview.json`
- Alert check command: `cmd/orloj-alertcheck`

## Alert Check Command

Run against a live API server:

```bash
go run ./cmd/orloj-alertcheck \
  --base-url=http://127.0.0.1:8080 \
  --namespace=default \
  --profile=monitoring/alerts/retry-deadletter-default.json \
  --json=true
```

Optional filters:

- `--task-name-prefix` to scope by task name
- `--task-system` to scope by `Task.spec.system`

Auth:

- `--api-token=<token>` or `ORLOJ_API_TOKEN=<token>`

## Exit Behavior

- `0`: no violations
- `2`: one or more alert violations found
- `1`: command/config/API failure

This allows straightforward CI/cron integration.

## Default Threshold Profile

The default profile checks:

- retry storm absolute total and per-task rate
- dead-letter absolute total and dead-letter task rate
- in-flight saturation ceiling
- max p95 latency ceiling
- optional `require_any_task_succeeded`

Tune thresholds in `monitoring/alerts/retry-deadletter-default.json` per environment.

## Dashboard Contract

`monitoring/dashboards/retry-deadletter-overview.json` defines the panel set for:

- retry totals
- dead-letter totals
- dead-letter task rate
- in-flight totals
- max p95 latency

The file is intentionally backend-agnostic so teams can map to their dashboard stack.
