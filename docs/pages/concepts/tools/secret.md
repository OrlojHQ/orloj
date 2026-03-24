# Secret

A **Secret** stores sensitive values (API keys, tokens, passwords) used by other resources. ModelEndpoints, Tools, McpServers, and TaskWebhooks reference Secrets for authentication.

## Defining a Secret

The simplest way to create a Secret is with the CLI:

```bash
orlojctl create secret openai-api-key --from-literal value=sk-your-api-key-here
```

Or with a YAML manifest:

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: openai-api-key
spec:
  stringData:
    value: sk-your-api-key-here
```

### Key Fields

| Field | Description |
|---|---|
| `data` | Base64-encoded key-value pairs. This is what the runtime reads at execution time. |
| `stringData` | Write-only plaintext convenience input. Entries are base64-encoded into `data` during normalization, then cleared. |

## How Secrets Work

- `stringData` entries are merged into `data` as base64 during normalization.
- Every `data` value must be non-empty valid base64.
- `stringData` is cleared after normalization (write-only behavior) -- it is never stored or returned by the API.
- Secret resolution is performed fresh per tool invocation. There is no caching of raw secret values, so rotated secrets take effect immediately.

## Environment Variable Override

In production, you can skip `Secret` resources entirely and inject values via environment variables:

```
ORLOJ_SECRET_<name>=<value>
```

See [Secret Handling](../../operations/security.md#secret-handling) for details.

## Related

- [ModelEndpoint](./model-endpoint.md) -- uses Secrets for model provider auth
- [Tool](./tool.md) -- uses Secrets for tool auth
- [McpServer](./mcp-server.md) -- uses Secrets for MCP server auth
- [Resource Reference: Secret](../../reference/resources/secret.md)
