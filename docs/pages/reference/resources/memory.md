# Memory

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

A Memory resource configures a persistent memory backend that agents can read from and write to using built-in memory tools. See [Memory Concepts](../../concepts/memory/index.md) for a full overview.

## spec

- `type` (string): categorization of the memory use case (e.g. `vector`, `kv`). Informational in v1.
- `provider` (string): backend implementation. Built-in values:
  - `in-memory` (default): in-process key-value store. No endpoint needed. Data is lost on restart.
  - `pgvector`: PostgreSQL with the pgvector extension. Full vector-similarity search. Requires `endpoint` (Postgres DSN) and `embedding_model` (ModelEndpoint reference). See [pgvector](../../concepts/memory/providers.md#pgvector).
  - `http`: delegates to an external HTTP service. Requires `endpoint`. See [HTTP Adapter](../../concepts/memory/providers.md#http-adapter).
  - Custom providers can be registered via the Go provider registry.
- `embedding_model` (string): reference to a ModelEndpoint resource that provides an OpenAI-compatible `/embeddings` API. Required for vector providers like `pgvector`. The endpoint's `base_url`, `auth`, and `default_model` are used to generate embeddings. Resolved in the same namespace by default; use `namespace/name` for cross-namespace references.
- `endpoint` (string): connection string or URL. For `pgvector`, a Postgres DSN (e.g. `postgres://user@host:5432/db`). For `http`, the adapter service URL. Not needed for `in-memory`.
- `auth` (object):
  - `secretRef` (string): reference to a Secret resource containing credentials. For `http`, used as a bearer token. For `pgvector`, injected as the Postgres password into the DSN.

### Built-in Memory Tools

When an Agent references a Memory resource via `spec.memory.ref` and explicitly grants operations with `spec.memory.allow`, the runtime exposes the following built-in tools:

| Tool | Description |
|---|---|
| `memory.read` | Retrieve a value by key. |
| `memory.write` | Store a key-value pair. |
| `memory.search` | Search entries by keyword (or vector similarity). |
| `memory.list` | List entries, optionally filtered by key prefix. |
| `memory.ingest` | Chunk a document into overlapping segments and store them. |

These tools do not need to be listed in the agent's `spec.tools` -- they are injected automatically.

## Defaults and Validation

- `provider` defaults to `in-memory` when omitted or empty.
- `endpoint` is required when `provider` is `pgvector`, `http`, or any cloud-hosted built-in provider.
- `embedding_model` is required when `provider` is `pgvector`. It must reference a valid ModelEndpoint.
- When `auth.secretRef` is set, the controller resolves the Secret and passes the token to the provider.
- The Memory controller validates the provider, resolves auth, and performs a connectivity check (`Ping`). Unsupported providers, missing secrets, or failed connectivity moves the resource to `Error` phase.

## status

- `phase`: `Pending`, `Ready`, or `Error`.
- `lastError`: description of the most recent error (e.g. unsupported provider, connectivity failure).
- `observedGeneration`

Example: `examples/resources/memories/research_memory.yaml`

See also: [Memory concepts](../../concepts/memory/).
