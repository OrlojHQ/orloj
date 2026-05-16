# Troubleshooting

Use this page for deterministic diagnosis and remediation of common failures.

## First Checks

```bash
curl -s http://127.0.0.1:8080/healthz | jq .
go run ./cmd/orlojctl get workers
go run ./cmd/orlojctl get tasks
```

If these checks fail, inspect `orlojd` and `orlojworker` logs first.

## Common Issues

### `postgres backend selected but --postgres-dsn is empty`

Cause:

- `--storage-backend=postgres` is set without DSN.

Fix:

```bash
export ORLOJ_POSTGRES_DSN='postgres://orloj:orloj@127.0.0.1:5432/orloj?sslmode=disable'
```

### Unsupported backend values

Cause:

- invalid value for storage/event/message/tool-isolation backend flags.

Fix:

- storage: `memory|postgres`
- event bus (`orlojd`): `memory|nats`
- runtime message bus: `none|memory|nats-jetstream`
- tool isolation: `none|container|wasm`

### Workers never claim tasks

Checks:

- worker is `Ready` and heartbeating
- execution mode matches deployment mode
- model provider/auth is valid
- task requirements (`region`, `gpu`, `model`) match worker capabilities

Commands:

```bash
go run ./cmd/orlojctl get workers
go run ./cmd/orlojctl get tasks
go run ./cmd/orlojctl trace task <task-name>
```

### Message-driven flow not progressing

Cause:

- worker consumer is not enabled.

Fix:

- set `--agent-message-consume`
- set non-`none` `--agent-message-bus-backend`

### Tool calls fail with permission denials

Cause:

- governance policy denies requested action.

Fix:

- validate `Agent.spec.roles`, `AgentRole`, and `ToolPermission`.
- inspect `tool_code`, `tool_reason`, and `retryable` in trace metadata.

### Model provider auth failures

Cause:

- missing or invalid API key on the ModelEndpoint resource.

Fix:

- verify `auth.secretRef` is set for providers that require auth (`openai`, `anthropic`, `azure-openai`).
- for `openai-compatible`, auth is optional, but if `auth.secretRef` is set, verify that Secret exists and is valid.
- if you use env-based secret resolution, set `ORLOJ_SECRET_<name>` (or your configured prefix) to match the `secretRef` value.

### Wasm/container runtime errors

Cause:

- missing runtime binary/module path or invalid runtime configuration.

Fix:

- verify backend-specific settings (container runtime settings or wasm module/runtime configuration).

## Observability Diagnostics

### Logs are unstructured or missing request IDs

Cause:

- `ORLOJ_LOG_FORMAT` is not set or binary predates the structured logging migration.

Fix:

- Set `ORLOJ_LOG_FORMAT=json` (default) to emit structured JSON logs with `request_id`, `trace_id`, and `span_id` fields.
- Set `ORLOJ_LOG_FORMAT=text` for human-readable output during local development.

### Traces not appearing in Jaeger/Tempo

Cause:

- `OTEL_EXPORTER_OTLP_ENDPOINT` is not set or the backend is unreachable.

Fix:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_EXPORTER_OTLP_INSECURE=true  # for non-TLS dev backends
```

Restart `orlojd` and `orlojworker`. Verify spans appear in the backend UI.

### Prometheus `/metrics` returning 404

Cause:

- Running a build that predates the metrics endpoint addition.

Fix:

- Rebuild from the latest source and verify `curl http://127.0.0.1:8080/metrics` returns Prometheus text output.

### Correlating a log entry with a trace

Use the `trace_id` field from a JSON log entry to search in your tracing backend:

```bash
# Find trace ID in logs
grep '"trace_id"' /var/log/orlojd.log | head -5
```

Then search for that trace ID in Jaeger, Tempo, or the web console Trace tab.

## Operator

### CRD applied but resource not appearing in Orloj

Cause:

- The operator failed to upsert the resource into Postgres.

Checks:

- Inspect CRD status: `kubectl get agent my-agent -o jsonpath='{.status}'`
- Look for `phase: SyncError` and read `syncError`.
- Check operator logs: `kubectl -n orloj logs deploy/orloj-operator --tail=200`
- Verify the operator has connectivity to Postgres (same DSN as `orlojd`).

Fix:

- Resolve the validation or connectivity error. The operator will retry automatically.

### Namespace stuck in Terminating (stale finalizer)

Cause:

- The operator was uninstalled or is down while CRD resources with the `orloj.dev/sync` finalizer still exist. Kubernetes cannot delete the namespace because the finalizer is never removed.

Fix:

- Redeploy the operator so it can process deletions and remove finalizers, or manually patch the finalizer off each stuck resource:

```bash
kubectl patch agent my-agent -p '{"metadata":{"finalizers":null}}' --type=merge
```

### REST API returns `X-Orloj-CRD-Managed` header warning

Cause:

- You are updating a resource via the REST API (or `orlojctl apply`) that was originally created by the CRD operator. The `--crd-conflict-policy=warn` mode is active.

Fix:

- Update the resource via `kubectl apply` or your Git repo instead. The operator will overwrite REST changes on its next reconcile.
- To enforce this, set `--crd-conflict-policy=reject` on `orlojd`.

### Resource adopted unexpectedly

Cause:

- You `kubectl apply`'d a CRD manifest whose `metadata.name` and `metadata.namespace` match an existing REST-created resource. The operator upserted it, adding the `orloj.dev/managed-by: crd-sync` annotation.

Fix:

- This is expected behavior. Once a CRD with a matching name is applied, the operator takes ownership. Delete the CRD to return to REST-only management, or keep the CRD as the source of truth.

### CRD status stuck on old generation

Cause:

- The status writer runs on a periodic interval (`--status-sync-interval`, default 5s). Immediately after apply, the status may lag.

Checks:

- Wait for one status sync interval and re-check: `kubectl get agent my-agent -o jsonpath='{.status.observedGeneration}'`
- Verify the operator pod is running and the leader election lease is held.

Fix:

- If status never updates, check operator logs for errors writing the status subresource.

## Escalation Workflow

1. Capture failing command and exact error text.
2. Capture task trace:

```bash
go run ./cmd/orlojctl trace task <task-name>
```

3. Capture recent events:

```bash
go run ./cmd/orlojctl events --once --timeout=30s --raw
```

4. Capture relevant Prometheus metrics (if applicable):

```bash
curl -s http://127.0.0.1:8080/metrics | grep orloj_
```

5. File an issue with logs, trace, metrics, and manifest snippets.
