# Model Routing

Orloj decouples agents from specific model providers through **ModelEndpoint** resources. A ModelEndpoint declares a provider, base URL, default model, and authentication -- and agents reference it by name. This lets you swap providers, manage credentials centrally, and route different agents to different models without modifying agent manifests.

## Model Endpoints

A ModelEndpoint resource configures a connection to a model provider.

```yaml
apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: openai-default
spec:
  provider: openai
  base_url: https://api.openai.com/v1
  default_model: gpt-4o-mini
  auth:
    secretRef: openai-api-key
```

### Supported Providers

| Provider | `provider` value | Default `base_url` |
|---|---|---|
| OpenAI | `openai` | `https://api.openai.com/v1` |
| Anthropic | `anthropic` | `https://api.anthropic.com/v1` |
| Azure OpenAI | `azure-openai` | (must be set explicitly) |
| Ollama | `ollama` | `http://127.0.0.1:11434` |
| Mock | `mock` | (no network calls) |

### Provider-Specific Options

Some providers require additional configuration via the `options` field:

**Anthropic:**
```yaml
spec:
  provider: anthropic
  base_url: https://api.anthropic.com/v1
  default_model: claude-3-5-sonnet-latest
  options:
    anthropic_version: "2023-06-01"
    max_tokens: "1024"
  auth:
    secretRef: anthropic-api-key
```

**Azure OpenAI:**
```yaml
spec:
  provider: azure-openai
  base_url: https://YOUR_RESOURCE_NAME.openai.azure.com
  default_model: gpt-4o-deployment
  options:
    api_version: "2024-10-21"
  auth:
    secretRef: azure-openai-api-key
```

**Ollama** (local, no auth required):
```yaml
spec:
  provider: ollama
  base_url: http://127.0.0.1:11434
  default_model: llama3.1
```

## Binding Agents to Models

Agents configure model routing through `spec.model_ref`, which points to a ModelEndpoint:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: writer-agent
spec:
  model_ref: openai-default
  prompt: |
    You are a writing agent.
```

## How Routing Works

When a worker executes an agent turn:

1. The runtime resolves the agent's referenced ModelEndpoint from `model_ref`.
2. The model gateway constructs a provider-specific API request using the endpoint's `base_url`, `default_model`, `options`, and auth credentials.
3. The request is sent to the provider and the response is returned to the agent execution loop.

ModelEndpoint references are resolved by name within the same namespace, or by `namespace/name` for cross-namespace references.

## Authentication

Model authentication is managed through Secret resources referenced by `auth.secretRef`. The simplest way to create one is the imperative CLI command:

```bash
orlojctl create secret openai-api-key --from-literal value=sk-your-api-key-here
```

Or with a YAML manifest via `orlojctl apply -f`:

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: openai-api-key
spec:
  stringData:
    value: sk-your-api-key-here
```

`stringData` values are base64-encoded into `data` during normalization and then cleared (write-only semantics). The runtime reads from `data` at execution time.

In production, you can also skip `Secret` resources entirely and inject values via environment variables (`ORLOJ_SECRET_<name>`). See [Secret Handling](../operations/security.md#secret-handling) for details.

## Governance Integration

AgentPolicy resources can restrict which models an agent is allowed to use via the `allowed_models` field:

```yaml
apiVersion: orloj.dev/v1
kind: AgentPolicy
metadata:
  name: cost-policy
spec:
  allowed_models:
    - gpt-4o
  max_tokens_per_run: 50000
```

If an agent's resolved endpoint `default_model` is not in the policy's `allowed_models` list, execution is denied.

## Related Resources

- [Resource Reference: ModelEndpoint, Secret](../reference/resources.md)
- [Configuration](../operations/configuration.md)
- [Guide: Configure Model Routing](../guides/configure-model-routing.md)
