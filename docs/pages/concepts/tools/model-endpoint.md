# ModelEndpoint

Orloj decouples agents from specific model providers through **ModelEndpoint** resources. A ModelEndpoint declares a provider, base URL, default model, and authentication -- and agents reference it by name. This lets you swap providers, manage credentials centrally, and route different agents to different models without modifying agent manifests.

## Defining a ModelEndpoint

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
| AWS Bedrock | `bedrock` | (SDK-managed) |
| Azure OpenAI | `azure-openai` | (must be set explicitly) |
| Ollama (native) | `ollama` | `http://127.0.0.1:11434` |
| OpenAI-compatible | `openai-compatible` | (must be set explicitly) |
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

**AWS Bedrock** (uses the Converse API via the AWS SDK):
```yaml
spec:
  provider: bedrock
  default_model: anthropic.claude-sonnet-4-20250514-v1:0
  options:
    region: us-east-1
    max_tokens: "4096"
  auth:
    secretRef: aws-credentials
```

Bedrock uses AWS IAM credentials instead of a simple API key. If `auth.secretRef` is set, the secret must contain a JSON blob with `access_key_id`, `secret_access_key`, and optionally `session_token`. If `auth.secretRef` is omitted, the AWS SDK resolves credentials from the environment (env vars, `~/.aws/credentials`, EC2/ECS IAM roles, etc.).

| Option | Description | Default |
|---|---|---|
| `region` | AWS region (required) | -- |
| `max_tokens` | Default max output tokens | `1024` |
| `profile` | AWS named profile from `~/.aws/config` | (default) |

Cross-region inference profiles (e.g. `us.anthropic.claude-sonnet-4-20250514-v1:0`) work transparently -- use the profile ID as `default_model`.

**Ollama** (native `/api/chat` endpoint, no auth required):
```yaml
spec:
  provider: ollama
  base_url: http://127.0.0.1:11434
  default_model: llama3.1
```

> **Ollama base URL tip:** For `provider: ollama`, use the server root (`http://host:11434`) and do **not** append `/v1`.

### OpenAI-Compatible Providers

The `openai-compatible` provider uses the OpenAI Chat Completions protocol (`/chat/completions`) with a custom `base_url`. This lets you connect to any service that exposes an OpenAI-compatible API. `auth.secretRef` is optional for this provider.

The following table lists tested providers and their configuration. Any service that implements the `/chat/completions` endpoint should work -- this list is not exhaustive.

| Provider | `base_url` | Example `default_model` | Auth required |
|---|---|---|---|
| Groq | `https://api.groq.com/openai/v1` | `llama-3.3-70b-versatile` | Yes |
| Together AI | `https://api.together.xyz/v1` | `meta-llama/Llama-3.3-70B-Instruct-Turbo` | Yes |
| Fireworks AI | `https://api.fireworks.ai/inference/v1` | `accounts/fireworks/models/llama-v3p3-70b-instruct` | Yes |
| Mistral AI | `https://api.mistral.ai/v1` | `mistral-large-latest` | Yes |
| DeepSeek | `https://api.deepseek.com/v1` | `deepseek-chat` | Yes |
| xAI (Grok) | `https://api.x.ai/v1` | `grok-3` | Yes |
| Google Gemini | `https://generativelanguage.googleapis.com/v1beta/openai` | `gemini-2.5-pro` | Yes |
| Perplexity | `https://api.perplexity.ai` | `sonar-pro` | Yes |
| OpenRouter | `https://openrouter.ai/api/v1` | `anthropic/claude-sonnet-4` | Yes |
| Cerebras | `https://api.cerebras.ai/v1` | `llama-4-scout-17b-16e-instruct` | Yes |
| SambaNova | `https://api.sambanova.ai/v1` | `Meta-Llama-3.3-70B-Instruct` | Yes |
| vLLM | `http://localhost:8000/v1` | (your deployed model) | No |
| text-generation-inference | `http://localhost:8080/v1` | (your deployed model) | No |
| LM Studio | `http://localhost:1234/v1` | (your loaded model) | No |
| LiteLLM proxy | `http://localhost:4000/v1` | (your configured model) | No |
| Ollama (OpenAI mode) | `http://127.0.0.1:11434/v1` | `llama3.1` | No |

