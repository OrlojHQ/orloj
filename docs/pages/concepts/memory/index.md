# Memory

Memory gives agents the ability to store, retrieve, and search information across execution steps and across tasks. Orloj implements memory as a layered system: conversation history provides short-term context within a single task turn, a task-scoped shared store lets agents in the same task exchange state, and persistent backends retain knowledge across task runs.

## How Memory Works

When an agent has `spec.memory.ref` set to a Memory resource, the runtime attaches that memory backend to the agent. Built-in memory operations are granted explicitly through `spec.memory.allow`, and the runtime exposes only those allowed operations as callable built-in tools. They behave like tools during execution, but are handled internally by the runtime without network calls.

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research-agent
spec:
  model_ref: openai-default
  prompt: |
    You are a research assistant. Use memory tools to store and retrieve findings.
  tools:
    - web_search
  memory:
    ref: research-memory
    allow:
      - read
      - write
      - search
  limits:
    max_steps: 10
```

The `memory.ref` field points to a Memory resource that configures the backing store:

```yaml
apiVersion: orloj.dev/v1
kind: Memory
metadata:
  name: research-memory
spec:
  type: vector
  provider: in-memory
```

For vector-similarity search with PostgreSQL and pgvector:

```yaml
apiVersion: orloj.dev/v1
kind: Memory
metadata:
  name: production-memory
spec:
  type: vector
  provider: pgvector
  endpoint: postgres://orloj@pgvector-host:5432/memories
  embedding_model: openai-embeddings   # references a ModelEndpoint
  auth:
    secretRef: pg-password
```

For a custom vector database via the HTTP adapter:

```yaml
apiVersion: orloj.dev/v1
kind: Memory
metadata:
  name: custom-vectordb
spec:
  type: vector
  provider: http
  endpoint: https://my-vector-adapter.example.com
  auth:
    secretRef: vector-db-api-key
```

See [Memory Providers](./providers.md) for the full list of supported providers.

## Memory Layers

### Conversation History

Every agent accumulates a message history during multi-turn execution within a single task turn. The system prompt, user context, model responses, and tool results are all appended to the conversation and sent to the model on each step. This gives the model continuity across reasoning steps without explicit memory tool calls.

Conversation history is ephemeral -- it exists only for the duration of the agent's current activation and is not shared between agents.

### Task-Scoped Shared Memory

When no persistent backend is configured (or as a fallback), memory tools operate on an in-process key-value store scoped to the current task. All agents within the same task share this store, enabling coordination:

- Agent A writes `memory.write({"key": "findings", "value": "..."})`
- Agent B reads `memory.read({"key": "findings"})`

This store is ephemeral and cleared when the task completes.

### Persistent Backends

When a Memory resource specifies a persistent provider, memory tools delegate to the configured backend. Data written by one task is available to future tasks that reference the same Memory resource.

There are two ways to connect a vector database:

**Built-in providers** -- Orloj ships Go implementations that connect directly to popular databases. Users configure `spec.endpoint` and `spec.auth.secretRef` on the Memory CRD and Orloj handles the rest. No extra infrastructure needed.

**HTTP adapter** -- For databases without a built-in provider, users deploy a lightweight adapter service that speaks a simple JSON contract and set `provider: http`. The adapter can be written in any language.

See [Memory Providers](./providers.md) for full details on each provider, configuration examples, and how to build custom providers.

## Built-in Memory Tools

When `spec.memory.ref` is set and `spec.memory.allow` grants the corresponding operations, the runtime exposes the following built-in tools. They do not need to be listed in `spec.tools`.

### `memory.read`

Retrieve a value by key.

```json
{"key": "research-findings"}
```

Returns `{"found": true, "key": "research-findings", "value": "..."}` or `{"found": false, "key": "research-findings"}`.

### `memory.write`

Store a value under a key. Overwrites any existing value.

```json
{"key": "research-findings", "value": "The study shows..."}
```

Returns `{"status": "ok", "key": "research-findings"}`.

### `memory.search`

Search stored entries by keyword (or vector similarity when a persistent backend with embeddings is configured).

```json
{"query": "climate data", "top_k": 5}
```

Returns `{"results": [{"key": "...", "value": "...", "score": 1.0}], "count": 3}`.

### `memory.list`

List stored entries, optionally filtered by key prefix.

```json
{"prefix": "research/"}
```

Returns `{"entries": [{"key": "...", "value": "..."}], "count": 5}`.

### `memory.ingest`

Chunk a document and store the pieces for later search. Useful for loading text files, reports, or other documents into memory.

```json
{
  "source": "quarterly-report",
  "content": "Full text of the document...",
  "chunk_size": 1000,
  "overlap": 200
}
```

The tool splits the content into overlapping windows and stores each chunk under `{source}/chunk-{NNNN}`. Returns `{"status": "ok", "source": "quarterly-report", "chunks_stored": 12}`.

`chunk_size` and `overlap` are optional and default to 1000 and 200 characters respectively.

## Memory in Agent Systems

In a multi-agent system, memory enables coordination between agents without requiring direct message passing for every piece of state:

- A **research agent** writes findings to memory.
- A **writer agent** reads those findings and produces a report.
- A **coordinator agent** lists memory entries to track overall progress.

All agents that reference the same Memory resource (via `spec.memory.ref`) and execute within the same task share the same backing store.

## Memory Resource Configuration

The Memory resource is a declarative configuration. It tells the runtime which backend to use and how to configure it.

### `spec` Fields

| Field | Description |
|---|---|
| `type` | Categorization of the memory use case (e.g. `vector`, `kv`). Informational; does not affect runtime behavior in v1. |
| `provider` | Backend implementation. `in-memory` (default), `pgvector`, `http` (external adapter), or a registered built-in provider name. See [Memory Providers](./providers.md). |
| `embedding_model` | Reference to a ModelEndpoint resource that provides an embeddings API. Required for vector providers like `pgvector`. The endpoint's `base_url`, `auth`, and `default_model` are used to generate embeddings. |
| `endpoint` | URL or connection string for the database or adapter service. Required for `pgvector`, `http`, and cloud-hosted built-in providers. Not needed for `in-memory`. |
| `auth.secretRef` | Reference to a Secret resource containing credentials (API key, password, bearer token). |

### Status

The controller reconciles Memory resources and reports backend health:

- **Ready** -- backend is configured and reachable.
- **Error** -- provider is unsupported or connectivity check failed. See `status.lastError` for details.

## Frontend

The Memory detail page in the UI includes an **Entries** tab that displays stored memory entries. You can search entries by keyword and browse keys and values. This is useful for debugging agent behavior and inspecting what data has been stored.

## Related Resources

- [Memory Providers](./providers.md)
- [Resource Reference: Memory](../../reference/resources.md#memory)
- [Agents and Agent Systems](../agents-and-systems.md)
- [Architecture Overview](../../architecture/overview.md)
- [API Reference](../../reference/api.md)
