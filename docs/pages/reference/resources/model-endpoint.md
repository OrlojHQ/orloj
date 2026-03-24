# ModelEndpoint

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `provider` (string): provider id (`openai`, `anthropic`, `azure-openai`, `ollama`, `mock`, or registry-added providers).
- `base_url` (string)
- `default_model` (string)
- `options` (map[string]string): provider-specific options.
- `auth.secretRef` (string): namespaced reference to a `Secret`.

## Defaults and Validation

- `provider` defaults to `openai` and is normalized to lowercase.
- `base_url` defaults by provider:
  - `openai` -> `https://api.openai.com/v1`
  - `anthropic` -> `https://api.anthropic.com/v1`
  - `ollama` -> `http://127.0.0.1:11434`
- `options` keys are normalized to lowercase; keys/values are trimmed.

## status

- `phase`, `lastError`, `observedGeneration`

Example: `examples/resources/model-endpoints/*.yaml`

See also: [Model endpoint concepts](../../concepts/tools/model-endpoint.md).
