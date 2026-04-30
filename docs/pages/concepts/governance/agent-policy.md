# AgentPolicy

An **AgentPolicy** sets execution constraints on agent systems or tasks. Policies can restrict model usage, block specific tools, and cap token consumption.

## Defining an AgentPolicy

```yaml
apiVersion: orloj.dev/v1
kind: AgentPolicy
metadata:
  name: cost-policy
spec:
  apply_mode: scoped
  target_systems:
    - report-system
  max_tokens_per_run: 50000
  allowed_models:
    - gpt-4o
  blocked_tools:
    - filesystem_delete
```

### Key Fields

| Field | Description |
|---|---|
| `apply_mode` | `scoped` (default) applies only to listed targets. `global` applies to all systems/tasks. |
| `target_systems` | AgentSystem names this policy applies to (when `scoped`). |
| `target_tasks` | Task names this policy applies to (when `scoped`). |
| `target_agents` | Agent names this policy applies to. When set, only listed agents are constrained; other agents in the same system are unaffected. When empty, the policy applies to all agents. |
| `allowed_models` | Whitelist of permitted model identifiers. Agents configured with unlisted models are denied. |
| `blocked_tools` | Tools that may not be invoked under this policy, regardless of agent permissions. |
| `max_tokens_per_run` | Maximum token budget for a single task execution. When `target_agents` is set, acts as a per-agent budget for the listed agents rather than a system-wide total. |

### Per-Agent Budgets

Use `target_agents` to set different token limits for different agents in the same system:

```yaml
apiVersion: orloj.dev/v1
kind: AgentPolicy
metadata:
  name: verdict-budget
spec:
  apply_mode: scoped
  target_systems:
    - fraud-system
  target_agents:
    - verdict-agent
  max_tokens_per_run: 4000
---
apiVersion: orloj.dev/v1
kind: AgentPolicy
metadata:
  name: analyst-budget
spec:
  apply_mode: scoped
  target_systems:
    - fraud-system
  target_agents:
    - velocity-analyst
    - geo-risk-analyst
    - pattern-analyst
  max_tokens_per_run: 1500
```

The verdict agent gets 4000 tokens per run while each analyst gets 1500. Agents not listed in any `target_agents` are only constrained by policies without `target_agents` (system-wide policies).

## How It Works

AgentPolicy is the first check in the [authorization flow](./). When an agent selects a tool call, the runtime checks all applicable policies before evaluating role-based permissions:

- If the tool is in `blocked_tools`, the call is immediately denied.
- If the agent's model is not in `allowed_models`, execution is denied.
- If `max_tokens_per_run` is exceeded, execution is stopped.

A `global` policy applies to every execution. A `scoped` policy applies only to the listed `target_systems` and `target_tasks`.

## Related

- [Governance Overview](./) -- how the three governance resources work together
- [AgentRole](./agent-role.md)
- [ToolPermission](./tool-permission.md)
- [Resource Reference: AgentPolicy](../../reference/resources/agent-policy.md)
