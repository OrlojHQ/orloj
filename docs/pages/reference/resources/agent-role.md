# AgentRole

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `description` (string)
- `permissions` ([]string): normalized permission strings.

## Defaults and Validation

- `permissions` are trimmed and deduplicated (case-insensitive).

## status

- `phase`, `lastError`, `observedGeneration`

Examples: `examples/resources/agent-roles/*.yaml`

See also: [Agent role concepts](../../concepts/governance/agent-role.md).
