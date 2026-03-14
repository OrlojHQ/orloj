# Load Testing (Phase 9)

## Purpose

Run repeatable message-driven reliability scenarios and enforce quality gates with non-zero exits.

The harness now supports:

- baseline throughput/lifecycle checks
- deterministic invalid-system deadletter injection
- retry-stress injection (timeout system)
- simulated worker crash/lease-expiry takeover injection
- machine-readable JSON reporting for CI/ops pipelines

## Load Harness

Command:

```bash
go run ./cmd/orloj-loadtest --help
```

Default behavior:

- applies baseline manifests from `examples/`
- waits for at least 2 ready workers
- creates 50 tasks concurrently
- polls until terminal state (`succeeded|failed|deadletter`) or timeout
- evaluates quality gates and exits `2` on gate failures

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

## Scenario 1: Invalid System Deadletters

Inject deterministic deadletters by routing some tasks to a missing system.

```bash
go run ./cmd/orloj-loadtest \
  --tasks=200 \
  --inject-invalid-system-rate=0.10 \
  --invalid-system-name=missing-system-loadtest
```

## Scenario 2: Retry Stress

Route a fraction of tasks to a timeout-focused system to force retries/deadletters.

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

## Scenario 3: Simulated Worker Crash + Lease Expiry Takeover

Patch a fraction of created tasks as `Running` with an expired lease owner so workers must take over.

```bash
go run ./cmd/orloj-loadtest \
  --tasks=200 \
  --inject-expired-lease-rate=0.15 \
  --expired-lease-owner=worker-crashed-simulated \
  --min-takeover-events=20
```

## JSON Report for CI/Ops

```bash
go run ./cmd/orloj-loadtest \
  --tasks=100 \
  --json=true
```

Exit codes:

- `0`: gates passed
- `2`: one or more quality gates failed
- `1`: command/config/runtime failure

## Quality Profile

Default profile artifact:

- `monitoring/loadtest/quality-default.json`

Profile fields:

- `min_success_rate`
- `max_deadletter_rate`
- `max_failed_rate`
- `max_timed_out`
- `min_retry_total`
- `min_takeover_events`

## Notes

- This harness validates reliability and failure behavior; it is not a microbenchmark throughput suite.
- For stable results, use Postgres + message-driven workers + fixed worker counts.
