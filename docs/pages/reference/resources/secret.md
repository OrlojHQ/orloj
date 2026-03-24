# Secret

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `data` (map[string]string): base64-encoded values.
- `stringData` (map[string]string): write-only plaintext convenience input.

## Defaults and Validation

- `stringData` entries are merged into `data` as base64 during normalization.
- Every `data` value must be non-empty valid base64.
- `stringData` is cleared after normalization (write-only behavior).

## status

- `phase`, `lastError`, `observedGeneration`

Examples: `examples/resources/secrets/*.yaml`

See also: [Secret concepts](../../concepts/tools/secret.md).
