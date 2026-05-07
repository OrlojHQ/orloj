# ModelEndpoint

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `provider` (string, required): provider id (`openai`, `anthropic`, `azure-openai`, `ollama`, `openai-compatible`, `mock`, or registry-added providers).
- `base_url` (string)
- `default_model` (string, required): the model identifier sent in API requests.
- `options` (map[string]string): provider-specific options.
- `auth.secretRef` (string): namespaced reference to a `Secret`.
- `allowPrivate` (boolean): for model gateways only, permits trusted local/private model endpoints, including loopback and RFC 1918 / ULA / CGNAT addresses. Cloud metadata, link-local, and unspecified addresses remain blocked.

## Defaults and Validation

- `provider` defaults to `openai` and is normalized to lowercase.
- `default_model` is required. Validation fails if omitted.
- `base_url` defaults by provider:
  - `openai` -> `https://api.openai.com/v1`
  - `anthropic` -> `https://api.anthropic.com/v1`
  - `ollama` -> `http://127.0.0.1:11434`
  - `openai-compatible` -> (no default; must be set explicitly)
- `options` keys are normalized to lowercase; keys/values are trimmed.
- `allowPrivate` defaults to `true` for `ollama` and `false` for all other providers. Set it to `true` for local/private `openai-compatible` servers such as vLLM, LM Studio, LocalAI, LiteLLM, or Ollama's `/v1` endpoint.
- auth behavior by provider:
  - `openai`, `anthropic`, `azure-openai`: `auth.secretRef` is required.
  - `openai-compatible`: `auth.secretRef` is optional.
  - `ollama`: `auth.secretRef` is optional and usually omitted.

## status

- `phase`, `lastError`, `observedGeneration`

Example: `examples/resources/model-endpoints/*.yaml`

See also: [Model endpoint concepts](../../concepts/tools/model-endpoint.md).