**Example -- Groq:**
```yaml
spec:
  provider: openai-compatible
  base_url: https://api.groq.com/openai/v1
  default_model: llama-3.3-70b-versatile
  auth:
    secretRef: groq-api-key
```

**Example -- Google Gemini (via OpenAI-compatible endpoint):**
```yaml
spec:
  provider: openai-compatible
  base_url: https://generativelanguage.googleapis.com/v1beta/openai
  default_model: gemini-2.5-pro
  auth:
    secretRef: gemini-api-key
```

**Example -- local vLLM server:**
```yaml
spec:
  provider: openai-compatible
  base_url: http://localhost:8000/v1
  default_model: meta-llama/Llama-3.1-8B-Instruct
  allowPrivate: true
```

> **Local endpoint note:** For `provider: openai-compatible`, set `allowPrivate: true` when `base_url` points at localhost or a private network. The native `ollama` provider defaults `allowPrivate` to `true`.
>
> **Ollama note:** Ollama exposes both a native API (`/api/chat`, used by the `ollama` provider) and an OpenAI-compatible API (`/v1/chat/completions`). Use whichever suits your setup -- the `openai-compatible` provider works with Ollama's `/v1` endpoint.
>
> **Not listed here?** Any service that implements OpenAI's `/chat/completions` endpoint should work. Set `provider: openai-compatible`, point `base_url` at the service's API root, and add `auth.secretRef` if the service requires an API key.

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

## Fallback Routing

When a model provider is down or rate-limited, you can configure `fallback_model_refs` on an agent to cascade through backup endpoints automatically:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: writer-agent
spec:
  model_ref: anthropic-claude
  fallback_model_refs:
    - openai-gpt4
    - ollama-local
  prompt: |
    You are a writing agent.
```

The router tries endpoints in order -- primary first, then each fallback. The first successful response wins. Fallback is triggered only on **retryable errors**:

- **429** (rate limit)
- **5xx** (server errors: 500, 502, 503, etc.)
- Connection failures, DNS errors, and timeouts

**Non-retryable errors** (400, 401, 403, 404, etc.) fail immediately without trying fallbacks -- these indicate configuration problems that retrying with a different provider won't solve.

If all endpoints are exhausted, the last error is returned to the agent worker.

Fallback is handled entirely within the model router. The agent worker and execution engine are unaware of it -- they still call `Complete()` once per step. [AgentPolicy](../governance/agent-policy.md) governance applies independently to each endpoint in the fallback chain.

## How Routing Works

When a worker executes an agent turn:

1. The runtime resolves the agent's referenced ModelEndpoint from `model_ref`.
2. The model gateway constructs a provider-specific API request using the endpoint's `base_url`, `default_model`, `options`, and auth credentials.
3. The request is sent to the provider and the response is returned to the agent execution loop.

ModelEndpoint references are resolved by name within the same namespace, or by `namespace/name` for cross-namespace references.

## Authentication

Model authentication is managed through [Secret](./secret.md) resources referenced by `auth.secretRef`.

- `openai`, `anthropic`, and `azure-openai` require `auth.secretRef`.
- `bedrock` accepts either with or without `auth.secretRef`. When set, the secret must be a JSON blob containing `access_key_id` and `secret_access_key`. When omitted, the AWS SDK default credential chain is used (env vars, instance profiles, SSO, etc.).
- `openai-compatible` accepts either with or without `auth.secretRef`.
- `ollama` usually runs without `auth.secretRef`.

The simplest way to create a Secret is the imperative CLI command:

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

In production, you can also skip `Secret` resources entirely and inject values via environment variables (`ORLOJ_SECRET_<name>`). See [Secret Handling](../../operations/security.md#secret-handling) for details.

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

## Related

- [Secret](./secret.md) -- credential storage for model auth
- [Agent](../agents/agent.md) -- agents that reference ModelEndpoints
- [Resource Reference: ModelEndpoint](../../reference/resources/model-endpoint.md)
- [Configuration](../../operations/configuration.md)
- [Guide: Configure Model Routing](../../guides/configure-model-routing.md)
