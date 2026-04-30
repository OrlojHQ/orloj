# AgentPolicy

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `max_tokens_per_run` (int)
- `allowed_models` ([]string)
- `blocked_tools` ([]string)
- `apply_mode` (string): `scoped` or `global`
- `target_systems` ([]string)
- `target_tasks` ([]string)
- `target_agents` ([]string): when set, only listed agents are subject to this policy's constraints (model checks, blocked tools, token budget). Agents not in the list are unaffected. When empty, the policy applies to all agents in the matched systems/tasks.

## Defaults and Validation

- `apply_mode` defaults to `scoped`.
- `apply_mode` must be `scoped` or `global`.

## status

- `phase`, `lastError`, `observedGeneration`

Example: `examples/resources/agent-policies/cost_policy.yaml`

See also: [Agent policy concepts](../../concepts/governance/agent-policy.md).
