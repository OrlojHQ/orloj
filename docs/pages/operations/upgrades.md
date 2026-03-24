# Upgrades and Rollbacks

This guide defines safe upgrade and rollback procedures for the Orloj server and workers.

## Principles

- prefer staged rollouts over full replacement
- take Postgres backups before upgrades
- validate reliability gates before production promotion
- couple release behavior with contract documentation

## Pre-Upgrade Checklist

- [ ] Read release notes and migration notes.
- [ ] Take a full Postgres backup per the [Backup and Restore](backup-restore.md) guide.
- [ ] Record the current `ORLOJ_SECRET_ENCRYPTION_KEY`.
- [ ] Verify baseline health (`/healthz`, workers, task flow).
- [ ] Run smoke checks in staging.

## Upgrade Procedure

1. Upgrade `orlojd` in staging.
2. Verify API health and resource status.
3. Upgrade one worker (canary).
4. Validate task execution paths used by your deployment.
5. Upgrade remaining workers.
6. Run reliability checks:
  - `orloj-loadtest`
  - `orloj-alertcheck`

## Production Rollout

- canary one server instance and one worker first
- monitor task success/dead-letter ratio, retry volume, p95 latency, heartbeat stability

## Rollback Triggers

- server health degradation
- retry/dead-letter rates exceed SLO thresholds
- unexpected increase in non-retryable runtime/policy failures

## Rollback Procedure

1. Revert server and worker binaries to previous release.
2. Restore previous configuration values.
3. Restore Postgres from backup if required (see [Backup and Restore](backup-restore.md)).
4. Re-run smoke checks before resuming rollout.

## Compatibility Guidance

- keep compatibility checks green for pinned downstream consumers
- avoid unversioned breaking changes on public contracts
- treat contract graduation and lifecycle changes as release events

## Validation Commands

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl get workers
go run ./cmd/orlojctl get tasks
go run ./cmd/orloj-loadtest --quality-profile=monitoring/loadtest/quality-default.json --tasks=50
go run ./cmd/orloj-alertcheck --profile=monitoring/alerts/retry-deadletter-default.json
```

For complete reliability and alert flag coverage (including gate/injection controls and verbose/debug options), see:

- [CLI reference](../reference/cli.md#orloj-loadtest)
- [CLI reference](../reference/cli.md#orloj-alertcheck)
- [Monitoring and Alerts](./monitoring-alerts.md)
