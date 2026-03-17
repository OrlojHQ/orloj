# Backup and Restore

This guide covers backup and restore procedures for Orloj deployments using the Postgres storage backend. Memory-backend deployments are ephemeral and do not require backup.

## What to Back Up

| Component | Location | Required |
|---|---|---|
| Postgres database | `ORLOJ_POSTGRES_DSN` target | Yes |
| Secret encryption key | `ORLOJ_SECRET_ENCRYPTION_KEY` env var or flag | Yes (if secrets exist) |
| Server/worker configuration | Flags, env vars, Kubernetes manifests | Recommended |
| Monitoring profiles | `monitoring/` directory | Recommended |

The secret encryption key is critical. Without it, encrypted `Secret` resource values cannot be decrypted after restore. Store it separately from the database backup in a secure vault.

## Postgres Backup

### Full Dump

```bash
pg_dump "$ORLOJ_POSTGRES_DSN" \
  --format=custom \
  --file=orloj-backup-$(date +%Y%m%d-%H%M%S).dump
```

`--format=custom` produces a compressed archive that supports selective restore and parallel jobs.

### Automated Scheduled Backup

For production, schedule backups with cron or your orchestrator's job scheduler:

```bash
# Daily backup with 7-day retention
0 2 * * * pg_dump "$ORLOJ_POSTGRES_DSN" --format=custom \
  --file=/backups/orloj-$(date +\%Y\%m\%d).dump \
  && find /backups -name "orloj-*.dump" -mtime +7 -delete
```

### Cloud-Managed Databases

If using a managed Postgres service (RDS, Cloud SQL, Azure Database), use the provider's automated backup and point-in-time recovery features instead of `pg_dump`. Ensure the retention window meets your recovery objectives.

## Restore Procedure

### 1. Stop Orloj Services

Stop `orlojd` and all `orlojworker` instances to prevent writes during restore.

### 2. Restore the Database

Restore to a fresh database or the existing one:

```bash
# Create fresh database (recommended)
createdb orloj_restored

# Restore from dump
pg_restore --dbname=orloj_restored \
  --clean --if-exists \
  --no-owner \
  orloj-backup-20260317-020000.dump
```

If restoring to the existing database:

```bash
pg_restore --dbname="$ORLOJ_POSTGRES_DSN" \
  --clean --if-exists \
  --no-owner \
  orloj-backup-20260317-020000.dump
```

### 3. Update DSN (if restored to a new database)

Point `ORLOJ_POSTGRES_DSN` to the restored database before restarting services.

### 4. Verify the Encryption Key

Ensure `ORLOJ_SECRET_ENCRYPTION_KEY` matches the key that was active when the backup was taken. A mismatched key will cause Secret resource decryption failures at runtime.

### 5. Restart and Validate

```bash
# Start orlojd
./orlojd --storage-backend=postgres ...

# Verify health
curl -sf http://127.0.0.1:8080/healthz | jq .

# Verify resources are accessible
go run ./cmd/orlojctl get agents
go run ./cmd/orlojctl get tasks
go run ./cmd/orlojctl get workers

# Run a smoke load test
go run ./cmd/orloj-loadtest \
  --base-url=http://127.0.0.1:8080 \
  --tasks=10 \
  --quality-profile=monitoring/loadtest/quality-default.json
```

## Point-in-Time Recovery

For Postgres deployments with WAL archiving enabled, you can recover to a specific point in time. This requires:

1. A base backup taken before the target recovery point.
2. Continuous WAL archiving to a durable location.
3. Postgres `recovery_target_time` configuration.

Refer to the [PostgreSQL PITR documentation](https://www.postgresql.org/docs/current/continuous-archiving.html) for setup details. Cloud-managed databases typically expose PITR as a built-in feature.

## Upgrade Safety

Before any Orloj version upgrade:

1. Take a full Postgres backup.
2. Record the current `ORLOJ_SECRET_ENCRYPTION_KEY`.
3. Record the current binary versions and configuration.
4. Proceed with the upgrade per the [Upgrades and Rollbacks](upgrades.md) guide.

If the upgrade fails, restore from the backup and revert to the previous binary version.

## Disaster Recovery Checklist

- [ ] Postgres backups run on a schedule and are verified periodically.
- [ ] Secret encryption key is stored in a secure vault, separate from backups.
- [ ] Backup retention meets your recovery point objective (RPO).
- [ ] Restore procedure has been tested in a non-production environment.
- [ ] Monitoring alerts cover backup job failures.
