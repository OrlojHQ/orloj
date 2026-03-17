# Operations Runbook

Use this runbook for baseline production operation and incident response.

## Reference Topology

1. `orlojd` server
2. Postgres state backend
3. NATS JetStream for message-driven execution
4. multiple `orlojworker` instances

## Startup Procedure

1. Start Postgres and NATS.
2. Start `orlojd` with `--storage-backend=postgres` and `--task-execution-mode=message-driven`.
3. Start at least two workers with `--agent-message-consume`.
4. Configure model provider and credentials.
5. Apply required resources (`ModelEndpoint`, `Tool`, `Agent`, `AgentSystem`, `Task`, governance resources).

## Verification

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl get workers
go run ./cmd/orlojctl get tasks
curl -s http://127.0.0.1:8080/metrics | head -20
```

Expected result:

- API health endpoint reports healthy.
- Workers report `Ready` and heartbeat updates.
- Tasks transition through expected lifecycle.
- `/metrics` returns Prometheus text output with `orloj_*` metrics.

## Failure and Recovery Expectations

- Worker crash: lease expires and another worker can claim.
- Retry behavior: delayed requeue until success or dead-letter.
- Policy/graph validation failures: non-retryable, deterministic dead-letter.
- Tool runtime denials/errors: normalized metadata in trace/log paths.

## Observability

- Configure `OTEL_EXPORTER_OTLP_ENDPOINT` on both `orlojd` and `orlojworker` for distributed tracing.
- Prometheus scrapes `/metrics` on the `orlojd` HTTP port -- add the target to your Prometheus scrape config.
- Logs are structured JSON by default (`ORLOJ_LOG_FORMAT=json`) with `request_id` and `trace_id` fields.
- The web console Trace tab shows task execution waterfall without any external backend.
- See [Observability](./observability.md) for full setup details.

## Reliability Operations

- Run `go run ./cmd/orloj-loadtest` for repeatable load/failure validation.
- Run `go run ./cmd/orloj-alertcheck` to validate retry/dead-letter thresholds.
- Keep alert and load profile thresholds aligned with SLO targets.

## Related Docs

- [Observability](./observability.md)
- [Deployment Overview](../deployment/index.md)
- [VPS Deployment](../deployment/vps.md)
- [Kubernetes Deployment](../deployment/kubernetes.md)
- [Configuration](./configuration.md)
- [Troubleshooting](./troubleshooting.md)
- [Upgrades and Rollbacks](./upgrades.md)
