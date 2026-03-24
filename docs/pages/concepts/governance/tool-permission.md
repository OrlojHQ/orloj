# ToolPermission

A **ToolPermission** defines what permissions are required to invoke a specific [Tool](../tools/tool.md). When an agent attempts to call a tool, the runtime checks the agent's accumulated permissions against the tool's ToolPermission.

## Defining a ToolPermission

```yaml
apiVersion: orloj.dev/v1
kind: ToolPermission
metadata:
  name: web-search-invoke
spec:
  tool_ref: web_search
  action: invoke
  match_mode: all
  apply_mode: global
  required_permissions:
    - tool:web_search:invoke
    - capability:web.read
```

### Key Fields

| Field | Description |
|---|---|
| `tool_ref` | The tool this permission gate applies to. Defaults to `metadata.name`. |
| `action` | The action being gated. Defaults to `invoke`. |
| `match_mode` | `all` requires every listed permission. `any` requires at least one. |
| `apply_mode` | `global` applies to all agents. `scoped` applies only to `target_agents`. |
| `required_permissions` | Permission strings the agent must hold (via [AgentRoles](./agent-role.md)). |

## Operation Rules

ToolPermissions can define per-operation-class verdicts using `operation_rules`:

```yaml
spec:
  tool_ref: database_tool
  operation_rules:
    - operation_class: read
      verdict: allow
    - operation_class: write
      verdict: approval_required
    - operation_class: delete
      verdict: deny
```

| Verdict | Behavior |
|---|---|
| `allow` | Proceed with the tool call (default). |
| `deny` | Block the call with a `permission_denied` error. |
| `approval_required` | Pause the task and create a [ToolApproval](./tool-approval.md) resource. |

Operation classes are declared on the [Tool](../tools/tool.md) resource via `spec.operation_classes`. When multiple rules match, the most restrictive verdict wins: `deny` > `approval_required` > `allow`.

## Related

- [Governance Overview](./) -- how the three governance resources work together
- [AgentPolicy](./agent-policy.md)
- [AgentRole](./agent-role.md)
- [ToolApproval](./tool-approval.md) -- what happens when `approval_required` is triggered
- [Resource Reference: ToolPermission](../../reference/resources/tool-permission.md)
