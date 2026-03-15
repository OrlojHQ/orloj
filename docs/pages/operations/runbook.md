# Operations Runbook

Use this runbook for baseline production operation and incident response.

## Reference Topology

1. `orlojd` control plane
2. Postgres state backend
3. NATS JetStream for message-driven execution
4. multiple `orlojworker` instances

## Startup Procedure

1. Start Postgres and NATS.
2. Start `orlojd` with `--storage-backend=postgres` and `--task-execution-mode=message-driven`.
3. Start at least two workers with `--agent-message-consume`.
4. Configure model provider and credentials.
5. Apply required resources (`ModelEndpoint`, `Tool`, `Agent`, `AgentSystem`, `Task`, governance CRDs).

## Verification

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl get workers
go run ./cmd/orlojctl get tasks
```

Expected result:

- API health endpoint reports healthy.
- Workers report `Ready` and heartbeat updates.
- Tasks transition through expected lifecycle.

## Failure and Recovery Expectations

- Worker crash: lease expires and another worker can claim.
- Retry behavior: delayed requeue until success or dead-letter.
- Policy/graph validation failures: non-retryable, deterministic dead-letter.
- Tool runtime denials/errors: normalized metadata in trace/log paths.

## Reliability Operations

- Run `go run ./cmd/orloj-loadtest` for repeatable load/failure validation.
- Run `go run ./cmd/orloj-alertcheck` to validate retry/dead-letter thresholds.
- Keep alert and load profile thresholds aligned with SLO targets.

## Related Docs

- [Configuration](./configuration.md)
- [Troubleshooting](./troubleshooting.md)
- [Upgrades and Rollbacks](./upgrades.md)
