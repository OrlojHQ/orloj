# Configure Model Routing

This guide is for platform engineers who need to route agents to different model providers. You will set up ModelEndpoints for multiple providers, bind agents to endpoints by reference, and verify that requests route correctly.

## Prerequisites

- Orloj server (`orlojd`) and at least one worker running
- API keys for the providers you want to configure
- `orlojctl` available

## What You Will Build

A multi-provider setup where different agents route to different model providers:
- A research agent using OpenAI's GPT-4o
- A writer agent using Anthropic's Claude

## Step 1: Create Secrets for API Keys

Each provider needs a Secret resource to hold its API key:

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: openai-api-key
spec:
  stringData:
    value: sk-your-openai-key-here
```

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: anthropic-api-key
spec:
  stringData:
    value: sk-ant-your-anthropic-key-here
```

Apply both:
```bash
orlojctl apply -f openai_api_key.yaml
orlojctl apply -f anthropic_api_key.yaml
```

The `stringData.value` is base64-encoded into `data` during normalization and then cleared. The runtime reads credentials from `data` at execution time.

> **Production note:** Secret resources are convenient for development but store values in the database without encryption at rest. In production, use environment variables instead (`ORLOJ_SECRET_openai_api_key`, `ORLOJ_SECRET_anthropic_api_key`) or an external secret manager. The resolver chain tries the resource store first, then env vars. See [Security and Isolation](../operations/security.md#secret-handling) for details.

## Step 2: Create Model Endpoints

**OpenAI endpoint:**
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

**Anthropic endpoint:**
```yaml
apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: anthropic-default
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

Apply both:
```bash
orlojctl apply -f openai_default.yaml
orlojctl apply -f anthropic_default.yaml
```

Verify they are ready:
```bash
orlojctl get model-endpoints
```

## Step 3: Bind Agents to Endpoints

Use `spec.model_ref` to point each agent at its ModelEndpoint:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research-agent
spec:
  model_ref: openai-default
  prompt: |
    You are a research assistant.
    Produce concise evidence-backed answers.
  limits:
    max_steps: 6
    timeout: 30s
```

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: writer-agent
spec:
  model_ref: anthropic-default
  prompt: |
    You are a writing agent.
    Produce clear, concise final output from provided research.
  limits:
    max_steps: 4
    timeout: 20s
```

Apply:
```bash
orlojctl apply -f research-agent.yaml
orlojctl apply -f writer-agent.yaml
```

When these agents execute, the model gateway resolves their `model_ref` to the corresponding ModelEndpoint, then constructs provider-specific API requests using the endpoint's `base_url`, `default_model`, `options`, and auth credentials.

## Step 4: Verify Routing

Submit a task that uses these agents and check the logs:

```bash
orlojctl apply -f task.yaml
orlojctl logs task/your-task-name
```

In the task trace, you should see model requests routing to the appropriate providers based on each agent's `model_ref`.

## Adding Azure OpenAI

Azure OpenAI requires an explicit `base_url` and an `api_version` option:

```yaml
apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: azure-openai-default
spec:
  provider: azure-openai
  base_url: https://YOUR_RESOURCE_NAME.openai.azure.com
  default_model: gpt-4o-deployment
  options:
    api_version: "2024-10-21"
  auth:
    secretRef: azure-openai-api-key
```

## Adding Ollama (Local Models)

For local model inference with no API key required:

```yaml
apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: ollama-default
spec:
  provider: ollama
  base_url: http://127.0.0.1:11434
  default_model: llama3.1
```

## Using Direct Model References

For simpler setups where you do not need provider abstraction, agents can set `spec.model` directly:

```yaml
spec:
  model: gpt-4o
```

The runtime uses the default provider configuration. If neither `model` nor `model_ref` is set, the agent defaults to `gpt-4o-mini`.

## Constraining Models with Policy

To restrict which models agents can use, create an AgentPolicy with `allowed_models`:

```yaml
apiVersion: orloj.dev/v1
kind: AgentPolicy
metadata:
  name: cost-policy
spec:
  allowed_models:
    - gpt-4o
    - claude-3-5-sonnet-latest
  max_tokens_per_run: 50000
```

Agents configured with models not on this list will be denied at execution time.

## Next Steps

- [Model Routing](../concepts/model-routing.md) -- deeper dive into ModelEndpoint configuration
- [Configuration](../operations/configuration.md) -- environment variables and flags for model gateway setup
- [Build a Custom Tool](./build-custom-tool.md) -- extend agent capabilities with external tools
