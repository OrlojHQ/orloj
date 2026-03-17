# Observability

Orloj provides built-in observability through OpenTelemetry tracing, Prometheus metrics, structured logging, and an in-app trace visualization UI. These features work out of the box in OSS deployments and integrate with standard observability backends.

## Trace Visualization (Web Console)

The web console includes a **Trace** tab on every task detail page. It renders the `TaskTraceEvent` data that the runtime already records during execution.

To view a task trace:

1. Open the web console at `http://<orlojd-address>/ui/`.
2. Navigate to a task and click into its detail page.
3. Click the **Trace** tab.

The trace view shows:

- **Summary bar** -- total events, cumulative latency, token count, tool calls, and error count.
- **Waterfall timeline** -- each row is one trace event (agent start/end, tool call, model call, error, dead-letter). The horizontal bar shows time offset from task start and duration.
- **Filters** -- filter by agent or branch when the task fans out across multiple agents.
- **Expandable detail rows** -- click any row to see step ID, attempt, branch, tool name, tokens, error code/reason, and the full message.

The trace data comes from `GET /v1/tasks/{name}` (the `status.trace` field). No additional backend is required -- trace events are stored alongside the task resource.

## OpenTelemetry Tracing

Orloj emits OpenTelemetry spans for task execution, agent steps, and message processing. Spans are exported via OTLP gRPC to any compatible backend (Jaeger, Grafana Tempo, Datadog, Honeycomb, etc.).

### Enabling OTel Export

Set the OTLP endpoint via environment variable:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
```

Or for non-TLS backends in development:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_EXPORTER_OTLP_INSECURE=true
```

Both `orlojd` and `orlojworker` initialize the OTel trace provider on startup. When no endpoint is configured, a no-op provider is installed and tracing has zero overhead.

### Span Hierarchy

Spans follow the task execution structure:

```
task.execute (root span)
├── agent.execute (one per agent step)
│   ├── model.call (model gateway invocations)
│   └── tool.execute (tool runtime calls)
└── ...
```

For message-driven execution, each message consumption creates a `message.process` span with a nested `agent.execute` span.

### Span Attributes

All spans carry `orloj.*` attributes:

| Attribute | Description |
|---|---|
| `orloj.task` | Task resource name |
| `orloj.system` | AgentSystem resource name |
| `orloj.namespace` | Resource namespace |
| `orloj.agent` | Agent resource name |
| `orloj.step_id` | Step identifier (e.g. `a1.s3`) |
| `orloj.attempt` | Current attempt number |
| `orloj.tokens.used` | Tokens consumed by this step |
| `orloj.tokens.estimated` | Estimated tokens (when exact count unavailable) |
| `orloj.tool_calls` | Number of tool invocations |
| `orloj.latency_ms` | Step duration in milliseconds |
| `orloj.message_id` | Message ID (message-driven mode) |
| `orloj.from_agent` | Source agent for message handoff |
| `orloj.to_agent` | Destination agent for message handoff |
| `orloj.branch_id` | Branch ID for fan-out tracking |
| `orloj.tool` | Tool name |
| `orloj.tool.attempt` | Tool retry attempt |
| `orloj.model` | Model identifier |

### W3C Trace Context

Orloj propagates `traceparent` and `tracestate` headers using the W3C Trace Context standard. This means external tools that support W3C propagation will automatically appear as child spans in your traces.

### Dual Write

OTel spans are emitted in parallel with the internal `Task.status.trace` events. The internal trace powers the web console trace tab, while OTel spans flow to your external tracing backend. Both views are consistent.

## Prometheus Metrics

Orloj exposes a standard Prometheus scrape endpoint at `/metrics` on the `orlojd` HTTP server. The endpoint is unauthenticated (like `/healthz`) so Prometheus can scrape it without API tokens.

### Available Metrics

