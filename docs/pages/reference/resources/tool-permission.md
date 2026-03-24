# ToolPermission

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `tool_ref` (string): tool name reference.
- `action` (string): action name (commonly `invoke`).
- `required_permissions` ([]string)
- `match_mode` (string): `all` or `any`
- `apply_mode` (string): `global` or `scoped`
- `target_agents` ([]string): required when `apply_mode=scoped`
- `operation_rules` ([]object): per-operation-class policy verdicts.
  - `operation_class` (string): `read`, `write`, `delete`, `admin`, or `*` (wildcard). Defaults to `*`.
  - `verdict` (string): `allow`, `deny`, or `approval_required`. Defaults to `allow`.

## Defaults and Validation

- `tool_ref` defaults to `metadata.name` when omitted.
- `action` defaults to `invoke`.
- `match_mode` defaults to `all`.
- `apply_mode` defaults to `global`.
- `required_permissions` and `target_agents` are trimmed and deduplicated.
- `target_agents` must be non-empty when `apply_mode=scoped`.
- `operation_rules` values are trimmed and lowercased. Invalid `operation_class` or `verdict` values are rejected.
- When `operation_rules` is present, the authorizer evaluates the tool's `operation_classes` against the rules. The most restrictive matching verdict wins (`deny` > `approval_required` > `allow`).
- When `operation_rules` is empty, behavior is unchanged (backward-compatible binary allow/deny).

## status

- `phase`, `lastError`, `observedGeneration`

Examples: `examples/resources/tool-permissions/*.yaml`

See also: [Tool permission concepts](../../concepts/governance/tool-permission.md).
