# Internal Tools

Flag reference for `orloj-loadtest` (reliability/load harness) and `orloj-alertcheck` (alert profile evaluator). These are operational and CI tools, not user-facing CLIs.

## `orloj-loadtest`

Print full flags:

```bash
go run ./cmd/orloj-loadtest -h
```

| Flag | Default | Description | Condition / Notes |
|---|---|---|---|
| `--base-url` | `http://127.0.0.1:8080` | Orloj API base URL. | n/a |
| `--namespace` | `default` | Target namespace. | n/a |
| `--tasks` | `50` | Number of tasks to create. | n/a |
| `--create-concurrency` | `10` | Concurrent task-create workers. | n/a |
| `--poll-concurrency` | `20` | Concurrent status-poll workers. | n/a |
| `--poll-interval` | `500ms` | Poll interval for task status. | n/a |
| `--run-timeout` | `5m` | Global run timeout. | n/a |
| `--task-system` | `report-system` | AgentSystem for generated tasks. | n/a |
| `--topic-prefix` | `loadtest-topic` | Task input topic prefix. | n/a |
| `--task-priority` | `high` | Task priority. | n/a |
| `--task-retry-attempts` | `3` | Generated `Task.spec.retry.max_attempts`. | n/a |
| `--task-retry-backoff` | `2s` | Generated `Task.spec.retry.backoff`. | n/a |
| `--message-retry-attempts` | `4` | Generated `Task.spec.message_retry.max_attempts`. | n/a |
| `--message-retry-backoff` | `200ms` | Generated `Task.spec.message_retry.backoff`. | n/a |
| `--message-retry-max-backoff` | `2s` | Generated `Task.spec.message_retry.max_backoff`. | n/a |
| `--message-retry-jitter` | `full` | Generated `Task.spec.message_retry.jitter`. | `none|full|equal`. |
| `--setup` | `true` | Apply baseline manifests before load run. | n/a |
| `--min-ready-workers` | `2` | Minimum ready workers required before run. | `0` disables check. |
| `--worker-ready-timeout` | `45s` | Max wait for worker readiness check. | n/a |
| `--inject-invalid-system-rate` | `0` | Fraction routed to invalid system. | Injection control. |
| `--invalid-system-name` | `missing-system-loadtest` | Invalid system name used for injection. | Injection control. |
| `--inject-timeout-system-rate` | `0` | Fraction routed to timeout system. | Injection control. |
| `--timeout-system-name` | `loadtest-timeout-system` | Timeout system name for injection. | Injection control. |
| `--timeout-agent-name` | `loadtest-timeout-agent` | Timeout-agent name in injected system. | Injection control. |
| `--timeout-agent-duration` | `1ms` | Timeout used by injected agent limits. | Injection control. |
| `--inject-expired-lease-rate` | `0` | Fraction patched for expired-lease takeover simulation. | Injection control. |
| `--expired-lease-owner` | `worker-crashed-simulated` | Synthetic owner ID used for expired-lease simulation. | Injection control. |
| `--quality-profile` | empty | Optional JSON profile for quality gates. | n/a |
| `--min-success-rate` | `0` | Minimum success-rate gate. | `0` disables. |
| `--max-deadletter-rate` | `-1` | Maximum deadletter-rate gate. | `-1` disables. |
| `--max-failed-rate` | `-1` | Maximum failed-rate gate. | `-1` disables. |
| `--max-timed-out` | `0` | Maximum timed-out task count gate. | `-1` disables. |
| `--min-retry-total` | `-1` | Minimum total retry-count gate. | `-1` disables. |
| `--min-takeover-events` | `-1` | Minimum takeover-history event-count gate. | `-1` disables. |
| `--json` | `false` | Emit machine-readable JSON report. | n/a |
| `--verbose` | `false` | Print periodic progress. | n/a |

---

## `orloj-alertcheck`

Print full flags:

```bash
go run ./cmd/orloj-alertcheck -h
```

| Flag | Default | Description |
|---|---|---|
| `--base-url` | `http://127.0.0.1:8080` | Orloj API base URL. |
| `--namespace` | `default` | Target namespace. |
| `--api-token` | empty | Optional bearer token for API auth (env fallback: `ORLOJ_API_TOKEN`). |
| `--profile` | `monitoring/alerts/retry-deadletter-default.json` | Alert threshold profile JSON file. |
| `--task-name-prefix` | empty | Optional task metadata.name prefix filter. |
| `--task-system` | empty | Optional `Task.spec.system` filter. |
| `--poll-concurrency` | `20` | Concurrent task metrics fetch workers. |
| `--timeout` | `2m` | Global command timeout. |
| `--json` | `true` | Emit JSON output. |
| `--verbose` | `false` | Emit verbose progress logs. |

## Command Discovery

Use help output as the authoritative source for your current build:

```bash
go run ./cmd/orloj-loadtest -h
go run ./cmd/orloj-alertcheck -h
```

## Related

- [Monitoring & Alerts](../operations/monitoring-alerts.md) — alert profiles and dashboards
- [Server Flags](./server-flags.md) — orlojd and orlojworker daemon flags