| Metric | Type | Labels | Description |
|---|---|---|---|
| `orloj_task_duration_seconds` | histogram | `namespace`, `system`, `status` | End-to-end task duration |
| `orloj_agent_step_duration_seconds` | histogram | `agent`, `step_type` | Duration of a single agent step |
| `orloj_tokens_used_total` | counter | `agent`, `model`, `type` | Tokens consumed (`type` = `used` or `estimated`) |
| `orloj_messages_total` | counter | `phase`, `agent` | Message lifecycle transitions |
| `orloj_deadletters_total` | counter | `agent` | Messages moved to dead-letter |
| `orloj_retries_total` | counter | `agent` | Message retry count |
| `orloj_inflight_messages` | gauge | `agent` | Currently in-flight messages |

### Prometheus Scrape Configuration

```yaml
scrape_configs:
  - job_name: orloj
    static_configs:
      - targets: ['orlojd:8080']
    metrics_path: /metrics
    scrape_interval: 15s
```

### Example Queries

Task success rate over the last hour:

```promql
sum(rate(orloj_task_duration_seconds_count{status="succeeded"}[1h]))
/
sum(rate(orloj_task_duration_seconds_count[1h]))
```

Token consumption by agent:

```promql
sum by (agent) (rate(orloj_tokens_used_total{type="used"}[5m]))
```

Dead-letter rate by agent:

```promql
sum by (agent) (rate(orloj_deadletters_total[5m]))
```

P95 agent step latency:

```promql
histogram_quantile(0.95, sum by (le, agent) (rate(orloj_agent_step_duration_seconds_bucket[5m])))
```

## Structured Logging

Both `orlojd` and `orlojworker` emit structured JSON logs by default. Log output can be configured via the `ORLOJ_LOG_FORMAT` environment variable.

### Configuration

| Variable | Values | Default | Description |
|---|---|---|---|
| `ORLOJ_LOG_FORMAT` | `json`, `text` | `json` | Log output format. Use `text` for local development. |

### Log Fields

All log entries include a `service` field (`orlojd` or `orlojworker`). When processing HTTP requests, entries also include:

- `request_id` -- unique ID for the request (propagated from `X-Request-ID` header or auto-generated)

When OpenTelemetry is enabled, log entries from traced code paths include:

- `trace_id` -- OTel trace ID for correlation with spans
- `span_id` -- OTel span ID

### Request ID Propagation

The HTTP server automatically generates a request ID for each incoming request and returns it in the `X-Request-ID` response header. If the client sends an `X-Request-ID` header, it is reused. This enables end-to-end request correlation across services.

### Correlating Logs with Traces

In Grafana, you can use the `trace_id` field to link from a log entry directly to the corresponding trace in Tempo or Jaeger. The trace ID in logs matches the OTel trace ID in exported spans.

## Docker Compose Example

To run Orloj with Jaeger and Prometheus in a local development stack:

```yaml
services:
  jaeger:
    image: jaegertracing/jaeger:2
    ports:
      - "16686:16686"  # Jaeger UI
      - "4317:4317"    # OTLP gRPC

  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"

  orlojd:
    image: orloj:latest
    command: >
      orlojd --embedded-worker
      --model-gateway-provider=openai
    environment:
      OTEL_EXPORTER_OTLP_ENDPOINT: jaeger:4317
      OTEL_EXPORTER_OTLP_INSECURE: "true"
      ORLOJ_LOG_FORMAT: json
    ports:
      - "8080:8080"
```

With the corresponding `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: orloj
    static_configs:
      - targets: ['orlojd:8080']
    scrape_interval: 15s
```

## CLI Trace Inspection

For operators who prefer the CLI, `orlojctl trace task` prints the full trace timeline:

```bash
go run ./cmd/orlojctl trace task my-task
```

This is useful for quick debugging without opening the web console or an external tracing backend.

## Related Docs

- [Monitoring and Alerts](./monitoring-alerts.md) -- `orloj-alertcheck` threshold profiles and dashboard contracts
- [Configuration](./configuration.md) -- all environment variables and CLI flags
- [Troubleshooting](./troubleshooting.md) -- diagnosis workflows
- [Runbook](./runbook.md) -- production operations
- [Extension Contracts](../reference/extensions.md) -- `MeteringSink` and `AuditSink` for custom integrations
- [Enterprise Observability Boundary](../boundaries/observability-enterprise.BOUNDARY.md) -- commercial features roadmap
