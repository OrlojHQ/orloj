# Operations Runbook

## Typical Production Topology

1. `orlojd` control plane
2. Postgres state backend
3. NATS JetStream for message-driven execution
4. multiple `orlojworker` instances

## Recommended Startup Pattern

1. Start Postgres and NATS.
2. Start `orlojd` with `--storage-backend=postgres` and `--task-execution-mode=message-driven`.
3. Start at least two workers with `--agent-message-consume`.
4. Configure task model gateway (`--model-gateway-provider=openai|anthropic|azure-openai|ollama`) and inject API key via env/secret.
5. Apply CRDs (`Memory`, `ModelEndpoint`, `Tool`, `Secret`, `Agent`, `AgentSystem`, `AgentPolicy`, `Task`).
6. Watch `/v1/events/watch` and `/v1/tasks/{name}/messages` for lifecycle behavior.

## Failure and Recovery Expectations

- Worker crash: task lease expires and another worker can take over.
- Message retry: delayed requeue by message policy until success or dead-letter.
- Policy/graph validation errors: classified non-retryable and dead-lettered.
- Tool runtime failures/denials emit normalized metadata (`tool_contract_version`, `tool_request_id`, `tool_attempt`, `tool_code`, `tool_reason`, `retryable`) in trace/log paths.

## Load/Scale Guidance

- Use Postgres for shared durable state.
- Keep worker count > 1 for takeover resilience.
- Monitor retry bursts and dead-letter growth as primary early-warning signals.
- Run `go run ./cmd/orloj-loadtest` for repeatable Phase 9 load/failure scenarios.
- Use `--inject-timeout-system-rate` for retry-stress and `--inject-expired-lease-rate` for takeover simulation.
- Use `--quality-profile=monitoring/loadtest/quality-default.json` (or explicit gate flags) for non-zero SLO enforcement.
- Run `go run ./cmd/orloj-alertcheck` against your namespace to evaluate retry/deadletter alert thresholds.
- Keep profile thresholds in `monitoring/alerts/retry-deadletter-default.json` aligned with SLO targets.
