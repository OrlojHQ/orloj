# AgentPolicy

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `max_tokens_per_run` (int)
- `allowed_models` ([]string)
- `blocked_tools` ([]string)
- `apply_mode` (string): `scoped` or `global`
- `target_systems` ([]string)
- `target_tasks` ([]string)

## Defaults and Validation

- `apply_mode` defaults to `scoped`.
- `apply_mode` must be `scoped` or `global`.

## status

- `phase`, `lastError`, `observedGeneration`

Example: `examples/resources/agent-policies/cost_policy.yaml`

See also: [Agent policy concepts](../../concepts/governance/agent-policy.md).
