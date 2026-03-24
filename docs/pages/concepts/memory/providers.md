# Memory Providers

Memory providers are the backends that store and retrieve data for Orloj's built-in memory tools. There are two paths to connect a vector database, both coexisting:

**Built-in providers** -- Orloj ships Go implementations that connect directly to popular databases. Users configure `spec.endpoint` and `spec.auth.secretRef` on the Memory CRD and Orloj handles the rest. No extra infrastructure needed.

**HTTP adapter** -- For databases without a built-in provider, users deploy a lightweight adapter service that speaks a simple JSON contract and set `provider: http`. The adapter can be written in any language.

Both paths use the same CRD fields: `spec.endpoint` for the database URL and `spec.auth.secretRef` for credentials.

## Built-in Providers

| Provider | Description |
|---|---|
| `in-memory` | In-process map. No endpoint needed. Useful for testing and single-instance deployments. Data is lost on restart. |
| `pgvector` | PostgreSQL with the pgvector extension. Full vector-similarity search via embeddings. Requires `endpoint` (Postgres DSN), `embedding_model` (ModelEndpoint reference), and optionally `auth.secretRef` (Postgres password). See [pgvector](#pgvector). |
| `http` | Delegates to an external HTTP service at `spec.endpoint`. See [HTTP Adapter](#http-adapter). |

## pgvector

The `pgvector` provider stores memory entries in PostgreSQL using the [pgvector](https://github.com/pgvector/pgvector) extension. Every write generates a vector embedding, enabling true cosine-similarity search via `memory.search`.

### Requirements

- A PostgreSQL instance with the `vector` extension installed (pgvector).
- A ModelEndpoint that serves an OpenAI-compatible `/embeddings` API (OpenAI, Azure OpenAI, Ollama, or any compatible provider).

### Configuration

```yaml
apiVersion: orloj.dev/v1
kind: Memory
metadata:
  name: team-knowledge
  namespace: production
spec:
  type: vector
  provider: pgvector
  endpoint: postgres://orloj@pgvector-host:5432/memories
  embedding_model: openai-embeddings
  auth:
    secretRef: pg-password
```

The `embedding_model` field references a ModelEndpoint by name:

```yaml
apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: openai-embeddings
  namespace: production
spec:
  provider: openai
  default_model: text-embedding-3-small
  auth:
    secretRef: openai-api-key
```

| Field | Description |
|---|---|
| `endpoint` | Full Postgres connection string (DSN). Example: `postgres://user@host:5432/dbname`. |
| `embedding_model` | Name of a ModelEndpoint in the same namespace (or `namespace/name` for cross-namespace). The endpoint's `base_url` and `auth` are used to call the embeddings API, and `default_model` selects the model. |
| `auth.secretRef` | Optional. Reference to a Secret containing the Postgres password. Injected into the DSN if the connection string doesn't already include one. |

### How It Works

On creation, the memory controller:

1. Resolves the `embedding_model` ModelEndpoint and builds an embedding provider.
2. Connects to PostgreSQL using the DSN from `endpoint`.
3. Generates a test embedding to auto-detect the vector dimension.
4. Creates the `vector` extension, table, and HNSW index if they don't exist.
5. Runs `Ping` to verify connectivity.

The table schema (default table name `orloj_memory`, overridable via `spec.options.table`):

```sql
CREATE TABLE orloj_memory (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    embedding  vector(<dim>),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX orloj_memory_embedding_idx
    ON orloj_memory USING hnsw (embedding vector_cosine_ops);
```

- **`memory.write`** embeds the value and upserts the row.
- **`memory.search`** embeds the query and performs cosine-similarity search.
- **`memory.read`** and **`memory.list`** operate on key/prefix without embeddings.
- **`memory.ingest`** chunks the document and stores each chunk with its embedding.

## HTTP Adapter

When `provider: http` is set, Orloj delegates all memory operations to an external service at `spec.endpoint`. This is the escape hatch for vector databases that don't have a built-in provider yet. The adapter can be written in any language and deployed anywhere Orloj can reach over HTTP.

### Contract

The service must implement five endpoints. All POST endpoints accept and return `application/json`.

**`POST /put`** -- Store a key-value pair.

```json
// Request
{"key": "findings/chunk-0001", "value": "The quarterly report shows..."}
// Response
{"status": "ok"}
```

**`POST /get`** -- Retrieve a value by key.

```json
// Request
{"key": "findings/chunk-0001"}
// Response
{"found": true, "key": "findings/chunk-0001", "value": "The quarterly report shows..."}
```

**`POST /search`** -- Search entries by keyword or vector similarity.

```json
// Request
{"query": "quarterly revenue", "top_k": 5}
// Response
{"results": [{"key": "...", "value": "...", "score": 0.92}]}
```

**`POST /list`** -- List entries by key prefix.

```json
// Request
{"prefix": "findings/"}
// Response
{"entries": [{"key": "...", "value": "..."}]}
```

**`GET /ping`** -- Health check.

```json
// Response
{"status": "ok"}
```

Errors are signaled via HTTP status codes (4xx/5xx) with an optional `{"error": "message"}` body.

### Authentication

When `spec.auth.secretRef` is set, Orloj sends an `Authorization: Bearer <token>` header on every request. The token is resolved from the referenced Secret resource.

### Example

```yaml
apiVersion: orloj.dev/v1
kind: Memory
metadata:
  name: custom-vectordb
spec:
  provider: http
  endpoint: https://my-adapter.example.com
  auth:
    secretRef: adapter-api-key
```

## Custom Providers

For contributors adding first-party vector database support, or users building custom Orloj binaries, providers can be registered directly in Go. Implement the `PersistentMemoryBackend` interface and register a factory at startup:

```go
import agentruntime "github.com/OrlojHQ/orloj/runtime"

func init() {
    agentruntime.DefaultMemoryProviderRegistry().Register("qdrant", func(cfg agentruntime.MemoryProviderConfig) (agentruntime.PersistentMemoryBackend, error) {
        // cfg.Endpoint, cfg.AuthToken, cfg.Embedder are available
        return NewQdrantBackend(cfg)
    })
}
```

The `MemoryProviderConfig` passed to the factory contains:

| Field | Description |
|---|---|
| `Type` | The `spec.type` from the Memory CRD (e.g. `vector`, `kv`). |
| `Provider` | The `spec.provider` value that matched the registration. |
| `EmbeddingModel` | The raw `spec.embedding_model` string from the Memory CRD. |
| `Endpoint` | The `spec.endpoint` URL or connection string. |
| `AuthToken` | Resolved bearer token from `spec.auth.secretRef`. |
| `Options` | Provider-specific configuration (currently unused). |
| `Embedder` | An `EmbeddingProvider` interface (with `Embed` and `Dimensions` methods). Non-nil when `spec.embedding_model` references a valid ModelEndpoint. Vector providers should use this for generating embeddings. |

The Memory controller calls the factory, runs `Ping` to verify connectivity, and moves the resource to `Ready` if successful.
