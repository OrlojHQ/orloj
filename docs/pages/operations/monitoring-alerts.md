# Monitoring and Alerts

Use `orloj-alertcheck` and dashboard contracts to validate runtime reliability signals.

> For Prometheus metrics, OpenTelemetry tracing, structured logging, and trace visualization, see [Observability](./observability.md).

## Purpose

This guide defines repeatable checks for retry storms, dead-letter growth, and latency saturation.

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

### `orloj-alertcheck` Flags

| Flag | Default | Description |
|---|---|---|
| `--base-url` | `http://127.0.0.1:8080` | Orloj API base URL. |
| `--namespace` | `default` | Target namespace for task queries. |
| `--api-token` | empty | Optional bearer token for API auth (env fallback: `ORLOJ_API_TOKEN`). |
| `--profile` | `monitoring/alerts/retry-deadletter-default.json` | Alert threshold profile JSON file. |
| `--task-name-prefix` | empty | Optional filter by task name prefix. |
| `--task-system` | empty | Optional filter by `Task.spec.system`. |
| `--poll-concurrency` | `20` | Concurrent task metrics fetch workers. |
| `--timeout` | `2m` | Global command timeout. |
| `--json` | `true` | Emit machine-readable JSON output. |
| `--verbose` | `false` | Emit verbose progress logs. |

For authoritative defaults and full CLI context, see [CLI reference](../reference/cli.md#orloj-alertcheck).

## Loadtest Reliability Gates and Injection Controls

Use `orloj-loadtest` to validate system behavior under expected and fault-injected load patterns.

Key reliability gate controls:

- `--quality-profile`
- `--min-success-rate`
- `--max-deadletter-rate`
- `--max-failed-rate`
- `--max-timed-out`
- `--min-retry-total`
- `--min-takeover-events`

Key injection controls:

- `--inject-invalid-system-rate`, `--invalid-system-name`
- `--inject-timeout-system-rate`, `--timeout-system-name`, `--timeout-agent-name`, `--timeout-agent-duration`
- `--inject-expired-lease-rate`, `--expired-lease-owner`

Worker readiness and pacing controls:

- `--min-ready-workers`, `--worker-ready-timeout`
- `--poll-concurrency`, `--poll-interval`, `--run-timeout`

For exhaustive loadtest flags and defaults, see [CLI reference](../reference/cli.md#orloj-loadtest).

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
