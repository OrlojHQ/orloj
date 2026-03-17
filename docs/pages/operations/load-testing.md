# Load Testing

Use `orloj-loadtest` to run repeatable reliability scenarios and enforce non-zero quality gates.

## Purpose

The load harness validates message-driven reliability behavior, including retries, dead-letter handling, and lease takeover events.

## Command Reference

```bash
go run ./cmd/orloj-loadtest --help
```

Default behavior:

- applies baseline manifests from `examples/`
- waits for at least two ready workers
- creates tasks concurrently
- waits for terminal state (`succeeded|failed|deadletter`) or timeout
- enforces quality gates (`exit 2` when gate fails)

## Baseline Run

```bash
go run ./cmd/orloj-loadtest \
  --base-url=http://127.0.0.1:8080 \
  --namespace=default \
  --tasks=200 \
  --create-concurrency=25 \
  --poll-concurrency=50 \
  --run-timeout=10m \
  --quality-profile=monitoring/loadtest/quality-default.json
```

## Failure Injection Scenarios

Invalid system dead-letter injection:

```bash
go run ./cmd/orloj-loadtest \
  --tasks=200 \
  --inject-invalid-system-rate=0.10 \
  --invalid-system-name=missing-system-loadtest
```

Retry stress scenario:

```bash
go run ./cmd/orloj-loadtest \
  --tasks=200 \
  --inject-timeout-system-rate=0.20 \
  --timeout-system-name=loadtest-timeout-system \
  --message-retry-attempts=6 \
  --message-retry-backoff=100ms \
  --message-retry-max-backoff=1s \
  --min-retry-total=50
```

Expired lease takeover simulation:

```bash
go run ./cmd/orloj-loadtest \
  --tasks=200 \
  --inject-expired-lease-rate=0.15 \
  --expired-lease-owner=worker-crashed-simulated \
  --min-takeover-events=20
```

## JSON Reporting and Exit Codes

```bash
go run ./cmd/orloj-loadtest --tasks=100 --json=true
```

- `0`: gates passed
- `2`: quality gates failed
- `1`: command/config/runtime failure

## Quality Profiles

### Default (manual validation)

- `monitoring/loadtest/quality-default.json`

For interactive or staging validation with full failure injection.

### CI (automated quality gates)

- `monitoring/loadtest/quality-ci.json`

Used by the `reliability` CI job. Runs 30 tasks with relaxed thresholds suitable for memory-backend, mock-model CI environments. No failure injection; validates baseline task flow health.

### Profile fields

- `min_success_rate`
- `max_deadletter_rate`
- `max_failed_rate`
- `max_timed_out`
- `min_retry_total`
- `min_takeover_events`

## Notes

This harness is for reliability and failure validation, not peak-throughput microbenchmarking.
