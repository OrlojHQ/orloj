# ModelEndpoint

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `provider` (string, required): provider id (`openai`, `anthropic`, `azure-openai`, `ollama`, `openai-compatible`, `mock`, or registry-added providers).
- `base_url` (string)
- `default_model` (string, required): the model identifier sent in API requests.
- `options` (map[string]string): provider-specific options.
- `auth.secretRef` (string): namespaced reference to a `Secret`.

## Defaults and Validation

- `provider` defaults to `openai` and is normalized to lowercase.
- `default_model` is required. Validation fails if omitted.
- `base_url` defaults by provider:
  - `openai` -> `https://api.openai.com/v1`
  - `anthropic` -> `https://api.anthropic.com/v1`
  - `ollama` -> `http://127.0.0.1:11434`
  - `openai-compatible` -> (no default; must be set explicitly)
- `options` keys are normalized to lowercase; keys/values are trimmed.

## status

- `phase`, `lastError`, `observedGeneration`

Example: `examples/resources/model-endpoints/*.yaml`

See also: [Model endpoint concepts](../../concepts/tools/model-endpoint.md).
