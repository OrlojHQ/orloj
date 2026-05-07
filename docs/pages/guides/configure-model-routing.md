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

Each provider needs a Secret resource to hold its API key. The fastest way is the CLI -- no YAML file needed:

```bash
orlojctl create secret openai-api-key --from-literal value=sk-your-openai-key-here
orlojctl create secret anthropic-api-key --from-literal value=sk-ant-your-anthropic-key-here
```

Alternatively, use YAML manifests with `orlojctl apply -f`:

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: openai-api-key
spec:
  stringData:
    value: sk-your-openai-key-here
```

> **Production note:** Enable `--secret-encryption-key` on `orlojd` and `orlojworker` to encrypt secret data at rest in the database, or use environment variables (`ORLOJ_SECRET_openai_api_key`) / an external secret manager. See [Security and Isolation](../operations/security.md#secret-handling) for details.

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

## Adding AWS Bedrock

Bedrock uses IAM credentials instead of a simple API key. For development, store explicit credentials as a JSON blob in a Secret:

```bash
orlojctl create secret aws-credentials --from-literal value='{"access_key_id":"AKIA...","secret_access_key":"..."}'
```

Then create the ModelEndpoint:

```yaml
apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: bedrock-claude
spec:
  provider: bedrock
  default_model: anthropic.claude-sonnet-4-20250514-v1:0
  options:
    region: us-east-1
    max_tokens: "4096"
  auth:
    secretRef: aws-credentials
```

For production deployments on AWS infrastructure (EC2, ECS, Lambda), you can omit `auth.secretRef` entirely and let the AWS SDK resolve credentials from the instance's IAM role:

```yaml
apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: bedrock-production
spec:
  provider: bedrock
  default_model: anthropic.claude-sonnet-4-20250514-v1:0
  options:
    region: us-east-1
```

Cross-region inference profiles (e.g. `us.anthropic.claude-sonnet-4-20250514-v1:0`) work transparently -- use the profile ID as `default_model`.

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

Use the Ollama server root as `base_url` for `provider: ollama`. Do not append `/v1` in this mode.

## Using OpenAI-Compatible Providers

The `openai-compatible` provider lets you connect to any service that speaks the OpenAI Chat Completions protocol. Set `base_url` to the provider's API base. `auth.secretRef` is optional.

This covers a wide range of providers: Groq, Together AI, Fireworks AI, Mistral AI, DeepSeek, xAI (Grok), Google Gemini, Perplexity, OpenRouter, Cerebras, SambaNova, vLLM, text-generation-inference, LM Studio, and LiteLLM. See [ModelEndpoint > OpenAI-Compatible Providers](../concepts/tools/model-endpoint.md#openai-compatible-providers) for the full list with `base_url` values and example models.

**Groq:**
```yaml
apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: groq-default
spec:
  provider: openai-compatible
  base_url: https://api.groq.com/openai/v1
  default_model: llama-3.3-70b-versatile
  auth:
    secretRef: groq-api-key
```

**Mistral AI:**
```yaml
apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: mistral-default
spec:
  provider: openai-compatible
  base_url: https://api.mistral.ai/v1
  default_model: mistral-large-latest
  auth:
    secretRef: mistral-api-key
```

**Google Gemini:**
```yaml
apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: gemini-default
spec:
  provider: openai-compatible
  base_url: https://generativelanguage.googleapis.com/v1beta/openai
  default_model: gemini-2.5-pro
  auth:
    secretRef: gemini-api-key
```

**Ollama via OpenAI-compatible endpoint:**
```yaml
apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: ollama-openai
spec:
  provider: openai-compatible
  base_url: http://127.0.0.1:11434/v1
  default_model: llama3.1
  allowPrivate: true
```

## Agent Requirement

Agents must set `spec.model_ref` to a valid ModelEndpoint.

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

- [Model Routing](../concepts/tools/model-endpoint.md) -- deeper dive into ModelEndpoint configuration
- [Configuration](../operations/configuration.md) -- environment variables and flags for model gateway setup
- [Build a Custom Tool](./build-custom-tool.md) -- extend agent capabilities with external tools
