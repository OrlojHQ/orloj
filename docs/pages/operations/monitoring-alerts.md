# Monitoring and Alerts

Use `orloj-alertcheck` and dashboard contracts to validate runtime reliability signals.

> For Prometheus metrics, OpenTelemetry tracing, structured logging, and the trace visualization UI, see [Observability](./observability.md).

## Purpose

This guide defines repeatable operational checks for retry storms, dead-letter growth, and latency saturation.

## Artifacts

- Alert profile (default): `monitoring/alerts/retry-deadletter-default.json`
- Alert profile (CI): `monitoring/alerts/retry-deadletter-ci.json`
- Dashboard contract: `monitoring/dashboards/retry-deadletter-overview.json`
- Alert check command: `cmd/orloj-alertcheck`

The CI profile uses a lower `min_tasks` floor and a higher latency ceiling to accommodate CI runner variability. It is used by the `reliability` job in `.github/workflows/ci.yml`.

## Alert Check Command

```bash
go run ./cmd/orloj-alertcheck \
  --base-url=http://127.0.0.1:8080 \
  --namespace=default \
  --profile=monitoring/alerts/retry-deadletter-default.json \
  --json=true
```

Optional filters:

- `--task-name-prefix`
- `--task-system`

Auth:

- `--api-token=<token>` or `ORLOJ_API_TOKEN=<token>`

## Exit Behavior

- `0`: no violations
- `2`: one or more alert violations found
- `1`: command/config/API failure

## Default Threshold Profile

The default profile checks:

- retry storm absolute total and per-task rate
- dead-letter absolute total and dead-letter task rate
- in-flight saturation ceiling
- max p95 latency ceiling (complement with `orloj_agent_step_duration_seconds` Prometheus histogram for live percentile queries)
- optional `require_any_task_succeeded`

## Dashboard Contract

`monitoring/dashboards/retry-deadletter-overview.json` defines backend-agnostic panel expectations for:

- retry totals
- dead-letter totals
- dead-letter task rate
- in-flight totals
- max p95 latency
